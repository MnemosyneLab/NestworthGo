package assistant

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Navigation grants are ephemeral desktop host state. Neither native model
// history nor a client page selector can mint a grant. Each explicit grant is
// bound to one not-yet-started request, config revision and current page lease.
type ModelNavigationTarget struct {
	Kind      string `json:"kind"`
	AccountID string `json:"accountId"`
}

func (t ModelNavigationTarget) Valid() bool {
	switch t.Kind {
	case "overview", "accounts":
		return t.AccountID == ""
	case "account":
		return canonicalID(t.AccountID)
	default:
		return false
	}
}

type DesktopPageLease struct {
	ID        string        `json:"id"`
	Epoch     string        `json:"epoch"`
	Revision  uint64        `json:"revision"`
	Page      PageSelection `json:"page"`
	ExpiresAt time.Time     `json:"expiresAt"`
}
type ModelNavigationIntent struct {
	ID             string                `json:"id"`
	LeaseID        string                `json:"leaseId"`
	RequestID      string                `json:"requestId"`
	RunID          string                `json:"runId"`
	ConversationID string                `json:"conversationId"`
	Target         ModelNavigationTarget `json:"target"`
	ExpiresAt      time.Time             `json:"expiresAt"`
}
type modelNavigationGrant struct {
	lease                     DesktopPageLease
	conversationID, requestID string
}
type modelNavigationGrantKey struct{}

func ModelNavigationEnabled(ctx context.Context) bool {
	g, ok := ctx.Value(modelNavigationGrantKey{}).(modelNavigationGrant)
	return ok && g.requestID != "" && ctx.Err() == nil
}
func (r *Runtime) navigationNow() time.Time {
	if r.options.NavigationClock != nil {
		return r.options.NavigationClock()
	}
	return time.Now().UTC()
}
func (r *Runtime) clearNavigation() {
	r.pageLease = nil
	r.navigationGrants = nil
	r.navigationIntents = nil
}
func (r *Runtime) currentLease(id string) bool {
	return r.pageLease != nil && r.pageLease.ID == id && r.pageLease.Epoch == r.epoch && r.pageLease.Revision == r.state.Revision && r.pageLease.ExpiresAt.After(r.navigationNow()) && r.state.Config.Enabled
}

// PublishPage revokes all old page/navigation leases. Values are selectors only,
// not proof of the visible DOM, filters, data or financial permission.
func (r *Runtime) PublishPage(ctx context.Context, epoch string, revision uint64, page PageSelection) (DesktopPageLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.valid(ctx, epoch); err != nil {
		return DesktopPageLease{}, err
	}
	if revision != r.state.Revision || !page.Valid() || !r.state.Config.Enabled || r.options.NavigationValidator == nil {
		return DesktopPageLease{}, ErrUnavailable
	}
	r.clearNavigation()
	lease := DesktopPageLease{ID: uuid.NewString(), Epoch: epoch, Revision: revision, Page: page, ExpiresAt: r.navigationNow().Add(2 * time.Minute)}
	r.pageLease = &lease
	return lease, nil
}
func (r *Runtime) GrantNavigation(ctx context.Context, epoch string, revision uint64, conversationID, requestID, leaseID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.valid(ctx, epoch); err != nil {
		return err
	}
	if revision != r.state.Revision || !r.currentLease(leaseID) || !canonicalID(requestID) || r.options.NavigationValidator == nil {
		return ErrStale
	}
	c := r.find(conversationID)
	if c == nil || r.active[conversationID] != nil {
		return ErrConflict
	}
	for _, item := range c.Runs {
		if item.Run.RequestID == requestID {
			return ErrConflict
		}
	}
	// One outstanding explicit intent per page; a newer authorization revokes it.
	r.navigationGrants = map[string]modelNavigationGrant{requestID: {lease: *r.pageLease, conversationID: conversationID, requestID: requestID}}
	return nil
}
func (r *Runtime) grantForStart(q StartRequest) (modelNavigationGrant, error) {
	if q.PageLeaseID != "" && (!r.currentLease(q.PageLeaseID) || r.pageLease.Page != q.Page) {
		return modelNavigationGrant{}, ErrStale
	}
	grant, ok := r.navigationGrants[q.RequestID]
	if !ok {
		return modelNavigationGrant{}, nil
	}
	delete(r.navigationGrants, q.RequestID)
	if grant.conversationID != q.ConversationID || grant.lease.ID != q.PageLeaseID || !r.currentLease(grant.lease.ID) {
		return modelNavigationGrant{}, ErrStale
	}
	return grant, nil
}
func (r *Runtime) offerNavigation(ctx context.Context, id, runID string, result Completion) {
	grant, ok := ctx.Value(modelNavigationGrantKey{}).(modelNavigationGrant)
	if !ok || result.Navigation == nil || !result.Navigation.Valid() || r.closed || !r.currentLease(grant.lease.ID) || ctx.Err() != nil {
		return
	}
	if r.navigationIntents == nil {
		r.navigationIntents = map[string]ModelNavigationIntent{}
	}
	// Bound and replace pending navigation; old responses cannot accumulate authority.
	r.navigationIntents = map[string]ModelNavigationIntent{}
	intent := ModelNavigationIntent{ID: uuid.NewString(), LeaseID: grant.lease.ID, RequestID: grant.requestID, RunID: runID, ConversationID: id, Target: *result.Navigation, ExpiresAt: grant.lease.ExpiresAt}
	r.navigationIntents[intent.ID] = intent
}
func (r *Runtime) navigationView(v *Conversation) {
	if len(v.Runs) == 0 {
		return
	}
	for _, intent := range r.navigationIntents {
		if intent.ConversationID == v.ID && intent.RunID == v.Runs[0].ID && r.currentLease(intent.LeaseID) {
			copy := intent
			v.Navigation = &copy
		}
	}
}

// ConsumeNavigation consumes before the external read, so uncertain responses
// cannot replay navigation. Revalidate live object/workspace and lease afterward.
// Financial review and visible-page generation are independently fenced by UI.
func (r *Runtime) ConsumeNavigation(ctx context.Context, epoch, conversationID, runID, intentID, leaseID string) (ModelNavigationTarget, error) {
	r.mu.Lock()
	if err := r.valid(ctx, epoch); err != nil {
		r.mu.Unlock()
		return ModelNavigationTarget{}, err
	}
	intent, ok := r.navigationIntents[intentID]
	if !ok || !r.currentLease(leaseID) || intent.LeaseID != leaseID || intent.ConversationID != conversationID || intent.RunID != runID || r.options.NavigationValidator == nil {
		r.mu.Unlock()
		return ModelNavigationTarget{}, ErrStale
	}
	delete(r.navigationIntents, intentID)
	r.mu.Unlock()
	if !r.options.NavigationValidator(ctx, intent.Target) {
		return ModelNavigationTarget{}, ErrUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.valid(ctx, epoch) != nil || !r.currentLease(leaseID) {
		return ModelNavigationTarget{}, ErrStale
	}
	return intent.Target, nil
}
