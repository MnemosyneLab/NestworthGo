package marketdata

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestWorkerProviderMapsAuthenticatedQuote(t *testing.T) {
	var captured *http.Request
	provider := NewWorkerProviderWithOptions(WorkerProviderOptions{
		Config: func() (WorkerConfig, error) {
			return WorkerConfig{BaseURL: "https://worker.example", APIToken: "secret"}, nil
		},
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			captured = request
			return workerResponse(http.StatusOK, `{"symbol":"D05.SI","currency":"SGD","price":54.32,"asOf":"2026-09-14T09:15:00Z"}`), nil
		}),
		Semaphore: make(chan struct{}, 2),
	})

	quote, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{
		ProviderSymbol: "D05.SI", QuoteCurrency: "SGD", Market: "SG",
	})
	if err != nil {
		t.Fatalf("quote error: %v", err)
	}
	if quote.Price.Canonical() != "54.32" || quote.Currency != "SGD" || quote.SourceKey != domain.WorkerProviderKey || !quote.Delayed {
		t.Fatalf("quote = %#v", quote)
	}
	if captured == nil || captured.URL.Path != "/v1/quote/D05.SI" || captured.Header.Get("Authorization") != "Bearer secret" {
		t.Fatalf("request = %#v", captured)
	}
}

func TestWorkerProviderMapsFinalizedHistoryWithSessionEvidence(t *testing.T) {
	var captured *http.Request
	provider := NewWorkerProviderWithOptions(WorkerProviderOptions{
		Config: func() (WorkerConfig, error) {
			return WorkerConfig{BaseURL: "https://worker.example/market", APIToken: "secret"}, nil
		},
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			captured = request
			return workerResponse(http.StatusOK, `{"symbol":"D05.SI","currency":"SGD","interval":"1d","prices":[{"date":"2026-09-11","close":53.82},{"date":"2026-09-14","close":54.10}]}`), nil
		}),
		Now:       func() time.Time { return time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC) },
		Semaphore: make(chan struct{}, 2),
	})

	outcome, err := provider.InstrumentDailyHistory(context.Background(), application.InstrumentMarketIdentity{
		ProviderSymbol: "D05.SI", QuoteCurrency: "SGD", Market: "SG", InstrumentType: "stock",
	}, application.DateRange{Start: "2026-09-11", End: "2026-09-14"})
	if err != nil {
		t.Fatalf("history error: %v", err)
	}
	if outcome.Status != application.MappingMapped || len(outcome.Batch.Observations) != 1 {
		t.Fatalf("outcome = %#v", outcome)
	}
	observation := outcome.Batch.Observations[0]
	if observation.MarketDate != "2026-09-11" || observation.Value != "53.82" || observation.PriceBasis != application.PriceBasisWorkerYahooClose || observation.TimestampBasis != application.TimestampBasisSessionClose {
		t.Fatalf("observation = %#v", observation)
	}
	wantEffective := time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC)
	if !observation.ValueEffectiveAt.Equal(wantEffective) {
		t.Fatalf("effective at = %s, want %s", observation.ValueEffectiveAt, wantEffective)
	}
	if captured == nil || captured.URL.Path != "/market/v1/history/D05.SI" || captured.URL.Query().Get("from") != "2026-09-11" || captured.Header.Get("Authorization") != "Bearer secret" {
		t.Fatalf("request = %#v", captured)
	}
}

func TestWorkerProviderReportsMissingLocalConfiguration(t *testing.T) {
	provider := NewWorkerProvider(nil, nil)
	code, reason := provider.LocalConfigStatus(context.Background())
	if code != application.ProviderConfigMissingKey || reason != "worker_url_missing" {
		t.Fatalf("status = %q, %q", code, reason)
	}
}

func workerResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(body)), ContentLength: int64(len(body)), Header: make(http.Header), Request: &http.Request{}}
}
