package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/version"
)

func (s *Service) CreateBackup(ctx context.Context, destPath string, settingsJSON []byte) (BackupCreateResult, error) {
	runtime, err := s.requireBackup()
	if err != nil {
		return BackupCreateResult{}, err
	}
	live := s.livePath()
	if live == "" {
		return BackupCreateResult{}, &domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"}
	}
	if !strings.HasSuffix(strings.ToLower(destPath), runtime.FileExt()) {
		destPath += runtime.FileExt()
	}
	if len(settingsJSON) == 0 {
		settingsJSON = []byte("{}\n")
	}
	var result BackupCreateResult
	err = s.WithExclusive(ctx, ExclusiveBackup, func(ctx context.Context) error {
		s.LockWrites()
		snapPath := filepath.Join(filepath.Dir(destPath), ".nestworth-snapshot-tmp.sqlite")
		_ = os.Remove(snapPath)
		snapErr := s.SnapshotTo(ctx, snapPath)
		s.UnlockWrites()
		if snapErr != nil {
			_ = os.Remove(snapPath)
			return snapErr
		}
		defer os.Remove(snapPath)
		_, counts, inspectErr := runtime.InspectDatabase(ctx, snapPath)
		if inspectErr != nil {
			return inspectErr
		}
		databaseBytes, readErr := os.ReadFile(snapPath)
		if readErr != nil {
			return &domain.Error{Code: domain.ErrUnavailable, Message: "the database snapshot could not be read"}
		}
		pkg := BackupPackage{
			CreatedAt: s.clock().UTC().Format(time.RFC3339),
			Database:  databaseBytes,
			Settings:  settingsJSON,
			Counts:    counts,
		}
		if writeErr := runtime.WritePackage(destPath, pkg); writeErr != nil {
			return writeErr
		}
		written, readPkgErr := runtime.ReadPackage(destPath)
		if readPkgErr != nil {
			return readPkgErr
		}
		status := BackupStatus{
			BackupFileName: filepath.Base(destPath), CreatedAt: written.CreatedAt, SchemaVersion: written.SchemaVersion,
			AppVersion: written.AppVersion, AppBuild: written.AppBuild, VerificationResult: "ok",
		}
		if statusErr := runtime.WriteStatus(live, status); statusErr != nil {
			return statusErr
		}
		result = BackupCreateResult{
			FileName: status.BackupFileName, CreatedAt: status.CreatedAt, SchemaVersion: status.SchemaVersion,
			AppVersion: version.Version, AppBuild: version.Build, Accounts: counts.Accounts, Holdings: counts.Holdings,
			Activities: counts.Activities, VerificationResult: "ok",
		}
		return nil
	})
	return result, err
}

func (s *Service) LastBackupStatus() (BackupStatus, bool, error) {
	runtime, err := s.requireBackup()
	if err != nil {
		return BackupStatus{}, false, err
	}
	live := s.livePath()
	if live == "" {
		return BackupStatus{}, false, nil
	}
	status, err := runtime.ReadStatus(live)
	if err != nil {
		if os.IsNotExist(err) {
			return BackupStatus{}, false, nil
		}
		return BackupStatus{}, false, err
	}
	return status, true, nil
}

func (s *Service) ExportCSVBytes(ctx context.Context, profile string, includeArchived bool) ([]byte, int, error) {
	codec, err := s.requireCSV()
	if err != nil {
		return nil, 0, err
	}
	var data []byte
	switch strings.ToLower(profile) {
	case CSVProfileAccounts:
		data, err = s.ExportAccountsCSV(ctx, includeArchived)
	case CSVProfileHoldings:
		data, err = s.ExportHoldingsCSV(ctx, includeArchived)
	default:
		return nil, 0, &domain.Error{Code: domain.ErrCSVInvalidFormat, Message: "CSV profile is not supported"}
	}
	if err != nil {
		return nil, 0, err
	}
	table, err := codec.Parse(data, ',')
	if err != nil {
		return nil, 0, err
	}
	return data, len(table.Rows), nil
}

func (s *Service) WriteExportFile(path string, data []byte) error {
	runtime, err := s.requireBackup()
	if err != nil {
		return err
	}
	return runtime.WriteAtomic(path, data, 0o600)
}

func (s *Service) DefaultBackupFileName() string {
	runtime := s.backupRuntime()
	if runtime == nil {
		return "Nestworth Backup" + BackupFileExt
	}
	return runtime.DefaultFileName(s.clock())
}
