package backup

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

var ErrAlreadyRunning = errors.New("another Nestworth instance is already using this database")

// AcquireInstanceLock must run before journal recovery or opening business DB.
// OS locks are released on process death; the private lock file may remain.
func AcquireInstanceLock(path string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(path))
		if parentErr != nil {
			return nil, parentErr
		}
		canonical = filepath.Join(parent, filepath.Base(path))
	} else if err != nil {
		return nil, err
	}
	lockPath := canonical + ".instance-lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	lock := flock.New(lockPath)
	ok, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAlreadyRunning
	}
	return lock.Close, nil
}
