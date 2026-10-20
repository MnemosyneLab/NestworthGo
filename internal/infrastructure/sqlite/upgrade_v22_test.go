package sqlite

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func syntheticV22(t *testing.T, path string) *sql.DB {
	t.Helper()
	old := syntheticV20(t, path)
	tx, err := old.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := applyV20Upgrade(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err := applyV21Upgrade(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return old
}

func TestV22SharedFactUpgradeRecoveryAndSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.db")
	syntheticV22(t, path).Close()
	ro, err := OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	ro.Close()
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version, count int
	if err := db.SQL.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != CurrentSchemaVersion {
		t.Fatal(version, err)
	}
	if err := db.SQL.QueryRow(`SELECT count(*) FROM review_financial_daily_facts`).Scan(&count); err != nil || count != 0 {
		t.Fatal("startup backfill", count, err)
	}
	copies, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".nestworth-v22-recovery-*", "original-v22.sqlite"))
	if err != nil || len(copies) != 1 {
		t.Fatal(copies, err)
	}
	info, err := os.Stat(copies[0])
	if err != nil || info.Mode().Perm() != 0400 {
		t.Fatal("recovery permissions", err)
	}
	recovery, err := OpenReadOnlyForVerify(copies[0])
	if err != nil {
		t.Fatal(err)
	}
	recovery.Close()
	if writable, err := Open(copies[0]); err == nil {
		writable.Close()
		t.Fatal("recovery opened writable")
	}
	snapshot := filepath.Join(t.TempDir(), "restored.db")
	if err := db.SnapshotTo(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restored.Close()
	if _, err := db.SQL.Exec(`DROP INDEX idx_review_financial_fact_lookup`); err != nil {
		t.Fatal(err)
	}
	if err := verifySchema(t.Context(), db.SQL); err == nil {
		t.Fatal("missing shared fact index accepted")
	}
}

func TestV22SharedFactRecoveryFlushFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.db")
	old := syntheticV22(t, path)
	defer old.Close()
	failure := errors.New("synthetic flush failure")
	if _, err := prepareV22Recovery(t.Context(), old, path, func(*os.File) error { return failure }, syncV22UpgradeDir); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	var version int
	if err := old.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 22 {
		t.Fatal("failed migration mutated old DB", version, err)
	}
}
