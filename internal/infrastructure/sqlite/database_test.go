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

	"github.com/waltwang/nestworth-go/internal/domain"
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
	var mediaTables int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'media_assets'").Scan(&mediaTables); err != nil {
		t.Fatalf("check media table: %v", err)
	}
	if mediaTables != 0 {
		t.Fatal("schema v8 unexpectedly contains media_assets")
	}
	for _, entity := range []struct{ table, column string }{
		{"members", "icon_key"}, {"institutions", "icon_key"}, {"account_groups", "icon_key"}, {"accounts", "icon_key"}, {"instruments", "icon_key"}, {"institutions", "institution_type"},
	} {
		var notNull int
		if err := database.SQL.QueryRow("SELECT \"notnull\" FROM pragma_table_info(?) WHERE name = ?", entity.table, entity.column).Scan(&notNull); err != nil {
			t.Fatalf("read %s.%s: %v", entity.table, entity.column, err)
		}
		if notNull != 1 {
			t.Errorf("%s.%s notnull = %d, want 1", entity.table, entity.column, notNull)
		}
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

func TestOpenEnforcesRestrictiveDatabaseMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mode.db")
	assertMode := func(file string, required bool) {
		t.Helper()
		info, statErr := os.Stat(file)
		if statErr != nil {
			if !required && errors.Is(statErr, os.ErrNotExist) {
				return
			}
			t.Fatal(statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s perm = %o, want 0600", filepath.Base(file), info.Mode().Perm())
		}
	}
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	assertMode(path, true)
	assertMode(path+"-wal", true)
	assertMode(path+"-shm", false)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + "-wal"); err == nil {
		if err := os.Chmod(path+"-wal", 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	assertMode(path, true)
	assertMode(path+"-wal", true)
	assertMode(path+"-shm", false)
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

func TestOpenRejectsSchema6FixtureWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema6.db")
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "schema6", "schema6-fixture.sql"))
	if err != nil {
		t.Fatal(err)
	}
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(string(script)); err != nil {
		_ = seed.Close()
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
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
	if bootstrapErr.Found != 6 || bootstrapErr.Supported != CurrentSchemaVersion {
		t.Fatalf("versions found=%d supported=%d, want 6 and %d", bootstrapErr.Found, bootstrapErr.Supported, CurrentSchemaVersion)
	}
	if bootstrapErr.Path != path {
		t.Fatalf("path = %q, want %q", bootstrapErr.Path, path)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("schema 6 database changed after rejected open")
	}
	message := openErr.Error()
	if !strings.Contains(message, "found version 6") || !strings.Contains(message, "supported version 9") || !strings.Contains(message, "new database") {
		t.Fatalf("legacy error = %v, want found/supported versions and new-database guidance", openErr)
	}
}

func TestOpenRejectsSchema7FixtureWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema7.db")
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "schema7", "schema7-fixture.sql"))
	if err != nil {
		t.Fatal(err)
	}
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(string(script)); err != nil {
		_ = seed.Close()
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
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
	if bootstrapErr.Found != 7 || bootstrapErr.Supported != CurrentSchemaVersion {
		t.Fatalf("versions found=%d supported=%d, want 7 and %d", bootstrapErr.Found, bootstrapErr.Supported, CurrentSchemaVersion)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("schema 7 database changed after rejected open")
	}
}

func TestOpenRejectsSchema8FixtureWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema8.db")
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "schema8", "schema8-fixture.sql"))
	if err != nil {
		t.Fatal(err)
	}
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(string(script)); err != nil {
		_ = seed.Close()
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
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
	if bootstrapErr.Found != 8 || bootstrapErr.Supported != CurrentSchemaVersion {
		t.Fatalf("versions found=%d supported=%d, want 8 and %d", bootstrapErr.Found, bootstrapErr.Supported, CurrentSchemaVersion)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("schema 8 database changed after rejected open")
	}
}

func TestOpenRewritesLegacyCashOnHandHoldingsCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repair.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := rewriteAccountsCheckFragment(ctx, first.SQL, cashOnHandBalanceOrHoldingsCheck, cashOnHandBalanceOnlyCheck); err != nil {
		t.Fatalf("install legacy check: %v", err)
	}
	if got := accountsCreateSQL(t, first.SQL); !schemaSQLContains(got, cashOnHandBalanceOnlyCheck) {
		t.Fatalf("legacy check missing from setup: %s", got)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open after legacy check: %v", err)
	}
	defer reopened.Close()
	got := accountsCreateSQL(t, reopened.SQL)
	if !schemaSQLContains(got, cashOnHandBalanceOrHoldingsCheck) {
		t.Fatalf("reopened accounts check = %s, want holdings allowed", got)
	}
	if schemaSQLContains(got, cashOnHandBalanceOnlyCheck) {
		t.Fatalf("reopened accounts check still has the legacy fragment: %s", got)
	}
}

func TestBootstrapErrorSafeErrorOmitsPathAndDriverText(t *testing.T) {
	cases := []struct {
		status BootstrapStatus
		want   domain.ErrorCode
	}{
		{StatusLegacyDatabase, domain.ErrDatabaseUpgradeRequired},
		{StatusUnsupportedFuture, domain.ErrDatabaseFromNewerVersion},
		{StatusIntegrityFailed, domain.ErrDatabaseIntegrityFailed},
		{StatusUnavailable, domain.ErrDatabaseUnavailable},
	}
	for _, testCase := range cases {
		err := (&BootstrapError{Status: testCase.status, Found: 7, Supported: 9, Path: "/secret/nestworth.db", Err: errors.New("sqlite: constraint failed")}).SafeError()
		if err.Code != testCase.want || err.Field != "database" {
			t.Fatalf("status %s SafeError = %+v, want %s", testCase.status, err, testCase.want)
		}
		if strings.Contains(err.Error(), "/secret") || strings.Contains(strings.ToLower(err.Error()), "sqlite") {
			t.Fatalf("SafeError leaked technical detail: %v", err)
		}
	}
}

func accountsCreateSQL(t *testing.T, database *sql.DB) string {
	t.Helper()
	var definition string
	if err := database.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'accounts'`).Scan(&definition); err != nil {
		t.Fatalf("read accounts sql: %v", err)
	}
	return definition
}
