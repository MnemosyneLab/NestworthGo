package mcpserver

import (
	"fmt"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
)

func responseReviewFixture(t *testing.T, n int) ledgerFixture {
	t.Helper()
	fx := newLedgerFixtureWithClock(t, func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	quotes := []application.AgentMarketDataItem{}
	for i := 0; i < n; i++ {
		instrument, err := fx.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: fmt.Sprintf("Synthetic investment %03d", i), Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = fx.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: fx.brokerage, InstrumentID: instrument.ID.String(), Quantity: "10", UnitCost: "1.23456789"}); err != nil {
			t.Fatal(err)
		}
		quotes = append(quotes, application.AgentMarketDataItem{InstrumentID: instrument.ID.String(), Currency: "USD", Kind: "close", Date: "2026-09-01", Timezone: "UTC", Value: "1.23456789", SourceTitle: "Synthetic valuation evidence"})
	}
	if _, err := fx.app.ImportAgentMarketData(t.Context(), domain.NewActivityID().String(), application.AgentMarketDataInput{Items: quotes}); err != nil {
		t.Fatal(err)
	}
	fx.app.SetClock(func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) })
	return fx
}

func TestResponseReviewMeasurements(t *testing.T) {
	for _, n := range []int{3, 80} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			fx := responseReviewFixture(t, n)
			overview, err := fx.app.HistoricalOverview(t.Context(), "2026-10-01", "2026-10-10")
			if err != nil {
				t.Fatal(err)
			}
			series, err := fx.app.HistoricalOverviewSeries(t.Context(), "2026-10-01", "2026-10-10")
			if err != nil {
				t.Fatal(err)
			}
			accounts, err := account.NewService(fx.app).AccountValuations(t.Context(), account.AccountFilterRequest{})
			if err != nil {
				t.Fatal(err)
			}
			for name, v := range map[string]any{"overview": overview, "series": series, "accounts": accounts} {
				raw, _ := jsoncodec.Marshal(v)
				wire, err := financialContextWireSize(v)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("synthetic holdings=%d %s data=%d complete-wire-reserve=%d", n, name, len(raw), wire)
			}
			c := ledgerSession(t, fx)
			listed, err := c.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := jsoncodec.Marshal(listed)
			t.Logf("discovery tools=%d bytes=%d", len(listed.Tools), len(raw))
		})
	}
}
