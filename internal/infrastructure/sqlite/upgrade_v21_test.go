package sqlite

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/testutil"
)

func TestV21ClosureUpgradePreservesRecoveryAndTracksFacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-v21.db")
	old := syntheticV21(t, path)
	old.Close()
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var version int
	if err = upgraded.SQL.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 22 {
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
	old := syntheticV21(t, path)
	defer old.Close()
	failure := errors.New("synthetic fsync failure")
	if _, err := prepareV21Recovery(t.Context(), old, path, func(*os.File) error { return failure }, syncV21UpgradeDir); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	var version int
	if err := old.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 21 {
		t.Fatal("upgrade ran before recovery")
	}
}

func syntheticV21(t *testing.T, path string) *sql.DB {
	t.Helper()
	current, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Schema21(t, current.SQL)
	if err := verifySchemaVersion(t.Context(), current.SQL, 21); err != nil {
		t.Fatal(err)
	}
	return current.SQL
}
