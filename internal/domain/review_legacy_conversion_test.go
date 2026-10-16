package domain

import "testing"

func TestLegacyConversionRequiresExplicitDailyPolicyAndKeepsOnlyTrueTargets(t *testing.T) {
	legacy := ReviewLegacySpec{SpecVersion: 1, Evaluator: ReviewLegacyEvaluator, Configuration: ReviewLegacyConfiguration{
		Policy:       DefaultInvestmentReviewPolicy(),
		Enrolled:     map[string]bool{"00000000-0000-4000-8000-000000000050": false, "00000000-0000-4000-8000-000000000051": true},
		Acknowledged: map[string]bool{"old-unresolvable-hash": true},
	}}
	if _, _, err := BuildReviewLegacyConversion(legacy, "instruments", "UTC", ReviewTriggerPolicy{}); err == nil {
		t.Fatal("silently reused the v1 calendar-span sustainDays as a daily trigger")
	}
	low, loss, err := BuildReviewLegacyConversion(legacy, "instruments", "UTC", ReviewTriggerPolicy{Kind: "consecutive", Count: 7})
	if err != nil {
		t.Fatal(err)
	}
	if low.Target.Kind != "instruments" || len(low.Target.InstrumentIDs) != 1 || low.Target.InstrumentIDs[0] != "00000000-0000-4000-8000-000000000051" || loss.Target.Kind != low.Target.Kind || len(loss.Target.InstrumentIDs) != 1 {
		t.Fatalf("legacy enrollment expanded or excluded a true target: %+v %+v", low.Target, loss.Target)
	}
	if low.TriggerPolicy.Kind != "consecutive" || low.TriggerPolicy.Count != 7 || loss.TriggerPolicy != low.TriggerPolicy || low.Predicate.Any == nil || len(low.Predicate.Any) != 2 || len(loss.Predicate.All) != 2 {
		t.Fatalf("conversion changed independent daily-rule shape: %+v %+v", low, loss)
	}
	if _, err := low.SemanticHash(); err != nil {
		t.Fatal(err)
	}
	broadLow, broadLoss, err := BuildReviewLegacyConversion(legacy, "all_current", "UTC", ReviewTriggerPolicy{Kind: "single"})
	if err != nil || broadLow.Target.Kind != "all_current" || len(broadLow.Target.InstrumentIDs) != 0 || broadLoss.Target.Kind != "all_current" {
		t.Fatalf("explicit target expansion failed: %+v %+v %v", broadLow.Target, broadLoss.Target, err)
	}
	if len(legacy.Configuration.Acknowledged) != 1 || len(legacy.Configuration.Enrolled) != 2 {
		t.Fatal("proposal mutated legacy preferences")
	}
}
