package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSchema6BackfillsFixtureCostsAndReopensIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema5-fixture.db")
	seedSchema5Fixture(t, path)

	database, err := Open(path)
	if err != nil {
		t.Fatalf("migrate schema5 fixture: %v", err)
	}
	if database.Status != StatusMigrated {
		t.Fatalf("migration status = %q, want migrated", database.Status)
	}
	assertSchema6FixtureCosts(t, database.SQL)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen schema6 fixture: %v", err)
	}
	defer reopened.Close()
	if reopened.Status != StatusReady {
		t.Fatalf("reopen status = %q, want ready", reopened.Status)
	}
	assertSchema6FixtureCosts(t, reopened.SQL)
}

func TestSchema6BackfillFailsWithoutQuoteOrTrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema5-missing-cost.db")
	seedSchema5Fixture(t, path)
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`DELETE FROM instrument_quotes WHERE instrument_id = '00000000-0000-4000-8000-000000000040'`); err != nil {
		_ = legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(path)
	var bootstrapErr *BootstrapError
	if !errors.As(err, &bootstrapErr) || bootstrapErr.Status != StatusMigrationFailed {
		t.Fatalf("Open error = %v, want migration failure", err)
	}
	if !strings.Contains(err.Error(), "00000000-0000-4000-8000-000000000053") {
		t.Fatalf("migration failure = %v, want the unresolved Holding ID", err)
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version int
	if err := check.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 5 {
		t.Fatalf("failed migration changed schema version to %d", version)
	}
	var columns int
	if err := check.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('history_origin_components') WHERE name = 'unit_cost'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 0 {
		t.Fatal("failed migration left the schema6 unit_cost column behind")
	}
}

func TestVerifyRejectsPositiveHoldingWithoutCostBasis(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema6-verify.db")
	seedSchema5Fixture(t, path)
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.SQL.Exec(`UPDATE history_origin_components SET unit_cost = NULL WHERE holding_id = '00000000-0000-4000-8000-000000000051'`); err != nil {
		t.Fatal(err)
	}
	if err := database.Verify(context.Background()); err == nil || !strings.Contains(err.Error(), "no resolvable cost basis") {
		t.Fatalf("Verify error = %v, want missing cost-basis error", err)
	}
}

func seedSchema5Fixture(t *testing.T, path string) {
	t.Helper()
	fixturePath := filepath.Join("..", "..", "..", "testdata", "v0.1.4", "schema5-fixture.sql")
	script, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(script)); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertSchema6FixtureCosts(t *testing.T, database *sql.DB) {
	t.Helper()
	var version int
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 6 {
		t.Fatalf("schema version = %d, want 6", version)
	}
	for table, column := range map[string]string{"history_origin_components": "unit_cost", "activity_effects": "cost_unit_price"} {
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s.%s count = %d, want 1", table, column, count)
		}
	}
	rows, err := database.Query(`SELECT holding_id, unit_cost FROM history_origin_components WHERE component_kind = 'holding_quantity' AND CAST(quantity AS REAL) > 0 ORDER BY holding_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := map[string]string{
		"00000000-0000-4000-8000-000000000050": "720",
		"00000000-0000-4000-8000-000000000051": "4.1",
		"00000000-0000-4000-8000-000000000053": "720",
	}
	got := make(map[string]string)
	for rows.Next() {
		var holdingID, cost string
		if err := rows.Scan(&holdingID, &cost); err != nil {
			t.Fatal(err)
		}
		got[holdingID] = cost
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("backfilled positive components = %#v, want %#v", got, want)
	}
	for holdingID, wantCost := range want {
		if got[holdingID] != wantCost {
			t.Fatalf("holding %s unit cost = %q, want %q", holdingID, got[holdingID], wantCost)
		}
	}
}
