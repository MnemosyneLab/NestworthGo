package domain

import (
	"testing"
	"time"
)

func reviewString(value string) *string { return &value }

func TestReviewDailyEntirePredicateUsesSameDate(t *testing.T) {
	spec := syntheticReviewSpec()
	bank, cny := InstrumentBankInvestmentProduct, CurrencyCode("CNY")
	input := ReviewDailyInput{
		On: "2026-10-10", InstrumentID: NewInstrumentID(), Quantity: reviewString("1"), InstrumentType: &bank,
		NativeCurrency: &cny, AmountExact: reviewString("900"),
		Performance: map[int]QuotePerformance{30: {WindowDays: 30, LatestOn: "2026-10-10", BaselineOn: "2026-09-10", ElapsedDays: 30, PeriodPercent: "0.01", AnnualizedPercent: reviewString("0.121666666667")}},
	}
	result, err := EvaluateReviewDaily(spec, input)
	if err != nil || result.Truth != ReviewFalse {
		t.Fatalf("older low yield must not compensate for today's small amount: %+v %v", result, err)
	}
	input.AmountExact = reviewString("1000.004")
	result, err = EvaluateReviewDaily(spec, input)
	if err != nil || result.Truth != ReviewTrue {
		t.Fatalf("M02 canonical four-place amount: %+v %v", result, err)
	}
	found := false
	for _, leaf := range result.Leaves {
		if leaf.Metric == "amount" {
			found = true
			if leaf.Actual != "1000.0040" {
				t.Fatalf("comparison evidence: %+v", leaf)
			}
		}
	}
	if !found {
		t.Fatal("amount evidence missing")
	}
	input.AmountExact = nil
	result, err = EvaluateReviewDaily(spec, input)
	if err != nil || result.Truth != ReviewUnknown {
		t.Fatalf("missing historical amount must stay unknown: %+v %v", result, err)
	}
	input.Quantity = reviewString("0")
	result, err = EvaluateReviewDaily(spec, input)
	if err != nil || result.Truth != ReviewFalse {
		t.Fatalf("historical zero quantity is a hard gate: %+v %v", result, err)
	}
	input.Quantity = nil
	result, err = EvaluateReviewDaily(spec, input)
	if err != nil || result.Truth != ReviewUnknown {
		t.Fatalf("current holding cannot fill unknown historical quantity: %+v %v", result, err)
	}
}

func TestReviewFXEconomicDateBranches(t *testing.T) {
	policy := DefaultReviewRuleDataPolicy()
	rate, _ := ParseFxRate("7")
	q := FXQuote{ID: NewFXQuoteID(), BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate, ObservationKind: "daily_reference", EffectiveDate: "2026-10-10"}
	cutoff, err := ReviewCivilCutoff("2026-10-10", "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	q.FetchedAt = cutoff.Add(8 * time.Hour)
	selected := ReviewSelectedFX{Quote: q, EconomicKind: "daily_reference"}
	got, err := AdmitReviewFX("USD", "CNY", &selected, "2026-10-10", "Asia/Shanghai", policy)
	if err != nil || !got.Known || got.Rate != "7" || got.EffectiveDate != "2026-10-10" {
		t.Fatalf("M09 late D reference: %+v %v", got, err)
	}
	selected.EconomicKind = "timestamped"
	q.ValueEffectiveAt = cutoff
	selected.Quote = q
	got, err = AdmitReviewFX("USD", "CNY", &selected, "2026-10-10", "Asia/Shanghai", policy)
	if err != nil || got.Known {
		t.Fatalf("M10 cutoff instant belongs to next day: %+v %v", got, err)
	}
	q.ValueEffectiveAt = cutoff.Add(-11 * time.Hour)
	q.FetchedAt = cutoff.Add(72 * time.Hour)
	selected.Quote = q
	got, err = AdmitReviewFX("USD", "CNY", &selected, "2026-10-10", "Asia/Shanghai", policy)
	if err != nil || !got.Known || !got.EffectiveAt.Equal(q.ValueEffectiveAt) {
		t.Fatalf("M10 late historical timestamp: %+v %v", got, err)
	}
	q.ValueEffectiveAt = time.Time{}
	selected.Quote = q
	got, err = AdmitReviewFX("USD", "CNY", &selected, "2026-10-10", "Asia/Shanghai", policy)
	if err != nil || got.Known {
		t.Fatalf("fetchedAt cannot replace economic timestamp: %+v %v", got, err)
	}
	identity, err := AdmitReviewFX("CNY", "CNY", nil, "2026-10-10", "Asia/Shanghai", policy)
	if err != nil || !identity.Known || identity.Rate != "1" {
		t.Fatalf("same-currency conversion: %+v %v", identity, err)
	}
}

func TestReviewFXReferenceAgesAndReciprocal(t *testing.T) {
	rate, _ := ParseFxRate("2")
	selected := ReviewSelectedFX{EconomicKind: "daily_reference", Quote: FXQuote{ID: NewFXQuoteID(), Rate: rate, BaseCurrency: "CNY", QuoteCurrency: "USD", EffectiveDate: "2026-10-03", ObservationKind: "close"}}
	got, err := AdmitReviewFX("USD", "CNY", &selected, "2026-10-10", "Etc/UTC", DefaultReviewRuleDataPolicy())
	if err != nil || !got.Known || got.Rate != "0.5" || got.Direction != FXMappingReciprocal || got.AgeDays != 7 {
		t.Fatalf("reciprocal day 7: %+v %v", got, err)
	}
	selected.Quote.EffectiveDate = "2026-10-02"
	got, err = AdmitReviewFX("USD", "CNY", &selected, "2026-10-10", "Etc/UTC", DefaultReviewRuleDataPolicy())
	if err != nil || got.Known || got.Reason != "fx_stale" {
		t.Fatalf("day 8: %+v %v", got, err)
	}
}

func TestReviewDailyPerformanceUsesClosedEndpointAndBoundedBaseline(t *testing.T) {
	_, instrument := reviewFixture(t)
	instrument.Type = InstrumentBankInvestmentProduct
	quotes := []InstrumentQuote{reviewQuote(t, instrument, "2026-09-10", "1"), reviewQuote(t, instrument, "2026-10-10", "1.001")}
	got, reason := ResolveReviewDailyPerformance(instrument, quotes, "2026-10-10", 30, 7)
	if reason != "" || got == nil || got.LatestOn != "2026-10-10" {
		t.Fatalf("daily endpoint: %+v %s", got, reason)
	}
	got, reason = ResolveReviewDailyPerformance(instrument, quotes, "2026-10-11", 30, 7)
	if got == nil || reason != "" || got.LatestOn != "2026-10-10" || got.AsOfOn != "2026-10-11" || got.ElapsedDays != 31 {
		t.Fatalf("closed endpoint must preserve source date: %+v %s", got, reason)
	}
	quotes[0] = reviewQuote(t, instrument, "2026-09-02", "1")
	got, reason = ResolveReviewDailyPerformance(instrument, quotes, "2026-10-10", 30, 7)
	if got != nil || reason != "baseline_too_old" {
		t.Fatalf("baseline gap eight days: %+v %s", got, reason)
	}
}

func TestReviewDailyCloseCarriesOnlyValidClosedDays(t *testing.T) {
	_, instrument := reviewFixture(t)
	instrument.Type = InstrumentStock
	quote := reviewQuote(t, instrument, "2026-10-10", "0")
	quote.PriceBasis = TiingoRawClosePriceBasis
	got, reason := ResolveReviewDailyClose(instrument, []InstrumentQuote{quote}, "2026-10-10")
	if reason != "" || got == nil || !got.UnitPrice.IsZero() {
		t.Fatalf("a valid zero price is still a positive holding's daily fact: %+v %s", got, reason)
	}
	got, reason = ResolveReviewDailyClose(instrument, []InstrumentQuote{quote}, "2026-10-11")
	if got == nil || reason != "" || got.EffectiveDate != "2026-10-10" {
		t.Fatalf("closed carry must preserve source fact: %+v %s", got, reason)
	}
	got, reason = ResolveReviewDailyClose(instrument, []InstrumentQuote{quote}, "2026-10-12")
	if got != nil || reason != "daily_close_missing" {
		t.Fatalf("missing trading day admitted: %+v %s", got, reason)
	}
	quote.ObservationKind = "realtime"
	got, reason = ResolveReviewDailyClose(instrument, []InstrumentQuote{quote}, "2026-10-10")
	if got != nil || reason != "daily_close_missing" {
		t.Fatalf("realtime price became a daily close: %+v %s", got, reason)
	}
}

func TestReviewDailyQuoteSeriesMatchesSameDateSelectionAndExactEndpoint(t *testing.T) {
	_, instrument := reviewFixture(t)
	instrument.Type = InstrumentStock
	makeClose := func(on, price string, source QuoteSourceKind) InstrumentQuote {
		quote := reviewQuote(t, instrument, on, price)
		quote.SourceKind = source
		quote.PriceBasis = "agent_raw_close_v1"
		return quote
	}
	base := makeClose("2026-09-10", "10", QuoteSourceAgent)
	provider := makeClose("2026-10-10", "100", QuoteSourceProvider)
	agent := makeClose("2026-10-10", "11", QuoteSourceAgent)
	olderAgent := agent
	olderAgent.ID = NewInstrumentQuoteID()
	olderAgent.Revision = -1
	olderAgent.UnitPrice, _ = ParseUnitPrice("12")
	foreign := makeClose("2026-10-10", "999", QuoteSourceAgent)
	foreign.InstrumentID = NewInstrumentID()
	future := makeClose("2026-10-11", "13", QuoteSourceAgent)
	quotes := []InstrumentQuote{future, provider, base, foreign, olderAgent, agent}
	series := NewReviewDailyQuoteSeries(instrument.ID, quotes)

	gotClose, reason := series.ResolveDailyClose(instrument, "2026-10-10")
	if reason != "" || gotClose == nil || gotClose.ID != agent.ID {
		t.Fatalf("daily source rank/revision selection: %+v %s", gotClose, reason)
	}
	gotPerformance, reason := series.ResolveDailyPerformance(instrument, "2026-10-10", 30, 7)
	if reason != "" || gotPerformance == nil || gotPerformance.BaselineOn != "2026-09-10" || gotPerformance.PeriodPercent != "10" {
		t.Fatalf("daily indexed performance: %+v %s", gotPerformance, reason)
	}
	if got, reason := series.ResolveDailyClose(instrument, "2026-10-11"); reason != "" || got == nil || got.ID != future.ID {
		t.Fatalf("future quote was lost from the immutable series: %+v %s", got, reason)
	}
	if got, reason := series.ResolveDailyPerformance(instrument, "2026-10-12", 30, 7); got != nil || reason != "daily_close_missing" {
		t.Fatalf("future data was carried forward: %+v %s", got, reason)
	}
}

func TestReviewDailyEvidenceDoesNotBorrowOtherInstrumentQuotes(t *testing.T) {
	_, target := reviewFixture(t)
	target.Type, target.QuoteCurrency = InstrumentStock, "CNY"
	other := target
	other.ID = NewInstrumentID()
	stockQuote := func(i Instrument, on, price string) InstrumentQuote {
		q := reviewQuote(t, i, on, price)
		q.PriceBasis = "agent_raw_close_v1"
		return q
	}
	for _, currency := range []CurrencyCode{"CNY", "USD"} {
		t.Run(string(currency), func(t *testing.T) {
			other.QuoteCurrency = currency
			base := stockQuote(other, "2026-09-10", "1000")
			end := stockQuote(other, "2026-10-10", "2000")
			for _, quotes := range [][]InstrumentQuote{{base, end}, {end, base}} {
				if got, reason := ResolveReviewDailyClose(target, quotes, "2026-10-10"); got != nil || reason != "daily_close_missing" {
					t.Fatalf("borrowed other close with %s, %v: %+v %s", currency, quotes, got, reason)
				}
				if got, reason := ResolveReviewDailyPerformance(target, quotes, "2026-10-10", 30, 7); got != nil || reason != "daily_close_missing" {
					t.Fatalf("borrowed other 100%% change with %s, %v: %+v %s", currency, quotes, got, reason)
				}
			}
			// A real target close must win regardless of the unrelated close's position.
			own := stockQuote(target, "2026-10-10", "10")
			for _, quotes := range [][]InstrumentQuote{{end, own}, {own, end}} {
				if got, reason := ResolveReviewDailyClose(target, quotes, "2026-10-10"); got == nil || reason != "" || got.InstrumentID != target.ID || got.UnitPrice.Canonical() != "10" {
					t.Fatalf("other close displaced target with %s: %+v %s", currency, got, reason)
				}
			}
		})
	}
}

func TestReviewCivilDateBoundaries(t *testing.T) {
	day, err := ReviewLatestClosedDay(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC), "Etc/UTC")
	if err != nil || day != "2026-10-10" {
		t.Fatalf("closed day: %s %v", day, err)
	}
	a, err := ReviewCivilCutoff("2026-03-08", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ReviewCivilCutoff("2026-03-07", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if a.Sub(b) != 23*time.Hour {
		t.Fatalf("spring day was %s", a.Sub(b))
	}
	a, err = ReviewCivilCutoff("2026-11-01", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	b, err = ReviewCivilCutoff("2026-10-31", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if a.Sub(b) != 25*time.Hour {
		t.Fatalf("fall day was %s", a.Sub(b))
	}
	if _, err := ReviewCivilCutoff("2011-12-30", "Pacific/Apia"); err == nil {
		t.Fatal("skipped date was normalized instead of rejected")
	}
}
