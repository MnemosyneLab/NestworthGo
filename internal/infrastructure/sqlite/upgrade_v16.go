package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// upgradeV16 is a single additive upgrade, not a migration chain. The verified,
// flushed recovery artifact is published read-only before the immediate transaction.
func upgradeV16(ctx context.Context, database *sql.DB, path string) error {
	generation, err := prepareV16Recovery(ctx, database, path, (*os.File).Sync, syncV16UpgradeDir)
	if err != nil {
		return err
	}
	return commitV16Upgrade(ctx, database, generation)
}

// Recovery copies are inspection artifacts, never writable live databases.
func isV16RecoveryCopy(path string) bool {
	return strings.EqualFold(filepath.Base(path), "original-v16.sqlite") && strings.HasPrefix(strings.ToLower(filepath.Base(filepath.Dir(path))), ".nestworth-v16-recovery-")
}

// Narrow IO seams allow failure tests without changing filesystem behavior globally.
func prepareV16Recovery(ctx context.Context, database *sql.DB, path string, syncFile func(*os.File) error, syncDir func(string) error) (int, error) {
	if err := verifySchemaVersion(ctx, database, 16); err != nil {
		return 0, err
	}
	var generation int
	if err := database.QueryRowContext(ctx, "PRAGMA data_version").Scan(&generation); err != nil {
		return 0, err
	}
	dir, err := os.MkdirTemp(filepath.Dir(path), ".nestworth-v16-recovery-")
	if err != nil {
		return 0, err
	}
	pending := filepath.Join(dir, ".snapshot-v16.partial")
	defer os.Remove(pending)
	original := &DB{SQL: database, Path: path}
	if err := original.SnapshotTo(ctx, pending); err != nil {
		return 0, fmt.Errorf("v16 recovery snapshot failed: %w", err)
	}
	verified, err := OpenReadOnlyForVerifyContext(ctx, pending)
	if err != nil {
		return 0, fmt.Errorf("v16 recovery verification failed: %w", err)
	}
	if err := verified.Close(); err != nil {
		return 0, err
	}
	file, err := os.OpenFile(pending, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	// Flush read-only permissions together with the complete, verified file data.
	freezeErr := file.Chmod(0400)
	var flushErr error
	if freezeErr == nil {
		flushErr = syncFile(file)
	}
	closeErr := file.Close()
	if err := errors.Join(freezeErr, flushErr, closeErr); err != nil {
		return 0, err
	}
	recoveryPath := filepath.Join(dir, "original-v16.sqlite")
	if err := os.Rename(pending, recoveryPath); err != nil {
		return 0, err
	}
	for _, directory := range []string{dir, filepath.Dir(path)} {
		if err := syncDir(directory); err != nil {
			return 0, err
		}
	}
	return generation, nil
}

func syncV16UpgradeDir(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	flushErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(flushErr, closeErr)
}

func commitV16Upgrade(ctx context.Context, database *sql.DB, generation int) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int
	if err := tx.QueryRowContext(ctx, "PRAGMA data_version").Scan(&current); err != nil {
		return err
	}
	if current != generation {
		return errors.New("database changed while preparing v16 recovery snapshot; retry upgrade")
	}
	if err := verifySchemaVersion(ctx, tx, 16); err != nil {
		return err
	}
	if err := applyV16Upgrade(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// The caller owns the transaction and rolls back failed verification.
func applyV16Upgrade(ctx context.Context, tx *sql.Tx) error {
	schema, err := schemaFS.ReadFile("reconciliation_schema.sql")
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(schema)); err != nil {
		return err
	}
	return verifySchema(ctx, tx)
}
