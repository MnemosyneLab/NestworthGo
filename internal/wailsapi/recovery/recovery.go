package recovery

import (
	"context"
	"strings"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/native"
)

type Service struct {
	recovery *application.Recovery
	dialogs  native.Dialogs
	quit     native.Quitter
	refresh  native.RefreshGate
}

func NewService(recovery *application.Recovery, dialogs native.Dialogs, quit native.Quitter, refresh native.RefreshGate) *Service {
	if dialogs == nil {
		dialogs = native.NoopDialogs{}
	}
	if quit == nil {
		quit = native.NoopQuitter{}
	}
	if refresh == nil {
		refresh = native.NoopRefresh{}
	}
	return &Service{recovery: recovery, dialogs: dialogs, quit: quit, refresh: refresh}
}

type RestorePreviewDTO struct {
	Cancelled         bool   `json:"cancelled"`
	Token             string `json:"token,omitempty"`
	FileName          string `json:"fileName,omitempty"`
	CreatedAt         string `json:"createdAt,omitempty"`
	AppVersion        string `json:"appVersion,omitempty"`
	AppBuild          string `json:"appBuild,omitempty"`
	SchemaVersion     int    `json:"schemaVersion,omitempty"`
	FormatVersion     int    `json:"formatVersion,omitempty"`
	BackupHousehold   string `json:"backupHousehold,omitempty"`
	BackupCurrency    string `json:"backupCurrency,omitempty"`
	BackupAccounts    int    `json:"backupAccounts"`
	BackupHoldings    int    `json:"backupHoldings"`
	BackupActivities  int    `json:"backupActivities"`
	CurrentHousehold  string `json:"currentHousehold,omitempty"`
	CurrentCurrency   string `json:"currentCurrency,omitempty"`
	CurrentAccounts   int    `json:"currentAccounts"`
	CurrentHoldings   int    `json:"currentHoldings"`
	CurrentActivities int    `json:"currentActivities"`
	SettingsReadable  bool   `json:"settingsReadable"`
	HasOpenSession    bool   `json:"hasOpenSession"`
}

func (s *Service) InspectBackup() (RestorePreviewDTO, error) {
	path, err := s.dialogs.OpenFile("Restore from backup", "Nestworth Backup", "*.nestworth-backup")
	if err != nil {
		return RestorePreviewDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the backup file could not be selected"})
	}
	if path == "" {
		return RestorePreviewDTO{Cancelled: true}, nil
	}
	if s.recovery == nil {
		return RestorePreviewDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "recovery is not available"})
	}
	preview, err := s.recovery.InspectBackup(context.Background(), path)
	if err != nil {
		return RestorePreviewDTO{}, apierror.Wrap(err)
	}
	return RestorePreviewDTO{
		Token: preview.Token, FileName: preview.FileName, CreatedAt: preview.CreatedAt,
		AppVersion: preview.AppVersion, AppBuild: preview.AppBuild, SchemaVersion: preview.SchemaVersion,
		FormatVersion: preview.FormatVersion, BackupHousehold: preview.BackupHousehold, BackupCurrency: preview.BackupCurrency,
		BackupAccounts: preview.BackupAccounts, BackupHoldings: preview.BackupHoldings, BackupActivities: preview.BackupActivities,
		CurrentHousehold: preview.CurrentHousehold, CurrentCurrency: preview.CurrentCurrency, CurrentAccounts: preview.CurrentAccounts,
		CurrentHoldings: preview.CurrentHoldings, CurrentActivities: preview.CurrentActivities,
		SettingsReadable: preview.SettingsReadable, HasOpenSession: preview.HasOpenSession,
	}, nil
}

type RestoreConfirmRequest struct {
	Token          string `json:"token"`
	Confirmation   string `json:"confirmation"`
	Acknowledged   bool   `json:"acknowledged"`
	RestoreChrome  bool   `json:"restoreChrome"`
	RestoreFormat  bool   `json:"restoreFormat"`
	RestoreRouting bool   `json:"restoreRouting"`
}

type RestoreResultDTO struct {
	RestartRequired bool `json:"restartRequired"`
}

func (s *Service) ConfirmRestore(request RestoreConfirmRequest) (RestoreResultDTO, error) {
	if s.recovery == nil {
		return RestoreResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "recovery is not available"})
	}
	if !request.Acknowledged || !strings.EqualFold(strings.TrimSpace(request.Confirmation), "RESTORE") {
		return RestoreResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrBackupRestoreConfirmation, Message: "restore confirmation is required"})
	}
	s.refresh.CancelAllAndWait()
	result, err := s.recovery.ConfirmRestore(context.Background(), application.RestoreConfirmInput{
		Token: request.Token, Confirmation: request.Confirmation, Acknowledged: request.Acknowledged,
		RestoreChrome: request.RestoreChrome, RestoreFormat: request.RestoreFormat, RestoreRouting: request.RestoreRouting,
	})
	if result.RestartRequired {
		go s.quit.Quit()
	}
	if err != nil {
		return RestoreResultDTO{RestartRequired: result.RestartRequired}, apierror.Wrap(err)
	}
	return RestoreResultDTO{RestartRequired: result.RestartRequired}, nil
}

func (s *Service) Shutdown() {
	if s != nil && s.recovery != nil {
		s.recovery.Shutdown()
	}
}
