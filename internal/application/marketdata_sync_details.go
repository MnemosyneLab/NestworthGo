package application

import (
	"context"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Both preview and execution use the same local latest-price eligibility rules.
func (s *Service) syncLatestTargets(ctx context.Context, request SyncRequest) ([]refreshTarget, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	var targets []refreshTarget
	if request.Scope != SyncScopeFX {
		for _, instrument := range snapshot.Instruments {
			if request.Scope == SyncScopeInstrument && instrument.ID.String() != request.InstrumentID {
				continue
			}
			target := instrumentRefreshTarget(instrument)
			if !target.skip {
				targets = append(targets, target)
			}
		}
	}
	if request.Scope != SyncScopeInstrument && snapshot.Household != nil {
		for _, pref := range snapshot.FXPreferences {
			if pref.SourceKind != domain.QuoteSourceProvider || (request.Scope == SyncScopeFX && !fxPreferenceMatches(pref, request)) {
				continue
			}
			targets = append(targets, refreshTarget{key: "fx:" + fxPairKey(pref.CurrencyA, pref.CurrencyB), kind: RefreshFXTarget, householdID: snapshot.Household.ID, baseCurrency: pref.CurrencyA, quoteCurrency: pref.CurrencyB, providerKey: s.FXProviderKey()})
		}
	}
	return s.applyQuoteCache(snapshot, targets, request.ForceRecheck), nil
}

func refreshDetail(target refreshTarget) SyncItemProgress {
	item := SyncItemProgress{TargetKey: target.key, Kind: "latest_" + string(target.kind), Provider: target.providerKey, Status: "planned", Detail: "missing_or_due"}
	if target.kind == RefreshInstrumentTarget {
		item.Label = target.instrument.Name
		item.Symbol = target.providerSymbol
	} else {
		item.Label = strings.TrimPrefix(target.key, "fx:")
	}
	if target.skip {
		item.Status = "cached"
		item.Detail = "recently_checked"
	}
	return item
}

func (s *Service) syncHistoryDetail(ctx context.Context, household domain.HouseholdID, target, provider string, rng DateRange) SyncItemProgress {
	item := SyncItemProgress{TargetKey: target, Provider: provider, Kind: "history_fx", Label: strings.TrimPrefix(target, "fx:"), Status: "planned", Detail: "missing_or_recheck", StartDate: string(rng.Start), EndDate: string(rng.End)}
	if strings.HasPrefix(target, "instrument:") {
		item.Kind = "history_instrument"
		id := domain.InstrumentID(strings.TrimPrefix(target, "instrument:"))
		if instrument, err := s.repository.Instrument(ctx, household, id); err == nil {
			item.Label = instrument.Name
			if instrument.ProviderSymbol != nil {
				item.Symbol = *instrument.ProviderSymbol
			}
		}
	}
	return item
}

func (s *Service) beginSyncItem(job *syncJobState, item SyncItemProgress) {
	item.Status = "running"
	s.publishSync(job, SyncEventProgress, func() { job.snapshot.Current = &item })
}

// Refresh progress is scoped to a request context, so concurrent operations and
// abandoned UI listeners cannot consume each other's events.
type refreshProgressKey struct{}

func WithRefreshProgress(ctx context.Context, callback func(SyncItemProgress)) context.Context {
	return context.WithValue(ctx, refreshProgressKey{}, callback)
}
func emitRefreshProgress(ctx context.Context, item SyncItemProgress) {
	if callback, ok := ctx.Value(refreshProgressKey{}).(func(SyncItemProgress)); ok {
		callback(item)
	}
}

// CoinGecko batches up to 100 distinct coin IDs for one quote currency.
func (s *Service) latestRequestEstimate(targets []refreshTarget) int {
	count := 0
	groups := map[string]map[string]bool{}
	registry := s.MarketDataRegistry()
	for _, target := range targets {
		if target.skip {
			continue
		}
		batchable := false
		if target.providerKey == CoinGeckoProviderKey && registry != nil {
			if provider, err := registry.Resolve(target.providerKey); err == nil {
				_, batchable = provider.(LatestInstrumentBatchProvider)
			}
		}
		if !batchable || target.kind != RefreshInstrumentTarget {
			count++
			continue
		}
		key := target.providerKey + ":" + target.instrument.QuoteCurrency.String()
		if groups[key] == nil {
			groups[key] = map[string]bool{}
		}
		groups[key][target.providerSymbol] = true
	}
	for _, ids := range groups {
		count += (len(ids) + 99) / 100
	}
	return count
}
