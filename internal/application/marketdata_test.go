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
	if !capabilities.LatestInstrument || !capabilities.LatestFX || capabilities.InstrumentSearch || capabilities.DailyHistory {
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
