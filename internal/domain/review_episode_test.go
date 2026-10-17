package domain

import (
	"testing"
	"time"
)

func TestReviewEpisodeKeepsIdentityAcrossRotationAndUnknown(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	acked := now.Add(-time.Hour)
	old := ReviewEpisodeIdentity{ID: "stable-uuid", TriggeredOn: "2026-10-10", FirstDetectedAt: now.AddDate(0, 0, -6), AcknowledgedAt: &acked}
	for _, latest := range []string{"2026-10-12", "2026-10-13"} {
		states := map[string]ReviewTruth{"2026-10-10": ReviewTrue, "2026-10-11": ReviewTrue, "2026-10-12": ReviewUnknown, "2026-10-13": ReviewTrue}
		out, err := AdvanceReviewEpisode("2026-10-10", latest, states, true, true, false, &old, "unused-new-id", now)
		if err != nil || out.Closed != nil || out.Opened || out.Current == nil || out.Current.ID != old.ID || out.Current.AcknowledgedAt != &acked {
			t.Fatalf("%s should retain acknowledged episode through U and window rotation: %+v %v", latest, out, err)
		}
	}
}

func TestReviewEpisodeProvenFalseSplitsHiddenRecovery(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	acked := now.Add(-time.Hour)
	old := ReviewEpisodeIdentity{ID: "old", TriggeredOn: "2026-10-10", FirstDetectedAt: now.AddDate(0, 0, -6), AcknowledgedAt: &acked}
	states := map[string]ReviewTruth{"2026-10-10": ReviewTrue, "2026-10-11": ReviewUnknown, "2026-10-12": ReviewFalse, "2026-10-13": ReviewTrue}
	out, err := AdvanceReviewEpisode("2026-10-10", "2026-10-13", states, true, true, false, &old, "new", now)
	if err != nil || out.Closed == nil || out.Closed.ID != "old" || out.Closed.ResolvedReason != "recovered" || out.Current == nil || out.Current.ID != "new" || out.Current.TriggeredOn != "2026-10-13" || out.Current.AcknowledgedAt != nil || !out.Opened {
		t.Fatalf("backfilled F must split identity without moving ack: %+v %v", out, err)
	}
	// Reconciliation of the unchanged projection uses the persisted current
	// episode. It cannot open a third identity or append another open event.
	repeated, err := AdvanceReviewEpisode("2026-10-13", "2026-10-13", map[string]ReviewTruth{"2026-10-13": ReviewTrue}, true, true, false, out.Current, "third", now)
	if err != nil || repeated.Opened || repeated.Closed != nil || repeated.Current.ID != "new" {
		t.Fatalf("same day created duplicate: %+v %v", repeated, err)
	}
}

func TestReviewEpisodePositionCloseAndUncertainCurrentQuantity(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	old := ReviewEpisodeIdentity{ID: "old", TriggeredOn: "2026-10-14", FirstDetectedAt: now.Add(-time.Hour)}
	states := map[string]ReviewTruth{"2026-10-14": ReviewTrue, "2026-10-15": ReviewTrue}
	uncertain, err := AdvanceReviewEpisode("2026-10-14", "2026-10-15", states, false, false, false, &old, "new", now)
	if err != nil || uncertain.Current == nil || uncertain.Closed != nil || uncertain.Opened {
		t.Fatalf("unknown current quantity must suspend, not resolve: %+v %v", uncertain, err)
	}
	closed, err := AdvanceReviewEpisode("2026-10-14", "2026-10-15", states, true, false, false, &old, "new", now)
	if err != nil || closed.Current != nil || closed.Closed == nil || closed.Closed.ResolvedReason != "position_closed" {
		t.Fatalf("proven close must resolve: %+v %v", closed, err)
	}
	opened, err := AdvanceReviewEpisode("2026-10-14", "2026-10-15", states, true, true, false, nil, "new-epoch", now)
	if err != nil || !opened.Opened || opened.Current == nil || opened.Current.ID != "new-epoch" {
		t.Fatalf("new epoch can alert immediately from still-valid rolling votes: %+v %v", opened, err)
	}
}
