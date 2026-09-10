package application

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// YahooFinanceProviderKey is the compiled-in provider binding used by the
// desktop experience. Keeping the key at the application boundary
// avoids making UI code depend on the infrastructure adapter.
const YahooFinanceProviderKey = "yahoo_finance"

const FrankfurterProviderKey = "frankfurter"

// TiingoProviderKey identifies the US-listed equity provider. It is part of
// InstrumentProviderKeys so US bindings can select Tiingo or Yahoo.
const TiingoProviderKey = "tiingo"

// InstrumentProviderKeys is the closed catalog of provider keys that can
// bind an Instrument's quote source. Frankfurter is FX-only and is not
// included. US instruments may use Yahoo or Tiingo; other markets stay
// Yahoo-only at routing time.
func InstrumentProviderKeys() []string {
	return []string{YahooFinanceProviderKey, TiingoProviderKey}
}

// MarketDataCapabilities describes the deliberately small provider surface.
// Providers cannot imply search or historical-data support.
type MarketDataCapabilities struct {
	LatestInstrument       bool
	LatestFX               bool
	InstrumentSearch       bool
	DailyHistory           bool
	InstrumentDailyHistory bool
	FXDailyHistory         bool
}

// InstrumentMarketIdentity is the provider-neutral identity needed for a
// current instrument quote. Provider metadata is not exposed to valuation.
type InstrumentMarketIdentity struct {
	ProviderKey    string
	ProviderSymbol string
	QuoteCurrency  domain.CurrencyCode
	Market         string
}

// FXMarketIdentity identifies one direct native-to-quote currency request.
// Providers derive their external symbol from these currencies.
type FXMarketIdentity struct {
	ProviderKey   string
	BaseCurrency  domain.CurrencyCode
	QuoteCurrency domain.CurrencyCode
}

type LatestInstrumentQuote struct {
	Price     domain.UnitPrice
	Currency  domain.CurrencyCode
	SourceKey string
	QuotedAt  time.Time
	Delayed   bool
}

type LatestFXQuote struct {
	Rate          domain.FxRate
	BaseCurrency  domain.CurrencyCode
	QuoteCurrency domain.CurrencyCode
	SourceKey     string
	QuotedAt      time.Time
	Delayed       bool
}

// MarketDataProvider is the stable application port implemented by
// infrastructure adapters and deterministic test doubles.
type MarketDataProvider interface {
	Key() string
	Capabilities() MarketDataCapabilities
	LatestInstrument(context.Context, InstrumentMarketIdentity) (LatestInstrumentQuote, error)
	LatestFX(context.Context, FXMarketIdentity) (LatestFXQuote, error)
}

// MarketDataRegistryPort keeps Service independent from provider routing and
// from the infrastructure package that supplies the production registry.
type MarketDataRegistryPort interface {
	Resolve(string) (MarketDataProvider, error)
	Default() (MarketDataProvider, error)
	Providers() []MarketDataProvider
}

// MarketDataRegistry is the deterministic compiled-in provider registry used
// by production and tests. Registration order chooses the default provider.
type MarketDataRegistry struct {
	providers  map[string]MarketDataProvider
	defaultKey string
}

func NewMarketDataRegistry(providers ...MarketDataProvider) *MarketDataRegistry {
	registry := &MarketDataRegistry{providers: make(map[string]MarketDataProvider)}
	for _, provider := range providers {
		_ = registry.Register(provider)
	}
	return registry
}

// NewMarketDataRegistryWithDefault makes the production default explicit;
// registration order remains relevant only to the legacy test-friendly
// constructor above.
func NewMarketDataRegistryWithDefault(defaultKey string, providers ...MarketDataProvider) *MarketDataRegistry {
	registry := NewMarketDataRegistry(providers...)
	key := strings.ToLower(strings.TrimSpace(defaultKey))
	if _, ok := registry.providers[key]; ok {
		registry.defaultKey = key
	} else {
		registry.defaultKey = ""
	}
	return registry
}

func (r *MarketDataRegistry) Register(provider MarketDataProvider) error {
	if provider == nil {
		return &domain.Error{Code: domain.ErrValidation, Field: "provider", Message: "provider is required"}
	}
	key := strings.ToLower(strings.TrimSpace(provider.Key()))
	if key == "" {
		return &domain.Error{Code: domain.ErrValidation, Field: "providerKey", Message: "provider key is required"}
	}
	if r.providers == nil {
		r.providers = make(map[string]MarketDataProvider)
	}
	if _, exists := r.providers[key]; exists {
		return &domain.Error{Code: domain.ErrConflict, Field: "providerKey", Message: "provider key is already registered"}
	}
	r.providers[key] = provider
	if r.defaultKey == "" {
		r.defaultKey = key
	}
	return nil
}

func (r *MarketDataRegistry) Resolve(key string) (MarketDataProvider, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if provider, ok := r.providers[key]; ok {
		return provider, nil
	}
	return nil, &domain.Error{Code: domain.ErrUnavailable, Field: "provider", Message: "provider is not configured"}
}

func (r *MarketDataRegistry) Default() (MarketDataProvider, error) {
	if r.defaultKey == "" {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Field: "provider", Message: "provider is not configured"}
	}
	return r.Resolve(r.defaultKey)
}

// Providers returns a deterministic copy for settings and capability-aware
// UI surfaces. It exposes no provider-specific response or network detail.
func (r *MarketDataRegistry) Providers() []MarketDataProvider {
	providers := make([]MarketDataProvider, 0, len(r.providers))
	for _, provider := range r.providers {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool {
		return strings.ToLower(providers[i].Key()) < strings.ToLower(providers[j].Key())
	})
	return providers
}
