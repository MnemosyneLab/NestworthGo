package marketdata

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	yfinanceclient "github.com/wnjoon/go-yfinance/pkg/client"
	yfinancemodels "github.com/wnjoon/go-yfinance/pkg/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

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

type fakeYahooTicker struct {
	quote         *yfinancemodels.Quote
	quoteErr      error
	bars          []yfinancemodels.Bar
	historyErr    error
	metadata      *yfinancemodels.ChartMeta
	historyParams []yfinancemodels.HistoryParams
	closed        bool
}

func (f *fakeYahooTicker) Quote() (*yfinancemodels.Quote, error) {
	return f.quote, f.quoteErr
}

func (f *fakeYahooTicker) History(params yfinancemodels.HistoryParams) ([]yfinancemodels.Bar, error) {
	f.historyParams = append(f.historyParams, params)
	return f.bars, f.historyErr
}

func (f *fakeYahooTicker) GetHistoryMetadata() *yfinancemodels.ChartMeta {
	return f.metadata
}

func (f *fakeYahooTicker) Close() { f.closed = true }

func TestYahooChartProviderUsesGoYFinanceForLatestQuote(t *testing.T) {
	quotedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ticker := &fakeYahooTicker{quote: &yfinancemodels.Quote{
		Symbol:             "QQQ",
		Currency:           "USD",
		RegularMarketPrice: 700.25,
		RegularMarketTime:  quotedAt,
		MarketState:        "REGULAR",
	}}
	var requestedSymbol string
	provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		TickerFactory: func(symbol string) (YahooTicker, error) {
			requestedSymbol = symbol
			return ticker, nil
		},
		Now: func() time.Time { return quotedAt.Add(time.Hour) },
	})

	quote, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
	if err != nil {
		t.Fatalf("latest quote error: %v", err)
	}
	if requestedSymbol != "QQQ" || quote.Price.Canonical() != "700.25" || quote.Currency != "USD" || quote.Delayed || quote.QuotedAt.Unix() != quotedAt.Unix() || quote.SourceKey != yahooProviderKey {
		t.Fatalf("latest quote = %#v, requested symbol = %q", quote, requestedSymbol)
	}
	if !ticker.closed {
		t.Fatal("provider did not close the go-yfinance ticker")
	}
}

func TestYahooChartProviderMapsGoYFinanceEquityHistory(t *testing.T) {
	ticker := &fakeYahooTicker{
		bars: []yfinancemodels.Bar{
			{Date: time.Date(2026, 9, 8, 13, 30, 0, 0, time.UTC), Close: 700.25},
			{Date: time.Date(2026, 9, 9, 13, 30, 0, 0, time.UTC), Close: 701.50},
		},
		metadata: &yfinancemodels.ChartMeta{
			Symbol:               "QQQ",
			Currency:             "USD",
			ExchangeTimezoneName: "America/New_York",
		},
	}
	provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		TickerFactory: func(string) (YahooTicker, error) { return ticker, nil },
		Now:           func() time.Time { return time.Date(2026, 9, 10, 22, 0, 0, 0, time.UTC) },
	})

	outcome, err := provider.InstrumentDailyHistory(context.Background(), application.InstrumentMarketIdentity{
		ProviderSymbol: "QQQ",
		QuoteCurrency:  "USD",
		Market:         "US",
	}, application.DateRange{Start: "2026-09-08", End: "2026-09-09"})
	if err != nil {
		t.Fatalf("history error: %v", err)
	}
	if outcome.Status != application.MappingMapped || len(outcome.Batch.Observations) != 2 {
		t.Fatalf("history outcome = %+v", outcome)
	}
	if got := outcome.Batch.Observations[0]; got.MarketDate != "2026-09-08" || got.Value != "700.25" || got.ValueEffectiveAt.UTC().Format(time.RFC3339) != "2026-09-08T20:00:00Z" || got.TimestampBasis != application.TimestampBasisSessionClose {
		t.Fatalf("first observation = %+v", got)
	}
	if outcome.Batch.Evidence.Adapter != "go-yfinance" || outcome.Batch.Evidence.AdapterVersion != yahooAdapterVersion {
		t.Fatalf("history evidence = %+v", outcome.Batch.Evidence)
	}
	if len(ticker.historyParams) != 1 {
		t.Fatalf("history calls = %d", len(ticker.historyParams))
	}
	params := ticker.historyParams[0]
	if params.Interval != "1d" || params.Start == nil || params.End == nil || params.Start.Format("2006-01-02") != "2026-09-08" || params.End.Format("2006-01-02") != "2026-09-10" || params.AutoAdjust || params.Actions || params.Repair || params.PrePost {
		t.Fatalf("history params = %+v", params)
	}
}

func TestYahooChartProviderDoesNotInventMissingCryptoBars(t *testing.T) {
	ticker := &fakeYahooTicker{
		bars: []yfinancemodels.Bar{
			{Date: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC), Close: 99.25},
			{Date: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), Close: 102.26},
		},
		metadata: &yfinancemodels.ChartMeta{Symbol: "SOL-USD", Currency: "USD"},
	}
	provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		TickerFactory: func(string) (YahooTicker, error) { return ticker, nil },
		Now:           func() time.Time { return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC) },
	})

	outcome, err := provider.InstrumentDailyHistory(context.Background(), application.InstrumentMarketIdentity{
		ProviderSymbol: "SOL-USD",
		QuoteCurrency:  "USD",
		InstrumentType: "crypto",
		Market:         "crypto",
	}, application.DateRange{Start: "2026-09-13", End: "2026-09-15"})
	if err != nil {
		t.Fatalf("history error: %v", err)
	}
	if len(outcome.Batch.Observations) != 1 || outcome.Batch.Observations[0].MarketDate != "2026-09-13" {
		t.Fatalf("history observations = %+v", outcome.Batch.Observations)
	}
	if len(outcome.Batch.UncertainRanges) != 1 || outcome.Batch.UncertainRanges[0].Start != "2026-09-15" || outcome.Batch.UncertainRanges[0].End != "2026-09-15" {
		t.Fatalf("uncertain ranges = %+v", outcome.Batch.UncertainRanges)
	}
}

func TestYahooChartProviderMapsGoYFinanceErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code domain.ErrorCode
	}{
		{name: "rate limit", err: yfinanceclient.WrapRateLimitError(), code: domain.ErrProviderRateLimit},
		{name: "authentication", err: yfinanceclient.WrapAuthError(errors.New("upstream auth")), code: domain.ErrProviderAuthentication},
		{name: "unsupported symbol", err: yfinanceclient.WrapInvalidSymbolError("UNKNOWN"), code: domain.ErrUnsupportedProviderSymbol},
		{name: "Yahoo chart error", err: yfinanceclient.NewChartAPIError("UNKNOWN", "Not Found", "symbol not found"), code: domain.ErrUnsupportedProviderSymbol},
		{name: "network", err: yfinanceclient.WrapNetworkError(errors.New("upstream unavailable")), code: domain.ErrProviderUnavailable},
		{name: "malformed response", err: yfinanceclient.WrapInvalidResponseError(errors.New("bad payload")), code: domain.ErrMalformedProviderResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
				TickerFactory: func(string) (YahooTicker, error) { return nil, test.err },
			})
			_, err := provider.LatestInstrument(context.Background(), application.InstrumentMarketIdentity{ProviderSymbol: "QQQ", QuoteCurrency: "USD"})
			assertProviderCode(t, err, test.code)
			if err != nil && strings.Contains(err.Error(), "upstream") {
				t.Fatalf("provider error leaked upstream details: %v", err)
			}
		})
	}
}
