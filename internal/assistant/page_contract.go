package assistant

import (
	"context"
	"time"
)

// PageBinding is a backend-owned lease for a future trusted page publisher. The
// client-only generations from #204 do not satisfy it. No current IPC/mobile or
// model tool can create a lease or mint an explicit navigation intent.
type PageBinding struct {
	Epoch              string    `json:"epoch"`
	Ledger             string    `json:"ledger"`
	ConversationID     string    `json:"conversationId"`
	PermissionRevision uint64    `json:"permissionRevision"`
	Surface            string    `json:"surface"`
	ContextID          string    `json:"contextId"`
	PageVersion        uint64    `json:"pageVersion"`
	CapturedAt         time.Time `json:"capturedAt"`
	ExpiresAt          time.Time `json:"expiresAt"`
}
type PageMetric struct {
	Ref            string   `json:"ref"`
	Label          string   `json:"label"`
	Value          *string  `json:"value"`
	Unit           string   `json:"unit"`
	Basis          string   `json:"basis"`
	Status         string   `json:"status"`
	MissingReasons []string `json:"missingReasons"`
	EvidenceRefs   []string `json:"evidenceRefs"`
}
type PageSnapshot struct {
	Version  int          `json:"version"`
	Binding  PageBinding  `json:"binding"`
	Page     string       `json:"page"`
	Scope    string       `json:"scope"`
	Timezone string       `json:"timezone"`
	Metrics  []PageMetric `json:"metrics"`
	Targets  []PageTarget `json:"targets"`
}
type PageTarget struct {
	Ref   string `json:"ref"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}
type NavigationIntent struct {
	Version    int         `json:"version"`
	Binding    PageBinding `json:"binding"`
	IntentID   string      `json:"intentId"`
	UserTurnID string      `json:"userTurnId"`
	Action     string      `json:"action"` // open_object only; never coordinates, routes or writes
	TargetRef  string      `json:"targetRef"`
}

// PageHost must intersect current surface/user/object permission, revalidate the
// current page generation and unresolved-review guard, resolve opaque target
// refs from its private map, and consume a host-recorded explicit user intent.
// It must not trust model/IPC assertions of approval, routes, IDs or page content.
// This port is intentionally unconnected until the real UI and scoped principal
// integration is verified. Structured page data must remain <=64 KiB/32 metrics/
// 64 targets, excluding DOM, inputs, notes, credentials and native reasoning.
type PageHost interface {
	Snapshot(context.Context, PageBinding) (PageSnapshot, error)
	Navigate(context.Context, NavigationIntent) error
}
type ClosedPageHost struct{}

func (ClosedPageHost) Snapshot(context.Context, PageBinding) (PageSnapshot, error) {
	return PageSnapshot{}, ErrUnavailable
}
func (ClosedPageHost) Navigate(context.Context, NavigationIntent) error { return ErrUnavailable }

// MatchesCurrentPage is a prerequisite fence, never authorization by itself.
func MatchesCurrentPage(request, current PageBinding, now time.Time) bool {
	return request == current && request.Epoch != "" && request.Ledger != "" && canonicalID(request.ConversationID) && request.ContextID != "" && request.PageVersion > 0 && request.PermissionRevision > 0 &&
		(request.Surface == "desktop" || request.Surface == "mobile") && !request.CapturedAt.After(now) && request.ExpiresAt.After(now) && request.ExpiresAt.After(request.CapturedAt) && request.ExpiresAt.Sub(request.CapturedAt) <= 2*time.Minute
}
