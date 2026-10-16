package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func syntheticHistoricalReviewSpec() domain.ReviewRuleSpec {
	return domain.ReviewRuleSpec{
		SpecVersion: domain.ReviewSpecVersion, MetricPolicyVersion: domain.ReviewMetricPolicyVersion, Evaluator: domain.ReviewEvaluator,
		Target: domain.ReviewRuleTarget{Kind: "all_current"},
		Predicate: domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
			{Leaf: &domain.ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []domain.InstrumentType{domain.InstrumentStock}}},
			{Leaf: &domain.ReviewRuleLeaf{Metric: "cost", Operator: "<", Value: "250", Currency: "CNY", FXMode: "native"}},
		}},
		Schedule:      domain.ReviewRuleSchedule{Period: "calendar_day", Timezone: "UTC", CutoffVersion: domain.ReviewCutoffVersion},
		TriggerPolicy: domain.ReviewTriggerPolicy{Kind: "single"},
		DataPolicy:    domain.DefaultReviewRuleDataPolicy(),
	}
}

func TestReviewHistoricalBatchDoesNotBackfillCurrentHolding(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "2")
	f.day(t, "2026-08-02")
	f.quantity(t, false)
	f.day(t, "2026-08-03")
	f.quantity(t, false) // current position is zero, but prior days were positive
	repo := sqlite.NewRepository(f.db)
	var before, after int
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	batch, err := repo.LoadReviewHistoricalBatch(t.Context(), f.account.Account.HouseholdID, f.now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		on, quantity, cost string
		truth              domain.ReviewTruth
	}{
		{"2026-08-01", "2", "200", domain.ReviewTrue},
		{"2026-08-02", "1", "100", domain.ReviewTrue},
		{"2026-08-03", "0", "", domain.ReviewFalse},
	} {
		input, _, err := ProjectReviewHistoricalDay(t.Context(), batch, "", syntheticHistoricalReviewSpec(), f.instrument.ID, tc.on)
		if err != nil {
			t.Fatal(err)
		}
		if input.Quantity == nil || *input.Quantity != tc.quantity || (tc.cost != "" && (input.CostExact == nil || *input.CostExact != tc.cost)) {
			t.Fatalf("%s: quantity=%v cost=%v", tc.on, input.Quantity, input.CostExact)
		}
		result, err := domain.EvaluateReviewDaily(syntheticHistoricalReviewSpec(), input)
		if err != nil || result.Truth != tc.truth {
			t.Fatalf("%s: result=%+v err=%v", tc.on, result, err)
		}
	}
	// Missing cost events are unknown, never a fabricated zero or an inherited
	// current-day cost. The separately proven quantity remains usable.
	delete(batch.CostEvents, f.holding.ID)
	input, coverage, err := ProjectReviewHistoricalDay(t.Context(), batch, "", syntheticHistoricalReviewSpec(), f.instrument.ID, "2026-08-01")
	if err != nil || input.CostExact != nil || input.Quantity == nil || *input.Quantity != "2" || len(coverage.Missing) == 0 {
		t.Fatalf("missing cost must remain unknown: %+v %+v %v", input, coverage, err)
	}
	result, err := domain.EvaluateReviewDaily(syntheticHistoricalReviewSpec(), input)
	if err != nil || result.Truth != domain.ReviewUnknown {
		t.Fatalf("unknown cost became hit: %+v %v", result, err)
	}
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("read-only projection wrote %d changes", after-before)
	}
}

func TestReviewHistoricalDayBeforeOriginIsUnknown(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "2")
	repo := sqlite.NewRepository(f.db)
	batch, err := repo.LoadReviewHistoricalBatch(t.Context(), f.account.Account.HouseholdID, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	input, coverage, err := ProjectReviewHistoricalDay(t.Context(), batch, "", syntheticHistoricalReviewSpec(), f.instrument.ID, "2026-07-31")
	if err != nil || input.Quantity != nil || len(coverage.Missing) != 1 || coverage.Missing[0] != "history_origin" {
		t.Fatalf("day before authoritative origin: %+v %+v %v", input, coverage, err)
	}
}
