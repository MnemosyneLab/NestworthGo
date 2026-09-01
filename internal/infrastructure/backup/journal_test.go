package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestReconcilePreparedKeepsLive(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "nestworth.db")
	if err := os.WriteFile(live, []byte("live"), 0o600); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(dir, StagingName(live))
	if err := os.WriteFile(staging, []byte("staging"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteJournal(live, Journal{Operation: "restore", State: StatePrepared, LiveName: "nestworth.db", StagingName: filepath.Base(staging), StartedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileOnStartup(live, nil); err != nil {
		t.Fatal(err)
	}
	if !fileExists(live) {
		t.Fatal("live database was removed")
	}
	if fileExists(staging) {
		t.Fatal("staging should be deleted")
	}
	if _, err := ReadJournal(live); !os.IsNotExist(err) {
		t.Fatalf("journal still present: %v", err)
	}
}

func TestReconcilePreparedMovesOrphanSidecarsBeforeRestoringSafety(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "nestworth.db")
	if err := os.WriteFile(live+"-wal", []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	safety := filepath.Join(dir, "nestworth.db.pre-restore-1")
	good, err := sqlite.Open(filepath.Join(dir, "good.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := good.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "good.db"), safety); err != nil {
		t.Fatal(err)
	}
	if err := WriteJournal(live, Journal{Operation: "restore", State: StatePrepared, LiveName: filepath.Base(live), SafetyName: filepath.Base(safety), StartedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileOnStartup(live, nil); err != nil {
		t.Fatal(err)
	}
	if !fileExists(live) || fileExists(safety) || fileExists(safety+"-wal") {
		t.Fatal("reconcile did not restore a complete live database group")
	}
}

func TestReconcileReplacementInstalledRollsBackInvalid(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "nestworth.db")
	if err := os.WriteFile(live, []byte("not-sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	safety := filepath.Join(dir, "nestworth.db.pre-restore-1")
	good, err := sqlite.Open(filepath.Join(dir, "good.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := good.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "good.db"), safety); err != nil {
		t.Fatal(err)
	}
	if err := WriteJournal(live, Journal{Operation: "restore", State: StateReplacementInstalled, LiveName: "nestworth.db", SafetyName: filepath.Base(safety), StartedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	store := settings.NewStore(filepath.Join(dir, "settings.json"))
	before := settings.Default()
	before.Language = settings.LanguageZhCN
	if err := store.Save(before); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileOnStartup(live, store); err != nil {
		t.Fatal(err)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Language != settings.LanguageZhCN {
		t.Fatalf("settings changed on rollback: %+v", after)
	}
	verified, err := sqlite.OpenReadOnlyForVerify(live)
	if err != nil {
		t.Fatal(err)
	}
	_ = verified.Close()
}

func TestInstallRestoreWithoutSession(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "nestworth.db")
	src, err := sqlite.Open(filepath.Join(dir, "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	pkg := Package{Manifest: NewManifest(time.Now().UTC(), data, []byte(`{}`), sqlite.EntityCounts{}), Database: data, Settings: []byte(`{}`)}
	if err := os.WriteFile(live, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InstallRestore(context.Background(), live, pkg, nil, RestoreClasses{}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	journal, err := ReadJournal(live)
	if err != nil {
		t.Fatal(err)
	}
	if journal.State != StateReplacementInstalled {
		t.Fatalf("state = %s", journal.State)
	}
	if !fileExists(live) {
		t.Fatal("live missing")
	}
}

func TestOriginalRenamedWithLiveTreatsAsInstalled(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "nestworth.db")
	good, err := sqlite.Open(live)
	if err != nil {
		t.Fatal(err)
	}
	if err := good.Close(); err != nil {
		t.Fatal(err)
	}
	safety := filepath.Join(dir, "nestworth.db.pre-restore-1")
	if err := os.WriteFile(safety, []byte("safety"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteJournal(live, Journal{Operation: "restore", State: StateOriginalRenamed, LiveName: "nestworth.db", SafetyName: filepath.Base(safety), StartedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileOnStartup(live, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadJournal(live); !os.IsNotExist(err) {
		t.Fatalf("journal still present: %v", err)
	}
}
