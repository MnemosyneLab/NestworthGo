// Package sqlite owns the local business database. UI and application code use
// the repository facade instead of opening SQLite directly.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql reconciliation_schema.sql reconciliation_schema_v17.sql upgrade_v17.sql review_schema_v19.sql source_changes_schema_v20.sql review_temporal_schema_v21.sql
var schemaFS embed.FS

const CurrentSchemaVersion = 21

type BootstrapStatus string

const (
	StatusReady             BootstrapStatus = "ready"
	StatusLegacyDatabase    BootstrapStatus = "legacy_database"
	StatusUnsupportedFuture BootstrapStatus = "unsupported_future_database"
	StatusIntegrityFailed   BootstrapStatus = "integrity_failed"
	StatusUnavailable       BootstrapStatus = "unavailable"
)

type BootstrapError struct {
	Status    BootstrapStatus
	Found     int
	Supported int
	Path      string
	Err       error
}

func (e *BootstrapError) Error() string {
	if e == nil {
		return ""
	}
	path := e.Path
	if path == "" {
		path = "(unknown path)"
	}
	if e.Status == StatusUnsupportedFuture {
		return fmt.Sprintf("incompatible-schema: found version %d, supported version %d, path %s; please create a new database", e.Found, e.Supported, path)
	}
	if e.Status == StatusLegacyDatabase {
		return fmt.Sprintf("incompatible-schema: found version %d, supported version %d, path %s; please create a new database", e.Found, e.Supported, path)
	}
	if e.Err == nil {
		return fmt.Sprintf("%s: found version %d, supported version %d, path %s; please create a new database", e.Status, e.Found, e.Supported, path)
	}
	return fmt.Sprintf("%s: found version %d, supported version %d, path %s: %v; please create a new database", e.Status, e.Found, e.Supported, path, e.Err)
}

func (e *BootstrapError) Unwrap() error { return e.Err }

// SafeError maps a bootstrap failure to a user-facing domain error without
// filesystem paths, SQL, or driver text.
func (e *BootstrapError) SafeError() *domain.Error {
	if e == nil {
		return &domain.Error{Code: domain.ErrDatabaseUnavailable, Field: "database", Message: "the local database could not be opened"}
	}
	switch e.Status {
	case StatusLegacyDatabase:
		return &domain.Error{Code: domain.ErrDatabaseUpgradeRequired, Field: "database", Message: "this database was created by an older Nestworth version"}
	case StatusUnsupportedFuture:
		return &domain.Error{Code: domain.ErrDatabaseFromNewerVersion, Field: "database", Message: "this database was created by a newer Nestworth version"}
	case StatusIntegrityFailed:
		return &domain.Error{Code: domain.ErrDatabaseIntegrityFailed, Field: "database", Message: "the local database failed an integrity check"}
	default:
		return &domain.Error{Code: domain.ErrDatabaseUnavailable, Field: "database", Message: "the local database could not be opened"}
	}
}

// DB is a single local SQLite connection pool. It is intentionally capped at
// one writer connection to make transaction boundaries and snapshots explicit.
type DB struct {
	SQL         *sql.DB
	Path        string
	Status      BootstrapStatus
	reviewClock reviewTemporalClock
}

func Open(path string) (*DB, error) {
	if isV15RecoveryCopy(path) || isV16RecoveryCopy(path) || isV17RecoveryCopy(path) || isV18RecoveryCopy(path) || isV19RecoveryCopy(path) || isV20RecoveryCopy(path) {
		return nil, &BootstrapError{Status: StatusUnavailable, Supported: CurrentSchemaVersion, Path: path, Err: errors.New("recovery copies are read-only; copy to a new live path before restoring")}
	}
	if strings.TrimSpace(path) == "" {
		return nil, &BootstrapError{Status: StatusUnavailable, Err: errors.New("database path is empty")}
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, &BootstrapError{Status: StatusUnavailable, Err: err}
		}
	}
	existed, err := databaseExists(path)
	if err != nil {
		return nil, &BootstrapError{Status: StatusUnavailable, Supported: CurrentSchemaVersion, Err: err}
	}
	// Inspect existing files before opening a writable connection. Every existing
	// database, including version zero, must use the current schema or verified v15.
	if existed {
		probeDSN, dsnErr := sqliteDSN(path, url.Values{
			"mode":    {"ro"},
			"_pragma": {"query_only=1", "foreign_keys=1", "recursive_triggers=1"},
		})
		if dsnErr != nil {
			return nil, &BootstrapError{Status: StatusUnavailable, Supported: CurrentSchemaVersion, Path: path, Err: dsnErr}
		}
		probe, err := sql.Open("sqlite", probeDSN)
		if err != nil {
			return nil, &BootstrapError{Status: StatusUnavailable, Supported: CurrentSchemaVersion, Path: path, Err: err}
		}
		found, readErr := readVersion(probe)
		if readErr == nil && (found == 15 || found == 16 || found == 17 || found == 18 || found == 19 || found == 20) {
			readErr = verifySchemaVersion(context.Background(), probe, found)
			if readErr != nil {
				probe.Close()
				return nil, &BootstrapError{Status: StatusIntegrityFailed, Found: found, Supported: CurrentSchemaVersion, Path: path, Err: readErr}
			}
		}
		closeErr := probe.Close()
		if readErr != nil || closeErr != nil {
			return nil, &BootstrapError{Status: StatusUnavailable, Supported: CurrentSchemaVersion, Path: path, Err: errors.Join(readErr, closeErr)}
		}
		if found != CurrentSchemaVersion && found != 15 && found != 16 && found != 17 && found != 18 && found != 19 && found != 20 {
			status := StatusLegacyDatabase
			if found > CurrentSchemaVersion {
				status = StatusUnsupportedFuture
			}
			return nil, &BootstrapError{Status: status, Found: found, Supported: CurrentSchemaVersion, Path: path}
		}
	}
	dsn, err := sqliteDSN(path, url.Values{
		"_txlock": {"immediate"},
		"_pragma": {"busy_timeout=5000", "foreign_keys=1", "recursive_triggers=1"},
	})
	if err != nil {
		return nil, &BootstrapError{Status: StatusUnavailable, Supported: CurrentSchemaVersion, Path: path, Err: err}
	}
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, &BootstrapError{Status: StatusUnavailable, Err: err}
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	database.SetConnMaxLifetime(0)
	closeOnError := func(status BootstrapStatus, found int, cause error) (*DB, error) {
		_ = database.Close()
		return nil, &BootstrapError{Status: status, Found: found, Supported: CurrentSchemaVersion, Path: path, Err: cause}
	}
	found, err := readVersion(database)
	if err != nil {
		return closeOnError(StatusUnavailable, 0, err)
	}
	if found > CurrentSchemaVersion {
		return closeOnError(StatusUnsupportedFuture, found, nil)
	}
	if found == 15 && existed {
		if err := upgradeV15(context.Background(), database, path); err != nil {
			return closeOnError(StatusUnavailable, found, err)
		}
		found = 16
	}
	if found == 16 && existed {
		if err := upgradeV16(context.Background(), database, path); err != nil {
			return closeOnError(StatusUnavailable, found, err)
		}
		found = 17
	}
	if found == 17 && existed {
		if err := upgradeV17(context.Background(), database, path); err != nil {
			return closeOnError(StatusUnavailable, found, err)
		}
		found = 18
	}
	if found == 18 && existed {
		if err := upgradeV18(context.Background(), database, path); err != nil {
			return closeOnError(StatusUnavailable, found, err)
		}
		found = 19
	}
	if found == 19 && existed {
		if err := upgradeV19(context.Background(), database, path); err != nil {
			return closeOnError(StatusUnavailable, found, err)
		}
		found = 20
	}
	if found == 20 && existed {
		if err := upgradeV20(context.Background(), database, path); err != nil {
			return closeOnError(StatusUnavailable, found, err)
		}
		found = 21
	}
	if found < CurrentSchemaVersion && existed {
		return closeOnError(StatusLegacyDatabase, found, nil)
	}
	if found == 0 {
		tx, err := database.BeginTx(context.Background(), nil)
		if err != nil {
			return closeOnError(StatusUnavailable, found, err)
		}
		schema, readErr := schemaFS.ReadFile("schema.sql")
		if readErr != nil {
			_ = tx.Rollback()
			return closeOnError(StatusUnavailable, found, readErr)
		}
		if _, execErr := tx.ExecContext(context.Background(), string(schema)); execErr != nil {
			_ = tx.Rollback()
			return closeOnError(StatusUnavailable, found, execErr)
		}
		reviewSchema, readErr := schemaFS.ReadFile("review_schema_v19.sql")
		if readErr != nil {
			_ = tx.Rollback()
			return closeOnError(StatusUnavailable, found, readErr)
		}
		if _, execErr := tx.ExecContext(context.Background(), string(reviewSchema)); execErr != nil {
			_ = tx.Rollback()
			return closeOnError(StatusUnavailable, found, execErr)
		}
		if err := installFinancialSourceTracking(context.Background(), tx); err != nil {
			_ = tx.Rollback()
			return closeOnError(StatusUnavailable, CurrentSchemaVersion, err)
		}
		if err := installReviewTemporalSchema(context.Background(), tx); err != nil {
			_ = tx.Rollback()
			return closeOnError(StatusUnavailable, CurrentSchemaVersion, err)
		}
		if err := verifySchema(context.Background(), tx); err != nil {
			_ = tx.Rollback()
			return closeOnError(StatusIntegrityFailed, CurrentSchemaVersion, err)
		}
		if err := tx.Commit(); err != nil {
			return closeOnError(StatusUnavailable, CurrentSchemaVersion, err)
		}
	} else {
		if err := verifySchema(context.Background(), database); err != nil {
			return closeOnError(StatusIntegrityFailed, found, err)
		}
	}
	if err := ensureCurrentHistoricalResolverPolicy(context.Background(), database); err != nil {
		return closeOnError(StatusUnavailable, CurrentSchemaVersion, err)
	}
	if path != ":memory:" {
		if _, err := database.ExecContext(context.Background(), "PRAGMA journal_mode = WAL"); err != nil {
			return closeOnError(StatusUnavailable, CurrentSchemaVersion, err)
		}
		tx, err := database.BeginTx(context.Background(), nil)
		if err != nil {
			return closeOnError(StatusUnavailable, CurrentSchemaVersion, err)
		}
		if err := tx.Commit(); err != nil {
			return closeOnError(StatusUnavailable, CurrentSchemaVersion, err)
		}
		if err := restrictLiveDatabaseFiles(path); err != nil {
			return closeOnError(StatusUnavailable, CurrentSchemaVersion, err)
		}
	}
	return &DB{SQL: database, Path: path, Status: StatusReady}, nil
}

func (db *DB) Close() error {
	if db == nil || db.SQL == nil {
		return nil
	}
	err := db.SQL.Close()
	db.SQL = nil
	return err
}

func (db *DB) Verify(ctx context.Context) error {
	if db == nil || db.SQL == nil {
		return errors.New("database is not open")
	}
	return verifySchema(ctx, db.SQL)
}

func (db *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if db == nil || db.SQL == nil {
		return errors.New("database is not open")
	}
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := validateReconciliationLinkTx(ctx, tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	linkCount, err := reconciliationLinkCountTx(ctx, tx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return mapSQLiteError(err)
	}
	if err := finishReconciliationLinkTx(ctx, tx, linkCount); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return mapSQLiteError(err)
	}
	return nil
}

func databaseExists(path string) (bool, error) {
	if path == ":memory:" {
		return false, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if info.IsDir() {
		return true, errors.New("database path is a directory")
	}
	return true, nil
}

func readVersion(database *sql.DB) (int, error) {
	var version int
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

// restrictLiveDatabaseFiles enforces 0600 on the live database and its
// app-owned WAL/SHM sidecars after Open has confirmed the path is the
// Nestworth database. Missing sidecars are ignored; the parent directory is
// already created 0700.
func restrictLiveDatabaseFiles(path string) error {
	if path == "" || path == ":memory:" {
		return nil
	}
	for _, file := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(file, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
