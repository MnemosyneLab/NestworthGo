package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type RefreshStatus string

const (
	RefreshFetched     RefreshStatus = "fetched"
	RefreshCached      RefreshStatus = "cached"
	RefreshSkipped     RefreshStatus = "skipped"
	RefreshFailed      RefreshStatus = "failed"
	RefreshRateLimited RefreshStatus = "rate_limited"
)

type RefreshTargetKind string

const (
	RefreshInstrumentTarget RefreshTargetKind = "instrument"
	RefreshFXTarget         RefreshTargetKind = "fx"
)

// RefreshTargetResult is deliberately a safe status vocabulary. It contains
// no provider message, URL, symbol, quote value, or financial context.
type RefreshTargetResult struct {
	TargetKey string
	Kind      RefreshTargetKind
	Status    RefreshStatus
	ErrorCode domain.ErrorCode
}

type RefreshResult struct {
	Items       []RefreshTargetResult
	RateLimited bool
}

type refreshTarget struct {
	key            string
	kind           RefreshTargetKind
	instrument     domain.Instrument
	baseCurrency   domain.CurrencyCode
	quoteCurrency  domain.CurrencyCode
	householdID    domain.HouseholdID
	providerKey    string
	providerSymbol string
	skip           bool
	cached         bool
	skipCode       domain.ErrorCode
}

// RefreshInstrument fetches one active or archived instrument only when its
// saved source is a complete Provider binding. Archived, manual, and
// incomplete bindings are reported as skipped without a network request.
func (s *Service) RefreshInstrument(ctx context.Context, id domain.InstrumentID) (RefreshResult, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return RefreshResult{}, err
	}
	for _, instrument := range snapshot.Instruments {
		if instrument.ID == id {
			return s.refreshTargets(ctx, []refreshTarget{instrumentRefreshTarget(instrument)}), nil
		}
	}
	return RefreshResult{Items: []RefreshTargetResult{{TargetKey: instrumentTargetKey(id), Kind: RefreshInstrumentTarget, Status: RefreshSkipped, ErrorCode: domain.ErrNotFound}}}, nil
}

// RefreshFX refreshes one selected provider-backed pair directly. Either
// input orientation is accepted; the canonical pair is used for preference
// lookup and result identity, while the provider receives the requested
// orientation. The pair does not need to include the Household base currency.
func (s *Service) RefreshFX(ctx context.Context, currencyA, currencyB string) (RefreshResult, error) {
	a, err := domain.ParseSupportedCurrency(currencyA)
	if err != nil {
		return refreshInputFailure(fxTargetKeyFromStrings(currencyA, currencyB), RefreshFXTarget, err), nil
	}
	b, err := domain.ParseSupportedCurrency(currencyB)
	if err != nil {
		return refreshInputFailure(fxTargetKeyFromStrings(currencyA, currencyB), RefreshFXTarget, err), nil
	}
	normalizedA, normalizedB, err := domain.NormalizeFXPair(a, b)
	if err != nil {
		return refreshInputFailure(fxTargetKeyFromStrings(currencyA, currencyB), RefreshFXTarget, err), nil
	}
	key := fxPairKey(normalizedA, normalizedB)
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return RefreshResult{}, err
	}
	if snapshot.Household == nil {
		return RefreshResult{Items: []RefreshTargetResult{{TargetKey: "fx:" + key, Kind: RefreshFXTarget, Status: RefreshSkipped, ErrorCode: domain.ErrUnavailable}}}, nil
	}
	preferenceFound := false
	providerPreference := false
	for _, preference := range snapshot.FXPreferences {
		if fxPairKey(preference.CurrencyA, preference.CurrencyB) != key {
			continue
		}
		preferenceFound = true
		providerPreference = preference.SourceKind == domain.QuoteSourceProvider
		break
	}
	target := refreshTarget{key: "fx:" + key, kind: RefreshFXTarget, baseCurrency: a, quoteCurrency: b, householdID: snapshot.Household.ID}
	if preferenceFound && !providerPreference {
		target.skip = true
		target.skipCode = domain.ErrUnavailable
	}
	return s.refreshTargets(ctx, []refreshTarget{target}), nil
}

// RefreshAll derives a deterministic set from the one local snapshot. It
// never contacts a provider while reading or writing SQLite, and stops
// starting later targets for the provider that returned a rate limit.
func (s *Service) RefreshAll(ctx context.Context) (RefreshResult, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return RefreshResult{}, err
	}
	targets := refreshTargetsForSnapshot(snapshot)
	targets = append(targets, extraFXRefreshTargetsForSnapshot(snapshot)...)
	return s.refreshTargets(ctx, targets), nil
}

func (s *Service) RefreshMissingOrStale(ctx context.Context) (RefreshResult, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return RefreshResult{}, err
	}
	targets := s.applyQuoteCache(snapshot, refreshTargetsForSnapshot(snapshot), false)
	return s.refreshTargets(ctx, targets), nil
}

// RefreshRequiredFX refreshes only the FX targets required by the current
// local portfolio snapshot. It is intentionally separate from RefreshAll so
// the UI can offer a clear, explicit FX-only action.
func (s *Service) RefreshRequiredFX(ctx context.Context) (RefreshResult, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return RefreshResult{}, err
	}
	targets := refreshTargetsForSnapshot(snapshot)
	fxTargets := make([]refreshTarget, 0, len(targets))
	for _, target := range targets {
		if target.kind == RefreshFXTarget {
			fxTargets = append(fxTargets, target)
		}
	}
	return s.refreshTargets(ctx, fxTargets), nil
}

func (s *Service) refreshTargets(ctx context.Context, targets []refreshTarget) RefreshResult {
	result := RefreshResult{Items: make([]RefreshTargetResult, 0, len(targets))}
	rateLimited := make(map[string]bool)
	for _, target := range targets {
		if target.skip {
			item, _ := s.refreshTarget(ctx, target, marketDataSnapshot{})
			result.Items = append(result.Items, item)
			continue
		}
		marketData := s.marketDataSnapshot(target)
		providerKey := refreshProviderKey(target, marketData)
		if providerKey != "" && rateLimited[providerKey] {
			result.Items = append(result.Items, RefreshTargetResult{TargetKey: target.key, Kind: target.kind, Status: RefreshSkipped, ErrorCode: domain.ErrProviderRateLimit})
			continue
		}
		item, hitRateLimit := s.refreshTarget(ctx, target, marketData)
		result.Items = append(result.Items, item)
		if hitRateLimit {
			if providerKey != "" {
				rateLimited[providerKey] = true
			}
			result.RateLimited = true
		}
	}
	return result
}

type marketDataSnapshot struct {
	registry      MarketDataRegistryPort
	fxProviderKey string
	defaultErr    error
}

// marketDataSnapshot copies the service routing state once. Registry methods
// run after the state lock is released because a registry may perform work of
// its own; the refresh then consistently uses this copied registry and key.
func (s *Service) marketDataSnapshot(target refreshTarget) marketDataSnapshot {
	s.stateMu.RLock()
	registry, key := s.marketData, s.fxProviderKey
	s.stateMu.RUnlock()

	key = strings.ToLower(strings.TrimSpace(key))
	if target.kind == RefreshFXTarget && key == "" && registry != nil {
		provider, err := registry.Default()
		if err != nil {
			return marketDataSnapshot{registry: registry, defaultErr: err}
		}
		key = strings.ToLower(strings.TrimSpace(provider.Key()))
	}
	return marketDataSnapshot{registry: registry, fxProviderKey: key}
}

func refreshProviderKey(target refreshTarget, marketData marketDataSnapshot) string {
	if target.kind == RefreshFXTarget {
		return marketData.fxProviderKey
	}
	return strings.ToLower(strings.TrimSpace(target.providerKey))
}

func (s *Service) refreshTarget(ctx context.Context, target refreshTarget, marketData marketDataSnapshot) (RefreshTargetResult, bool) {
	if target.skip {
		if target.cached {
			return cachedRefresh(target), false
		}
		return RefreshTargetResult{TargetKey: target.key, Kind: target.kind, Status: RefreshSkipped, ErrorCode: target.skipCode}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var provider MarketDataProvider
	var err error
	if marketData.registry == nil {
		err = &domain.Error{Code: domain.ErrUnavailable, Field: "provider", Message: "provider is not configured"}
	} else if target.kind == RefreshFXTarget {
		if marketData.fxProviderKey == "" {
			if marketData.defaultErr != nil {
				err = marketData.defaultErr
			} else {
				err = &domain.Error{Code: domain.ErrUnavailable, Field: "provider", Message: "provider is not configured"}
			}
		} else {
			provider, err = marketData.registry.Resolve(marketData.fxProviderKey)
		}
	} else {
		provider, err = marketData.registry.Resolve(target.providerKey)
	}
	if err != nil {
		return failedRefresh(target, err), false
	}
	capabilities := provider.Capabilities()
	if target.kind == RefreshInstrumentTarget && !capabilities.LatestInstrument {
		return skippedRefresh(target, domain.ErrUnavailable), false
	}
	if target.kind == RefreshFXTarget && !capabilities.LatestFX {
		return skippedRefresh(target, domain.ErrUnavailable), false
	}

	if target.kind == RefreshInstrumentTarget {
		quote, providerErr := provider.LatestInstrument(ctx, InstrumentMarketIdentity{ProviderKey: target.providerKey, ProviderSymbol: target.providerSymbol, QuoteCurrency: target.instrument.QuoteCurrency})
		if providerErr != nil {
			return providerRefreshFailure(target, providerErr)
		}
		quotedAt, timeErr := NormalizeProviderObservationTime(quote.QuotedAt, s.clock())
		if timeErr != nil || quote.Currency != target.instrument.QuoteCurrency || quote.SourceKey == "" {
			return failedRefresh(target, malformedProviderError()), false
		}
		stored, createErr := domain.NewInstrumentQuote(target.instrument, domain.InstrumentQuoteInput{UnitPrice: quote.Price, Currency: quote.Currency, SourceKind: domain.QuoteSourceProvider, SourceKey: quote.SourceKey, QuotedAt: quotedAt, Delayed: quote.Delayed}, s.clock())
		if createErr != nil {
			return failedRefresh(target, malformedProviderError()), false
		}
		inserted, persistErr := s.repository.AppendProviderInstrumentQuoteIfChanged(ctx, stored)
		if persistErr != nil {
			return failedRefresh(target, persistErr), false
		}
		if inserted {
			return fetchedRefresh(target), false
		}
		return cachedRefresh(target), false
	}

	quote, providerErr := provider.LatestFX(ctx, FXMarketIdentity{ProviderKey: provider.Key(), BaseCurrency: target.baseCurrency, QuoteCurrency: target.quoteCurrency})
	if providerErr != nil {
		return providerRefreshFailure(target, providerErr)
	}
	quotedAt, timeErr := NormalizeProviderObservationTime(quote.QuotedAt, s.clock())
	if timeErr != nil || quote.BaseCurrency != target.baseCurrency || quote.QuoteCurrency != target.quoteCurrency || quote.SourceKey == "" {
		return failedRefresh(target, malformedProviderError()), false
	}
	if target.householdID == "" {
		return failedRefresh(target, &domain.Error{Code: domain.ErrUnavailable, Message: "household was not found"}), false
	}
	stored, createErr := domain.NewFXQuote(domain.FXQuoteInput{HouseholdID: target.householdID, BaseCurrency: quote.BaseCurrency, QuoteCurrency: quote.QuoteCurrency, Rate: quote.Rate, SourceKind: domain.QuoteSourceProvider, SourceKey: quote.SourceKey, QuotedAt: quotedAt, Delayed: quote.Delayed}, s.clock())
	if createErr != nil {
		return failedRefresh(target, malformedProviderError()), false
	}
	inserted, persistErr := s.repository.AppendProviderFXQuoteIfChanged(ctx, stored)
	if persistErr != nil {
		return failedRefresh(target, persistErr), false
	}
	s.rememberProviderFXPreference(ctx, target)
	if inserted {
		return fetchedRefresh(target), false
	}
	return cachedRefresh(target), false
}

func refreshTargetsForSnapshot(snapshot domain.PortfolioSnapshot) []refreshTarget {
	if snapshot.Household == nil {
		return []refreshTarget{}
	}
	accounts := make(map[domain.AccountID]domain.AccountRecord, len(snapshot.Accounts))
	for _, record := range snapshot.Accounts {
		accounts[record.Account.ID] = record
	}
	instruments := make(map[domain.InstrumentID]domain.Instrument, len(snapshot.Instruments))
	targetsByKey := make(map[string]refreshTarget, len(snapshot.Instruments))
	for _, instrument := range snapshot.Instruments {
		instruments[instrument.ID] = instrument
		target := instrumentRefreshTarget(instrument)
		targetsByKey[target.key] = target
	}
	requiredFX := make(map[string]refreshTarget)
	addRequiredFX := func(currency domain.CurrencyCode) {
		if currency == "" || currency == snapshot.Household.BaseCurrency {
			return
		}
		a, b, err := domain.NormalizeFXPair(currency, snapshot.Household.BaseCurrency)
		if err != nil {
			return
		}
		key := fxPairKey(a, b)
		requiredFX[key] = refreshTarget{key: "fx:" + key, kind: RefreshFXTarget, baseCurrency: currency, quoteCurrency: snapshot.Household.BaseCurrency, householdID: snapshot.Household.ID}
	}
	for _, record := range snapshot.Accounts {
		if record.Account.ArchivedAt != nil || record.LatestValue == nil {
			continue
		}
		addRequiredFX(record.LatestValue.Amount.Currency())
	}
	for _, holding := range snapshot.Holdings {
		if holding.ArchivedAt != nil {
			continue
		}
		account, ok := accounts[holding.AccountID]
		if !ok || account.Account.ArchivedAt != nil || account.Account.TrackingMode != domain.TrackingHoldings {
			continue
		}
		instrument, ok := instruments[holding.InstrumentID]
		if ok && instrument.ArchivedAt == nil {
			addRequiredFX(instrument.QuoteCurrency)
		}
	}
	for _, cash := range snapshot.CashValues {
		account, ok := accounts[cash.AccountID]
		if ok && account.Account.ArchivedAt == nil && account.Account.TrackingMode == domain.TrackingHoldings {
			addRequiredFX(cash.Amount.Currency())
		}
	}
	preferences := make(map[string]domain.FXPreference, len(snapshot.FXPreferences))
	for _, preference := range snapshot.FXPreferences {
		preferences[fxPairKey(preference.CurrencyA, preference.CurrencyB)] = preference
	}
	for key, target := range requiredFX {
		preference, ok := preferences[key]
		if ok && preference.SourceKind != domain.QuoteSourceProvider {
			target.skip = true
			target.skipCode = domain.ErrUnavailable
		}
		targetsByKey[target.key] = target
	}
	targets := make([]refreshTarget, 0, len(targetsByKey))
	for _, target := range targetsByKey {
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].key < targets[j].key })
	return targets
}

func (s *Service) applyQuoteCache(snapshot domain.PortfolioSnapshot, targets []refreshTarget, force bool) []refreshTarget {
	if force {
		return targets
	}
	ttl := s.QuoteCacheTTL()
	now := s.clock()
	for index := range targets {
		if targets[index].skip {
			continue
		}
		quotedAt := latestProviderQuotedAt(snapshot, targets[index])
		if quotedAt.IsZero() {
			continue
		}
		if now.Sub(quotedAt) < ttl {
			targets[index].skip = true
			targets[index].cached = true
		}
	}
	return targets
}

func latestProviderQuotedAt(snapshot domain.PortfolioSnapshot, target refreshTarget) time.Time {
	var selected time.Time
	if target.kind == RefreshInstrumentTarget {
		for _, quote := range snapshot.InstrumentQuotes {
			if quote.InstrumentID != target.instrument.ID || quote.SourceKind != domain.QuoteSourceProvider {
				continue
			}
			if selected.IsZero() || quote.QuotedAt.After(selected) {
				selected = quote.QuotedAt
			}
		}
		return selected
	}
	for _, quote := range snapshot.FXQuotes {
		if quote.SourceKind != domain.QuoteSourceProvider {
			continue
		}
		if !((quote.BaseCurrency == target.baseCurrency && quote.QuoteCurrency == target.quoteCurrency) || (quote.BaseCurrency == target.quoteCurrency && quote.QuoteCurrency == target.baseCurrency)) {
			continue
		}
		if selected.IsZero() || quote.QuotedAt.After(selected) {
			selected = quote.QuotedAt
		}
	}
	return selected
}

func (s *Service) rememberProviderFXPreference(ctx context.Context, target refreshTarget) {
	if target.householdID == "" || target.kind != RefreshFXTarget {
		return
	}
	preferences, err := s.repository.ListFXPreferences(ctx, target.householdID)
	if err != nil {
		return
	}
	if findFXPreference(preferences, target.baseCurrency, target.quoteCurrency) != nil {
		return
	}
	_, _ = s.SetFXPreference(ctx, target.baseCurrency.String(), target.quoteCurrency.String(), string(domain.QuoteSourceProvider))
}

// extraFXRefreshTargetsForSnapshot adds provider-backed preferences that are
// not currently needed to value an account or holding. Required valuation FX
// targets remain in the first group returned by refreshTargetsForSnapshot, so
// a provider rate limit on an optional pair can never prevent a required pair
// from being attempted.
func extraFXRefreshTargetsForSnapshot(snapshot domain.PortfolioSnapshot) []refreshTarget {
	if snapshot.Household == nil {
		return []refreshTarget{}
	}
	required := make(map[string]struct{})
	for _, target := range refreshTargetsForSnapshot(snapshot) {
		if target.kind == RefreshFXTarget {
			required[target.key] = struct{}{}
		}
	}
	extras := make([]refreshTarget, 0)
	seen := make(map[string]struct{}, len(required))
	for key := range required {
		seen[key] = struct{}{}
	}
	for _, preference := range snapshot.FXPreferences {
		if preference.SourceKind != domain.QuoteSourceProvider {
			continue
		}
		a, b, err := domain.NormalizeFXPair(preference.CurrencyA, preference.CurrencyB)
		if err != nil {
			continue
		}
		key := "fx:" + fxPairKey(a, b)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		extras = append(extras, refreshTarget{key: key, kind: RefreshFXTarget, baseCurrency: a, quoteCurrency: b, householdID: snapshot.Household.ID})
	}
	sort.Slice(extras, func(i, j int) bool { return extras[i].key < extras[j].key })
	return extras
}

func instrumentRefreshTarget(instrument domain.Instrument) refreshTarget {
	target := refreshTarget{key: instrumentTargetKey(instrument.ID), kind: RefreshInstrumentTarget, instrument: instrument}
	if instrument.ArchivedAt != nil || instrument.QuoteSource != domain.QuoteSourceProvider {
		target.skip = true
		target.skipCode = domain.ErrUnavailable
		return target
	}
	if instrument.ProviderKey == nil || instrument.ProviderSymbol == nil || strings.TrimSpace(*instrument.ProviderKey) == "" || strings.TrimSpace(*instrument.ProviderSymbol) == "" {
		target.skip = true
		target.skipCode = domain.ErrValidation
		return target
	}
	target.providerKey = strings.TrimSpace(*instrument.ProviderKey)
	target.providerSymbol = strings.TrimSpace(*instrument.ProviderSymbol)
	return target
}

func instrumentTargetKey(id domain.InstrumentID) string { return "instrument:" + id.String() }

func fxTargetKeyFromStrings(first, second string) string {
	return "fx:" + strings.ToUpper(strings.TrimSpace(first)) + "/" + strings.ToUpper(strings.TrimSpace(second))
}

func fxPairKey(first, second domain.CurrencyCode) string {
	a, b, err := domain.NormalizeFXPair(first, second)
	if err != nil {
		return first.String() + "/" + second.String()
	}
	return a.String() + "/" + b.String()
}

func fetchedRefresh(target refreshTarget) RefreshTargetResult {
	return RefreshTargetResult{TargetKey: target.key, Kind: target.kind, Status: RefreshFetched}
}

func cachedRefresh(target refreshTarget) RefreshTargetResult {
	return RefreshTargetResult{TargetKey: target.key, Kind: target.kind, Status: RefreshCached}
}

func skippedRefresh(target refreshTarget, code domain.ErrorCode) RefreshTargetResult {
	return RefreshTargetResult{TargetKey: target.key, Kind: target.kind, Status: RefreshSkipped, ErrorCode: code}
}

func failedRefresh(target refreshTarget, err error) RefreshTargetResult {
	return RefreshTargetResult{TargetKey: target.key, Kind: target.kind, Status: RefreshFailed, ErrorCode: refreshErrorCode(err)}
}

func providerRefreshFailure(target refreshTarget, err error) (RefreshTargetResult, bool) {
	code := refreshErrorCode(err)
	if code == domain.ErrProviderRateLimit {
		return RefreshTargetResult{TargetKey: target.key, Kind: target.kind, Status: RefreshRateLimited, ErrorCode: code}, true
	}
	return RefreshTargetResult{TargetKey: target.key, Kind: target.kind, Status: RefreshFailed, ErrorCode: code}, false
}

func refreshInputFailure(key string, kind RefreshTargetKind, err error) RefreshResult {
	return RefreshResult{Items: []RefreshTargetResult{{TargetKey: key, Kind: kind, Status: RefreshFailed, ErrorCode: refreshErrorCode(err)}}}
}

func refreshErrorCode(err error) domain.ErrorCode {
	if err == nil {
		return ""
	}
	var domainErr *domain.Error
	if errors.As(err, &domainErr) && domainErr != nil && domainErr.Code != "" {
		return domainErr.Code
	}
	return domain.ErrUnavailable
}

func malformedProviderError() error {
	return &domain.Error{Code: domain.ErrMalformedProviderResponse, Message: "provider response is malformed"}
}
