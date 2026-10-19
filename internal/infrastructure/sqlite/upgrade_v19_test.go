package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func syntheticV19(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"schema.sql", "review_schema_v19.sql"} {
		raw, e := schemaFS.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	if _, err = db.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO investment_review_control(household_id,enabled,projection_status,published_input_fingerprint) SELECT id,0,'complete','synthetic-old-fingerprint' FROM households`); err != nil {
		t.Fatal(err)
	}
	return db
}
func TestV19UpgradeInitializesWithoutHistoryBackfillAndKeepsRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-v19.db")
	old := syntheticV19(t, path)
	if _, err := old.Exec(`INSERT INTO app_configuration VALUES('synthetic-private-setting','preserve exactly')`); err != nil {
		t.Fatal(err)
	}
	old.Close()
	read, err := OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err = read.SQL.QueryRow(`PRAGMA user_version`).Scan(&n); err != nil || n != 19 {
		t.Fatalf("readonly migrated: %d %v", n, err)
	}
	read.Close()
	current, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	if err = current.SQL.QueryRow(`SELECT count(*) FROM financial_source_changes`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("fabricated history: %d %v", n, err)
	}
	var revision, floor int
	var epoch, version, status, setting string
	var enabled int
	if err = current.SQL.QueryRow(`SELECT revision,journal_floor,tracking_epoch,tracking_version FROM financial_source_revision`).Scan(&revision, &floor, &epoch, &version); err != nil || revision != 0 || floor != 0 || len(epoch) != 32 || version != financialTrackingVersion {
		t.Fatalf("initial tracking: %d/%d %s %s %v", revision, floor, epoch, version, err)
	}
	if err = current.SQL.QueryRow(`SELECT enabled,projection_status FROM investment_review_control`).Scan(&enabled, &status); err != nil || enabled != 0 || status != "pending" {
		t.Fatalf("control: %d %s %v", enabled, status, err)
	}
	if err = current.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key='synthetic-private-setting'`).Scan(&setting); err != nil || setting != "preserve exactly" {
		t.Fatal("configuration changed")
	}
	copies, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".nestworth-v19-recovery-*", "original-v19.sqlite"))
	if err != nil || len(copies) != 1 {
		t.Fatalf("recovery copies: %v %v", copies, err)
	}
	info, _ := os.Stat(copies[0])
	if info.Mode().Perm() != 0400 {
		t.Fatalf("writable recovery %v", info.Mode())
	}
	recovery, err := OpenReadOnlyForVerify(copies[0])
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	if err = recovery.SQL.QueryRow(`PRAGMA user_version`).Scan(&n); err != nil || n != 19 {
		t.Fatal("recovery version lost")
	}
	if _, err = Open(copies[0]); err == nil {
		t.Fatal("recovery opened writable")
	}
	if err = current.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	current.Close()
	current, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	var reopened string
	if err = current.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&reopened); err != nil || reopened != epoch {
		t.Fatal("normal restart rotated epoch")
	}
}
func TestV19UpgradeRejectsConcurrentWriterAfterRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-v19.db")
	db := syntheticV19(t, path)
	defer db.Close()
	generation, err := prepareV19Recovery(t.Context(), db, path, (*os.File).Sync, syncV19UpgradeDir)
	if err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err = other.Exec(`UPDATE households SET name='Concurrent synthetic change'`); err != nil {
		t.Fatal(err)
	}
	if err = commitV19Upgrade(t.Context(), db, generation); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("concurrent upgrade committed: %v", err)
	}
	var version int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 19 {
		t.Fatal("failed upgrade modified version")
	}
	var tableCount int
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='financial_source_revision'`).Scan(&tableCount); err != nil || tableCount != 0 {
		t.Fatal("failed upgrade leaked metadata")
	}
}
func TestFinancialSourceSnapshotReadonlyResetAndIntegrity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.SQL.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	var epoch string
	if err = db.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(t.TempDir(), "complete.db")
	if err = db.SnapshotTo(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnlyForVerify(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var copyEpoch string
	if err = ro.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&copyEpoch); err != nil || copyEpoch != epoch {
		t.Fatal("backup did not retain metadata")
	}
	if err = ro.ResetFinancialSourceTracking(t.Context()); err == nil {
		t.Fatal("readonly reset succeeded")
	}
	ro.Close()
	live, err := Open(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	before := sourceRevision(t, live.SQL)
	if err = live.ResetFinancialSourceTracking(t.Context()); err != nil {
		t.Fatal(err)
	}
	var fresh string
	var floor int64
	if err = live.SQL.QueryRow(`SELECT tracking_epoch,journal_floor FROM financial_source_revision`).Scan(&fresh, &floor); err != nil || fresh == epoch || floor != before {
		t.Fatalf("reset failed: %s %d %v", fresh, floor, err)
	}
	if err = live.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = live.SQL.Exec(`UPDATE households SET name='Synthetic after restore'`); err != nil {
		t.Fatal(err)
	}
	if sourceRevision(t, live.SQL) != before+1 {
		t.Fatal("post-reset write untracked")
	}
	if _, err = live.SQL.Exec(`DELETE FROM financial_source_changes`); err != nil {
		t.Fatal(err)
	}
	if err = live.Verify(t.Context()); err == nil {
		t.Fatal("missing journal entry accepted")
	}
}

func TestFinancialSourcePruneAndFinanceOnlyImportEpoch(t *testing.T) {
	const household = "00000000-0000-4000-8000-000000000001"
	original, err := Open(filepath.Join(t.TempDir(), "synthetic-source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	financeSQL := `INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`
	if _, err = original.SQL.Exec(financeSQL); err != nil {
		t.Fatal(err)
	}
	var oldEpoch string
	if err = original.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&oldEpoch); err != nil {
		t.Fatal(err)
	}
	if _, err = original.SQL.Exec(`UPDATE households SET name='Synthetic updated'`); err != nil {
		t.Fatal(err)
	}
	if err = original.PruneFinancialSourceChanges(t.Context(), household, 1); err != nil {
		t.Fatal(err)
	}
	if err = original.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = original.PruneFinancialSourceChanges(t.Context(), household, 3); err == nil {
		t.Fatal("pruned beyond current revision")
	}
	if err = original.PruneFinancialSourceChanges(t.Context(), household, 0); err == nil {
		t.Fatal("floor moved backwards")
	}
	// JSON has no importer in the current product. Its finance-only import
	// contract is tested at the storage boundary: importing into a fresh DB
	// never copies the source journal/epoch, even when financial IDs are equal.
	imported, err := Open(filepath.Join(t.TempDir(), "synthetic-import.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer imported.Close()
	if _, err = imported.SQL.Exec(financeSQL); err != nil {
		t.Fatal(err)
	}
	var newEpoch string
	if err = imported.SQL.QueryRow(`SELECT tracking_epoch FROM financial_source_revision`).Scan(&newEpoch); err != nil || newEpoch == oldEpoch {
		t.Fatal("import reused source epoch")
	}
	if err = imported.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestV19UpgradeRecoveryIOFailureLeavesSourceUntouched(t *testing.T) {
	for _, kind := range []string{"file-sync", "directory-sync"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "synthetic-v19.db")
			db := syntheticV19(t, path)
			defer db.Close()
			syncFile := func(file *os.File) error { return file.Sync() }
			syncDir := syncV19UpgradeDir
			if kind == "file-sync" {
				syncFile = func(*os.File) error { return fmt.Errorf("synthetic file sync failure") }
			} else {
				syncDir = func(string) error { return fmt.Errorf("synthetic directory sync failure") }
			}
			if _, err := prepareV19Recovery(t.Context(), db, path, syncFile, syncDir); err == nil {
				t.Fatal("failed flush accepted")
			}
			var version int
			var name string
			if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 19 {
				t.Fatalf("version modified %d %v", version, err)
			}
			if err := db.QueryRow(`SELECT name FROM households`).Scan(&name); err != nil || name != "Synthetic" {
				t.Fatal("financial source modified")
			}
			if err := verifySchemaVersion(t.Context(), db, 19); err != nil {
				t.Fatal(err)
			}
		})
	}
}
