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

type CSVExportResultDTO struct {
	Cancelled bool   `json:"cancelled"`
	FileName  string `json:"fileName,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Rows      int    `json:"rows"`
}

func (s *Service) ExportCSV(profile string, includeArchived bool) (CSVExportResultDTO, error) {
	if s.app == nil {
		return CSVExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	filename := "Nestworth-" + strings.ToLower(profile) + ".csv"
	path, err := s.dialogs.SaveFile("Export CSV", filename, "CSV", "*.csv")
	if err != nil {
		return CSVExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the export location could not be selected"})
	}
	if path == "" {
		return CSVExportResultDTO{Cancelled: true}, nil
	}
	if !strings.HasSuffix(strings.ToLower(path), ".csv") {
		path += ".csv"
	}
	if _, statErr := os.Stat(path); statErr == nil {
		replace, confirmErr := s.dialogs.ConfirmReplace(filepath.Base(path))
		if confirmErr != nil || !replace {
			return CSVExportResultDTO{Cancelled: true}, nil
		}
	}
	data, rows, err := s.app.ExportCSVBytes(context.Background(), profile, includeArchived)
	if err != nil {
		return CSVExportResultDTO{}, apierror.Wrap(err)
	}
	if err := s.app.WriteExportFile(path, data); err != nil {
		return CSVExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the CSV file could not be written"})
	}
	return CSVExportResultDTO{FileName: filepath.Base(path), Profile: strings.ToLower(profile), Rows: rows}, nil
}

type CSVFileDTO struct {
	Cancelled   bool     `json:"cancelled"`
	Token       string   `json:"token,omitempty"`
	Profile     string   `json:"profile,omitempty"`
	FileName    string   `json:"fileName,omitempty"`
	Delimiter   string   `json:"delimiter,omitempty"`
	HasBOM      bool     `json:"hasBom"`
	Headers     []string `json:"headers,omitempty"`
	RowCount    int      `json:"rowCount"`
	ColumnCount int      `json:"columnCount"`
}

func (s *Service) SelectCSV(profile, sessionToken string) (CSVFileDTO, error) {
	path, err := s.dialogs.OpenFile("Import CSV", "CSV", "*.csv")
	if err != nil {
		return CSVFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the CSV file could not be selected"})
	}
	if path == "" {
		return CSVFileDTO{Cancelled: true}, nil
	}
	if s.app == nil {
		return CSVFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CSVFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrCSVInvalidEncoding, Message: "the CSV file could not be read"})
	}
	selected, err := s.app.SelectCSV(profile, sessionToken, filepath.Base(path), data)
	if err != nil {
		return CSVFileDTO{}, apierror.Wrap(err)
	}
	return CSVFileDTO{
		Token: selected.Token, Profile: selected.Profile, FileName: selected.FileName,
		Delimiter: selected.Delimiter, HasBOM: selected.HasBOM, Headers: selected.Headers,
		RowCount: selected.RowCount, ColumnCount: selected.ColumnCount,
	}, nil
}

type CSVOptionsRequest struct {
	Token           string                            `json:"token"`
	Profile         string                            `json:"profile"`
	Delimiter       string                            `json:"delimiter"`
	DateFormat      string                            `json:"dateFormat"`
	DecimalSep      string                            `json:"decimalSep"`
	GroupingSep     string                            `json:"groupingSep"`
	Mapping         map[string]string                 `json:"mapping"`
	AccountsMapping map[string]string                 `json:"accountsMapping"`
	HoldingsMapping map[string]string                 `json:"holdingsMapping"`
	Unresolved      []application.CSVUnresolvedAction `json:"unresolved"`
}

type CSVPreviewDTO struct {
	Profile     string                          `json:"profile"`
	Headers     []string                        `json:"headers"`
	PreviewRows [][]string                      `json:"previewRows"`
	Stats       application.CSVPreviewStats     `json:"stats"`
	Errors      []application.CSVRowError       `json:"errors"`
	Warnings    []application.CSVWarning        `json:"warnings"`
	Unresolved  []application.CSVUnresolvedName `json:"unresolved"`
	CanCommit   bool                            `json:"canCommit"`
}

type CSVConfirmDTO struct {
	Confirmed bool `json:"confirmed"`
}

func (s *Service) PreviewCSV(request CSVOptionsRequest) (CSVPreviewDTO, error) {
	if s.app == nil {
		return CSVPreviewDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	view, err := s.app.PreviewCSV(context.Background(), application.CSVPreviewRequest{
		Token: request.Token, Profile: request.Profile, Delimiter: request.Delimiter,
		DateFormat: request.DateFormat, DecimalSep: request.DecimalSep, GroupingSep: request.GroupingSep,
		Mapping: request.Mapping, AccountsMapping: request.AccountsMapping, HoldingsMapping: request.HoldingsMapping,
		Unresolved: request.Unresolved,
	})
	if err != nil {
		return CSVPreviewDTO{}, apierror.Wrap(err)
	}
	return CSVPreviewDTO{
		Profile: view.Profile, Headers: view.Headers, PreviewRows: view.PreviewRows, Stats: view.Stats,
		Errors: view.Errors, Warnings: view.Warnings, Unresolved: view.Unresolved, CanCommit: view.CanCommit,
	}, nil
}

func (s *Service) ConfirmCSV(token string) (CSVConfirmDTO, error) {
	if s.app == nil {
		return CSVConfirmDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	if err := s.app.ConfirmCSV(token); err != nil {
		return CSVConfirmDTO{}, apierror.Wrap(err)
	}
	return CSVConfirmDTO{Confirmed: true}, nil
}

func (s *Service) CommitCSV(token string) (application.CSVPreviewStats, error) {
	if s.app == nil {
		return application.CSVPreviewStats{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	s.refresh.CancelAllAndWait()
	stats, err := s.app.CommitCSV(context.Background(), token)
	if err != nil {
		return application.CSVPreviewStats{}, apierror.Wrap(err)
	}
	return stats, nil
}

type CSVErrorFileDTO struct {
	Cancelled bool `json:"cancelled"`
}

func (s *Service) DownloadCSVErrors(token string) (CSVErrorFileDTO, error) {
	if s.app == nil {
		return CSVErrorFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	data, err := s.app.CSVErrorReport(token)
	if err != nil {
		return CSVErrorFileDTO{}, apierror.Wrap(err)
	}
	if data == nil {
		return CSVErrorFileDTO{Cancelled: true}, nil
	}
	path, err := s.dialogs.SaveFile("Download CSV errors", "Nestworth CSV Errors.csv", "CSV", "*.csv")
	if err != nil || path == "" {
		return CSVErrorFileDTO{Cancelled: true}, nil
	}
	if err := s.app.WriteExportFile(path, data); err != nil {
		return CSVErrorFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the error CSV could not be written"})
	}
	return CSVErrorFileDTO{}, nil
}

func (s *Service) CancelCSV(token string) {
	if s.app != nil {
		s.app.CancelCSV(token)
	}
}

func (s *Service) Shutdown() {
	if s != nil && s.app != nil {
		s.app.ShutdownCSV()
	}
}
