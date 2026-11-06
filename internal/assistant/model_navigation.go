package assistant

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// Navigation uses ephemeral host leases. Persisted history cannot restore them.
type ModelNavigationTarget struct {
	Trend     *ReturnTrendAction `json:"trend,omitempty"`
	Tab       string             `json:"tab,omitempty"`
	Kind      string             `json:"kind"`
	AccountID string             `json:"accountId"`
}

// ReturnTrendAction is a finite presentation command, never a DOM instruction or
// financial mutation. Account identity is resolved from a run-local targetRef.
type ReturnTrendAction struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Valuation   string `json:"valuation"`
	IncludeCash bool   `json:"includeCash"`
	Currency    string `json:"currency,omitempty"`
	AssetClass  string `json:"assetClass,omitempty"`
	Display     string `json:"display"`
}

func (a ReturnTrendAction) Valid() bool {
	from, err := time.Parse("2006-01-02", a.From)
	if err != nil || from.Format("2006-01-02") != a.From || a.From < "0001-01-01" {
		return false
	}
	to, err := time.Parse("2006-01-02", a.To)
	if err != nil || to.Format("2006-01-02") != a.To || a.To < a.From {
		return false
	}
	return (a.Valuation == "base" || a.Valuation == "native") &&
		(a.Currency == "" || slices.Contains(domain.SupportedCurrencies(), domain.CurrencyCode(a.Currency))) &&
		(a.AssetClass == "" || a.AssetClass == "cash" || slices.Contains(domain.AllInstrumentTypes(), domain.InstrumentType(a.AssetClass))) &&
		(a.Display == "cumulative_amount" || a.Display == "linked_rate" || a.Display == "period_return_amount")
}

// Refuse rather than silently clamp the user's explicit inclusive civil range.
func (a ReturnTrendAction) WithinHistory(origin *domain.HistoryOrigin, now time.Time) bool {
	if origin == nil || !a.Valid() {
		return false
	}
	zone, err := time.LoadLocation(origin.Timezone)
	return err == nil && a.From >= origin.StartedAt.In(zone).Format("2006-01-02") && a.To < now.In(zone).Format("2006-01-02")
}

func (t ModelNavigationTarget) Valid() bool {
	if t.Kind == "return-trend" {
		return t.Tab == "" && (t.AccountID == "" || canonicalID(t.AccountID)) && t.Trend != nil && t.Trend.Valid()
	}
	if t.Trend != nil {
		return false
	}
	if t.Kind == "return-analysis" {
		return t.AccountID == "" && (t.Tab == "calendar" || t.Tab == "trend" || t.Tab == "contribution")
	}
	if t.Tab != "" {
		return false
	}
	switch t.Kind {
	case "overview", "accounts", "historical-overview", "asset-changes", "history", "portfolio", "available-funds", "tasks", "directory", "market-data", "data-health", "settings":
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
	generation     uint64
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
	generation                uint64
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
	return time.Now()
}
func (r *Runtime) clearNavigation() {
	r.navigationGeneration++
	r.pageLease = nil
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

// A current desktop observation admits navigation automatically. This lease is
// freshness metadata, not user approval; financial permissions remain separate.
func (r *Runtime) grantForStart(q StartRequest) (modelNavigationGrant, error) {
	if q.PageLeaseID == "" {
		return modelNavigationGrant{}, nil
	}
	// Publication can race Start even after the frontend checks its observation.
	// Drop optional navigation authority, not the conversation. Runtime epoch and
	// configuration admission are still checked independently by Start.
	if !r.currentLease(q.PageLeaseID) || r.pageLease.Page != q.Page {
		return modelNavigationGrant{}, nil
	}
	return modelNavigationGrant{lease: *r.pageLease, conversationID: q.ConversationID, requestID: q.RequestID, generation: r.navigationGeneration}, nil
}
func (r *Runtime) offerNavigation(ctx context.Context, id, runID string, result Completion) {
	grant, ok := ctx.Value(modelNavigationGrantKey{}).(modelNavigationGrant)
	if !ok || grant.generation != r.navigationGeneration || result.Navigation == nil || !result.Navigation.Valid() || r.closed || !r.currentLease(grant.lease.ID) || ctx.Err() != nil {
		return
	}
	if r.navigationIntents == nil {
		r.navigationIntents = map[string]ModelNavigationIntent{}
	}
	// Bound and replace pending navigation; old responses cannot accumulate authority.
	r.navigationIntents = map[string]ModelNavigationIntent{}
	intent := ModelNavigationIntent{generation: r.navigationGeneration, ID: uuid.NewString(), LeaseID: grant.lease.ID, RequestID: grant.requestID, RunID: runID, ConversationID: id, Target: *result.Navigation, ExpiresAt: grant.lease.ExpiresAt}
	if r.hasNavigationRun(intent) {
		r.navigationIntents[intent.ID] = intent
	}
}
func (r *Runtime) navigationView(v *Conversation) {
	if len(v.Runs) == 0 {
		return
	}
	for _, intent := range r.navigationIntents {
		if intent.generation == r.navigationGeneration && r.hasNavigationRun(intent) && intent.ConversationID == v.ID && intent.RunID == v.Runs[0].ID && r.currentLease(intent.LeaseID) {
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
	if r.anyPendingReview() || !ok || intent.generation != r.navigationGeneration || !r.hasNavigationRun(intent) || !r.currentLease(leaseID) || intent.LeaseID != leaseID || intent.ConversationID != conversationID || intent.RunID != runID || r.options.NavigationValidator == nil {
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
	if r.valid(ctx, epoch) != nil || intent.generation != r.navigationGeneration || !r.hasNavigationRun(intent) || !r.currentLease(leaseID) {
		return ModelNavigationTarget{}, ErrStale
	}
	return intent.Target, nil
}

// Caller owns mu. A captured intent is never valid merely because its map entry
// exists: it must still belong to the current successful turn of a live session.
func (r *Runtime) hasNavigationRun(intent ModelNavigationIntent) bool {
	c := r.find(intent.ConversationID)
	if c == nil || len(c.Runs) == 0 {
		return false
	}
	run := c.Runs[len(c.Runs)-1].Run
	return run.ID == intent.RunID && run.RequestID == intent.RequestID && run.State == "succeeded" && run.ConfigRevision == r.state.Revision && run.PageLeaseID == intent.LeaseID
}
