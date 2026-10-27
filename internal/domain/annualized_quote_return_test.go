package domain

import "testing"

func TestAnnualizedQuoteReturnEligibilityAndEvidence(t *testing.T) {
	for _, kind := range []InstrumentType{InstrumentBankInvestmentProduct, InstrumentMutualFund, InstrumentETF} {
		t.Run(string(kind), func(t *testing.T) {
			_, instrument := reviewFixture(t)
			instrument.Type = kind
			quotes := []InstrumentQuote{reviewQuote(t, instrument, "2026-10-01", "100"), reviewQuote(t, instrument, "2026-10-10", "101")}
			if kind == InstrumentETF {
				for n := range quotes {
					quotes[n].PriceBasis = "agent_raw_close_v1"
				}
			}
			got := ProjectQuotePerformance(instrument, quotes)
			if len(got) != 1 || got[0].AnnualizedPercent == nil || *got[0].AnnualizedPercent != "40.555555555556" {
				t.Fatalf("descriptive return: %+v", got)
			}
			daily, reason := NewReviewDailyQuoteSeries(instrument.ID, quotes).ResolveDailyPerformance(instrument, "2026-10-10", 7, 3)
			if reason != "" || daily == nil || daily.AnnualizedPercent == nil || *daily.AnnualizedPercent != *got[0].AnnualizedPercent {
				t.Fatalf("review and detail disagree: %+v %s", daily, reason)
			}
			quotes[0].EffectiveDate = "invalid"
			if invalid := ProjectQuotePerformance(instrument, quotes); len(invalid) != 0 {
				t.Fatalf("invalid date produced return: %+v", invalid)
			}
		})
	}
}

func TestAnnualizedReviewScopesSupportedSetsAndRejectsExclusions(t *testing.T) {
	for _, kinds := range [][]InstrumentType{{InstrumentMutualFund}, {InstrumentETF}, {InstrumentMutualFund, InstrumentETF, InstrumentBankInvestmentProduct}} {
		spec := syntheticReviewSpec()
		spec.Predicate.All[0].Leaf.Types = kinds
		if _, err := spec.Normalize(); err != nil {
			t.Fatalf("supported types %v: %v", kinds, err)
		}
	}
	for _, scenario := range []string{"negative gate", "unsupported member", "or sibling"} {
		spec := syntheticReviewSpec()
		switch scenario {
		case "negative gate":
			spec.Predicate.All[0].Leaf.Operator = "not_in"
		case "unsupported member":
			spec.Predicate.All[0].Leaf.Types = []InstrumentType{InstrumentETF, InstrumentStock}
		case "or sibling":
			spec.Predicate.Any, spec.Predicate.All = spec.Predicate.All, nil
		}
		if _, err := spec.Normalize(); err == nil {
			t.Fatalf("accepted %s", scenario)
		}
	}
}
