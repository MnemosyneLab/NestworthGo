package backup

import (
	"context"
	"os"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Runtime implements application.BackupRuntime.
type Runtime struct{}

func NewRuntime() Runtime { return Runtime{} }

func (Runtime) FileExt() string { return FileExt }

func (Runtime) MaxPackageBytes() int64 { return MaxMemberBytes }

func (Runtime) DefaultFileName(now time.Time) string { return DefaultBackupFileName(now) }

func (Runtime) WriteAtomic(path string, data []byte, perm os.FileMode) error {
	return WriteAtomicFile(path, data, perm)
}

func (Runtime) WritePackage(path string, pkg application.BackupPackage) error {
	return WritePackage(path, toPackage(pkg))
}

func (Runtime) ReadPackage(path string) (application.BackupPackage, error) {
	pkg, err := ReadPackage(path)
	if err != nil {
		return application.BackupPackage{}, err
	}
	return fromPackage(pkg), nil
}

func (Runtime) WriteStatus(liveDBPath string, status application.BackupStatus) error {
	return WriteStatus(liveDBPath, Status{
		FormatVersion:      FormatVersion,
		BackupFileName:     status.BackupFileName,
		CreatedAt:          status.CreatedAt,
		SchemaVersion:      status.SchemaVersion,
		AppVersion:         status.AppVersion,
		AppBuild:           status.AppBuild,
		VerificationResult: status.VerificationResult,
	})
}

func (Runtime) ReadStatus(liveDBPath string) (application.BackupStatus, error) {
	status, err := ReadStatus(liveDBPath)
	if err != nil {
		return application.BackupStatus{}, err
	}
	return application.BackupStatus{
		BackupFileName:     status.BackupFileName,
		CreatedAt:          status.CreatedAt,
		SchemaVersion:      status.SchemaVersion,
		AppVersion:         status.AppVersion,
		AppBuild:           status.AppBuild,
		VerificationResult: status.VerificationResult,
	}, nil
}

func (Runtime) InspectDatabase(ctx context.Context, sqlitePath string) (application.HouseholdPreview, application.EntityCounts, error) {
	verified, err := sqlite.OpenReadOnlyForVerify(sqlitePath)
	if err != nil {
		return application.HouseholdPreview{}, application.EntityCounts{}, err
	}
	defer verified.Close()
	summary, err := verified.HouseholdSummary(ctx)
	if err != nil {
		return application.HouseholdPreview{}, application.EntityCounts{}, err
	}
	counts, err := verified.EntityCounts(ctx)
	if err != nil {
		return application.HouseholdPreview{}, application.EntityCounts{}, err
	}
	return application.HouseholdPreview{Name: summary.Name, BaseCurrency: summary.BaseCurrency}, application.EntityCounts{
		Households: counts.Households, Accounts: counts.Accounts, Holdings: counts.Holdings,
		Activities: counts.Activities, Instruments: counts.Instruments, Members: counts.Members,
	}, nil
}

func (Runtime) VerifyExtracted(ctx context.Context, sqlitePath string) error {
	return VerifyExtractedDatabase(ctx, sqlitePath)
}

func (Runtime) ExtractDatabase(pkg application.BackupPackage, dest string) error {
	return ExtractDatabase(toPackage(pkg), dest)
}

func (Runtime) InstallRestore(ctx context.Context, liveDBPath string, pkg application.BackupPackage, hooks application.RestoreHooks, classes application.RestoreClasses, now time.Time) error {
	return InstallRestore(ctx, liveDBPath, toPackage(pkg), &SessionHooks{
		Quiesce: hooks.Quiesce, Checkpoint: hooks.Checkpoint, Close: hooks.Close,
	}, RestoreClasses{Chrome: classes.Chrome, Format: classes.Format, Routing: classes.Routing}, now)
}

func toPackage(pkg application.BackupPackage) Package {
	counts := sqlite.EntityCounts{
		Households: pkg.Counts.Households, Accounts: pkg.Counts.Accounts, Holdings: pkg.Counts.Holdings,
		Activities: pkg.Counts.Activities, Instruments: pkg.Counts.Instruments, Members: pkg.Counts.Members,
	}
	manifest := NewManifest(time.Now().UTC(), pkg.Database, pkg.Settings, counts)
	if pkg.CreatedAt != "" {
		manifest.CreatedAt = pkg.CreatedAt
	}
	if pkg.AppVersion != "" {
		manifest.AppVersion = pkg.AppVersion
	}
	if pkg.AppBuild != "" {
		manifest.AppBuild = pkg.AppBuild
	}
	if pkg.SchemaVersion != 0 {
		manifest.SchemaVersion = pkg.SchemaVersion
	}
	if pkg.FormatVersion != 0 {
		manifest.FormatVersion = pkg.FormatVersion
	}
	if pkg.DatabaseSHA256 != "" {
		meta := manifest.Members[MemberDatabase]
		meta.SHA256 = pkg.DatabaseSHA256
		manifest.Members[MemberDatabase] = meta
	}
	return Package{Manifest: manifest, Database: pkg.Database, Settings: pkg.Settings}
}

func fromPackage(pkg Package) application.BackupPackage {
	sha := ""
	if meta, ok := pkg.Manifest.Members[MemberDatabase]; ok {
		sha = meta.SHA256
	}
	return application.BackupPackage{
		CreatedAt:      pkg.Manifest.CreatedAt,
		AppVersion:     pkg.Manifest.AppVersion,
		AppBuild:       pkg.Manifest.AppBuild,
		SchemaVersion:  pkg.Manifest.SchemaVersion,
		FormatVersion:  pkg.Manifest.FormatVersion,
		Database:       pkg.Database,
		Settings:       pkg.Settings,
		DatabaseSHA256: sha,
		Counts: application.EntityCounts{
			Households: pkg.Manifest.RowCounts.Households, Accounts: pkg.Manifest.RowCounts.Accounts,
			Holdings: pkg.Manifest.RowCounts.Holdings, Activities: pkg.Manifest.RowCounts.Activities,
			Instruments: pkg.Manifest.RowCounts.Instruments, Members: pkg.Manifest.RowCounts.Members,
		},
	}
}
