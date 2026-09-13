package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	frankfurterProviderKey    = "frankfurter"
	frankfurterHost           = "api.frankfurter.dev"
	frankfurterRequestTimeout = 8 * time.Second
	frankfurterMaxBodyBytes   = int64(2 * 1024 * 1024)
)

var sharedFrankfurterSemaphore = make(chan struct{}, 2)

type FrankfurterProviderOptions struct {
	Transport   http.RoundTripper
	Timeout     time.Duration
	MaxBodySize int64
	Semaphore   chan struct{}
}

type FrankfurterProvider struct {
	conn *providerHTTPClient
}

func NewFrankfurterProvider(transport http.RoundTripper) *FrankfurterProvider {
	return NewFrankfurterProviderWithOptions(FrankfurterProviderOptions{Transport: transport})
}

func NewFrankfurterProviderWithOptions(options FrankfurterProviderOptions) *FrankfurterProvider {
	return &FrankfurterProvider{
		conn: newProviderHTTPClient(providerHTTPOptions(options), frankfurterRequestTimeout, frankfurterMaxBodyBytes, sharedFrankfurterSemaphore),
	}
}

func (p *FrankfurterProvider) Key() string { return frankfurterProviderKey }

func (p *FrankfurterProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestFX: true, FXDailyHistory: true}
}

func (p *FrankfurterProvider) LatestInstrument(context.Context, application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	return application.LatestInstrumentQuote{}, providerError(domain.ErrUnavailable, "provider does not support instrument refresh")
}

func (p *FrankfurterProvider) LatestFX(ctx context.Context, identity application.FXMarketIdentity) (application.LatestFXQuote, error) {
	base, err := domain.ParseCurrency(identity.BaseCurrency.String())
	if err != nil {
		return application.LatestFXQuote{}, providerValidation("baseCurrency", "base currency is invalid")
	}
	quote, err := domain.ParseCurrency(identity.QuoteCurrency.String())
	if err != nil {
		return application.LatestFXQuote{}, providerValidation("quoteCurrency", "quote currency is invalid")
	}
	if base == quote {
		return application.LatestFXQuote{}, providerValidation("currencyPair", "base and quote currencies must differ")
	}
	body, err := p.fetch(ctx, base, quote)
	if err != nil {
		return application.LatestFXQuote{}, err
	}
	normalized, err := normalizeFrankfurterRate(body, base, quote)
	if err != nil {
		return application.LatestFXQuote{}, err
	}
	rate, err := domain.ParseFxRate(normalized.Rate)
	if err != nil {
		return application.LatestFXQuote{}, malformedProvider()
	}
	return application.LatestFXQuote{
		Rate:          rate,
		BaseCurrency:  base,
		QuoteCurrency: quote,
		SourceKey:     p.Key(),
		QuotedAt:      normalized.Date,
		Delayed:       true,
	}, nil
}

func (p *FrankfurterProvider) FXDailyHistory(ctx context.Context, identity application.FXMarketIdentity, rng application.DateRange) (application.MappingOutcome[application.FXDailyObservation], error) {
	base, err := domain.ParseCurrency(identity.BaseCurrency.String())
	if err != nil {
		return application.MappingOutcome[application.FXDailyObservation]{}, providerValidation("baseCurrency", "base currency is invalid")
	}
	quote, err := domain.ParseCurrency(identity.QuoteCurrency.String())
	if err != nil {
		return application.MappingOutcome[application.FXDailyObservation]{}, providerValidation("quoteCurrency", "quote currency is invalid")
	}
	if base == quote {
		return application.MappingOutcome[application.FXDailyObservation]{
			Status: application.MappingUnsupported,
			Reason: "identity_pair_is_not_a_raw_observation",
		}, nil
	}
	if _, err := application.InclusiveMarketDates(rng); err != nil {
		return application.MappingOutcome[application.FXDailyObservation]{}, err
	}
	body, err := p.fetchHistory(ctx, base, quote, rng)
	if err != nil {
		return application.MappingOutcome[application.FXDailyObservation]{}, err
	}
	return QualifyFrankfurterHistory(frankfurterHistoryRequestMeta(base, quote, rng), body)
}

func (p *FrankfurterProvider) fetchHistory(ctx context.Context, base, quote domain.CurrencyCode, rng application.DateRange) ([]byte, error) {
	return p.conn.doFetch(ctx, frankfurterHistoryURL(base, quote, rng), func(request *http.Request) {
		request.Header.Set("Accept", "application/json")
	}, classifyFrankfurterResponse)
}

func (p *FrankfurterProvider) fetch(ctx context.Context, base, quote domain.CurrencyCode) ([]byte, error) {
	return p.conn.doFetch(ctx, frankfurterRateURL(base, quote), func(request *http.Request) {
		request.Header.Set("Accept", "application/json")
	}, classifyFrankfurterResponse)
}

func classifyFrankfurterResponse(response *http.Response) error {
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return providerError(domain.ErrProviderAuthentication, "provider authentication failed")
	case http.StatusTooManyRequests:
		return providerError(domain.ErrProviderRateLimit, "provider rate limit reached")
	case http.StatusNotFound:
		return providerError(domain.ErrUnsupportedProviderSymbol, "provider currency pair is unsupported")
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return malformedProvider()
	}
	if response.StatusCode >= 500 {
		return providerUnavailable("provider is unavailable")
	}
	if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusUnprocessableEntity {
		return providerError(domain.ErrUnsupportedProviderSymbol, "provider currency pair is unsupported")
	}
	if response.StatusCode != http.StatusOK {
		return malformedProvider()
	}
	return nil
}

func frankfurterRateURL(base, quote domain.CurrencyCode) *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   frankfurterHost,
		Path:   "/v2/rate/" + url.PathEscape(base.String()) + "/" + url.PathEscape(quote.String()),
	}
}

func frankfurterHistoryURL(base, quote domain.CurrencyCode, rng application.DateRange) *url.URL {
	query := url.Values{}
	query.Set("from", string(rng.Start))
	query.Set("to", string(rng.End))
	query.Set("base", base.String())
	query.Set("quotes", quote.String())
	return &url.URL{
		Scheme:   "https",
		Host:     frankfurterHost,
		Path:     "/v2/rates",
		RawQuery: query.Encode(),
	}
}

func frankfurterHistoryRequestMeta(base, quote domain.CurrencyCode, rng application.DateRange) vnextFixtureMeta {
	meta := vnextFixtureMeta{
		FixtureID:     "frankfurter-v2-history-request",
		Provider:      frankfurterProviderKey,
		Capability:    "FXDailyHistory",
		BaseCurrency:  base.String(),
		QuoteCurrency: quote.String(),
		SourcePolicy:  domain.FrankfurterV2BlendedPolicy,
		Clock:         time.Now().UTC().Format(time.RFC3339),
	}
	meta.RequestedRange.Start = string(rng.Start)
	meta.RequestedRange.End = string(rng.End)
	if finalized, err := domain.LastFinalizedUSEquityMarketDate(time.Now().UTC()); err == nil {
		meta.LastFinalizedMarketDate = finalized
	}
	return meta
}

type frankfurterRateResponse struct {
	Date  string          `json:"date"`
	Base  string          `json:"base"`
	Quote string          `json:"quote"`
	Rate  json.RawMessage `json:"rate"`
}

type normalizedFrankfurterRate struct {
	Date time.Time
	Rate string
}

func normalizeFrankfurterRate(body []byte, expectedBase, expectedQuote domain.CurrencyCode) (normalizedFrankfurterRate, error) {
	var response frankfurterRateResponse
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return normalizedFrankfurterRate{}, malformedProvider()
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return normalizedFrankfurterRate{}, malformedProvider()
	}
	base, err := domain.ParseCurrency(response.Base)
	if err != nil || base != expectedBase {
		return normalizedFrankfurterRate{}, malformedProvider()
	}
	quote, err := domain.ParseCurrency(response.Quote)
	if err != nil || quote != expectedQuote {
		return normalizedFrankfurterRate{}, malformedProvider()
	}
	date, err := time.Parse("2006-01-02", strings.TrimSpace(response.Date))
	if err != nil {
		return normalizedFrankfurterRate{}, malformedProvider()
	}
	date, err = application.NormalizeProviderObservationTime(date.UTC(), time.Now().UTC())
	if err != nil {
		return normalizedFrankfurterRate{}, malformedProvider()
	}
	rate, err := jsonNumberLexeme(response.Rate)
	if err != nil {
		return normalizedFrankfurterRate{}, malformedProvider()
	}
	if _, err := domain.ParseFxRate(rate); err != nil {
		return normalizedFrankfurterRate{}, malformedProvider()
	}
	return normalizedFrankfurterRate{Date: date, Rate: rate}, nil
}

var _ application.MarketDataProvider = (*FrankfurterProvider)(nil)
var _ application.FXHistoryProvider = (*FrankfurterProvider)(nil)
