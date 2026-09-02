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
	service.mu.Lock()
	pending := service.pending[preview.Token]
	service.mu.Unlock()
	if pending.stagedPath == "" {
		t.Fatal("expected a staged restore package")
	}
	info, err := os.Stat(pending.stagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("staged perm = %o, want 0600", info.Mode().Perm())
	}
}

func TestRestorePreviewExpiresAndDeletesStagedFile(t *testing.T) {
	dir := t.TempDir()
	backupPath, live := writeTestBackup(t, dir)
	clock := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	service := NewService(live, nil, nil, memoryDialogs{open: backupPath}, nil, nil)
	service.now = func() time.Time { return clock }
	preview, err := service.InspectBackup()
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	staged := service.pending[preview.Token].stagedPath
	service.mu.Unlock()
	clock = clock.Add(pendingRestoreTTL)
	_, err = service.ConfirmRestore(RestoreConfirmRequest{Token: preview.Token, Confirmation: "RESTORE", Acknowledged: true})
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
	service := NewService(live, nil, nil, memoryDialogs{open: backupPath}, nil, nil)
	service.now = func() time.Time { return clock }
	first, err := service.InspectBackup()
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	firstPath := service.pending[first.Token].stagedPath
	service.mu.Unlock()
	clock = clock.Add(time.Second)
	second, err := service.InspectBackup()
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second)
	if _, err := service.InspectBackup(); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	_, firstKept := service.pending[first.Token]
	sessionCount := len(service.pending)
	service.mu.Unlock()
	if firstKept || sessionCount != maxPendingRestoreSessions {
		t.Fatalf("eviction failed: kept first=%v count=%d", firstKept, sessionCount)
	}
	if _, statErr := os.Stat(firstPath); !os.IsNotExist(statErr) {
		t.Fatalf("evicted staged file still present: %v", statErr)
	}
	_, err = service.ConfirmRestore(RestoreConfirmRequest{Token: first.Token, Confirmation: "RESTORE", Acknowledged: true})
	if err == nil {
		t.Fatal("expected evicted token to fail")
	}
	_ = second
}

func TestRestorePreviewTokenIsSingleUse(t *testing.T) {
	dir := t.TempDir()
	backupPath, live := writeTestBackup(t, dir)
	service := NewService(live, nil, nil, memoryDialogs{open: backupPath}, native.NoopQuitter{}, nil)
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

func TestRestoreShutdownDeletesStagedFiles(t *testing.T) {
	dir := t.TempDir()
	backupPath, live := writeTestBackup(t, dir)
	service := NewService(live, nil, nil, memoryDialogs{open: backupPath}, nil, nil)
	preview, err := service.InspectBackup()
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	staged := service.pending[preview.Token].stagedPath
	service.mu.Unlock()
	service.Shutdown()
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("shutdown left staged file: %v", err)
	}
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
