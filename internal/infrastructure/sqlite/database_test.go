package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenCreatesAndVerifiesDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nestworth.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.Close()
	if database.Status != StatusMigrated {
		t.Fatalf("database status = %q, want migrated", database.Status)
	}
	var version int
	if err := database.SQL.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}
	var table string
	if err := database.SQL.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'accounts'").Scan(&table); err != nil {
		t.Fatalf("accounts table missing: %v", err)
	}
}

func TestOpenMigratesSchemaV1IconColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema-v1.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if _, err := legacy.Exec(string(schema)); err != nil {
		t.Fatalf("create schema v1: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}
	database, err := Open(path)
	if err != nil {
		t.Fatalf("migrate schema v1: %v", err)
	}
	defer database.Close()
	if database.Status != StatusMigrated {
		t.Fatalf("database status = %q, want migrated", database.Status)
	}
	for _, table := range []string{"institutions", "accounts"} {
		var count int
		if err := database.SQL.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'icon_key'", table).Scan(&count); err != nil {
			t.Fatalf("inspect %s columns: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("%s.icon_key count = %d, want 1", table, count)
		}
	}
}

func TestOpenMigratesSanitizedSchema2FixtureAndReopensIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema2.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open schema2 fixture: %v", err)
	}
	fixturePath := filepath.Join("..", "..", "..", "testdata", "v0.1.2", "schema2-v0.1.1.sql")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read schema2 fixture %s: %v", fixturePath, err)
	}
	if _, err := legacy.Exec(string(fixture)); err != nil {
		t.Fatalf("seed schema2 fixture: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close schema2 fixture: %v", err)
	}
	database, err := Open(path)
	if err != nil {
		t.Fatalf("migrate schema2 fixture: %v", err)
	}
	if database.Status != StatusMigrated {
		t.Fatalf("migration status = %q, want migrated", database.Status)
	}
	var version int
	if err := database.SQL.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, err = %v", version, err)
	}
	var accountCount, valueCount int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&accountCount); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM account_values").Scan(&valueCount); err != nil {
		t.Fatal(err)
	}
	if accountCount != 3 || valueCount != 3 {
		t.Fatalf("legacy rows changed during migration: accounts=%d values=%d", accountCount, valueCount)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PreMigrationSnapshotPath(path, 2)); err != nil {
		t.Fatalf("schema2 pre-migration snapshot missing: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen migrated schema2 fixture: %v", err)
	}
	defer reopened.Close()
	if reopened.Status != StatusReady {
		t.Fatalf("reopen status = %q, want ready", reopened.Status)
	}
	var triggerCount int
	if err := reopened.SQL.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger'").Scan(&triggerCount); err != nil {
		t.Fatal(err)
	}
	if triggerCount != 0 {
		t.Fatalf("history migration created %d triggers", triggerCount)
	}
	for _, table := range []string{"history_origins", "history_origin_components", "activities", "activity_effects", "daily_valuation_snapshots", "history_snapshot_state"} {
		var count int
		if err := reopened.SQL.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("schema4 table %s count = %d, want 1", table, count)
		}
	}
	if err := reopened.Verify(context.Background()); err != nil {
		t.Fatalf("reopened database verification: %v", err)
	}
}

func TestOpenBlocksUnsupportedFutureDatabaseWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	initial, err := Open(path)
	if err != nil {
		t.Fatalf("initial Open returned error: %v", err)
	}
	if _, err := initial.SQL.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatalf("set future schema version: %v", err)
	}
	if err := initial.Close(); err != nil {
		t.Fatalf("close initial database: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read future database: %v", err)
	}

	_, err = Open(path)
	var bootstrapErr *BootstrapError
	if !errors.As(err, &bootstrapErr) || bootstrapErr.Status != StatusUnsupportedFuture {
		t.Fatalf("Open error = %v, want unsupported future database", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read future database after blocked open: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("future database changed after blocked open")
	}
}

func TestOpenSnapshotsExistingDatabaseBeforeMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := legacy.Exec("CREATE TABLE legacy_marker (value TEXT NOT NULL)"); err != nil {
		t.Fatalf("write legacy database: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}
	if _, err := Open(path); err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}
	if _, err := os.Stat(PreMigrationSnapshotPath(path, 0)); err != nil {
		t.Fatalf("pre-migration snapshot missing: %v", err)
	}
}

func TestOpenRejectsCurrentVersionWithoutRequiredSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "malformed.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open malformed database: %v", err)
	}
	if _, err := database.Exec(fmt.Sprintf("PRAGMA user_version = %d", CurrentSchemaVersion)); err != nil {
		t.Fatalf("set schema version: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close malformed database: %v", err)
	}
	_, err = Open(path)
	var bootstrapErr *BootstrapError
	if !errors.As(err, &bootstrapErr) || bootstrapErr.Status != StatusIntegrityFailed {
		t.Fatalf("Open error = %v, want integrity failure", err)
	}
}

func TestFailedSchema3MigrationRollsBackCollidingObjectsAndCanBeRetried(t *testing.T) {
	cases := []struct {
		name      string
		collision string
		object    string
	}{
		{name: "table", collision: `CREATE TABLE instruments (sentinel TEXT NOT NULL);`, object: "instruments"},
		{name: "index", collision: `CREATE TABLE legacy_collision (id TEXT); CREATE INDEX idx_instruments_household ON legacy_collision(id);`, object: "idx_instruments_household"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "schema2-collision.db")
			seedSchema2MigrationFixture(t, path, testCase.collision)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			assertMigrationCollision(t, path, testCase.object)
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("failed migration changed the primary database")
			}
			assertSchema2MigrationState(t, path, testCase.object)

			// The immutable sibling snapshot is reused on a retry; it must not
			// mask the original migration error or cause a partial schema 3.
			assertMigrationCollision(t, path, testCase.object)
			assertSchema2MigrationState(t, path, testCase.object)
		})
	}
}

func seedSchema2MigrationFixture(t *testing.T, path, collision string) {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join("..", "..", "..", "testdata", "v0.1.2", "schema2-v0.1.1.sql")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec(string(fixture)); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec(collision); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertMigrationCollision(t *testing.T, path, object string) {
	t.Helper()
	_, err := Open(path)
	var bootstrapErr *BootstrapError
	if !errors.As(err, &bootstrapErr) || bootstrapErr.Status != StatusMigrationFailed {
		t.Fatalf("Open collision error = %v, want migration failure for %s", err, object)
	}
}

func assertSchema2MigrationState(t *testing.T, path, object string) {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var version int
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("schema version after failed migration = %d, want 2", version)
	}
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name = ?", object).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("collision object %q count = %d, want 1", object, count)
	}
	for _, table := range []string{"holdings", "account_cash_values", "instrument_quotes", "fx_quotes", "fx_preferences"} {
		if err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("partial migration left table %q", table)
		}
	}
}
