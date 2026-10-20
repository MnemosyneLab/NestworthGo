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

// upgradeV22 is a single additive upgrade, not a migration chain. The verified,
// flushed recovery artifact is published read-only before the immediate transaction.
func upgradeV22(ctx context.Context, database *sql.DB, path string) error {
	generation, err := prepareV22Recovery(ctx, database, path, (*os.File).Sync, syncV22UpgradeDir)
	if err != nil {
		return err
	}
	return commitV22Upgrade(ctx, database, generation)
}

// Recovery copies are inspection artifacts, never writable live databases.
func isV22RecoveryCopy(path string) bool {
	return strings.EqualFold(filepath.Base(path), "original-v22.sqlite") && strings.HasPrefix(strings.ToLower(filepath.Base(filepath.Dir(path))), ".nestworth-v22-recovery-")
}

// Narrow IO seams allow failure tests without changing filesystem behavior globally.
func prepareV22Recovery(ctx context.Context, database *sql.DB, path string, syncFile func(*os.File) error, syncDir func(string) error) (int, error) {
	if err := verifySchemaVersion(ctx, database, 22); err != nil {
		return 0, err
	}
	var generation int
	if err := database.QueryRowContext(ctx, "PRAGMA data_version").Scan(&generation); err != nil {
		return 0, err
	}
	dir, err := os.MkdirTemp(filepath.Dir(path), ".nestworth-v22-recovery-")
	if err != nil {
		return 0, err
	}
	pending := filepath.Join(dir, ".snapshot-v22.partial")
	defer os.Remove(pending)
	original := &DB{SQL: database, Path: path}
	if err := original.SnapshotTo(ctx, pending); err != nil {
		return 0, fmt.Errorf("v22 recovery snapshot failed: %w", err)
	}
	verified, err := OpenReadOnlyForVerifyContext(ctx, pending)
	if err != nil {
		return 0, fmt.Errorf("v22 recovery verification failed: %w", err)
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
	recoveryPath := filepath.Join(dir, "original-v22.sqlite")
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

func syncV22UpgradeDir(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	flushErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(flushErr, closeErr)
}

func commitV22Upgrade(ctx context.Context, database *sql.DB, generation int) error {
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
		return errors.New("database changed while preparing v22 recovery snapshot; retry upgrade")
	}
	if err := verifySchemaVersion(ctx, tx, 22); err != nil {
		return err
	}
	if err := applyV22Upgrade(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// The caller owns the transaction; no historical financial rows are replayed.
func applyV22Upgrade(ctx context.Context, tx *sql.Tx) error {
	if err := installReviewFactSchema(ctx, tx); err != nil {
		return err
	}
	return verifySchemaVersion(ctx, tx, 23)
}
