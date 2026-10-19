package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// This reduced harness isolates trigger semantics for EVERY source column. It
// intentionally does not stand in for the actual-ledger/FK/application tests.
func sourceProbe(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys%3d1&_pragma=recursive_triggers%3d1")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, table := range financialSourceTables {
		fields := []string{}
		pk := []string{}
		for _, c := range financialSourceColumns(table) {
			fields = append(fields, c.name+" "+c.typeName)
			if c.primaryKey > 0 {
				pk = append(pk, c.name)
			}
		}
		fields = append(fields, "PRIMARY KEY("+strings.Join(pk, ",")+")")
		if _, err = db.Exec("CREATE TABLE " + table + "(" + strings.Join(fields, ",") + ")"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`CREATE TABLE investment_review_control(projection_status TEXT,published_input_fingerprint TEXT,published_on TEXT)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = installFinancialSourceTracking(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO households(id,name,base_currency) VALUES('synthetic-household','Synthetic','CNY')`); err != nil {
		t.Fatal(err)
	}
	return db
}
func sourceRevision(t *testing.T, q schemaQuery) int64 {
	t.Helper()
	var n int64
	if err := q.QueryRowContext(t.Context(), `SELECT revision FROM financial_source_revision`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestFinancialSourceEveryTableColumnAndOperation(t *testing.T) {
	// Sequential cases remove their source row and retain the journal so every
	// column is checked without recompiling all trigger DDL once per table.
	db := sourceProbe(t)
	for _, table := range financialSourceTables {
		t.Run(table, func(t *testing.T) {
			if table != "households" {
				cols, values, args := []string{}, []string{}, []any{}
				for _, c := range financialSourceColumns(table) {
					cols = append(cols, c.name)
					values = append(values, "?")
					var v any = "old"
					if c.typeName == "INTEGER" {
						v = 1
					}
					if c.name == "projection_kind" {
						v = "baseline"
					}
					args = append(args, v)
				}
				before := sourceRevision(t, db)
				if _, err := db.Exec("INSERT INTO "+table+"("+strings.Join(cols, ",")+") VALUES("+strings.Join(values, ",")+")", args...); err != nil {
					t.Fatal(err)
				}
				if got := sourceRevision(t, db); got != before+1 {
					t.Fatalf("insert invisible: %d -> %d", before, got)
				}
			}
			for _, c := range financialSourceColumns(table) {
				if table == "households" && c.primaryKey > 0 {
					// The revision FK deliberately rejects changing the household ID.
					continue
				}
				before := sourceRevision(t, db)
				var v any = "new"
				if c.typeName == "INTEGER" {
					v = 2
				}
				if c.name == "projection_kind" {
					v = "event"
				}
				if _, err := db.Exec("UPDATE "+table+" SET "+c.name+"=?", v); err != nil {
					t.Fatal(err)
				}
				want := before + 1
				if table == "history_snapshot_state" && c.name != "input_generation" && c.name != "resolver_policy_version" && c.name != "household_id" {
					want = before
				}
				if got := sourceRevision(t, db); got != want {
					t.Fatalf("column %s revision=%d want=%d", c.name, got, want)
				}
				// Identical writes never generate journal rows.
				if _, err := db.Exec("UPDATE "+table+" SET "+c.name+"=?", v); err != nil {
					t.Fatal(err)
				}
				if got := sourceRevision(t, db); got != want {
					t.Fatalf("same-value %s changed revision", c.name)
				}
			}
			if table != "households" {
				before := sourceRevision(t, db)
				if _, err := db.Exec("DELETE FROM " + table); err != nil {
					t.Fatal(err)
				}
				if sourceRevision(t, db) != before+1 {
					t.Fatal("delete invisible")
				}
			}
			var count int64
			if err := db.QueryRow(`SELECT count(*) FROM financial_source_changes`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != sourceRevision(t, db) {
				t.Fatal("revision/journal gap")
			}
		})
	}
}
func TestFinancialSourceOldNewRollbackReplaceAndReplay(t *testing.T) {
	db := sourceProbe(t)
	if _, err := db.Exec(`INSERT INTO instrument_quotes(id,instrument_id,unit_price,quoted_at,value_effective_at) VALUES('quote','old-instrument','1.0001','2026-10-01T00:00:00.000000001Z','2026-10-01T00:00:00.000000001Z')`); err != nil {
		t.Fatal(err)
	}
	before := sourceRevision(t, db)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE instrument_quotes SET instrument_id='new-instrument',unit_price='1.0002',value_effective_at='2026-11-01T00:00:00.000000002Z'`); err != nil {
		t.Fatal(err)
	}
	var oldScope, newScope, oldAt, newAt string
	if err = tx.QueryRow(`SELECT old_scope_json,new_scope_json,old_effective_at,new_effective_at FROM financial_source_changes ORDER BY revision DESC LIMIT 1`).Scan(&oldScope, &newScope, &oldAt, &newAt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(oldScope, "old-instrument") || !strings.Contains(newScope, "new-instrument") || oldAt != "2026-10-01T00:00:00.000000001Z" || newAt != "2026-11-01T00:00:00.000000002Z" {
		t.Fatalf("lost dependencies: %s %s %s %s", oldScope, newScope, oldAt, newAt)
	}
	if strings.Contains(oldScope, "1.0001") || strings.Contains(newScope, "1.0002") {
		t.Fatal("financial payload copied into scope")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if sourceRevision(t, db) != before {
		t.Fatal("rollback leaked revision")
	}
	// Even a foreign connection with recursive triggers disabled gets a full
	// household invalidation for REPLACE insertion; managed connections use ON.
	if _, err = db.Exec(`PRAGMA recursive_triggers=OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT OR REPLACE INTO instrument_quotes(id,instrument_id) VALUES('quote','replacement')`); err != nil {
		t.Fatal(err)
	}
	if sourceRevision(t, db) != before+1 {
		t.Fatal("REPLACE not conservatively invalidated")
	}
	if _, err = db.Exec(`INSERT INTO holding_quantity_values(id,projection_kind) VALUES('derived','replay')`); err != nil {
		t.Fatal(err)
	}
	before = sourceRevision(t, db)
	if _, err = db.Exec(`UPDATE holding_quantity_values SET quantity='1.0002' WHERE id='derived'`); err != nil {
		t.Fatal(err)
	}
	if sourceRevision(t, db) != before {
		t.Fatal("replay caused source feedback")
	}
	if _, err = db.Exec(`UPDATE holding_quantity_values SET projection_kind='baseline' WHERE id='derived'`); err != nil {
		t.Fatal(err)
	}
	if sourceRevision(t, db) != before+1 {
		t.Fatal("promotion to source not tracked")
	}
	const secret = "SYNTHETIC_CREDENTIAL_CANARY"
	if _, err = db.Exec(`INSERT INTO app_configuration VALUES(?,?)`, secret, secret); err != nil {
		t.Fatal(err)
	}
	var leaked int
	if err = db.QueryRow(`SELECT count(*) FROM financial_source_changes WHERE source_id LIKE ? OR old_scope_json LIKE ? OR new_scope_json LIKE ?`, "%"+secret+"%", "%"+secret+"%", "%"+secret+"%").Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("credential leak: %d %v", leaked, err)
	}
}
func TestFinancialSourceActualDatabaseConcurrentRollbackAndVerifier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.SQL.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	before := sourceRevision(t, db.SQL)
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn := db.SQL
			if i%2 == 1 {
				conn = other.SQL
			}
			_, e := conn.Exec(`INSERT INTO app_configuration VALUES(?,?)`, fmt.Sprintf("synthetic/%d", i), "value")
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if sourceRevision(t, db.SQL) != before+20 {
		t.Fatal("concurrent revision lost")
	}
	if err = db.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, e := tx.Exec(`UPDATE households SET name='rolled back'`)
		if e != nil {
			return e
		}
		return fmt.Errorf("synthetic rollback")
	}); err == nil {
		t.Fatal("expected rollback")
	}
	if sourceRevision(t, db.SQL) != before+20 {
		t.Fatal("failed business transaction logged")
	}
	if _, err = db.SQL.Exec(`DROP TRIGGER financial_source_accounts_update`); err != nil {
		t.Fatal(err)
	}
	if err = db.Verify(context.Background()); err == nil {
		t.Fatal("missing trigger accepted")
	}
}

func TestFinancialSourceActualCascadeAndOldNewIdentity(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "synthetic-cascade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`,
		`INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,icon_key,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','Synthetic','stock','CNY','chart-line','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`,
		`INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at) VALUES('00000000-0000-4000-8000-000000000003','00000000-0000-4000-8000-000000000002','1.0001','CNY','manual','manual','2026-10-01T00:00:00.000000001Z','2026-10-01T00:00:00Z')`,
	} {
		if _, err = db.SQL.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	before := sourceRevision(t, db.SQL)
	if _, err = db.SQL.Exec(`UPDATE instrument_quotes SET id='00000000-0000-4000-8000-000000000004'`); err != nil {
		t.Fatal(err)
	}
	var oldID, newID string
	if err = db.SQL.QueryRow(`SELECT json_extract(old_scope_json,'$.id'),json_extract(new_scope_json,'$.id') FROM financial_source_changes ORDER BY revision DESC LIMIT 1`).Scan(&oldID, &newID); err != nil || oldID == newID || sourceRevision(t, db.SQL) != before+1 {
		t.Fatalf("identity move lost: %s %s %v", oldID, newID, err)
	}
	before = sourceRevision(t, db.SQL)
	if _, err = db.SQL.Exec(`DELETE FROM instruments`); err != nil {
		t.Fatal(err)
	}
	var deletes int
	if err = db.SQL.QueryRow(`SELECT count(*) FROM financial_source_changes WHERE revision>? AND operation='delete' AND source_table IN ('instruments','instrument_quotes')`, before).Scan(&deletes); err != nil || deletes != 2 || sourceRevision(t, db.SQL) != before+2 {
		t.Fatalf("cascade lost: count=%d err=%v", deletes, err)
	}
	if err = db.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Exercise household deletion with live cascading source children too.
	if _, err = db.SQL.Exec(`INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,icon_key,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','Synthetic','stock','CNY','chart-line','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z'); INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at) VALUES('00000000-0000-4000-8000-000000000003','00000000-0000-4000-8000-000000000002','1.0001','CNY','manual','manual','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.SQL.Exec(`DELETE FROM households`); err != nil {
		t.Fatal(err)
	}
	if err = db.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestFinancialSourceParentTimesAndTamperedVerifier(t *testing.T) {
	db := sourceProbe(t)
	if _, err := db.Exec(`INSERT INTO activities(id,effective_at) VALUES('activity','2026-10-01T00:00:00.000000001Z'); INSERT INTO activity_effects(id,activity_id) VALUES('effect','activity')`); err != nil {
		t.Fatal(err)
	}
	var oldAt, newAt string
	if _, err := db.Exec(`UPDATE activity_effects SET quantity='0.0001'`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT old_effective_at,new_effective_at FROM financial_source_changes ORDER BY revision DESC LIMIT 1`).Scan(&oldAt, &newAt); err != nil || oldAt != "2026-10-01T00:00:00.000000001Z" || newAt != oldAt {
		t.Fatalf("parent time lost %s/%s %v", oldAt, newAt, err)
	}
	actual, err := Open(filepath.Join(t.TempDir(), "synthetic-tamper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer actual.Close()
	if _, err = actual.SQL.Exec(`PRAGMA recursive_triggers=OFF`); err != nil {
		t.Fatal(err)
	}
	if err = actual.Verify(t.Context()); err == nil {
		t.Fatal("disabled recursive triggers accepted")
	}
	if _, err = actual.SQL.Exec(`PRAGMA recursive_triggers=ON; DROP TRIGGER financial_source_households_update; CREATE TRIGGER financial_source_households_update BEFORE UPDATE ON households BEGIN SELECT 1; END`); err != nil {
		t.Fatal(err)
	}
	if err = actual.Verify(t.Context()); err == nil {
		t.Fatal("tampered trigger body accepted")
	}
}

func TestFinancialSourceManifestMatchesActualSchema(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "synthetic-manifest.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range financialSourceTables {
		rows, err := db.SQL.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		names := []string{}
		for rows.Next() {
			var cid, notNull, pk int
			var name, typ string
			var defaultValue any
			if err = rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		expected := []string{}
		for _, column := range financialSourceColumns(table) {
			expected = append(expected, column.name)
		}
		if strings.Join(names, ",") != strings.Join(expected, ",") {
			t.Fatalf("manifest differs from actual %s: %v/%v", table, names, expected)
		}
		t.Logf("SOURCE_MANIFEST\t%s\t%s\t%s\t%s", table, strings.Join(expected, ","), financialScope(table, "OLD"), financialEffective(table, "OLD"))
	}
}
