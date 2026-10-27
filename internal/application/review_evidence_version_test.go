package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
)

func TestReviewEvidenceKeepsActualVersionThroughPendingFailedAndRetainedEdit(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	requireDailyReviewPrices(t, &f)
	importReviewDailyClose(t, f, "2026-10-01", "100")
	f.day(t, "2026-10-08")
	importReviewDailyClose(t, f, "2026-10-08", "98")
	f.day(t, "2026-10-09")
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
		{Leaf: &domain.ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []domain.InstrumentType{domain.InstrumentCrypto}}},
		{Any: []domain.ReviewPredicateNode{
			{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: "<", Value: "0", LookbackDays: 7}},
			{Leaf: &domain.ReviewRuleLeaf{Metric: "cost", Operator: "<", Value: "1", Currency: "CNY", FXMode: "native"}},
		}},
	}}
	rule := saveSyntheticReviewRule(t, f, "Synthetic versioned evidence", spec)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{}, "", 10)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("initial %+v %v", page, err)
	}
	episode := page.Items[0].Rules[0].EpisodeID
	priceLeaf := func(node *domain.ReviewPredicateNode) *domain.ReviewRuleLeaf {
		t.Helper()
		if node == nil || len(node.All) != 2 {
			t.Fatalf("lost AND context: %+v", node)
		}
		for _, child := range node.All {
			if len(child.Any) == 2 {
				for _, alternative := range child.Any {
					if alternative.Leaf != nil && alternative.Leaf.Metric == "price_change" {
						return alternative.Leaf
					}
				}
			}
		}
		t.Fatal("lost OR/price context")
		return nil
	}
	check := func(wantDays int, wantThreshold string) {
		t.Helper()
		listed, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{}, "", 10)
		if err != nil || len(listed.Items) != 1 {
			t.Fatalf("list %+v %v", listed, err)
		}
		got, err := f.s.GetReviewAlertV2(t.Context(), string(episode))
		if err != nil {
			t.Fatal(err)
		}
		for _, group := range []domain.ReviewAlertGroup{listed.Items[0], got} {
			result := group.Rules[0].LatestResult
			if result == nil || result.Predicate == nil {
				t.Fatalf("lost AND/OR context: %+v", result)
			}
			leaf := priceLeaf(result.Predicate)
			if leaf.LookbackDays != wantDays || leaf.Value != wantThreshold {
				t.Fatalf("wrong condition: %+v", leaf)
			}
			if wantDays == 7 && (result.Leaves[1].Actual != "-2" || result.Leaves[1].Truth != domain.ReviewTrue) {
				t.Fatalf("original vote changed %+v", result)
			}
		}
		daily, err := f.s.ListReviewDailyEvidenceV2(t.Context(), string(episode), "", 100)
		if err != nil || len(daily.Items) == 0 {
			t.Fatalf("daily %+v %v", daily, err)
		}
		for _, day := range daily.Items {
			if day.Predicate == nil || priceLeaf(day.Predicate).LookbackDays != wantDays {
				t.Fatalf("wrong daily context %+v", day)
			}
		}
	}
	check(7, "0")
	spec.Predicate.All[1].Any[0].Leaf.LookbackDays = 30
	spec.Predicate.All[1].Any[0].Leaf.Value = "-10"
	_, err = f.s.UpdateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(rule.ID), MutationID: string(domain.NewMutationID()), ExpectedRevision: rule.RowRevision, Name: rule.Name, Enabled: true, Severity: rule.Severity, Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	check(7, "0") // Current rule is v2, but the published evidence is still v1.
	if _, err := f.db.SQL.Exec(`CREATE TRIGGER synthetic_fail_review_publish BEFORE INSERT ON investment_review_daily_results BEGIN SELECT RAISE(ABORT,'synthetic recomputation failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err == nil {
		t.Fatal("expected failed recomputation")
	}
	check(7, "0")
	if _, err := f.db.SQL.Exec(`DROP TRIGGER synthetic_fail_review_publish`); err != nil {
		t.Fatal(err)
	}
	// A second version with identical vote bytes cannot prove the condition.
	if _, err := f.db.SQL.Exec(`INSERT INTO investment_review_daily_results(rule_id,condition_version,instrument_id,timezone_version,on_date,predicate_truth,reason,evidence_json,input_fingerprint,evaluated_at) SELECT rule_id,2,instrument_id,timezone_version,on_date,predicate_truth,reason,evidence_json,input_fingerprint,evaluated_at FROM investment_review_daily_results WHERE rule_id=? AND condition_version=1`, string(rule.ID)); err != nil {
		t.Fatal(err)
	}
	ambiguous, err := f.s.GetReviewAlertV2(t.Context(), string(episode))
	if err != nil || ambiguous.Rules[0].LatestResult.Predicate != nil {
		t.Fatalf("ambiguous evidence borrowed a condition %+v %v", ambiguous, err)
	}
	if _, err := f.s.ListReviewDailyEvidenceV2(t.Context(), string(episode), "", 100); err == nil {
		t.Fatal("ambiguous daily version exposed")
	}
	if _, err := f.db.SQL.Exec(`DELETE FROM investment_review_daily_results WHERE rule_id=? AND condition_version=2`, string(rule.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	check(30, "-10") // The same episode can now contain v2 evidence.
	var opened int
	if err := f.db.SQL.QueryRow(`SELECT opened_condition_version FROM investment_review_episodes WHERE id=?`, string(episode)).Scan(&opened); err != nil || opened != 1 {
		t.Fatalf("expected retained v1 episode %d %v", opened, err)
	}
}
