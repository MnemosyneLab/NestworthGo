package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	RestorePreviewTTL         = 15 * time.Minute
	maxPendingRestoreSessions = 2
)

type RestorePreview struct {
	Token             string
	FileName          string
	CreatedAt         string
	AppVersion        string
	AppBuild          string
	SchemaVersion     int
	FormatVersion     int
	BackupHousehold   string
	BackupCurrency    string
	BackupAccounts    int
	BackupHoldings    int
	BackupActivities  int
	CurrentHousehold  string
	CurrentCurrency   string
	CurrentAccounts   int
	CurrentHoldings   int
	CurrentActivities int
	SettingsReadable  bool
	HasOpenSession    bool
}

type RestoreConfirmInput struct {
	Token          string
	Confirmation   string
	Acknowledged   bool
	RestoreChrome  bool
	RestoreFormat  bool
	RestoreRouting bool
}

type RestoreResult struct {
	RestartRequired bool
	KeepExclusive   bool
}

type pendingRestore struct {
	stagedPath  string
	fileName    string
	fingerprint string
	createdAt   time.Time
	sizeBytes   int64
}

// Recovery owns backup inspect/confirm sessions and restore orchestration.
// It can run without a live *Service so blocked startup can still Restore.
type Recovery struct {
	liveDBPath string
	backup     BackupRuntime
	app        *Service
	now        func() time.Time

	mu      sync.Mutex
	pending map[string]pendingRestore
}

func NewRecovery(liveDBPath string, runtime BackupRuntime, app *Service) *Recovery {
	return &Recovery{
		liveDBPath: liveDBPath,
		backup:     runtime,
		app:        app,
		now:        time.Now,
		pending:    map[string]pendingRestore{},
	}
}

func (r *Recovery) InspectBackup(ctx context.Context, sourcePath string) (RestorePreview, error) {
	if r == nil || r.backup == nil {
		return RestorePreview{}, &domain.Error{Code: domain.ErrUnavailable, Message: "backup runtime is not configured"}
	}
	pkg, err := r.backup.ReadPackage(sourcePath)
	if err != nil {
		return RestorePreview{}, err
	}
	dir := filepath.Dir(r.liveDBPath)
	inspectPath := filepath.Join(dir, ".nestworth-inspect-"+randomToken()+".sqlite")
	if err := r.backup.ExtractDatabase(pkg, inspectPath); err != nil {
		return RestorePreview{}, err
	}
	defer os.Remove(inspectPath)
	if err := r.backup.VerifyExtracted(ctx, inspectPath); err != nil {
		return RestorePreview{}, err
	}
	summary, counts, err := r.backup.InspectDatabase(ctx, inspectPath)
	if err != nil {
		return RestorePreview{}, err
	}
	preview := RestorePreview{
		Token:            randomToken(),
		FileName:         filepath.Base(sourcePath),
		CreatedAt:        pkg.CreatedAt,
		AppVersion:       pkg.AppVersion,
		AppBuild:         pkg.AppBuild,
		SchemaVersion:    pkg.SchemaVersion,
		FormatVersion:    pkg.FormatVersion,
		BackupHousehold:  summary.Name,
		BackupCurrency:   summary.BaseCurrency,
		BackupAccounts:   counts.Accounts,
		BackupHoldings:   counts.Holdings,
		BackupActivities: counts.Activities,
		HasOpenSession:   r.app != nil,
		SettingsReadable: jsonLooksLikeSettings(pkg.Settings),
	}
	if r.app != nil {
		current, previewErr := r.app.CurrentDatabasePreview(ctx)
		if previewErr == nil {
			preview.CurrentHousehold = current.HouseholdName
			preview.CurrentCurrency = current.BaseCurrency
			preview.CurrentAccounts = current.Accounts
			preview.CurrentHoldings = current.Holdings
			preview.CurrentActivities = current.Activities
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(r.clock())
	if err := r.retainLocked(int64(len(pkg.Database))); err != nil {
		return RestorePreview{}, err
	}
	stagedPath := filepath.Join(dir, ".nestworth-restore-"+preview.Token+r.backup.FileExt())
	if err := r.backup.WritePackage(stagedPath, pkg); err != nil {
		return RestorePreview{}, err
	}
	r.pending[preview.Token] = pendingRestore{
		stagedPath:  stagedPath,
		fileName:    preview.FileName,
		fingerprint: pkg.DatabaseSHA256,
		createdAt:   r.clock(),
		sizeBytes:   int64(len(pkg.Database)),
	}
	return preview, nil
}

func (r *Recovery) ConfirmRestore(ctx context.Context, input RestoreConfirmInput) (RestoreResult, error) {
	if !input.Acknowledged || !strings.EqualFold(strings.TrimSpace(input.Confirmation), "RESTORE") {
		return RestoreResult{}, &domain.Error{Code: domain.ErrBackupRestoreConfirmation, Message: "restore confirmation is required"}
	}
	r.mu.Lock()
	r.expireLocked(r.clock())
	pending, ok := r.pending[input.Token]
	if ok {
		delete(r.pending, input.Token)
	}
	r.mu.Unlock()
	if !ok {
		return RestoreResult{}, &domain.Error{Code: domain.ErrNotFound, Message: "restore preview expired"}
	}
	defer os.Remove(pending.stagedPath)
	pkg, err := r.backup.ReadPackage(pending.stagedPath)
	if err != nil {
		return RestoreResult{}, &domain.Error{Code: domain.ErrNotFound, Message: "restore preview expired"}
	}
	if !strings.EqualFold(pkg.DatabaseSHA256, pending.fingerprint) {
		return RestoreResult{}, &domain.Error{Code: domain.ErrBackupChecksumFailed, Message: "restore preview expired"}
	}
	classes := RestoreClasses{Chrome: input.RestoreChrome, Format: input.RestoreFormat, Routing: input.RestoreRouting}
	if r.app == nil {
		err := r.backup.InstallRestore(ctx, r.liveDBPath, pkg, RestoreHooks{}, classes, r.clock())
		if err != nil {
			return RestoreResult{}, err
		}
		return RestoreResult{RestartRequired: true}, nil
	}
	var result RestoreResult
	err = r.app.WithExclusiveKeep(ctx, ExclusiveRestore, func(ctx context.Context) (bool, error) {
		var unlocked bool
		unlock := func() {
			if !unlocked {
				r.app.UnlockWrites()
				unlocked = true
			}
		}
		sessionClosed := false
		hooks := RestoreHooks{}
		r.app.LockWrites()
		hooks.Quiesce = func(context.Context) error { return nil }
		hooks.Checkpoint = r.app.CheckpointWAL
		hooks.Close = func() error {
			closeErr := r.app.CloseDatabase()
			if closeErr == nil {
				sessionClosed = true
			}
			return closeErr
		}
		installErr := r.backup.InstallRestore(ctx, r.liveDBPath, pkg, hooks, classes, r.clock())
		if installErr != nil {
			if sessionClosed {
				result = RestoreResult{RestartRequired: true, KeepExclusive: true}
				return true, installErr
			}
			unlock()
			return false, installErr
		}
		result = RestoreResult{RestartRequired: true, KeepExclusive: true}
		return true, nil
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func (r *Recovery) Shutdown() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for token, pending := range r.pending {
		_ = os.Remove(pending.stagedPath)
		delete(r.pending, token)
	}
}

func (r *Recovery) clock() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now()
}

// SetClock replaces the restore-session clock. Tests use it to exercise TTL.
func (r *Recovery) SetClock(now func() time.Time) {
	if r == nil {
		return
	}
	r.now = now
}

func (r *Recovery) expireLocked(now time.Time) {
	for token, pending := range r.pending {
		if now.Sub(pending.createdAt) >= RestorePreviewTTL {
			_ = os.Remove(pending.stagedPath)
			delete(r.pending, token)
		}
	}
}

func (r *Recovery) retainLocked(extraBytes int64) error {
	maxBytes := int64(1 << 30)
	if r.backup != nil {
		maxBytes = r.backup.MaxPackageBytes()
	}
	for {
		if len(r.pending) < maxPendingRestoreSessions && r.bytesLocked()+extraBytes <= maxBytes {
			return nil
		}
		if len(r.pending) == 0 {
			return &domain.Error{Code: domain.ErrUnavailable, Message: "restore preview could not be retained"}
		}
		var oldest string
		var created time.Time
		for token, pending := range r.pending {
			if oldest == "" || pending.createdAt.Before(created) {
				oldest = token
				created = pending.createdAt
			}
		}
		_ = os.Remove(r.pending[oldest].stagedPath)
		delete(r.pending, oldest)
	}
}

func (r *Recovery) bytesLocked() int64 {
	var total int64
	for _, pending := range r.pending {
		total += pending.sizeBytes
	}
	return total
}

func jsonLooksLikeSettings(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	return strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, "schema_version")
}

func randomToken() string {
	var entropy [8]byte
	_, _ = rand.Read(entropy[:])
	return hex.EncodeToString(entropy[:])
}
