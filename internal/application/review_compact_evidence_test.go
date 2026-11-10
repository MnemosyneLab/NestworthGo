package application

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewCompactEvidenceFindsKnownOutsidePageWithHistoricalPosition(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	requireDailyReviewPrices(t, &f)
	importReviewDailyClose(t, f, "2026-10-01", "100")
	f.day(t, "2026-10-08")
	importReviewDailyClose(t, f, "2026-10-08", "98")
	f.day(t, "2026-10-09")
	importReviewDailyClose(t, f, "2026-10-09", "105")
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
		{Leaf: &domain.ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []domain.InstrumentType{domain.InstrumentCrypto}}},
		{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: "<", Value: "0", LookbackDays: 7}},
		{Leaf: &domain.ReviewRuleLeaf{Metric: "cost", Operator: "<", Value: "250", Currency: "CNY", FXMode: "native"}},
	}}
	spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 7, MinHits: 1}
	rule := saveSyntheticReviewRule(t, f, "Synthetic compact evidence", spec)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-10-10")
	f.quantity(t, true) // Today's two units must never backfill an earlier one-unit vote.
	f.day(t, "2026-11-10")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var episode string
	if err := f.db.SQL.QueryRow(`SELECT id FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(&episode); err != nil {
		t.Fatal(err)
	}
	first, err := f.s.ListReviewDailyEvidenceV2(t.Context(), episode, "", 20)
	if err != nil || len(first.Items) != 20 || first.NextCursor == "" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	if first.Items[0].On != "2026-11-09" || first.Items[0].Truth != domain.ReviewUnknown {
		t.Fatalf("latest unknown lost: %+v", first.Items[0])
	}
	priceActual := func(result *domain.ReviewDailyResult) string {
		if result != nil {
			for _, leaf := range result.Leaves {
				if leaf.Metric == "price_change" {
					return leaf.Actual
				}
			}
		}
		return ""
	}
	known := first.LatestKnownResult
	if known == nil || known.On != "2026-10-09" || known.Truth != domain.ReviewFalse || known.Predicate == nil || priceActual(known) != "5" {
		t.Fatalf("known result clipped or wrong: %+v", known)
	}
	if known.AmountExact == nil || *known.AmountExact != "105" || known.CostExact == nil || *known.CostExact != "100" || known.Currency == nil || *known.Currency != "CNY" {
		t.Fatalf("historical position wrong: %+v", known)
	}
	if first.ConditionVersion != 1 || first.EpisodeRevision < 1 {
		t.Fatalf("missing evidence context: %+v", first)
	}
	for _, item := range first.Items {
		if item.On == known.On {
			t.Fatal("fixture does not exercise a known result outside page one")
		}
	}
	second, err := f.s.ListReviewDailyEvidenceV2(t.Context(), episode, first.NextCursor, 20)
	if err != nil || second.LatestKnownResult == nil || second.LatestKnownResult.On != known.On || second.ReportRevision != first.ReportRevision {
		t.Fatalf("cursor changed known result: %+v %v", second, err)
	}
	alerts, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{}, "", 10)
	if err != nil || len(alerts.Items) != 1 {
		t.Fatalf("alert: %+v %v", alerts, err)
	}
	alert := alerts.Items[0].Rules[0]
	if alert.TriggerWindow == nil || alert.TriggerWindow.WindowEnd != "2026-10-08" || alert.TriggerWindow.HitCount != 1 || alert.Window.WindowEnd != "2026-11-09" {
		t.Fatalf("trigger and current windows mixed: %+v", alert)
	}

	// A valid input from a different fingerprint is not authority for this vote.
	if _, err := f.db.SQL.Exec(`UPDATE investment_review_daily_inputs SET input_fingerprint='synthetic-mismatch' WHERE instrument_id=? AND on_date='2026-10-09'`, f.instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	missing, err := f.s.ListReviewDailyEvidenceV2(t.Context(), episode, "", 20)
	if err != nil || missing.LatestKnownResult == nil {
		t.Fatalf("mismatched input read: %+v %v", missing, err)
	}
	if missing.LatestKnownResult.AmountExact != nil || missing.LatestKnownResult.CostExact != nil || missing.LatestKnownResult.Currency != nil || priceActual(missing.LatestKnownResult) != "5" {
		t.Fatalf("mismatched input leaked position or erased vote: %+v", missing.LatestKnownResult)
	}
}
