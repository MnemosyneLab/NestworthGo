package marketdata

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func frankfurterFixtureResponse(t *testing.T, name string, status int) *http.Response {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../../testdata/v0.1.2/frankfurter", name))
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Header: make(http.Header), Request: &http.Request{}}
}

func TestFrankfurterProviderNormalizesDailyRateFixture(t *testing.T) {
	var captured *http.Request
	provider := NewFrankfurterProviderWithOptions(FrankfurterProviderOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			captured = request
			return frankfurterFixtureResponse(t, "rate-usd-cny.json", http.StatusOK), nil
		}),
		Semaphore: make(chan struct{}, 2),
	})
	quote, err := provider.LatestFX(context.Background(), application.FXMarketIdentity{BaseCurrency: "USD", QuoteCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if quote.Rate.Canonical() != "6.9" || quote.BaseCurrency != "USD" || quote.QuoteCurrency != "CNY" || quote.SourceKey != frankfurterProviderKey || !quote.Delayed || quote.QuotedAt.Format("2006-01-02") != "2026-08-21" {
		t.Fatalf("Frankfurter quote = %#v", quote)
	}
	if captured == nil || captured.URL.Host != frankfurterHost || captured.URL.EscapedPath() != "/v2/rate/USD/CNY" {
		t.Fatalf("Frankfurter request = %#v", captured)
	}
	if provider.Capabilities().LatestInstrument || !provider.Capabilities().LatestFX {
		t.Fatalf("Frankfurter capabilities = %#v", provider.Capabilities())
	}
}

func TestFrankfurterProviderMapsMalformedAndHTTPResponsesSafely(t *testing.T) {
	cases := []struct {
		name string
		body string
		code domain.ErrorCode
	}{
		{name: "malformed-json", body: `{`, code: domain.ErrMalformedProviderResponse},
		{name: "currency-mismatch", body: `{"date":"2026-08-21","base":"EUR","quote":"CNY","rate":6.9}`, code: domain.ErrMalformedProviderResponse},
		{name: "malformed-rate", body: `{"date":"2026-08-21","base":"USD","quote":"CNY","rate":"not-a-decimal"}`, code: domain.ErrMalformedProviderResponse},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			provider := NewFrankfurterProviderWithOptions(FrankfurterProviderOptions{
				Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(testCase.body)), ContentLength: int64(len(testCase.body)), Header: make(http.Header), Request: request}, nil
				}),
				Semaphore: make(chan struct{}, 2),
			})
			_, err := provider.LatestFX(context.Background(), application.FXMarketIdentity{BaseCurrency: "USD", QuoteCurrency: "CNY"})
			assertProviderCode(t, err, testCase.code)
			if strings.Contains(err.Error(), "not-a-decimal") || strings.Contains(err.Error(), frankfurterHost) {
				t.Fatalf("Frankfurter error leaked provider details: %v", err)
			}
		})
	}

	statusCases := map[int]domain.ErrorCode{
		http.StatusUnauthorized:        domain.ErrProviderAuthentication,
		http.StatusForbidden:           domain.ErrProviderAuthentication,
		http.StatusBadRequest:          domain.ErrUnsupportedProviderSymbol,
		http.StatusNotFound:            domain.ErrUnsupportedProviderSymbol,
		http.StatusUnprocessableEntity: domain.ErrUnsupportedProviderSymbol,
		http.StatusTooManyRequests:     domain.ErrProviderRateLimit,
		http.StatusInternalServerError: domain.ErrProviderUnavailable,
		http.StatusFound:               domain.ErrMalformedProviderResponse,
	}
	for status, code := range statusCases {
		t.Run(http.StatusText(status), func(t *testing.T) {
			provider := NewFrankfurterProviderWithOptions(FrankfurterProviderOptions{
				Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("{}")), ContentLength: 2, Header: make(http.Header), Request: request}, nil
				}),
				Semaphore: make(chan struct{}, 2),
			})
			_, err := provider.LatestFX(context.Background(), application.FXMarketIdentity{BaseCurrency: "USD", QuoteCurrency: "CNY"})
			assertProviderCode(t, err, code)
		})
	}
}

func TestFrankfurterProviderEnforcesBodyLimitAndCancellation(t *testing.T) {
	large := bytes.Repeat([]byte("x"), int(frankfurterMaxBodyBytes+1))
	provider := NewFrankfurterProviderWithOptions(FrankfurterProviderOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(large)), ContentLength: int64(len(large)), Header: make(http.Header), Request: request}, nil
		}),
		Semaphore: make(chan struct{}, 2),
	})
	_, err := provider.LatestFX(context.Background(), application.FXMarketIdentity{BaseCurrency: "USD", QuoteCurrency: "CNY"})
	assertProviderCode(t, err, domain.ErrMarketDataResponseTooLarge)

	blocking := NewFrankfurterProviderWithOptions(FrankfurterProviderOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
		Semaphore: make(chan struct{}, 2),
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = blocking.LatestFX(ctx, application.FXMarketIdentity{BaseCurrency: "USD", QuoteCurrency: "CNY"})
	assertProviderCode(t, err, domain.ErrProviderUnavailable)

	timeoutProvider := NewFrankfurterProviderWithOptions(FrankfurterProviderOptions{
		Timeout: 1 * time.Millisecond,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
		Semaphore: make(chan struct{}, 2),
	})
	_, err = timeoutProvider.LatestFX(context.Background(), application.FXMarketIdentity{BaseCurrency: "USD", QuoteCurrency: "CNY"})
	assertProviderCode(t, err, domain.ErrProviderUnavailable)
}
