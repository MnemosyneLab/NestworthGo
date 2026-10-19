package application

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewCertifiedRuleAndSuppressionContinuity(t *testing.T) {
	for _, kind := range []string{"condition", "policy", "timezone", "suppression"} {
		t.Run(kind, func(t *testing.T) {
			f, r := certifiedFixture(t)
			if kind == "suppression" {
				_, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: f.instrument.ID.String(), Mode: "until_date", UntilOn: "2026-10-14"})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				rules, err := f.s.ListReviewRulesV2(t.Context(), "", 10)
				if err != nil {
					t.Fatal(err)
				}
				rule := rules[0]
				switch kind {
				case "condition":
					for i := range rule.Spec.Predicate.All {
						if rule.Spec.Predicate.All[i].Leaf.Metric == "cost" {
							rule.Spec.Predicate.All[i].Leaf.Value = "50"
						}
					}
				case "policy":
					rule.Spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "consecutive", Count: 3}
				case "timezone":
					rule.Spec.Schedule.Timezone = "Asia/Shanghai"
				}
				_, err = f.s.UpdateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(rule.ID), MutationID: string(domain.NewMutationID()), ExpectedRevision: rule.RowRevision, Name: rule.Name, Enabled: rule.Enabled, Severity: rule.Severity, Spec: rule.Spec})
				if err != nil {
					t.Fatal(err)
				}
			}
			stage := certifiedStage(t, f, r)
			assertReviewFullOracle(t, reviewFullPipelineFixture{db: f.db, service: f.s, householdID: f.account.Account.HouseholdID, now: f.now}, stage)
			got, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
			if err != nil || got.DayCount == 0 {
				t.Fatal("mutation skipped reconciliation", got, err)
			}
			*f.now = f.now.Add(time.Nanosecond)
			got, err = f.s.ReconcileInvestmentReviewsV2(t.Context())
			if err != nil || got.DayCount != 0 {
				t.Fatal("legitimate mutation broke chain", got, err)
			}
			f.day(t, "2026-10-13")
			p := certifiedStage(t, f, r)
			if err := certifiedPublish(t.Context(), f, r, p, func() time.Time { return *f.now }); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReviewCertifiedConnectionRenewalAndNoopCancellation(t *testing.T) {
	f, r := certifiedFixture(t)
	f.db.SQL.SetMaxIdleConns(0)
	f.db.SQL.SetMaxIdleConns(1)
	*f.now = f.now.Add(time.Nanosecond)
	_, unchanged, err := r.ConfirmCertifiedReviewNoChange(t.Context(), f.account.Account.HouseholdID, *f.now, func() time.Time { return *f.now }, f.s.reviewProviderSnapshot())
	if err != nil || unchanged {
		t.Fatal("connection renewal incorrectly confirmed no-op", unchanged, err)
	}
	got, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	*f.now = f.now.Add(time.Nanosecond)
	got, err = f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || got.DayCount != 0 {
		t.Fatal("full renewal did not recover", got, err)
	}
	before := noChangeProjectionRows(t, f.db)
	var raw string
	if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	_, ok, err := r.ConfirmCertifiedReviewNoChange(ctx, f.account.Account.HouseholdID, *f.now, func() time.Time { cancel(); return *f.now }, f.s.reviewProviderSnapshot())
	if ok || err == nil {
		t.Fatal("cancelled confirmation succeeded", ok, err)
	}
	var after string
	if err := f.db.SQL.QueryRow(`SELECT certificate_json FROM review_temporal_checkpoints`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if raw != after || !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
		t.Fatal("cancelled confirmation committed partial state")
	}
}
