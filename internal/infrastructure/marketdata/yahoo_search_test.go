package marketdata

import (
	"context"
	"errors"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	yfinancemodels "github.com/wnjoon/go-yfinance/pkg/models"
)

type fakeYahooSearch struct {
	quotes    []yfinancemodels.SearchQuote
	err       error
	query     string
	limit     int
	closed    bool
	callCount int
}

func (f *fakeYahooSearch) Quotes(query string, maxResults int) ([]yfinancemodels.SearchQuote, error) {
	f.query = query
	f.limit = maxResults
	f.callCount++
	return f.quotes, f.err
}

func (f *fakeYahooSearch) Close() { f.closed = true }

func TestYahooChartProviderSearchesWithGoYFinance(t *testing.T) {
	search := &fakeYahooSearch{
		quotes: []yfinancemodels.SearchQuote{
			{Symbol: "NVDA", ShortName: "NVIDIA", LongName: "NVIDIA Corporation", Exchange: "NMS", ExchangeDisp: "NASDAQ", QuoteType: "EQUITY"},
			{Symbol: "QQQ", ShortName: "Invesco QQQ Trust", Exchange: "NMS", ExchangeDisp: "NASDAQ", QuoteType: "ETF"},
			{Symbol: "USDJPY=X", ShortName: "USD/JPY", QuoteType: "CURRENCY"},
		},
	}
	provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		SearchFactory: func() (YahooSearch, error) { return search, nil },
	})

	hits, err := provider.SearchInstruments(context.Background(), "nvda", "stock", 8)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if search.query != "nvda" || search.limit != yahooSearchFetchMax || !search.closed || search.callCount != 1 {
		t.Fatalf("search client = %#v", search)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %#v, want 2 stock/ETF results", hits)
	}
	if hits[0].ProviderKey != yahooProviderKey || hits[0].ProviderSymbol != "NVDA" || hits[0].Name != "NVIDIA Corporation" || hits[0].Symbol != "NVDA" || hits[0].Type != string(domain.InstrumentStock) || hits[0].MarketCode != "NASDAQ" || hits[0].CountryCode != "US" || hits[0].QuoteCurrency != "USD" {
		t.Fatalf("stock hit = %#v", hits[0])
	}
	if hits[1].Type != string(domain.InstrumentETF) || hits[1].ProviderSymbol != "QQQ" {
		t.Fatalf("etf hit = %#v", hits[1])
	}
}

func TestMapYahooSearchQuoteFillsNonUSListings(t *testing.T) {
	tests := []struct {
		name  string
		quote yfinancemodels.SearchQuote
		want  application.InstrumentSearchHit
	}{
		{
			name:  "Hong Kong equity",
			quote: yfinancemodels.SearchQuote{Symbol: "0700.HK", LongName: "Tencent Holdings Limited", Exchange: "HKG", ExchangeDisp: "HKSE", QuoteType: "EQUITY"},
			want:  application.InstrumentSearchHit{ProviderKey: yahooProviderKey, ProviderSymbol: "0700.HK", Name: "Tencent Holdings Limited", Symbol: "0700", Type: "stock", MarketCode: "HKEX", CountryCode: "HK", QuoteCurrency: "HKD", Exchange: "HKSE"},
		},
		{
			name:  "Shanghai equity from suffix",
			quote: yfinancemodels.SearchQuote{Symbol: "600519.SS", ShortName: "Kweichow Moutai", QuoteType: "equity"},
			want:  application.InstrumentSearchHit{ProviderKey: yahooProviderKey, ProviderSymbol: "600519.SS", Name: "Kweichow Moutai", Symbol: "600519", Type: "stock", MarketCode: "SSE", CountryCode: "CN", QuoteCurrency: "CNY"},
		},
		{
			name:  "Singapore ETF",
			quote: yfinancemodels.SearchQuote{Symbol: "ES3.SI", LongName: "SPDR STI ETF", Exchange: "SES", ExchangeDisp: "SES", QuoteType: "ETF"},
			want:  application.InstrumentSearchHit{ProviderKey: yahooProviderKey, ProviderSymbol: "ES3.SI", Name: "SPDR STI ETF", Symbol: "ES3", Type: "etf", MarketCode: "SGX", CountryCode: "SG", QuoteCurrency: "SGD", Exchange: "SES"},
		},
		{
			name:  "NYSE Arca ETF",
			quote: yfinancemodels.SearchQuote{Symbol: "SPY", LongName: "SPDR S&P 500 ETF Trust", Exchange: "PCX", ExchangeDisp: "NYSEArca", QuoteType: "ETF"},
			want:  application.InstrumentSearchHit{ProviderKey: yahooProviderKey, ProviderSymbol: "SPY", Name: "SPDR S&P 500 ETF Trust", Symbol: "SPY", Type: "etf", MarketCode: "ARCA", CountryCode: "US", QuoteCurrency: "USD", Exchange: "NYSEArca"},
		},
		{
			name:  "Cboe BZX ETF",
			quote: yfinancemodels.SearchQuote{Symbol: "ARKK", LongName: "ARK Innovation ETF", Exchange: "BTS", ExchangeDisp: "Cboe BZX", QuoteType: "ETF"},
			want:  application.InstrumentSearchHit{ProviderKey: yahooProviderKey, ProviderSymbol: "ARKK", Name: "ARK Innovation ETF", Symbol: "ARKK", Type: "etf", MarketCode: "BZX", CountryCode: "US", QuoteCurrency: "USD", Exchange: "Cboe BZX"},
		},
		{
			name:  "Cboe EDGX ETF from display name",
			quote: yfinancemodels.SearchQuote{Symbol: "SPSM", LongName: "SPDR Portfolio S&P 600 Small Cap ETF", ExchangeDisp: "Cboe EDGX", QuoteType: "ETF"},
			want:  application.InstrumentSearchHit{ProviderKey: yahooProviderKey, ProviderSymbol: "SPSM", Name: "SPDR Portfolio S&P 600 Small Cap ETF", Symbol: "SPSM", Type: "etf", MarketCode: "EDGX", CountryCode: "US", QuoteCurrency: "USD", Exchange: "Cboe EDGX"},
		},
		{
			name:  "IEX listed equity",
			quote: yfinancemodels.SearchQuote{Symbol: "IEXA", LongName: "Example IEX Listing", Exchange: "IEXG", ExchangeDisp: "Investors Exchange", QuoteType: "EQUITY"},
			want:  application.InstrumentSearchHit{ProviderKey: yahooProviderKey, ProviderSymbol: "IEXA", Name: "Example IEX Listing", Symbol: "IEXA", Type: "stock", MarketCode: "IEX", CountryCode: "US", QuoteCurrency: "USD", Exchange: "Investors Exchange"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := mapYahooSearchQuote(test.quote)
			if !ok {
				t.Fatal("expected a mapped hit")
			}
			if got != test.want {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestYahooLocalSymbolKeepsUSClassShares(t *testing.T) {
	if got := yahooLocalSymbol("BRK-B"); got != "BRK-B" {
		t.Fatalf("BRK-B = %q", got)
	}
	if got := yahooLocalSymbol("BF.B"); got != "BF.B" {
		t.Fatalf("BF.B = %q", got)
	}
}

func TestYahooChartProviderSearchMapsLibraryErrors(t *testing.T) {
	provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		SearchFactory: func() (YahooSearch, error) {
			return &fakeYahooSearch{err: errors.New("upstream unavailable")}, nil
		},
	})
	_, err := provider.SearchInstruments(context.Background(), "NVDA", "stock", 8)
	assertProviderCode(t, err, domain.ErrProviderUnavailable)
}

func TestYahooChartProviderSearchRequiresQuery(t *testing.T) {
	provider := NewYahooChartProviderWithOptions(YahooChartProviderOptions{
		SearchFactory: func() (YahooSearch, error) {
			t.Fatal("empty query should not create a search client")
			return nil, nil
		},
	})
	_, err := provider.SearchInstruments(context.Background(), "   ", "stock", 8)
	assertProviderCode(t, err, domain.ErrValidation)
}
