package marketdata

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/secrets"
)

func TestQualifyTiingoLatestMapsDelayedUSSnapshot(t *testing.T) {
	meta, body := mustLoadVNext(t, "providers/tiingo/aapl-iex-latest.json")
	quote, err := QualifyTiingoLatest(meta, body)
	if err != nil {
		t.Fatal(err)
	}
	if quote.Price.Canonical() != "185.25" || quote.Currency != "USD" || !quote.Delayed || quote.SourceKey != application.TiingoProviderKey {
		t.Fatalf("latest = %#v", quote)
	}
	if quote.QuotedAt.UTC().Format(time.RFC3339) != "2026-09-04T20:00:00Z" {
		t.Fatalf("quoted_at = %s", quote.QuotedAt.UTC())
	}
}

func TestTiingoLatestRefusesNonUSAndMissingSecret(t *testing.T) {
	store := secrets.NewMemoryStore()
	provider := NewTiingoProviderWithOptions(TiingoProviderOptions{Secrets: store, Semaphore: make(chan struct{}, 2), Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("missing secret still contacted Tiingo")
		return nil, nil
	})})
	_, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "AAPL", QuoteCurrency: "USD", Market: "US"})
	assertProviderCode(t, err, domain.ErrUnavailable)

	if _, err := store.Put(context.Background(), application.TiingoSecretRef(), []byte("test-token")); err != nil {
		t.Fatal(err)
	}
	_, err = provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "000001.SS", QuoteCurrency: "CNY", Market: "CN"})
	assertProviderCode(t, err, domain.ErrUnsupportedProviderSymbol)
}

func TestTiingoLocalConfigStatusDoesNotContactNetwork(t *testing.T) {
	store := secrets.NewMemoryStore()
	provider := NewTiingoProviderWithOptions(TiingoProviderOptions{Secrets: store, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("LocalConfigStatus contacted Tiingo")
		return nil, nil
	})})
	code, reason := provider.LocalConfigStatus(context.Background())
	if code != application.ProviderConfigMissingKey {
		t.Fatalf("status = %s %s", code, reason)
	}
	if _, err := store.Put(context.Background(), application.TiingoSecretRef(), []byte("test-token")); err != nil {
		t.Fatal(err)
	}
	code, reason = provider.LocalConfigStatus(context.Background())
	if code != application.ProviderConfigOK {
		t.Fatalf("configured status = %s %s", code, reason)
	}
}

func TestTiingoProviderHistoryAndLatestUseFixtures(t *testing.T) {
	store := secrets.NewMemoryStore()
	if _, err := store.Put(context.Background(), application.TiingoSecretRef(), []byte("test-token")); err != nil {
		t.Fatal(err)
	}
	_, latestBody := mustLoadVNext(t, "providers/tiingo/aapl-iex-latest.json")
	_, historyBody := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	var paths []string
	provider := NewTiingoProviderWithOptions(TiingoProviderOptions{
		Secrets:   store,
		Now:       func() time.Time { return time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC) },
		Semaphore: make(chan struct{}, 2),
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if strings.Contains(request.URL.RawQuery, "test-token") && strings.Contains(request.URL.RawQuery, "token=") {
				paths = append(paths, request.URL.EscapedPath())
			} else {
				t.Fatalf("token missing or leaked into path: %s", request.URL.Path)
			}
			body := latestBody
			if strings.Contains(request.URL.Path, "/prices") {
				body = historyBody
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Header: make(http.Header), Request: request}, nil
		}),
	})
	quote, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "AAPL", QuoteCurrency: "USD", Market: "US"})
	if err != nil {
		t.Fatal(err)
	}
	if quote.Price.Canonical() != "185.25" || !quote.Delayed {
		t.Fatalf("latest quote = %#v", quote)
	}
	outcome, err := provider.InstrumentDailyHistory(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "AAPL", QuoteCurrency: "USD", Market: "US"}, application.DateRange{Start: "2026-09-04", End: "2026-09-09"})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != application.MappingMapped || len(outcome.Batch.Observations) != 3 {
		t.Fatalf("history = %+v", outcome)
	}
	if outcome.Batch.Observations[0].SplitFactor != "" && outcome.Batch.Observations[0].Value == "" {
		t.Fatal("split replaced raw close")
	}
	cn, err := provider.InstrumentDailyHistory(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "000001.SS", QuoteCurrency: "CNY", Market: "CN"}, application.DateRange{Start: "2026-09-04", End: "2026-09-08"})
	if err != nil {
		t.Fatal(err)
	}
	if cn.Status != application.MappingUnsupported || cn.Reason != "tiingo_us_listed_only" || len(cn.Batch.Observations) != 0 {
		t.Fatalf("CN history = %+v", cn)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v", paths)
	}
}
