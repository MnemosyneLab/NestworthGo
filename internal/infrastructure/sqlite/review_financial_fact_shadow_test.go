package sqlite

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
)

func TestReviewFinancialFactSemanticNamespaces(t *testing.T) {
	p := domain.ReviewPublication{FXProviderKey: "synthetic", Capture: domain.ReviewProjectionCapture{Temporal: &domain.ReviewTemporalCertificate{ClockEpoch: "session-a"}}}
	day := domain.ReviewPublicationDay{On: "2026-10-10"}
	rule := domain.ReviewRule{Spec: domain.ReviewRuleSpec{MetricPolicyVersion: domain.ReviewMetricPolicyVersion, DataPolicy: domain.DefaultReviewRuleDataPolicy(), Schedule: domain.ReviewRuleSchedule{Period: "calendar_day", Timezone: "UTC", CutoffVersion: domain.ReviewCutoffVersion}, Predicate: domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Currency: "CNY", FXMode: "native", Value: "10", Operator: ">"}}}}
	key, err := reviewFactKey(p, rule, day)
	if err != nil {
		t.Fatal(err)
	}
	for _, dimension := range []string{"threshold", "timezone", "currency", "fx", "lookback", "provider", "calendar", "resolver", "policy"} {
		t.Run(dimension, func(t *testing.T) {
			other := rule
			leaf := *rule.Spec.Predicate.Leaf
			other.Spec.Predicate.Leaf = &leaf
			publication := p
			temporal := *p.Capture.Temporal
			publication.Capture.Temporal = &temporal
			switch dimension {
			case "threshold":
				leaf.Value = "20"
				leaf.Operator = "<"
				other.CurrentConditionVersion = 2
				other.ID = domain.NewReviewRuleID()
			case "timezone":
				other.Spec.Schedule.Timezone = "Asia/Shanghai"
			case "currency":
				leaf.Currency = "USD"
			case "fx":
				leaf.FXMode = "convert_on_day"
			case "lookback":
				leaf.LookbackDays = 7
			case "provider":
				publication.FXProviderKey = "other"
			case "calendar":
				temporal.ClockEpoch = "session-b"
			case "resolver":
				publication.Capture.Historical.History.ResolverPolicyVersion = "other"
			case "policy":
				other.Spec.DataPolicy.BaselineMaxGapDays++
			}
			actual, err := reviewFactKey(publication, other, day)
			if err != nil {
				t.Fatal(err)
			}
			if (actual == key) != (dimension == "threshold") {
				t.Fatalf("semantic key collision/separation for %s", dimension)
			}
		})
	}
	r := NewRepository(nil)
	if r.reviewFinancialFactShadowDays != 0 || !r.reviewWindowBaseline {
		t.Fatal("default shadow or B changed")
	}
	if r.WithReviewFinancialFactShadow(10000).reviewFinancialFactShadowDays != 128 || r.WithReviewFinancialFactShadow(-1).reviewFinancialFactShadowDays != 0 || r.reviewFinancialFactShadowDays != 0 {
		t.Fatal("shadow configuration is unbounded or mutated base")
	}
}
