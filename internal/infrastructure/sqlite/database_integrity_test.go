package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestFreshDatabasePassesSQLiteIntegrityAndForeignKeyChecks(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	assertSQLiteHealth(t, database.SQL)
}

func assertSQLiteHealth(t *testing.T, database *sql.DB) {
	t.Helper()
	var integrity string
	if err := database.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity_check=%q err=%v", integrity, err)
	}
	rows, err := database.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		var table, rowID, parent, foreignKey string
		if err := rows.Scan(&table, &rowID, &parent, &foreignKey); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("foreign_key_check found %s row %s parent %s key %s", table, rowID, parent, foreignKey)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
