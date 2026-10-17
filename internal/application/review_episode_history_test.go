package application

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewEpisodeHistoryPagesResolvedAcknowledgedAndReboughtEpisodes(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	rule, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic history", Enabled: true, Severity: "warning", Spec: syntheticHistoricalReviewSpec()})
	if err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-10-02")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var old string
	if err := f.db.SQL.QueryRow(`SELECT id FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&old); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), old, string(domain.NewMutationID()), 1); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-10-03")
	f.quantity(t, false)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-10-04")
	f.quantity(t, true)
	f.day(t, "2026-10-05")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	first, err := f.s.ListReviewEpisodeHistoryV2(t.Context(), string(rule.ID), "", 1)
	if err != nil || len(first.Items) != 1 || first.Items[0].ResolvedAt != "" || first.NextCursor == "" {
		t.Fatalf("first history page: %+v %v", first, err)
	}
	second, err := f.s.ListReviewEpisodeHistoryV2(t.Context(), string(rule.ID), first.NextCursor, 1)
	if err != nil || len(second.Items) != 1 || string(second.Items[0].EpisodeID) != old || second.Items[0].ResolvedReason != "position_closed" || second.Items[0].AcknowledgedAt == "" || second.Items[0].OpenedConditionVersion != 1 || second.NextCursor != "" {
		t.Fatalf("resolved history page: %+v %v", second, err)
	}
	events := map[string]int{}
	cursor := ""
	for {
		page, err := f.s.ListReviewEpisodeEventsV2(t.Context(), old, cursor, 1)
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("event page: %+v %v", page, err)
		}
		events[page.Items[0].Kind]++
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if events["opened"] != 1 || events["acknowledged"] != 1 || events["resolved"] != 1 || len(events) != 3 {
		t.Fatalf("history events lost or duplicated: %+v", events)
	}
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil || after != before {
		t.Fatalf("history read wrote %d rows: %v", after-before, err)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), string(first.Items[0].EpisodeID), string(domain.NewMutationID()), first.Items[0].Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ListReviewEpisodeHistoryV2(t.Context(), string(rule.ID), first.NextCursor, 1); err == nil {
		t.Fatal("history cursor survived an acknowledgement")
	}
	if _, err := f.s.ListReviewEpisodeEventsV2(t.Context(), old, "", 1); err != nil {
		t.Fatalf("historical episode events became inaccessible: %v", err)
	}
}
