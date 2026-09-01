package recovery

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

type recordingQuitter struct{ quit bool }

func (q *recordingQuitter) Quit() { q.quit = true }

func TestInspectBackupCancel(t *testing.T) {
	service := NewService(filepath.Join(t.TempDir(), "nestworth.db"), nil, nil, memoryDialogs{}, nil, nil)
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
	service := NewService(filepath.Join(dir, "nestworth.db"), nil, nil, memoryDialogs{open: path}, nil, nil)
	_, err := service.InspectBackup()
	if err == nil {
		t.Fatal("expected invalid backup to fail")
	}
}

func TestConfirmRestoreRequiresTypedConfirmation(t *testing.T) {
	app := wailstest.NewService(t)
	service := NewService(filepath.Join(t.TempDir(), "nestworth.db"), app, nil, native.NoopDialogs{}, nil, nil)
	_, err := service.ConfirmRestore(RestoreConfirmRequest{Token: "missing", Confirmation: "no", Acknowledged: true})
	if err == nil {
		t.Fatal("expected confirmation error")
	}
}

func TestInspectBackupPreviewFromValidPackage(t *testing.T) {
	dir := t.TempDir()
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
	backupPath := filepath.Join(dir, "ok.nestworth-backup")
	if err := backup.WritePackage(backupPath, pkg); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(dir, "nestworth.db")
	service := NewService(live, nil, nil, memoryDialogs{open: backupPath}, nil, nil)
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
