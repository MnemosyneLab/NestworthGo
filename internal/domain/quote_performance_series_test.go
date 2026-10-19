package domain

import (
	"testing"
)

func seriesDates(points []QuotePerformancePoint) []string {
	dates := make([]string, 0, len(points))
	for _, point := range points {
		dates = append(dates, point.EffectiveDate)
	}
	return dates
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for n := range a {
		if a[n] != b[n] {
			return false
		}
	}
	return true
}

// Intraday and realtime facts never enter the chart, even when they are newer
// than the close of the same date, while Agent, manual and provider closes do.
func TestPerformanceSeriesKeepsOnlyDailyCloses(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentStock
	daily := func(date, price string, source QuoteSourceKind) InstrumentQuote {
		q := reviewQuote(t, i, date, price)
		q.SourceKind, q.PriceBasis = source, "agent_raw_close_v1"
		return q
	}
	realtime := reviewQuote(t, i, "2026-10-14", "999")
	realtime.ObservationKind, realtime.PriceBasis = "realtime", "agent_latest_v1"
	realtime.QuotedAt = realtime.QuotedAt.Add(20 * 3600e9)
	manual := reviewQuote(t, i, "2026-10-13", "888")
	manual.ObservationKind = "manual"
	quotes := []InstrumentQuote{
		daily("2026-10-10", "100", QuoteSourceProvider), daily("2026-10-12", "101", QuoteSourceManual),
		daily("2026-10-13", "102", QuoteSourceAgent), daily("2026-10-14", "103", QuoteSourceProvider),
		realtime, manual,
	}
	got := ProjectQuotePerformanceSeries(i, quotes, 7)
	if !equalStrings(seriesDates(got.Points), []string{"2026-10-10", "2026-10-12", "2026-10-13", "2026-10-14"}) {
		t.Fatalf("dates %v", seriesDates(got.Points))
	}
	for _, point := range got.Points {
		if point.ObservationKind != "close" || point.Value == "999" || point.Value == "888" {
			t.Fatalf("a non-daily observation reached the chart: %+v", point)
		}
	}
	wantSources := []QuoteSourceKind{QuoteSourceProvider, QuoteSourceManual, QuoteSourceAgent, QuoteSourceProvider}
	for n, point := range got.Points {
		if point.SourceKind != wantSources[n] {
			t.Fatalf("point %d source %s, want %s", n, point.SourceKind, wantSources[n])
		}
	}
}

// Several eligible sources for one civil date collapse with the figures' own
// source rank, so the chart and the headline read the same value.
func TestPerformanceSeriesDailySelectionMatchesFigures(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentStock
	mk := func(date, price string, source QuoteSourceKind) InstrumentQuote {
		q := reviewQuote(t, i, date, price)
		q.SourceKind, q.PriceBasis = source, "agent_raw_close_v1"
		return q
	}
	quotes := []InstrumentQuote{mk("2026-10-07", "100", QuoteSourceProvider), mk("2026-10-14", "150", QuoteSourceProvider), mk("2026-10-14", "110", QuoteSourceAgent)}
	got := ProjectQuotePerformanceSeries(i, quotes, 7)
	if len(got.Points) != 2 || got.Points[1].Value != "110" || got.Item == nil || got.Item.PeriodPercent != "10" {
		t.Fatalf("series %+v", got)
	}
}

// With sparse or delayed data the chart spans exactly the headline's dates.
func TestPerformanceSeriesAlignsWithHeadlineDates(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentStock
	var quotes []InstrumentQuote
	for _, p := range []struct{ on, price string }{{"2026-07-01", "50"}, {"2026-09-02", "80"}, {"2026-10-02", "90"}, {"2026-10-05", "92"}, {"2026-10-08", "100"}} {
		q := reviewQuote(t, i, p.on, p.price)
		q.PriceBasis = "agent_raw_close_v1"
		quotes = append(quotes, q)
	}
	for _, window := range []int{7, 0} {
		got := ProjectQuotePerformanceSeries(i, quotes, window)
		if got.Item == nil {
			t.Fatalf("window %d unavailable: %+v", window, got)
		}
		dates := seriesDates(got.Points)
		if got.From != got.Item.BaselineOn || got.To != got.Item.LatestOn || dates[0] != got.Item.BaselineOn || dates[len(dates)-1] != got.Item.LatestOn {
			t.Errorf("window %d chart %v (%s..%s) differs from headline %s..%s", window, dates, got.From, got.To, got.Item.BaselineOn, got.Item.LatestOn)
		}
	}
	// 7 days back from the delayed latest close (Oct 8) is Oct 1; the nearest
	// earlier baseline is Sep 2 even though it is a month older.
	got := ProjectQuotePerformanceSeries(i, quotes, 7)
	if got.From != "2026-09-02" || got.To != "2026-10-08" {
		t.Fatalf("delayed window %s..%s", got.From, got.To)
	}
}

// A period without history reaching its target has no headline, but still
// charts every daily observation inside it.
func TestPerformanceSeriesWithoutBaselineHasNoHeadline(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentStock
	var quotes []InstrumentQuote
	for _, p := range []struct{ on, price string }{{"2026-10-05", "90"}, {"2026-10-14", "100"}} {
		q := reviewQuote(t, i, p.on, p.price)
		q.PriceBasis = "agent_raw_close_v1"
		quotes = append(quotes, q)
	}
	got := ProjectQuotePerformanceSeries(i, quotes, 90)
	if got.Item != nil || got.UnavailableReason != "insufficient_daily_history" || len(got.Points) != 2 || got.From != "2026-10-05" || got.To != "2026-10-14" {
		t.Fatalf("series %+v", got)
	}
	if single := ProjectQuotePerformanceSeries(i, quotes[:1], 7); single.Item != nil || single.UnavailableReason != "insufficient_daily_history" || len(single.Points) != 1 {
		t.Fatalf("single point %+v", single)
	}
	if empty := ProjectQuotePerformanceSeries(i, nil, 7); len(empty.Points) != 0 || empty.UnavailableReason != "insufficient_daily_history" || empty.From != "" {
		t.Fatalf("empty %+v", empty)
	}
	i.Type = InstrumentType("unsupported")
	if unsupported := ProjectQuotePerformanceSeries(i, quotes, 7); unsupported.UnavailableReason != "unsupported_instrument" {
		t.Fatalf("unsupported %+v", unsupported)
	}
}

// A genuine price-basis change is a discontinuity the chart must not bridge and
// that makes the headline unavailable; equivalent raw-close providers are not.
func TestPerformanceSeriesPreservesPriceBasisDiscontinuities(t *testing.T) {
	_, i := reviewFixture(t)
	i.Type = InstrumentStock
	mk := func(date, price, basis string) InstrumentQuote {
		q := reviewQuote(t, i, date, price)
		q.PriceBasis = basis
		return q
	}
	quotes := []InstrumentQuote{mk("2026-10-01", "99", "agent_raw_close_v1"), mk("2026-10-08", "100", "agent_raw_close_v1"), mk("2026-10-09", "101", TiingoRawClosePriceBasis), mk("2026-10-12", "50", "adjusted_close"), mk("2026-10-13", "51", "adjusted_close"), mk("2026-10-14", "52", "agent_raw_close_v1")}
	got := ProjectQuotePerformanceSeries(i, quotes, 7)
	breaks := []bool{}
	for _, point := range got.Points {
		breaks = append(breaks, point.BreakBefore)
	}
	want := []bool{false, false, false, true, false, true}
	for n := range want {
		if breaks[n] != want[n] {
			t.Fatalf("break flags %v, want %v", breaks, want)
		}
	}
	if got.Item != nil || got.UnavailableReason != "incomparable_daily_prices" || len(got.Points) != 6 {
		t.Fatalf("series %+v", got)
	}
	// NAV never treats two different bases as one unit.
	_, fund := reviewFixture(t)
	a, b := reviewQuote(t, fund, "2026-10-08", "1"), reviewQuote(t, fund, "2026-10-14", "1.01")
	b.PriceBasis = "cumulative_nav"
	if nav := ProjectQuotePerformanceSeries(fund, []InstrumentQuote{a, b}, 7); !nav.Points[1].BreakBefore {
		t.Fatalf("NAV basis change not marked: %+v", nav.Points)
	}
}

// The returns section lists the fixed windows only; all history stays a chart period.
func TestQuoteReturnWindowsExcludeAllHistory(t *testing.T) {
	if len(QuoteReturnWindows) != 5 || QuoteReturnWindows[0] != 7 || QuoteReturnWindows[4] != 365 {
		t.Fatalf("return windows %v", QuoteReturnWindows)
	}
	for _, window := range QuoteReturnWindows {
		if window <= 0 || !ValidQuotePeriodWindow(window) {
			t.Fatalf("invalid return window %d", window)
		}
	}
	if !ValidQuotePeriodWindow(0) || ValidQuotePeriodWindow(45) || ValidQuotePeriodWindow(-1) {
		t.Fatal("period window validation")
	}
	_, i := reviewFixture(t)
	i.Type = InstrumentBankInvestmentProduct
	quotes := []InstrumentQuote{reviewQuote(t, i, "2025-09-01", "1"), reviewQuote(t, i, "2026-04-01", "1.02"), reviewQuote(t, i, "2026-07-01", "1.03"), reviewQuote(t, i, "2026-09-10", "1.04"), reviewQuote(t, i, "2026-10-14", "1.05")}
	items := ProjectQuotePerformanceWindows(i, quotes, QuoteReturnWindows)
	if len(items) != 5 {
		t.Fatalf("items %+v", items)
	}
	for _, item := range items {
		if item.WindowDays == 0 || item.AnnualizedPercent == nil {
			t.Fatalf("return %+v", item)
		}
	}
}
