package domain

import (
	"testing"
)

func TestDescriptivePerformanceDatesAndAlertIndependence(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentBankInvestmentProduct
	for _, tc := range []struct {
		base, end string
		days      int
	}{{"2026-09-30", "2026-10-09", 9}, {"2024-02-20", "2024-03-01", 10}, {"2026-08-01", "2026-10-09", 69}} {
		points := []InstrumentQuote{reviewQuote(t, i, tc.base, "1"), reviewQuote(t, i, tc.end, "1.009")}
		got := ProjectQuotePerformance(i, points, nil)
		if len(got) == 0 || got[0].ElapsedDays != tc.days || got[0].BaselineOn != tc.base || got[0].LatestOn != tc.end || got[0].PeriodPercent != "0.9" || got[0].AnnualizedPercent == nil {
			t.Fatalf("performance %+v", got)
		}
	}
	points := []InstrumentQuote{reviewQuote(t, i, "2026-09-24", "1"), reviewQuote(t, i, "2026-10-05", "1.01"), reviewQuote(t, i, "2026-10-14", "1.02")}
	got := ProjectQuotePerformance(i, points, nil)
	if len(got) != 1 || got[0].WindowDays != 7 || got[0].ElapsedDays != 9 {
		t.Fatalf("20-day history %+v", got)
	}
}
func TestDescriptivePerformanceRejectsIncomparableEvidence(t *testing.T) {
	_, i := reviewFixture(t)
	for _, kind := range []string{"flat_nav", "currency", "binding", "distribution", "whole_value", "source", "unsafe", "single"} {
		t.Run(kind, func(t *testing.T) {
			points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "1"), reviewQuote(t, i, "2026-10-09", "1.02")}
			var unsafe []string
			switch kind {
			case "flat_nav":
				points[1].UnitPrice = points[0].UnitPrice
			case "currency":
				points[0].Currency = "CNY"
			case "binding":
				points[0].BindingRevision = 2
			case "distribution":
				points[1].DividendCash = "0.1"
			case "whole_value":
				points[0].PriceBasis = "total_value"
			case "source":
				points[0].SourceKey = "other"
			case "unsafe":
				unsafe = []string{"2026-10-04"}
			case "single":
				points = points[:1]
			}
			if got := ProjectQuotePerformance(i, points, unsafe); len(got) != 0 {
				t.Fatalf("unexpected %+v", got)
			}
		})
	}
}
func TestDescriptiveStockFlatPriceAndNoSplitFactorOne(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentStock
	points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "100"), reviewQuote(t, i, "2026-10-09", "100")}
	for n := range points {
		points[n].PriceBasis = "tiingo_raw_close_v1"
		points[n].SplitFactor = "1"
	}
	got := ProjectQuotePerformance(i, points, nil)
	if len(got) != 1 || got[0].PeriodPercent != "0" || got[0].AnnualizedPercent != nil {
		t.Fatalf("flat stock %+v", got)
	}
	points[1].SplitFactor = "2"
	if got := ProjectQuotePerformance(i, points, nil); len(got) != 0 {
		t.Fatalf("split %+v", got)
	}
}
