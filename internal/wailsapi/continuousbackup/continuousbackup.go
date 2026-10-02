// Package continuousbackup exposes explicit UI operations only. Neither cloud
// credentials nor restore controls are added to the portable skill or MCP.
package continuousbackup

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	infra "github.com/waltwang/nestworth-go/internal/infrastructure/continuousbackup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/recovery"
)

type Service struct {
	manager   *infra.Manager
	app       *application.Service
	inspector *application.Recovery
	recovery  *recovery.Service
}

func NewService(m *infra.Manager, app *application.Service, inspector *application.Recovery, r *recovery.Service) *Service {
	return &Service{manager: m, app: app, inspector: inspector, recovery: r}
}
func safe(err error) error {
	if err == nil {
		return nil
	}
	// Preserve validation codes from the shared restore inspector; infrastructure
	// errors never carry provider responses, SQL, paths, or credential values.
	if e, ok := err.(*domain.Error); ok {
		return apierror.Wrap(e)
	}
	code := domain.ErrUnavailable
	if err == infra.ErrConfiguration {
		code = domain.ErrValidation
	}
	message := "continuous backup is unavailable; check the configuration and connection"
	if err == infra.ErrConfiguration {
		message = infra.ErrConfiguration.Error()
	}
	return apierror.Wrap(&domain.Error{Code: code, Message: message})
}
func (s *Service) Status() (infra.View, error) {
	if s.manager == nil {
		return infra.View{}, safe(infra.ErrUnavailable)
	}
	v, err := s.manager.View()
	return v, safe(err)
}
func (s *Service) Configure(ctx context.Context, u infra.Update) (infra.View, error) {
	if s.manager == nil {
		return infra.View{}, safe(infra.ErrUnavailable)
	}
	apply := func(ctx context.Context) error { return s.manager.Configure(ctx, u) }
	var err error
	if s.app != nil {
		err = s.app.WithExclusive(ctx, application.ExclusiveBackup, apply)
	} else {
		err = apply(ctx)
	}
	if err != nil {
		return infra.View{}, safe(err)
	}
	return s.Status()
}
func (s *Service) TestConnection(ctx context.Context) error {
	if s.manager == nil {
		return safe(infra.ErrUnavailable)
	}
	return safe(s.manager.TestConnection(ctx))
}
func (s *Service) BackupNow(ctx context.Context) error {
	if s.manager == nil {
		return safe(infra.ErrUnavailable)
	}
	return safe(s.manager.BackupNow(ctx))
}
func (s *Service) RecoveryPoints(ctx context.Context) ([]infra.RecoveryPoint, error) {
	if s.manager == nil {
		return nil, safe(infra.ErrUnavailable)
	}
	points, err := s.manager.RecoveryPoints(ctx)
	return points, safe(err)
}

// InspectRestore downloads separately and feeds a verified schema-15 candidate
// through the existing token-based preview/journal path. It cannot install it.
func (s *Service) InspectRestore(ctx context.Context, p infra.RecoveryPoint) (recovery.RestorePreviewDTO, error) {
	if s.manager == nil || s.inspector == nil {
		return recovery.RestorePreviewDTO{}, safe(infra.ErrUnavailable)
	}
	candidate, err := s.manager.Stage(ctx, p)
	if err != nil {
		return recovery.RestorePreviewDTO{}, safe(err)
	}
	path := candidate.Path
	defer os.RemoveAll(filepath.Dir(path))
	verified, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		return recovery.RestorePreviewDTO{}, safe(err)
	}
	counts, err := verified.EntityCounts(ctx)
	verified.Close()
	if err != nil {
		return recovery.RestorePreviewDTO{}, safe(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > backup.MaxMemberBytes {
		return recovery.RestorePreviewDTO{}, safe(infra.ErrUnavailable)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return recovery.RestorePreviewDTO{}, safe(infra.ErrUnavailable)
	}
	pkg := backup.Package{Manifest: backup.NewManifest(time.Now().UTC(), data, []byte("{}\n"), counts), Database: data, Settings: []byte("{}\n")}
	pkg.Manifest.AppVersion = candidate.AppVersion
	pkg.Manifest.AppBuild = candidate.AppBuild
	if captured, parseErr := time.Parse(time.RFC3339Nano, p.CapturedAt); parseErr == nil {
		pkg.Manifest.CreatedAt = captured.UTC().Format(time.RFC3339Nano)
	}
	packagePath := filepath.Join(filepath.Dir(path), "cloud-recovery.nestworth-backup")
	if err := backup.WritePackage(packagePath, pkg); err != nil {
		return recovery.RestorePreviewDTO{}, safe(err)
	}
	preview, err := s.inspector.InspectBackup(ctx, packagePath)
	return recovery.PreviewDTO(preview), safe(err)
}
func (s *Service) ConfirmRestore(request recovery.RestoreConfirmRequest) (recovery.RestoreResultDTO, error) {
	if s.recovery == nil {
		return recovery.RestoreResultDTO{}, safe(infra.ErrUnavailable)
	}
	return s.recovery.ConfirmRestore(request)
}

func (s *Service) RetentionStatus() (infra.RetentionStatus, error) {
	if s.manager == nil {
		return infra.RetentionStatus{}, safe(infra.ErrUnavailable)
	}
	v, err := s.manager.RetentionStatus()
	return v, safe(err)
}
func (s *Service) ConfigureRetention(u infra.RetentionUpdate) (infra.RetentionStatus, error) {
	if s.manager == nil {
		return infra.RetentionStatus{}, safe(infra.ErrUnavailable)
	}
	v, err := s.manager.ConfigureRetention(u)
	return v, safe(err)
}
func (s *Service) PreviewRetention(ctx context.Context, days int) (infra.RetentionPreview, error) {
	if s.manager == nil {
		return infra.RetentionPreview{}, safe(infra.ErrUnavailable)
	}
	v, err := s.manager.PreviewRetention(ctx, days)
	return v, safe(err)
}
func (s *Service) ExecuteRetention(ctx context.Context, u infra.CleanupRequest) (infra.RetentionStatus, error) {
	if s.manager == nil {
		return infra.RetentionStatus{}, safe(infra.ErrUnavailable)
	}
	v, err := s.manager.ExecuteRetention(ctx, u)
	return v, safe(err)
}
func (s *Service) CancelCleanup() {
	if s.manager != nil {
		s.manager.CancelCleanup()
	}
}
