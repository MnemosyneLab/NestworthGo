package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
)

type FullCapabilityRequest struct {
	Name      string          `json:"capability"`
	Arguments json.RawMessage `json:"arguments"`
}

// The adapter obtains metadata and operation identity from the shared registry,
// never from model assertions. The runtime remains the desktop authority fence.
type FullCapabilitySession interface {
	Capability(context.Context, string, json.RawMessage) (agentcapability.Definition, string, error)
	CallCapability(context.Context, string, json.RawMessage) (json.RawMessage, error)
	InspectCapability(context.Context, string, string) (ChangeReview, error)
}

func (r *Runtime) executeCapability(ctx context.Context, epoch, id, runID string, revision uint64, q FullCapabilityRequest) (output json.RawMessage, failure error) {
	dispatched := false
	defer func() {
		if failure != nil && !dispatched && !errors.Is(failure, ErrRecoveryRequired) {
			failure = unsubmittedFullCapability{failure}
		}
	}()
	if len(q.Arguments) > 64<<10 {
		return nil, ErrUnavailable
	}
	session, err := r.reviewSession(ctx, epoch, id, revision, true)
	if err != nil {
		return nil, err
	}
	s, ok := session.(FullCapabilitySession)
	if !ok {
		return nil, ErrUnavailable
	}
	definition, operationID, err := s.Capability(ctx, q.Name, q.Arguments)
	if err != nil {
		return nil, err
	}
	writing := definition.Effect == agentcapability.Write
	if writing {
		release, err := acquireRunWrite(ctx)
		if err != nil {
			return nil, err
		}
		defer release(false)
	}
	var previous *ReviewRef
	r.mu.Lock()
	c := r.find(id)
	if r.valid(ctx, epoch) != nil || revision != r.state.Revision || !r.fullEnabled || c == nil || len(c.Runs) == 0 || c.Runs[len(c.Runs)-1].Run.ID != runID || terminal(c.Runs[len(c.Runs)-1].Run.State) || r.active[id] == nil {
		r.mu.Unlock()
		return nil, ErrStale
	}
	if err := r.options.Credentials.With(ctx, func(key []byte) error {
		if len(key) == 0 || strings.Contains(string(q.Arguments), string(key)) {
			return ErrUnavailable
		}
		return nil
	}); err != nil {
		r.mu.Unlock()
		return nil, ErrUnavailable
	}
	if writing {
		if r.pendingReview(c) {
			r.mu.Unlock()
			return nil, ErrRecoveryRequired
		}
		if r.reviewBusy[id] || len(c.Reviews) >= MaxRuns {
			r.mu.Unlock()
			return nil, ErrConflict
		}
		priorReviews := append([]ReviewRef(nil), c.Reviews...)
		found := false
		for i, ref := range c.Reviews {
			if ref.ID == operationID {
				if ref.Tool != q.Name {
					r.mu.Unlock()
					return nil, ErrConflict
				}
				found = true
				saved := ref
				previous = &saved
				c.Reviews[i].Resolved = false
				c.Reviews[i].Unsubmitted = false
			}
		}
		if !found {
			ref := r.operationRef(id, runID, operationID, q.Name)
			ref.Tool = q.Name
			c.Reviews = append(c.Reviews, ref)
		}
		r.reviewBusy[id] = true
		r.clearNavigation()
		err = r.persist()
		if err != nil {
			// CallCapability has not run. Restore the exact pre-admission references
			// so a later successful finish save cannot create a permanently unknown
			// attempt or erase a previous resolved receipt. A save-then-error followed
			// by process death still leaves its durable reference conservatively fenced.
			c.Reviews = priorReviews
			delete(r.reviewBusy, id)
			r.mu.Unlock()
			return nil, err
		}
	}
	r.mu.Unlock()
	if writing {
		defer func() { r.mu.Lock(); delete(r.reviewBusy, id); r.mu.Unlock() }()
	}
	// Revocation fences this acquired session, including queued domain writes.
	dispatched = true
	result, callErr := s.CallCapability(ctx, q.Name, q.Arguments)
	if writing {
		// Inspection is read-only and uses the original identity even after cancellation.
		review, inspectErr := s.InspectCapability(context.Background(), operationID, q.Name)
		var undispatched interface{ NotDispatched() bool }
		unsubmitted := errors.As(callErr, &undispatched) && undispatched.NotDispatched()
		r.mu.Lock()
		if c := r.find(id); c != nil && inspectErr == nil {
			for i := range c.Reviews {
				if c.Reviews[i].ID == operationID {
					c.Reviews[i].Resolved = unsubmitted || review.State == "succeeded" || review.State == "failed"
					c.Reviews[i].Unsubmitted = unsubmitted
					if unsubmitted && previous != nil {
						c.Reviews[i] = *previous
					}
				}
			}
			if err := r.persist(); err != nil {
				callErr = err
			}
		}
		r.mu.Unlock()
	}
	return result, callErr
}

// Runtime preflight performs no business dispatch. The marker describes only
// this attempt; it does not resolve an older operation's uncertain receipt.
type unsubmittedFullCapability struct{ error }

func (e unsubmittedFullCapability) Unwrap() error     { return e.error }
func (unsubmittedFullCapability) NotDispatched() bool { return true }

// ErrRecoveryRequired describes an existing unresolved operation, not a newly
// rejected mutation. It must not become a new-write argument diagnostic.
var ErrRecoveryRequired = errors.New("assistant_operation_recovery_required")
