package recovery

import (
	"context"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/settings"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/native"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

type memoryDialogs struct {
	open string
}

func (m memoryDialogs) SaveFile(string, string, string, string) (string, error) { return "", nil }
func (m memoryDialogs) OpenFile(string, string, string) (string, error)         { return m.open, nil }
func (m memoryDialogs) ConfirmReplace(string) (bool, error)                     { return false, nil }

func newTestRecovery(t *testing.T, live string, dialogs native.Dialogs, quit native.Quitter) *Service {
	t.Helper()
	recovery := application.NewRecovery(live, backup.NewRuntime(), nil)
	t.Cleanup(recovery.Shutdown)
	return NewService(recovery, dialogs, quit, native.NoopRefresh{})
}

func TestInspectBackupCancel(t *testing.T) {
	service := NewService(nil, memoryDialogs{}, nil, nil)
	result, err := service.InspectBackup()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Cancelled {
		t.Fatal("empty path should cancel")
	}
}

func TestInspectBackupRejectsInvalidArchive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.nestworth-backup")
	if err := os.WriteFile(path, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := newTestRecovery(t, filepath.Join(dir, "nestworth.db"), memoryDialogs{open: path}, nil)
	_, err := service.InspectBackup()
	if err == nil {
		t.Fatal("expected invalid backup to fail")
	}
}

func TestConfirmRestoreRequiresTypedConfirmation(t *testing.T) {
	app := wailstest.NewService(t)
	recovery := application.NewRecovery(filepath.Join(t.TempDir(), "nestworth.db"), backup.NewRuntime(), app)
	t.Cleanup(recovery.Shutdown)
	service := NewService(recovery, native.NoopDialogs{}, nil, nil)
	_, err := service.ConfirmRestore(RestoreConfirmRequest{Token: "missing", Confirmation: "no", Acknowledged: true})
	if err == nil {
		t.Fatal("expected confirmation error")
	}
}

func TestInspectBackupPreviewFromValidPackage(t *testing.T) {
	dir := t.TempDir()
	backupPath, live := writeTestBackup(t, dir)
	service := newTestRecovery(t, live, memoryDialogs{open: backupPath}, nil)
	preview, err := service.InspectBackup()
	if err != nil {
		t.Fatal(err)
	}
	if preview.Cancelled || preview.Token == "" || preview.SchemaVersion != sqlite.CurrentSchemaVersion {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.HasOpenSession {
		t.Fatal("blocked-startup inspect should report no open session")
	}
}

func TestRestorePreviewTokenIsSingleUse(t *testing.T) {
	dir := t.TempDir()
	backupPath, live := writeTestBackup(t, dir)
	service := newTestRecovery(t, live, memoryDialogs{open: backupPath}, native.NoopQuitter{})
	preview, err := service.InspectBackup()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = service.ConfirmRestore(RestoreConfirmRequest{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true})
	_, err = service.ConfirmRestore(RestoreConfirmRequest{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true})
	if err == nil {
		t.Fatal("expected reused token to fail")
	}
	service.Shutdown()
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

func TestRestoreBackupOnNewComputer(t *testing.T) {
	for _, openSession := range []bool{false, true} {
		t.Run(fmt.Sprintf("open_session_%t", openSession), func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			source := filepath.Join(dir, "source.db")
			db, err := sqlite.Open(source)
			if err != nil {
				t.Fatal(err)
			}
			app := application.NewService(sqlite.NewRepository(db))
			appports.Wire(app)
			if err := app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Existing family", BaseCurrency: "SGD", MemberNames: []string{"Alice"}}); err != nil {
				t.Fatal(err)
			}
			saved := settings.NewStore(filepath.Join(dir, "source-settings.json"))
			if err := saved.Attach(sqlite.NewConfigurationRepository(db)); err != nil {
				t.Fatal(err)
			}
			value := settings.Default()
			value.Currency = "SGD"
			value.Timezone = "Asia/Singapore"
			value.TiingoAPIKey = "test-migrated-key"
			if err := saved.Save(value); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			prefs, _ := settings.PresentationJSON(value)
			archive := filepath.Join(dir, "old-computer.nestworth-backup")
			if err := backup.WritePackage(archive, backup.Package{Manifest: backup.NewManifest(time.Now(), data, prefs, sqlite.EntityCounts{}), Database: data, Settings: prefs}); err != nil {
				t.Fatal(err)
			}
			live := filepath.Join(dir, "new-computer", "nestworth.db")
			var fresh *application.Service
			if openSession {
				empty, err := sqlite.Open(live)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { empty.Close() })
				fresh = application.NewService(sqlite.NewRepository(empty))
				appports.Wire(fresh)
				fresh.SetLiveDatabasePath(live)
			}
			recovery := application.NewRecovery(live, backup.NewRuntime(), fresh)
			defer recovery.Shutdown()
			service := NewService(recovery, memoryDialogs{open: archive}, native.NoopQuitter{}, native.NoopRefresh{})
			preview, err := service.InspectBackup()
			if err != nil {
				t.Fatal(err)
			}
			if preview.BackupHousehold != "Existing family" || preview.CurrentHousehold != "" {
				t.Fatalf("unexpected preview: %+v", preview)
			}
			result, err := service.ConfirmRestore(RestoreConfirmRequest{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true})
			if err != nil || !result.RestartRequired {
				t.Fatal("restore failed", err)
			}
			local := settings.NewStore(filepath.Join(dir, "new-computer", "settings.json"))
			if err := backup.ReconcileOnStartup(live, local); err != nil {
				t.Fatal(err)
			}
			restored, err := sqlite.Open(live)
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			summary, err := restored.HouseholdSummary(ctx)
			if err != nil || summary.Name != "Existing family" || summary.BaseCurrency != "SGD" {
				t.Fatal("household was not restored", err)
			}
			if err := local.Attach(sqlite.NewConfigurationRepository(restored)); err != nil {
				t.Fatal(err)
			}
			got, err := local.Load()
			if err != nil || got.TiingoAPIKey != value.TiingoAPIKey || got.Timezone != value.Timezone {
				t.Fatal("important settings were not restored", err)
			}
		})
	}
}
