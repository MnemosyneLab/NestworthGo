package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

const (
	JournalFileName = ".nestworth-restore-journal.json"
	StatusFileName  = ".nestworth-backup-status.json"

	StatePrepared             = "prepared"
	StateOriginalRenamed      = "original-renamed"
	StateReplacementInstalled = "replacement-installed"
	StateVerified             = "verified"
)

type Journal struct {
	FormatVersion  int    `json:"format_version"`
	Operation      string `json:"operation"`
	State          string `json:"state"`
	StartedAt      string `json:"started_at"`
	LiveName       string `json:"live_name"`
	SafetyName     string `json:"safety_name"`
	StagingName    string `json:"staging_name"`
	BackupSHA256   string `json:"backup_database_sha256"`
	AppVersion     string `json:"app_version"`
	AppBuild       string `json:"app_build"`
	SchemaVersion  int    `json:"schema_version"`
	BackupFormat   int    `json:"backup_format_version"`
	RestoreChrome  bool   `json:"restore_chrome"`
	RestoreFormat  bool   `json:"restore_format"`
	RestoreRouting bool   `json:"restore_routing"`
	SettingsJSON   string `json:"settings_json,omitempty"`
}

type Status struct {
	FormatVersion      int    `json:"format_version"`
	BackupFileName     string `json:"backup_file_name"`
	CreatedAt          string `json:"created_at"`
	SchemaVersion      int    `json:"schema_version"`
	AppVersion         string `json:"app_version"`
	AppBuild           string `json:"app_build"`
	VerificationResult string `json:"verification_result"`
}

func JournalPath(liveDBPath string) string {
	return filepath.Join(filepath.Dir(liveDBPath), JournalFileName)
}

func StatusPath(liveDBPath string) string {
	return filepath.Join(filepath.Dir(liveDBPath), StatusFileName)
}

func WriteJournal(liveDBPath string, journal Journal) error {
	journal.FormatVersion = FormatVersion
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "restore journal could not be encoded"}
	}
	data = append(data, '\n')
	if err := WriteAtomicFile(JournalPath(liveDBPath), data, 0o600); err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "restore journal could not be written"}
	}
	return nil
}

func ReadJournal(liveDBPath string) (Journal, error) {
	data, err := os.ReadFile(JournalPath(liveDBPath))
	if errors.Is(err, os.ErrNotExist) {
		return Journal{}, os.ErrNotExist
	}
	if err != nil {
		return Journal{}, &domain.Error{Code: domain.ErrUnavailable, Message: "restore journal could not be read"}
	}
	var journal Journal
	if err := json.Unmarshal(data, &journal); err != nil {
		return Journal{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "restore journal could not be parsed"}
	}
	return journal, nil
}

func RemoveJournal(liveDBPath string) error {
	err := os.Remove(JournalPath(liveDBPath))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "restore journal could not be removed"}
	}
	return nil
}

func WriteStatus(liveDBPath string, status Status) error {
	status.FormatVersion = FormatVersion
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "backup status could not be encoded"}
	}
	data = append(data, '\n')
	if err := WriteAtomicFile(StatusPath(liveDBPath), data, 0o600); err != nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "backup status could not be written"}
	}
	return nil
}

func ReadStatus(liveDBPath string) (Status, error) {
	data, err := os.ReadFile(StatusPath(liveDBPath))
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, os.ErrNotExist
	}
	if err != nil {
		return Status{}, &domain.Error{Code: domain.ErrUnavailable, Message: "backup status could not be read"}
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return Status{}, &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "backup status could not be parsed"}
	}
	return status, nil
}

// ReconcileOnStartup applies an unfinished restore journal before sqlite.Open.
// It never creates an empty current-schema database.
func ReconcileOnStartup(liveDBPath string, store *settings.Store) error {
	journal, err := ReadJournal(liveDBPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	liveExists := GroupExists(liveDBPath)
	safetyExists := journal.SafetyName != "" && GroupExists(filepath.Join(filepath.Dir(liveDBPath), journal.SafetyName))
	liveMainExists := fileExists(liveDBPath)
	safetyMainExists := journal.SafetyName != "" && fileExists(filepath.Join(filepath.Dir(liveDBPath), journal.SafetyName))
	state := journal.State
	if state == StateOriginalRenamed && liveExists && safetyExists {
		state = StateReplacementInstalled
	}
	switch state {
	case StatePrepared:
		if liveMainExists {
			_ = os.Remove(filepath.Join(filepath.Dir(liveDBPath), journal.StagingName))
			return RemoveJournal(liveDBPath)
		}
		if !liveExists && safetyMainExists {
			if err := restoreSafetyGroup(liveDBPath, journal); err != nil {
				return err
			}
			return RemoveJournal(liveDBPath)
		}
		if liveExists && safetyMainExists {
			if err := moveOrphanSidecars(liveDBPath, filepath.Join(filepath.Dir(liveDBPath), journal.SafetyName), os.Rename); err != nil {
				return &domain.Error{Code: domain.ErrBackupRestoreRollbackFailed, Message: "orphan database sidecars could not be reconciled"}
			}
			if err := restoreSafetyGroup(liveDBPath, journal); err != nil {
				return err
			}
			return RemoveJournal(liveDBPath)
		}
		return &domain.Error{Code: domain.ErrBackupRestoreRollbackFailed, Message: "restore could not be completed and no safety copy is available"}
	case StateOriginalRenamed:
		if !liveExists && safetyExists {
			if err := restoreSafetyGroup(liveDBPath, journal); err != nil {
				return err
			}
			return RemoveJournal(liveDBPath)
		}
		if liveExists && safetyExists {
			return verifyOrRollbackReplacement(liveDBPath, journal, store)
		}
		return &domain.Error{Code: domain.ErrBackupRestoreRollbackFailed, Message: "restore could not be completed and no safety copy is available"}
	case StateReplacementInstalled:
		return verifyOrRollbackReplacement(liveDBPath, journal, store)
	case StateVerified:
		return finalizeVerified(liveDBPath, journal, store)
	default:
		return &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "restore journal state is not supported"}
	}
}

func verifyOrRollbackReplacement(liveDBPath string, journal Journal, store *settings.Store) error {
	if !GroupExists(liveDBPath) {
		if err := restoreSafetyGroup(liveDBPath, journal); err != nil {
			return err
		}
		return RemoveJournal(liveDBPath)
	}
	verified, err := sqlite.OpenReadOnlyForVerify(liveDBPath)
	if err != nil {
		failedName, nameErr := UniqueFailedRestoreName(liveDBPath, time.Now().UTC())
		if nameErr != nil {
			return &domain.Error{Code: domain.ErrBackupRestoreRollbackFailed, Message: "restored database failed verification and could not be quarantined"}
		}
		failed := filepath.Join(filepath.Dir(liveDBPath), failedName)
		if moveErr := MoveFileGroup(liveDBPath, failed); moveErr != nil {
			return &domain.Error{Code: domain.ErrBackupRestoreRollbackFailed, Message: "restored database failed verification and could not be quarantined"}
		}
		if moveErr := restoreSafetyGroup(liveDBPath, journal); moveErr != nil {
			return &domain.Error{Code: domain.ErrBackupRestoreRollbackFailed, Message: "restored database failed verification and the safety copy could not be restored"}
		}
		return RemoveJournal(liveDBPath)
	}
	_ = verified.Close()
	journal.State = StateVerified
	if err := WriteJournal(liveDBPath, journal); err != nil {
		return err
	}
	return finalizeVerified(liveDBPath, journal, store)
}

func finalizeVerified(liveDBPath string, journal Journal, store *settings.Store) error {
	dir := filepath.Dir(liveDBPath)
	if journal.StagingName != "" {
		_ = os.Remove(filepath.Join(dir, journal.StagingName))
	}
	if store != nil && journal.SettingsJSON != "" && (journal.RestoreChrome || journal.RestoreFormat || journal.RestoreRouting) {
		current, loadErr := store.Load()
		if loadErr == nil {
			var backupSettings settings.Settings
			if json.Unmarshal([]byte(journal.SettingsJSON), &backupSettings) == nil {
				merged := MergeSettings(current, backupSettings, journal.RestoreChrome, journal.RestoreFormat, journal.RestoreRouting)
				if merged.Validate() == nil {
					_ = store.Save(merged)
				}
			}
		}
	}
	return RemoveJournal(liveDBPath)
}

func MergeSettings(current, backup settings.Settings, chrome, format, routing bool) settings.Settings {
	merged := current
	if chrome {
		merged.Appearance = backup.Appearance
		merged.Accent = backup.Accent
		merged.Language = backup.Language
		merged.WindowWidth = backup.WindowWidth
		merged.WindowHeight = backup.WindowHeight
	}
	if format {
		merged.Currency = backup.Currency
		merged.DecimalSeparator = backup.DecimalSeparator
		merged.GroupingSeparator = backup.GroupingSeparator
		merged.DecimalPlaces = backup.DecimalPlaces
		merged.Timezone = backup.Timezone
		merged.WeekStart = backup.WeekStart
		merged.DateFormat = backup.DateFormat
		merged.TimeFormat = backup.TimeFormat
	}
	if routing {
		merged.FXProvider = backup.FXProvider
		merged.QuoteCacheTTL = backup.QuoteCacheTTL
	}
	return merged
}

func restoreSafetyGroup(liveDBPath string, journal Journal) error {
	if journal.SafetyName == "" {
		return &domain.Error{Code: domain.ErrBackupRestoreRollbackFailed, Message: "safety copy is missing"}
	}
	dir := filepath.Dir(liveDBPath)
	safety := filepath.Join(dir, journal.SafetyName)
	if !fileExists(safety) {
		return &domain.Error{Code: domain.ErrBackupRestoreRollbackFailed, Message: "safety copy is missing or incomplete"}
	}
	if err := MoveFileGroup(safety, liveDBPath); err != nil {
		return &domain.Error{Code: domain.ErrBackupRestoreRollbackFailed, Message: "safety copy could not be restored"}
	}
	_ = syncDir(dir)
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func VerifyExtractedDatabase(ctx context.Context, path string) error {
	db, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.HouseholdSummary(ctx); err != nil {
		return err
	}
	if _, err := db.EntityCounts(ctx); err != nil {
		return err
	}
	return nil
}
