// Package marketdata contains the native provider adapters. Yahoo-specific
// response fields stay inside this package and are normalized before they
// reach the application or domain layers.
package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	yfinanceclient "github.com/wnjoon/go-yfinance/pkg/client"
	yfinancemodels "github.com/wnjoon/go-yfinance/pkg/models"
	yfinanceticker "github.com/wnjoon/go-yfinance/pkg/ticker"
)

const (
	yahooProviderKey    = "yahoo_finance"
	yahooRequestTimeout = 8 * time.Second
	yahooAdapterVersion = "go-yfinance-v1.7.0"
)

var sharedYahooSemaphore = make(chan struct{}, 2)

// YahooTicker is the small part of go-yfinance used by the application. It
// keeps the provider testable without replacing the upstream implementation.
type YahooTicker interface {
	Quote() (*yfinancemodels.Quote, error)
	History(yfinancemodels.HistoryParams) ([]yfinancemodels.Bar, error)
	GetHistoryMetadata() *yfinancemodels.ChartMeta
	Close()
}

// YahooTickerFactory creates one upstream ticker for one request.
type YahooTickerFactory func(symbol string) (YahooTicker, error)

type YahooChartProviderOptions struct {
	TickerFactory YahooTickerFactory
	Semaphore     chan struct{}
	Now           func() time.Time
}

type YahooChartProvider struct {
	factory   YahooTickerFactory
	semaphore chan struct{}
	now       func() time.Time
}

type managedYahooTicker struct {
	ticker *yfinanceticker.Ticker
	client *yfinanceclient.Client
}

func (t *managedYahooTicker) Quote() (*yfinancemodels.Quote, error) {
	return t.ticker.Quote()
}

func (t *managedYahooTicker) History(params yfinancemodels.HistoryParams) ([]yfinancemodels.Bar, error) {
	return t.ticker.History(params)
}

func (t *managedYahooTicker) GetHistoryMetadata() *yfinancemodels.ChartMeta {
	return t.ticker.GetHistoryMetadata()
}

func (t *managedYahooTicker) Close() {
	t.ticker.Close()
	t.client.Close()
}

func defaultYahooTickerFactory(symbol string) (YahooTicker, error) {
	client, err := yfinanceclient.New(yfinanceclient.WithTimeout(int(yahooRequestTimeout / time.Second)))
	if err != nil {
		return nil, err
	}
	ticker, err := yfinanceticker.New(symbol, yfinanceticker.WithClient(client))
	if err != nil {
		client.Close()
		return nil, err
	}
	return &managedYahooTicker{ticker: ticker, client: client}, nil
}

func NewYahooChartProvider() *YahooChartProvider {
	return NewYahooChartProviderWithOptions(YahooChartProviderOptions{})
}

func NewYahooChartProviderWithOptions(options YahooChartProviderOptions) *YahooChartProvider {
	factory := options.TickerFactory
	if factory == nil {
		factory = defaultYahooTickerFactory
	}
	semaphore := options.Semaphore
	if semaphore == nil {
		semaphore = sharedYahooSemaphore
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &YahooChartProvider{factory: factory, semaphore: semaphore, now: now}
}

func (p *YahooChartProvider) Key() string { return yahooProviderKey }

func (p *YahooChartProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestInstrument: true, InstrumentDailyHistory: true}
}

func (p *YahooChartProvider) LatestInstrument(ctx context.Context, identity application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	if strings.TrimSpace(identity.ProviderSymbol) == "" {
		return application.LatestInstrumentQuote{}, providerValidation("providerSymbol", "provider symbol is required")
	}
	currency, err := domain.ParseCurrency(identity.QuoteCurrency.String())
	if err != nil {
		return application.LatestInstrumentQuote{}, providerValidation("quoteCurrency", "quote currency is invalid")
	}

	var quote *yfinancemodels.Quote
	err = p.withTicker(ctx, identity.ProviderSymbol, func(ticker YahooTicker) error {
		quote, err = ticker.Quote()
		return err
	})
	if err != nil {
		return application.LatestInstrumentQuote{}, err
	}
	return normalizeYahooQuote(quote, identity.ProviderSymbol, currency, p.clock())
}

func (p *YahooChartProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	return application.LatestFXQuote{}, &domain.Error{Code: domain.ErrUnavailable, Message: "provider does not support FX refresh"}
}

func (p *YahooChartProvider) InstrumentDailyHistory(ctx context.Context, identity application.InstrumentMarketIdentity, rng application.DateRange) (application.MappingOutcome[application.InstrumentDailyObservation], error) {
	if strings.TrimSpace(identity.ProviderSymbol) == "" {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, providerValidation("providerSymbol", "provider symbol is required")
	}
	if _, err := domain.ParseCurrency(identity.QuoteCurrency.String()); err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, providerValidation("quoteCurrency", "quote currency is invalid")
	}
	if _, err := application.InclusiveMarketDates(rng); err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}

	start, err := domain.ParseMarketDate(string(rng.Start))
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	end, err := domain.ParseMarketDate(string(rng.End))
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	startAt, err := time.Parse("2006-01-02", start)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	endAt, err := time.Parse("2006-01-02", end)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	endExclusive := endAt.AddDate(0, 0, 1).UTC()

	var bars []yfinancemodels.Bar
	var metadata *yfinancemodels.ChartMeta
	err = p.withTicker(ctx, identity.ProviderSymbol, func(ticker YahooTicker) error {
		bars, err = ticker.History(yfinancemodels.HistoryParams{
			Interval:   "1d",
			Start:      &startAt,
			End:        &endExclusive,
			PrePost:    false,
			AutoAdjust: false,
			Actions:    false,
			Repair:     false,
			KeepNA:     false,
		})
		if err == nil {
			metadata = ticker.GetHistoryMetadata()
		}
		return err
	})
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}

	return qualifyYFinanceHistory(identity, rng, bars, metadata, p.clock())
}

func (p *YahooChartProvider) withTicker(ctx context.Context, symbol string, operation func(YahooTicker) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.semaphore != nil {
		select {
		case p.semaphore <- struct{}{}:
			defer func() { <-p.semaphore }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ticker, err := p.factory(symbol)
	if err != nil {
		return mapYahooLibraryError(err)
	}
	if ticker == nil {
		return malformedProvider()
	}
	defer ticker.Close()
	err = operation(ticker)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return mapYahooLibraryError(err)
}

func (p *YahooChartProvider) clock() time.Time {
	now := p.now()
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func normalizeYahooQuote(quote *yfinancemodels.Quote, expectedSymbol string, expectedCurrency domain.CurrencyCode, now time.Time) (application.LatestInstrumentQuote, error) {
	if quote == nil {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	if actual := strings.TrimSpace(quote.Symbol); actual != "" && !strings.EqualFold(actual, strings.TrimSpace(expectedSymbol)) {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	actualCurrency, err := domain.ParseCurrency(quote.Currency)
	if err != nil || actualCurrency != expectedCurrency {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	price, quotedAt, err := selectYahooQuote(quote)
	if err != nil {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	normalizedAt, err := application.NormalizeProviderObservationTime(quotedAt, now)
	if err != nil {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	return application.LatestInstrumentQuote{
		Price:     price,
		Currency:  expectedCurrency,
		SourceKey: yahooProviderKey,
		QuotedAt:  normalizedAt,
		Delayed:   false,
	}, nil
}

func selectYahooQuote(quote *yfinancemodels.Quote) (domain.UnitPrice, time.Time, error) {
	state := strings.ToUpper(strings.TrimSpace(quote.MarketState))
	var candidates []struct {
		price float64
		at    time.Time
	}
	switch state {
	case "PRE":
		candidates = append(candidates, struct {
			price float64
			at    time.Time
		}{quote.PreMarketPrice, quote.PreMarketTime})
	case "POST":
		candidates = append(candidates, struct {
			price float64
			at    time.Time
		}{quote.PostMarketPrice, quote.PostMarketTime})
	}
	candidates = append(candidates,
		struct {
			price float64
			at    time.Time
		}{quote.RegularMarketPrice, quote.RegularMarketTime},
		struct {
			price float64
			at    time.Time
		}{quote.PostMarketPrice, quote.PostMarketTime},
		struct {
			price float64
			at    time.Time
		}{quote.PreMarketPrice, quote.PreMarketTime},
	)
	for _, candidate := range candidates {
		if !isUsableYahooPrice(candidate.price) || candidate.at.IsZero() {
			continue
		}
		price, err := domain.ParseUnitPrice(yahooPriceLexeme(candidate.price))
		if err != nil {
			return domain.UnitPrice{}, time.Time{}, err
		}
		return price, candidate.at, nil
	}
	return domain.UnitPrice{}, time.Time{}, errors.New("Yahoo quote has no usable price")
}

func qualifyYFinanceHistory(identity application.InstrumentMarketIdentity, rng application.DateRange, bars []yfinancemodels.Bar, metadata *yfinancemodels.ChartMeta, now time.Time) (application.MappingOutcome[application.InstrumentDailyObservation], error) {
	meta := yfinanceHistoryRequestMeta(identity, rng, now)
	crypto := domain.InstrumentUsesCryptoDailyBar(identity.InstrumentType, identity.Market)
	if !meta.PriceBasisVerified || meta.PriceBasis != string(application.PriceBasisYahooClose) {
		return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingUnsupported, Reason: "unsupported_price_basis"}, nil
	}
	if !crypto {
		schedule, supported := domain.EquitySessionScheduleForMarket(identity.Market)
		if !supported || meta.SessionPolicy != schedule.Policy || meta.SessionKind != string(domain.SessionKindRegular) || meta.CloseClock != schedule.CloseClock {
			return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingUnsupported, Reason: "session_policy_unverified"}, nil
		}
	}
	if metadata == nil {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "malformed_response"), nil
	}
	expectedCurrency, err := domain.ParseCurrency(identity.QuoteCurrency.String())
	if err != nil {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "quote_currency_mismatch"), nil
	}
	if actual := strings.TrimSpace(metadata.Symbol); actual != "" && !strings.EqualFold(actual, strings.TrimSpace(identity.ProviderSymbol)) {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "provider_symbol_mismatch"), nil
	}
	actualCurrency, err := domain.ParseCurrency(metadata.Currency)
	if err != nil || actualCurrency != expectedCurrency {
		return invalidInstrumentOutcome(domain.HistoryCompleteness{Status: "invalid"}, "quote_currency_mismatch"), nil
	}

	timezone := strings.TrimSpace(metadata.ExchangeTimezoneName)
	if crypto {
		timezone = "UTC"
	} else {
		schedule, _ := domain.EquitySessionScheduleForMarket(identity.Market)
		if timezone == "" {
			return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingUncertain, Reason: "session_timezone_unknown"}, nil
		}
		if timezone != schedule.Timezone {
			return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingUncertain, Reason: "session_timezone_unknown"}, nil
		}
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingUncertain, Reason: "session_timezone_unknown"}, nil
	}

	completeness, err := completenessFor(meta, false)
	if err != nil {
		return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
	}
	start, _ := domain.ParseMarketDate(string(rng.Start))
	end, _ := domain.ParseMarketDate(string(rng.End))
	observations := make([]application.InstrumentDailyObservation, 0, len(bars))
	seenDates := map[string]struct{}{}
	for _, bar := range bars {
		if !isUsableYahooPrice(bar.Close) || bar.Date.IsZero() {
			continue
		}
		marketDate := bar.Date.In(location).Format("2006-01-02")
		if marketDate < start || marketDate > end {
			return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingUnsupported, Reason: "provider_date_outside_requested_range"}, nil
		}
		if _, duplicate := seenDates[marketDate]; duplicate {
			return invalidInstrumentOutcome(completeness, "duplicate_market_date"), nil
		}
		seenDates[marketDate] = struct{}{}
		if !dateInRanges(marketDate, completeness.VerifiedRanges) {
			continue
		}
		price, err := domain.ParseUnitPrice(yahooPriceLexeme(bar.Close))
		if err != nil {
			return invalidInstrumentOutcome(completeness, "malformed_close"), nil
		}
		observation := application.InstrumentDailyObservation{
			MarketDate:        application.MarketDate(marketDate),
			Value:             price.Canonical(),
			Currency:          expectedCurrency.String(),
			ProviderTimestamp: bar.Date.UTC(),
			Kind:              application.InstrumentObservationClose,
			PriceBasis:        application.PriceBasisYahooClose,
		}
		if crypto {
			observation.TimestampBasis = application.TimestampBasisPolicyDerived
			observation.ValueEffectiveAt, err = domain.CryptoDailyBarEligibleAt(marketDate)
		} else {
			schedule, _ := domain.EquitySessionScheduleForMarket(identity.Market)
			resolution, resolutionErr := domain.ResolveEquitySessionClose(marketDate, identity.Market, domain.SessionEvidence{
				Kind:       domain.SessionKindRegular,
				Timezone:   timezone,
				CloseClock: schedule.CloseClock,
				Policy:     schedule.Policy,
			})
			if resolutionErr != nil {
				return application.MappingOutcome[application.InstrumentDailyObservation]{}, resolutionErr
			}
			if resolution.Status != "mapped" {
				status := application.MappingUncertain
				if resolution.Status == "unsupported" {
					status = application.MappingUnsupported
				}
				return application.MappingOutcome[application.InstrumentDailyObservation]{Status: status, Reason: resolution.Reason}, nil
			}
			observation.TimestampBasis = application.TimestampBasisSessionClose
			observation.ValueEffectiveAt = resolution.CloseInstant
		}
		if err != nil {
			return application.MappingOutcome[application.InstrumentDailyObservation]{}, err
		}
		observations = append(observations, observation)
	}
	if len(observations) == 0 {
		return application.MappingOutcome[application.InstrumentDailyObservation]{Status: application.MappingUnsupported, Reason: "unsupported_price_basis"}, nil
	}
	status := mappingStatusFor(completeness, application.MappingMapped)
	timestampBasis := application.TimestampBasisSessionClose
	if crypto {
		timestampBasis = application.TimestampBasisPolicyDerived
	}
	return application.MappingOutcome[application.InstrumentDailyObservation]{
		Status: status,
		Reason: completeness.Reason,
		Batch: application.HistoryBatch[application.InstrumentDailyObservation]{
			Observations:    observations,
			VerifiedRanges:  toAppRanges(completeness.VerifiedRanges),
			PendingRanges:   toAppRanges(completeness.PendingRanges),
			UncertainRanges: toAppRanges(completeness.UncertainRanges),
			Evidence: application.ResponseEvidence{
				Adapter:         "go-yfinance",
				AdapterVersion:  yahooAdapterVersion,
				SourcePolicy:    string(application.PriceBasisYahooClose),
				RequestIdentity: "yahoo-finance-history",
				PriceBasis:      application.PriceBasisYahooClose,
				TimestampBasis:  timestampBasis,
				SessionPolicy:   meta.SessionPolicy,
			},
		},
	}, nil
}

func yfinanceHistoryRequestMeta(identity application.InstrumentMarketIdentity, rng application.DateRange, now time.Time) vnextFixtureMeta {
	meta := vnextFixtureMeta{
		FixtureID:          "yahoo-finance-history",
		Provider:           yahooProviderKey,
		Capability:         "InstrumentDailyHistory",
		ProviderSymbol:     identity.ProviderSymbol,
		InstrumentType:     identity.InstrumentType,
		QuoteCurrency:      identity.QuoteCurrency.String(),
		Market:             identity.Market,
		PriceBasis:         string(application.PriceBasisYahooClose),
		PriceBasisVerified: true,
		SessionKind:        string(domain.SessionKindRegular),
		Clock:              now.UTC().Format(time.RFC3339),
	}
	meta.RequestedRange.Start = string(rng.Start)
	meta.RequestedRange.End = string(rng.End)
	if domain.InstrumentUsesCryptoDailyBar(identity.InstrumentType, identity.Market) {
		meta.SessionPolicy = domain.YahooCryptoUTCDailyBarPolicy
		meta.SessionTimezone = "UTC"
		if finalized, err := domain.LastFinalizedCryptoMarketDate(now.UTC()); err == nil {
			meta.LastFinalizedMarketDate = finalized
		}
		return meta
	}
	if schedule, supported := domain.EquitySessionScheduleForMarket(identity.Market); supported {
		meta.SessionPolicy = schedule.Policy
		meta.SessionTimezone = schedule.Timezone
		meta.CloseClock = schedule.CloseClock
		if finalized, err := domain.LastFinalizedEquityMarketDate(now.UTC(), identity.Market); err == nil {
			meta.LastFinalizedMarketDate = finalized
		}
	}
	return meta
}

func isUsableYahooPrice(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// go-yfinance exposes Yahoo numeric fields as float64. Format them at the
// application's eight-decimal precision boundary before exact parsing so
// binary floating-point tails cannot be mistaken for malformed prices.
func yahooPriceLexeme(value float64) string {
	return strconv.FormatFloat(value, 'f', 8, 64)
}

func mapYahooLibraryError(err error) error {
	if err == nil {
		return nil
	}
	var chartError *yfinanceclient.ChartAPIError
	if errors.As(err, &chartError) {
		return unsupportedProviderSymbol()
	}
	switch {
	case yfinanceclient.IsRateLimitError(err):
		return providerError(domain.ErrProviderRateLimit, "provider rate limit reached")
	case yfinanceclient.IsAuthError(err):
		return providerError(domain.ErrProviderAuthentication, "provider authentication failed")
	case yfinanceclient.IsNotFoundError(err), yfinanceclient.IsInvalidSymbolError(err), yfinanceclient.IsNoDataError(err):
		return unsupportedProviderSymbol()
	case yfinanceclient.IsTimeoutError(err), errors.Is(err, yfinanceclient.ErrNetwork):
		return providerUnavailable("provider is unavailable")
	case errors.Is(err, yfinanceclient.ErrInvalidResponse):
		return malformedProvider()
	default:
		return providerUnavailable("provider is unavailable")
	}
}

// chartError is retained for the offline Yahoo fixture qualification tests.
// Production requests are decoded by go-yfinance.
type chartError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

func jsonNumberLexeme(raw json.RawMessage) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	number, ok := value.(json.Number)
	if !ok || strings.TrimSpace(number.String()) == "" {
		return "", errors.New("financial value is not a JSON number")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", errors.New("financial value contains trailing data")
	}
	return number.String(), nil
}

func isJSONNull(raw json.RawMessage) bool { return strings.TrimSpace(string(raw)) == "null" }

func providerValidation(field, message string) error {
	return &domain.Error{Code: domain.ErrValidation, Field: field, Message: message}
}

func providerError(code domain.ErrorCode, message string) error {
	return &domain.Error{Code: code, Message: message}
}

func providerUnavailable(message string) error {
	return providerError(domain.ErrProviderUnavailable, message)
}

func malformedProvider() error {
	return providerError(domain.ErrMalformedProviderResponse, "provider response is malformed")
}

func unsupportedProviderSymbol() error {
	return providerError(domain.ErrUnsupportedProviderSymbol, "provider symbol is unsupported")
}

var _ application.MarketDataProvider = (*YahooChartProvider)(nil)
var _ application.InstrumentHistoryProvider = (*YahooChartProvider)(nil)
