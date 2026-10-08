package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAgentCurrentQuoteOverlayUsesObservationTime(t *testing.T) {
	t.Parallel()
	id := domain.NewInstrumentID()
	price := func(value string) domain.UnitPrice {
		parsed, err := domain.ParseUnitPrice(value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	day := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	provider := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "USD", UnitPrice: price("100"), SourceKind: domain.QuoteSourceProvider, QuotedAt: day.Add(12 * time.Hour), CreatedAt: day.Add(12 * time.Hour)}
	agent := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "USD", UnitPrice: price("101"), SourceKind: domain.QuoteSourceAgent, QuotedAt: day.Add(11 * time.Hour), CreatedAt: day.Add(20 * time.Hour)}
	instrument := domain.Instrument{ID: id, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceProvider}
	if got := selectInstrumentQuote(instrument, []domain.InstrumentQuote{provider, agent}); got == nil || got.ID != provider.ID {
		t.Fatalf("newer provider observation should win: %+v", got)
	}
	agent.QuotedAt = provider.QuotedAt
	if got := selectInstrumentQuote(instrument, []domain.InstrumentQuote{provider, agent}); got == nil || got.ID != agent.ID {
		t.Fatalf("Agent should win equal observation time: %+v", got)
	}
	instrument.QuoteSource = domain.QuoteSourceManual
	if got := selectInstrumentQuote(instrument, []domain.InstrumentQuote{provider, agent}); got == nil || got.ID != agent.ID {
		t.Fatalf("Agent should supplement a manual preference: %+v", got)
	}
	instrument.QuoteSource = domain.QuoteSourceAgent
	if got := selectInstrumentQuote(instrument, []domain.InstrumentQuote{provider}); got != nil {
		t.Fatalf("Agent preference admitted provider quote: %+v", got)
	}
}

func TestAgentCurrentDailyCloseWinsSameDateButNewerRealtimeWins(t *testing.T) {
	t.Parallel()
	id := domain.NewInstrumentID()
	price := func(value string) domain.UnitPrice {
		parsed, err := domain.ParseUnitPrice(value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	day := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	providerClose := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "USD", UnitPrice: price("11"), SourceKind: domain.QuoteSourceProvider, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-20", QuotedAt: day.Add(16 * time.Hour)}
	agentClose := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "USD", UnitPrice: price("10"), SourceKind: domain.QuoteSourceAgent, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-20", QuotedAt: day}
	instrument := domain.Instrument{ID: id, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceProvider}
	if got := selectInstrumentQuote(instrument, []domain.InstrumentQuote{providerClose, agentClose}); got == nil || got.ID != agentClose.ID {
		t.Fatalf("Agent close should win same source date: %+v", got)
	}
	providerRealtime := providerClose
	providerRealtime.ID = domain.NewInstrumentQuoteID()
	providerRealtime.ObservationKind = string(InstrumentObservationRealtime)
	providerRealtime.EffectiveDate = ""
	providerRealtime.QuotedAt = day.AddDate(0, 0, 1).Add(time.Hour)
	if got := selectInstrumentQuote(instrument, []domain.InstrumentQuote{providerClose, agentClose, providerRealtime}); got == nil || got.ID != providerRealtime.ID {
		t.Fatalf("newer provider realtime observation should win: %+v", got)
	}
}

func TestAgentDailyQuoteWinsExactDateAndCarriedNAVIsIncomplete(t *testing.T) {
	t.Parallel()
	id := domain.NewInstrumentID()
	providerKey := domain.TiingoProviderKey
	instrument := domain.Instrument{ID: id, Type: domain.InstrumentMutualFund, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceProvider, ProviderKey: &providerKey, ProviderBindingRevision: 2}
	price := func(value string) domain.UnitPrice {
		parsed, err := domain.ParseUnitPrice(value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	day := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	agent := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "USD", UnitPrice: price("10"), SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-20", ValueEffectiveAt: day, QuotedAt: day, PriceBasis: "agent_unit_nav_v1", SourcePolicyVersion: "agent_supplied_v1", TimestampBasis: "date_label"}
	provider := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "USD", UnitPrice: price("11"), SourceKind: domain.QuoteSourceProvider, SourceKey: providerKey, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-20", ValueEffectiveAt: day, QuotedAt: day, BindingRevision: 2, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: string(PriceBasisTiingoRawClose), TimestampBasis: string(TimestampBasisSessionClose)}
	selection := selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(instrument, []domain.InstrumentQuote{provider, agent}, nil, "2026-09-20", day.AddDate(0, 0, 1))
	if selection.quote == nil || selection.quote.ID != agent.ID || !selection.coverageComplete {
		t.Fatalf("exact Agent NAV should win and be complete: %+v", selection)
	}
	selection = selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(instrument, []domain.InstrumentQuote{agent}, nil, "2026-09-21", day.AddDate(0, 0, 2))
	if selection.quote == nil || selection.quote.ID != agent.ID || selection.coverageComplete {
		t.Fatalf("carried Agent NAV should value but remain incomplete: %+v", selection)
	}
	provider.EffectiveDate = "2026-09-21"
	selection = selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(instrument, []domain.InstrumentQuote{provider, agent}, nil, "2026-09-21", day.AddDate(0, 0, 2))
	if selection.quote == nil || selection.quote.ID != provider.ID {
		t.Fatalf("newer provider date should beat carried Agent NAV: %+v", selection)
	}
}

func TestAgentHistoricalFXExactAndCarryCoverage(t *testing.T) {
	t.Parallel()
	householdID := domain.NewHouseholdID()
	pref := domain.FXPreference{HouseholdID: householdID, CurrencyA: "USD", CurrencyB: "SGD", SourceKind: domain.QuoteSourceProvider}
	rate := func(value string) domain.FxRate {
		parsed, err := domain.ParseFxRate(value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	day := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	agent := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "SGD", Rate: rate("1.4"), SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", ObservationKind: string(FXObservationDailyReference), EffectiveDate: "2026-09-20", ValueEffectiveAt: day, SourcePolicyVersion: "agent_supplied_v1", TimestampBasis: "date_label"}
	provider := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "SGD", Rate: rate("1.3"), SourceKind: domain.QuoteSourceProvider, SourceKey: domain.FrankfurterProviderKey, ObservationKind: string(FXObservationDailyReference), EffectiveDate: "2026-09-20", ValueEffectiveAt: day, SourcePolicyVersion: domain.FrankfurterV2BlendedPolicy, TimestampBasis: string(TimestampBasisPolicyDerived)}
	selected := selectHistoricalFXQuoteAtMarketDate(pref, []domain.FXQuote{provider, agent}, "USD", "SGD", domain.FrankfurterProviderKey, "2026-09-20", day.AddDate(0, 0, 1))
	if selected == nil || selected.ID != agent.ID || !historicalFXCoverageCompleteAtMarketDate(domain.PortfolioSnapshot{}, *selected, "2026-09-20") {
		t.Fatalf("exact Agent FX should win and be complete: %+v", selected)
	}
	selected = selectHistoricalFXQuoteAtMarketDate(pref, []domain.FXQuote{agent}, "USD", "SGD", "", "2026-09-21", day.AddDate(0, 0, 2))
	if selected == nil || selected.ID != agent.ID || historicalFXCoverageCompleteAtMarketDate(domain.PortfolioSnapshot{}, *selected, "2026-09-21") {
		t.Fatalf("carried Agent FX should remain incomplete: %+v", selected)
	}
}

func TestAgentDatesRemoveOrdinaryHistoryRequests(t *testing.T) {
	t.Parallel()
	ranges := []DateRange{{Start: "2026-09-18", End: "2026-09-22"}}
	remaining, err := excludeExactDates(ranges, map[string]struct{}{"2026-09-19": {}, "2026-09-21": {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 3 || remaining[0].Start != "2026-09-18" || remaining[1].Start != "2026-09-20" || remaining[2].Start != "2026-09-22" {
		t.Fatalf("ordinary provider fetch still includes Agent exact dates: %+v", remaining)
	}
}

func TestChartDailyAgentWinsProviderForSameEffectiveDate(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	provider := domain.QuoteSeriesPoint{ID: "provider", SourceKind: domain.QuoteSourceProvider, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-20", QuotedAt: day.Add(20 * time.Hour), Value: "11"}
	agent := domain.QuoteSeriesPoint{ID: "agent", SourceKind: domain.QuoteSourceAgent, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-20", QuotedAt: day.Add(10 * time.Hour), Value: "10"}
	for _, observations := range [][]domain.QuoteSeriesPoint{{provider, agent}, {agent, provider}} {
		points := chartQuotePoints(observations, domain.TrendOneYear, time.UTC)
		if len(points) != 1 || points[0].ID != "agent" {
			t.Fatalf("chart did not use exact Agent close: %+v", points)
		}
	}
}

func TestHistoricalAnchorUsesAgentDailyDateAndIgnoresAgentLatest(t *testing.T) {
	t.Parallel()
	id := domain.NewInstrumentID()
	day := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	daily := domain.InstrumentQuote{InstrumentID: id, SourceKind: domain.QuoteSourceAgent, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-19", QuotedAt: day, SourcePolicyVersion: "agent_supplied_v1", PriceBasis: "agent_unit_nav_v1", TimestampBasis: "date_label"}
	latest := daily
	latest.ObservationKind = string(InstrumentObservationRealtime)
	if hasQuoteOnOrBefore([]domain.InstrumentQuote{latest}, "2026-09-19", time.UTC) {
		t.Fatal("Agent latest quote was treated as a historical opening anchor")
	}
	if !hasQuoteOnOrBefore([]domain.InstrumentQuote{daily}, "2026-09-19", time.UTC) {
		t.Fatal("Agent daily date was not treated as a historical opening anchor")
	}
}

func TestAgentStockClosedDaysCarryCompleteButMissingTradingDayDoesNot(t *testing.T) {
	t.Parallel()
	id := domain.NewInstrumentID()
	market := "US"
	price, err := domain.ParseUnitPrice("100")
	if err != nil {
		t.Fatal(err)
	}
	friday := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "USD", UnitPrice: price,
		SourceKind: domain.QuoteSourceAgent, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-18",
		ValueEffectiveAt: time.Date(2026, 9, 18, 20, 0, 0, 0, time.UTC), PriceBasis: "agent_raw_close_v1", SourcePolicyVersion: "agent_supplied_v1", TimestampBasis: "source_timestamp"}
	instrument := domain.Instrument{ID: id, Type: domain.InstrumentStock, MarketCode: &market, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceAgent}
	selection := selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(instrument, []domain.InstrumentQuote{friday}, nil, "2026-09-20", time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC))
	if selection.quote == nil || !selection.coverageComplete {
		t.Fatalf("stock close should cover weekend: %+v", selection)
	}
	selection = selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(instrument, []domain.InstrumentQuote{friday}, nil, "2026-09-21", time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC))
	if selection.quote == nil || selection.coverageComplete {
		t.Fatalf("stock close should not complete missing Monday: %+v", selection)
	}
	instrument.Type = domain.InstrumentMutualFund
	friday.PriceBasis = "agent_unit_nav_v1"
	selection = selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(instrument, []domain.InstrumentQuote{friday}, nil, "2026-09-20", time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC))
	if selection.quote == nil || selection.coverageComplete {
		t.Fatalf("NAV carry should remain incomplete on missing date: %+v", selection)
	}
}

func TestAgentMixedDailyAndRealtimeSelectionIsOrderIndependent(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	id := domain.NewInstrumentID()
	householdID := domain.NewHouseholdID()
	instrument := domain.Instrument{ID: id, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceProvider}
	preference := domain.FXPreference{HouseholdID: householdID, SourceKind: domain.QuoteSourceProvider}
	points := []domain.QuoteSeriesPoint{
		{ID: "agent-close", SourceKind: domain.QuoteSourceAgent, ObservationKind: "close", EffectiveDate: "2026-09-20", QuotedAt: day, Value: "10"},
		{ID: "provider-close", SourceKind: domain.QuoteSourceProvider, ObservationKind: "close", EffectiveDate: "2026-09-20", QuotedAt: day.Add(16 * time.Hour), Value: "11"},
		{ID: "provider-realtime", SourceKind: domain.QuoteSourceProvider, ObservationKind: "realtime", QuotedAt: day.Add(12 * time.Hour), Value: "12"},
	}
	quotes := make([]domain.InstrumentQuote, 3)
	fx := make([]domain.FXQuote, 3)
	for n, point := range points {
		price, _ := domain.ParseUnitPrice(point.Value)
		rate, _ := domain.ParseFxRate(point.Value)
		quotes[n] = domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "USD", UnitPrice: price, SourceKind: point.SourceKind, ObservationKind: point.ObservationKind, EffectiveDate: point.EffectiveDate, QuotedAt: point.QuotedAt}
		kind := string(FXObservationLatest)
		if point.EffectiveDate != "" {
			kind = string(FXObservationDailyReference)
		}
		fx[n] = domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "SGD", Rate: rate, SourceKind: point.SourceKind, ObservationKind: kind, EffectiveDate: point.EffectiveDate, QuotedAt: point.QuotedAt}
	}
	for _, order := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		orderedQuotes := []domain.InstrumentQuote{quotes[order[0]], quotes[order[1]], quotes[order[2]]}
		if got := selectInstrumentQuote(instrument, orderedQuotes); got == nil || got.ID != quotes[2].ID {
			t.Fatalf("order %v selected instrument %+v", order, got)
		}
		orderedFX := []domain.FXQuote{fx[order[0]], fx[order[1]], fx[order[2]]}
		if got := selectFXQuote(preference, orderedFX, "USD", "SGD", "", nil); got == nil || got.ID != fx[2].ID {
			t.Fatalf("order %v selected FX %+v", order, got)
		}
		orderedPoints := []domain.QuoteSeriesPoint{points[order[0]], points[order[1]], points[order[2]]}
		got := chartQuotePoints(orderedPoints, domain.TrendOneYear, time.UTC)
		if len(got) != 1 || got[0].ID != points[2].ID {
			t.Fatalf("order %v selected chart %+v", order, got)
		}
	}
}
