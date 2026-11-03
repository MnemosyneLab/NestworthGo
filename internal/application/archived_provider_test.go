package application

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestArchivedProviderFactsRemainReadableWithoutRefreshOrFallback(t *testing.T) {
	ctx := context.Background()
	fake := &countingProvider{inner: &syncFakeProvider{key: YahooFinanceProviderKey}}
	service, repo, instrument, _ := newSyncFixture(t, fake)
	// Seed an old-release binding directly into this synthetic DB. Current public
	// creation APIs must reject it, but repositories must still read old records.
	source := "tiingo"
	basis := "tiingo_raw_close_v1"
	instrument.ProviderKey = &source
	if err := repo.UpdateInstrument(ctx, instrument); err != nil {
		t.Fatal(err)
	}
	price, err := domain.ParseUnitPrice("185.25")
	if err != nil {
		t.Fatal(err)
	}
	effective := time.Date(2026, 9, 4, 20, 0, 0, 0, time.UTC)
	_, err = repo.CommitInstrumentHistory(ctx, sqlite.InstrumentHistoryCommit{
		HouseholdID: instrument.HouseholdID, InstrumentID: instrument.ID, ProviderKey: source,
		ProviderSymbol: "AAPL", QuoteCurrency: "USD", Market: "US", Status: "mapped",
		Adapter: "archived_import", SourcePolicy: basis, FetchedAt: effective.Add(time.Minute),
		Observations: []sqlite.InstrumentHistoryObservation{{MarketDate: "2026-09-04", Value: price.Canonical(), Currency: "USD", ValueEffectiveAt: effective, Kind: "close", PriceBasis: basis, TimestampBasis: "session_close"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := repo.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(before) != 1 {
		t.Fatalf("quotes=%+v err=%v", before, err)
	}
	restored, err := repo.Instrument(ctx, instrument.HouseholdID, instrument.ID)
	if err != nil {
		t.Fatal(err)
	}
	selected := selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(restored, before, nil, "2026-09-06", effective.Add(48*time.Hour)).quote
	if selected == nil || selected.UnitPrice.Canonical() != price.Canonical() || selected.SourceKey != source {
		t.Fatalf("archived valuation evidence lost: %+v", selected)
	}
	result, err := service.RefreshInstrument(ctx, instrument.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Status == RefreshFetched || result.Items[0].Status == RefreshCached {
		t.Fatalf("retired refresh succeeded: %+v", result)
	}
	if fake.networkCalls() != 0 {
		t.Fatal("retired binding silently fell back to Yahoo")
	}
	plan, err := service.PlanHistorySync(ctx, HistorySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range plan.Instruments {
		if item.InstrumentID == instrument.ID && len(item.FetchRanges) > 0 {
			t.Fatal("retired binding planned a network fetch")
		}
	}
	if _, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Rejected", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: source, ProviderSymbol: "AAPL", MarketCode: "US"}); err == nil {
		t.Fatal("retired source was selectable")
	}
	if _, err := service.UpdateInstrument(ctx, instrument.ID, InstrumentInput{ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "AAPL"}); err != nil {
		t.Fatal(err)
	}
	after, err := repo.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("explicit provider selection rewrote facts: %v", err)
	}
}

func TestArchivedQuoteQualityRemainsStrict(t *testing.T) {
	quote := domain.InstrumentQuote{SourceKey: "tiingo", PriceBasis: "tiingo_raw_close_v1", SourcePolicyVersion: "tiingo_raw_close_v1", TimestampBasis: "session_close"}
	if !historicalInstrumentQualityMatches(quote) {
		t.Fatal("valid archived fact rejected")
	}
	for _, field := range []string{"basis", "policy", "timestamp"} {
		invalid := quote
		switch field {
		case "basis":
			invalid.PriceBasis = "adjusted_close"
		case "policy":
			invalid.SourcePolicyVersion = "unknown"
		case "timestamp":
			invalid.TimestampBasis = "date_label"
		}
		if historicalInstrumentQualityMatches(invalid) {
			t.Fatalf("invalid archived %s accepted", field)
		}
	}
}
