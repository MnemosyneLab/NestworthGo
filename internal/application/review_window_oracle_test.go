package application

import (
	"reflect"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewWindowGapFallsBackToFullOracle(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "2")
	spec := syntheticHistoricalReviewSpec()
	spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 7, MinHits: 3}
	rule, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic gap oracle", Enabled: true, Severity: "warning", Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-08-31")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Keep the final date and streak anchor but remove a date in the trailing
	// interval. Both SQL paths still select W stored slots; planning must repair.
	if _, err := f.db.SQL.Exec(`DELETE FROM investment_review_daily_inputs WHERE on_date='2026-08-29'`); err != nil {
		t.Fatal(err)
	}
	repository := sqlite.NewRepository(f.db)
	candidate, err := repository.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := repository.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(candidate, baseline) {
		t.Fatal("sparse slot captures differ")
	}
	plan, err := PlanReviewRuleInstrument(t.Context(), candidate, rule, f.instrument.ID, *f.now, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Incremental || len(plan.Days) != 30 {
		t.Fatalf("gap failed to force full repair: incremental=%t days=%d", plan.Incremental, len(plan.Days))
	}
	full, err := repository.WithFullReviewRecompute().CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := PlanReviewRuleInstrument(t.Context(), full, rule, f.instrument.ID, *f.now, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan, expected) {
		t.Fatal("gap repair differs from independently captured full oracle")
	}
}
