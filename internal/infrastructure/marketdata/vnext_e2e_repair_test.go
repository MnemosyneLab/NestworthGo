package marketdata

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestE2ETiingoUSManualFXRepairProbe(t *testing.T) {
	sgt, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	originClock := time.Date(2026, 9, 6, 12, 0, 0, 0, sgt)
	repairClock := time.Date(2026, 9, 10, 0, 5, 0, 0, sgt)
	ctx := context.Background()

	svc, database, repo, household, instrument, owner := seedRepairWorkspace(t, originClock)
	seedRepairPortfolio(t, svc, repo, originClock, instrument, owner)
	svc.SetClock(func() time.Time { return repairClock })
	fetchedAt := repairClock.UTC()

	planBefore, err := svc.PlanMarketDataRepair(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if planBefore.LastFinalizedMarketDate != "2026-09-08" {
		t.Fatalf("last finalized = %q, want 2026-09-08", planBefore.LastFinalizedMarketDate)
	}
	if !planBefore.ManualFX {
		t.Fatal("expected ManualFX fixture path")
	}
	if len(planBefore.Instruments) != 1 {
		t.Fatalf("instruments = %d, want 1", len(planBefore.Instruments))
	}
	instPlan := planBefore.Instruments[0]
	if !instPlan.OpeningAnchorMissing {
		t.Fatal("expected opening-anchor missing before persist")
	}
	if instPlan.FetchRange.Start != "2025-09-06" || planBefore.Instruments[0].FetchRange.End != "2026-09-08" {
		t.Fatalf("fetch range = %+v, want lookback 2025-09-06 through last finalized 2026-09-08", instPlan.FetchRange)
	}
	if len(instPlan.MissingRanges) == 0 {
		t.Fatal("expected coverage gap before persist")
	}

	completeMeta, completeBody := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	complete, err := QualifyTiingoHistory(completeMeta, completeBody)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, complete, fetchedAt)); err != nil {
		t.Fatal(err)
	}

	state, err := repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom == nil || *state.DirtyFrom != "2026-09-06" {
		t.Fatalf("dirty_from after persist = %v, want origin 2026-09-06", state.DirtyFrom)
	}
	if state.DirtyTo == nil || *state.DirtyTo != "2026-09-09" {
		t.Fatalf("dirty_to after persist = %v, want last closed local 2026-09-09", state.DirtyTo)
	}

	snaps, err := repo.ListDailyValuationSnapshots(ctx, household.ID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 0 {
		t.Fatalf("persist without rebuild should leave 0 snapshots (interruption), got %d", len(snaps))
	}

	planAfter, err := svc.PlanMarketDataRepair(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if planAfter.Instruments[0].OpeningAnchorMissing {
		t.Fatal("opening anchor should resolve after 2026-09-04 close persisted")
	}
	if planAfter.Instruments[0].OpeningAnchorDate != "2026-09-04" {
		t.Fatalf("opening anchor = %q, want 2026-09-04", planAfter.Instruments[0].OpeningAnchorDate)
	}

	cutoff, err := application.HouseholdCutoffAt("2026-09-09", "Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	wantCutoff := time.Date(2026, 9, 9, 15, 59, 59, 999000000, time.UTC)
	if !cutoff.Equal(wantCutoff) {
		t.Fatalf("household cutoff = %s, want %s", cutoff.UTC().Format(time.RFC3339Nano), wantCutoff.Format(time.RFC3339Nano))
	}

	if _, err := svc.RebuildDirtySnapshots(ctx); err != nil {
		t.Fatal(err)
	}
	state, err = repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom != nil || state.DirtyTo != nil {
		t.Fatalf("expected dirty range cleared after rebuild, got from=%v to=%v", state.DirtyFrom, state.DirtyTo)
	}

	completeOracle := mustOracleWithoutCorrection(t)
	assertClosedSnapshotsMatchOracle(t, database, repo, household.ID, instrument.ID, completeOracle, 1)

	trend, err := svc.NetWorthTrend(ctx, domain.TrendAllTime)
	if err != nil {
		t.Fatal(err)
	}
	assertTrendAssets(t, trend, "2026-09-06", "3850.875")
	assertTrendAssets(t, trend, "2026-09-07", "3850.875")
	assertTrendAssets(t, trend, "2026-09-08", "3861")
	assertTrendAssets(t, trend, "2026-09-09", "3850.875")

	again, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, complete, fetchedAt))
	if err != nil {
		t.Fatal(err)
	}
	if !again.Unchanged {
		t.Fatalf("idempotent complete persist status = %+v, want Unchanged", again)
	}
	if _, err := svc.RebuildDirtySnapshots(ctx); err != nil {
		t.Fatal(err)
	}
	assertClosedSnapshotsMatchOracle(t, database, repo, household.ID, instrument.ID, completeOracle, 1)

	correctionMeta, correctionBody := mustLoadVNext(t, "providers/tiingo/aapl-eod-correction.json")
	correction, err := QualifyTiingoHistory(correctionMeta, correctionBody)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, correction, fetchedAt.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	state, err = repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom == nil || *state.DirtyFrom == "" || state.DirtyTo == nil || *state.DirtyTo != "2026-09-09" {
		t.Fatalf("correction should dirty through 2026-09-09, got from=%v to=%v", state.DirtyFrom, state.DirtyTo)
	}

	if _, err := svc.RebuildDirtySnapshots(ctx); err != nil {
		t.Fatal(err)
	}
	fullOracle := mustFullOracle(t)
	assertClosedSnapshotsMatchOracle(t, database, repo, household.ID, instrument.ID, fullOracle, 2)

	correctionAgain, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, correction, fetchedAt.Add(2*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	if !correctionAgain.Unchanged {
		t.Fatalf("idempotent correction persist = %+v, want Unchanged", correctionAgain)
	}
	if _, err := svc.RebuildDirtySnapshots(ctx); err != nil {
		t.Fatal(err)
	}
	assertClosedSnapshotsMatchOracle(t, database, repo, household.ID, instrument.ID, fullOracle, 2)

	writeRepairProbeArtifact(t, household.ID, instrument.ID, planBefore, planAfter, fullOracle)
}

func TestE2ERepairInterruptionRecoversFromDirtyRange(t *testing.T) {
	sgt, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	originClock := time.Date(2026, 9, 6, 12, 0, 0, 0, sgt)
	repairClock := time.Date(2026, 9, 10, 0, 5, 0, 0, sgt)
	ctx := context.Background()
	svc, database, repo, household, instrument, owner := seedRepairWorkspace(t, originClock)
	seedRepairPortfolio(t, svc, repo, originClock, instrument, owner)
	svc.SetClock(func() time.Time { return repairClock })
	completeMeta, completeBody := mustLoadVNext(t, "providers/tiingo/aapl-eod-complete.json")
	complete, err := QualifyTiingoHistory(completeMeta, completeBody)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitInstrumentHistory(ctx, instrumentCommit(household, instrument, complete, repairClock.UTC())); err != nil {
		t.Fatal(err)
	}
	state, err := repo.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DirtyFrom == nil || *state.DirtyFrom != "2026-09-06" || state.DirtyTo == nil || *state.DirtyTo != "2026-09-09" {
		t.Fatalf("interrupt dirty = from=%v to=%v", state.DirtyFrom, state.DirtyTo)
	}
	snaps, err := repo.ListDailyValuationSnapshots(ctx, household.ID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 0 {
		t.Fatalf("crash before rebuild should leave 0 snapshots, got %d", len(snaps))
	}
	if _, err := svc.RebuildDirtySnapshots(ctx); err != nil {
		t.Fatal(err)
	}
	assertClosedSnapshotsMatchOracle(t, database, repo, household.ID, instrument.ID, mustOracleWithoutCorrection(t), 1)
}

func seedRepairWorkspace(t *testing.T, originClock time.Time) (*application.Service, *sqlite.DB, *sqlite.Repository, domain.Household, domain.Instrument, domain.MemberID) {
	t.Helper()
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "repair.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repo := sqlite.NewRepository(database)
	service := application.NewService(repo)
	service.SetClock(func() time.Time { return originClock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Repair", BaseCurrency: "SGD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil || bootstrap.Household == nil {
		t.Fatalf("bootstrap: %v", err)
	}
	instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Apple", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider",
		ProviderKey: application.TiingoProviderKey, ProviderSymbol: "AAPL", MarketCode: "US",
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, database, repo, *bootstrap.Household, instrument, bootstrap.Members[0].ID
}

func seedRepairPortfolio(t *testing.T, svc *application.Service, repo *sqlite.Repository, originClock time.Time, instrument domain.Instrument, owner domain.MemberID) {
	t.Helper()
	ctx := context.Background()
	acc, err := svc.CreateAccount(ctx, application.AccountInput{
		Name: "Holdings", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{owner},
	})
	if err != nil {
		t.Fatal(err)
	}
	price, err := domain.ParseUnitPrice("185.25")
	if err != nil {
		t.Fatal(err)
	}
	session, err := domain.ResolveEquitySessionClose("2026-09-04", "US", domain.SessionEvidence{Kind: domain.SessionKindRegular, Timezone: domain.USEquitySessionTimezone, CloseClock: domain.USEquityRegularCloseClock})
	if err != nil || session.Status != "mapped" {
		t.Fatalf("opening-anchor session: %#v err=%v", session, err)
	}
	quote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{
		UnitPrice: price, SourceKind: domain.QuoteSourceProvider, SourceKey: application.TiingoProviderKey,
		QuotedAt: session.CloseInstant, Delayed: true,
	}, originClock)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendInstrumentQuote(ctx, quote); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateHolding(ctx, application.HoldingInput{AccountID: acc.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AppendAccountCashValue(ctx, acc.Account.ID, "1000", "USD", originClock.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetFXPreference(ctx, "USD", "SGD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AppendManualFXQuote(ctx, "USD", "SGD", "1.35", "2026-09-06T00:00:00+08:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
}

func assertClosedSnapshotsMatchOracle(t *testing.T, database *sqlite.DB, repo *sqlite.Repository, householdID domain.HouseholdID, instrumentID domain.InstrumentID, oracle domain.OracleSlice, minWedRevision int) {
	t.Helper()
	ctx := context.Background()
	snaps, err := repo.ListDailyValuationSnapshots(ctx, householdID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	byDate := map[string]domain.DailyValuationSnapshot{}
	for _, snap := range snaps {
		byDate[snap.LocalDate] = snap
	}
	wantByDate := map[string]domain.OracleSnapshot{}
	for _, snap := range oracle.Snapshots {
		wantByDate[snap.LocalDate] = snap
	}
	for _, localDate := range []string{"2026-09-06", "2026-09-07", "2026-09-08", "2026-09-09"} {
		want, ok := wantByDate[localDate]
		if !ok {
			t.Fatalf("oracle missing %s", localDate)
		}
		got, ok := byDate[localDate]
		if !ok {
			t.Fatalf("missing snapshot %s", localDate)
		}
		if got.AssetsAmount == nil || got.AssetsAmount.CanonicalAmount() != want.AssetsRounded {
			t.Fatalf("%s assets = %v, want %s", localDate, got.AssetsAmount, want.AssetsRounded)
		}
		if got.Complete != want.Complete {
			t.Fatalf("%s complete = %v, want %v", localDate, got.Complete, want.Complete)
		}
		item := snapshotInstrumentItem(t, got, instrumentID)
		if item.NativeAmount != want.NativeHoldingExact {
			t.Fatalf("%s native holding = %q, want %s", localDate, item.NativeAmount, want.NativeHoldingExact)
		}
		if item.BaseAmountExact != want.BaseHoldingExact {
			t.Fatalf("%s holding exact = %q, want %s", localDate, item.BaseAmountExact, want.BaseHoldingExact)
		}
		if item.QuoteID == nil || *item.QuoteID == "" {
			t.Fatalf("%s missing quote provenance", localDate)
		}
		if item.FXQuoteID == nil || *item.FXQuoteID == "" {
			t.Fatalf("%s missing FX provenance", localDate)
		}
		var revision int
		if err := database.SQL.QueryRow(`SELECT revision FROM instrument_quotes WHERE id = ?`, *item.QuoteID).Scan(&revision); err != nil {
			t.Fatal(err)
		}
		if localDate == "2026-09-09" && revision < minWedRevision {
			t.Fatalf("%s quote revision = %d, want >= %d", localDate, revision, minWedRevision)
		}
	}
}

func snapshotInstrumentItem(t *testing.T, snap domain.DailyValuationSnapshot, instrumentID domain.InstrumentID) domain.DailyValuationSnapshotItem {
	t.Helper()
	for _, item := range snap.Items {
		if item.InstrumentID != nil && *item.InstrumentID == instrumentID {
			return item
		}
	}
	t.Fatalf("snapshot %s missing instrument %s", snap.LocalDate, instrumentID)
	return domain.DailyValuationSnapshotItem{}
}

func assertTrendAssets(t *testing.T, trend domain.NetWorthTrend, localDate, want string) {
	t.Helper()
	for _, point := range trend.Points {
		if point.LocalDate != localDate {
			continue
		}
		if point.Assets == nil || point.Assets.CanonicalAmount() != want {
			t.Fatalf("analytics %s assets = %v, want %s", localDate, point.Assets, want)
		}
		if point.Status != domain.TrendPointComplete {
			t.Fatalf("analytics %s status = %s", localDate, point.Status)
		}
		return
	}
	t.Fatalf("analytics missing %s", localDate)
}

func mustFullOracle(t *testing.T) domain.OracleSlice {
	t.Helper()
	scenario, err := domain.FirstVerticalSliceScenario()
	if err != nil {
		t.Fatal(err)
	}
	slice, err := domain.EvaluateMarketDataOracle(scenario)
	if err != nil {
		t.Fatal(err)
	}
	return slice
}

func mustOracleWithoutCorrection(t *testing.T) domain.OracleSlice {
	t.Helper()
	scenario, err := domain.FirstVerticalSliceScenario()
	if err != nil {
		t.Fatal(err)
	}
	filtered := scenario.Closes[:0]
	for _, close := range scenario.Closes {
		if close.Revision >= 2 {
			continue
		}
		filtered = append(filtered, close)
	}
	scenario.Closes = filtered
	slice, err := domain.EvaluateMarketDataOracle(scenario)
	if err != nil {
		t.Fatal(err)
	}
	return slice
}

func writeRepairProbeArtifact(t *testing.T, householdID domain.HouseholdID, instrumentID domain.InstrumentID, before, after application.HistoryRepairPlan, oracle domain.OracleSlice) {
	t.Helper()
	dir := "/opt/cursor/artifacts"
	if _, err := os.Stat(dir); err != nil {
		return
	}
	payload := map[string]any{
		"scenario":                domain.FirstVerticalSliceScenarioID,
		"householdId":             householdID.String(),
		"instrumentId":            instrumentID.String(),
		"lastFinalizedMarketDate": after.LastFinalizedMarketDate,
		"openingAnchorBefore":     before.Instruments[0].OpeningAnchorMissing,
		"openingAnchorAfter":      after.Instruments[0].OpeningAnchorDate,
		"resolverPolicy":          after.ResolverPolicyVersion,
		"manualFx":                after.ManualFX,
		"oracleAssets": map[string]string{
			"2026-09-06": snapshotOracleAssets(oracle, "2026-09-06"),
			"2026-09-07": snapshotOracleAssets(oracle, "2026-09-07"),
			"2026-09-08": snapshotOracleAssets(oracle, "2026-09-08"),
			"2026-09-09": snapshotOracleAssets(oracle, "2026-09-09"),
		},
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "step03_e2e_repair_probe.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func snapshotOracleAssets(oracle domain.OracleSlice, localDate string) string {
	for _, snap := range oracle.Snapshots {
		if snap.LocalDate == localDate {
			return snap.AssetsRounded
		}
	}
	return ""
}
