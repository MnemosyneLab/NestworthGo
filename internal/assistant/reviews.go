package assistant

import (
	"context"
	"encoding/json"
	"strings"
)

// CashProposal is deliberately narrower than the shared ledger command union.
// Model data never includes a plan, mutation identity or approval assertion.
type CashProposal struct {
	Kind        string `json:"kind"`
	AccountID   string `json:"accountId"`
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	Reason      string `json:"reason"`
	EffectiveAt string `json:"effectiveAt"`
	Note        string `json:"note,omitempty"`
}
type ChangeReview struct {
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
	ID       string `json:"id"`
	Resolved bool   `json:"resolved"`
}

// Acquiring a session precedes runtime revalidation. Revocation thereafter
// fences that exact session, including calls waiting on the application gate.
type ReviewSession interface {
	Preview(context.Context, CashProposal) (ChangeReview, error)
	Inspect(context.Context, string) (ChangeReview, error)
	Confirm(context.Context, string, string) (ChangeReview, error)
	Cancel(context.Context, string) error
	Repreview(context.Context, string) (ChangeReview, error)
}
type ReviewProvider interface {
	AcquireReview(context.Context, string) (ReviewSession, error)
}
type proposalKey struct{}
type ProposalHandler func(context.Context, json.RawMessage) (json.RawMessage, error)

func CurrentProposalHandler(ctx context.Context) ProposalHandler {
	h, _ := ctx.Value(proposalKey{}).(ProposalHandler)
	return h
}
func (r *Runtime) SetWritePermission(ctx context.Context, epoch string, revision uint64, enabled bool) error {
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
	r.writeEnabled = enabled
	if r.options.Capabilities != nil {
		r.options.Capabilities.Revoke()
	}
	r.clearNavigation()
	r.state.Revision++
	return r.persist()
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
	if write && (!r.writeEnabled || !r.state.Config.Enabled) {
		return nil, ErrUnavailable
	}
	return s, nil
}
func (r *Runtime) propose(ctx context.Context, epoch, id, runID string, revision uint64, input json.RawMessage) (json.RawMessage, error) {
	// Strict decoding is performed by the capability adapter before this callback.
	var proposal CashProposal
	if len(input) > 4096 || decodeProposal(input, &proposal) != nil {
		return nil, ErrUnavailable
	}
	s, err := r.reviewSession(ctx, epoch, id, revision, true)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	c := r.find(id)
	if err := r.options.Credentials.With(ctx, func(key []byte) error {
		if len(key) == 0 || strings.Contains(string(input), string(key)) {
			return ErrUnavailable
		}
		return nil
	}); err != nil {
		r.mu.Unlock()
		return nil, ErrUnavailable
	}
	if c == nil || r.pendingReview(c) || r.reviewBusy[id] || len(c.Reviews) >= MaxRuns || len(c.Runs) == 0 || c.Runs[len(c.Runs)-1].Run.ID != runID || terminal(c.Runs[len(c.Runs)-1].Run.State) || r.active[id] == nil {
		r.mu.Unlock()
		return nil, ErrConflict
	}
	r.reviewBusy[id] = true
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.reviewBusy, id); r.mu.Unlock() }()
	review, err := s.Preview(ctx, proposal)
	if err != nil {
		return nil, ErrUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.valid(ctx, epoch) != nil || revision != r.state.Revision {
		return nil, ErrStale
	}
	c = r.find(id)
	if c == nil {
		return nil, ErrStale
	}
	c.Reviews = append(c.Reviews, ReviewRef{ID: review.ID})
	r.clearNavigation()
	if err := r.persist(); err != nil {
		return nil, err
	}
	// No review digest or host confirmation surface crosses into model history.
	return json.RawMessage(`{"status":"awaiting_user_review","message":"The desktop has a frozen preview. Nothing has been posted. Only the user can confirm it in the review panel."}`), nil
}

// Reviews reconciles authoritative state only; it never redispatches a writer.
func (r *Runtime) Reviews(ctx context.Context, epoch, id string) ([]ChangeReview, error) {
	r.mu.Lock()
	if r.valid(ctx, epoch) != nil || r.find(id) == nil {
		r.mu.Unlock()
		return nil, ErrStale
	}
	refs := append([]ReviewRef(nil), r.find(id).Reviews...)
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
		review, err := s.Inspect(ctx, ref.ID)
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
				resolved := review.State == "succeeded" || review.State == "cancelled"
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

// ReviewAction is desktop-only. Confirmation accepts the exact displayed digest;
// cancellation cannot erase an unresolved submission. Every retry inspects first.
func (r *Runtime) ReviewAction(ctx context.Context, epoch, id, reviewID, digest, action string) error {
	if action != "confirm" && action != "cancel" && action != "repreview" {
		return ErrUnavailable
	}
	r.mu.Lock()
	if r.valid(ctx, epoch) != nil || r.find(id) == nil {
		r.mu.Unlock()
		return ErrStale
	}
	c := r.find(id)
	found := false
	for _, ref := range c.Reviews {
		found = found || ref.ID == reviewID
	}
	if !found || r.reviewBusy[id] || r.active[id] != nil {
		r.mu.Unlock()
		return ErrConflict
	}
	revision := r.state.Revision
	r.reviewBusy[id] = true
	r.wg.Add(1)
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.reviewBusy, id); r.mu.Unlock(); r.wg.Done() }()
	s, err := r.reviewSession(ctx, epoch, id, revision, action != "cancel")
	if err != nil {
		return err
	}
	var replacement ChangeReview
	switch action {
	case "confirm":
		_, err = s.Confirm(ctx, reviewID, digest)
	case "cancel":
		err = s.Cancel(ctx, reviewID)
	case "repreview":
		replacement, err = s.Repreview(ctx, reviewID)
	}
	if err != nil {
		return ErrUnavailable
	}
	if action == "repreview" {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.valid(ctx, epoch) != nil || revision != r.state.Revision || r.find(id) == nil {
			return ErrStale
		}
		for i := range r.find(id).Reviews {
			if r.find(id).Reviews[i].ID == reviewID {
				r.find(id).Reviews[i] = ReviewRef{ID: replacement.ID}
			}
		}
		return r.persist()
	}
	return nil
}
