package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestV013Schema3FixtureIsSanitizedAndIntact(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "testdata", "v0.1.3", "schema3-fixture.sql")
	script, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	defer database.Close()
	if _, err := database.Exec(string(script)); err != nil {
		t.Fatalf("execute fixture: %v", err)
	}
	var version int
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read fixture version: %v", err)
	}
	if version != 3 {
		t.Fatalf("fixture schema = %d, want 3", version)
	}
	var tableCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'").Scan(&tableCount); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount != 14 {
		t.Fatalf("fixture table count = %d, want 14", tableCount)
	}
	for _, check := range []struct {
		query string
		want  int
	}{
		{`SELECT COUNT(*) FROM households WHERE base_currency = 'CNY'`, 1},
		{`SELECT COUNT(*) FROM members WHERE archived_at IS NOT NULL`, 1},
		{`SELECT COUNT(*) FROM holdings WHERE quantity = '0'`, 1},
		{`SELECT COUNT(*) FROM instrument_quotes WHERE source_kind = 'provider'`, 2},
		{`SELECT COUNT(*) FROM fx_preferences`, 2},
	} {
		var got int
		if err := database.QueryRowContext(context.Background(), check.query).Scan(&got); err != nil {
			t.Fatalf("query %q: %v", check.query, err)
		}
		if got != check.want {
			t.Fatalf("query %q = %d, want %d", check.query, got, check.want)
		}
	}
	var integrity string
	if err := database.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity_check = %q, err=%v", integrity, err)
	}
	rows, err := database.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("fixture has a foreign-key violation")
	}

	path := filepath.Join(t.TempDir(), "schema3.db")
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open file fixture: %v", err)
	}
	if _, err := seed.Exec(string(script)); err != nil {
		_ = seed.Close()
		t.Fatalf("seed file fixture: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close file fixture: %v", err)
	}
	opened, err := Open(path)
	if err != nil {
		t.Fatalf("Open rejected frozen schema-3 fixture: %v", err)
	}
	if opened.Status != StatusMigrated {
		t.Fatalf("frozen fixture status = %q, want migrated to current schema", opened.Status)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("close opened fixture: %v", err)
	}
}
