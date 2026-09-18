package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestInstrumentHistoryStartUsesOwnershipAndCorrections(t *testing.T) {
	origin := domain.HistoryOrigin{Timezone: "Asia/Singapore", StartedAt: time.Date(2024, 9, 14, 0, 0, 0, 0, time.UTC)}
	initial := domain.NewInstrumentID()
	later := domain.NewInstrumentID()
	reversed := domain.NewInstrumentID()
	qty, _ := domain.ParseQuantity("1")
	zero, _ := domain.ParseQuantity("0")
	effect := func(id domain.InstrumentID) []domain.ActivityEffect {
		return []domain.ActivityEffect{{Target: domain.EffectTargetHoldingQuantity, Direction: domain.EffectAdded, InstrumentID: &id, Quantity: &qty}}
	}
	removedID := domain.NewActivityID()
	activities := []domain.Activity{
		{ID: removedID, EffectiveAt: origin.StartedAt.AddDate(0, 1, 0), Effects: effect(reversed)},
		{ID: domain.NewActivityID(), ReversesActivityID: &removedID, EffectiveAt: origin.StartedAt.AddDate(0, 1, 0)},
		{ID: domain.NewActivityID(), EffectiveAt: time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC), Effects: effect(later)},
		// Another account's backdated purchase determines the shared instrument start.
		{ID: domain.NewActivityID(), EffectiveAt: time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC), Effects: effect(later)},
	}
	starts, err := instrumentHistoryStartDates(origin, []domain.HistoryOriginComponent{
		{Kind: domain.HistoryOriginHoldingQuantity, InstrumentID: &initial, Quantity: &qty},
		{Kind: domain.HistoryOriginHoldingQuantity, InstrumentID: &later, Quantity: &zero},
	}, activities)
	if err != nil {
		t.Fatal(err)
	}
	if starts[initial] != "2024-09-14" || starts[later] != "2026-06-02" || starts[reversed] != "" {
		t.Fatalf("starts = %+v", starts)
	}
}

func TestUnusedInstrumentStartsSevenDaysBeforeCreation(t *testing.T) {
	service, _, _, _ := newSyncFixture(t, &syncFakeProvider{key: TiingoProviderKey}, &syncFakeProvider{key: YahooFinanceProviderKey})
	instrument, err := service.CreateInstrument(context.Background(), InstrumentInput{Name: "Unheld", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "UNHELD", MarketCode: "US"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanHistorySync(context.Background(), HistorySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, need := range plan.Instruments {
		if need.InstrumentID == instrument.ID {
			if need.LatestOnly || len(need.FetchRanges) == 0 || need.FetchRange.Start != "2026-09-03" {
				t.Fatalf("unused need = %+v", need)
			}
			return
		}
	}
	t.Fatal("new instrument target missing")
}

func TestCoinGeckoHistoryPlanClipsOnlyUnavailableDates(t *testing.T) {
	coverage := domain.InstrumentHistoryCoverage{InstrumentType: "crypto", ProviderKey: CoinGeckoProviderKey, ProviderSymbol: "bitcoin", Market: "CRYPTO"}
	need, err := planInstrumentRepairNeed(coverage, "2024-09-14", "2026-09-17")
	if err != nil {
		t.Fatal(err)
	}
	need, err = applyHistorySyncPolicy(need, coverage, "2026-09-17", time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(need.UnavailableRanges) != 1 || len(need.FetchRanges) != 1 || need.FetchRanges[0].Start != "2025-09-19" || need.FetchRanges[0].End != "2026-09-17" {
		t.Fatalf("need = %+v", need)
	}
}

func TestCoinGeckoHistoryPersistsWithValuationPolicy(t *testing.T) {
	cg := &syncFakeProvider{key: CoinGeckoProviderKey, history: func(_ context.Context, identity InstrumentMarketIdentity, rng DateRange, _ int) (MappingOutcome[InstrumentDailyObservation], error) {
		stamp := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
		return MappingOutcome[InstrumentDailyObservation]{Status: MappingMapped, Batch: HistoryBatch[InstrumentDailyObservation]{
			Observations:   []InstrumentDailyObservation{{MarketDate: "2026-09-08", Value: "60000", Currency: "USD", ValueEffectiveAt: stamp, ProviderTimestamp: stamp, Kind: InstrumentObservationClose, PriceBasis: PriceBasis(domain.CoinGeckoDailyPriceBasis), TimestampBasis: TimestampBasisObservedPublication}},
			VerifiedRanges: []DateRange{rng}, Evidence: ResponseEvidence{Adapter: CoinGeckoProviderKey, SourcePolicy: domain.CoinGeckoDailyPriceBasis},
		}}, nil
	}}
	service, repo, _, _ := newSyncFixture(t, &syncFakeProvider{key: TiingoProviderKey}, &syncFakeProvider{key: YahooFinanceProviderKey}, cg)
	ctx := context.Background()
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Bitcoin", Type: "crypto", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: CoinGeckoProviderKey, ProviderSymbol: "BITCOIN"})
	if err != nil {
		t.Fatal(err)
	}
	if *instrument.ProviderSymbol != "bitcoin" || *instrument.MarketCode != "CRYPTO" {
		t.Fatalf("identity=%+v", instrument)
	}
	addSyncHoldings(t, service, instrument)
	plan, err := service.PlanHistorySync(ctx, HistorySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, need := range plan.Instruments {
		if need.InstrumentID == instrument.ID && need.HistoryStartDate != "2026-09-10" {
			t.Fatalf("start=%+v", need)
		}
	}
	if _, err := service.StartMarketDataSync(ctx, SyncRequest{Scope: SyncScopeInstrument, InstrumentID: instrument.ID.String()}); err != nil {
		t.Fatal(err)
	}
	service.syncWG.Wait()
	quotes, err := repo.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, quote := range quotes {
		if quote.ObservationKind == "close" {
			if quote.SourcePolicyVersion != domain.CoinGeckoDailyPriceBasis || !historicalInstrumentQualityMatches(quote) {
				t.Fatalf("unusable daily quote=%+v", quote)
			}
			return
		}
	}
	t.Fatal("CoinGecko history did not persist")
}

type fencedCoinGeckoBatch struct {
	*syncFakeProvider
	duringFetch func()
	quote       LatestInstrumentQuote
}

func (p *fencedCoinGeckoBatch) LatestInstruments(_ context.Context, identities []InstrumentMarketIdentity) map[string]LatestInstrumentResult {
	p.duringFetch()
	results := map[string]LatestInstrumentResult{}
	for _, identity := range identities {
		results[LatestInstrumentIdentityKey(identity)] = LatestInstrumentResult{Quote: p.quote}
	}
	return results
}
func TestCoinGeckoBatchCannotPersistAcrossWorkspaceFence(t *testing.T) {
	p := &fencedCoinGeckoBatch{syncFakeProvider: &syncFakeProvider{key: CoinGeckoProviderKey}}
	service, repo, _, _ := newSyncFixture(t, p)
	ctx := context.Background()
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Bitcoin", Type: "crypto", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: CoinGeckoProviderKey, ProviderSymbol: "bitcoin"})
	if err != nil {
		t.Fatal(err)
	}
	price, _ := domain.ParseUnitPrice("60000")
	p.quote = LatestInstrumentQuote{Price: price, Currency: "USD", SourceKey: CoinGeckoProviderKey, QuotedAt: service.clock()}
	p.duringFetch = func() { service.refreshEpoch.Add(1) }
	result, err := service.RefreshInstrument(ctx, instrument.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Status != RefreshFailed {
		t.Fatalf("result=%+v", result)
	}
	quotes, err := repo.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(quotes) != 0 {
		t.Fatalf("quotes persisted across workspace fence: %v %v", quotes, err)
	}
}
