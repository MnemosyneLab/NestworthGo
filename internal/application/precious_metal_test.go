package application

import (
	"context"
	"encoding/json"
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestMetalTemplatePreviewRefreshAndMissingFX(t *testing.T) {
	_, s, fake, _, _ := newRefreshFixture(t)
	ctx := context.Background()
	s.SetMarketDataRegistry(NewMarketDataRegistry(fake, &refreshAliasProvider{key: YahooFinanceProviderKey, target: fake}))
	raw, _ := domain.ParseUnitPrice(domain.TroyOunceGrams)
	fake.instruments["GC=F"] = struct {
		quote LatestInstrumentQuote
		err   error
	}{quote: LatestInstrumentQuote{Price: raw, Currency: "USD", SourceKey: "fake", QuotedAt: s.clock()}}
	input := InstrumentInput{Name: "Gold", Type: "precious_metal", QuoteCurrency: "CNY", QuoteSource: "provider", ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "GC=F", MarketCode: domain.MetalFuturesMarket, MetalTemplate: "gold", QuantityUnit: "g"}
	instrument, err := s.CreateInstrument(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.FXQuoteHistory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	preview, _, err := s.PreviewMetalQuote(ctx, "gold", "g", "CNY")
	if err != nil || preview.Price.Canonical() != "6.9" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	after, err := s.FXQuoteHistory(ctx)
	if err != nil || len(after) != len(before) {
		t.Fatal("preview wrote FX quotes")
	}
	result, err := s.RefreshInstrument(ctx, instrument.ID)
	if err != nil || len(result.Items) != 1 || result.Items[0].Status != RefreshFetched {
		t.Fatalf("refresh: %+v %v", result, err)
	}
	quote, err := s.CurrentInstrumentQuote(ctx, instrument.ID)
	if err != nil || quote == nil || quote.UnitPrice.Canonical() != "6.9" || quote.Currency != "CNY" {
		t.Fatalf("quote: %+v %v", quote, err)
	}
	var evidence metalConversionEvidence
	if err := json.Unmarshal([]byte(quote.ConversionJSON), &evidence); err != nil || evidence.RawPrice != domain.TroyOunceGrams || evidence.FXRate != "6.9" {
		t.Fatalf("evidence: %+v %v", evidence, err)
	}
	originalID := quote.ID
	delete(fake.fx, "USD/CNY")
	result, err = s.RefreshInstrument(ctx, instrument.ID)
	if err != nil || result.Items[0].Status != RefreshFailed {
		t.Fatalf("missing FX: %+v %v", result, err)
	}
	quote, err = s.CurrentInstrumentQuote(ctx, instrument.ID)
	if err != nil || quote == nil || quote.ID != originalID {
		t.Fatal("missing FX replaced last valid price")
	}
	input.Replace = true
	input.QuantityUnit = "troy_oz"
	if _, err := s.UpdateInstrument(ctx, instrument.ID, input); err == nil {
		t.Fatal("unit change must be rejected")
	}
}

func TestMetalCurrencyVariantsAndRequiredFX(t *testing.T) {
	_, s, _, _, _ := newRefreshFixture(t)
	ctx := context.Background()
	for _, unit := range []string{"g", "troy_oz"} {
		if _, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Gold", Type: "precious_metal", QuoteCurrency: "CNY", QuoteSource: "provider", ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "GC=F", MarketCode: domain.MetalFuturesMarket, MetalTemplate: "gold", QuantityUnit: unit}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, target := range refreshTargetsForSnapshot(snapshot) {
		if target.kind == RefreshFXTarget && target.baseCurrency == "USD" && target.quoteCurrency == "CNY" {
			found = true
		}
	}
	if !found {
		t.Fatal("USD FX dependency is required even when target equals household currency")
	}
}

func TestMetalHistoryNeverUsesFutureFX(t *testing.T) {
	_, s, _, _, _ := newRefreshFixture(t)
	ctx := context.Background()
	if _, err := s.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	now := s.clock()
	if _, err := s.AppendManualFXQuote(ctx, "USD", "CNY", "2", now.Add(-48*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendManualFXQuote(ctx, "USD", "CNY", "99", now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		t.Fatal(err)
	}
	instrument := domain.Instrument{HouseholdID: household.ID, Type: domain.InstrumentPreciousMetal, MetalTemplate: "gold", QuantityUnit: "g", QuoteCurrency: "CNY"}
	outcome := MappingOutcome[InstrumentDailyObservation]{Status: MappingMapped, Batch: HistoryBatch[InstrumentDailyObservation]{Observations: []InstrumentDailyObservation{{MarketDate: "2026-08-22", Value: domain.TroyOunceGrams, Currency: "USD", ValueEffectiveAt: now.Add(-24 * time.Hour)}}}}
	got, err := s.convertMetalHistory(ctx, nil, instrument, DateRange{Start: "2026-08-22", End: "2026-08-22"}, outcome, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Batch.Observations[0].Value != "2" || got.Batch.Observations[0].Currency != "CNY" {
		t.Fatalf("historical conversion used wrong FX: %+v", got)
	}
}
