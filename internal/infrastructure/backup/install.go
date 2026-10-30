package backup

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"os"
	"path/filepath"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type RestoreClasses struct {
	Chrome  bool
	Format  bool
	Routing bool
}

// SessionHooks are optional. When the live database was never opened, all
// hooks are nil and InstallRestore only moves files.
type SessionHooks struct {
	Quiesce    func(context.Context) error
	Checkpoint func(context.Context) error
	Close      func() error
}

func InstallRestore(ctx context.Context, liveDBPath string, pkg Package, hooks *SessionHooks, classes RestoreClasses, now time.Time) error {
	dir := filepath.Dir(liveDBPath)
	stagingName := StagingName(liveDBPath)
	stagingPath := filepath.Join(dir, stagingName)
	_ = os.Remove(stagingPath)
	if err := ExtractDatabase(pkg, stagingPath); err != nil {
		return err
	}
	if err := verifyRestoreCandidate(ctx, stagingPath, pkg.Manifest.SchemaVersion); err != nil {
		_ = os.Remove(stagingPath)
		return err
	}
	if hooks != nil && hooks.Quiesce != nil {
		if err := hooks.Quiesce(ctx); err != nil {
			_ = os.Remove(stagingPath)
			return err
		}
	}
	if hooks != nil && hooks.Checkpoint != nil {
		if err := hooks.Checkpoint(ctx); err != nil {
			_ = os.Remove(stagingPath)
			return err
		}
	}
	safetyName, err := UniqueSafetyName(liveDBPath, now)
	if err != nil {
		_ = os.Remove(stagingPath)
		return err
	}
	originalPresent := GroupExists(liveDBPath)
	journal := Journal{
		Operation:       "restore",
		State:           StatePrepared,
		StartedAt:       now.UTC().Format(time.RFC3339),
		LiveName:        filepath.Base(liveDBPath),
		SafetyName:      safetyName,
		StagingName:     stagingName,
		OriginalPresent: &originalPresent,
		BackupSHA256:    pkg.Manifest.Members[MemberDatabase].SHA256,
		AppVersion:      pkg.Manifest.AppVersion,
		AppBuild:        pkg.Manifest.AppBuild,
		SchemaVersion:   pkg.Manifest.SchemaVersion,
		BackupFormat:    pkg.Manifest.FormatVersion,
		RestoreChrome:   classes.Chrome,
		RestoreFormat:   classes.Format,
		RestoreRouting:  classes.Routing,
		SettingsJSON:    string(pkg.Settings),
	}
	if err := WriteJournal(liveDBPath, journal); err != nil {
		_ = os.Remove(stagingPath)
		return err
	}
	if hooks != nil && hooks.Close != nil {
		if err := hooks.Close(); err != nil {
			_ = os.Remove(stagingPath)
			_ = RemoveJournal(liveDBPath)
			return &domain.Error{Code: domain.ErrBackupRestoreSwapFailed, Message: "the current database session could not be closed"}
		}
	}
	if GroupExists(liveDBPath) {
		if err := MoveFileGroup(liveDBPath, filepath.Join(dir, safetyName)); err != nil {
			_ = os.Remove(stagingPath)
			return err
		}
	}
	journal.State = StateOriginalRenamed
	if err := WriteJournal(liveDBPath, journal); err != nil {
		return err
	}
	if err := MoveFileGroup(stagingPath, liveDBPath); err != nil {
		if originalPresent {
			if rollbackErr := beginSafetyRollback(liveDBPath, journal); rollbackErr != nil {
				return rollbackErr
			}
		}
		return &domain.Error{Code: domain.ErrBackupRestoreSwapFailed, Message: "the restored database could not be installed"}
	}
	journal.State = StateReplacementInstalled
	if err := WriteJournal(liveDBPath, journal); err != nil {
		return err
	}
	return nil
}

// Supported versions must still agree with the checksummed backup manifest.
func verifyRestoreCandidate(ctx context.Context, path string, expectedVersion int) error {
	db, err := sqlite.OpenReadOnlyForVerifyContext(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	var version int
	if err := db.SQL.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != expectedVersion {
		return &domain.Error{Code: domain.ErrBackupIntegrityFailed, Message: "database schema does not match backup manifest"}
	}
	return nil
}
