package application

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewRulePreviewIsReadOnlyAndDateKeyed(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "2")
	f.day(t, "2026-08-02")
	spec := syntheticHistoricalReviewSpec()
	var before, after int
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	first, err := f.s.PreviewReviewRuleV2(t.Context(), spec, f.instrument.ID.String(), "2026-08-01", "")
	if err != nil || len(first.Days) != 1 || first.Days[0].Result.Truth != domain.ReviewTrue || first.Window.Truth != domain.ReviewTrue || first.SourceToken == "" {
		t.Fatalf("daily preview: %+v %v", first, err)
	}
	second, err := f.s.PreviewReviewRuleV2(t.Context(), spec, f.instrument.ID.String(), "2026-08-01", first.SourceToken)
	if err != nil || second.SourceToken != first.SourceToken || second.Window != first.Window {
		t.Fatalf("repeat preview changed evidence: %+v %v", second, err)
	}
	if _, err := f.s.PreviewReviewRuleV2(t.Context(), spec, f.instrument.ID.String(), "2026-08-02", ""); err == nil {
		t.Fatal("accepted an unfinished day")
	}
	if _, err := f.s.PreviewReviewRuleV2(t.Context(), spec, f.instrument.ID.String(), "2026-08-01", "stale"); err == nil {
		t.Fatal("accepted a stale source token")
	}
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("preview wrote %d rows", after-before)
	}
}

func TestReviewRulePreviewCountsFullTrailingTrueSuffix(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-07")
	spec := syntheticHistoricalReviewSpec()
	spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "consecutive", Count: 3}
	preview, err := f.s.PreviewReviewRuleV2(t.Context(), spec, f.instrument.ID.String(), "2026-10-06", "")
	if err != nil || len(preview.Days) != 3 || preview.Window.HitCount != 3 || preview.Window.ConsecutiveCount != 6 || preview.Window.Truth != domain.ReviewTrue {
		t.Fatalf("three-day window must display six-day trailing streak: %+v %v", preview, err)
	}
}

func TestReviewRulePreviewMarksSkippedApiaDateUnknown(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "Pacific/Apia", "2011-12-29", "1")
	f.day(t, "2011-12-31") // the local 2011-12-30 never existed
	spec := syntheticHistoricalReviewSpec()
	spec.Schedule.Timezone = "Pacific/Apia"
	preview, err := f.s.PreviewReviewRuleV2(t.Context(), spec, f.instrument.ID.String(), "2011-12-30", "")
	if err != nil || len(preview.Days) != 1 || preview.Days[0].Result.Truth != domain.ReviewUnknown || preview.Window.Truth != domain.ReviewUnknown || len(preview.Days[0].Coverage.Missing) != 1 || preview.Days[0].Coverage.Missing[0] != "nonexistent_local_date" {
		t.Fatalf("skipped local day needs one U slot: %+v %v", preview, err)
	}
	// The authoritative planner must agree with preview on the same civil day.
	rule := domain.ReviewRule{ID: domain.NewReviewRuleID(), HouseholdID: f.account.Account.HouseholdID, Evaluator: domain.ReviewEvaluator, CurrentConditionVersion: 1, Spec: spec}
	batch, err := f.s.repository.(ReviewHistoricalBatchReader).LoadReviewHistoricalBatch(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	capture := domain.ReviewProjectionCapture{HouseholdID: f.account.Account.HouseholdID, Historical: batch}
	plan, err := PlanReviewRuleInstrument(t.Context(), capture, rule, f.instrument.ID, *f.now, "")
	if err != nil || len(plan.Days) != 2 || plan.Days[1].On != "2011-12-30" || plan.Days[1].Result.Truth != domain.ReviewUnknown || plan.LatestWindow.Truth != preview.Window.Truth {
		t.Fatalf("preview and authoritative day plan diverged: %+v %v", plan, err)
	}
}
