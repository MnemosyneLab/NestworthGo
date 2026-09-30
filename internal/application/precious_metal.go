package application

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type metalConversionEvidence struct {
	Policy      string     `json:"policy"`
	Symbol      string     `json:"symbol"`
	RawPrice    string     `json:"rawPrice"`
	RawCurrency string     `json:"rawCurrency"`
	RawUnit     string     `json:"rawUnit"`
	RawQuotedAt time.Time  `json:"rawQuotedAt"`
	FXRate      string     `json:"fxRate"`
	FXSource    string     `json:"fxSource,omitempty"`
	FXQuotedAt  *time.Time `json:"fxQuotedAt,omitempty"`
	Currency    string     `json:"currency"`
	Unit        string     `json:"unit"`
}

type metalCacheKey struct{}
type metalLatestResult struct {
	quote LatestInstrumentQuote
	err   error
}
type metalFetchCache struct {
	mu     sync.Mutex
	quotes map[string]metalLatestResult
}

func withMetalFetchCache(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Value(metalCacheKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, metalCacheKey{}, &metalFetchCache{quotes: map[string]metalLatestResult{}})
}

func metalEvidence(instrument domain.Instrument, raw domain.UnitPrice, rawAt time.Time, rate decimal.Decimal, fxSource string, fxAt *time.Time) string {
	evidence := metalConversionEvidence{Policy: domain.MetalConversionPolicy, Symbol: domain.MetalProviderSymbol(instrument.MetalTemplate), RawPrice: raw.Canonical(), RawCurrency: "USD", RawUnit: "troy_oz", RawQuotedAt: rawAt, FXRate: rate.String(), FXSource: fxSource, FXQuotedAt: fxAt, Currency: instrument.QuoteCurrency.String(), Unit: instrument.QuantityUnit}
	data, _ := json.Marshal(evidence)
	return string(data)
}

func (s *Service) latestMetalQuote(ctx context.Context, instrument domain.Instrument, provider MarketDataProvider, persistFX bool) (LatestInstrumentQuote, string, error) {
	identity := InstrumentMarketIdentity{ProviderKey: provider.Key(), ProviderSymbol: domain.MetalProviderSymbol(instrument.MetalTemplate), QuoteCurrency: "USD", Market: domain.MetalFuturesMarket, InstrumentType: string(instrument.Type)}
	var raw LatestInstrumentQuote
	var err error
	if cache, ok := ctx.Value(metalCacheKey{}).(*metalFetchCache); ok {
		cache.mu.Lock()
		key := provider.Key() + ":" + identity.ProviderSymbol
		result, exists := cache.quotes[key]
		if !exists {
			result.quote, result.err = provider.LatestInstrument(ctx, identity)
			cache.quotes[key] = result
		}
		cache.mu.Unlock()
		raw, err = result.quote, result.err
	} else {
		raw, err = provider.LatestInstrument(ctx, identity)
	}
	if err != nil {
		return LatestInstrumentQuote{}, "", err
	}
	if raw.Currency != "USD" || raw.SourceKey == "" {
		return LatestInstrumentQuote{}, "", malformedProviderError()
	}
	raw.QuotedAt, err = NormalizeProviderObservationTime(raw.QuotedAt, s.clock())
	if err != nil {
		return LatestInstrumentQuote{}, "", err
	}
	rate := decimal.NewFromInt(1)
	var fxAt *time.Time
	fxSource := ""
	delayed := raw.Delayed || s.clock().Sub(raw.QuotedAt) > 7*24*time.Hour
	quotedAt := raw.QuotedAt
	if instrument.QuoteCurrency != "USD" {
		preferences, prefErr := s.repository.ListFXPreferences(ctx, instrument.HouseholdID)
		if prefErr != nil {
			return LatestInstrumentQuote{}, "", prefErr
		}
		localOnly := false
		for _, preference := range preferences {
			a, b, _ := domain.NormalizeFXPair("USD", instrument.QuoteCurrency)
			if preference.CurrencyA == a && preference.CurrencyB == b {
				localOnly = preference.SourceKind != domain.QuoteSourceProvider
				break
			}
		}
		storedFX, storedErr := s.CurrentFXQuote(ctx, "USD", instrument.QuoteCurrency)
		if storedErr != nil {
			return LatestInstrumentQuote{}, "", storedErr
		}
		if storedFX != nil && storedFX.SourceKind == domain.QuoteSourceAgent && !storedFX.QuotedAt.After(s.clock()) && !domain.LatestRequestDue(storedFX.QuotedAt, s.clock(), s.QuoteCacheTTL(), false) {
			localOnly = true
		}
		if !localOnly && persistFX {
			result, refreshErr := s.RefreshFX(context.WithValue(ctx, metalFXDependencyKey{}, true), "USD", instrument.QuoteCurrency.String())
			if refreshErr != nil {
				return LatestInstrumentQuote{}, "", refreshErr
			}
			for _, item := range result.Items {
				if item.Status != RefreshFetched && item.Status != RefreshCached {
					return LatestInstrumentQuote{}, "", &domain.Error{Code: item.ErrorCode, Field: "fxRate", Message: "metal conversion exchange rate could not be refreshed"}
				}
			}
		}
		var fx *domain.FXQuote
		if !localOnly && !persistFX {
			registry := s.MarketDataRegistry()
			provider, resolveErr := registry.Resolve(s.FXProviderKey())
			if resolveErr != nil {
				return LatestInstrumentQuote{}, "", resolveErr
			}
			quote, fetchErr := provider.LatestFX(ctx, FXMarketIdentity{ProviderKey: provider.Key(), BaseCurrency: "USD", QuoteCurrency: instrument.QuoteCurrency})
			if fetchErr != nil {
				return LatestInstrumentQuote{}, "", fetchErr
			}
			at, timeErr := NormalizeProviderObservationTime(quote.QuotedAt, s.clock())
			if timeErr != nil || quote.BaseCurrency != "USD" || quote.QuoteCurrency != instrument.QuoteCurrency || quote.SourceKey == "" {
				return LatestInstrumentQuote{}, "", malformedProviderError()
			}
			fx = &domain.FXQuote{BaseCurrency: quote.BaseCurrency, QuoteCurrency: quote.QuoteCurrency, Rate: quote.Rate, QuotedAt: at, SourceKey: quote.SourceKey, Delayed: quote.Delayed}
		} else {
			fx = storedFX
			if !localOnly && persistFX {
				fx, err = s.CurrentFXQuote(ctx, "USD", instrument.QuoteCurrency)
				if err != nil {
					return LatestInstrumentQuote{}, "", err
				}
			}
		}
		if fx == nil {
			return LatestInstrumentQuote{}, "", &domain.Error{Code: domain.ErrUnavailable, Field: "fxRate", Message: "metal conversion requires a USD exchange rate"}
		}
		rate = fx.Rate.Decimal()
		if fx.BaseCurrency != "USD" {
			rate = decimal.NewFromInt(1).DivRound(rate, 24)
		}
		fxAt = &fx.QuotedAt
		fxSource = fx.SourceKey
		delayed = delayed || fx.Delayed || s.clock().Sub(fx.QuotedAt) > 7*24*time.Hour
		if fx.QuotedAt.After(quotedAt) {
			quotedAt = fx.QuotedAt
		}
	}
	price, err := domain.ConvertMetalPrice(raw.Price, instrument.QuantityUnit, rate)
	if err != nil {
		return LatestInstrumentQuote{}, "", err
	}
	return LatestInstrumentQuote{Price: price, Currency: instrument.QuoteCurrency, SourceKey: raw.SourceKey, QuotedAt: quotedAt, Delayed: delayed}, metalEvidence(instrument, raw.Price, raw.QuotedAt, rate, fxSource, fxAt), nil
}

// PreviewMetalQuote fetches a reference price without creating an instrument.
// It does not persist quotes or FX preferences; saving never depends on preview.
func (s *Service) PreviewMetalQuote(ctx context.Context, template, unit, currency string) (LatestInstrumentQuote, string, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return LatestInstrumentQuote{}, "", err
	}
	symbol := domain.MetalProviderSymbol(template)
	instrument, err := newInstrumentFromInput(household.ID, InstrumentInput{Name: template, Type: "precious_metal", MetalTemplate: template, QuantityUnit: unit, QuoteCurrency: currency, QuoteSource: "provider", ProviderKey: YahooFinanceProviderKey, ProviderSymbol: symbol, MarketCode: domain.MetalFuturesMarket}, s.clock())
	if err != nil {
		return LatestInstrumentQuote{}, "", err
	}
	registry := s.MarketDataRegistry()
	if registry == nil {
		return LatestInstrumentQuote{}, "", &domain.Error{Code: domain.ErrUnavailable, Message: "provider not configured"}
	}
	provider, err := registry.Resolve(YahooFinanceProviderKey)
	if err != nil {
		return LatestInstrumentQuote{}, "", err
	}
	return s.latestMetalQuote(withMetalFetchCache(ctx), instrument, provider, false)
}

// Convert historical quotes with a past-only FX reference. Missing dependencies
// fail the batch before any instrument price/coverage is committed.
func (s *Service) convertMetalHistory(ctx context.Context, job *syncJobState, instrument domain.Instrument, rng DateRange, outcome MappingOutcome[InstrumentDailyObservation], stopped map[string]string) (MappingOutcome[InstrumentDailyObservation], error) {
	var fxQuotes []domain.FXQuote
	if instrument.QuoteCurrency != "USD" {
		preferences, err := s.repository.ListFXPreferences(ctx, instrument.HouseholdID)
		if err != nil {
			return outcome, err
		}
		preferenceSource := domain.QuoteSourceProvider
		a, b, _ := domain.NormalizeFXPair("USD", instrument.QuoteCurrency)
		for _, pref := range preferences {
			if pref.CurrencyA == a && pref.CurrencyB == b {
				preferenceSource = pref.SourceKind
				break
			}
		}
		quotes, err := s.repository.ListFXQuotes(ctx, instrument.HouseholdID)
		if err != nil {
			return outcome, err
		}
		for _, q := range quotes {
			if q.SourceKind != domain.QuoteSourceProvider && quoteSourceAllowed(preferenceSource, q.SourceKind) && ((q.BaseCurrency == "USD" && q.QuoteCurrency == instrument.QuoteCurrency) || (q.QuoteCurrency == "USD" && q.BaseCurrency == instrument.QuoteCurrency)) {
				if q.SourceKind != domain.QuoteSourceAgent || q.ObservationKind == string(FXObservationDailyReference) {
					fxQuotes = append(fxQuotes, q)
				}
			}
		}
		localCoversAll := len(outcome.Batch.Observations) > 0
		for _, observation := range outcome.Batch.Observations {
			if metalHistoricalFXQuote(fxQuotes, observation) == nil {
				localCoversAll = false
				break
			}
		}
		if preferenceSource == domain.QuoteSourceProvider && !localCoversAll {
			provider, err := s.MarketDataRegistry().Resolve(s.FXProviderKey())
			if err != nil {
				return outcome, err
			}
			history, ok := provider.(FXHistoryProvider)
			if !ok {
				return outcome, &domain.Error{Code: domain.ErrUnavailable, Message: "FX history unavailable"}
			}
			start, err := time.Parse("2006-01-02", string(rng.Start))
			if err != nil {
				return outcome, err
			}
			fxIdentity := FXMarketIdentity{ProviderKey: provider.Key(), BaseCurrency: "USD", QuoteCurrency: instrument.QuoteCurrency}
			fxOutcome, err := s.fetchFXHistoryWithRetry(ctx, job, history, fxIdentity, DateRange{Start: MarketDate(start.AddDate(0, 0, -7).Format("2006-01-02")), End: rng.End}, provider.Key(), stopped)
			if err != nil {
				return outcome, err
			}
			if err := RefuseFailClosedFXHistory(fxOutcome); err != nil {
				return outcome, err
			}
			for _, observation := range fxOutcome.Batch.Observations {
				if observation.BaseCurrency != "USD" || observation.QuoteCurrency != instrument.QuoteCurrency.String() || observation.ValueEffectiveAt.IsZero() {
					continue
				}
				rate, err := domain.ParseFxRate(observation.Rate)
				if err != nil {
					return outcome, err
				}
				fxQuotes = append(fxQuotes, domain.FXQuote{BaseCurrency: "USD", QuoteCurrency: instrument.QuoteCurrency, Rate: rate, SourceKind: domain.QuoteSourceProvider, QuotedAt: observation.ValueEffectiveAt, EffectiveDate: string(observation.MarketDate), SourceKey: provider.Key()})
			}
		}
	}
	for index := range outcome.Batch.Observations {
		observation := &outcome.Batch.Observations[index]
		raw, err := domain.ParseUnitPrice(observation.Value)
		if err != nil {
			return outcome, err
		}
		if observation.Currency != "USD" {
			return outcome, malformedProviderError()
		}
		rate := decimal.NewFromInt(1)
		var at *time.Time
		source := ""
		if instrument.QuoteCurrency != "USD" {
			selected := metalHistoricalFXQuote(fxQuotes, *observation)
			if selected == nil {
				return outcome, &domain.Error{Code: domain.ErrUnavailable, Field: "fxRate", Message: "historical USD exchange rate missing"}
			}
			rate = selected.Rate.Decimal()
			if selected.BaseCurrency != "USD" {
				rate = decimal.NewFromInt(1).DivRound(rate, 24)
			}
			at = &selected.QuotedAt
			source = selected.SourceKey
		}
		converted, err := domain.ConvertMetalPrice(raw, instrument.QuantityUnit, rate)
		if err != nil {
			return outcome, err
		}
		observation.ConversionJSON = metalEvidence(instrument, raw, observation.ValueEffectiveAt, rate, source, at)
		observation.Value = converted.Canonical()
		observation.Currency = instrument.QuoteCurrency.String()
	}
	return outcome, nil
}

func metalHistoricalFXQuote(quotes []domain.FXQuote, observation InstrumentDailyObservation) *domain.FXQuote {
	var selected *domain.FXQuote
	for index := range quotes {
		quote := &quotes[index]
		exactAgent := quote.SourceKind == domain.QuoteSourceAgent && quote.ObservationKind == string(FXObservationDailyReference) && quote.EffectiveDate == string(observation.MarketDate)
		if quote.SourceKind == domain.QuoteSourceAgent && !exactAgent {
			continue
		}
		if !exactAgent && (quote.QuotedAt.After(observation.ValueEffectiveAt) || observation.ValueEffectiveAt.Sub(quote.QuotedAt) > 7*24*time.Hour) {
			continue
		}
		if selected == nil || (exactAgent && selected.SourceKind != domain.QuoteSourceAgent) || (exactAgent == (selected.SourceKind == domain.QuoteSourceAgent) && currentQuoteLater(quote.SourceKind, quote.QuotedAt, quote.CreatedAt, quote.ID.String(), selected.SourceKind, selected.QuotedAt, selected.CreatedAt, selected.ID.String())) {
			selected = quote
		}
	}
	return selected
}

type metalFXDependencyKey struct{}

// Revalue existing metal references after an FX-only update, without another
// Yahoo request. Historical close observations remain immutable; this appends
// a current derived observation with the original metal timestamp as evidence.
func (s *Service) repriceMetalsForFX(ctx context.Context, householdID domain.HouseholdID, base, currency domain.CurrencyCode) error {
	if ctx.Value(metalFXDependencyKey{}) == true {
		return nil
	}
	if base != "USD" && currency != "USD" {
		return nil
	}
	target := currency
	if currency == "USD" {
		target = base
	}
	instruments, err := s.repository.ListInstruments(ctx, householdID, false)
	if err != nil {
		return err
	}
	fx, err := s.CurrentFXQuote(ctx, "USD", target)
	if err != nil {
		return err
	}
	if fx == nil {
		return nil
	}
	rate := fx.Rate.Decimal()
	if fx.BaseCurrency != "USD" {
		rate = decimal.NewFromInt(1).DivRound(rate, 24)
	}
	epoch := s.refreshEpoch.Load()
	for _, instrument := range instruments {
		if !instrument.UsesMetalConversion() || instrument.QuoteCurrency != target || instrument.QuoteSource != domain.QuoteSourceProvider {
			continue
		}
		quotes, err := s.repository.ListInstrumentQuotes(ctx, instrument.ID)
		if err != nil {
			return err
		}
		var evidence metalConversionEvidence
		found := false
		for _, quote := range quotes {
			if quote.SourceKind != domain.QuoteSourceProvider || quote.ConversionJSON == "" {
				continue
			}
			if err := json.Unmarshal([]byte(quote.ConversionJSON), &evidence); err != nil {
				continue
			}
			if evidence.Policy != domain.MetalConversionPolicy || evidence.Symbol != domain.MetalProviderSymbol(instrument.MetalTemplate) {
				continue
			}
			found = true
			break
		}
		if !found {
			continue
		}
		raw, err := domain.ParseUnitPrice(evidence.RawPrice)
		if err != nil {
			return err
		}
		price, err := domain.ConvertMetalPrice(raw, instrument.QuantityUnit, rate)
		if err != nil {
			return err
		}
		at := evidence.RawQuotedAt
		if fx.QuotedAt.After(at) {
			at = fx.QuotedAt
		}
		quote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, Currency: target, SourceKind: domain.QuoteSourceProvider, SourceKey: YahooFinanceProviderKey, QuotedAt: at, Delayed: true}, s.clock())
		if err != nil {
			return err
		}
		quote.ConversionJSON = metalEvidence(instrument, raw, evidence.RawQuotedAt, rate, fx.SourceKey, &fx.QuotedAt)
		if err := s.persistRefreshWrite(ctx, epoch, func(ctx context.Context) error {
			_, err := s.repository.AppendProviderInstrumentQuoteIfChanged(ctx, quote)
			return err
		}); err != nil {
			return err
		}
		s.invalidateAnalysis()
	}
	return nil
}
