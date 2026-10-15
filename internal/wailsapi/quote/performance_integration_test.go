package quote_test

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	"github.com/waltwang/nestworth-go/internal/wailsapi/quote"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

// Exercise the merged schema and public writers through real SQLite, followed
// by the exact bound reads used by PR91. All evidence is synthetic and local.
func TestPerformanceBindingsDailySeriesAndFixedReturns(t *testing.T) {
	for _, kind := range []string{"stock", "bank_investment_product", "precious_metal", "crypto"} {
		t.Run(kind, func(t *testing.T) {
			app := wailstest.NewService(t)
			app.SetClock(func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) })
			ctx := t.Context()
			if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{HouseholdName: "Synthetic performance", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner"}}); err != nil {
				t.Fatal(err)
			}
			i, err := instrument.NewService(app).CreateInstrument(ctx, instrument.InstrumentRequest{Name: "Synthetic instrument", Type: kind, QuoteCurrency: "USD", QuoteSource: "agent"})
			if err != nil {
				t.Fatal(err)
			}
			end := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
			observations := []application.AgentMarketDataItem{}
			observationKind := "close"
			if kind == "bank_investment_product" {
				observationKind = "nav"
			}
			for n, days := range []int{365, 180, 90, 30, 7, 0} {
				observations = append(observations, application.AgentMarketDataItem{InstrumentID: i.ID, Currency: "USD", Value: strconv.Itoa(100 + n), Kind: observationKind, Date: end.AddDate(0, 0, -days).Format(time.DateOnly), Timezone: "UTC", SourceTitle: "Synthetic daily fixture"})
			}
			observations = append(observations, application.AgentMarketDataItem{InstrumentID: i.ID, Currency: "USD", Value: "999", Kind: "latest", QuotedAt: "2026-10-14T23:00:00Z", SourceTitle: "Synthetic realtime fixture"})
			if _, err := app.ImportAgentMarketData(ctx, uuid.NewString(), application.AgentMarketDataInput{Items: observations}); err != nil {
				t.Fatal(err)
			}
			service := quote.NewService(app)
			returns, err := service.InstrumentQuoteReturns(ctx, i.ID)
			if err != nil {
				t.Fatal(err)
			}
			windows := []int{}
			for _, item := range returns.Items {
				windows = append(windows, item.WindowDays)
				if item.LatestOn != "2026-10-14" || item.ElapsedDays != item.WindowDays {
					t.Fatalf("return dates: %+v", item)
				}
				if kind == "bank_investment_product" && item.AnnualizedPercent == nil {
					t.Fatalf("NAV annualization lost: %+v", item)
				}
			}
			if !reflect.DeepEqual(windows, []int{7, 30, 90, 180, 365}) {
				t.Fatalf("fixed returns changed: %v", windows)
			}
			for _, days := range []int{7, 30, 90, 180, 365, 0} {
				series, err := service.InstrumentQuotePerformanceSeries(ctx, i.ID, days)
				if err != nil || series.Item == nil || len(series.Points) < 2 {
					t.Fatalf("series %d: %+v %v", days, series, err)
				}
				if series.From != series.Item.BaselineOn || series.To != series.Item.LatestOn || series.Points[0].EffectiveDate != series.From || series.Points[len(series.Points)-1].EffectiveDate != series.To {
					t.Fatalf("headline/chart interval mismatch: %+v", series)
				}
				for _, p := range series.Points {
					if p.ObservationKind != "close" || p.Value == "999" {
						t.Fatalf("intraday entered daily chart: %+v", p)
					}
				}
			}
			history, err := service.InstrumentQuoteSeries(ctx, i.ID, "all", "all")
			if err != nil || len(history.Observations) != 7 {
				t.Fatalf("raw Market Data history changed: %+v %v", history, err)
			}
			// PR89's service remains usable against the same database schema.
			if _, err := reconciliation.NewService(app).List(ctx); err != nil {
				t.Fatalf("reconciliation schema/read: %v", err)
			}
		})
	}
}
