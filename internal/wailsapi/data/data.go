package data

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/native"
)

type Service struct {
	app     *application.Service
	store   *settings.Store
	dialogs native.Dialogs
	refresh native.RefreshGate
}

func NewService(app *application.Service, store *settings.Store, dialogs native.Dialogs, refresh native.RefreshGate) *Service {
	if dialogs == nil {
		dialogs = native.NoopDialogs{}
	}
	if refresh == nil {
		refresh = native.NoopRefresh{}
	}
	return &Service{app: app, store: store, dialogs: dialogs, refresh: refresh}
}

type BackupStatusDTO struct {
	Available          bool   `json:"available"`
	BackupFileName     string `json:"backupFileName,omitempty"`
	CreatedAt          string `json:"createdAt,omitempty"`
	SchemaVersion      int    `json:"schemaVersion,omitempty"`
	AppVersion         string `json:"appVersion,omitempty"`
	AppBuild           string `json:"appBuild,omitempty"`
	VerificationResult string `json:"verificationResult,omitempty"`
}

func (s *Service) LastBackupStatus() (BackupStatusDTO, error) {
	if s.app == nil {
		return BackupStatusDTO{}, nil
	}
	status, available, err := s.app.LastBackupStatus()
	if err != nil {
		return BackupStatusDTO{}, apierror.Wrap(err)
	}
	if !available {
		return BackupStatusDTO{}, nil
	}
	return BackupStatusDTO{
		Available: true, BackupFileName: status.BackupFileName, CreatedAt: status.CreatedAt,
		SchemaVersion: status.SchemaVersion, AppVersion: status.AppVersion, AppBuild: status.AppBuild,
		VerificationResult: status.VerificationResult,
	}, nil
}

type BackupResultDTO struct {
	Cancelled          bool   `json:"cancelled"`
	FileName           string `json:"fileName,omitempty"`
	CreatedAt          string `json:"createdAt,omitempty"`
	SchemaVersion      int    `json:"schemaVersion,omitempty"`
	AppVersion         string `json:"appVersion,omitempty"`
	AppBuild           string `json:"appBuild,omitempty"`
	Accounts           int    `json:"accounts"`
	Holdings           int    `json:"holdings"`
	Activities         int    `json:"activities"`
	VerificationResult string `json:"verificationResult,omitempty"`
}

func (s *Service) CreateBackup() (BackupResultDTO, error) {
	if s.app == nil {
		return BackupResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	filename := s.app.DefaultBackupFileName()
	path, err := s.dialogs.SaveFile("Back up data", filename, "Nestworth Backup", "*"+application.BackupFileExt)
	if err != nil {
		return BackupResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the backup location could not be selected"})
	}
	if path == "" {
		return BackupResultDTO{Cancelled: true}, nil
	}
	if !strings.HasSuffix(strings.ToLower(path), application.BackupFileExt) {
		path += application.BackupFileExt
	}
	if _, statErr := os.Stat(path); statErr == nil {
		replace, confirmErr := s.dialogs.ConfirmReplace(filepath.Base(path))
		if confirmErr != nil {
			return BackupResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the existing backup could not be confirmed"})
		}
		if !replace {
			return BackupResultDTO{Cancelled: true}, nil
		}
	}
	settingsJSON := []byte("{}\n")
	if s.store != nil {
		if current, loadErr := s.store.Load(); loadErr == nil {
			if encoded, encodeErr := json.Marshal(current); encodeErr == nil {
				settingsJSON = append(encoded, '\n')
			}
		}
	}
	s.refresh.CancelAllAndWait()
	result, err := s.app.CreateBackup(context.Background(), path, settingsJSON)
	if err != nil {
		return BackupResultDTO{}, apierror.Wrap(err)
	}
	return BackupResultDTO{
		FileName: result.FileName, CreatedAt: result.CreatedAt, SchemaVersion: result.SchemaVersion,
		AppVersion: result.AppVersion, AppBuild: result.AppBuild, Accounts: result.Accounts,
		Holdings: result.Holdings, Activities: result.Activities, VerificationResult: result.VerificationResult,
	}, nil
}

type ExportResultDTO struct {
	Cancelled bool   `json:"cancelled"`
	FileName  string `json:"fileName,omitempty"`
}

func (s *Service) ExportJSON() (ExportResultDTO, error) {
	if s.app == nil {
		return ExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	path, err := s.dialogs.SaveFile("Export data", "Nestworth.nestworth.json", "Nestworth JSON", "*.json")
	if err != nil {
		return ExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the export location could not be selected"})
	}
	if path == "" {
		return ExportResultDTO{Cancelled: true}, nil
	}
	if !strings.HasSuffix(strings.ToLower(path), ".json") {
		path += ".nestworth.json"
	}
	if _, err := os.Stat(path); err == nil {
		replace, err := s.dialogs.ConfirmReplace(filepath.Base(path))
		if err != nil {
			return ExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the existing export could not be confirmed"})
		}
		if !replace {
			return ExportResultDTO{Cancelled: true}, nil
		}
	}
	data, err := s.app.ExportJSONBytes(context.Background())
	if err != nil {
		return ExportResultDTO{}, apierror.Wrap(err)
	}
	if err := s.app.WriteExportFile(path, data); err != nil {
		return ExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the JSON file could not be written"})
	}
	return ExportResultDTO{FileName: filepath.Base(path)}, nil
}
