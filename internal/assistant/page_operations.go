package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type PageOperation struct {
	RequiresFull bool       `json:"-"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	SnapshotID   string     `json:"snapshotId"`
	TargetRef    string     `json:"targetRef"`
	Action       string     `json:"action"`
	Value        *string    `json:"value,omitempty"`
	OptionRef    string     `json:"optionRef,omitempty"`
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

func CurrentPageOperator(ctx context.Context) ToolHandler {
	h, _ := ctx.Value(pageOperatorKey{}).(ToolHandler)
	return h
}
func (r *Runtime) pageOperationAllowed(ctx context.Context, epoch string, revision uint64, requiresFull bool) bool {
	return ctx.Err() == nil && r.valid(ctx, epoch) == nil && r.state.Revision == revision && r.state.Config.Enabled && (!requiresFull || r.fullEnabled) && !r.anyPendingReview() && len(r.reviewBusy) == 0
}
func (r *Runtime) PendingPageOperations(ctx context.Context, epoch string) ([]PageReadRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.valid(ctx, epoch) != nil {
		return nil, ErrStale
	}
	out := []PageReadRequest{}
	for id, p := range r.pageOperations {
		if !p.claimed && r.pageOperationAllowed(p.ctx, epoch, p.revision, p.operation.RequiresFull) {
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
	if !ok || p.claimed || ctx.Err() != nil || !r.pageOperationAllowed(p.ctx, epoch, p.revision, p.operation.RequiresFull) {
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
func (r *Runtime) pageOperator(runCtx context.Context, epoch string, revision uint64) ToolHandler {
	var mu sync.Mutex
	uncertain := false
	used := map[string]bool{}
	// All steps share the same run write lease and unknown-outcome fence.
	single := func(ctx context.Context, args json.RawMessage, refresh bool) (json.RawMessage, error) {
		op, err := decodePageOperation(args)
		if err != nil {
			return nil, pagePreflightRefusal(ctx, runCtx, err)
		}
		if !refresh {
			key, _ := jsoncodec.Marshal(op)
			if used[string(key)] {
				return nil, ErrConflict
			}
		}
		current, err := r.capturePage(ctx, revision)
		if err != nil {
			return nil, pagePreflightRefusal(ctx, runCtx, err)
		}
		if !refresh && current.ID != op.SnapshotID {
			return nil, pagePreflightRefusal(ctx, runCtx, ErrStale)
		}
		if refresh {
			op.SnapshotID = current.ID
		}
		// Repeating an identical gesture is never automatic recovery. A lost result
		// fences this operator for the rest of the run, even with a new snapshot.
		keyOp := op
		// Freshly observed state permits another intentional gesture.
		key, _ := jsoncodec.Marshal(keyOp)
		if used[string(key)] {
			return nil, ErrConflict
		}
		found := false
		op.RequiresFull = true
		for _, n := range current.Nodes {
			if n.TargetRef == op.TargetRef {
				for _, a := range n.Actions {
					if a == op.Action {
						found = true
						op.RequiresFull = n.Effect != "presentation"
					}
				}
			}
		}
		if !found {
			return nil, pagePreflightRefusal(ctx, runCtx, ErrUnavailable)
		}
		r.mu.Lock()
		if !r.pageOperationAllowed(runCtx, epoch, revision, op.RequiresFull) || ctx.Err() != nil || len(r.pageOperations) > 0 {
			r.mu.Unlock()
			return nil, pagePreflightRefusal(ctx, runCtx, ErrStale)
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
		var fields map[string]json.RawMessage
		if len(args) > 256<<10 || jsoncodec.Unmarshal(args, &fields) != nil || fields == nil {
			return nil, pagePreflightRefusal(ctx, runCtx, ErrUnavailable)
		}
		if _, batch := fields["actions"]; !batch {
			return single(ctx, args, false)
		}
		var request struct {
			SnapshotID string            `json:"snapshotId"`
			Actions    []json.RawMessage `json:"actions"`
		}
		if len(fields) != 2 || jsoncodec.Unmarshal(args, &request) != nil || !canonicalID(request.SnapshotID) || len(request.Actions) < 1 || len(request.Actions) > 20 {
			return nil, pagePreflightRefusal(ctx, runCtx, ErrUnavailable)
		}
		operations := make([]json.RawMessage, len(request.Actions))
		// Validate the entire envelope before any dispatch. No selectors, guessed
		// replacement nodes, per-step snapshots or permissions are accepted.
		for i, raw := range request.Actions {
			var action map[string]json.RawMessage
			if jsoncodec.Unmarshal(raw, &action) != nil || action == nil || action["snapshotId"] != nil {
				return nil, pagePreflightRefusal(ctx, runCtx, ErrUnavailable)
			}
			action["snapshotId"], _ = jsoncodec.Marshal(request.SnapshotID)
			operations[i], _ = jsoncodec.Marshal(action)
			op, err := decodePageOperation(operations[i])
			if err != nil {
				return nil, pagePreflightRefusal(ctx, runCtx, err)
			}
			if r.options.Credentials.With(ctx, func(secret []byte) error {
				if len(secret) > 0 && (strings.Contains(string(operations[i]), string(secret)) || op.Value != nil && strings.Contains(*op.Value, string(secret))) {
					return ErrUnavailable
				}
				return nil
			}) != nil {
				return nil, pagePreflightRefusal(ctx, runCtx, ErrUnavailable)
			}
		}
		result := PageOperationBatchResult{State: "dispatched", Results: []PageOperationResult{}}
		for i, raw := range operations {
			response, err := single(ctx, raw, i > 0)
			var step PageOperationResult
			if err != nil {
				// A later refusal must not erase earlier dispatches or claim that the
				// whole batch was never submitted. Error text contains no model values.
				step.State = "error"
				if errors.Is(err, ErrStale) {
					step.State = "stale"
				}
				if uncertain {
					step.State = "unverified"
				}
				result.Results = append(result.Results, step)
				result.State = step.State
				result.Partial = i > 0 || uncertain
				return jsoncodec.Marshal(result)
			}
			if jsoncodec.Unmarshal(response, &step) != nil {
				uncertain = true
				step.State = "unverified"
			}
			result.Results = append(result.Results, step)
			if step.State != "dispatched" {
				result.State = step.State
				result.Partial = i > 0 || step.State == "unverified"
				return jsoncodec.Marshal(result)
			}
		}
		// The final observation is presentation evidence only. A failed read does
		// not erase acknowledged gestures or turn them into financial receipts.
		final, err := r.capturePage(ctx, revision)
		if err == nil {
			result.SnapshotID = final.ID
			result.ObservationState = "available"
		} else {
			result.ObservationState = "unavailable"
		}
		return jsoncodec.Marshal(result)
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

// This proof covers only explicit preflight rejection before enqueue. Cancellation,
// prior gestures and post-enqueue outcomes deliberately do not use this helper.
func pagePreflightRefusal(ctx, runCtx context.Context, err error) error {
	if ctx.Err() != nil || runCtx.Err() != nil {
		return err
	}
	return unsubmittedPageOperation{err}
}

type unsubmittedPageOperation struct{ error }

func (e unsubmittedPageOperation) Unwrap() error     { return e.error }
func (unsubmittedPageOperation) NotDispatched() bool { return true }

// Dispatched acknowledges UI event delivery only, never a financial commit.
// Partial means an incomplete batch may already have effects; results contains
// attempted steps only, and the unreported suffix was not dispatched.
type PageOperationBatchResult struct {
	State            string                `json:"state"`
	Results          []PageOperationResult `json:"results"`
	Partial          bool                  `json:"partial"`
	SnapshotID       string                `json:"snapshotId,omitempty"`
	ObservationState string                `json:"observationState,omitempty"`
}

func decodePageOperation(args json.RawMessage) (PageOperation, error) {
	var op PageOperation
	var fields map[string]json.RawMessage
	if len(args) > 32<<10 || jsoncodec.Unmarshal(args, &fields) != nil || fields == nil || jsoncodec.Unmarshal(args, &op) != nil || !canonicalID(op.SnapshotID) || !canonicalID(op.TargetRef) {
		return PageOperation{}, ErrUnavailable
	}
	for k, v := range fields {
		if string(v) == "null" || k != "snapshotId" && k != "targetRef" && k != "action" && k != "value" && k != "optionRef" {
			return PageOperation{}, ErrUnavailable
		}
	}
	switch op.Action {
	case "activate":
		if op.Value != nil || op.OptionRef != "" {
			return PageOperation{}, ErrUnavailable
		}
	case "set_value":
		if op.Value == nil || !validPageOperationValue(*op.Value) || op.OptionRef != "" {
			return PageOperation{}, ErrUnavailable
		}
	case "select_option":
		if op.Value != nil || !canonicalID(op.OptionRef) {
			return PageOperation{}, ErrUnavailable
		}
	default:
		return PageOperation{}, ErrUnavailable
	}
	return op, nil
}
