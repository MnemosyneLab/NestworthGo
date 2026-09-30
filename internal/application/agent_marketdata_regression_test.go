package application

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestAgentFXRetractionRestoresConvertedMetalRevision(t *testing.T) {
	s, ctx, _, setClock := newOnboardedService(t, "agent-metal-fx-rollback", nil)
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	setClock(now)
	_, err := s.SetFXPreference(ctx, "USD", "CNY", "manual")
	if err != nil {
		t.Fatal(err)
	}
	oldAt := now.Add(-3 * time.Hour)
	_, err = s.AppendManualFXQuote(ctx, "USD", "CNY", "6", oldAt.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Gold", Type: "precious_metal", QuoteCurrency: "CNY", QuoteSource: "provider", ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "GC=F", MarketCode: domain.MetalFuturesMarket, MetalTemplate: "gold", QuantityUnit: "g"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := domain.ParseUnitPrice(domain.TroyOunceGrams)
	p, _ := domain.ParseUnitPrice("6")
	initial, err := domain.NewInstrumentQuote(i, domain.InstrumentQuoteInput{UnitPrice: p, SourceKind: domain.QuoteSourceProvider, SourceKey: YahooFinanceProviderKey, QuotedAt: oldAt, Delayed: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	rate, _ := domain.ParseFxRate("6")
	initial.ConversionJSON = metalEvidence(i, raw, now.Add(-4*time.Hour), rate.Decimal(), "manual", &oldAt)
	if _, err = s.repository.AppendProviderInstrumentQuoteIfChanged(ctx, initial); err != nil {
		t.Fatal(err)
	}
	added, err := s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{BaseCurrency: "USD", QuoteCurrency: "CNY", Value: "7", Kind: "latest", QuotedAt: now.Add(-time.Hour).Format(time.RFC3339), SourceTitle: "Agent FX"}}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.CurrentInstrumentQuote(ctx, i.ID)
	if err != nil || before == nil || before.UnitPrice.Canonical() != "7" {
		t.Fatalf("setup reprice failed: %+v %v", before, err)
	}
	result, err := s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{Operation: "retract", TargetQuoteID: added.QuoteIDs[0], SourceTitle: "Withdraw"}}})
	if err != nil {
		t.Fatal(err)
	}
	fx, err := s.CurrentFXQuote(ctx, "USD", "CNY")
	if err != nil || fx == nil || fx.Rate.Canonical() != "6" {
		t.Fatalf("FX fallback failed: %+v %v", fx, err)
	}
	after, err := s.CurrentInstrumentQuote(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after == nil || after.UnitPrice.Canonical() != "6" {
		t.Fatalf("withdrawn FX still affects metal price: quote=%+v, receipt=%+v", after, result)
	}
	if !after.QuotedAt.Equal(oldAt) || after.Revision <= before.Revision {
		t.Fatalf("fallback did not preserve observation time and advance conversion revision: before=%+v after=%+v", before, after)
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	selected := selectInstrumentQuote(i, snapshot.InstrumentQuotes)
	if selected == nil || selected.ID != after.ID {
		t.Fatalf("portfolio retained withdrawn conversion: %+v", selected)
	}
	quotes, err := s.repository.ListInstrumentQuotes(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 3 {
		t.Fatalf("restoring a previously saved value should append one revision: %d quotes", len(quotes))
	}
	if err = s.repriceMetalsForFX(ctx, i.HouseholdID, "USD", "CNY"); err != nil {
		t.Fatal(err)
	}
	again, err := s.repository.ListInstrumentQuotes(ctx, i.ID)
	if err != nil || len(again) != len(quotes) {
		t.Fatalf("unchanged conversion duplicated facts: %d, %v", len(again), err)
	}
}

func TestAgentMixedQuotesAgreeAcrossCurrentAndPortfolio(t *testing.T) {
	s, ctx, _, setClock := newOnboardedService(t, "agent-selection-consistency", nil)
	setClock(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Stock", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "ABC", MarketCode: "US"})
	if err != nil {
		t.Fatal(err)
	}
	repo := s.repository.(*sqlite.Repository)
	_, err = repo.CommitInstrumentHistory(ctx, sqlite.InstrumentHistoryCommit{
		HouseholdID: i.HouseholdID, InstrumentID: i.ID, ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "ABC", QuoteCurrency: "USD", Market: "US", Status: "mapped", FetchedAt: s.clock(),
		Observations: []sqlite.InstrumentHistoryObservation{{MarketDate: "2026-09-28", Value: "11", Currency: "USD", Kind: "close", ValueEffectiveAt: time.Date(2026, 9, 28, 16, 0, 0, 0, time.UTC)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{
		{InstrumentID: i.ID.String(), Currency: "USD", Kind: "close", Date: "2026-09-28", Timezone: "UTC", Value: "10", SourceTitle: "Close"},
		{InstrumentID: i.ID.String(), Currency: "USD", Kind: "latest", QuotedAt: "2026-09-28T12:00:00Z", Value: "12", SourceTitle: "Latest"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.CurrentInstrumentQuote(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	selected := selectInstrumentQuote(i, snapshot.InstrumentQuotes)
	if current == nil || selected == nil || current.ID != selected.ID || current.UnitPrice.Canonical() != "12" {
		t.Fatalf("local current/MCP quote=%+v differs from portfolio snapshot/overview quote=%+v", current, selected)
	}
}

func TestAgentDailySeriesFiltersEffectiveDateAcrossTimezones(t *testing.T) {
	for _, timezone := range []string{"Asia/Singapore", "America/New_York"} {
		t.Run(timezone, func(t *testing.T) {
			s, i, _ := newAgentMarketDataFixture(t)
			ctx := t.Context()
			for _, date := range []string{"2026-09-27", "2026-09-28"} {
				item := agentNAV(i, "1.02", date)
				item.Timezone = timezone
				fx := AgentMarketDataItem{BaseCurrency: "USD", QuoteCurrency: "CNY", Value: "7", Kind: "daily_reference", Date: date, Timezone: timezone, SourceTitle: "Daily FX"}
				if _, err := s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{item, fx}}); err != nil {
					t.Fatal(err)
				}
			}
			// This realtime observation is outside the UTC date and must stay out.
			_, err := s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: i.ID.String(), Currency: "USD", Value: "2", Kind: "latest", QuotedAt: "2026-09-27T23:59:00Z", SourceTitle: "Realtime"}}})
			if err != nil {
				t.Fatal(err)
			}
			rng, err := domain.ParseTrendRange("2026-09-28:2026-09-28")
			if err != nil {
				t.Fatal(err)
			}
			instrument, err := s.InstrumentQuoteSeries(ctx, i.ID, rng, domain.QuoteSourceFilterAll)
			if err != nil {
				t.Fatal(err)
			}
			fx, err := s.FXQuoteSeries(ctx, "USD", "CNY", rng, domain.QuoteSourceFilterAll)
			if err != nil {
				t.Fatal(err)
			}
			for _, series := range []domain.QuoteSeries{instrument, fx} {
				if len(series.Observations) != 1 || len(series.Points) != 1 || series.OutsideRange || series.Observations[0].EffectiveDate != "2026-09-28" {
					t.Fatalf("effective date lost or adjacent observation included: %+v", series)
				}
			}
			from := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
			history, err := s.InstrumentQuoteHistory(ctx, i.ID, QuoteHistoryQuery{From: &from})
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if timezone == "Asia/Singapore" {
				want = 0
			}
			if len(history) != want {
				t.Fatalf("explicit timestamp query stopped using instant bounds: %d, want %d", len(history), want)
			}
		})
	}
}

func TestAgentLastFXRetractionMakesConvertedMetalUnavailable(t *testing.T) {
	s, ctx, _, setClock := newOnboardedService(t, "last-metal-fx", nil)
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	setClock(now)
	added, err := s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{BaseCurrency: "USD", QuoteCurrency: "CNY", Value: "7", Kind: "latest", QuotedAt: now.Add(-time.Hour).Format(time.RFC3339), SourceTitle: "Agent FX"}}})
	if err != nil {
		t.Fatal(err)
	}
	i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Gold", Type: "precious_metal", QuoteCurrency: "CNY", QuoteSource: "provider", ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "GC=F", MarketCode: domain.MetalFuturesMarket, MetalTemplate: "gold", QuantityUnit: "g"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := domain.ParseUnitPrice(domain.TroyOunceGrams)
	price, _ := domain.ParseUnitPrice("7")
	rate, _ := domain.ParseFxRate("7")
	at := now.Add(-time.Hour)
	q, err := domain.NewInstrumentQuote(i, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceProvider, SourceKey: YahooFinanceProviderKey, QuotedAt: at}, now)
	if err != nil {
		t.Fatal(err)
	}
	q.ConversionJSON = metalEvidence(i, raw, now.Add(-2*time.Hour), rate.Decimal(), "agent", &at)
	if _, err = s.repository.AppendProviderInstrumentQuoteIfChanged(ctx, q); err != nil {
		t.Fatal(err)
	}
	if current, err := s.CurrentInstrumentQuote(ctx, i.ID); err != nil || current == nil {
		t.Fatalf("conversion unavailable before withdrawal: %+v %v", current, err)
	}
	_, err = s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{Operation: "retract", TargetQuoteID: added.QuoteIDs[0], SourceTitle: "Withdraw"}}})
	if err != nil {
		t.Fatal(err)
	}
	if current, err := s.CurrentInstrumentQuote(ctx, i.ID); err != nil || current != nil {
		t.Fatalf("withdrawn last FX still supplies converted price: %+v %v", current, err)
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	quantity, _ := domain.ParseQuantity("1")
	valuation := NewValuationService(s.repository, s.clock)
	component, missing, err := valuation.valueHolding(snapshot, domain.NewAccountID(), domain.Holding{ID: domain.NewHoldingID(), InstrumentID: i.ID, Quantity: quantity}, i)
	if err != nil || component.Available || len(missing) != 1 || missing[0].Kind != domain.MissingFXRate {
		t.Fatalf("portfolio should report missing conversion FX: %+v %+v %v", component, missing, err)
	}
	// A direct local-currency Agent price does not depend on the withdrawn FX.
	_, err = s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: i.ID.String(), Currency: "CNY", Value: "8", Kind: "latest", QuotedAt: now.Format(time.RFC3339), SourceTitle: "Local gold price"}}})
	if err != nil {
		t.Fatal(err)
	}
	if current, err := s.CurrentInstrumentQuote(ctx, i.ID); err != nil || current == nil || current.UnitPrice.Canonical() != "8" {
		t.Fatalf("direct local price should remain available: %+v %v", current, err)
	}
}

func TestAgentMixedFXAgreeAcrossCurrentAndPortfolio(t *testing.T) {
	s, ctx, bootstrap, setClock := newOnboardedService(t, "agent-fx-selection", nil)
	setClock(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	_, err := s.SetFXPreference(ctx, "USD", "CNY", "provider")
	if err != nil {
		t.Fatal(err)
	}
	repo := s.repository.(*sqlite.Repository)
	_, err = repo.CommitFXHistory(ctx, sqlite.FXHistoryCommit{HouseholdID: bootstrap.Household.ID, ProviderKey: s.FXProviderKey(), BaseCurrency: "USD", QuoteCurrency: "CNY", Status: "mapped", FetchedAt: s.clock(),
		Observations: []sqlite.FXHistoryObservation{{MarketDate: "2026-09-28", Rate: "6", BaseCurrency: "USD", QuoteCurrency: "CNY", Kind: "daily_reference", ValueEffectiveAt: time.Date(2026, 9, 28, 16, 0, 0, 0, time.UTC)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{
		{BaseCurrency: "USD", QuoteCurrency: "CNY", Value: "7", Kind: "daily_reference", Date: "2026-09-28", Timezone: "UTC", SourceTitle: "Daily FX"},
		{BaseCurrency: "USD", QuoteCurrency: "CNY", Value: "8", Kind: "latest", QuotedAt: "2026-09-28T12:00:00Z", SourceTitle: "Latest FX"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.CurrentFXQuote(ctx, "USD", "CNY")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := repo.ReadPortfolioSnapshot(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	pref := findFXPreference(snapshot.FXPreferences, "USD", "CNY")
	if pref == nil {
		t.Fatal("missing FX preference")
	}
	selected := selectFXQuote(*pref, snapshot.FXQuotes, "USD", "CNY", s.FXProviderKey(), nil)
	if current == nil || selected == nil || current.ID != selected.ID || current.Rate.Canonical() != "8" {
		t.Fatalf("current FX and portfolio selection disagree: %+v %+v", current, selected)
	}
}
