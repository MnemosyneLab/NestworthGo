package sqlite

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// These use the real schema/driver and the production configuration UPSERT.
// Keep each conflict case independent so a phantom insert cannot hide another.
func financialConflictDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "synthetic-conflict.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.SQL.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z'); INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,icon_key,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','Synthetic','stock','CNY','chart-line','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func conflictSourceInsert(table, id, value string) (string, []any) {
	if table == "app_configuration" {
		return `INSERT INTO app_configuration(key,value) VALUES(?,?)`, []any{id, value}
	}
	return `INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at) VALUES(?,'00000000-0000-4000-8000-000000000002',?,'CNY','manual','manual','2026-10-01T00:00:00.000000001Z','2026-10-01T00:00:00Z')`, []any{id, value}
}
func conflictSourceUpsert(table string) string {
	if table == "app_configuration" {
		return " ON CONFLICT(key) DO UPDATE SET value=excluded.value"
	}
	return " ON CONFLICT(id) DO UPDATE SET unit_price=excluded.unit_price"
}
func conflictSourceState(t *testing.T, db *DB, table string) (int64, int64, string) {
	t.Helper()
	var count int64
	var value string
	key, column := "id", "unit_price"
	if table == "app_configuration" {
		key, column = "key", "value"
	}
	if err := db.SQL.QueryRow(`SELECT ` + column + ` FROM ` + table + ` WHERE ` + key + `='00000000-0000-4000-8000-000000000003'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if err := db.SQL.QueryRow(`SELECT count(*) FROM financial_source_changes`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return sourceRevision(t, db.SQL), count, value
}

func TestFinancialSourceInsertConflictSemantics(t *testing.T) {
	for _, table := range []string{"app_configuration", "instrument_quotes"} {
		t.Run(table, func(t *testing.T) {
			for _, scenario := range []string{"fresh-insert", "identical-upsert", "changed-upsert", "ignored-insert"} {
				t.Run(scenario, func(t *testing.T) {
					db := financialConflictDB(t)
					statement, args := conflictSourceInsert(table, "00000000-0000-4000-8000-000000000003", "1.0001")
					if scenario != "fresh-insert" {
						if _, err := db.SQL.Exec(statement, args...); err != nil {
							t.Fatal(err)
						}
					}
					before := sourceRevision(t, db.SQL)
					var beforeCount int64
					if err := db.SQL.QueryRow(`SELECT count(*) FROM financial_source_changes`).Scan(&beforeCount); err != nil {
						t.Fatal(err)
					}
					wantDelta := int64(1)
					wantOperation := "insert"
					wantValue := "1.0001"
					var err error
					switch scenario {
					case "fresh-insert":
						_, err = db.SQL.Exec(statement, args...)
					case "identical-upsert":
						wantDelta = 0
						if table == "app_configuration" {
							err = NewConfigurationRepository(db).SaveConfiguration(args[0].(string), []byte("1.0001"))
						} else {
							_, err = db.SQL.Exec(statement+conflictSourceUpsert(table), args...)
						}
					case "changed-upsert":
						wantOperation = "update"
						wantValue = "1.0002"
						args[1] = wantValue
						if table == "app_configuration" {
							err = NewConfigurationRepository(db).SaveConfiguration(args[0].(string), []byte(wantValue))
						} else {
							_, err = db.SQL.Exec(statement+conflictSourceUpsert(table), args...)
						}
					case "ignored-insert":
						wantDelta = 0
						args[1] = "1.0002"
						var result sql.Result
						result, err = db.SQL.Exec(strings.Replace(statement, "INSERT INTO", "INSERT OR IGNORE INTO", 1), args...)
						if err == nil {
							if n, e := result.RowsAffected(); e != nil || n != 0 {
								t.Fatalf("ignored insert affected source: %d %v", n, e)
							}
						}
					}
					if err != nil {
						t.Fatal(err)
					}
					revision, count, value := conflictSourceState(t, db, table)
					if revision != before+wantDelta || count != beforeCount+wantDelta || value != wantValue {
						t.Fatalf("%s: revision delta=%d journal delta=%d value=%s; want %d/%s", scenario, revision-before, count-beforeCount, value, wantDelta, wantValue)
					}
					if wantDelta > 0 {
						var operation string
						if err = db.SQL.QueryRow(`SELECT operation FROM financial_source_changes ORDER BY revision DESC LIMIT 1`).Scan(&operation); err != nil || operation != wantOperation {
							t.Fatalf("operation=%s want=%s err=%v", operation, wantOperation, err)
						}
					}
					if err = db.Verify(t.Context()); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func TestFinancialSourceReplaceAndRollback(t *testing.T) {
	for _, table := range []string{"app_configuration", "instrument_quotes"} {
		t.Run(table, func(t *testing.T) {
			for _, recursive := range []int{0, 1} {
				t.Run(fmt.Sprintf("recursive-%d", recursive), func(t *testing.T) {
					db := financialConflictDB(t)
					statement, args := conflictSourceInsert(table, "00000000-0000-4000-8000-000000000003", "1.0001")
					if _, err := db.SQL.Exec(statement, args...); err != nil {
						t.Fatal(err)
					}
					before, count, _ := conflictSourceState(t, db, table)
					if _, err := db.SQL.Exec(fmt.Sprintf(`PRAGMA recursive_triggers=%d`, recursive)); err != nil {
						t.Fatal(err)
					}
					args[1] = "1.0002"
					replace := strings.Replace(statement, "INSERT INTO", "INSERT OR REPLACE INTO", 1)
					if _, err := db.SQL.Exec(replace, args...); err != nil {
						t.Fatal(err)
					}
					revision, afterCount, value := conflictSourceState(t, db, table)
					delta := int64(1 + recursive)
					if revision != before+delta || afterCount != count+delta || value != "1.0002" {
						t.Fatalf("replacement lost: revision=%d count=%d value=%s", revision, afterCount, value)
					}
					var insertCount, deleteCount int
					if err := db.SQL.QueryRow(`SELECT count(*) FILTER(WHERE operation='insert'),count(*) FILTER(WHERE operation='delete') FROM financial_source_changes WHERE revision>?`, before).Scan(&insertCount, &deleteCount); err != nil || insertCount != 1 || deleteCount != recursive {
						t.Fatalf("REPLACE operations insert=%d delete=%d err=%v", insertCount, deleteCount, err)
					}
					tx, err := db.SQL.BeginTx(t.Context(), nil)
					if err != nil {
						t.Fatal(err)
					}
					args[1] = "1.0003"
					if _, err = tx.Exec(replace, args...); err != nil {
						t.Fatal(err)
					}
					if err = tx.Rollback(); err != nil {
						t.Fatal(err)
					}
					gotRevision, gotCount, gotValue := conflictSourceState(t, db, table)
					if gotRevision != revision || gotCount != afterCount || gotValue != value {
						t.Fatal("REPLACE rollback leaked source/revision/journal")
					}
					if _, err = db.SQL.Exec(`PRAGMA recursive_triggers=ON`); err != nil {
						t.Fatal(err)
					}
					if err = db.Verify(t.Context()); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func TestFinancialSourceFailedJournalPublicationRollsBackSource(t *testing.T) {
	for _, table := range []string{"app_configuration", "instrument_quotes"} {
		t.Run(table, func(t *testing.T) {
			db := financialConflictDB(t)
			statement, args := conflictSourceInsert(table, "00000000-0000-4000-8000-000000000003", "1.0001")
			if _, err := db.SQL.Exec(statement, args...); err != nil {
				t.Fatal(err)
			}
			before, count, value := conflictSourceState(t, db, table)
			if _, err := db.SQL.Exec(`CREATE TRIGGER synthetic_reject_journal BEFORE INSERT ON financial_source_changes BEGIN SELECT RAISE(ABORT,'synthetic journal failure'); END`); err != nil {
				t.Fatal(err)
			}
			args[1] = "1.0002"
			fresh, freshArgs := conflictSourceInsert(table, "00000000-0000-4000-8000-000000000004", "1.0002")
			key := "id"
			if table == "app_configuration" {
				key = "key"
			}
			operations := []struct {
				name, sql string
				args      []any
			}{
				{"fresh-insert", fresh, freshArgs},
				{"fresh-ignored-insert", strings.Replace(fresh, "INSERT INTO", "INSERT OR IGNORE INTO", 1), freshArgs},
				{"changed-upsert", statement + conflictSourceUpsert(table), args},
				{"replace", strings.Replace(statement, "INSERT INTO", "INSERT OR REPLACE INTO", 1), args},
				{"delete", `DELETE FROM ` + table + ` WHERE ` + key + `=?`, args[:1]},
			}
			for _, operation := range operations {
				t.Run(operation.name, func(t *testing.T) {
					if _, err := db.SQL.Exec(operation.sql, operation.args...); err == nil || !strings.Contains(err.Error(), "synthetic journal failure") {
						t.Fatalf("journal failure not propagated: %v", err)
					}
					gotRevision, gotCount, gotValue := conflictSourceState(t, db, table)
					if gotRevision != before || gotCount != count || gotValue != value {
						t.Fatal("failed journal publication leaked source/revision/journal")
					}
					var sourceRows int
					if err := db.SQL.QueryRow(`SELECT count(*) FROM ` + table).Scan(&sourceRows); err != nil || sourceRows != 1 {
						t.Fatalf("failed source row persisted: %d %v", sourceRows, err)
					}
				})
			}
			if _, err := db.SQL.Exec(`DROP TRIGGER synthetic_reject_journal`); err != nil {
				t.Fatal(err)
			}
			if err := db.Verify(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFinancialSourceVerifierRejectsBeforeInsertDefinition(t *testing.T) {
	db := financialConflictDB(t)
	old := strings.Replace(financialTriggerStatements()["financial_source_app_configuration_insert"], " AFTER INSERT ", " BEFORE INSERT ", 1)
	if _, err := db.SQL.Exec(`DROP TRIGGER financial_source_app_configuration_insert; ` + old); err != nil {
		t.Fatal(err)
	}
	if err := db.Verify(t.Context()); err == nil {
		t.Fatal("BEFORE INSERT definition accepted")
	}
}
