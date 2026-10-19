package domain

import (
	"testing"
)

func TestDescriptivePerformanceDatesAndAlertIndependence(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentBankInvestmentProduct
	points := []InstrumentQuote{reviewQuote(t, i, "2026-09-30", "1"), reviewQuote(t, i, "2026-10-09", "1.009")}
	got := ProjectQuotePerformance(i, points)
	if len(got) == 0 || got[0].ElapsedDays != 9 || got[0].BaselineOn != "2026-09-30" || got[0].LatestOn != "2026-10-09" || got[0].PeriodPercent != "0.9" || got[0].AnnualizedPercent == nil {
		t.Fatalf("performance %+v", got)
	}
	points = []InstrumentQuote{reviewQuote(t, i, "2026-09-24", "1"), reviewQuote(t, i, "2026-10-05", "1.01"), reviewQuote(t, i, "2026-10-14", "1.02")}
	got = ProjectQuotePerformance(i, points)
	if len(got) != 1 || got[0].WindowDays != 7 || got[0].ElapsedDays != 9 {
		t.Fatalf("20-day history %+v", got)
	}
}
func TestDescriptivePerformanceRejectsIncomparableEvidence(t *testing.T) {
	_, i := reviewFixture(t)
	for _, kind := range []string{"income_distribution", "currency", "binding", "distribution", "whole_value", "single"} {
		t.Run(kind, func(t *testing.T) {
			points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "1"), reviewQuote(t, i, "2026-10-09", "1.02")}
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
			case "single":
				points = points[:1]
			}
			if got := ProjectQuotePerformance(i, points); len(got) != 0 {
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
	got := ProjectQuotePerformance(i, points)
	if len(got) != 1 || got[0].PeriodPercent != "0" || got[0].AnnualizedPercent != nil {
		t.Fatalf("flat stock %+v", got)
	}
	points[1].SplitFactor = "2"
	if got := ProjectQuotePerformance(i, points); len(got) != 0 {
		t.Fatalf("split %+v", got)
	}
}

func TestDescriptiveYahooEquityPerformance(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentStock
	points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "100"), reviewQuote(t, i, "2026-10-09", "105")}
	for n := range points {
		points[n].PriceBasis = YahooRawClosePriceBasis
		points[n].SourceKind = QuoteSourceProvider
		points[n].SourceKey = YahooFinanceProviderKey
		points[n].TimestampBasis = "session_close"
	}
	got := ProjectQuotePerformance(i, points)
	if len(got) != 1 || got[0].PeriodPercent != "5" || got[0].ElapsedDays != 8 || got[0].BaselineOn != "2026-10-01" || got[0].LatestOn != "2026-10-09" {
		t.Fatalf("Yahoo performance %+v", got)
	}
	points[1].SplitFactor = "2"
	if got := ProjectQuotePerformance(i, points); len(got) != 0 {
		t.Fatalf("split %+v", got)
	}
}
func TestDescriptiveFlatOrdinaryFundNAV(t *testing.T) {
	_, i := reviewFixture(t)
	points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "1.2345"), reviewQuote(t, i, "2026-10-10", "1.2345")}
	got := ProjectQuotePerformance(i, points)
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
	got := ProjectQuotePerformance(i, points)
	if len(got) != 1 || got[0].PeriodPercent != "-100" || got[0].LatestOn != "2026-10-10" || got[0].BaselineOn != "2026-10-01" || got[0].ElapsedDays != 9 {
		t.Fatalf("zero endpoint %+v", got)
	}
	points[0].UnitPrice = points[2].UnitPrice
	if got := ProjectQuotePerformance(i, points); len(got) != 0 {
		t.Fatalf("zero divisor %+v", got)
	}
	points[0] = reviewQuote(t, i, "2026-10-01", "100")
	points[0].PriceBasis = TiingoRawClosePriceBasis
	points[2].PriceBasis = "unsupported"
	if got := ProjectQuotePerformance(i, points); len(got) != 0 {
		t.Fatalf("unsupported latest fell back %+v", got)
	}
	i.Type = InstrumentMutualFund
	for n := range points {
		points[n].PriceBasis = "agent_unit_nav_v1"
	}
	if got := ProjectQuotePerformance(i, points); len(got) != 0 {
		t.Fatalf("zero NAV latest fell back %+v", got)
	}
}

func TestDescriptiveAgentEquityPerformance(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentStock
	i.ProviderBindingRevision = 3
	points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "100"), reviewQuote(t, i, "2026-10-09", "105")}
	for n := range points {
		points[n].PriceBasis = "agent_raw_close_v1"
		points[n].BindingRevision = i.ProviderBindingRevision
		points[n].SplitFactor = "1"
	}
	got := ProjectQuotePerformance(i, points)
	if len(got) != 1 || got[0].WindowDays != 7 || got[0].PeriodPercent != "5" || got[0].ElapsedDays != 8 || got[0].BaselineOn != "2026-10-01" || got[0].LatestOn != "2026-10-09" || got[0].AnnualizedPercent != nil {
		t.Fatalf("Agent performance %+v", got)
	}
	for _, mismatch := range []string{"unknown_source", "price_basis", "timestamp_basis", "binding", "currency", "date", "split", "zero_baseline"} {
		t.Run(mismatch, func(t *testing.T) {
			changed := append([]InstrumentQuote(nil), points...)
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
			}
			if got := ProjectQuotePerformance(i, changed); len(got) != 0 {
				t.Fatalf("incomparable Agent performance %+v", got)
			}
		})
	}
	points[1].UnitPrice, _ = ParseUnitPrice("0")
	got = ProjectQuotePerformance(i, points)
	if len(got) != 1 || got[0].PeriodPercent != "-100" || got[0].ElapsedDays != 8 || got[0].LatestOn != "2026-10-09" {
		t.Fatalf("Agent zero endpoint %+v", got)
	}
}

func TestDescriptiveDailySourcesAndEncodings(t *testing.T) {
	for _, tc := range []struct {
		kind   InstrumentType
		source QuoteSourceKind
	}{
		{InstrumentStock, QuoteSourceProvider},
		{InstrumentBankInvestmentProduct, QuoteSourceManual},
	} {
		t.Run(string(tc.kind)+"/"+string(tc.source), func(t *testing.T) {
			kind, source := tc.kind, tc.source
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
				got := ProjectQuotePerformanceReport(i, points)
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
			got := ProjectQuotePerformanceReport(i, points)
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
		got := ProjectQuotePerformanceReport(i, points)
		if len(got.Items) != 1 || got.Items[0].PeriodPercent != "0" || got.Items[0].AnnualizedPercent == nil || *got.Items[0].AnnualizedPercent != "0" {
			t.Fatalf("zero/revisions %+v", got)
		}
	}
	if got := ProjectQuotePerformanceReport(i, []InstrumentQuote{base}); got.UnavailableReason != "insufficient_daily_history" {
		t.Fatal(got)
	}
	latest.PriceBasis = "cumulative_nav"
	if got := ProjectQuotePerformanceReport(i, []InstrumentQuote{base, latest}); got.UnavailableReason != "incomparable_daily_prices" || len(got.Items) != 0 {
		t.Fatal(got)
	}
	i.Type = InstrumentType("unsupported")
	if got := ProjectQuotePerformanceReport(i, nil); got.UnavailableReason != "unsupported_instrument" {
		t.Fatal(got)
	}
}

func TestDescriptiveNAVNeutralSplitMarker(t *testing.T) {
	_, i := reviewFixture(t)
	points := []InstrumentQuote{reviewQuote(t, i, "2026-10-05", "1"), reviewQuote(t, i, "2026-10-14", "1.01")}
	points[0].SourceKind = QuoteSourceProvider
	points[0].TimestampBasis = "session_close"
	points[0].SplitFactor = "1"
	if got := ProjectQuotePerformance(i, points); len(got) != 1 {
		t.Fatalf("neutral factor: %+v", got)
	}
	points[0].SplitFactor = "2"
	if got := ProjectQuotePerformance(i, points); len(got) != 0 {
		t.Fatalf("unit change: %+v", got)
	}
}

func TestDescriptiveUnavailableLongWindowKeepsShortWindow(t *testing.T) {
	_, i := reviewFixture(t)
	points := []InstrumentQuote{reviewQuote(t, i, "2026-09-01", "1"), reviewQuote(t, i, "2026-10-05", "1"), reviewQuote(t, i, "2026-10-14", "1.009")}
	points[0].DividendCash = "0.01"
	report := ProjectQuotePerformanceReport(i, points)
	if len(report.Items) != 1 || report.Items[0].WindowDays != 7 || report.UnavailableReason != "" {
		t.Fatalf("independent windows: %+v", report)
	}
}

// Every type with dated close history must expose descriptive performance;
// only an unknown type is unsupported. The accepted bases are explicit.
func TestDescriptivePerformanceSupportsEveryPricedInstrumentType(t *testing.T) {
	for _, tc := range []struct {
		kind      InstrumentType
		basis     string
		timestamp string
	}{
		{InstrumentStock, TiingoRawClosePriceBasis, "session_close"},
		{InstrumentETF, YahooRawClosePriceBasis, "session_close"},
		{InstrumentBankInvestmentProduct, "agent_unit_nav_v1", "date_label"},
		{InstrumentPreciousMetal, YahooRawClosePriceBasis, "policy_derived"},
		{InstrumentCrypto, CoinGeckoDailyPriceBasis, "observed_publication"},
		{InstrumentCrypto, YahooRawClosePriceBasis, "policy_derived"},
		{InstrumentBond, "agent_raw_close_v1", "date_label"},
	} {
		t.Run(string(tc.kind)+"/"+tc.basis+"/"+tc.timestamp, func(t *testing.T) {
			_, i := reviewFixture(t)
			i.Type = tc.kind
			points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "100"), reviewQuote(t, i, "2026-10-09", "110")}
			for n := range points {
				points[n].PriceBasis, points[n].TimestampBasis = tc.basis, tc.timestamp
				points[n].SourceKind = QuoteSourceProvider
			}
			report := ProjectQuotePerformanceReport(i, points)
			if len(report.Items) != 1 || report.Items[0].PeriodPercent != "10" || report.UnavailableReason != "" {
				t.Fatalf("%s performance %+v", tc.kind, report)
			}
		})
	}
}

func TestDescriptivePerformanceRejectsIncomparableMetalAndCryptoBases(t *testing.T) {
	for _, tc := range []struct {
		kind      InstrumentType
		basis     string
		timestamp string
	}{
		// Policy-derived and publication labels belong only to the types whose adapters emit them.
		{InstrumentStock, YahooRawClosePriceBasis, "policy_derived"},
		{InstrumentPreciousMetal, YahooRawClosePriceBasis, "observed_publication"},
		{InstrumentPreciousMetal, CoinGeckoDailyPriceBasis, "policy_derived"},
		{InstrumentPreciousMetal, TiingoRawClosePriceBasis, "session_close"},
		{InstrumentCrypto, TiingoRawClosePriceBasis, "session_close"},
		{InstrumentCrypto, "adjusted_close", "policy_derived"},
		{InstrumentBond, YahooRawClosePriceBasis, "session_close"},
		{InstrumentOther, "agent_unit_nav_v1", "date_label"},
	} {
		t.Run(string(tc.kind)+"/"+tc.basis+"/"+tc.timestamp, func(t *testing.T) {
			_, i := reviewFixture(t)
			i.Type = tc.kind
			points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "100"), reviewQuote(t, i, "2026-10-09", "110")}
			for n := range points {
				points[n].PriceBasis, points[n].TimestampBasis = tc.basis, tc.timestamp
			}
			if got := ProjectQuotePerformance(i, points); len(got) != 0 {
				t.Fatalf("unexpected comparable %+v", got)
			}
		})
	}
	_, i := reviewFixture(t)
	i.Type = InstrumentPreciousMetal
	points := []InstrumentQuote{reviewQuote(t, i, "2026-10-01", "100"), reviewQuote(t, i, "2026-10-09", "110")}
	for n := range points {
		points[n].PriceBasis, points[n].TimestampBasis = YahooRawClosePriceBasis, "policy_derived"
	}
	points[1].SplitFactor = "2"
	if got := ProjectQuotePerformance(i, points); len(got) != 0 {
		t.Fatalf("unit change %+v", got)
	}
}

// The period selector reuses the descriptive rules: nearest earlier baseline,
// actual civil days, and no fabricated change when a period lacks history.
func TestQuotePeriodWindowsUseNearestEarlierBaselineAndAllHistory(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentPreciousMetal
	var points []InstrumentQuote
	for _, p := range []struct{ on, price string }{{"2026-04-20", "50"}, {"2026-07-15", "80"}, {"2026-09-20", "90"}, {"2026-10-09", "95"}, {"2026-10-14", "100"}} {
		q := reviewQuote(t, i, p.on, p.price)
		q.PriceBasis, q.TimestampBasis = YahooRawClosePriceBasis, "policy_derived"
		points = append(points, q)
	}
	got := ProjectQuotePerformanceWindows(i, points, QuotePeriodWindows)
	byWindow := map[int]QuotePerformance{}
	for _, item := range got {
		byWindow[item.WindowDays] = item
	}
	want := map[int]struct {
		baseline string
		percent  string
	}{
		7:  {"2026-09-20", "11.1111111111111111111111"},
		30: {"2026-07-15", "25"},
		90: {"2026-07-15", "25"},
		0:  {"2026-04-20", "100"},
	}
	for window, expectation := range want {
		item, ok := byWindow[window]
		if !ok || item.BaselineOn != expectation.baseline || item.LatestOn != "2026-10-14" || item.PeriodPercent != expectation.percent {
			t.Errorf("window %d = %+v ok=%v, want baseline %s percent %s", window, item, ok, expectation.baseline, expectation.percent)
		}
	}
	for _, window := range []int{180} {
		if _, ok := byWindow[window]; ok {
			t.Errorf("a %d-day change was invented without an earlier baseline: %+v", window, byWindow[window])
		}
	}
	// Existing 7/30 callers must be unchanged by the added periods.
	if legacy := ProjectQuotePerformance(i, points); len(legacy) != 2 || legacy[0].WindowDays != 7 || legacy[1].WindowDays != 30 {
		t.Errorf("legacy windows %+v", legacy)
	}
}

func TestQuotePeriodAllHistoryIsUnavailableAcrossIncomparableObservation(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentStock
	points := []InstrumentQuote{reviewQuote(t, i, "2026-06-01", "100"), reviewQuote(t, i, "2026-08-01", "50"), reviewQuote(t, i, "2026-10-01", "55"), reviewQuote(t, i, "2026-10-14", "60")}
	for n := range points {
		points[n].PriceBasis, points[n].TimestampBasis = "agent_raw_close_v1", "date_label"
	}
	points[1].SplitFactor = "2"
	byWindow := map[int]bool{}
	for _, item := range ProjectQuotePerformanceWindows(i, points, QuotePeriodWindows) {
		byWindow[item.WindowDays] = true
	}
	if byWindow[0] || byWindow[30] || byWindow[90] || !byWindow[7] {
		t.Fatalf("a split inside a period must make only that period unavailable: %+v", byWindow)
	}
}

func TestHoldingGroupClassification(t *testing.T) {
	for kind, want := range map[InstrumentType]HoldingGroupKey{
		InstrumentStock: HoldingGroupStock, InstrumentETF: HoldingGroupFund,
		InstrumentBond: HoldingGroupBond, InstrumentBankInvestmentProduct: HoldingGroupBankWealth,
		InstrumentPreciousMetal: HoldingGroupMetal, InstrumentCrypto: HoldingGroupCrypto,
		InstrumentType("future_kind"): HoldingGroupOther,
	} {
		if got := HoldingGroupFor(kind, false); got != want {
			t.Errorf("%s grouped as %s, want %s", kind, got, want)
		}
	}
	if got := HoldingGroupFor(InstrumentStock, true); got != HoldingGroupCashLike {
		t.Errorf("managed stock grouped as %s", got)
	}
	seen := map[HoldingGroupKey]bool{}
	for _, key := range HoldingGroupOrder {
		if seen[key] {
			t.Errorf("duplicate group %s", key)
		}
		seen[key] = true
	}
	for _, key := range []HoldingGroupKey{HoldingGroupStock, HoldingGroupFund, HoldingGroupBond, HoldingGroupBankWealth, HoldingGroupMetal, HoldingGroupCrypto, HoldingGroupCashLike, HoldingGroupOther} {
		if !seen[key] {
			t.Errorf("group %s is absent from the display order", key)
		}
	}
}
