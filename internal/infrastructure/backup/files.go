package backup

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func sidecarPaths(mainPath string) (wal, shm string) {
	return mainPath + "-wal", mainPath + "-shm"
}

// GroupExists is true when the SQLite main file or any WAL/SHM sidecar is
// present. Restore must treat an orphan sidecar as part of the live group.
func GroupExists(mainPath string) bool {
	if fileExists(mainPath) {
		return true
	}
	wal, shm := sidecarPaths(mainPath)
	return fileExists(wal) || fileExists(shm)
}

type moveStep struct {
	from string
	to   string
}

type renameFile func(string, string) error

// MoveFileGroup renames a SQLite main file and any present -wal/-shm sidecars
// as one group. dest must not already have a main file or sidecars. A sidecar
// failure rolls back files already moved.
func MoveFileGroup(src, dest string) error {
	return moveFileGroup(src, dest, os.Rename)
}

func moveFileGroup(src, dest string, rename renameFile) error {
	if GroupExists(dest) {
		return &domain.Error{Code: domain.ErrConflict, Message: "destination database already exists"}
	}
	srcWAL, srcSHM := sidecarPaths(src)
	destWAL, destSHM := sidecarPaths(dest)
	hasMain := fileExists(src)
	hasWAL := fileExists(srcWAL)
	hasSHM := fileExists(srcSHM)
	if !hasMain && !hasWAL && !hasSHM {
		return &domain.Error{Code: domain.ErrNotFound, Message: "database file group is missing"}
	}
	var done []moveStep
	rollback := func() error {
		for i := len(done) - 1; i >= 0; i-- {
			if err := rename(done[i].to, done[i].from); err != nil {
				return err
			}
		}
		done = nil
		return nil
	}
	moveOne := func(from, to string, present bool) error {
		if !present {
			return nil
		}
		if err := rename(from, to); err != nil {
			return err
		}
		done = append(done, moveStep{from: from, to: to})
		return nil
	}
	if err := moveOne(src, dest, hasMain); err != nil {
		return moveFailure(rollback)
	}
	if err := moveOne(srcWAL, destWAL, hasWAL); err != nil {
		return moveFailure(rollback)
	}
	if err := moveOne(srcSHM, destSHM, hasSHM); err != nil {
		return moveFailure(rollback)
	}
	_ = syncDir(filepath.Dir(dest))
	return nil
}

func moveFailure(rollback func() error) error {
	if rollbackErr := rollback(); rollbackErr != nil {
		return &domain.Error{Code: domain.ErrBackupRestoreRollbackFailed, Message: "the database file group could not be moved back safely"}
	}
	return &domain.Error{Code: domain.ErrBackupRestoreSwapFailed, Message: "the database file group could not be moved"}
}

// moveOrphanSidecars moves only sidecars when a crashed swap left the source
// group without its main database file. It is deliberately separate from
// MoveFileGroup so restore never treats an orphan sidecar as a valid database.
func moveOrphanSidecars(src, dest string, rename renameFile) error {
	srcWAL, srcSHM := sidecarPaths(src)
	destWAL, destSHM := sidecarPaths(dest)
	if fileExists(src) || fileExists(destWAL) || fileExists(destSHM) {
		return &domain.Error{Code: domain.ErrConflict, Message: "orphan sidecars cannot be moved to an occupied database group"}
	}
	steps := []moveStep{}
	if fileExists(srcWAL) {
		steps = append(steps, moveStep{from: srcWAL, to: destWAL})
	}
	if fileExists(srcSHM) {
		steps = append(steps, moveStep{from: srcSHM, to: destSHM})
	}
	if len(steps) == 0 {
		return &domain.Error{Code: domain.ErrNotFound, Message: "database sidecars are missing"}
	}
	var done []moveStep
	rollback := func() error {
		for i := len(done) - 1; i >= 0; i-- {
			if err := rename(done[i].to, done[i].from); err != nil {
				return err
			}
		}
		return nil
	}
	for _, step := range steps {
		if err := rename(step.from, step.to); err != nil {
			return moveFailure(rollback)
		}
		done = append(done, step)
	}
	return nil
}

func uniqueGroupName(liveDBPath string, now time.Time, marker string) (string, error) {
	var entropy [4]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	stamp := now.UTC().Format("20060102T150405Z")
	base := filepath.Base(liveDBPath)
	name := fmt.Sprintf("%s.%s-%s-%s", base, marker, stamp, hex.EncodeToString(entropy[:]))
	dir := filepath.Dir(liveDBPath)
	if GroupExists(filepath.Join(dir, name)) {
		return "", &domain.Error{Code: domain.ErrConflict, Message: "a restore temporary database group with this name already exists"}
	}
	return name, nil
}

func UniqueSafetyName(liveDBPath string, now time.Time) (string, error) {
	return uniqueGroupName(liveDBPath, now, "pre-restore")
}

func UniqueFailedRestoreName(liveDBPath string, now time.Time) (string, error) {
	return uniqueGroupName(liveDBPath, now, "failed-restore")
}

func StagingName(liveDBPath string) string {
	return filepath.Base(liveDBPath) + ".restore-staging"
}
