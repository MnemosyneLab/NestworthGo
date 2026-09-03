package application

import (
	"context"
	"os"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const BackupFileExt = ".nestworth-backup"

type EntityCounts struct {
	Households  int `json:"households"`
	Accounts    int `json:"accounts"`
	Holdings    int `json:"holdings"`
	Activities  int `json:"activities"`
	Instruments int `json:"instruments"`
	Members     int `json:"members"`
}

type HouseholdPreview struct {
	Name         string
	BaseCurrency string
}

type BackupPackage struct {
	CreatedAt      string
	AppVersion     string
	AppBuild       string
	SchemaVersion  int
	FormatVersion  int
	Database       []byte
	Settings       []byte
	Counts         EntityCounts
	DatabaseSHA256 string
}

type BackupStatus struct {
	BackupFileName     string
	CreatedAt          string
	SchemaVersion      int
	AppVersion         string
	AppBuild           string
	VerificationResult string
}

type RestoreClasses struct {
	Chrome  bool
	Format  bool
	Routing bool
}

type RestoreHooks struct {
	Quiesce    func(context.Context) error
	Checkpoint func(context.Context) error
	Close      func() error
}

type BackupCreateResult struct {
	FileName           string
	CreatedAt          string
	SchemaVersion      int
	AppVersion         string
	AppBuild           string
	Accounts           int
	Holdings           int
	Activities         int
	VerificationResult string
}

// BackupRuntime is the application port for backup packaging, restore
// install, status sidecars, and read-only inspect of extracted databases.
type BackupRuntime interface {
	FileExt() string
	MaxPackageBytes() int64
	DefaultFileName(now time.Time) string
	WriteAtomic(path string, data []byte, perm os.FileMode) error
	WritePackage(path string, pkg BackupPackage) error
	ReadPackage(path string) (BackupPackage, error)
	WriteStatus(liveDBPath string, status BackupStatus) error
	ReadStatus(liveDBPath string) (BackupStatus, error)
	InspectDatabase(ctx context.Context, sqlitePath string) (HouseholdPreview, EntityCounts, error)
	VerifyExtracted(ctx context.Context, sqlitePath string) error
	ExtractDatabase(pkg BackupPackage, dest string) error
	InstallRestore(ctx context.Context, liveDBPath string, pkg BackupPackage, hooks RestoreHooks, classes RestoreClasses, now time.Time) error
}

func (s *Service) SetBackup(runtime BackupRuntime) {
	s.stateMu.Lock()
	s.backup = runtime
	s.stateMu.Unlock()
}

func (s *Service) SetLiveDatabasePath(path string) {
	s.stateMu.Lock()
	s.liveDBPath = path
	s.stateMu.Unlock()
}

func (s *Service) backupRuntime() BackupRuntime {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.backup
}

func (s *Service) livePath() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.liveDBPath
}

func (s *Service) requireBackup() (BackupRuntime, error) {
	runtime := s.backupRuntime()
	if runtime == nil {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "backup runtime is not configured"}
	}
	return runtime, nil
}
