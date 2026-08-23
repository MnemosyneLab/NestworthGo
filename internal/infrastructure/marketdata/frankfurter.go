package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	client      *http.Client
	maxBodySize int64
	semaphore   chan struct{}
}

func NewFrankfurterProvider(transport http.RoundTripper) *FrankfurterProvider {
	return NewFrankfurterProviderWithOptions(FrankfurterProviderOptions{Transport: transport})
}

func NewFrankfurterProviderWithOptions(options FrankfurterProviderOptions) *FrankfurterProvider {
	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = frankfurterRequestTimeout
	}
	maxBodySize := options.MaxBodySize
	if maxBodySize <= 0 {
		maxBodySize = frankfurterMaxBodyBytes
	}
	semaphore := options.Semaphore
	if semaphore == nil {
		semaphore = sharedFrankfurterSemaphore
	}
	return &FrankfurterProvider{
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		maxBodySize: maxBodySize,
		semaphore:   semaphore,
	}
}

func (p *FrankfurterProvider) Key() string { return frankfurterProviderKey }

func (p *FrankfurterProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestFX: true}
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

func (p *FrankfurterProvider) fetch(ctx context.Context, base, quote domain.CurrencyCode) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case p.semaphore <- struct{}{}:
		defer func() { <-p.semaphore }()
	case <-ctx.Done():
		return nil, providerUnavailable("provider request was cancelled")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, frankfurterRateURL(base, quote).String(), nil)
	if err != nil {
		return nil, providerUnavailable("provider is unavailable")
	}
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUnavailable("provider request was cancelled")
		}
		return nil, providerUnavailable("provider is unavailable")
	}
	if response == nil || response.Body == nil {
		return nil, malformedProvider()
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, providerError(domain.ErrProviderAuthentication, "provider authentication failed")
	case http.StatusTooManyRequests:
		return nil, providerError(domain.ErrProviderRateLimit, "provider rate limit reached")
	case http.StatusNotFound:
		return nil, providerError(domain.ErrUnsupportedProviderSymbol, "provider currency pair is unsupported")
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return nil, malformedProvider()
	default:
		if response.StatusCode >= 500 {
			return nil, providerUnavailable("provider is unavailable")
		}
		if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusUnprocessableEntity {
			return nil, providerError(domain.ErrUnsupportedProviderSymbol, "provider currency pair is unsupported")
		}
		if response.StatusCode != http.StatusOK {
			return nil, malformedProvider()
		}
	}
	if response.ContentLength > p.maxBodySize {
		return nil, providerError(domain.ErrMarketDataResponseTooLarge, "provider response is too large")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, p.maxBodySize+1))
	if err != nil {
		return nil, providerUnavailable("provider is unavailable")
	}
	if int64(len(body)) > p.maxBodySize {
		return nil, providerError(domain.ErrMarketDataResponseTooLarge, "provider response is too large")
	}
	return body, nil
}

func frankfurterRateURL(base, quote domain.CurrencyCode) *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   frankfurterHost,
		Path:   "/v2/rate/" + url.PathEscape(base.String()) + "/" + url.PathEscape(quote.String()),
	}
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
