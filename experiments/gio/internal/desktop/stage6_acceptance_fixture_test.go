package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

func openStage6Database(t *testing.T, path string, clock func() time.Time, providers ...application.MarketDataRegistryPort) (*sqlite.DB, *sqlite.Repository, *application.Service) {
	t.Helper()
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewRepository(db)
	svc := application.NewService(repo, providers...)
	appports.AttachSQLiteHistory(svc, repo)
	svc.SetLiveDatabasePath(db.Path)
	svc.SetClock(clock)
	return db, repo, svc
}

func stage6SeedBytes(t *testing.T, db *sqlite.DB) []byte {
	t.Helper()
	if err := db.CheckpointWAL(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(db.Path)
	if err != nil {
		t.Fatal(err)
	}
	return seed
}

func writeStage6Fixture(t *testing.T, dir string, seed []byte) string {
	t.Helper()
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "nestworth.db")
	if err := os.WriteFile(path, seed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"language":"en","theme":"light"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func seedStage6Snapshots(t *testing.T, start time.Time, closedDays int, dirty bool) ([]byte, domain.HouseholdID) {
	t.Helper()
	ctx := t.Context()
	now := start
	db, _, svc := openStage6Database(t, filepath.Join(t.TempDir(), "source.db"), func() time.Time { return now },
		application.NewMarketDataRegistry(&unavailableOverviewProvider{}))
	if err := svc.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic Stage 6 snapshots", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	bs, err := svc.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateAccount(ctx, application.AccountInput{Name: "Synthetic cash", AccountType: "cash_on_hand", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD",
		InitialAmount: "125.25", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bs.Members[0].ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = start.AddDate(0, 0, closedDays)
	for offset := 0; offset < closedDays; offset += 31 {
		days := min(31, closedDays-offset)
		from, to := start.AddDate(0, 0, offset), start.AddDate(0, 0, offset+days-1)
		if n, err := svc.RebuildHistoricalSnapshots(ctx, from.Format(time.DateOnly), to.Format(time.DateOnly)); err != nil || n != days {
			t.Fatalf("synthetic seed rebuilt %d of %d days: %v", n, days, err)
		}
	}
	// Model old derived data while leaving the authoritative ledger intact.
	target := start.AddDate(0, 0, 1).Format(time.DateOnly)
	if _, err := db.SQL.ExecContext(ctx, `UPDATE daily_valuation_snapshots SET content_hash = 'v2:synthetic-old', resolver_policy_version = 'synthetic-old' WHERE local_date IN (?, ?)`,
		target, start.AddDate(0, 0, 2).Format(time.DateOnly)); err != nil {
		t.Fatal(err)
	}
	if dirty {
		if _, err := db.SQL.ExecContext(ctx, `UPDATE history_snapshot_state SET dirty_from=?, dirty_to=?`, target, target); err != nil {
			t.Fatal(err)
		}
	}
	return stage6SeedBytes(t, db), bs.Household.ID
}

func seedStage6LockedSettlement(t *testing.T) []byte {
	t.Helper()
	clock := func() time.Time { return accountTestClock }
	db, _, svc := openStage6Database(t, filepath.Join(t.TempDir(), "settlement.db"), clock)
	ids, err := seedExistingBuy(t.Context(), svc, clock())
	if err != nil {
		t.Fatal(err)
	}
	seedTestLockedProductWithUnlock(t, svc, ids.account, nil, "2026-11-22")
	svc.SetClock(func() time.Time { return time.Date(2026, time.November, 23, 17, 0, 0, 0, time.UTC) })
	startTestHoldingReconciliation(t, reconciliation.NewService(svc), ids.account)
	return stage6SeedBytes(t, db)
}

func seedStage6LinkedCashTransfer(t *testing.T) []byte {
	t.Helper()
	clock := func() time.Time { return accountTestClock }
	db, _, svc := openStage6Database(t, filepath.Join(t.TempDir(), "transfer.db"), clock)
	ids, err := seedExistingBuy(t.Context(), svc, clock())
	if err != nil {
		t.Fatal(err)
	}
	seedLinkedTransferPeers(t, svc)
	svc.SetClock(linkedContractClock)
	startTestHoldingReconciliation(t, reconciliation.NewService(svc), ids.account)
	return stage6SeedBytes(t, db)
}

func seedStage6TermDeposit(t *testing.T) []byte {
	t.Helper()
	clock := func() time.Time { return accountTestClock }
	db, _, svc := openStage6Database(t, filepath.Join(t.TempDir(), "deposit.db"), clock)
	ids, err := seedExistingBuy(t.Context(), svc, clock())
	if err != nil {
		t.Fatal(err)
	}
	seedLinkedTermDeposit(t, svc, ids.account)
	startTestHoldingReconciliation(t, reconciliation.NewService(svc), ids.account)
	return stage6SeedBytes(t, db)
}

func TestExportStage6DesktopFixtures(t *testing.T) {
	output := os.Getenv("NESTWORTH_STAGE6_QA_DIR")
	if output == "" {
		t.Skip("set NESTWORTH_STAGE6_QA_DIR to an existing disposable directory to export desktop fixtures")
	}
	root, err := os.MkdirTemp(output, "nestworth-stage6-")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	start := today.AddDate(0, 0, -4).Add(9 * time.Hour)
	// Keep the one-day dirty window outside the default Overview trend,
	// whose stock startup can materialize recent derived revisions.
	snapshotStart := today.AddDate(0, 0, -45).Add(9 * time.Hour)
	snapshotSeed, _ := seedStage6Snapshots(t, snapshotStart, 45, true)
	calendarStart := time.Date(today.Year(), today.Month(), 1, 8, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	calendarSeed, _, _ := seedStage6ReturnCalendar(t, calendarStart)
	contributionSeed, _, _, dividend := seedStage6Contribution(t, calendarStart)
	marketSeed, _ := seedStage6Market(t, start)
	marketPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "market-readback"), marketSeed)
	now := time.Now().UTC().Truncate(time.Millisecond)
	fx := &stage6LatestFXProvider{refreshTestProvider: refreshTestProvider{now: now}}
	prices := &stage6LatestInstrumentProvider{now: now}
	db, _, svc := openStage6Database(t, marketPath, func() time.Time { return now },
		application.NewMarketDataRegistryWithDefault(fx.Key(), fx, prices))
	result, err := svc.RefreshMissingOrStale(t.Context())
	status := make(map[application.RefreshStatus]int)
	for _, item := range result.Items {
		status[item.Status]++
	}
	if err != nil || status[application.RefreshFetched] != 2 || status[application.RefreshFailed] != 1 {
		t.Fatalf("seed market partial readback = %+v: %v", result, err)
	}
	marketReadback := stage6SeedBytes(t, db)
	lockedProductSeed := seedStage6LockedSettlement(t)
	termDepositSeed := seedStage6TermDeposit(t)
	linkedCashTransferSeed := seedStage6LinkedCashTransfer(t)
	for _, profile := range []struct {
		name string
		seed []byte
	}{
		{"snapshots", snapshotSeed},
		{"market", marketSeed},
		{"market-readback", marketReadback},
		{"return-calendar", calendarSeed},
		{"contribution", contributionSeed},
		{"locked-settlement", lockedProductSeed},
		{"locked-terms", lockedProductSeed},
		{"locked-renewal", lockedProductSeed},
		{"add-product", lockedProductSeed},
		{"funds-product", lockedProductSeed},
		{"account-product", lockedProductSeed},
		{"linked-cash-transfer", linkedCashTransferSeed},
		{"check-account", lockedProductSeed},
		{"deposit-corrections", termDepositSeed},
	} {
		dir := filepath.Join(root, profile.name)
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, label := range []string{"source", "gio", "wails"} {
			writeStage6Fixture(t, filepath.Join(dir, label), profile.seed)
		}
	}
	t.Logf("fixtures=%s snapshot_repair_date=%s adjacent_stale_date=%s return_calendar_day=%s contribution_activity=%s", root,
		snapshotStart.AddDate(0, 0, 1).Format(time.DateOnly), snapshotStart.AddDate(0, 0, 2).Format(time.DateOnly),
		calendarStart.AddDate(0, 0, 1).Format(time.DateOnly), dividend.ID)
}
