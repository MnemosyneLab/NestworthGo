package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// upgradeV15 is intentionally a single additive upgrade, not a migration chain.
// A verified, consistent v15 snapshot is retained before any schema mutation.
// Both the new column and version change roll back together on any failure.
func upgradeV15(ctx context.Context, database *sql.DB, path string) error {
	if err := verifySchemaVersion(ctx, database, 15); err != nil {
		return err
	}
	var generation int
	if err := database.QueryRowContext(ctx, "PRAGMA data_version").Scan(&generation); err != nil {
		return err
	}
	recoveryDir, err := os.MkdirTemp(filepath.Dir(path), ".nestworth-v15-recovery-")
	if err != nil {
		return err
	}
	recoveryPath := filepath.Join(recoveryDir, "original-v15.sqlite")
	original := &DB{SQL: database, Path: path}
	if err := original.SnapshotTo(ctx, recoveryPath); err != nil {
		return fmt.Errorf("v15 recovery snapshot failed: %w", err)
	}
	recovery, err := OpenReadOnlyForVerifyContext(ctx, recoveryPath)
	if err != nil {
		return fmt.Errorf("v15 recovery verification failed: %w", err)
	}
	if err := recovery.Close(); err != nil {
		return err
	}
	file, err := os.OpenFile(recoveryPath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return err
	}
	// Persist both directory entries before committing the live schema.
	for _, dir := range []string{recoveryDir, filepath.Dir(path)} {
		handle, err := os.Open(dir)
		if err != nil {
			return err
		}
		syncErr, closeErr := handle.Sync(), handle.Close()
		if err := errors.Join(syncErr, closeErr); err != nil {
			return err
		}
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentGeneration int
	if err := tx.QueryRowContext(ctx, "PRAGMA data_version").Scan(&currentGeneration); err != nil {
		return err
	}
	if currentGeneration != generation {
		return errors.New("database changed while preparing v15 recovery snapshot; retry upgrade")
	}
	if err := verifySchemaVersion(ctx, tx, 15); err != nil {
		return err
	}
	if err := applyV15Upgrade(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// The caller owns the transaction and rolls back failed verification.
func applyV15Upgrade(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "ALTER TABLE instruments ADD COLUMN short_name TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version = 16"); err != nil {
		return err
	}
	if err := verifySchema(ctx, tx); err != nil {
		return err
	}
	return nil
}
