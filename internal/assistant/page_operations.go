package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type PageOperation struct {
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	SnapshotID string     `json:"snapshotId"`
	TargetRef  string     `json:"targetRef"`
	Action     string     `json:"action"`
	Value      *string    `json:"value,omitempty"`
	OptionRef  string     `json:"optionRef,omitempty"`
}
type PageOperationResult struct {
	State      string `json:"state"`
	SnapshotID string `json:"snapshotId"`
}
type pendingPageOperation struct {
	operation PageOperation
	revision  uint64
	ctx       context.Context
	claimed   bool
	response  chan PageOperationResult
}
type pageOperatorKey struct{}

func CurrentPageOperator(ctx context.Context) ProposalHandler {
	h, _ := ctx.Value(pageOperatorKey{}).(ProposalHandler)
	return h
}
func (r *Runtime) pageOperationAllowed(ctx context.Context, epoch string, revision uint64) bool {
	return ctx.Err() == nil && r.valid(ctx, epoch) == nil && r.state.Revision == revision && r.state.Config.Enabled && r.fullEnabled && !r.anyPendingReview() && len(r.reviewBusy) == 0
}
func (r *Runtime) PendingPageOperations(ctx context.Context, epoch string) ([]PageReadRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.valid(ctx, epoch) != nil {
		return nil, ErrStale
	}
	out := []PageReadRequest{}
	for id, p := range r.pageOperations {
		if !p.claimed && r.pageOperationAllowed(p.ctx, epoch, p.revision) {
			out = append(out, PageReadRequest{ID: id})
		}
	}
	return out, nil
}

// Claim exactly once, immediately before frontend dispatch. Polling and remounts
// cannot replay a claimed command; cancellation before claim is fail-closed.
func (r *Runtime) ClaimPageOperation(ctx context.Context, epoch, id string) (PageOperation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pageOperations[id]
	if !ok || p.claimed || ctx.Err() != nil || !r.pageOperationAllowed(p.ctx, epoch, p.revision) {
		return PageOperation{}, ErrStale
	}
	if p.operation.ExpiresAt != nil && !time.Now().Before(*p.operation.ExpiresAt) {
		return PageOperation{}, ErrStale
	}
	p.claimed = true
	if p.operation.ExpiresAt == nil || time.Now().Add(2*time.Second).Before(*p.operation.ExpiresAt) {
		expires := time.Now().Add(2 * time.Second)
		p.operation.ExpiresAt = &expires
	}
	r.pageOperations[id] = p
	return p.operation, nil
}
func (r *Runtime) DeliverPageOperation(ctx context.Context, epoch, id string, result PageOperationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pageOperations[id]
	if !ok || !p.claimed || r.valid(ctx, epoch) != nil || p.ctx.Err() != nil || p.revision != r.state.Revision || result.SnapshotID != p.operation.SnapshotID {
		return ErrStale
	}
	switch result.State {
	case "dispatched", "blocked", "stale", "invalid_target", "unverified", "not_dispatched":
	default:
		return ErrUnavailable
	}
	delete(r.pageOperations, id)
	p.response <- result
	return nil
}
func (r *Runtime) pageOperator(runCtx context.Context, epoch string, revision uint64) ProposalHandler {
	var mu sync.Mutex
	uncertain := false
	used := map[string]bool{}
	return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		release, err := acquireRunWrite(runCtx)
		if err != nil {
			return nil, err
		}
		defer func() { release(uncertain) }()
		mu.Lock()
		defer mu.Unlock()
		if uncertain || runCtx.Err() != nil {
			return nil, ErrStale
		}
		var op PageOperation
		var fields map[string]json.RawMessage
		if len(args) > 32<<10 || jsoncodec.Unmarshal(args, &fields) != nil || fields == nil || jsoncodec.Unmarshal(args, &op) != nil || !canonicalID(op.SnapshotID) || !canonicalID(op.TargetRef) {
			return nil, ErrUnavailable
		}
		for k, v := range fields {
			if string(v) == "null" || k != "snapshotId" && k != "targetRef" && k != "action" && k != "value" && k != "optionRef" {
				return nil, ErrUnavailable
			}
		}
		switch op.Action {
		case "activate":
			if op.Value != nil || op.OptionRef != "" {
				return nil, ErrUnavailable
			}
		case "set_value":
			if op.Value == nil || !validPageOperationValue(*op.Value) || op.OptionRef != "" {
				return nil, ErrUnavailable
			}
		case "select_option":
			if op.Value != nil || !canonicalID(op.OptionRef) {
				return nil, ErrUnavailable
			}
		default:
			return nil, ErrUnavailable
		}
		// Repeating an identical gesture is never automatic recovery. A lost result
		// fences this operator for the rest of the run, even with a new snapshot.
		keyOp := op
		// Freshly observed state permits another intentional gesture.
		key, _ := jsoncodec.Marshal(keyOp)
		if used[string(key)] {
			return nil, ErrConflict
		}
		current, err := r.capturePage(ctx, revision)
		if err != nil {
			return nil, err
		}
		if current.ID != op.SnapshotID {
			return nil, ErrStale
		}
		found := false
		for _, n := range current.Nodes {
			if n.TargetRef == op.TargetRef {
				for _, a := range n.Actions {
					if a == op.Action {
						found = true
					}
				}
			}
		}
		if !found {
			return nil, ErrUnavailable
		}
		r.mu.Lock()
		if !r.pageOperationAllowed(runCtx, epoch, revision) || ctx.Err() != nil || len(r.pageOperations) > 0 {
			r.mu.Unlock()
			return nil, ErrStale
		}
		if r.options.Credentials.With(ctx, func(secret []byte) error {
			if len(secret) > 0 && (strings.Contains(string(args), string(secret)) || op.Value != nil && strings.Contains(*op.Value, string(secret))) {
				return ErrUnavailable
			}
			return nil
		}) != nil {
			r.mu.Unlock()
			return nil, ErrUnavailable
		}
		if r.pageOperations == nil {
			r.pageOperations = map[string]pendingPageOperation{}
		}
		expires := time.Now().Add(10 * time.Second)
		op.ExpiresAt = &expires
		id := uuid.NewString()
		response := make(chan PageOperationResult, 1)
		r.pageOperations[id] = pendingPageOperation{operation: op, revision: revision, ctx: runCtx, response: response}
		r.clearNavigation()
		r.mu.Unlock()
		defer func() { r.mu.Lock(); delete(r.pageOperations, id); r.mu.Unlock() }()
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		finish := func(result PageOperationResult) (json.RawMessage, error) {
			if result.State == "dispatched" || result.State == "unverified" {
				used[string(key)] = true
			}
			if result.State == "unverified" {
				uncertain = true
			}
			return jsoncodec.Marshal(result)
		}
		select {
		case result := <-response:
			return finish(result)
		case <-runCtx.Done():
			uncertain = true
			return nil, ErrStale
		case <-ctx.Done():
			uncertain = true
			return nil, ErrStale
		case <-timer.C:
			return finish(r.expirePageOperation(id, op.SnapshotID, response))
		}
	}
}

// expirePageOperation serializes timeout with claim and delivery. Only an
// unclaimed request proves no dispatch; an expired host lease cannot prove that.
// Prefer a result already accepted by Deliver even if the timer won the select.
func (r *Runtime) expirePageOperation(id, snapshotID string, response chan PageOperationResult) PageOperationResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case result := <-response:
		return result
	default:
	}
	p, ok := r.pageOperations[id]
	delete(r.pageOperations, id)
	if ok && !p.claimed {
		return PageOperationResult{State: "not_dispatched", SnapshotID: snapshotID}
	}
	return PageOperationResult{State: "unverified", SnapshotID: snapshotID}
}

// Match JSON Schema maxLength (Unicode code points), not UTF-16 code units.
// The byte cap bounds IPC payloads; these limits do not alter business validation.
func validPageOperationValue(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= 2048 && len(value) <= 8192 && !strings.ContainsRune(value, 0)
}
