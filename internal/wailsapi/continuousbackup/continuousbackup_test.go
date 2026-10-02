package continuousbackup

import (
	"context"
	infra "github.com/waltwang/nestworth-go/internal/infrastructure/continuousbackup"
	"github.com/waltwang/nestworth-go/internal/wailsapi/recovery"
	"strings"
	"testing"
)

func TestUnavailableServiceAndSanitizedErrors(t *testing.T) {
	s := NewService(nil, nil, nil, nil)
	if _, err := s.Status(); err == nil {
		t.Fatal("nil manager available")
	}
	if _, err := s.Configure(context.Background(), infra.Update{}); err == nil {
		t.Fatal("nil manager configured")
	}
	if _, err := s.InspectRestore(context.Background(), infra.RecoveryPoint{}); err == nil {
		t.Fatal("nil restore")
	}
	if _, err := s.ConfirmRestore(recovery.RestoreConfirmRequest{}); err == nil {
		t.Fatal("nil confirmation")
	}
	if err := safe(infra.ErrConfiguration); err == nil || strings.Contains(err.Error(), "accessKeyID") {
		t.Fatal("unsafe error")
	}
}

func TestRetentionServiceUnavailableAndRedacted(t *testing.T) {
	s := NewService(nil, nil, nil, nil)
	if _, err := s.RetentionStatus(); err == nil {
		t.Fatal("nil retention available")
	}
	if _, err := s.ConfigureRetention(infra.RetentionUpdate{Enabled: true, Days: 30, Acknowledged: true}); err == nil {
		t.Fatal("nil retention configured")
	}
	if _, err := s.PreviewRetention(context.Background(), 30); err == nil {
		t.Fatal("nil preview")
	}
	if _, err := s.ExecuteRetention(context.Background(), infra.CleanupRequest{Token: "test", Acknowledged: true}); err == nil {
		t.Fatal("nil cleanup")
	}
	s.CancelCleanup()
	if err := safe(infra.ErrRetention); err == nil || strings.Contains(err.Error(), "token") {
		t.Fatal("unsafe cleanup error")
	}
}
