package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestV014Schema5FixtureIsSanitizedAndComplete(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "testdata", "v0.1.4", "schema5-fixture.sql")
	script, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	text := string(script)
	for _, forbidden := range []string{"/Users/", "production", "credential", "provider token"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("fixture contains forbidden marker %q", forbidden)
		}
	}

	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	defer database.Close()
	if _, err := database.Exec(text); err != nil {
		t.Fatalf("execute fixture: %v", err)
	}
	var version int
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read fixture version: %v", err)
	}
	if version != 5 {
		t.Fatalf("fixture schema = %d, want 5", version)
	}

	checks := []struct {
		name  string
		query string
		want  int
	}{
		{"households", `SELECT COUNT(*) FROM households`, 1},
		{"members", `SELECT COUNT(*) FROM members WHERE household_id = '00000000-0000-4000-8000-000000000001'`, 3},
		{"history origins", `SELECT COUNT(*) FROM history_origins`, 1},
		{"starting point holdings", `SELECT COUNT(*) FROM history_origin_components WHERE component_kind = 'holding_quantity'`, 2},
		{"activities", `SELECT COUNT(*) FROM activities`, 5},
		{"trades", `SELECT COUNT(*) FROM activity_trade_details`, 3},
		{"position transfer effects", `SELECT COUNT(*) FROM activity_effects WHERE activity_id = '00000000-0000-4000-8000-000000000112'`, 2},
		{"undo", `SELECT COUNT(*) FROM activities WHERE reverses_activity_id IS NOT NULL`, 1},
		{"fix replacement", `SELECT COUNT(*) FROM activity_correction_groups WHERE replacement_activity_id IS NOT NULL`, 1},
		{"manual instrument quotes", `SELECT COUNT(*) FROM instrument_quotes WHERE source_kind = 'manual'`, 3},
		{"provider instrument quotes", `SELECT COUNT(*) FROM instrument_quotes WHERE source_kind = 'provider'`, 3},
		{"provider FX quotes", `SELECT COUNT(*) FROM fx_quotes WHERE source_kind = 'provider'`, 2},
	}
	for _, check := range checks {
		var got int
		if err := database.QueryRowContext(context.Background(), check.query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", check.name, err)
		}
		if got != check.want {
			t.Fatalf("%s = %d, want %d", check.name, got, check.want)
		}
	}
	assertSQLiteHealth(t, database)

	var summary struct {
		BaselineCommit string `json:"baseline_commit"`
		SchemaVersion  int    `json:"schema_version"`
		Fixture        string `json:"fixture"`
	}
	summaryPath := filepath.Join("..", "..", "..", "testdata", "v0.1.4", "baseline-summary.json")
	summaryBytes, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read baseline summary: %v", err)
	}
	if err := json.Unmarshal(summaryBytes, &summary); err != nil {
		t.Fatalf("decode baseline summary: %v", err)
	}
	if summary.BaselineCommit != "3d0a274f2addc89695dd11e582b7a0f73ef7f2db" || summary.SchemaVersion != 5 || summary.Fixture != "schema5-fixture.sql" {
		t.Fatalf("baseline summary does not identify the frozen v0.1.3/schema-5 fixture: %+v", summary)
	}

	path := filepath.Join(t.TempDir(), "schema5.db")
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open file fixture: %v", err)
	}
	if _, err := seed.Exec(text); err != nil {
		_ = seed.Close()
		t.Fatalf("seed file fixture: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close file fixture: %v", err)
	}
	opened, err := Open(path)
	if err != nil {
		t.Fatalf("Open rejected frozen schema-5 fixture: %v", err)
	}
	if opened.Status != StatusMigrated {
		t.Fatalf("frozen fixture status = %q, want migrated to current schema", opened.Status)
	}
	if err := opened.Verify(context.Background()); err != nil {
		t.Fatalf("frozen fixture verification failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("close opened fixture: %v", err)
	}
}
