package marketdata

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/secrets"
)

func TestControlledLiveProviderChecks(t *testing.T) {
	if os.Getenv("NESTWORTH_LIVE_MARKET_DATA") != "1" {
		t.Skip("NOT RUN: set NESTWORTH_LIVE_MARKET_DATA=1 for controlled live provider HTTP")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	frankfurter := NewFrankfurterProvider(nil)
	latest, err := frankfurter.LatestFX(ctx, application.FXMarketIdentity{BaseCurrency: "USD", QuoteCurrency: "SGD"})
	if err != nil {
		t.Fatalf("Frankfurter latest live: %v", err)
	}
	if latest.Rate.Canonical() == "" || latest.SourceKey != frankfurterProviderKey {
		t.Fatalf("Frankfurter latest live quote = %#v", latest)
	}
	history, err := frankfurter.FXDailyHistory(ctx, application.FXMarketIdentity{BaseCurrency: "USD", QuoteCurrency: "SGD"}, application.DateRange{Start: "2026-09-04", End: "2026-09-08"})
	if err != nil {
		t.Fatalf("Frankfurter history live: %v", err)
	}
	if history.Status != application.MappingMapped || len(history.Batch.Observations) == 0 {
		t.Fatalf("Frankfurter history live = %+v", history)
	}

	yahoo := NewYahooChartProvider(nil)
	yahooHistory, err := yahoo.InstrumentDailyHistory(ctx, application.InstrumentMarketIdentity{ProviderSymbol: "AAPL", QuoteCurrency: "USD", Market: "US"}, application.DateRange{Start: "2026-09-04", End: "2026-09-08"})
	if err != nil {
		t.Fatalf("Yahoo history live: %v", err)
	}
	if len(yahooHistory.Batch.Observations) != 0 {
		t.Fatalf("Yahoo live history invented closes: %+v", yahooHistory.Batch.Observations)
	}

	if os.Getenv("TIINGO_API_KEY") == "" {
		t.Log("Tiingo latest live NOT RUN: TIINGO_API_KEY unset")
		return
	}
	store := secrets.NewMemoryStore()
	if _, err := store.Put(ctx, application.TiingoSecretRef(), []byte(os.Getenv("TIINGO_API_KEY"))); err != nil {
		t.Fatal(err)
	}
	tiingo := NewTiingoProvider(store, nil)
	quote, err := tiingo.LatestInstrument(ctx, application.InstrumentMarketIdentity{ProviderSymbol: "AAPL", QuoteCurrency: "USD", Market: "US"})
	if err != nil {
		t.Fatalf("Tiingo latest live: %v", err)
	}
	if quote.Price.Canonical() == "" || !quote.Delayed {
		t.Fatalf("Tiingo latest live = %#v", quote)
	}
}
