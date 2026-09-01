package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// OpenReadOnlyForVerify opens an existing SQLite file without creating a
// schema, running v9 repair, enabling WAL, or creating parent directories.
// It is the only supported way to inspect a backup snapshot or a restored
// candidate before treating it as the live database.
func OpenReadOnlyForVerify(path string) (*DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "database path is empty"}
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "database file is missing"}
		}
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "database could not be opened"}
	}
	if info.IsDir() {
		return nil, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "database path is a directory"}
	}
	dsn := path + "?mode=ro&_pragma=query_only%3d1&_pragma=foreign_keys%3d1&_pragma=busy_timeout%3d5000"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "database could not be opened"}
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	database.SetConnMaxLifetime(0)
	found, err := readVersion(database)
	if err != nil {
		_ = database.Close()
		return nil, &domain.Error{Code: domain.ErrBackupIntegrityFailed, Message: "database could not be read"}
	}
	if found != CurrentSchemaVersion {
		_ = database.Close()
		return nil, &domain.Error{Code: domain.ErrBackupSchemaUnsupported, Message: fmt.Sprintf("database schema version is %d, want %d", found, CurrentSchemaVersion)}
	}
	db := &DB{SQL: database, Path: path, Status: StatusReady}
	if err := verifySchema(context.Background(), database); err != nil {
		_ = database.Close()
		return nil, &domain.Error{Code: domain.ErrBackupIntegrityFailed, Message: "database failed integrity verification"}
	}
	return db, nil
}

// CheckpointWAL runs a truncating checkpoint on an open writable database.
// It must be called before Close when a live session exists.
func (db *DB) CheckpointWAL(ctx context.Context) error {
	if db == nil || db.SQL == nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "database is not open"}
	}
	_, err := db.SQL.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	if err != nil {
		return &domain.Error{Code: domain.ErrBackupRestoreSwapFailed, Message: "database checkpoint failed"}
	}
	return nil
}

// SnapshotTo writes a consistent copy of the open database to dest using
// VACUUM INTO. dest must not already exist. The copy does not include WAL or
// SHM sidecar files.
func (db *DB) SnapshotTo(ctx context.Context, dest string) error {
	if db == nil || db.SQL == nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "database is not open"}
	}
	if strings.TrimSpace(dest) == "" {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "snapshot destination is empty"}
	}
	if _, err := os.Stat(dest); err == nil {
		return &domain.Error{Code: domain.ErrConflict, Message: "snapshot destination already exists"}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "snapshot destination could not be checked"}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "snapshot directory could not be created"}
	}
	quoted := "'" + strings.ReplaceAll(dest, "'", "''") + "'"
	if _, err := db.SQL.ExecContext(ctx, "VACUUM INTO "+quoted); err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "database snapshot failed"}
	}
	if err := os.Chmod(dest, 0o600); err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "snapshot permissions could not be set"}
	}
	return nil
}

// EntityCounts is a non-authoritative preview aid stored in backup manifests.
type EntityCounts struct {
	Households  int `json:"households"`
	Accounts    int `json:"accounts"`
	Holdings    int `json:"holdings"`
	Activities  int `json:"activities"`
	Instruments int `json:"instruments"`
	Members     int `json:"members"`
}

func (db *DB) EntityCounts(ctx context.Context) (EntityCounts, error) {
	if db == nil || db.SQL == nil {
		return EntityCounts{}, &domain.Error{Code: domain.ErrUnavailable, Message: "database is not open"}
	}
	count := func(table string) (int, error) {
		var n int
		if err := db.SQL.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
			return 0, err
		}
		return n, nil
	}
	var result EntityCounts
	var err error
	if result.Households, err = count("households"); err != nil {
		return EntityCounts{}, &domain.Error{Code: domain.ErrBackupIntegrityFailed, Message: "database counts could not be read"}
	}
	if result.Accounts, err = count("accounts"); err != nil {
		return EntityCounts{}, &domain.Error{Code: domain.ErrBackupIntegrityFailed, Message: "database counts could not be read"}
	}
	if result.Holdings, err = count("holdings"); err != nil {
		return EntityCounts{}, &domain.Error{Code: domain.ErrBackupIntegrityFailed, Message: "database counts could not be read"}
	}
	if result.Activities, err = count("activities"); err != nil {
		return EntityCounts{}, &domain.Error{Code: domain.ErrBackupIntegrityFailed, Message: "database counts could not be read"}
	}
	if result.Instruments, err = count("instruments"); err != nil {
		return EntityCounts{}, &domain.Error{Code: domain.ErrBackupIntegrityFailed, Message: "database counts could not be read"}
	}
	if result.Members, err = count("members"); err != nil {
		return EntityCounts{}, &domain.Error{Code: domain.ErrBackupIntegrityFailed, Message: "database counts could not be read"}
	}
	return result, nil
}

// HouseholdSummary is the restore-preview identity of a database.
type HouseholdSummary struct {
	Name         string
	BaseCurrency string
}

func (db *DB) HouseholdSummary(ctx context.Context) (HouseholdSummary, error) {
	if db == nil || db.SQL == nil {
		return HouseholdSummary{}, &domain.Error{Code: domain.ErrUnavailable, Message: "database is not open"}
	}
	var name, currency string
	err := db.SQL.QueryRowContext(ctx, `SELECT name, base_currency FROM households WHERE singleton_key = 1`).Scan(&name, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return HouseholdSummary{}, nil
	}
	if err != nil {
		return HouseholdSummary{}, &domain.Error{Code: domain.ErrBackupIntegrityFailed, Message: "household could not be read"}
	}
	return HouseholdSummary{Name: name, BaseCurrency: currency}, nil
}
