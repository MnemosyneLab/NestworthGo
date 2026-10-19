package continuousbackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	wailsrecovery "github.com/waltwang/nestworth-go/internal/wailsapi/recovery"
)

func restoreSource(t *testing.T, db *sqlite.DB) string {
	t.Helper()
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "snapshot.db")
	if err := db.SnapshotTo(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	pkg := backup.Package{Manifest: backup.NewManifest(time.Now(), data, []byte("{}\n"), sqlite.EntityCounts{}), Database: data, Settings: []byte("{}\n")}
	source := filepath.Join(dir, "source.nestworth-backup")
	if err := backup.WritePackage(source, pkg); err != nil {
		t.Fatal(err)
	}
	return source
}

// A quiesce-hook failure must remove the staged candidate before checkpoint
// or swap. The package is a local snapshot, so this does not build a replica.
func TestQuiesceHookFailureRemovesStagingBeforeCheckpoint(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	live := filepath.Join(dir, "live.db")
	liveDB, err := sqlite.Open(live)
	if err != nil {
		t.Fatal(err)
	}
	defer liveDB.Close()
	if _, err = liveDB.SQL.Exec(`INSERT INTO app_configuration(key,value) VALUES('quiesce-original','intact')`); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(dir, "snapshot.db")
	if err = liveDB.SnapshotTo(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "candidate.nestworth-backup")
	pkg := backup.Package{Manifest: backup.NewManifest(time.Now().UTC(), data, []byte("{}\n"), sqlite.EntityCounts{}), Database: data, Settings: []byte("{}\n")}
	if err = backup.WritePackage(archive, pkg); err != nil {
		t.Fatal(err)
	}
	recovery := application.NewRecovery(live, backup.NewRuntime(), nil)
	recovery.SetBeforeRestore(func(context.Context) error {
		return errors.New("synthetic quiesce failure")
	})
	defer recovery.Shutdown()
	preview, err := recovery.InspectBackup(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	service := wailsrecovery.NewService(recovery, nil, nil, nil)
	if _, err = service.ConfirmRestore(wailsrecovery.RestoreConfirmRequest{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true}); err == nil {
		t.Fatal("quiesce failure was accepted")
	}
	if _, statErr := os.Stat(filepath.Join(dir, backup.StagingName(live))); !os.IsNotExist(statErr) {
		t.Fatal("staging remained after quiesce failure", statErr)
	}
	var value string
	if err = liveDB.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key='quiesce-original'`).Scan(&value); err != nil || value != "intact" {
		t.Fatalf("live session changed: %q %v", value, err)
	}
}

func TestJournalFailureAfterBackupPauseRequiresRestart(t *testing.T) {
	m, db, _ := fixture(t)
	ctx := context.Background()
	if err := m.Configure(ctx, testUpdate(true)); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, "recent-backup-confirmed")
	app := application.NewService(sqlite.NewRepository(db))
	r := application.NewRecovery(m.path, backup.NewRuntime(), app)
	r.SetBeforeRestore(m.PauseForRestore)
	defer r.Shutdown()
	preview, err := r.InspectBackup(ctx, restoreSource(t, db))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(filepath.Dir(m.path), backup.JournalFileName), 0700); err != nil {
		t.Fatal(err)
	}
	result, err := r.ConfirmRestore(ctx, application.RestoreConfirmInput{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true})
	if err == nil {
		t.Fatal("journal directory did not fail install")
	}
	v, viewErr := m.View()
	if viewErr != nil {
		t.Fatal(viewErr)
	}
	writeErr := app.WithWrite(ctx, func(context.Context) error { return nil })
	enableErr := m.Configure(ctx, testUpdate(true))
	t.Logf("result=%+v enabled=%t restoreState=%s applicationWrite=%v reenable=%v", result, v.Enabled, v.RestoreState, writeErr, enableErr)
	if !result.RestartRequired || !result.KeepExclusive || writeErr == nil {
		t.Error("quiesced restore failure must require restart and fence writes")
	}
	if v.Enabled || enableErr == nil {
		t.Error("backup must remain durably disabled and fenced until restart")
	}
	if _, err := db.SQL.Exec(`INSERT INTO app_configuration(key,value) VALUES('original-session','intact')`); err != nil {
		t.Fatal("original database session was changed", err)
	}
}

type restoreFailureRuntime struct {
	backup.Runtime
	phase string
}

func (r restoreFailureRuntime) InstallRestore(ctx context.Context, path string, pkg application.BackupPackage, hooks application.RestoreHooks, classes application.RestoreClasses, now time.Time) error {
	failure := errors.New("synthetic restore failure")
	switch r.phase {
	case "before-quiesce":
		return failure
	case "checkpoint":
		hooks.Checkpoint = func(context.Context) error { return failure }
	case "postclose":
		closeDB := hooks.Close
		hooks.Close = func() error {
			if err := closeDB(); err != nil {
				return err
			}
			return failure
		}
	}
	return r.Runtime.InstallRestore(ctx, path, pkg, hooks, classes, now)
}

func TestRestoreFailuresFenceQuiescedSessionAndKeepBackupDisabled(t *testing.T) {
	for _, phase := range []string{"before-quiesce", "checkpoint", "postclose"} {
		t.Run(phase, func(t *testing.T) {
			m, db, _ := fixture(t)
			ctx := context.Background()
			if _, err := db.SQL.Exec(`INSERT INTO app_configuration(key,value) VALUES('restore-original','backed-up')`); err != nil {
				t.Fatal(err)
			}
			source := restoreSource(t, db)
			if _, err := db.SQL.Exec(`UPDATE app_configuration SET value='original-local' WHERE key='restore-original'`); err != nil {
				t.Fatal(err)
			}
			if err := m.Configure(ctx, testUpdate(true)); err != nil {
				t.Fatal(err)
			}
			waitState(t, m, "recent-backup-confirmed")
			app := application.NewService(sqlite.NewRepository(db))
			r := application.NewRecovery(m.path, restoreFailureRuntime{Runtime: backup.NewRuntime(), phase: phase}, app)
			r.SetBeforeRestore(func(ctx context.Context) error {
				return m.PauseForRestore(ctx)
			})
			defer r.Shutdown()
			preview, err := r.InspectBackup(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.ConfirmRestore(ctx, application.RestoreConfirmInput{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true})
			if err == nil {
				t.Fatalf("phase %s result=%+v error=%v", phase, result, err)
			}
			fenced := phase != "before-quiesce"
			if result.RestartRequired != fenced || result.KeepExclusive != fenced {
				t.Fatalf("phase %s fence=%+v", phase, result)
			}
			writeErr := app.WithWrite(ctx, func(context.Context) error { return nil })
			if (writeErr != nil) != fenced {
				t.Fatalf("phase %s write error=%v", phase, writeErr)
			}
			v, viewErr := m.View()
			if viewErr != nil || v.Enabled == fenced {
				t.Fatalf("phase %s backup=%+v error=%v", phase, v, viewErr)
			}
			if phase == "postclose" {
				if db.SQL != nil {
					t.Fatal("database was not closed")
				}
			} else {
				var value string
				if err := db.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key='restore-original'`).Scan(&value); err != nil || value != "original-local" {
					t.Fatalf("original changed: %q %v", value, err)
				}
			}
			if fenced && !errors.Is(m.Configure(ctx, testUpdate(true)), ErrDisabled) {
				t.Fatal("quiesced manager allowed enable")
			}
			if err := m.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := backup.ReconcileOnStartup(m.path, nil); err != nil {
				t.Fatal(err)
			}
			reopened, err := sqlite.Open(m.path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			var value string
			want := "original-local"
			if err := reopened.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key='restore-original'`).Scan(&value); err != nil || value != want {
				t.Fatalf("restart database=%q want %q: %v", value, want, err)
			}
			if phase == "before-quiesce" {
				return
			}
			restarted, err := New(m.path, reopened)
			if err != nil {
				t.Fatal(err)
			}
			restarted.factory = m.factory
			restarted.interval = time.Hour
			defer restarted.Close(ctx)
			if err := restarted.Start(ctx); err != nil {
				t.Fatal(err)
			}
			v, _ = restarted.View()
			if v.Enabled || v.State != "disabled" || v.RestoreState != "" {
				t.Fatalf("restart silently enabled backup: %+v", v)
			}
			if err := restarted.Configure(ctx, testUpdate(true)); err != nil {
				t.Fatal("deliberate enable failed after restart", err)
			}
			waitState(t, restarted, "recent-backup-confirmed")
		})
	}
}
