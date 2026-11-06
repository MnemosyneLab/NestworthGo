package assistant

import (
	"context"
	"encoding/json"
)

type ChangeReview struct {
	Summary            string   `json:"summary"`
	Source             string   `json:"source"`
	ID                 string   `json:"id"`
	Digest             string   `json:"digest"`
	State              string   `json:"state"`
	ExpiresAt          string   `json:"expiresAt"`
	Kind               string   `json:"kind"`
	Account            string   `json:"account"`
	AccountInstitution string   `json:"accountInstitution"`
	AccountOwners      []string `json:"accountOwners"`
	AccountType        string   `json:"accountType"`
	AccountCurrency    string   `json:"accountCurrency"`
	Amount             string   `json:"amount"`
	Currency           string   `json:"currency"`
	EffectiveAt        string   `json:"effectiveAt"`
	Reason             string   `json:"reason"`
	Note               string   `json:"note"`
	ResultingAmount    string   `json:"resultingAmount"`
}
type ReviewRef struct {
	Unsubmitted bool   `json:"unsubmitted,omitempty"`
	Tool        string `json:"tool,omitempty"`
	ID          string `json:"id"`
	Resolved    bool   `json:"resolved"`
}

// Acquiring a session precedes runtime revalidation. Revocation thereafter
// fences that exact session, including calls waiting on the application gate.
type ReviewSession interface {
	Inspect(context.Context, string) (ChangeReview, error)
	RetireUnsubmitted(context.Context, string) (ChangeReview, error)
}
type ReviewProvider interface {
	AcquireReview(context.Context, string) (ReviewSession, error)
}
type ToolHandler func(context.Context, json.RawMessage) (json.RawMessage, error)

func (r *Runtime) SetFullAccess(ctx context.Context, epoch string, revision uint64, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.valid(ctx, epoch); err != nil {
		return err
	}
	if revision != r.state.Revision {
		return ErrStale
	}
	if enabled && (!r.state.Config.Enabled || r.options.Reviews == nil || len(r.active) != 0) {
		return ErrUnavailable
	}
	r.fullEnabled = enabled
	if r.options.Capabilities != nil {
		r.options.Capabilities.Revoke()
	}
	r.clearNavigation()
	r.state.Revision++
	if err := r.persist(); err != nil {
		r.fullEnabled = false
		if r.options.Capabilities != nil {
			r.options.Capabilities.Revoke()
		}
		return err
	}
	return nil
}
func (r *Runtime) pendingReview(c *ConversationState) bool {
	for _, ref := range c.Reviews {
		if !ref.Resolved {
			return true
		}
	}
	return false
}
func (r *Runtime) anyPendingReview() bool {
	for i := range r.state.Conversations {
		if r.pendingReview(&r.state.Conversations[i]) {
			return true
		}
	}
	return false
}
func (r *Runtime) reviewSession(ctx context.Context, epoch, id string, revision uint64, write bool) (ReviewSession, error) {
	r.mu.Lock()
	if r.valid(ctx, epoch) != nil || r.find(id) == nil || r.options.Reviews == nil {
		r.mu.Unlock()
		return nil, ErrStale
	}
	provider := r.options.Reviews
	r.mu.Unlock()
	s, err := provider.AcquireReview(ctx, id)
	if err != nil {
		return nil, ErrUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.valid(ctx, epoch) != nil || r.find(id) == nil || revision != r.state.Revision {
		return nil, ErrStale
	}
	if write && (!r.fullEnabled || !r.state.Config.Enabled) {
		return nil, ErrUnavailable
	}
	return s, nil
}

// Reviews reconciles authoritative state only; it never redispatches a writer.
func (r *Runtime) Reviews(ctx context.Context, epoch, id string) ([]ChangeReview, error) {
	r.mu.Lock()
	if r.valid(ctx, epoch) != nil || r.find(id) == nil {
		r.mu.Unlock()
		return nil, ErrStale
	}
	refs := append([]ReviewRef(nil), r.find(id).Reviews...)
	idle := r.active[id] == nil && !r.reviewBusy[id]
	revision := r.state.Revision
	r.mu.Unlock()
	if len(refs) == 0 {
		return []ChangeReview{}, nil
	}
	s, err := r.reviewSession(ctx, epoch, id, revision, false)
	if err != nil {
		return nil, err
	}
	out := make([]ChangeReview, 0, len(refs))
	for _, ref := range refs {
		var review ChangeReview
		var err error
		if ref.Tool != "" && ref.Unsubmitted {
			review = ChangeReview{ID: ref.ID, Source: "ai_full", State: "cancelled", Kind: "capability", Summary: ref.Tool}
		} else if ref.Tool != "" {
			full, ok := s.(FullCapabilitySession)
			if !ok {
				return nil, ErrUnavailable
			}
			review, err = full.InspectCapability(ctx, ref.ID, ref.Tool)
		} else {
			review, err = s.Inspect(ctx, ref.ID)
			if err == nil && idle && (review.State == "awaiting_confirmation" || review.State == "needs_preview") {
				// A stopped legacy convenience write cannot resume itself. Retire
				// only a proven unsubmitted preview; unknown receipts stay fenced.
				review, err = s.RetireUnsubmitted(ctx, ref.ID)
			}
		}
		if err != nil {
			return nil, ErrUnavailable
		}
		out = append(out, review)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.valid(ctx, epoch) != nil || revision != r.state.Revision || r.find(id) == nil {
		return nil, ErrStale
	}
	changed := false
	for i := range r.find(id).Reviews {
		for _, review := range out {
			ref := &r.find(id).Reviews[i]
			if ref.ID == review.ID {
				resolved := review.State == "succeeded" || review.State == "cancelled" || (ref.Tool != "" && review.State == "failed")
				if ref.Resolved != resolved {
					ref.Resolved = resolved
					changed = true
				}
			}
		}
	}
	if changed {
		if err := r.persist(); err != nil {
			return nil, err
		}
	}
	return out, nil
}
