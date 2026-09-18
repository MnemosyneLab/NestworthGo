package application

import (
	"context"
	"errors"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type deterministicProvider struct{ key string }

func (p deterministicProvider) Key() string { return p.key }

func (p deterministicProvider) Capabilities() MarketDataCapabilities {
	return MarketDataCapabilities{LatestInstrument: true, LatestFX: true}
}

func (p deterministicProvider) LatestInstrument(context.Context, InstrumentMarketIdentity) (LatestInstrumentQuote, error) {
	price, _ := domain.ParseUnitPrice("1.25")
	return LatestInstrumentQuote{Price: price, Currency: "USD", SourceKey: p.key}, nil
}

func (p deterministicProvider) LatestFX(context.Context, FXMarketIdentity) (LatestFXQuote, error) {
	rate, _ := domain.ParseFxRate("1.25")
	return LatestFXQuote{Rate: rate, BaseCurrency: "USD", QuoteCurrency: "CNY", SourceKey: p.key}, nil
}

func TestMarketDataRegistryResolvesDeterministicProviders(t *testing.T) {
	first := deterministicProvider{key: "first"}
	second := deterministicProvider{key: "second"}
	registry := NewMarketDataRegistry(first, second)
	resolved, err := registry.Resolve("second")
	if err != nil || resolved.Key() != "second" {
		t.Fatalf("Resolve(second) = %v, %v", resolved, err)
	}
	defaultProvider, err := registry.Default()
	if err != nil || defaultProvider.Key() != "first" {
		t.Fatalf("Default() = %v, %v", defaultProvider, err)
	}
	if _, err := registry.Resolve("missing"); err == nil {
		t.Fatal("missing provider resolved without an error")
	}
	providers := registry.Providers()
	if len(providers) != 2 || providers[0].Key() != "first" || providers[1].Key() != "second" {
		t.Fatalf("Providers() = %v, want deterministic first/second order", providers)
	}
	if err := registry.Register(first); err == nil {
		t.Fatal("duplicate provider registration succeeded")
	}
	capabilities := resolved.Capabilities()
	if !capabilities.LatestInstrument || !capabilities.LatestFX || capabilities.InstrumentSearch || capabilities.DailyHistory || capabilities.InstrumentDailyHistory || capabilities.FXDailyHistory {
		t.Fatalf("capabilities = %#v", capabilities)
	}
}

func TestMarketDataRegistryMissingDefaultIsSafe(t *testing.T) {
	registry := NewMarketDataRegistry()
	_, err := registry.Default()
	if err == nil {
		t.Fatal("empty registry returned a provider")
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrUnavailable {
		t.Fatalf("empty registry error = %#v", err)
	}
}

func TestMarketDataRegistrySupportsExplicitDefault(t *testing.T) {
	registry := NewMarketDataRegistryWithDefault("second", deterministicProvider{key: "first"}, deterministicProvider{key: "second"})
	provider, err := registry.Default()
	if err != nil || provider.Key() != "second" {
		t.Fatalf("explicit Default() = %v, %v", provider, err)
	}
}

func TestInstrumentProviderKeysExcludesFXOnlyProviders(t *testing.T) {
	keys := InstrumentProviderKeys()
	if len(keys) != 4 || keys[3] != CoinGeckoProviderKey || keys[0] != YahooFinanceProviderKey || keys[1] != TiingoProviderKey || keys[2] != WorkerProviderKey {
		t.Fatalf("InstrumentProviderKeys() = %v, want [%s %s %s]", keys, YahooFinanceProviderKey, TiingoProviderKey, WorkerProviderKey)
	}
	for _, key := range keys {
		if key == FrankfurterProviderKey {
			t.Fatalf("InstrumentProviderKeys included FX-only key %s", key)
		}
	}
}

type yahooSearchProvider struct {
	deterministicProvider
	hits  []InstrumentSearchHit
	err   error
	query string
	typ   string
	limit int
}

func (p *yahooSearchProvider) Capabilities() MarketDataCapabilities {
	return MarketDataCapabilities{LatestInstrument: true, InstrumentSearch: true}
}

func (p *yahooSearchProvider) SearchInstruments(_ context.Context, query, instrumentType string, limit int) ([]InstrumentSearchHit, error) {
	p.query = query
	p.typ = instrumentType
	p.limit = limit
	return p.hits, p.err
}

func TestSearchInstrumentsUsesYahooProvider(t *testing.T) {
	yahoo := &yahooSearchProvider{
		deterministicProvider: deterministicProvider{key: YahooFinanceProviderKey},
		hits: []InstrumentSearchHit{{
			ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "NVDA", Name: "NVIDIA Corporation",
			Symbol: "NVDA", Type: "stock", MarketCode: "NASDAQ", CountryCode: "US", QuoteCurrency: "USD",
		}},
	}
	service := NewService(nil, NewMarketDataRegistry(yahoo, deterministicProvider{key: TiingoProviderKey}))
	hits, err := service.SearchInstruments(context.Background(), "  NVDA  ", "stock")
	if err != nil {
		t.Fatalf("SearchInstruments: %v", err)
	}
	if yahoo.query != "NVDA" || yahoo.typ != "stock" || yahoo.limit != instrumentSearchLimit {
		t.Fatalf("yahoo search args = query=%q type=%q limit=%d", yahoo.query, yahoo.typ, yahoo.limit)
	}
	if len(hits) != 1 || hits[0].ProviderSymbol != "NVDA" {
		t.Fatalf("hits = %#v", hits)
	}
}

func TestSearchInstrumentsRejectsUnsupportedType(t *testing.T) {
	yahoo := &yahooSearchProvider{deterministicProvider: deterministicProvider{key: YahooFinanceProviderKey}}
	service := NewService(nil, NewMarketDataRegistry(yahoo))
	_, err := service.SearchInstruments(context.Background(), "XAU", "precious_metal")
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrValidation || domainErr.Field != "type" {
		t.Fatalf("err = %#v", err)
	}
	if yahoo.query != "" {
		t.Fatal("unsupported search should not call Yahoo")
	}
}

func TestSearchInstrumentsRequiresYahooSearchCapability(t *testing.T) {
	service := NewService(nil, NewMarketDataRegistry(deterministicProvider{key: YahooFinanceProviderKey}))
	_, err := service.SearchInstruments(context.Background(), "NVDA", "stock")
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrUnavailable {
		t.Fatalf("err = %#v", err)
	}
}

func TestSearchCryptoUsesCoinGecko(t *testing.T) {
	cg := &yahooSearchProvider{deterministicProvider: deterministicProvider{key: CoinGeckoProviderKey}, hits: []InstrumentSearchHit{{ProviderKey: CoinGeckoProviderKey, ProviderSymbol: "bitcoin", Type: "crypto"}}}
	yahoo := &yahooSearchProvider{deterministicProvider: deterministicProvider{key: YahooFinanceProviderKey}}
	service := NewService(nil, NewMarketDataRegistry(yahoo, cg))
	hits, err := service.SearchInstruments(context.Background(), "BTC", "crypto")
	if err != nil || len(hits) != 1 || cg.typ != "crypto" || yahoo.query != "" {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
}
