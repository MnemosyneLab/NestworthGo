// Package marketdata contains the native provider adapters. Yahoo-specific
// URLs and response fields stop in this package and never cross into domain,
// application, SQLite, or UI code.
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
	yahooProviderKey    = "yahoo_finance"
	yahooChartHost      = "query1.finance.yahoo.com"
	yahooRequestTimeout = 8 * time.Second
	yahooMaxBodyBytes   = int64(2 * 1024 * 1024)

	// Keep this profile in lockstep with the Rust client's browser_headers.
	yahooAcceptHeader = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	// The Rust client advertises browser compression codecs. The Go client
	// deliberately requests identity so it can keep the same browser profile
	// without adding Brotli/Zstandard decoder dependencies.
	yahooAcceptEncodingHeader          = "identity"
	yahooAcceptLanguageHeader          = "en-US,en;q=0.9"
	yahooPriorityHeader                = "u=0, i"
	yahooSecCHUAHeader                 = `"Not=A?Brand";v="99", "Google Chrome";v="151", "Chromium";v="151"`
	yahooSecCHUAMobileHeader           = "?0"
	yahooSecCHUAPlatformHeader         = `"macOS"`
	yahooSecFetchDestHeader            = "document"
	yahooSecFetchModeHeader            = "navigate"
	yahooSecFetchSiteHeader            = "none"
	yahooSecFetchUserHeader            = "?1"
	yahooUpgradeInsecureRequestsHeader = "1"
	yahooUserAgentHeader               = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"
)

var sharedYahooSemaphore = make(chan struct{}, 2)

type YahooChartProviderOptions struct {
	Transport   http.RoundTripper
	Timeout     time.Duration
	MaxBodySize int64
	Semaphore   chan struct{}
}

type YahooChartProvider struct {
	conn *providerHTTPClient
}

func NewYahooChartProvider(transport http.RoundTripper) *YahooChartProvider {
	return NewYahooChartProviderWithOptions(YahooChartProviderOptions{Transport: transport})
}

func NewYahooChartProviderWithOptions(options YahooChartProviderOptions) *YahooChartProvider {
	return &YahooChartProvider{
		conn: newProviderHTTPClient(providerHTTPOptions(options), yahooRequestTimeout, yahooMaxBodyBytes, sharedYahooSemaphore),
	}
}

func (p *YahooChartProvider) Key() string { return yahooProviderKey }

func (p *YahooChartProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestInstrument: true}
}

func (p *YahooChartProvider) LatestInstrument(ctx context.Context, identity application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	if strings.TrimSpace(identity.ProviderSymbol) == "" {
		return application.LatestInstrumentQuote{}, providerValidation("providerSymbol", "provider symbol is required")
	}
	currency, err := domain.ParseCurrency(identity.QuoteCurrency.String())
	if err != nil {
		return application.LatestInstrumentQuote{}, providerValidation("quoteCurrency", "quote currency is invalid")
	}
	body, err := p.fetch(ctx, identity.ProviderSymbol)
	if err != nil {
		return application.LatestInstrumentQuote{}, err
	}
	normalized, err := normalizeChart(body, currency, false)
	if err != nil {
		return application.LatestInstrumentQuote{}, err
	}
	price, err := domain.ParseUnitPrice(normalized.Value)
	if err != nil {
		return application.LatestInstrumentQuote{}, malformedProvider()
	}
	return application.LatestInstrumentQuote{Price: price, Currency: currency, SourceKey: p.Key(), QuotedAt: normalized.QuotedAt, Delayed: normalized.Delayed}, nil
}

func (p *YahooChartProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	return application.LatestFXQuote{}, &domain.Error{Code: domain.ErrUnavailable, Message: "provider does not support FX refresh"}
}

func (p *YahooChartProvider) fetch(ctx context.Context, symbol string) ([]byte, error) {
	return p.conn.doFetch(ctx, yahooChartURL(symbol), func(request *http.Request) {
		request.Header = yahooBrowserHeaders()
	}, classifyYahooResponse)
}

func classifyYahooResponse(response *http.Response) error {
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return providerError(domain.ErrProviderAuthentication, "provider authentication failed")
	case http.StatusTooManyRequests:
		return providerError(domain.ErrProviderRateLimit, "provider rate limit reached")
	case http.StatusNotFound:
		return unsupportedProviderSymbol()
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return malformedProvider()
	}
	if response.StatusCode >= 500 {
		return providerUnavailable("provider is unavailable")
	}
	if response.StatusCode != http.StatusOK {
		return malformedProvider()
	}
	if encoding := strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Encoding"))); encoding != "" && encoding != "identity" {
		return malformedProvider()
	}
	return nil
}

func yahooBrowserHeaders() http.Header {
	headers := make(http.Header, 12)
	headers.Set("Accept", yahooAcceptHeader)
	headers.Set("Accept-Encoding", yahooAcceptEncodingHeader)
	headers.Set("Accept-Language", yahooAcceptLanguageHeader)
	headers.Set("Priority", yahooPriorityHeader)
	headers.Set("Sec-CH-UA", yahooSecCHUAHeader)
	headers.Set("Sec-CH-UA-Mobile", yahooSecCHUAMobileHeader)
	headers.Set("Sec-CH-UA-Platform", yahooSecCHUAPlatformHeader)
	headers.Set("Sec-Fetch-Dest", yahooSecFetchDestHeader)
	headers.Set("Sec-Fetch-Mode", yahooSecFetchModeHeader)
	headers.Set("Sec-Fetch-Site", yahooSecFetchSiteHeader)
	headers.Set("Sec-Fetch-User", yahooSecFetchUserHeader)
	headers.Set("Upgrade-Insecure-Requests", yahooUpgradeInsecureRequestsHeader)
	headers.Set("User-Agent", yahooUserAgentHeader)
	return headers
}

func yahooChartURL(symbol string) *url.URL {
	return &url.URL{
		Scheme:   "https",
		Host:     yahooChartHost,
		Path:     "/v8/finance/chart/" + symbol,
		RawPath:  "/v8/finance/chart/" + url.PathEscape(symbol),
		RawQuery: "range=1m&interval=1d",
	}
}

type chartEnvelope struct {
	Chart chartPayload `json:"chart"`
}

type chartPayload struct {
	Result []chartResult `json:"result"`
	Error  *chartError   `json:"error"`
}

type chartError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type chartResult struct {
	Meta       chartMeta       `json:"meta"`
	Timestamp  []int64         `json:"timestamp"`
	Indicators chartIndicators `json:"indicators"`
}

type chartMeta struct {
	Currency           string          `json:"currency"`
	RegularMarketPrice json.RawMessage `json:"regularMarketPrice"`
	RegularMarketTime  *int64          `json:"regularMarketTime"`
}

type chartIndicators struct {
	Quote []chartQuote `json:"quote"`
}

type chartQuote struct {
	Close []json.RawMessage `json:"close"`
}

type normalizedChartQuote struct {
	Value    string
	QuotedAt time.Time
	Delayed  bool
}

func normalizeChart(body []byte, expectedCurrency domain.CurrencyCode, fx bool) (normalizedChartQuote, error) {
	return normalizeChartAt(body, expectedCurrency, fx, time.Now().UTC())
}

func normalizeChartAt(body []byte, expectedCurrency domain.CurrencyCode, fx bool, now time.Time) (normalizedChartQuote, error) {
	var envelope chartEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return normalizedChartQuote{}, malformedProvider()
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return normalizedChartQuote{}, malformedProvider()
	}
	if envelope.Chart.Error != nil {
		return normalizedChartQuote{}, unsupportedProviderSymbol()
	}
	if len(envelope.Chart.Result) != 1 {
		return normalizedChartQuote{}, malformedProvider()
	}
	result := envelope.Chart.Result[0]
	actualCurrency, err := domain.ParseCurrency(result.Meta.Currency)
	if err != nil || actualCurrency != expectedCurrency {
		return normalizedChartQuote{}, malformedProvider()
	}
	if raw := result.Meta.RegularMarketPrice; len(raw) > 0 && !isJSONNull(raw) {
		lexeme, parseErr := jsonNumberLexeme(raw)
		if parseErr != nil {
			return normalizedChartQuote{}, malformedProvider()
		}
		price, priceErr := parseProviderPrice(lexeme, fx)
		if priceErr != nil {
			return normalizedChartQuote{}, malformedProvider()
		}
		if result.Meta.RegularMarketTime != nil {
			quotedAt, timeErr := unixTimestampAt(*result.Meta.RegularMarketTime, now)
			if timeErr != nil {
				return normalizedChartQuote{}, malformedProvider()
			}
			return normalizedChartQuote{Value: price, QuotedAt: quotedAt}, nil
		}
	}
	return fallbackCloseAt(result, fx, now)
}

func fallbackCloseAt(result chartResult, fx bool, now time.Time) (normalizedChartQuote, error) {
	if len(result.Indicators.Quote) != 1 || len(result.Timestamp) == 0 || len(result.Timestamp) != len(result.Indicators.Quote[0].Close) {
		return normalizedChartQuote{}, malformedProvider()
	}
	var selected string
	var selectedAt time.Time
	found := false
	for index, raw := range result.Indicators.Quote[0].Close {
		if isJSONNull(raw) {
			continue
		}
		lexeme, err := jsonNumberLexeme(raw)
		if err != nil {
			return normalizedChartQuote{}, malformedProvider()
		}
		price, err := parseProviderPrice(lexeme, fx)
		if err != nil {
			return normalizedChartQuote{}, malformedProvider()
		}
		quotedAt, err := unixTimestampAt(result.Timestamp[index], now)
		if err != nil {
			return normalizedChartQuote{}, malformedProvider()
		}
		selected, selectedAt, found = price, quotedAt, true
	}
	if !found {
		return normalizedChartQuote{}, malformedProvider()
	}
	return normalizedChartQuote{Value: selected, QuotedAt: selectedAt, Delayed: true}, nil
}

func parseProviderPrice(lexeme string, fx bool) (string, error) {
	if fx {
		rate, err := domain.ParseFxRate(lexeme)
		if err != nil {
			return "", err
		}
		return rate.Canonical(), nil
	}
	price, err := domain.ParseUnitPrice(lexeme)
	if err != nil {
		return "", err
	}
	return price.Canonical(), nil
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

func unixTimestampAt(value int64, now time.Time) (time.Time, error) {
	if value <= 0 {
		return time.Time{}, errors.New("timestamp is invalid")
	}
	return application.NormalizeProviderObservationTime(time.Unix(value, 0).UTC(), now)
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
