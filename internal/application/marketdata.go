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
// InstrumentProviderKeys so bindings can select Tiingo or Yahoo.
const TiingoProviderKey = "tiingo"

// WorkerProviderKey identifies the authenticated Cloudflare Worker proxy for
// Yahoo Finance. It is a separate selectable provider, not a replacement for
// the native Yahoo adapter.
const WorkerProviderKey = "worker"

// InstrumentProviderKeys is the closed catalog of provider keys that can
// bind an Instrument's quote source. Frankfurter is FX-only and is not
// included. Worker is an explicit proxy route and does not replace the native
// Yahoo or Tiingo adapters.
func InstrumentProviderKeys() []string {
	return []string{YahooFinanceProviderKey, TiingoProviderKey, WorkerProviderKey}
}

// MarketDataCapabilities describes the deliberately small provider surface.
// Search and history must be declared explicitly; adapters cannot imply them.
type MarketDataCapabilities struct {
	LatestInstrument       bool
	LatestFX               bool
	InstrumentSearch       bool
	DailyHistory           bool
	InstrumentDailyHistory bool
	FXDailyHistory         bool
}

func (c MarketDataCapabilities) SupportsInstrumentSearch() bool {
	return c.InstrumentSearch
}

// InstrumentSearchHit is a provider-normalized candidate for creating an
// Instrument. Yahoo-specific quoteType and exchange codes are mapped before
// this type crosses the application boundary.
type InstrumentSearchHit struct {
	ProviderKey    string
	ProviderSymbol string
	Name           string
	Symbol         string
	Type           string
	MarketCode     string
	CountryCode    string
	QuoteCurrency  string
	Exchange       string
}

// InstrumentSearchProvider is an optional search capability. It is not part of
// MarketDataProvider so latest-only fakes and adapters stay source compatible.
type InstrumentSearchProvider interface {
	SearchInstruments(ctx context.Context, query, instrumentType string, limit int) ([]InstrumentSearchHit, error)
}

// InstrumentMarketIdentity is the provider-neutral identity needed for a
// current instrument quote. Provider metadata is not exposed to valuation.
type InstrumentMarketIdentity struct {
	ProviderKey    string
	ProviderSymbol string
	QuoteCurrency  domain.CurrencyCode
	Market         string
	InstrumentType string
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

// ProviderLocalStatus is an optional adapter surface for Data Health.
// Implementations must inspect local configuration only and must not
// perform network I/O.
type ProviderLocalStatus interface {
	LocalConfigStatus(context.Context) (code, reason string)
}

const (
	ProviderConfigOK          = "ok"
	ProviderConfigMissingKey  = "missing_key"
	ProviderConfigUnavailable = "unavailable"
)

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

const instrumentSearchQueryMaxRunes = 80
const instrumentSearchLimit = 8

// SearchInstruments looks up stocks and ETFs through the native Yahoo Finance
// library. Other providers are not used for this form-assist path.
func (s *Service) SearchInstruments(ctx context.Context, query, instrumentType string) ([]InstrumentSearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "query", Message: "search query is required"}
	}
	if len([]rune(query)) > instrumentSearchQueryMaxRunes {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "query", Message: "search query is too long"}
	}
	parsedType, err := domain.ParseInstrumentType(instrumentType)
	if err != nil {
		return nil, err
	}
	if parsedType != domain.InstrumentStock && parsedType != domain.InstrumentETF {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "type", Message: "search is only available for stocks and ETFs"}
	}
	registry := s.MarketDataRegistry()
	if registry == nil {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Field: "provider", Message: "provider is not configured"}
	}
	provider, err := registry.Resolve(YahooFinanceProviderKey)
	if err != nil {
		return nil, err
	}
	if !provider.Capabilities().SupportsInstrumentSearch() {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Field: "provider", Message: "provider does not support instrument search"}
	}
	searcher, ok := provider.(InstrumentSearchProvider)
	if !ok {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Field: "provider", Message: "provider does not support instrument search"}
	}
	return searcher.SearchInstruments(ctx, query, string(parsedType), instrumentSearchLimit)
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
