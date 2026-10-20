package sqlite

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestV23SharedInputFormatUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.db")
	old := syntheticV22(t, path)
	tx, err := old.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyV22Upgrade(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("synthetic v23 recovery flush")
	if _, err := prepareV23Recovery(t.Context(), old, path, func(*os.File) error { return failure }, syncV23UpgradeDir); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	var version int
	if err := old.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 23 {
		t.Fatal("failed recovery changed format", version, err)
	}
	old.Close()
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SQL.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 24 {
		t.Fatal(version, err)
	}
	var facts int
	if err := db.SQL.QueryRow(`SELECT count(*) FROM review_financial_daily_facts`).Scan(&facts); err != nil || facts != 0 {
		t.Fatal("startup backfilled", facts, err)
	}
	copies, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".nestworth-v23-recovery-*", "original-v23.sqlite"))
	if err != nil || len(copies) != 1 {
		t.Fatal(copies, err)
	}
	info, err := os.Stat(copies[0])
	if err != nil || info.Mode().Perm() != 0400 {
		t.Fatal("recovery not read-only", err)
	}
	recovery, err := OpenReadOnlyForVerify(copies[0])
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	if err := recovery.SQL.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 23 {
		t.Fatal(version, err)
	}
	if writable, err := Open(copies[0]); err == nil {
		writable.Close()
		t.Fatal("recovery accepted as live")
	}
}
