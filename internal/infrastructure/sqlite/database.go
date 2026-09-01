// Package sqlite owns the local business database. UI and application code use
// the repository facade instead of opening SQLite directly.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

const CurrentSchemaVersion = 9

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

// DB is a single local SQLite connection pool. It is intentionally capped at
// one writer connection to make transaction boundaries and snapshots explicit.
type DB struct {
	SQL    *sql.DB
	Path   string
	Status BootstrapStatus
}

func Open(path string) (*DB, error) {
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
	dsn := path + "?_txlock=immediate&_pragma=busy_timeout%3d5000&_pragma=foreign_keys%3d1"
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
	if existed && fileSize(path) > 0 && found < CurrentSchemaVersion {
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
		if err := verifySchema(context.Background(), tx); err != nil {
			_ = tx.Rollback()
			return closeOnError(StatusIntegrityFailed, CurrentSchemaVersion, err)
		}
		if err := tx.Commit(); err != nil {
			return closeOnError(StatusUnavailable, CurrentSchemaVersion, err)
		}
	} else {
		if err := repairV9CashOnHandHoldingsCheck(context.Background(), database); err != nil {
			return closeOnError(StatusUnavailable, found, err)
		}
		if err := verifySchema(context.Background(), database); err != nil {
			return closeOnError(StatusIntegrityFailed, found, err)
		}
	}
	if path != ":memory:" {
		if _, err := database.ExecContext(context.Background(), "PRAGMA journal_mode = WAL"); err != nil {
			return closeOnError(StatusUnavailable, CurrentSchemaVersion, err)
		}
	}
	return &DB{SQL: database, Path: path, Status: StatusReady}, nil
}

func (db *DB) Close() error {
	if db == nil || db.SQL == nil {
		return nil
	}
	return db.SQL.Close()
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
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return mapSQLiteError(err)
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

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
