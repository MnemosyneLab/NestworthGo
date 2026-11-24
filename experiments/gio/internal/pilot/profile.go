package pilot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Prepare creates a fresh sample ledger and rejects any existing data directory.
func Prepare(ctx context.Context, paths Paths) error {
	return ensureFixture(ctx, paths, false, seedFixture)
}

// PrepareOnboarding creates the empty onboarding fixture in a fresh explicit run.
func PrepareOnboarding(ctx context.Context, paths Paths) error {
	if os.Getenv("NESTWORTH_GIO_RUN_DIR") != paths.RootPath() {
		return errors.New("preparing onboarding requires an explicit NESTWORTH_GIO_RUN_DIR")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	if filepath.Join(configDir, AppID) == paths.RootPath() {
		return errors.New("use a disposable run directory for onboarding")
	}
	return ensureFixture(ctx, paths, false, seedOnboardingFixture)
}

// Ensure initializes a new profile or verifies an existing synthetic ledger.
func Ensure(ctx context.Context, paths Paths) error {
	if err := ensureFixture(ctx, paths, true, seedFixture); err != nil {
		return err
	}
	return Verify(ctx, paths)
}

func ensureFixture(ctx context.Context, paths Paths, reuse bool, seed func(context.Context, Paths) error) error {
	root := paths.RootPath()
	if err := checkRoot(root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	if err := requirePrivateDirectory(root); err != nil {
		return err
	}
	// Serialize the first publish across processes, independently of the database's
	// GUI lifetime lock. Closing this descriptor releases the directory lock.
	dir, err := os.Open(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	for {
		err = syscall.Flock(int(dir.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("lock Gio profile for initialization: %w", err)
	}
	if _, err := os.Lstat(paths.data); err == nil {
		if !reuse {
			return fmt.Errorf("create fresh fixture directory: %w", os.ErrExist)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Publish both files together only after the seed is closed and verified.
	// A failed or interrupted seed leaves the final data path available for retry.
	staging, err := os.MkdirTemp(root, ".gio-seed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	candidate := dataPaths(staging)
	if err := prepareCandidate(ctx, candidate, seed); err != nil {
		return err
	}
	if err := os.Rename(staging, paths.data); err != nil {
		return fmt.Errorf("publish synthetic Gio fixture: %w", err)
	}
	return nil
}

func prepareCandidate(ctx context.Context, paths Paths, seed func(context.Context, Paths) error) (result error) {
	unlock, err := backup.AcquireInstanceLock(paths.database)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, unlock()) }()
	if err := seed(ctx, paths); err != nil {
		return err
	}
	files, err := inspectFixture(paths)
	if err != nil {
		return err
	}
	return verifyFixture(ctx, paths, files)
}

func lockVerifiedFixture(ctx context.Context, paths Paths) (os.FileInfo, func() error, error) {
	if err := checkRoot(paths.RootPath()); err != nil {
		return nil, nil, err
	}
	files, err := inspectFixture(paths)
	if err != nil {
		return nil, nil, err
	}
	if !files.lock {
		// Older pilots may have no lock file. Check the marker on a copy
		// before creating anything beside an unrecognized ledger. Always
		// verify again once the shared writer lock stabilizes the files.
		if err := verifyFixture(ctx, paths, files); err != nil {
			return nil, nil, err
		}
	}
	unlock, err := backup.AcquireInstanceLock(paths.database)
	if err != nil {
		return nil, nil, err
	}
	current, err := inspectFixture(paths)
	if err == nil {
		if !current.lock || !os.SameFile(files.database, current.database) {
			err = errors.New("the isolated database changed while acquiring its instance lock")
		} else {
			err = verifyFixture(ctx, paths, current)
		}
	}
	if err != nil {
		return nil, nil, errors.Join(err, unlock())
	}
	return current.database, unlock, nil
}

// Verify checks an existing fixture and releases its instance lock without
// opening a writable SQLite connection.
func Verify(ctx context.Context, paths Paths) error {
	_, unlock, err := lockVerifiedFixture(ctx, paths)
	if err != nil {
		return err
	}
	return unlock()
}

// Session owns a verified writable database and its shared Wails instance lock.
type Session struct {
	DB    *sqlite.DB
	close func() error
}

// Close checkpoints and closes the database before releasing its lock.
// Callers must first stop work that uses the database.
func (s *Session) Close() error { return s.close() }

func OpenSession(ctx context.Context, paths Paths) (*Session, error) {
	if err := ensureFixture(ctx, paths, true, seedFixture); err != nil {
		return nil, err
	}
	verified, unlock, err := lockVerifiedFixture(ctx, paths)
	if err != nil {
		return nil, err
	}
	db, err := openVerified(ctx, paths, verified)
	if err != nil {
		return nil, errors.Join(err, unlock())
	}
	return &Session{DB: db, close: sync.OnceValue(func() error {
		return errors.Join(db.CheckpointWAL(context.Background()), db.Close(), unlock())
	})}, nil
}
