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

// upgradeV23 is a single additive upgrade, not a migration chain. The verified,
// flushed recovery artifact is published read-only before the immediate transaction.
func upgradeV23(ctx context.Context, database *sql.DB, path string) error {
	generation, err := prepareV23Recovery(ctx, database, path, (*os.File).Sync, syncV23UpgradeDir)
	if err != nil {
		return err
	}
	return commitV23Upgrade(ctx, database, generation)
}

// Recovery copies are inspection artifacts, never writable live databases.
func isV23RecoveryCopy(path string) bool {
	return strings.EqualFold(filepath.Base(path), "original-v23.sqlite") && strings.HasPrefix(strings.ToLower(filepath.Base(filepath.Dir(path))), ".nestworth-v23-recovery-")
}

// Narrow IO seams allow failure tests without changing filesystem behavior globally.
func prepareV23Recovery(ctx context.Context, database *sql.DB, path string, syncFile func(*os.File) error, syncDir func(string) error) (int, error) {
	if err := verifySchemaVersion(ctx, database, 23); err != nil {
		return 0, err
	}
	var generation int
	if err := database.QueryRowContext(ctx, "PRAGMA data_version").Scan(&generation); err != nil {
		return 0, err
	}
	dir, err := os.MkdirTemp(filepath.Dir(path), ".nestworth-v23-recovery-")
	if err != nil {
		return 0, err
	}
	pending := filepath.Join(dir, ".snapshot-v23.partial")
	defer os.Remove(pending)
	original := &DB{SQL: database, Path: path}
	if err := original.SnapshotTo(ctx, pending); err != nil {
		return 0, fmt.Errorf("v23 recovery snapshot failed: %w", err)
	}
	verified, err := OpenReadOnlyForVerifyContext(ctx, pending)
	if err != nil {
		return 0, fmt.Errorf("v23 recovery verification failed: %w", err)
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
	recoveryPath := filepath.Join(dir, "original-v23.sqlite")
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

func syncV23UpgradeDir(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	flushErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(flushErr, closeErr)
}

func commitV23Upgrade(ctx context.Context, database *sql.DB, generation int) error {
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
		return errors.New("database changed while preparing v23 recovery snapshot; retry upgrade")
	}
	if err := verifySchemaVersion(ctx, tx, 23); err != nil {
		return err
	}
	if err := applyV23Upgrade(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// The caller owns the transaction; no historical financial rows are replayed.
// Only a format-version fence: no table rewrite or historical backfill.
func applyV23Upgrade(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version=24"); err != nil {
		return err
	}
	return verifySchemaVersion(ctx, tx, 24)
}
