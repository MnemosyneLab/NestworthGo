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
