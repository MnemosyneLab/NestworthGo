package pilot

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestCashReceiptRemainsUnconfirmedAfterSealedOlderRestore(t *testing.T) {
	ctx := t.Context()
	root, paths := testPaths(t)
	t.Setenv("NESTWORTH_GIO_RUN_DIR", root)
	if err := Prepare(ctx, paths); err != nil {
		t.Fatal(err)
	}
	session, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	token := bytes.Repeat([]byte{0x2a}, receiptSize)
	repo := sqlite.NewRepository(session.DB)
	if err := sqlite.NewConfigurationRepository(session.DB).SaveConfiguration(recoveryMarkerKey, onboardingMarker(paths, token)); err != nil {
		t.Fatal(err)
	}
	app := application.NewService(repo)
	now := historyStart().AddDate(0, 0, 21)
	app.SetClock(func() time.Time { return now })
	core, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := app.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(accounts) != 1 || accounts[0].LatestValue == nil {
		t.Fatalf("expected synthetic checking: %+v / %v", accounts, err)
	}
	bank := accounts[0]
	reserve, err := app.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic reserve", AccountType: "bank_account", TrackingMode: "balance", BalanceSheetRole: "asset",
		DefaultCurrency: "USD", InitialAmount: "0", OwnerIDs: []domain.MemberID{core.Members[0].ID}, IncludeInNetWorth: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	id, digest := domain.NewMutationID().String(), strings.Repeat("b", 64)
	if err := session.CashReceipts.Remember(ctx, id, digest); err != nil {
		t.Fatal(err)
	}
	app.SetBackup(backup.NewRuntime())
	app.SetLiveDatabasePath(paths.database)
	filename := filepath.Join(t.TempDir(), "before-transfer"+backup.FileExt)
	if _, err := app.CreateBackup(ctx, filename, nil); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 3 {
		archive.Close()
		t.Fatal("backup packaged private UI files")
	}
	for _, file := range archive.File {
		if file.Name != backup.MemberManifest && file.Name != backup.MemberDatabase && file.Name != backup.MemberSettings {
			archive.Close()
			t.Fatalf("unexpected backup member %q", file.Name)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	pkg, err := backup.ReadPackage(filename)
	if err != nil || bytes.Contains(pkg.Database, []byte(id)) || bytes.Contains(pkg.Settings, []byte(digest)) {
		t.Fatalf("backup contains private pre-commit identity or failed to read: %v", err)
	}
	money, err := domain.ParseMoney("25", "USD")
	if err != nil {
		t.Fatal(err)
	}
	posted, err := app.RecordChangeWithMutation(ctx, domain.CashTransferInput{
		HouseholdID: core.Household.ID, FromAccountID: bank.Account.ID, ToAccountID: reserve.Account.ID,
		Sent: money, Received: money, EffectiveAt: now,
	}, id, digest)
	if err != nil || posted.Activity.Kind != domain.ActivityCashTransfer {
		t.Fatalf("synthetic transfer did not commit: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.CashReceipts.Pending(ctx); err == nil {
		t.Fatal("old receipt store worked after releasing its instance lock")
	}

	installOlderReceiptBackup(t, paths, pkg, token, now)
	reopened, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored := application.NewService(sqlite.NewRepository(reopened.DB))
	pending, err := reopened.CashReceipts.Pending(ctx)
	if err != nil || len(pending) != 1 || pending[0].MutationID != id || pending[0].MatchesLedger {
		t.Fatalf("sidecar did not stay unconfirmed across same-household restore: %+v / %v", pending, err)
	}
	household, err := restored.Household(ctx)
	if err != nil || household.ID != core.Household.ID {
		t.Fatalf("restore changed the household: %v", err)
	}
	receipt, err := restored.InspectChangeReceipt(ctx, id, digest)
	if err != nil || receipt != nil {
		t.Fatalf("older database contained a later receipt: %+v / %v", receipt, err)
	}
	before, err := sqlite.NewRepository(reopened.DB).AccountRecord(ctx, core.Household.ID, bank.Account.ID)
	if err != nil || before.LatestValue.ID != bank.LatestValue.ID {
		t.Fatalf("older bank value not retained: %+v / %v", before.LatestValue, err)
	}
	var count int
	if err := reopened.DB.SQL.QueryRowContext(ctx, "SELECT COUNT(*) FROM activities").Scan(&count); err != nil || count != 6 {
		t.Fatalf("restore or receipt lookup changed history: %d / %v", count, err)
	}
}

func installOlderReceiptBackup(t *testing.T, paths Paths, pkg backup.Package, token []byte, now time.Time) {
	t.Helper()
	unlock, err := backup.AcquireInstanceLock(paths.database)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := backup.InstallRestore(t.Context(), paths.database, pkg, nil, backup.RestoreClasses{Chrome: true, Format: true}, now); err != nil {
		t.Fatal(err)
	}
	journal, err := backup.ReadJournal(paths.database)
	if err != nil {
		t.Fatal(err)
	}
	hashFile := func(path string) string {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return recoveryHash(raw)
	}
	raw, err := jsoncodec.Marshal(recoverySeal{
		Token: token, Journal: hashFile(backup.JournalPath(paths.database)),
		Safety: hashFile(filepath.Join(paths.data, journal.SafetyName)), Settings: hashFile(paths.settings),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := backup.WriteAtomicFile(filepath.Join(paths.data, recoveryReceipt), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCashReceiptStoreRejectsLinksAndOtherHousehold(t *testing.T) {
	ctx := t.Context()
	root, paths := testPaths(t)
	if err := Prepare(ctx, paths); err != nil {
		t.Fatal(err)
	}
	session, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	receipts := filepath.Join(root, cashReceiptDirectory)
	outside := t.TempDir()
	if err := os.Symlink(outside, receipts); err != nil {
		t.Fatal(err)
	}
	id, digest := domain.NewMutationID().String(), strings.Repeat("c", 64)
	if err := session.CashReceipts.Remember(ctx, id, digest); err == nil {
		t.Fatal("receipt storage traversed a symlink")
	}
	if _, err := session.CashReceipts.Pending(ctx); err == nil {
		t.Fatal("receipt reads traversed a symlink")
	}
	files, err := os.ReadDir(outside)
	if err != nil || len(files) != 0 {
		t.Fatalf("unexpected external writes: %+v / %v", files, err)
	}
	if err := os.Remove(receipts); err != nil {
		t.Fatal(err)
	}
	if err := session.CashReceipts.Remember(ctx, id, digest); err != nil {
		t.Fatal(err)
	}
	if err := session.CashReceipts.Remember(ctx, id, strings.Repeat("d", 64)); err == nil {
		t.Fatal("saving a reused mutation replaced its original digest")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	_, different := testPaths(t)
	if err := Prepare(ctx, different); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(different.database)
	if err != nil {
		t.Fatal(err)
	}
	// Copy in place to exercise the household check even when path and inode
	// are unchanged. Both endpoints here are empty-of-user-data test fixtures.
	if err := os.WriteFile(paths.database, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	pending, err := reopened.CashReceipts.Pending(ctx)
	if err != nil || len(pending) != 1 || pending[0].MatchesLedger || pending[0].SHA256 != "" {
		t.Fatalf("receipt crossed the household boundary: %+v / %v", pending, err)
	}
}
