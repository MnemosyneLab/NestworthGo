package domain

import (
	"reflect"
	"testing"
)

func TestPlanReviewInvalidationOldNewAndFallback(t *testing.T) {
	rules := []ReviewRule{{ID: "rule-a"}, {ID: "rule-b"}}
	quote := func(id, on string) *ReviewQuoteDependency {
		return &ReviewQuoteDependency{InstrumentID: InstrumentID(id), EffectiveDate: on}
	}
	plan := PlanReviewInvalidation([]ReviewSourceChange{{Table: "instrument_quotes", Operation: "update", Old: quote("old", "2026-10-02"), New: quote("new", "2026-10-09")}, {Table: "instrument_quotes", Operation: "delete", Old: quote("new", "2026-10-01")}}, rules)
	if plan.Full || !reflect.DeepEqual(plan.Dirty, []ReviewDirtyPair{{"rule-a", "new", "2026-10-01"}, {"rule-a", "old", "2026-10-02"}, {"rule-b", "new", "2026-10-01"}, {"rule-b", "old", "2026-10-02"}}) {
		t.Fatalf("wrong OLD/NEW union %+v", plan)
	}
	for _, change := range []ReviewSourceChange{{Table: "activities", Operation: "update"}, {Table: "fx_quotes", Operation: "delete"}, {Table: "instrument_quotes", Operation: "delete"}, {Table: "instrument_quotes", Operation: "insert", New: quote("x", "")}, {Table: "instrument_quotes", Operation: "update", Old: quote("x", "2026-10-01")}} {
		if !PlanReviewInvalidation([]ReviewSourceChange{change}, rules).Full {
			t.Fatal("incomplete dependency did not fall back", change)
		}
	}
	before := []ReviewQuoteDaySeal{{InstrumentID: "old", On: "2026-10-02", Fingerprint: "one"}}
	after := []ReviewQuoteDaySeal{{InstrumentID: "new", On: "2026-10-09", Fingerprint: "two"}}
	if !ReviewQuoteChangesCovered(before, after, plan, rules) {
		t.Fatal("complete scopes rejected")
	}
	plan.Dirty = plan.Dirty[:1]
	if ReviewQuoteChangesCovered(before, after, plan, rules) {
		t.Fatal("omitted OLD scope accepted")
	}
}

func TestReviewSuffixFingerprintIncludesPrivateFXScalar(t *testing.T) {
	first, err := ParseFXRate("2")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseFXRate("3")
	if err != nil {
		t.Fatal(err)
	}
	input := ReviewDailyInput{SelectedFX: map[CurrencyCode]ReviewSelectedFX{CurrencyCode("USD"): {Quote: FXQuote{Rate: first}}}}
	suffix := &ReviewSuffixCapture{Prefix: []ReviewSuffixDay{{Evidence: ReviewReusableDailyEvidence{Input: input}}}}
	before, err := ReviewSuffixFingerprint(suffix)
	if err != nil {
		t.Fatal(err)
	}
	selection := suffix.Prefix[0].Evidence.Input.SelectedFX[CurrencyCode("USD")]
	selection.Quote.Rate = second
	suffix.Prefix[0].Evidence.Input.SelectedFX[CurrencyCode("USD")] = selection
	after, err := ReviewSuffixFingerprint(suffix)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("exact selected FX rate escaped prefix authentication")
	}
}
