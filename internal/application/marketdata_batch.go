package application

import (
	"context"
)

// Optional batch capability preserves the per-instrument validation/persistence path.
type LatestInstrumentBatchProvider interface {
	LatestInstruments(context.Context, []InstrumentMarketIdentity) map[string]LatestInstrumentResult
}
type LatestInstrumentResult struct {
	Quote LatestInstrumentQuote
	Err   error
}
type latestBatchContextKey struct{}
type latestBatchState struct {
	epoch    uint64
	registry MarketDataRegistryPort
	results  map[string]LatestInstrumentResult
}

func LatestInstrumentIdentityKey(identity InstrumentMarketIdentity) string {
	return identity.ProviderKey + ":" + identity.ProviderSymbol + ":" + identity.QuoteCurrency.String()
}
func (s *Service) withLatestBatches(ctx context.Context, targets []refreshTarget) context.Context {
	if ctx.Value(latestBatchContextKey{}) != nil {
		return ctx
	}
	epoch := s.refreshEpoch.Load()
	results := map[string]LatestInstrumentResult{}
	registry := s.MarketDataRegistry()
	if registry == nil {
		return ctx
	}
	groups := map[string][]InstrumentMarketIdentity{}
	for _, target := range targets {
		if target.skip || target.kind != RefreshInstrumentTarget || target.providerKey != CoinGeckoProviderKey || target.instrument.UsesMetalConversion() {
			continue
		}
		identity := InstrumentMarketIdentity{ProviderKey: target.providerKey, ProviderSymbol: target.providerSymbol, QuoteCurrency: target.instrument.QuoteCurrency, Market: instrumentMarket(target.instrument), InstrumentType: string(target.instrument.Type)}
		groups[target.providerKey] = append(groups[target.providerKey], identity)
	}
	for key, ids := range groups {
		provider, err := registry.Resolve(key)
		if err != nil {
			continue
		}
		batch, ok := provider.(LatestInstrumentBatchProvider)
		if !ok {
			continue
		}
		for key, value := range batch.LatestInstruments(ctx, ids) {
			results[key] = value
		}
	}
	return context.WithValue(ctx, latestBatchContextKey{}, latestBatchState{epoch: epoch, registry: registry, results: results})
}
func latestInstrumentQuote(ctx context.Context, provider MarketDataProvider, identity InstrumentMarketIdentity) (LatestInstrumentQuote, error) {
	if batch, ok := ctx.Value(latestBatchContextKey{}).(latestBatchState); ok {
		if result, exists := batch.results[LatestInstrumentIdentityKey(identity)]; exists {
			return result.Quote, result.Err
		}
	}
	return provider.LatestInstrument(ctx, identity)
}
