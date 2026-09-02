package appports

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestBackupAndCSVImportWithoutWails(t *testing.T) {
	service, ctx := newWiredService(t, "backup-csv-ports", []string{"Alice"})
	dest := filepath.Join(t.TempDir(), "household.nestworth-backup")
	created, err := service.CreateBackup(ctx, dest, []byte("{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if created.FileName == "" || created.VerificationResult != "ok" {
		t.Fatalf("backup result = %+v", created)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	}
	status, available, err := service.LastBackupStatus()
	if err != nil || !available || status.BackupFileName != created.FileName {
		t.Fatalf("status available=%v err=%v %+v", available, err, status)
	}

	csv := []byte("account_name,account_type,balance_sheet_role,tracking_mode,currency,current_value,value_date,ownership\nChecking,bank_account,asset,balance,CNY,10,2026-08-01,Alice:100%\n")
	selected, err := service.SelectCSV(application.CSVProfileAccounts, "", "accounts.csv", csv)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewCSV(ctx, application.CSVPreviewRequest{
		Token: selected.Token, Profile: application.CSVProfileAccounts, DateFormat: application.CSVDateISO, DecimalSep: ".", GroupingSep: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.CanCommit {
		t.Fatalf("preview errors: %+v", preview.Errors)
	}
	if err := service.ConfirmCSV(selected.Token); err != nil {
		t.Fatal(err)
	}
	stats, err := service.CommitCSV(ctx, selected.Token)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CreateAccounts != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	accounts, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(accounts) != 1 || accounts[0].Account.Name != "Checking" {
		t.Fatalf("accounts = %+v err=%v", accounts, err)
	}
}

func TestRestorePreviewExpiresAndDeletesStagedFile(t *testing.T) {
	dir := t.TempDir()
	backupPath, live := writeTestBackup(t, dir)
	clock := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	recovery := application.NewRecovery(live, backup.NewRuntime(), nil)
	recovery.SetClock(func() time.Time { return clock })
	t.Cleanup(recovery.Shutdown)
	preview, err := recovery.InspectBackup(context.Background(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	staged := stagedRestorePath(t, dir, preview.Token)
	clock = clock.Add(application.RestorePreviewTTL)
	_, err = recovery.ConfirmRestore(context.Background(), application.RestoreConfirmInput{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true})
	if err == nil {
		t.Fatal("expected expired token to fail")
	}
	if _, statErr := os.Stat(staged); !os.IsNotExist(statErr) {
		t.Fatalf("staged file still present: %v", statErr)
	}
}

func TestRestorePreviewReuseAndEviction(t *testing.T) {
	dir := t.TempDir()
	backupPath, live := writeTestBackup(t, dir)
	clock := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	recovery := application.NewRecovery(live, backup.NewRuntime(), nil)
	recovery.SetClock(func() time.Time { return clock })
	t.Cleanup(recovery.Shutdown)
	first, err := recovery.InspectBackup(context.Background(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	firstPath := stagedRestorePath(t, dir, first.Token)
	clock = clock.Add(time.Second)
	if _, err := recovery.InspectBackup(context.Background(), backupPath); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second)
	if _, err := recovery.InspectBackup(context.Background(), backupPath); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(firstPath); !os.IsNotExist(statErr) {
		t.Fatalf("evicted staged file still present: %v", statErr)
	}
	_, err = recovery.ConfirmRestore(context.Background(), application.RestoreConfirmInput{Token: first.Token, Confirmation: "RESTORE", Acknowledged: true})
	if err == nil {
		t.Fatal("expected evicted token to fail")
	}
}

func TestRestorePreviewTokenIsSingleUse(t *testing.T) {
	dir := t.TempDir()
	backupPath, live := writeTestBackup(t, dir)
	recovery := application.NewRecovery(live, backup.NewRuntime(), nil)
	t.Cleanup(recovery.Shutdown)
	preview, err := recovery.InspectBackup(context.Background(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recovery.ConfirmRestore(context.Background(), application.RestoreConfirmInput{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := recovery.ConfirmRestore(context.Background(), application.RestoreConfirmInput{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true}); err == nil {
		t.Fatal("expected reused token to fail")
	}
}

func TestRestoreShutdownDeletesStagedFiles(t *testing.T) {
	dir := t.TempDir()
	backupPath, live := writeTestBackup(t, dir)
	recovery := application.NewRecovery(live, backup.NewRuntime(), nil)
	preview, err := recovery.InspectBackup(context.Background(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	staged := stagedRestorePath(t, dir, preview.Token)
	recovery.Shutdown()
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("shutdown left staged file: %v", err)
	}
}

func TestInspectBackupStagesPackageWithPrivateMode(t *testing.T) {
	dir := t.TempDir()
	backupPath, live := writeTestBackup(t, dir)
	recovery := application.NewRecovery(live, backup.NewRuntime(), nil)
	t.Cleanup(recovery.Shutdown)
	preview, err := recovery.InspectBackup(context.Background(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Token == "" || preview.SchemaVersion != sqlite.CurrentSchemaVersion || preview.HasOpenSession {
		t.Fatalf("preview = %+v", preview)
	}
	info, err := os.Stat(stagedRestorePath(t, dir, preview.Token))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("staged perm = %o, want 0600", info.Mode().Perm())
	}
}

func newWiredService(t *testing.T, name string, members []string) (*application.Service, context.Context) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".db")
	database, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := application.NewService(sqlite.NewRepository(database))
	Wire(service)
	service.SetLiveDatabasePath(path)
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Test", BaseCurrency: "CNY", MemberNames: members}); err != nil {
		t.Fatal(err)
	}
	return service, ctx
}

func writeTestBackup(t *testing.T, dir string) (backupPath, live string) {
	t.Helper()
	dbPath := filepath.Join(dir, "src.db")
	database, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	pkg := backup.Package{
		Manifest: backup.NewManifest(time.Now().UTC(), data, []byte("{}\n"), sqlite.EntityCounts{}),
		Database: data,
		Settings: []byte("{}\n"),
	}
	backupPath = filepath.Join(dir, "ok.nestworth-backup")
	if err := backup.WritePackage(backupPath, pkg); err != nil {
		t.Fatal(err)
	}
	return backupPath, filepath.Join(dir, "nestworth.db")
}

func stagedRestorePath(t *testing.T, dir, token string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".nestworth-restore-"+token+"*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("staged files = %v err=%v", matches, err)
	}
	return matches[0]
}
