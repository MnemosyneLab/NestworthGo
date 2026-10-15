package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func syntheticV16(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-v16.db")
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/schema_v16.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(raw)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	return path
}
func TestV16UpgradeRetainsVerifiedReadonlyRecoveryAndReopens(t *testing.T) {
	path := syntheticV16(t)
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	db.Close()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".nestworth-v16-recovery-*", "original-v16.sqlite"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("recovery: %v %v", matches, err)
	}
	if _, err = Open(matches[0]); err == nil {
		t.Fatal("recovery opened writable")
	}
	ro, err := OpenReadOnlyForVerify(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var version int
	ro.SQL.QueryRow("PRAGMA user_version").Scan(&version)
	ro.Close()
	if version != 16 {
		t.Fatal(version)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}
func TestV16RecoveryFlushFailureAndRetry(t *testing.T) {
	path := syntheticV16(t)
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	_, err = prepareV16Recovery(ctx, db, path, func(*os.File) error { return errors.New("synthetic flush failure") }, syncUpgradeDir)
	if err == nil {
		t.Fatal("flush failure ignored")
	}
	if err := verifySchemaVersion(ctx, db, 16); err != nil {
		t.Fatal(err)
	}
	if err := upgradeV16(ctx, db, path); err != nil {
		t.Fatal(err)
	}
}
func TestV16UpgradeFailureRollsBackDDL(t *testing.T) {
	path := syntheticV16(t)
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("CREATE TABLE reconciliation_items(collision TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err = applyV16Upgrade(ctx, tx); err == nil {
		t.Fatal("collision accepted")
	}
	tx.Rollback()
	if err = verifySchemaVersion(ctx, db, 16); err != nil {
		t.Fatal(err)
	}
}

func TestReconciliationReplayTriggerFreshUpgradeAndVerifierParity(t *testing.T) {
	for _, upgraded := range []bool{false, true} {
		t.Run(fmt.Sprint("upgraded_", upgraded), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "synthetic.db")
			if upgraded {
				path = syntheticV16(t)
			}
			db, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err = db.Verify(context.Background()); err != nil {
				t.Fatal(err)
			}
			for _, table := range []string{"account_values", "account_cash_values"} {
				name := "reconciliation_" + table + "_insert"
				var ddl string
				if err = db.SQL.QueryRow("SELECT sql FROM sqlite_master WHERE name=?", name).Scan(&ddl); err != nil {
					t.Fatal(err)
				}
				predicate := " WHEN NEW.projection_kind <> 'replay'"
				if !strings.Contains(ddl, predicate) {
					t.Fatalf("%s includes replay", name)
				}
				if _, err = db.SQL.Exec("DROP TRIGGER " + name); err != nil {
					t.Fatal(err)
				}
				if _, err = db.SQL.Exec(strings.Replace(ddl, predicate, "", 1)); err != nil {
					t.Fatal(err)
				}
				if err = db.Verify(context.Background()); err == nil {
					t.Fatal("verifier accepted replay revision trigger")
				}
				if _, err = db.SQL.Exec("DROP TRIGGER " + name); err != nil {
					t.Fatal(err)
				}
				if _, err = db.SQL.Exec(ddl); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
