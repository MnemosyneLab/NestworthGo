package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func populatedV15(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-v15.sqlite")
	seed, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("testdata/schema_v15.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	// Synthetic identities and exact decimal strings only; no real financial database.
	if _, err := seed.Exec(`
 INSERT INTO households VALUES('00000000-0000-4000-8000-000000000001',1,'Synthetic','USD','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');
 INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,symbol,isin,icon_key,created_at,updated_at)
 VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','基金 A','mutual_fund','USD','000001','US0000000001','chart-line','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');
 INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at)
 VALUES('00000000-0000-4000-8000-000000000003','00000000-0000-4000-8000-000000000002','123.45678901','USD','manual','manual','2026-09-01T00:00:00.000Z','2026-09-01T00:00:00.000Z');
 `); err != nil {
		t.Fatal(err)
	}
	if err := verifySchemaVersion(context.Background(), seed, 15); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// Snapshot every preexisting column in every table, sorted by its full row.
func allV15Rows(t *testing.T, db *sql.DB) map[string][][]any {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	rows.Close()
	out := map[string][][]any{}
	for _, n := range names {
		info, err := db.Query(`PRAGMA table_info("` + n + `")`)
		if err != nil {
			t.Fatal(err)
		}
		var columns string
		for info.Next() {
			var cid, notnull, pk int
			var name, typ string
			var def sql.NullString
			if err := info.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
				t.Fatal(err)
			}
			if name == "short_name" {
				continue
			}
			if columns != "" {
				columns += ","
			}
			columns += `"` + name + `"`
		}
		info.Close()
		data, err := db.Query(`SELECT ` + columns + ` FROM "` + n + `" ORDER BY ` + columns)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := data.Columns()
		out[n] = [][]any{}
		for data.Next() {
			values := make([]any, len(cols))
			dest := make([]any, len(cols))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := data.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			out[n] = append(out[n], values)
		}
		if err := data.Err(); err != nil {
			t.Fatal(err)
		}
		data.Close()
	}
	return out
}

func TestPopulatedV15UpgradeRecoveryAndReopen(t *testing.T) {
	path := populatedV15(t)
	old, err := OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	before := allV15Rows(t, old.SQL)
	old.Close()
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := allV15Rows(t, upgraded.SQL); !reflect.DeepEqual(got, before) {
		t.Fatal("upgrade changed existing columns")
	}
	var alias string
	if err := upgraded.SQL.QueryRow(`SELECT short_name FROM instruments`).Scan(&alias); err != nil || alias != "" {
		t.Fatalf("default alias %q: %v", alias, err)
	}
	if err := upgraded.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	upgraded.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Close()
	snapshots, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".nestworth-v15-recovery-*", "original-v15.sqlite"))
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("recovery snapshots %v: %v", snapshots, err)
	}
	fileInfo, err := os.Stat(snapshots[0])
	if err != nil || fileInfo.Mode().Perm() != 0600 {
		t.Fatalf("recovery permissions: %v %v", fileInfo, err)
	}
	dirInfo, err := os.Stat(filepath.Dir(snapshots[0]))
	if err != nil || dirInfo.Mode().Perm() != 0700 {
		t.Fatalf("recovery directory permissions: %v %v", dirInfo, err)
	}
	recovery, err := OpenReadOnlyForVerify(snapshots[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := allV15Rows(t, recovery.SQL); !reflect.DeepEqual(got, before) {
		t.Fatal("recovery differs from v15 records")
	}
	recovery.Close()
	// Restoring the recovery copy into a synthetic destination upgrades it again.
	data, err := os.ReadFile(snapshots[0])
	if err != nil {
		t.Fatal(err)
	}
	restore := filepath.Join(t.TempDir(), "restored.sqlite")
	if err := os.WriteFile(restore, data, 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(restore)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if got := allV15Rows(t, restored.SQL); !reflect.DeepEqual(got, before) {
		t.Fatal("restore changed original values")
	}
}

func TestV15UpgradeDDLAndVersionRollBackOnVerificationFailure(t *testing.T) {
	path := populatedV15(t)
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before := allV15Rows(t, db)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Break a required index after successful initial verification to force the final check to fail.
	if _, err := tx.Exec(`DROP INDEX idx_instruments_household`); err != nil {
		t.Fatal(err)
	}
	if err := applyV15Upgrade(context.Background(), tx); err == nil {
		t.Fatal("upgrade accepted defective schema")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := verifySchemaVersion(context.Background(), db, 15); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, allV15Rows(t, db)) {
		t.Fatal("rollback changed original values")
	}
}

func TestV15UpgradeBlocksBeforeMutationWhenRecoveryCannotBeCreated(t *testing.T) {
	path := populatedV15(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)
	if _, err := Open(path); err == nil {
		t.Fatal("opened without recovery snapshot")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed backup changed live database")
	}
}

func TestMalformedV15IsRejectedBeforeWriting(t *testing.T) {
	path := populatedV15(t)
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(`DROP INDEX idx_instruments_household`); err != nil {
		t.Fatal(err)
	}
	seed.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("accepted malformed v15")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("malformed v15 was modified")
	}
}

func TestV15UpgradeCancelledCommitRollsBack(t *testing.T) {
	path := populatedV15(t)
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before := allV15Rows(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyV15Upgrade(ctx, tx); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := tx.Commit(); err == nil {
		t.Fatal("cancelled commit succeeded")
	}
	tx.Rollback()
	if err := verifySchemaVersion(context.Background(), db, 15); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, allV15Rows(t, db)) {
		t.Fatal("cancelled transaction changed records")
	}
}
