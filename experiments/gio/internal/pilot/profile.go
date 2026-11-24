package pilot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Prepare creates a fresh sample ledger and rejects any existing data directory.
func Prepare(ctx context.Context, paths Paths) error {
	_, err := ensureFixture(ctx, paths, false)
	return err
}

// Ensure initializes a new profile or verifies an existing synthetic ledger.
func Ensure(ctx context.Context, paths Paths) error {
	_, err := ensureFixture(ctx, paths, true)
	return err
}

func ensureFixture(ctx context.Context, paths Paths, reuse bool) (os.FileInfo, error) {
	root := paths.RootPath()
	if err := checkRoot(root); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	if err := requirePrivateDirectory(root); err != nil {
		return nil, err
	}
	// Serialize the first publish across processes, independently of the database's
	// GUI lifetime lock. Closing this descriptor releases the directory lock.
	dir, err := os.Open(root)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	for {
		err = syscall.Flock(int(dir.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("lock Gio profile for initialization: %w", err)
	}
	if _, err := os.Lstat(paths.data); err == nil {
		if !reuse {
			return nil, fmt.Errorf("create fresh fixture directory: %w", os.ErrExist)
		}
		return verifyFixture(ctx, paths)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// Publish both files together only after the seed is closed and verified.
	// A failed or interrupted seed leaves the final data path available for retry.
	staging, err := os.MkdirTemp(root, ".gio-seed-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)
	candidate := dataPaths(staging)
	if err := seedFixture(ctx, candidate); err != nil {
		return nil, err
	}
	verified, err := verifyFixture(ctx, candidate)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(staging, paths.data); err != nil {
		return nil, fmt.Errorf("publish synthetic Gio fixture: %w", err)
	}
	return verified, nil
}

// OpenSession holds the same per-database writer lock as the Wails desktop.
// The caller must release it after checkpointing and closing the database.
func OpenSession(ctx context.Context, paths Paths) (*sqlite.DB, func() error, error) {
	verified, err := ensureFixture(ctx, paths, true)
	if err != nil {
		return nil, nil, err
	}
	unlock, err := backup.AcquireInstanceLock(paths.database)
	if err != nil {
		return nil, nil, err
	}
	db, err := openVerified(ctx, paths, verified)
	if err != nil {
		return nil, nil, errors.Join(err, unlock())
	}
	return db, unlock, nil
}
