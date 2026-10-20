package sqlite

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestV21ClosureUpgradePreservesRecoveryAndTracksFacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-v21.db")
	old := syntheticV20(t, path)
	tx, err := old.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = applyV20Upgrade(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	old.Close()
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var version int
	if err = upgraded.SQL.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != CurrentSchemaVersion {
		t.Fatalf("version %d %v", version, err)
	}
	copies, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".nestworth-v21-recovery-*", "original-v21.sqlite"))
	if err != nil || len(copies) != 1 {
		t.Fatalf("recovery: %v %v", copies, err)
	}
	info, err := os.Stat(copies[0])
	if err != nil || info.Mode().Perm() != 0400 {
		t.Fatalf("recovery mode: %v %v", info, err)
	}
	recovery, err := OpenReadOnlyForVerify(copies[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = recovery.SQL.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 21 {
		t.Fatal("recovery mutated")
	}
	recovery.Close()
	if writable, err := Open(copies[0]); err == nil {
		writable.Close()
		t.Fatal("opened recovery writable")
	}
	if err = verifySchema(t.Context(), upgraded.SQL); err != nil {
		t.Fatal(err)
	}
	// Every closure insertion/update/deletion is in the financial trigger manifest.
	for _, op := range []string{"insert", "update", "delete"} {
		var n int
		if err = upgraded.SQL.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='trigger' AND name=?`, "financial_source_agent_instrument_closures_"+op).Scan(&n); err != nil || n != 1 {
			t.Fatalf("missing tracking %s %v", op, err)
		}
	}
}
func TestV21ClosureUpgradeStopsAfterRecoveryFlushFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-v21.db")
	old := syntheticV20(t, path)
	defer old.Close()
	tx, err := old.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = applyV20Upgrade(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("synthetic fsync failure")
	if _, err = prepareV21Recovery(t.Context(), old, path, func(*os.File) error { return failure }, syncV21UpgradeDir); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	var version int
	if err = old.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 21 {
		t.Fatal("upgrade ran before recovery")
	}
}
