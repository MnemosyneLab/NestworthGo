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

//go:embed schema.sql schema3.sql schema4.sql schema5.sql
var schemaFS embed.FS

const CurrentSchemaVersion = 5

type BootstrapStatus string

const (
	StatusReady             BootstrapStatus = "ready"
	StatusMigrated          BootstrapStatus = "migrated"
	StatusUnsupportedFuture BootstrapStatus = "unsupported_future_database"
	StatusMigrationFailed   BootstrapStatus = "migration_failed"
	StatusIntegrityFailed   BootstrapStatus = "integrity_failed"
	StatusUnavailable       BootstrapStatus = "unavailable"
)

type BootstrapError struct {
	Status    BootstrapStatus
	Found     int
	Supported int
	Err       error
}

func (e *BootstrapError) Error() string {
	if e == nil {
		return ""
	}
	if e.Status == StatusUnsupportedFuture {
		return fmt.Sprintf("database schema %d is newer than supported schema %d", e.Found, e.Supported)
	}
	if e.Err == nil {
		return string(e.Status)
	}
	return fmt.Sprintf("%s: %v", e.Status, e.Err)
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
		return nil, &BootstrapError{Status: status, Found: found, Supported: CurrentSchemaVersion, Err: cause}
	}
	found, err := readVersion(database)
	if err != nil {
		return closeOnError(StatusUnavailable, 0, err)
	}
	if found > CurrentSchemaVersion {
		return closeOnError(StatusUnsupportedFuture, found, nil)
	}
	migrated := false
	if found < CurrentSchemaVersion && existed && (found > 0 || fileSize(path) > 0) {
		if err := createPreMigrationSnapshot(database, path, found); err != nil {
			return closeOnError(StatusMigrationFailed, found, err)
		}
	}
	tx, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		return closeOnError(StatusUnavailable, found, err)
	}
	var lockedVersion int
	if err := tx.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&lockedVersion); err != nil {
		_ = tx.Rollback()
		return closeOnError(StatusUnavailable, found, err)
	}
	if lockedVersion > CurrentSchemaVersion {
		_ = tx.Rollback()
		return closeOnError(StatusUnsupportedFuture, lockedVersion, nil)
	}
	if lockedVersion == 0 {
		schema, readErr := schemaFS.ReadFile("schema.sql")
		if readErr != nil {
			_ = tx.Rollback()
			return closeOnError(StatusMigrationFailed, lockedVersion, readErr)
		}
		if _, execErr := tx.ExecContext(context.Background(), string(schema)); execErr != nil {
			_ = tx.Rollback()
			return closeOnError(StatusMigrationFailed, lockedVersion, execErr)
		}
		lockedVersion = 1
		migrated = true
	}
	for lockedVersion < CurrentSchemaVersion {
		switch lockedVersion {
		case 1:
			if _, err := tx.ExecContext(context.Background(), "ALTER TABLE institutions ADD COLUMN icon_key TEXT"); err != nil {
				_ = tx.Rollback()
				return closeOnError(StatusMigrationFailed, lockedVersion, err)
			}
			if _, err := tx.ExecContext(context.Background(), "ALTER TABLE accounts ADD COLUMN icon_key TEXT"); err != nil {
				_ = tx.Rollback()
				return closeOnError(StatusMigrationFailed, lockedVersion, err)
			}
			lockedVersion = 2
			migrated = true
		case 2:
			schema, readErr := schemaFS.ReadFile("schema3.sql")
			if readErr != nil {
				_ = tx.Rollback()
				return closeOnError(StatusMigrationFailed, lockedVersion, readErr)
			}
			if _, execErr := tx.ExecContext(context.Background(), string(schema)); execErr != nil {
				_ = tx.Rollback()
				return closeOnError(StatusMigrationFailed, lockedVersion, execErr)
			}
			lockedVersion = 3
			migrated = true
		case 3:
			schema, readErr := schemaFS.ReadFile("schema4.sql")
			if readErr != nil {
				_ = tx.Rollback()
				return closeOnError(StatusMigrationFailed, lockedVersion, readErr)
			}
			if _, execErr := tx.ExecContext(context.Background(), string(schema)); execErr != nil {
				_ = tx.Rollback()
				return closeOnError(StatusMigrationFailed, lockedVersion, execErr)
			}
			lockedVersion = 4
			migrated = true
		case 4:
			schema, readErr := schemaFS.ReadFile("schema5.sql")
			if readErr != nil {
				_ = tx.Rollback()
				return closeOnError(StatusMigrationFailed, lockedVersion, readErr)
			}
			if _, execErr := tx.ExecContext(context.Background(), string(schema)); execErr != nil {
				_ = tx.Rollback()
				return closeOnError(StatusMigrationFailed, lockedVersion, execErr)
			}
			lockedVersion = 5
			migrated = true
		default:
			_ = tx.Rollback()
			return closeOnError(StatusMigrationFailed, lockedVersion, fmt.Errorf("no migration from schema %d", lockedVersion))
		}
	}
	if _, err := tx.ExecContext(context.Background(), fmt.Sprintf("PRAGMA user_version = %d", lockedVersion)); err != nil {
		_ = tx.Rollback()
		return closeOnError(StatusMigrationFailed, lockedVersion, err)
	}
	if err := verifySchema(context.Background(), tx); err != nil {
		_ = tx.Rollback()
		return closeOnError(StatusIntegrityFailed, lockedVersion, err)
	}
	if err := tx.Commit(); err != nil {
		return closeOnError(StatusMigrationFailed, lockedVersion, err)
	}
	if path != ":memory:" {
		if _, err := database.ExecContext(context.Background(), "PRAGMA journal_mode = WAL"); err != nil {
			return closeOnError(StatusUnavailable, lockedVersion, err)
		}
	}
	result := &DB{SQL: database, Path: path, Status: StatusReady}
	if migrated {
		result.Status = StatusMigrated
	}
	return result, nil
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
		return err
	}
	return tx.Commit()
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

func PreMigrationSnapshotPath(path string, found int) string {
	return fmt.Sprintf("%s.pre-migrate-%d", path, found)
}

func createPreMigrationSnapshot(database *sql.DB, path string, found int) error {
	snapshot := PreMigrationSnapshotPath(path, found)
	if info, err := os.Stat(snapshot); err == nil {
		if info.IsDir() {
			return errors.New("pre-migration snapshot path is a directory")
		}
		// A previous failed attempt may already have created the immutable
		// recovery sibling. Never overwrite it; reuse it for a retry.
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	statement := fmt.Sprintf("VACUUM INTO '%s'", strings.ReplaceAll(snapshot, "'", "''"))
	_, err := database.Exec(statement)
	return err
}
