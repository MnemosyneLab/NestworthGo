package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
)

func TestReviewPriorityAliasesAndPersistence(t *testing.T) {
	s, _, _, _ := overviewFixture(t)
	for _, item := range []struct{ input, want string }{{"notice", "p2"}, {"warning", "p1"}, {"p0", "p0"}, {"p1", "p1"}, {"p2", "p2"}} {
		input := SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic priority", Enabled: true, Severity: item.input, Spec: syntheticHistoricalReviewSpec()}
		created, err := s.CreateReviewRuleV2(t.Context(), input)
		if err != nil || created.Severity != item.want {
			t.Fatalf("create %s: %+v %v", item.input, created, err)
		}
		retry, err := s.CreateReviewRuleV2(t.Context(), input)
		if err != nil || retry.ID != created.ID || retry.Severity != item.want {
			t.Fatalf("retry: %+v %v", retry, err)
		}
		input.Severity, input.ExpectedRevision, input.MutationID = "p0", created.RowRevision, string(domain.NewMutationID())
		updated, err := s.UpdateReviewRuleV2(t.Context(), input)
		if err != nil || updated.Severity != "p0" || updated.CurrentConditionVersion != created.CurrentConditionVersion {
			t.Fatalf("priority-only update: %+v %v", updated, err)
		}
	}
}

func TestReviewPriorityOrdersAlertsAndPreservesEvidenceContext(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	f.day(t, "2026-08-02")
	for _, priority := range []string{"notice", "p0", "warning"} {
		_, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic " + priority, Enabled: true, Severity: priority, Spec: syntheticHistoricalReviewSpec()})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{Status: "pending"}, "", 1)
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Rules) != 3 {
		t.Fatalf("alerts %+v %v", page, err)
	}
	for n, want := range []string{"p0", "p1", "p2"} {
		rule := page.Items[0].Rules[n]
		if rule.Severity != want || rule.LatestResult == nil || rule.LatestResult.Predicate == nil {
			t.Fatalf("rule %d: %+v", n, rule)
		}
		daily, err := f.s.ListReviewDailyEvidenceV2(t.Context(), string(rule.EpisodeID), "", 10)
		if err != nil || len(daily.Items) == 0 || daily.Items[0].Predicate == nil {
			t.Fatalf("daily context %+v %v", daily, err)
		}
	}
}
