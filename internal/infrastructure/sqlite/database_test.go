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

	_ "modernc.org/sqlite"
)

func TestOpenCreatesAndVerifiesCurrentDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nestworth.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.Close()
	if database.Status != StatusReady {
		t.Fatalf("database status = %q, want ready", database.Status)
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
	var projectionDefault string
	if err := database.SQL.QueryRow("SELECT dflt_value FROM pragma_table_info('account_values') WHERE name = 'projection_kind'").Scan(&projectionDefault); err != nil {
		t.Fatalf("read projection default: %v", err)
	}
	if projectionDefault != "'baseline'" {
		t.Fatalf("projection default = %q, want 'baseline'", projectionDefault)
	}
	if err := database.Verify(context.Background()); err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
}

func TestOpenReopensCurrentDatabaseWithoutMigrationStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.Status != StatusReady {
		t.Fatalf("reopen status = %q, want ready", reopened.Status)
	}
}

func TestOpenRejectsLegacyDatabaseWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec("PRAGMA user_version = 5; CREATE TABLE marker (value TEXT NOT NULL); INSERT INTO marker(value) VALUES('preserve');"); err != nil {
		_ = legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, openErr := Open(path)
	var bootstrapErr *BootstrapError
	if !errors.As(openErr, &bootstrapErr) || bootstrapErr.Status != StatusLegacyDatabase {
		t.Fatalf("Open error = %v, want legacy database rejection", openErr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("legacy database changed after rejected open")
	}
	if !strings.Contains(openErr.Error(), "new database") {
		t.Fatalf("legacy error = %v, want recovery guidance", openErr)
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
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Open(path)
	var bootstrapErr *BootstrapError
	if !errors.As(err, &bootstrapErr) || bootstrapErr.Status != StatusUnsupportedFuture {
		t.Fatalf("Open error = %v, want unsupported future database", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("future database changed after blocked open")
	}
}

func TestOpenRejectsCurrentVersionWithoutRequiredSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "malformed.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(fmt.Sprintf("PRAGMA user_version = %d", CurrentSchemaVersion)); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Open(path)
	var bootstrapErr *BootstrapError
	if !errors.As(err, &bootstrapErr) || bootstrapErr.Status != StatusIntegrityFailed {
		t.Fatalf("Open error = %v, want integrity failure", err)
	}
}
