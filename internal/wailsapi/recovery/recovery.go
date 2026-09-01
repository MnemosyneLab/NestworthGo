package recovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/native"
)

type Service struct {
	liveDBPath string
	app        *application.Service
	store      *settings.Store
	dialogs    native.Dialogs
	quit       native.Quitter
	refresh    native.RefreshGate

	mu      sync.Mutex
	pending map[string]pendingRestore
}

type pendingRestore struct {
	pkg      backup.Package
	fileName string
}

func NewService(liveDBPath string, app *application.Service, store *settings.Store, dialogs native.Dialogs, quit native.Quitter, refresh native.RefreshGate) *Service {
	if dialogs == nil {
		dialogs = native.NoopDialogs{}
	}
	if quit == nil {
		quit = native.NoopQuitter{}
	}
	if refresh == nil {
		refresh = native.NoopRefresh{}
	}
	return &Service{liveDBPath: liveDBPath, app: app, store: store, dialogs: dialogs, quit: quit, refresh: refresh, pending: map[string]pendingRestore{}}
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
	pkg, err := backup.ReadPackage(path)
	if err != nil {
		return RestorePreviewDTO{}, apierror.Wrap(err)
	}
	dir := filepath.Dir(s.liveDBPath)
	inspectPath := filepath.Join(dir, ".nestworth-inspect-"+randomSuffix()+".sqlite")
	if err := backup.ExtractDatabase(pkg, inspectPath); err != nil {
		return RestorePreviewDTO{}, apierror.Wrap(err)
	}
	defer os.Remove(inspectPath)
	if err := backup.VerifyExtractedDatabase(context.Background(), inspectPath); err != nil {
		return RestorePreviewDTO{}, apierror.Wrap(err)
	}
	verified, err := sqlite.OpenReadOnlyForVerify(inspectPath)
	if err != nil {
		return RestorePreviewDTO{}, apierror.Wrap(err)
	}
	ctx := context.Background()
	summary, err := verified.HouseholdSummary(ctx)
	if err != nil {
		_ = verified.Close()
		return RestorePreviewDTO{}, apierror.Wrap(err)
	}
	counts, err := verified.EntityCounts(ctx)
	if err != nil {
		_ = verified.Close()
		return RestorePreviewDTO{}, apierror.Wrap(err)
	}
	_ = verified.Close()
	dto := RestorePreviewDTO{
		Token:            newToken(),
		FileName:         filepath.Base(path),
		CreatedAt:        pkg.Manifest.CreatedAt,
		AppVersion:       pkg.Manifest.AppVersion,
		AppBuild:         pkg.Manifest.AppBuild,
		SchemaVersion:    pkg.Manifest.SchemaVersion,
		FormatVersion:    pkg.Manifest.FormatVersion,
		BackupHousehold:  summary.Name,
		BackupCurrency:   summary.BaseCurrency,
		BackupAccounts:   counts.Accounts,
		BackupHoldings:   counts.Holdings,
		BackupActivities: counts.Activities,
		HasOpenSession:   s.app != nil,
		SettingsReadable: jsonLooksLikeSettings(pkg.Settings),
	}
	if s.app != nil {
		current, previewErr := s.app.CurrentDatabasePreview(ctx)
		if previewErr == nil {
			dto.CurrentHousehold = current.HouseholdName
			dto.CurrentCurrency = current.BaseCurrency
			dto.CurrentAccounts = current.Accounts
			dto.CurrentHoldings = current.Holdings
			dto.CurrentActivities = current.Activities
		}
	}
	s.mu.Lock()
	s.pending[dto.Token] = pendingRestore{pkg: pkg, fileName: dto.FileName}
	s.mu.Unlock()
	return dto, nil
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
	if !request.Acknowledged || !strings.EqualFold(strings.TrimSpace(request.Confirmation), "RESTORE") {
		return RestoreResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrBackupRestoreConfirmation, Message: "restore confirmation is required"})
	}
	s.mu.Lock()
	pending, ok := s.pending[request.Token]
	if ok {
		delete(s.pending, request.Token)
	}
	s.mu.Unlock()
	if !ok {
		return RestoreResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrNotFound, Message: "restore preview expired"})
	}
	if s.app != nil {
		if err := s.app.BeginExclusiveOperation(); err != nil {
			return RestoreResultDTO{}, apierror.Wrap(err)
		}
	}
	s.refresh.CancelAllAndWait()
	var unlocked bool
	unlock := func() {
		if s.app != nil && !unlocked {
			s.app.UnlockWrites()
			unlocked = true
		}
	}
	sessionClosed := false
	hooks := &backup.SessionHooks{}
	if s.app != nil {
		s.app.LockWrites()
		hooks.Quiesce = func(context.Context) error { return nil }
		hooks.Checkpoint = s.app.CheckpointWAL
		hooks.Close = func() error {
			err := s.app.CloseDatabase()
			if err == nil {
				sessionClosed = true
			}
			return err
		}
	}
	err := backup.InstallRestore(context.Background(), s.liveDBPath, pending.pkg, hooks, backup.RestoreClasses{
		Chrome: request.RestoreChrome, Format: request.RestoreFormat, Routing: request.RestoreRouting,
	}, time.Now())
	if err != nil {
		if sessionClosed {
			go s.quit.Quit()
			return RestoreResultDTO{RestartRequired: true}, apierror.Wrap(err)
		}
		unlock()
		if s.app != nil {
			s.app.EndExclusiveOperation()
		}
		return RestoreResultDTO{}, apierror.Wrap(err)
	}
	go s.quit.Quit()
	return RestoreResultDTO{RestartRequired: true}, nil
}

func jsonLooksLikeSettings(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	return strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, "schema_version")
}

func newToken() string {
	return randomSuffix()
}

func randomSuffix() string {
	var entropy [8]byte
	_, _ = rand.Read(entropy[:])
	return hex.EncodeToString(entropy[:])
}
