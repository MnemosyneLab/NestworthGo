package application

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// mappedMetalFuturesRange mimics the Yahoo COMEX adapter: one policy-derived
// daily reference per calendar day in the requested range.
func mappedMetalFuturesRange(rng DateRange, base int) MappingOutcome[InstrumentDailyObservation] {
	start, _ := time.Parse(time.DateOnly, string(rng.Start))
	end, _ := time.Parse(time.DateOnly, string(rng.End))
	batch := HistoryBatch[InstrumentDailyObservation]{Evidence: ResponseEvidence{Adapter: "go-yfinance", PriceBasis: PriceBasisYahooClose, SourcePolicy: string(PriceBasisYahooClose), TimestampBasis: TimestampBasisPolicyDerived, SessionPolicy: "yahoo_comex_daily_reference_v1"}}
	for day, n := start, 0; !day.After(end); day, n = day.AddDate(0, 0, 1), n+1 {
		date := day.Format(time.DateOnly)
		eligible, _ := domain.MetalDailyBarEligibleAt(date)
		price, _ := domain.ParseUnitPrice(strconv.Itoa(base + n*10))
		batch.Observations = append(batch.Observations, InstrumentDailyObservation{MarketDate: MarketDate(date), Value: price.Canonical(), Currency: "USD", ValueEffectiveAt: eligible, Kind: InstrumentObservationClose, PriceBasis: PriceBasisYahooClose, TimestampBasis: TimestampBasisPolicyDerived})
	}
	batch.VerifiedRanges = []DateRange{rng}
	return MappingOutcome[InstrumentDailyObservation]{Status: MappingMapped, Batch: batch}
}

// A synced precious-metal instrument must expose the same descriptive period
// performance as stocks and funds. Before, the type gate and the futures
// reference's policy-derived timestamp both made the report unavailable.
func TestPreciousMetalSyncedHistoryHasPeriodPerformance(t *testing.T) {
	t.Parallel()
	tiingo := &syncFakeProvider{key: TiingoProviderKey}
	var requested []DateRange
	yahoo := &syncFakeProvider{key: YahooFinanceProviderKey, history: func(_ context.Context, identity InstrumentMarketIdentity, rng DateRange, _ int) (MappingOutcome[InstrumentDailyObservation], error) {
		if identity.ProviderSymbol != "GC=F" {
			return MappingOutcome[InstrumentDailyObservation]{Status: MappingUnsupported, Reason: "unsupported_price_basis"}, nil
		}
		requested = append(requested, rng)
		return mappedMetalFuturesRange(rng, 2000), nil
	}}
	service, _, _, _ := newSyncFixture(t, tiingo, yahoo)
	ctx := context.Background()
	metal, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Gold", Type: "precious_metal", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "GC=F", MarketCode: domain.MetalFuturesMarket, MetalTemplate: "gold", QuantityUnit: "troy_oz"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartMarketDataSync(ctx, SyncRequest{Scope: SyncScopeInstrument, InstrumentID: metal.ID.String()}); err != nil {
		t.Fatal(err)
	}
	waitSyncTerminal(t, service)
	if len(requested) == 0 {
		t.Fatal("metal history was never requested")
	}
	report, err := service.InstrumentQuoteReturns(ctx, metal.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture only syncs a few days, shorter than the 7-day window, so the
	// returns are hidden for insufficient history; the type must still be supported.
	if report.UnavailableReason == "unsupported_instrument" {
		t.Fatalf("precious metal performance unsupported: %+v (requested %+v)", report, requested)
	}
	for _, item := range report.Items {
		if item.WindowDays == 0 || item.ElapsedDays < 1 || item.PeriodPercent == "" || item.PeriodPercent == "0" {
			t.Fatalf("return %+v", item)
		}
	}
	// All history is a chart period only; it still has a headline and a daily chart.
	all, err := service.InstrumentQuotePerformanceSeries(ctx, metal.ID, 0)
	if err != nil || all.Item == nil || all.Item.WindowDays != 0 || len(all.Points) < 2 || all.From != all.Item.BaselineOn || all.To != all.Item.LatestOn {
		t.Fatalf("all-history series %+v %v", all, err)
	}
	if _, err := service.InstrumentQuotePerformanceSeries(ctx, metal.ID, 45); err == nil {
		t.Fatal("an unsupported period was accepted")
	}
	// The series the chart reads is independent of the performance projection.
	series, err := service.InstrumentQuoteSeries(ctx, metal.ID, domain.TrendAllTime, domain.QuoteSourceFilterAll)
	if err != nil || len(series.Points) < 2 {
		t.Fatalf("metal chart series %+v %v", series, err)
	}
}

// Market Data history keeps every observation, including non-daily manual
// quotes, while the holding performance chart only reads eligible daily closes.
func TestPerformanceSeriesLeavesMarketDataHistoryUnchanged(t *testing.T) {
	t.Parallel()
	service, _, _, _ := newSyncFixture(t, &syncFakeProvider{key: TiingoProviderKey}, &syncFakeProvider{key: YahooFinanceProviderKey})
	ctx := context.Background()
	stock, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Manual stock", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	for _, price := range []string{"10", "11"} {
		if _, err := service.AppendManualInstrumentQuote(ctx, stock.ID, price, "", false); err != nil {
			t.Fatal(err)
		}
	}
	market, err := service.InstrumentQuoteSeries(ctx, stock.ID, domain.TrendAllTime, domain.QuoteSourceFilterAll)
	if err != nil || len(market.Observations) != 2 {
		t.Fatalf("market data history changed: %+v %v", market, err)
	}
	performance, err := service.InstrumentQuotePerformanceSeries(ctx, stock.ID, 0)
	if err != nil || len(performance.Points) != 0 || performance.Item != nil || performance.UnavailableReason != "insufficient_daily_history" {
		t.Fatalf("non-daily quotes reached the performance chart: %+v %v", performance, err)
	}
	managed, err := service.InstrumentQuoteReturns(ctx, stock.ID)
	if err != nil || len(managed.Items) != 0 {
		t.Fatalf("returns %+v %v", managed, err)
	}
}
