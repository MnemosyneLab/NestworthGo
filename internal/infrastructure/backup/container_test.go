package backup

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestWriteAndReadPackageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "Nestworth Backup 2026-09-01 12-00.nestworth-backup")
	database := []byte("sqlite-bytes")
	settingsJSON := []byte(`{"schema_version":1}`)
	pkg := Package{
		Manifest: NewManifest(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), database, settingsJSON, sqlite.EntityCounts{Accounts: 2}),
		Database: database,
		Settings: settingsJSON,
	}
	if err := WritePackage(dest, pkg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 0600", info.Mode().Perm())
	}
	got, err := ReadPackage(dest)
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest.FormatVersion != 1 || got.Manifest.SchemaVersion != sqlite.CurrentSchemaVersion {
		t.Fatalf("manifest %+v", got.Manifest)
	}
	if !bytes.Equal(got.Database, database) || !bytes.Equal(got.Settings, settingsJSON) {
		t.Fatal("payload mismatch")
	}
}

func TestReadPackageRejectsExtraMember(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.nestworth-backup")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, name := range []string{MemberManifest, MemberDatabase, MemberSettings, "extra.txt"} {
		w, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("{}")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	_, err = ReadPackage(path)
	if err == nil {
		t.Fatal("expected extra member to be rejected")
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrBackupInvalidFormat {
		t.Fatalf("err = %v", err)
	}
}

func TestWritePackageRejectsChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "broken.nestworth-backup")
	database := []byte("hello")
	settingsJSON := []byte(`{}`)
	pkg := Package{
		Manifest: NewManifest(time.Now().UTC(), database, settingsJSON, sqlite.EntityCounts{}),
		Database: database,
		Settings: settingsJSON,
	}
	pkg.Manifest.Members[MemberDatabase] = MemberMeta{SHA256: "00", SizeBytes: int64(len(database))}
	if err := WritePackage(dest, pkg); err == nil {
		t.Fatal("expected checksum failure before installation")
	}
	if fileExists(dest) {
		t.Fatal("invalid package should not be installed")
	}
}

func TestWritePackagePreservesExistingOnValidationFailure(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "backup.nestworth-backup")
	oldDatabase := []byte("old")
	oldSettings := []byte(`{}`)
	old := Package{Manifest: NewManifest(time.Now().UTC(), oldDatabase, oldSettings, sqlite.EntityCounts{}), Database: oldDatabase, Settings: oldSettings}
	if err := WritePackage(dest, old); err != nil {
		t.Fatal(err)
	}
	broken := Package{Manifest: NewManifest(time.Now().UTC(), []byte("new"), oldSettings, sqlite.EntityCounts{}), Database: []byte("new"), Settings: oldSettings}
	broken.Manifest.Members[MemberDatabase] = MemberMeta{SHA256: "00", SizeBytes: 3}
	if err := WritePackage(dest, broken); err == nil {
		t.Fatal("expected invalid replacement to be rejected")
	}
	got, err := ReadPackage(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Database, oldDatabase) {
		t.Fatalf("database = %q, want old package", got.Database)
	}
}

func TestMoveFileGroupMovesSidecars(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nestworth.db")
	if err := os.WriteFile(src, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src+"-wal", []byte("wal"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "nestworth.db.pre-restore-1")
	if err := MoveFileGroup(src, dest); err != nil {
		t.Fatal(err)
	}
	if fileExists(src) || fileExists(src+"-wal") {
		t.Fatal("source files still present")
	}
	if !fileExists(dest) || !fileExists(dest+"-wal") {
		t.Fatal("destination files missing")
	}
}

func TestMoveFileGroupMovesOrphanWAL(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nestworth.db")
	if err := os.WriteFile(src+"-wal", []byte("wal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src+"-shm", []byte("shm"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !GroupExists(src) {
		t.Fatal("orphan sidecars should count as a live group")
	}
	dest := filepath.Join(dir, "nestworth.db.pre-restore-orphan")
	if err := MoveFileGroup(src, dest); err != nil {
		t.Fatal(err)
	}
	if fileExists(src+"-wal") || fileExists(src+"-shm") {
		t.Fatal("source sidecars still present")
	}
	if fileExists(dest) {
		t.Fatal("destination main file should not be created from an orphan group")
	}
	if !fileExists(dest+"-wal") || !fileExists(dest+"-shm") {
		t.Fatal("destination sidecars missing")
	}
}

func TestMoveFileGroupRejectsExistingDestSidecar(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nestworth.db")
	if err := os.WriteFile(src, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "nestworth.db.pre-restore-2")
	if err := os.WriteFile(dest+"-shm", []byte("shm"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MoveFileGroup(src, dest); err == nil {
		t.Fatal("expected dest sidecar conflict")
	}
	if !fileExists(src) {
		t.Fatal("source main file should remain after a rejected move")
	}
}

func TestMoveFileGroupRollsBackWhenSidecarMoveFails(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nestworth.db")
	dest := filepath.Join(dir, "nestworth.db.pre-restore")
	if err := os.WriteFile(src, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src+"-wal", []byte("wal"), 0o600); err != nil {
		t.Fatal(err)
	}
	rename := func(from, to string) error {
		if from == src+"-wal" {
			return errors.New("injected sidecar failure")
		}
		return os.Rename(from, to)
	}
	if err := moveFileGroup(src, dest, rename); err == nil {
		t.Fatal("expected move failure")
	}
	if !fileExists(src) || !fileExists(src+"-wal") || fileExists(dest) || fileExists(dest+"-wal") {
		t.Fatal("failed move did not restore the complete source group")
	}
	var domainErr *domain.Error
	if err := moveFileGroup(src, dest, rename); !errors.As(err, &domainErr) || domainErr.Code != domain.ErrBackupRestoreSwapFailed {
		t.Fatalf("retry error = %v, want swap failure", err)
	}
}

func TestMoveFileGroupReportsRollbackFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nestworth.db")
	dest := filepath.Join(dir, "nestworth.db.pre-restore")
	if err := os.WriteFile(src, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src+"-wal", []byte("wal"), 0o600); err != nil {
		t.Fatal(err)
	}
	rename := func(from, to string) error {
		if from == src+"-wal" || from == dest {
			return errors.New("injected move or rollback failure")
		}
		return os.Rename(from, to)
	}
	err := moveFileGroup(src, dest, rename)
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrBackupRestoreRollbackFailed {
		t.Fatalf("error = %v, want rollback failure", err)
	}
}
