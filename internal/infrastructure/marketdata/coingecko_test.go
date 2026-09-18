package marketdata

import (
	"context"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func cgIdentity(id string) application.InstrumentMarketIdentity {
	return application.InstrumentMarketIdentity{ProviderKey: domain.CoinGeckoProviderKey, ProviderSymbol: id, InstrumentType: "crypto", Market: "CRYPTO", QuoteCurrency: "USD"}
}
func cgFixture(t *testing.T, body string, inspect func(*http.Request)) *CoinGeckoProvider {
	t.Helper()
	p := NewCoinGeckoProvider(func() (string, error) { return "fixture-key", nil }, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("x-cg-demo-api-key") != "fixture-key" || strings.Contains(r.URL.String(), "fixture-key") {
			t.Fatal("unsafe authentication")
		}
		if inspect != nil {
			inspect(r)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	p.now = func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) }
	return p
}
func TestCoinGeckoBatchPreservesMissingPrices(t *testing.T) {
	calls := 0
	stamp := time.Date(2026, 9, 18, 11, 59, 0, 0, time.UTC).Unix()
	p := cgFixture(t, fmt.Sprintf(`{"bitcoin":{"usd":123.123456789,"last_updated_at":%d},"ethereum":{"usd":null,"last_updated_at":%d}}`, stamp, stamp), func(r *http.Request) {
		calls++
		if r.URL.Query().Get("ids") != "bitcoin,ethereum" {
			t.Fatal(r.URL)
		}
	})
	btc, eth := cgIdentity("bitcoin"), cgIdentity("ethereum")
	result := p.LatestInstruments(context.Background(), []application.InstrumentMarketIdentity{btc, eth})
	if calls != 1 || result[application.LatestInstrumentIdentityKey(btc)].Quote.Price.Canonical() != "123.12345679" || result[application.LatestInstrumentIdentityKey(eth)].Err == nil {
		t.Fatalf("result=%+v calls=%d", result, calls)
	}
	p.LatestInstruments(context.Background(), []application.InstrumentMarketIdentity{btc, eth})
	if calls != 1 {
		t.Fatal("batch cache missed")
	}
}
func TestCoinGeckoSearchUsesIDs(t *testing.T) {
	p := cgFixture(t, `{"coins":[{"id":"bitcoin","name":"Bitcoin","symbol":"btc"},{"id":"bitcoin-other","name":"Other","symbol":"btc"}]}`, nil)
	hits, err := p.SearchInstruments(context.Background(), "BTC", "crypto", 8)
	if err != nil || len(hits) != 2 || hits[0].ProviderSymbol != "bitcoin" || hits[1].ProviderSymbol != "bitcoin-other" {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
}
func TestCoinGeckoDailyReferenceDoesNotInventCloseOrMissingDay(t *testing.T) {
	stamp := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	p := cgFixture(t, fmt.Sprintf(`{"prices":[[%d,60000.1],[%d,61000]]}`, stamp.UnixMilli(), stamp.Add(12*time.Hour).UnixMilli()), func(r *http.Request) {
		if r.URL.Query().Get("interval") != "daily" || r.URL.Query().Get("from") != "2026-09-16" {
			t.Fatal(r.URL)
		}
	})
	out, err := p.InstrumentDailyHistory(context.Background(), cgIdentity("bitcoin"), application.DateRange{Start: "2026-09-16", End: "2026-09-17"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Batch.Observations) != 1 || len(out.Batch.VerifiedRanges) != 1 || len(out.Batch.PendingRanges) != 1 || out.Status != application.MappingPending {
		t.Fatalf("%+v", out)
	}
	point := out.Batch.Observations[0]
	if point.MarketDate != "2026-09-16" || !point.ValueEffectiveAt.Equal(stamp) || !point.ProviderTimestamp.Equal(stamp) {
		t.Fatalf("point=%+v", point)
	}
}
func TestCoinGeckoRejectsOutOfRangeWithoutNetwork(t *testing.T) {
	p := cgFixture(t, `{}`, func(*http.Request) { t.Fatal("old range contacted provider") })
	_, err := p.InstrumentDailyHistory(context.Background(), cgIdentity("bitcoin"), application.DateRange{Start: "2024-09-14", End: "2024-09-20"})
	if err == nil {
		t.Fatal("expected history limit")
	}
}
func TestCoinGeckoTinyPriceIsNotZero(t *testing.T) {
	p := cgFixture(t, `{"bitcoin":{"usd":0.00000000001,"last_updated_at":1789700000}}`, nil)
	if _, err := p.LatestInstrument(context.Background(), cgIdentity("bitcoin")); err == nil {
		t.Fatal("tiny price rounded to zero")
	}
}
func TestCoinGeckoMissingKeyIsOffline(t *testing.T) {
	p := NewCoinGeckoProvider(nil, roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("missing key contacted network"); return nil, nil }))
	if _, err := p.LatestInstrument(context.Background(), cgIdentity("bitcoin")); err == nil {
		t.Fatal("expected authentication error")
	}
}

func TestCoinGeckoRateLimitStopsRemainingBatchCurrencies(t *testing.T) {
	calls := 0
	p := NewCoinGeckoProvider(func() (string, error) { return "fixture-key", nil }, roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	}))
	first, second := cgIdentity("bitcoin"), cgIdentity("ethereum")
	second.QuoteCurrency = "CNY"
	results := p.LatestInstruments(context.Background(), []application.InstrumentMarketIdentity{first, second})
	if calls != 1 {
		t.Fatalf("requests after rate limit: %d", calls)
	}
	for _, result := range results {
		assertProviderCode(t, result.Err, domain.ErrProviderRateLimit)
	}
}
func TestCoinGeckoSupportedCurrenciesExcludeTokens(t *testing.T) {
	p := cgFixture(t, `["usd","cny","sgd","btc","eth"]`, nil)
	values, err := p.SupportedQuoteCurrencies(context.Background())
	if err != nil || strings.Join(values, ",") != "USD,CNY,SGD" {
		t.Fatalf("currencies=%v err=%v", values, err)
	}
}
