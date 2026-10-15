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
	report, err := service.InstrumentQuotePeriods(ctx, metal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.UnavailableReason != "" || len(report.Items) == 0 {
		t.Fatalf("precious metal performance unavailable: %+v (requested %+v)", report, requested)
	}
	all := report.Items[len(report.Items)-1]
	if all.WindowDays != 0 || all.ElapsedDays < 1 || all.PeriodPercent == "" || all.PeriodPercent == "0" {
		t.Fatalf("all-history change %+v", all)
	}
	// The series the chart reads is independent of the performance projection.
	series, err := service.InstrumentQuoteSeries(ctx, metal.ID, domain.TrendAllTime, domain.QuoteSourceFilterAll)
	if err != nil || len(series.Points) < 2 {
		t.Fatalf("metal chart series %+v %v", series, err)
	}
}
