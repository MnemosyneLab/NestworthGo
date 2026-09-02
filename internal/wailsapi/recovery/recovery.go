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

const (
	// pendingRestoreTTL is the documented lifetime of a Restore preview token
	// and its staged package. Inspect, confirm, and shutdown expire entries
	// that are older than this.
	pendingRestoreTTL         = 15 * time.Minute
	maxPendingRestoreSessions = 2
	maxPendingRestoreBytes    = backup.MaxMemberBytes
)

type Service struct {
	liveDBPath string
	app        *application.Service
	store      *settings.Store
	dialogs    native.Dialogs
	quit       native.Quitter
	refresh    native.RefreshGate
	now        func() time.Time

	mu      sync.Mutex
	pending map[string]pendingRestore
}

type pendingRestore struct {
	stagedPath  string
	fileName    string
	fingerprint string
	createdAt   time.Time
	sizeBytes   int64
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
	return &Service{liveDBPath: liveDBPath, app: app, store: store, dialogs: dialogs, quit: quit, refresh: refresh, now: time.Now, pending: map[string]pendingRestore{}}
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
	s.expireLocked(s.clock())
	if err := s.retainLocked(int64(len(pkg.Database))); err != nil {
		s.mu.Unlock()
		return RestorePreviewDTO{}, apierror.Wrap(err)
	}
	stagedPath := filepath.Join(dir, ".nestworth-restore-"+dto.Token+backup.FileExt)
	if err := backup.WritePackage(stagedPath, pkg); err != nil {
		s.mu.Unlock()
		return RestorePreviewDTO{}, apierror.Wrap(err)
	}
	fingerprint := ""
	if meta, ok := pkg.Manifest.Members[backup.MemberDatabase]; ok {
		fingerprint = meta.SHA256
	}
	s.pending[dto.Token] = pendingRestore{
		stagedPath:  stagedPath,
		fileName:    dto.FileName,
		fingerprint: fingerprint,
		createdAt:   s.clock(),
		sizeBytes:   int64(len(pkg.Database)),
	}
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
	s.expireLocked(s.clock())
	pending, ok := s.pending[request.Token]
	if ok {
		delete(s.pending, request.Token)
	}
	s.mu.Unlock()
	if !ok {
		return RestoreResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrNotFound, Message: "restore preview expired"})
	}
	defer os.Remove(pending.stagedPath)
	pkg, err := backup.ReadPackage(pending.stagedPath)
	if err != nil {
		return RestoreResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrNotFound, Message: "restore preview expired"})
	}
	if meta, ok := pkg.Manifest.Members[backup.MemberDatabase]; !ok || !strings.EqualFold(meta.SHA256, pending.fingerprint) {
		return RestoreResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrBackupChecksumFailed, Message: "restore preview expired"})
	}
	if s.app == nil {
		s.refresh.CancelAllAndWait()
		err := backup.InstallRestore(context.Background(), s.liveDBPath, pkg, &backup.SessionHooks{}, backup.RestoreClasses{
			Chrome: request.RestoreChrome, Format: request.RestoreFormat, Routing: request.RestoreRouting,
		}, time.Now())
		if err != nil {
			return RestoreResultDTO{}, apierror.Wrap(err)
		}
		go s.quit.Quit()
		return RestoreResultDTO{RestartRequired: true}, nil
	}
	var result RestoreResultDTO
	err = s.app.WithExclusiveKeep(context.Background(), application.ExclusiveRestore, func(ctx context.Context) (bool, error) {
		s.refresh.CancelAllAndWait()
		var unlocked bool
		unlock := func() {
			if !unlocked {
				s.app.UnlockWrites()
				unlocked = true
			}
		}
		sessionClosed := false
		hooks := &backup.SessionHooks{}
		s.app.LockWrites()
		hooks.Quiesce = func(context.Context) error { return nil }
		hooks.Checkpoint = s.app.CheckpointWAL
		hooks.Close = func() error {
			closeErr := s.app.CloseDatabase()
			if closeErr == nil {
				sessionClosed = true
			}
			return closeErr
		}
		installErr := backup.InstallRestore(ctx, s.liveDBPath, pkg, hooks, backup.RestoreClasses{
			Chrome: request.RestoreChrome, Format: request.RestoreFormat, Routing: request.RestoreRouting,
		}, time.Now())
		if installErr != nil {
			if sessionClosed {
				go s.quit.Quit()
				result = RestoreResultDTO{RestartRequired: true}
				return true, installErr
			}
			unlock()
			return false, installErr
		}
		go s.quit.Quit()
		result = RestoreResultDTO{RestartRequired: true}
		return true, nil
	})
	if err != nil {
		return result, apierror.Wrap(err)
	}
	return result, nil
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

func (s *Service) clock() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

// Shutdown deletes staged Restore packages. It is safe to call more than once.
func (s *Service) Shutdown() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for token := range s.pending {
		s.dropLocked(token)
	}
}

func (s *Service) expireLocked(now time.Time) {
	for token, pending := range s.pending {
		if now.Sub(pending.createdAt) >= pendingRestoreTTL {
			s.dropLocked(token)
		}
	}
}

func (s *Service) retainLocked(extraBytes int64) error {
	for {
		if len(s.pending) < maxPendingRestoreSessions && s.pendingBytesLocked()+extraBytes <= maxPendingRestoreBytes {
			return nil
		}
		if len(s.pending) == 0 {
			return &domain.Error{Code: domain.ErrUnavailable, Message: "restore preview could not be retained"}
		}
		s.dropLocked(oldestPendingToken(s.pending))
	}
}

func (s *Service) pendingBytesLocked() int64 {
	var total int64
	for _, pending := range s.pending {
		total += pending.sizeBytes
	}
	return total
}

func (s *Service) dropLocked(token string) {
	pending, ok := s.pending[token]
	if !ok {
		return
	}
	delete(s.pending, token)
	if pending.stagedPath != "" {
		_ = os.Remove(pending.stagedPath)
	}
}

func oldestPendingToken(pending map[string]pendingRestore) string {
	var token string
	var created time.Time
	for key, item := range pending {
		if token == "" || item.createdAt.Before(created) {
			token = key
			created = item.createdAt
		}
	}
	return token
}
