package marketdata

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func fixtureResponse(t *testing.T, name string, status int) *http.Response {
	t.Helper()
	data, err := osReadFile(filepath.Join("../../../testdata/v0.1.2/yahoo", name))
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Header: make(http.Header), Request: &http.Request{}}
}

var osReadFile = func(path string) ([]byte, error) { return os.ReadFile(path) }

func providerWithFixture(t *testing.T, fixture string, status int, captured func(*http.Request)) *YahooChartProvider {
	t.Helper()
	return NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if captured != nil {
				captured(request)
			}
			return fixtureResponse(t, fixture, status), nil
		}),
		Semaphore: make(chan struct{}, 2),
	})
}

func TestYahooChartProviderNormalizesSanitizedFixtures(t *testing.T) {
	regular := providerWithFixture(t, "regular-price.json", http.StatusOK, nil)
	quote, err := regular.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
	if err != nil {
		t.Fatalf("regular quote error: %v", err)
	}
	if quote.Price.Canonical() != "700.25" || quote.Currency != "USD" || quote.Delayed || quote.QuotedAt.Unix() != 1767225600 || quote.SourceKey != yahooProviderKey {
		t.Fatalf("regular quote = %#v", quote)
	}

	fallback := providerWithFixture(t, "close-fallback.json", http.StatusOK, nil)
	quote, err = fallback.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "ES3", QuoteCurrency: "USD"})
	if err != nil {
		t.Fatalf("fallback quote error: %v", err)
	}
	if quote.Price.Canonical() != "4.05" || !quote.Delayed || quote.QuotedAt.Unix() != 1767225600 {
		t.Fatalf("fallback quote = %#v", quote)
	}

	fx := providerWithFixture(t, "fx.json", http.StatusOK, nil)
	rate, err := fx.LatestFX(context.Background(), application.FXMarketIdentity{BaseCurrency: "USD", QuoteCurrency: "CNY"})
	if err != nil {
		t.Fatalf("FX quote error: %v", err)
	}
	if rate.Rate.Canonical() != "6.9" || rate.BaseCurrency != "USD" || rate.QuoteCurrency != "CNY" || rate.Delayed {
		t.Fatalf("FX quote = %#v", rate)
	}
}

func TestYahooChartProviderUsesRustBrowserHeaders(t *testing.T) {
	var captured []*http.Request
	provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			captured = append(captured, request)
			fixture := "regular-price.json"
			if strings.HasSuffix(request.URL.EscapedPath(), "/USDCNY=X") {
				fixture = "fx.json"
			}
			return fixtureResponse(t, fixture, http.StatusOK), nil
		}),
		Semaphore: make(chan struct{}, 2),
	})
	if _, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"}); err != nil {
		t.Fatalf("instrument request error: %v", err)
	}
	if _, err := provider.LatestFX(context.Background(), application.FXMarketIdentity{BaseCurrency: "USD", QuoteCurrency: "CNY"}); err != nil {
		t.Fatalf("FX request error: %v", err)
	}
	if len(captured) != 2 {
		t.Fatalf("captured %d Yahoo requests, want Instrument and FX", len(captured))
	}
	expected := map[string]string{
		"Accept":                    yahooAcceptHeader,
		"Accept-Encoding":           yahooAcceptEncodingHeader,
		"Accept-Language":           yahooAcceptLanguageHeader,
		"Priority":                  yahooPriorityHeader,
		"Sec-CH-UA":                 yahooSecCHUAHeader,
		"Sec-CH-UA-Mobile":          yahooSecCHUAMobileHeader,
		"Sec-CH-UA-Platform":        yahooSecCHUAPlatformHeader,
		"Sec-Fetch-Dest":            yahooSecFetchDestHeader,
		"Sec-Fetch-Mode":            yahooSecFetchModeHeader,
		"Sec-Fetch-Site":            yahooSecFetchSiteHeader,
		"Sec-Fetch-User":            yahooSecFetchUserHeader,
		"Upgrade-Insecure-Requests": yahooUpgradeInsecureRequestsHeader,
		"User-Agent":                yahooUserAgentHeader,
	}
	for index, request := range captured {
		for header, want := range expected {
			if got := request.Header.Get(header); got != want {
				t.Errorf("request %d %s = %q, want %q", index, header, got, want)
			}
		}
	}
}

func TestYahooChartProviderRejectsUnexpectedContentEncoding(t *testing.T) {
	provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			response := fixtureResponse(t, "regular-price.json", http.StatusOK)
			response.Header.Set("Content-Encoding", "gzip")
			response.Request = request
			return response, nil
		}),
		Semaphore: make(chan struct{}, 2),
	})
	_, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
	assertProviderCode(t, err, domain.ErrMalformedProviderResponse)
}

func TestYahooChartProviderMapsMalformedAndStatusFixturesSafely(t *testing.T) {
	malformed := []string{"malformed-decimal.json", "misaligned-close.json", "null-close.json", "currency-mismatch.json"}
	for _, fixture := range malformed {
		t.Run(fixture, func(t *testing.T) {
			provider := providerWithFixture(t, fixture, http.StatusOK, nil)
			_, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
			assertProviderCode(t, err, domain.ErrMalformedProviderResponse)
			if strings.Contains(err.Error(), "not-a-decimal") || strings.Contains(err.Error(), "query1.finance") {
				t.Fatalf("provider error leaked response details: %v", err)
			}
		})
	}
	for _, fixture := range []string{"api-error.json", "unknown-symbol.json"} {
		t.Run(fixture, func(t *testing.T) {
			provider := providerWithFixture(t, fixture, http.StatusOK, nil)
			_, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
			assertProviderCode(t, err, domain.ErrUnsupportedProviderSymbol)
		})
	}
	statusCases := map[string]struct {
		status int
		code   domain.ErrorCode
	}{
		"http-401.json": {http.StatusUnauthorized, domain.ErrProviderAuthentication},
		"http-403.json": {http.StatusForbidden, domain.ErrProviderAuthentication},
		"http-429.json": {http.StatusTooManyRequests, domain.ErrProviderRateLimit},
		"http-500.json": {http.StatusInternalServerError, domain.ErrProviderUnavailable},
	}
	for fixture, expected := range statusCases {
		t.Run(fixture, func(t *testing.T) {
			provider := providerWithFixture(t, fixture, expected.status, nil)
			_, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
			assertProviderCode(t, err, expected.code)
		})
	}
	provider := providerWithFixture(t, "regular-price.json", http.StatusNotFound, nil)
	_, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
	assertProviderCode(t, err, domain.ErrUnsupportedProviderSymbol)
}

func TestYahooChartProviderEscapesInstrumentSymbolsAndDerivesFXSymbols(t *testing.T) {
	var paths []string
	provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			paths = append(paths, request.URL.EscapedPath())
			fixture := "regular-price.json"
			if strings.HasSuffix(request.URL.EscapedPath(), "/USDCNY=X") {
				fixture = "fx.json"
			}
			return fixtureResponse(t, fixture, http.StatusOK), nil
		}),
		Semaphore: make(chan struct{}, 2),
	})
	if _, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "A/B?C", QuoteCurrency: "USD"}); err != nil {
		t.Fatalf("instrument request error: %v", err)
	}
	if _, err := provider.LatestFX(context.Background(), application.FXMarketIdentity{BaseCurrency: "USD", QuoteCurrency: "CNY"}); err != nil {
		t.Fatalf("FX request error: %v paths=%v", err, paths)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[0], "/A%2FB%3FC") || !strings.HasSuffix(paths[1], "/USDCNY=X") {
		t.Fatalf("escaped paths = %v", paths)
	}
}

func TestYahooChartProviderEnforcesBodyLimitAndCancellation(t *testing.T) {
	large := bytes.Repeat([]byte("x"), int(yahooMaxBodyBytes+1))
	provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(large)), ContentLength: int64(len(large)), Header: make(http.Header), Request: request}, nil
		}),
		Semaphore: make(chan struct{}, 2),
	})
	_, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
	assertProviderCode(t, err, domain.ErrMarketDataResponseTooLarge)
	streamed := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(large)), ContentLength: -1, Header: make(http.Header), Request: request}, nil
		}),
		Semaphore: make(chan struct{}, 2),
	})
	_, err = streamed.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
	assertProviderCode(t, err, domain.ErrMarketDataResponseTooLarge)

	redirect := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusFound, Body: io.NopCloser(strings.NewReader("redirect")), Header: http.Header{"Location": []string{"https://example.invalid/"}}, Request: request}, nil
		}),
		Semaphore: make(chan struct{}, 2),
	})
	_, err = redirect.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
	assertProviderCode(t, err, domain.ErrMalformedProviderResponse)

	blocking := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
		Timeout:   20 * time.Millisecond,
		Semaphore: make(chan struct{}, 2),
	})
	_, err = blocking.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
	assertProviderCode(t, err, domain.ErrProviderUnavailable)
}

func TestYahooChartProviderSharesTwoRequestSemaphore(t *testing.T) {
	var mu sync.Mutex
	active, maximum := 0, 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		active++
		if active > maximum {
			maximum = active
		}
		mu.Unlock()
		time.Sleep(15 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		return fixtureResponse(t, "regular-price.json", http.StatusOK), nil
	})
	provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{Transport: transport, Semaphore: make(chan struct{}, 2)})
	var wait sync.WaitGroup
	for i := 0; i < 6; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _ = provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
		}()
	}
	wait.Wait()
	if maximum > 2 {
		t.Fatalf("maximum concurrent requests = %d, want <= 2", maximum)
	}
}

func assertProviderCode(t *testing.T, err error, expected domain.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected provider error %q", expected)
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != expected {
		t.Fatalf("error = %#v, want code %q", err, expected)
	}
}
