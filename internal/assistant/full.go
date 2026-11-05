package assistant

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type fullKey struct{}

func CurrentFullHandler(ctx context.Context) ProposalHandler {
	h, _ := ctx.Value(fullKey{}).(ProposalHandler)
	return h
}

// FullReviewSession is distinct from the human confirmation port. The adapter
// owns strict command validation and never accepts model operation identities.
type FullReviewSession interface {
	PreviewFull(context.Context, json.RawMessage) (ChangeReview, error)
	ExecuteFull(context.Context, string) (ChangeReview, error)
}

func (r *Runtime) executeFull(ctx context.Context, epoch, id, runID string, revision uint64, input json.RawMessage) (json.RawMessage, error) {
	if len(input) > 4096 {
		return nil, ErrUnavailable
	}
	session, err := r.reviewSession(ctx, epoch, id, revision, true)
	if err != nil {
		return nil, err
	}
	s, ok := session.(FullReviewSession)
	if !ok {
		return nil, ErrUnavailable
	}
	r.mu.Lock()
	c := r.find(id)
	if r.valid(ctx, epoch) != nil || revision != r.state.Revision || !r.fullEnabled || c == nil || r.pendingReview(c) || r.reviewBusy[id] || len(c.Reviews) >= MaxRuns || len(c.Runs) == 0 || c.Runs[len(c.Runs)-1].Run.ID != runID || terminal(c.Runs[len(c.Runs)-1].Run.State) || r.active[id] == nil {
		r.mu.Unlock()
		return nil, ErrConflict
	}
	if err := r.options.Credentials.With(ctx, func(key []byte) error {
		if len(key) == 0 || strings.Contains(string(input), string(key)) {
			return ErrUnavailable
		}
		return nil
	}); err != nil {
		r.mu.Unlock()
		return nil, ErrUnavailable
	}
	r.reviewBusy[id] = true
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.reviewBusy, id); r.mu.Unlock() }()
	review, err := s.PreviewFull(ctx, input)
	if err != nil {
		return nil, ErrUnavailable
	}
	r.mu.Lock()
	c = r.find(id)
	if r.valid(ctx, epoch) != nil || revision != r.state.Revision || !r.fullEnabled || c == nil || r.active[id] == nil {
		r.mu.Unlock()
		return nil, ErrStale
	}
	c.Reviews = append(c.Reviews, ReviewRef{ID: review.ID})
	r.clearNavigation()
	// Save the recovery identity before any business dispatch. On persistence
	// failure leave the in-memory fence in place; the unsubmitted preview is inert.
	err = r.persist()
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	// The acquired session's cancellation/generation is checked under the same
	// application gate as durable admission. Revoke cannot swap in a new session.
	result, err := s.ExecuteFull(ctx, review.ID)
	if err != nil {
		return nil, ErrUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.valid(ctx, epoch) != nil || revision != r.state.Revision || r.find(id) == nil {
		return nil, ErrStale
	}
	if result.State != "succeeded" {
		return nil, ErrUnavailable
	}
	for i := range r.find(id).Reviews {
		if r.find(id).Reviews[i].ID == review.ID {
			r.find(id).Reviews[i].Resolved = true
		}
	}
	if err := r.persist(); err != nil {
		return nil, err
	}
	return jsoncodec.Marshal(map[string]string{"status": "succeeded", "operationRef": review.ID, "source": "ai_full"})
}
