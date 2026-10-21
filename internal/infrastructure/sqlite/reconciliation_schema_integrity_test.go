package sqlite

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupportedReconciliationReplayTriggerVerifier(t *testing.T) {
	for _, version := range []int{21, 22} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "synthetic.db")
			if version == 21 {
				old := syntheticV21(t, path)
				old.Close()
			}
			db, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err = db.Verify(t.Context()); err != nil {
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
				if err = db.Verify(t.Context()); err == nil {
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
