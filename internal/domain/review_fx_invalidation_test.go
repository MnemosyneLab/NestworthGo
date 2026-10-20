package domain

import (
	"reflect"
	"testing"
	"time"
)

func reviewFXProofFixture(t *testing.T) ReviewProjectionCapture {
	t.Helper()
	rate, err := ParseFXRate("2.123456789123")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	q := FXQuote{ID: NewFXQuoteID(), BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate, SourceKind: QuoteSourceAgent, SourceKey: "agent", EffectiveDate: "2026-10-01", ObservationKind: "daily_reference", ValueEffectiveAt: at, QuotedAt: at, SourcePolicyVersion: "synthetic", TimestampBasis: "date_label"}
	later := q
	later.ID = NewFXQuoteID()
	later.EffectiveDate = "2026-10-08"
	later.ValueEffectiveAt = at.AddDate(0, 0, 7)
	later.QuotedAt = later.ValueEffectiveAt
	return ReviewProjectionCapture{Rules: []ReviewRule{{ID: "rule", Spec: ReviewRuleSpec{Predicate: ReviewPredicateNode{Leaf: &ReviewRuleLeaf{Metric: "cost", Currency: "CNY", FXMode: "convert_on_day"}}}}}, Historical: ReviewHistoricalBatch{History: HistoricalSnapshotBatch{FXQuoteFacts: []FXQuote{q, later}, Portfolio: PortfolioSnapshot{Instruments: []Instrument{{ID: "instrument", QuoteCurrency: "USD"}}, FXQuotes: []FXQuote{later}}}}}
}
func TestReviewFXDescriptionCompleteCanonicalCandidates(t *testing.T) {
	c := reviewFXProofFixture(t)
	before, err := DescribeReviewFX(c.Historical)
	if err != nil || !before.Eligible {
		t.Fatal(before, err)
	}
	reversed := c.Historical
	reversed.History.FXQuoteFacts = append([]FXQuote{}, c.Historical.History.FXQuoteFacts...)
	reversed.History.FXQuoteFacts[0], reversed.History.FXQuoteFacts[1] = reversed.History.FXQuoteFacts[1], reversed.History.FXQuoteFacts[0]
	canonical, err := DescribeReviewFX(reversed)
	if err != nil || !reflect.DeepEqual(before, canonical) {
		t.Fatal("candidate ordering changed the description", err)
	}
	for _, scenario := range []string{"rate", "selection", "portfolio-only", "missing-fact", "duplicate", "timestamp", "pair"} {
		t.Run(scenario, func(t *testing.T) {
			batch := c.Historical
			batch.History.FXQuoteFacts = append([]FXQuote{}, batch.History.FXQuoteFacts...)
			batch.History.Portfolio.FXQuotes = append([]FXQuote{}, batch.History.Portfolio.FXQuotes...)
			switch scenario {
			case "rate":
				batch.History.FXQuoteFacts[0].Rate, _ = ParseFXRate("3.123456789123")
			case "selection":
				batch.History.FXQuoteFacts[0].SourcePolicyVersion = "changed"
			case "portfolio-only":
				batch.History.Portfolio.FXQuotes[0].Rate, _ = ParseFXRate("3")
			case "missing-fact":
				batch.History.FXQuoteFacts = batch.History.FXQuoteFacts[:1]
			case "duplicate":
				batch.History.FXQuoteFacts = append(batch.History.FXQuoteFacts, batch.History.FXQuoteFacts[0])
			case "timestamp":
				batch.History.FXQuoteFacts[0].ObservationKind = "manual"
			case "pair":
				batch.History.FXQuoteFacts[0].BaseCurrency = "EUR"
			}
			after, e := DescribeReviewFX(batch)
			if e != nil {
				t.Fatal(e)
			}
			if reflect.DeepEqual(before.Buckets, after.Buckets) {
				t.Fatal("candidate change escaped complete inventory")
			}
			if after.NonFXFingerprint != before.NonFXFingerprint {
				t.Fatal("FX payload escaped its separable inventory")
			}
			if scenario != "rate" && scenario != "selection" && after.Eligible {
				t.Fatal("unproven inventory eligible")
			}
		})
	}
}
func TestReviewFXPlanRequiresCompleteOldNewCoverage(t *testing.T) {
	c := reviewFXProofFixture(t)
	before, err := DescribeReviewFX(c.Historical)
	if err != nil {
		t.Fatal(err)
	}
	q := c.Historical.History.FXQuoteFacts[0]
	c.Historical.History.FXQuoteFacts[0].Rate, _ = ParseFXRate("3")
	after, err := DescribeReviewFX(c.Historical)
	if err != nil {
		t.Fatal(err)
	}
	scope := &ReviewFXDependency{q.ID, q.BaseCurrency, q.QuoteCurrency, q.EffectiveDate}
	changes := []ReviewSourceChange{{Table: "fx_quotes", Operation: "update", OldFX: scope, NewFX: scope}}
	plan := PlanReviewFXInvalidation(changes, before, after, c)
	if plan.Full || !reflect.DeepEqual(plan.Dirty, []ReviewDirtyPair{{"rule", "instrument", "2026-10-01"}}) {
		t.Fatal("complete candidate correction rejected", plan)
	}
	for _, scenario := range []string{"missing-description", "legacy-version", "missing-journal", "wrong-date", "hidden-delete", "mixed", "selection", "inverse-rule", "date-move", "non-fx"} {
		t.Run(scenario, func(t *testing.T) {
			b, a := *before, *after
			cc := c
			cc.Rules = append([]ReviewRule{}, c.Rules...)
			cs := append([]ReviewSourceChange{}, changes...)
			switch scenario {
			case "missing-description":
				if !PlanReviewFXInvalidation(cs, nil, &a, cc).Full {
					t.Fatal("missing description accepted")
				}
				return
			case "legacy-version":
				b.Version = "legacy"
			case "missing-journal":
				cs = nil
			case "wrong-date":
				bad := *scope
				bad.On = "2026-10-09"
				cs[0].OldFX = &bad
				cs[0].NewFX = &bad
			case "hidden-delete":
				cs[0].Operation = "insert"
				cs[0].OldFX = nil
			case "mixed":
				cs = append(cs, ReviewSourceChange{Table: "instrument_quotes", Operation: "update"})
			case "selection":
				cc.Historical.History.FXQuoteFacts = append([]FXQuote{}, c.Historical.History.FXQuoteFacts...)
				cc.Historical.History.FXQuoteFacts[0].SourceKey = "different"
				desc, e := DescribeReviewFX(cc.Historical)
				if e != nil {
					t.Fatal(e)
				}
				a = *desc
			case "inverse-rule":
				cc.Rules[0].Spec.Predicate = ReviewPredicateNode{Leaf: &ReviewRuleLeaf{Currency: "USD", FXMode: "convert_on_day"}}
			case "date-move":
				bad := *scope
				bad.On = "2026-10-06"
				cs[0].NewFX = &bad
			case "non-fx":
				a.NonFXFingerprint = "changed"
			}
			if !PlanReviewFXInvalidation(cs, &b, &a, cc).Full {
				t.Fatal("unproven change accepted")
			}
		})
	}
}
