package pilot

import (
	"bytes"
	"errors"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/testledger"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestProfileReconcilesAnInterruptedWailsRestore(t *testing.T) {
	ctx := t.Context()
	_, paths := pathsForDefaultProfile(t)
	if err := seedTestLedger(ctx, paths); err != nil {
		t.Fatal(err)
	}
	session, err := OpenSession(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	app := testfixture.NewService(testfixture.Repository(session.DB))
	now := testledger.HistoryStart().AddDate(0, 0, 20)
	testfixture.SetClock(app, func() time.Time { return now })
	app.SetBackup(backup.NewRuntime())
	app.SetLiveDatabasePath(paths.database)
	core, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := testfixture.CreateAccount(app, ctx, application.AccountInput{
		Name: "Recovery Sample Reserve", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "42.25",
		IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{core.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	prefs := settings.Default()
	prefs.Appearance, prefs.Accent, prefs.DateFormat = settings.AppearanceDark, settings.AccentMint, settings.DateFormatDayFirst
	presentation, err := settings.PresentationJSON(prefs)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "recovery"+backup.FileExt)
	if _, err := app.CreateBackup(ctx, archive, presentation); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	if _, err := testfixture.AppendAccountValue(app, ctx, reserve.Account.ID, "99.50", ""); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	pkg, err := backup.ReadPackage(archive)
	if err != nil {
		t.Fatal(err)
	}
	installOlderReceiptBackup(t, paths, pkg, now)
	journal, err := backup.ReadJournal(paths.database)
	if err != nil || journal.State != backup.StateReplacementInstalled {
		t.Fatalf("test did not stage an installed Wails restore: %v, %v", journal.State, err)
	}
	for range 2 {
		reopened, err := OpenSession(ctx, paths)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = reopened.Close() })
		record, err := testfixture.Repository(reopened.DB).AccountRecord(ctx, core.Household.ID, reserve.Account.ID)
		if err != nil || record.LatestValue == nil || record.LatestValue.Amount.CanonicalAmount() != "42.25" {
			t.Fatalf("expected value from the installed backup: %+v, %v", record.LatestValue, err)
		}
		store := settings.NewStore(paths.SettingsPath())
		if err := store.Attach(sqlite.NewConfigurationRepository(reopened.DB)); err != nil {
			t.Fatal(err)
		}
		got, err := store.Load()
		if err != nil || got.Appearance != prefs.Appearance || got.Accent != prefs.Accent || got.DateFormat != prefs.DateFormat {
			t.Fatalf("selected Wails presentation settings did not recover: %+v, %v", got, err)
		}
		if _, err := os.Stat(backup.JournalPath(paths.database)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("startup did not finalize the restore journal: %v", err)
		}
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProfileRejectsInvalidRestoreJournalAndReleasesLock(t *testing.T) {
	_, paths := testPaths(t)
	ctx := t.Context()
	if err := seedTestLedger(ctx, paths); err != nil {
		t.Fatal(err)
	}
	journal := backup.JournalPath(paths.database)
	before, err := os.ReadFile(paths.database)
	if err != nil {
		t.Fatal(err)
	}
	invalid := []byte("a malformed restore journal")
	if err := os.WriteFile(journal, invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := OpenSession(ctx, paths); !errors.Is(err, ErrStartupRecovery) {
		if s != nil {
			_ = s.Close()
		}
		t.Fatalf("startup should block on a malformed journal: %v", err)
	}
	if got, err := os.ReadFile(paths.database); err != nil || !bytes.Equal(before, got) {
		t.Fatalf("failed recovery changed the database: %v", err)
	}
	if got, err := os.ReadFile(journal); err != nil || !bytes.Equal(invalid, got) {
		t.Fatalf("failed recovery changed the invalid journal: %v", err)
	}
	unlock, err := backup.AcquireInstanceLock(paths.database)
	if err != nil {
		t.Fatalf("failed recovery kept the instance lock: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}
