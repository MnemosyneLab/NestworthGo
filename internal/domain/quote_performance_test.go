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
	for _, kind := range []string{"income_distribution", "currency", "binding", "distribution", "whole_value", "unsafe", "single"} {
		t.Run(kind, func(t *testing.T) {
			points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "1"), reviewQuote(t, i, "2026-10-09", "1.02")}
			var unsafe []string
			switch kind {
			case "income_distribution":
				points[1].UnitPrice = points[0].UnitPrice
				points[1].DividendCash = "0.01"
			case "currency":
				points[0].Currency = "CNY"
			case "binding":
				points[0].BindingRevision = 2
			case "distribution":
				points[1].DividendCash = "0.1"
			case "whole_value":
				points[0].PriceBasis = "total_value"
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

func TestDescriptiveYahooEquityPerformance(t *testing.T) {
	for _, kind := range []InstrumentType{InstrumentStock, InstrumentETF} {
		t.Run(string(kind), func(t *testing.T) {
			_, i := reviewFixture(t)
			i.Type = kind
			points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "100"), reviewQuote(t, i, "2026-10-09", "105")}
			for n := range points {
				points[n].PriceBasis = YahooRawClosePriceBasis
				points[n].SourceKind = QuoteSourceProvider
				points[n].SourceKey = YahooFinanceProviderKey
				points[n].TimestampBasis = "session_close"
			}
			got := ProjectQuotePerformance(i, points, nil)
			if len(got) != 1 || got[0].PeriodPercent != "5" || got[0].ElapsedDays != 8 || got[0].BaselineOn != "2026-10-01" || got[0].LatestOn != "2026-10-09" {
				t.Fatalf("Yahoo performance %+v", got)
			}
			points[1].SplitFactor = "2"
			if got := ProjectQuotePerformance(i, points, nil); len(got) != 0 {
				t.Fatalf("split %+v", got)
			}
		})
	}
}
func TestDescriptiveFlatOrdinaryFundNAV(t *testing.T) {
	_, i := reviewFixture(t)
	points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "1.2345"), reviewQuote(t, i, "2026-10-10", "1.2345")}
	got := ProjectQuotePerformance(i, points, nil)
	if len(got) != 1 || got[0].PeriodPercent != "0" || got[0].ElapsedDays != 9 || got[0].BaselineOn != "2026-10-01" || got[0].LatestOn != "2026-10-10" || got[0].AnnualizedPercent != nil {
		t.Fatalf("flat NAV %+v", got)
	}
}
func TestDescriptiveLatestEndpointAndZeroBaseline(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentStock
	points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "100"), reviewQuote(t, i, "2026-10-09", "110"), reviewQuote(t, i, "2026-10-10", "0")}
	for n := range points {
		points[n].PriceBasis = TiingoRawClosePriceBasis
	}
	got := ProjectQuotePerformance(i, points, nil)
	if len(got) != 1 || got[0].PeriodPercent != "-100" || got[0].LatestOn != "2026-10-10" || got[0].BaselineOn != "2026-10-01" || got[0].ElapsedDays != 9 {
		t.Fatalf("zero endpoint %+v", got)
	}
	points[0].UnitPrice = points[2].UnitPrice
	if got := ProjectQuotePerformance(i, points, nil); len(got) != 0 {
		t.Fatalf("zero divisor %+v", got)
	}
	points[0] = reviewQuote(t, i, "2026-10-01", "100")
	points[0].PriceBasis = TiingoRawClosePriceBasis
	points[2].PriceBasis = "unsupported"
	if got := ProjectQuotePerformance(i, points, nil); len(got) != 0 {
		t.Fatalf("unsupported latest fell back %+v", got)
	}
	i.Type = InstrumentMutualFund
	for n := range points {
		points[n].PriceBasis = "agent_unit_nav_v1"
	}
	if got := ProjectQuotePerformance(i, points, nil); len(got) != 0 {
		t.Fatalf("zero NAV latest fell back %+v", got)
	}
}

func TestDescriptiveAgentEquityPerformance(t *testing.T) {
	for _, kind := range []InstrumentType{InstrumentStock, InstrumentETF} {
		t.Run(string(kind), func(t *testing.T) {
			_, i := reviewFixture(t)
			i.Type = kind
			i.ProviderBindingRevision = 3
			points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "100"), reviewQuote(t, i, "2026-10-09", "105")}
			for n := range points {
				points[n].PriceBasis = "agent_raw_close_v1"
				points[n].BindingRevision = i.ProviderBindingRevision
				points[n].SplitFactor = "1"
			}
			got := ProjectQuotePerformance(i, points, nil)
			if len(got) != 1 || got[0].WindowDays != 7 || got[0].PeriodPercent != "5" || got[0].ElapsedDays != 8 || got[0].BaselineOn != "2026-10-01" || got[0].LatestOn != "2026-10-09" || got[0].AnnualizedPercent != nil {
				t.Fatalf("Agent performance %+v", got)
			}
			for _, mismatch := range []string{"unknown_source", "price_basis", "timestamp_basis", "binding", "currency", "date", "split", "zero_baseline", "unsafe"} {
				t.Run(mismatch, func(t *testing.T) {
					changed := append([]InstrumentQuote(nil), points...)
					var unsafe []string
					switch mismatch {
					case "unknown_source":
						changed[0].SourceKind = "unknown"
					case "price_basis":
						changed[0].PriceBasis = "adjusted_close"
					case "timestamp_basis":
						changed[0].TimestampBasis = "unknown"
					case "binding":
						changed[0].BindingRevision++
					case "currency":
						changed[0].Currency = "CNY"
					case "date":
						changed[0].EffectiveDate = "invalid"
					case "split":
						changed[1].SplitFactor = "2"
					case "zero_baseline":
						changed[0].UnitPrice, _ = ParseUnitPrice("0")
					case "unsafe":
						unsafe = []string{"2026-10-04"}
					}
					if got := ProjectQuotePerformance(i, changed, unsafe); len(got) != 0 {
						t.Fatalf("incomparable Agent performance %+v", got)
					}
				})
			}
			points[1].UnitPrice, _ = ParseUnitPrice("0")
			got = ProjectQuotePerformance(i, points, nil)
			if len(got) != 1 || got[0].PeriodPercent != "-100" || got[0].ElapsedDays != 8 || got[0].LatestOn != "2026-10-09" {
				t.Fatalf("Agent zero endpoint %+v", got)
			}
		})
	}
}

func TestDescriptiveDailySourcesAndEncodings(t *testing.T) {
	for _, kind := range []InstrumentType{InstrumentStock, InstrumentETF, InstrumentMutualFund, InstrumentBankInvestmentProduct} {
		for _, source := range []QuoteSourceKind{QuoteSourceManual, QuoteSourceProvider, QuoteSourceAgent} {
			t.Run(string(kind)+"/"+string(source), func(t *testing.T) {
				_, i := reviewFixture(t)
				i.Type = kind
				i.QuoteSource = QuoteSourceManual // preference must not constrain evidence
				points := []InstrumentQuote{reviewQuote(t, i, "2026-10-05", "1"), reviewQuote(t, i, "2026-10-14", "1.009")}
				for n := range points {
					points[n].SourceKind = source
					points[n].SourceKey = "synthetic"
				}
				if kind == InstrumentStock || kind == InstrumentETF {
					points[0].PriceBasis = TiingoRawClosePriceBasis
					points[1].PriceBasis = YahooRawClosePriceBasis
				}
				points[1].TimestampBasis = "source_timestamp"
				check := func(points []InstrumentQuote) {
					t.Helper()
					got := ProjectQuotePerformanceReport(i, points, nil)
					if len(got.Items) != 1 || got.UnavailableReason != "" || got.Items[0].PeriodPercent != "0.9" || got.Items[0].ElapsedDays != 9 {
						t.Fatalf("daily evidence: %+v", got)
					}
				}
				check(points)
				points[0].SourceKind = QuoteSourceManual
				points[1].SourceKind = QuoteSourceAgent
				points[1].SourceKey = "different source"
				check(points)
			})
		}
	}
}

func TestDescriptiveActualDaysAndIntradayExclusion(t *testing.T) {
	for _, tc := range []struct {
		base   string
		days   int
		annual string
	}{{"2026-10-07", 7, "46.928571428571"}, {"2026-10-06", 8, "41.0625"}, {"2026-10-05", 9, "36.5"}} {
		t.Run(tc.base, func(t *testing.T) {
			_, i := reviewFixture(t)
			i.Type = InstrumentBankInvestmentProduct
			points := []InstrumentQuote{reviewQuote(t, i, "2026-09-24", "1"), reviewQuote(t, i, tc.base, "1"), reviewQuote(t, i, "2026-10-14", "1.009")}
			intraday := reviewQuote(t, i, "2026-10-15", "99")
			intraday.ObservationKind = "realtime"
			intraday.PriceBasis = "agent_latest_v1"
			points = append(points, intraday)
			got := ProjectQuotePerformanceReport(i, points, nil)
			if len(got.Items) != 1 || got.Items[0].WindowDays != 7 || got.Items[0].BaselineOn != tc.base || got.Items[0].LatestOn != "2026-10-14" || got.Items[0].ElapsedDays != tc.days || *got.Items[0].AnnualizedPercent != tc.annual {
				t.Fatalf("window %+v", got)
			}
		})
	}
}

func TestDescriptiveSameDaySelectionAndUnavailableReasons(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentBankInvestmentProduct
	base := reviewQuote(t, i, "2026-10-05", "1")
	older := reviewQuote(t, i, "2026-10-14", "1.1")
	older.SourceKind = QuoteSourceProvider
	latest := reviewQuote(t, i, "2026-10-14", "1")
	latest.Revision = 2
	previous := latest
	previous.Revision = 1
	previous.UnitPrice = older.UnitPrice
	for _, points := range [][]InstrumentQuote{{base, older, latest, previous}, {previous, latest, older, base}} {
		got := ProjectQuotePerformanceReport(i, points, nil)
		if len(got.Items) != 1 || got.Items[0].PeriodPercent != "0" || got.Items[0].AnnualizedPercent == nil || *got.Items[0].AnnualizedPercent != "0" {
			t.Fatalf("zero/revisions %+v", got)
		}
	}
	if got := ProjectQuotePerformanceReport(i, []InstrumentQuote{base}, nil); got.UnavailableReason != "insufficient_daily_history" {
		t.Fatal(got)
	}
	latest.PriceBasis = "cumulative_nav"
	if got := ProjectQuotePerformanceReport(i, []InstrumentQuote{base, latest}, nil); got.UnavailableReason != "incomparable_daily_prices" || len(got.Items) != 0 {
		t.Fatal(got)
	}
	i.Type = InstrumentType("unsupported")
	if got := ProjectQuotePerformanceReport(i, nil, nil); got.UnavailableReason != "unsupported_instrument" {
		t.Fatal(got)
	}
}

func TestDescriptiveNAVNeutralSplitMarker(t *testing.T) {
	_, i := reviewFixture(t)
	points := []InstrumentQuote{reviewQuote(t, i, "2026-10-05", "1"), reviewQuote(t, i, "2026-10-14", "1.01")}
	points[0].SourceKind = QuoteSourceProvider
	points[0].TimestampBasis = "session_close"
	points[0].SplitFactor = "1"
	if got := ProjectQuotePerformance(i, points, nil); len(got) != 1 {
		t.Fatalf("neutral factor: %+v", got)
	}
	points[0].SplitFactor = "2"
	if got := ProjectQuotePerformance(i, points, nil); len(got) != 0 {
		t.Fatalf("unit change: %+v", got)
	}
}

func TestDescriptiveUnavailableLongWindowKeepsShortWindow(t *testing.T) {
	_, i := reviewFixture(t)
	points := []InstrumentQuote{reviewQuote(t, i, "2026-09-01", "1"), reviewQuote(t, i, "2026-10-05", "1"), reviewQuote(t, i, "2026-10-14", "1.009")}
	report := ProjectQuotePerformanceReport(i, points, []string{"2026-09-15"})
	if len(report.Items) != 1 || report.Items[0].WindowDays != 7 || report.UnavailableReason != "" {
		t.Fatalf("independent windows: %+v", report)
	}
}
