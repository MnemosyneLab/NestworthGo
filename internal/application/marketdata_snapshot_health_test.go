package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestIncompleteMetalSnapshotHealthLifecycle(t *testing.T) {
	now := time.Date(2026, 9, 18, 4, 0, 0, 0, time.UTC)
	s, ctx := newProductTestServiceTZ(t, now, "Asia/Singapore")
	s.setClock(func() time.Time { return now })
	provider := &countingProvider{inner: &syncFakeProvider{key: YahooFinanceProviderKey}}
	s.SetMarketDataRegistry(NewMarketDataRegistry(provider))
	account := seedHoldingsCash(t, s, ctx, "10000")
	metal, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Gold", Type: "precious_metal", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "GC=F", MarketCode: domain.MetalFuturesMarket, MetalTemplate: "gold", QuantityUnit: "troy_oz"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateHolding(ctx, HoldingInput{AccountID: account.String(), InstrumentID: metal.ID.String(), Quantity: "1", UnitCost: "4000"}); err != nil {
		t.Fatal(err)
	}
	household, err := s.repository.Household(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repo := s.repository.(*sqlite.Repository)
	commit := sqlite.InstrumentHistoryCommit{HouseholdID: household.ID, InstrumentID: metal.ID, ProviderKey: YahooFinanceProviderKey, ProviderSymbol: "GC=F", QuoteCurrency: "USD", Market: domain.MetalFuturesMarket, Status: "mapped", SourcePolicy: string(PriceBasisYahooClose), Adapter: "yahoo_chart", VerifiedRanges: []sqlite.DateSpan{{Start: "2026-09-11", End: "2026-09-18"}}}
	for _, date := range []string{"2026-09-17", "2026-09-18"} {
		eligible, err := domain.MetalDailyBarEligibleAt(date)
		if err != nil {
			t.Fatal(err)
		}
		commit.Observations = append(commit.Observations, sqlite.InstrumentHistoryObservation{MarketDate: date, Value: "4000", Currency: "USD", ValueEffectiveAt: eligible, Kind: "close", PriceBasis: string(PriceBasisYahooClose), TimestampBasis: string(TimestampBasisPolicyDerived)})
	}
	now = time.Date(2026, 9, 22, 3, 0, 0, 0, time.UTC)
	commit.FetchedAt = now
	if _, err := repo.CommitInstrumentHistory(ctx, commit); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RebuildDirtySnapshots(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom != nil {
		t.Fatal("fixture must have a completed cursor")
	}
	report, err := s.ScanMarketDataHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Healthy {
		t.Fatal("incomplete snapshot incorrectly reported healthy")
	}
	var pending *HealthIssue
	for i := range report.Issues {
		if report.Issues[i].Kind == HealthKindHistoryPending {
			pending = &report.Issues[i]
		}
	}
	if pending == nil {
		t.Fatalf("missing waiting issue: %+v", report.Issues)
	}
	if pending.RangeStart != "2026-09-21" || pending.InstrumentID != metal.ID.String() || pending.NextCheckAt != "2026-09-22T12:00:00+08:00" || pending.Executable {
		t.Fatalf("pending issue: %+v", pending)
	}
	if provider.networkCalls() != 0 {
		t.Fatal("local health scan contacted provider")
	}
	now = time.Date(2026, 9, 22, 4, 1, 0, 0, time.UTC)
	report, err = s.ScanMarketDataHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if hasKind(report, HealthKindHistoryPending) || !hasIssueForInstrument(report, metal.ID.String(), HealthKindMissingInstrumentHistory, true) {
		t.Fatalf("after deadline: %+v", report.Issues)
	}
	// A coverage-only update (without changing any price) must dirty the old
	// snapshot and resolve its completeness when the sync rebuild runs.
	commit.Observations = nil
	commit.VerifiedRanges = []sqlite.DateSpan{{Start: "2026-09-21", End: "2026-09-21"}}
	commit.FetchedAt = now
	result, err := repo.CommitInstrumentHistory(ctx, commit)
	if err != nil {
		t.Fatal(err)
	}
	if result.PersistedObservations != 0 || result.CoverageDays != 1 {
		t.Fatalf("expected coverage-only update: %+v", result)
	}
	state, err = repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom == nil || *state.DirtyFrom > "2026-09-21" {
		t.Fatalf("coverage did not dirty snapshots: %+v", state)
	}
	// Simulate the legacy missing-dirty-marker case: health and Repair All must
	// still find the stored incomplete snapshot now that its inputs are ready.
	if err := repo.CompleteDailySnapshotRange(ctx, household.ID, "2026-09-21", now); err != nil {
		t.Fatal(err)
	}
	report, err = s.ScanMarketDataHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Healthy || len(readySnapshotDates(report.Issues)) != 1 {
		t.Fatalf("ready snapshot not repairable: %+v", report)
	}
	preview, err := s.PreviewMarketDataSync(ctx, SyncRequest{Scope: SyncScopeRepairAll})
	if err != nil {
		t.Fatal(err)
	}
	if preview.SnapshotWorkEstimate != 1 {
		t.Fatalf("snapshot estimate: %d", preview.SnapshotWorkEstimate)
	}
	rebuilt, err := s.RebuildDirtySnapshots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt != 1 {
		t.Fatalf("rebuilt %d", rebuilt)
	}
	report, err = s.ScanMarketDataHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Healthy {
		t.Fatalf("still incomplete: %+v", report.Issues)
	}
	snapshots, err := repo.ListDailyValuationSnapshots(ctx, household.ID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range snapshots {
		if !snapshot.Complete {
			t.Fatalf("snapshot %s still incomplete", snapshot.LocalDate)
		}
	}
	if provider.networkCalls() != 0 {
		t.Fatal("repair diagnosis unexpectedly contacted provider")
	}
}
