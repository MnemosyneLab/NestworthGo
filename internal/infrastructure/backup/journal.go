package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
)

const (
	JournalFileName = ".nestworth-restore-journal.json"
	StatusFileName  = ".nestworth-backup-status.json"

	StatePrepared             = "prepared"
	StateOriginalRenamed      = "original-renamed"
	StateReplacementInstalled = "replacement-installed"
	StateVerified             = "verified"
	StateQuarantining         = "quarantining"
	StateRollingBack          = "rolling-back"
)

type Journal struct {
	FormatVersion int    `json:"format_version"`
	Operation     string `json:"operation"`
	State         string `json:"state"`
	StartedAt     string `json:"started_at"`
	LiveName      string `json:"live_name"`
	SafetyName    string `json:"safety_name"`
	StagingName   string `json:"staging_name"`
	// Nil preserves the ambiguity of journals written before this field existed.
	OriginalPresent *bool  `json:"original_present,omitempty"`
	FailedName      string `json:"failed_name,omitempty"`
	BackupSHA256    string `json:"backup_database_sha256"`
	AppVersion      string `json:"app_version"`
	AppBuild        string `json:"app_build"`
	SchemaVersion   int    `json:"schema_version"`
	BackupFormat    int    `json:"backup_format_version"`
	RestoreChrome   bool   `json:"restore_chrome"`
	RestoreFormat   bool   `json:"restore_format"`
	RestoreRouting  bool   `json:"restore_routing"`
	SettingsJSON    string `json:"settings_json,omitempty"`
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
	data, err := jsoncodec.MarshalIndent(journal, "", "  ")
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
	if err := jsoncodec.Unmarshal(data, &journal); err != nil {
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
	data, err := jsoncodec.MarshalIndent(status, "", "  ")
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
	if err := jsoncodec.Unmarshal(data, &status); err != nil {
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
	dir := filepath.Dir(liveDBPath)
	safety := filepath.Join(dir, journal.SafetyName)
	liveExists := GroupExists(liveDBPath)
	safetyExists := journal.SafetyName != "" && GroupExists(safety)
	liveMainExists := fileExists(liveDBPath)
	safetyMainExists := journal.SafetyName != "" && fileExists(safety)
	firstRestore := journal.OriginalPresent != nil && !*journal.OriginalPresent
	switch journal.State {
	case StatePrepared:
		if safetyMainExists && !liveMainExists {
			// The original main file moved first; its sidecars may still be
			// split across live and safety at either subsequent rename.
			if liveExists {
				if err := moveOrphanSidecars(liveDBPath, safety, os.Rename); err != nil {
					return recoveryBlocked("orphan database sidecars could not be reconciled")
				}
			}
			return beginSafetyRollback(liveDBPath, journal)
		}
		if liveMainExists && !safetyMainExists {
			if firstRestore {
				return verifyOrRollbackReplacement(liveDBPath, journal, store)
			}
			// Also finish a rollback interrupted under an older journal.
			if safetyExists {
				if err := moveOrphanSidecars(safety, liveDBPath, os.Rename); err != nil {
					return recoveryBlocked("original database sidecars could not be reconciled")
				}
			}
			return finishRecovery(liveDBPath, journal)
		}
		if firstRestore && !liveExists && !safetyExists {
			return installStagedOnStartup(liveDBPath, journal, store)
		}
		return recoveryBlocked("restore could not be completed and no complete original copy is available")
	case StateOriginalRenamed:
		if liveMainExists {
			staging := filepath.Join(dir, journal.StagingName)
			if journal.StagingName != "" && GroupExists(staging) {
				if err := moveOrphanSidecars(staging, liveDBPath, os.Rename); err != nil {
					return recoveryBlocked("restored database sidecars could not be reconciled")
				}
			}
			// The staging rename may have completed before its journal write,
			// including a first restore with no original/safety database.
			return verifyOrRollbackReplacement(liveDBPath, journal, store)
		}
		if safetyMainExists {
			if liveExists {
				if err := moveOrphanSidecars(liveDBPath, safety, os.Rename); err != nil {
					return recoveryBlocked("orphan database sidecars could not be reconciled")
				}
			}
			return beginSafetyRollback(liveDBPath, journal)
		}
		if firstRestore && !liveExists && !safetyExists {
			return installStagedOnStartup(liveDBPath, journal, store)
		}
		return recoveryBlocked("restore could not be completed and no complete original copy is available")
	case StateReplacementInstalled, StateVerified:
		return verifyOrRollbackReplacement(liveDBPath, journal, store)
	case StateQuarantining:
		return quarantineAndRollback(liveDBPath, journal)
	case StateRollingBack:
		return resumeSafetyRollback(liveDBPath, journal)
	default:
		return &domain.Error{Code: domain.ErrBackupInvalidFormat, Message: "restore journal state is not supported"}
	}
}

func recoveryBlocked(message string) error {
	return &domain.Error{Code: domain.ErrBackupRestoreRollbackFailed, Message: message}
}

func installStagedOnStartup(liveDBPath string, journal Journal, store *settings.Store) error {
	if journal.StagingName == "" {
		return recoveryBlocked("the staged database is missing")
	}
	staging := filepath.Join(filepath.Dir(liveDBPath), journal.StagingName)
	if err := VerifyExtractedDatabase(context.Background(), staging); err != nil {
		return err
	}
	journal.State = StateOriginalRenamed
	if err := WriteJournal(liveDBPath, journal); err != nil {
		return err
	}
	if err := MoveFileGroup(staging, liveDBPath); err != nil {
		return err
	}
	journal.State = StateReplacementInstalled
	if err := WriteJournal(liveDBPath, journal); err != nil {
		return err
	}
	return verifyOrRollbackReplacement(liveDBPath, journal, store)
}

func verifyOrRollbackReplacement(liveDBPath string, journal Journal, store *settings.Store) error {
	verified, err := sqlite.OpenReadOnlyForVerify(liveDBPath)
	if err != nil {
		// Without an original main file, retain both the invalid/missing live
		// group and journal. A later startup must not bootstrap an empty ledger.
		if journal.SafetyName == "" || !fileExists(filepath.Join(filepath.Dir(liveDBPath), journal.SafetyName)) {
			return recoveryBlocked("restored database failed verification and no complete safety copy is available")
		}
		if !GroupExists(liveDBPath) {
			return beginSafetyRollback(liveDBPath, journal)
		}
		failedName, nameErr := UniqueFailedRestoreName(liveDBPath, time.Now().UTC())
		if nameErr != nil {
			return recoveryBlocked("restored database failed verification and could not be quarantined")
		}
		journal.State = StateQuarantining
		journal.FailedName = failedName
		if err := WriteJournal(liveDBPath, journal); err != nil {
			return err
		}
		return quarantineAndRollback(liveDBPath, journal)
	}
	if err := verified.Close(); err != nil {
		return err
	}
	if journal.State == StateVerified {
		return finalizeVerified(liveDBPath, journal, store)
	}
	// Rotate only the installed live copy. An interrupted reset is safely
	// repeatable; the verified backup/safety artifacts retain their old epoch.
	restored, err := sqlite.Open(liveDBPath)
	if err != nil {
		return err
	}
	resetErr := restored.ResetFinancialSourceTracking(context.Background())
	closeErr := restored.Close()
	if err := errors.Join(resetErr, closeErr); err != nil {
		return err
	}
	journal.State = StateVerified
	if err := WriteJournal(liveDBPath, journal); err != nil {
		return err
	}
	return finalizeVerified(liveDBPath, journal, store)
}

func quarantineAndRollback(liveDBPath string, journal Journal) error {
	if journal.FailedName == "" {
		return recoveryBlocked("failed restore quarantine is missing")
	}
	failed := filepath.Join(filepath.Dir(liveDBPath), journal.FailedName)
	if err := resumeFileGroupMove(liveDBPath, failed); err != nil {
		return recoveryBlocked("restored database failed verification and could not be quarantined")
	}
	return beginSafetyRollback(liveDBPath, journal)
}

func beginSafetyRollback(liveDBPath string, journal Journal) error {
	journal.State = StateRollingBack
	if err := WriteJournal(liveDBPath, journal); err != nil {
		return err
	}
	return resumeSafetyRollback(liveDBPath, journal)
}

func resumeSafetyRollback(liveDBPath string, journal Journal) error {
	if journal.SafetyName == "" {
		return recoveryBlocked("safety copy is missing")
	}
	safety := filepath.Join(filepath.Dir(liveDBPath), journal.SafetyName)
	if !fileExists(safety) && !fileExists(liveDBPath) {
		return recoveryBlocked("safety copy is missing or incomplete")
	}
	if err := resumeFileGroupMove(safety, liveDBPath); err != nil {
		return recoveryBlocked("safety copy could not be restored")
	}
	return finishRecovery(liveDBPath, journal)
}

// A crash may split the main-first group move across its source/destination.
// Only disjoint sidecars can be reunited; occupied files are never overwritten.
func resumeFileGroupMove(src, dest string) error {
	if fileExists(src) {
		return MoveFileGroup(src, dest)
	}
	if GroupExists(src) {
		return moveOrphanSidecars(src, dest, os.Rename)
	}
	if !GroupExists(dest) {
		return recoveryBlocked("database file group is missing")
	}
	return nil
}

func finishRecovery(liveDBPath string, journal Journal) error {
	if journal.StagingName != "" {
		staging := filepath.Join(filepath.Dir(liveDBPath), journal.StagingName)
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Remove(staging + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				return &domain.Error{Code: domain.ErrUnavailable, Message: "restore staging files could not be removed"}
			}
		}
	}
	return RemoveJournal(liveDBPath)
}

func finalizeVerified(liveDBPath string, journal Journal, store *settings.Store) error {
	if store != nil {
		db, err := sqlite.Open(liveDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		restored := settings.NewStore(store.Path)
		if err := restored.Attach(sqlite.NewConfigurationRepository(db)); err != nil {
			return err
		}
		if journal.SettingsJSON != "" && (journal.RestoreChrome || journal.RestoreFormat || journal.RestoreRouting) {
			current, err := restored.Load()
			if err != nil {
				return err
			}
			backupSettings := current
			if err := jsoncodec.Unmarshal([]byte(journal.SettingsJSON), &backupSettings); err != nil {
				return err
			}
			// Durable configuration always comes from the restored database.
			presentation, err := settings.PresentationJSON(backupSettings)
			if err != nil {
				return err
			}
			backupSettings = current
			if err := jsoncodec.Unmarshal(presentation, &backupSettings); err != nil {
				return err
			}
			merged := MergeSettings(current, backupSettings, journal.RestoreChrome, journal.RestoreFormat, false)
			if err := restored.Save(merged); err != nil {
				return err
			}
		}
	}
	return finishRecovery(liveDBPath, journal)
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
