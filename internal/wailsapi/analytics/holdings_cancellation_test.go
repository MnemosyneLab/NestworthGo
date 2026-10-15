package analytics_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func TestHoldingsReadCancellationAndLiveErrors(t *testing.T) {
	app := wailstest.NewService(t)
	s := analytics.NewService(app)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.InstrumentHoldings(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("synthetic SQLite read did not reproduce cancellation: %v", err)
	}
	id := domain.NewAccountID().String()
	tests := []struct {
		name string
		read func(context.Context) error
	}{
		{"holdings", func(ctx context.Context) error { _, err := s.InstrumentHoldings(ctx); return err }},
		{"account holdings", func(ctx context.Context) error { _, err := s.AccountHoldingDetails(ctx, id); return err }},
		{"groups", func(ctx context.Context) error { _, err := s.InstrumentHoldingGroups(ctx); return err }},
		{"account groups", func(ctx context.Context) error { _, err := s.AccountHoldingGroups(ctx, id); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.read(ctx); err != nil {
				t.Fatalf("abandoned read became a binding failure: %v", err)
			}
			if err := tt.read(t.Context()); err != nil {
				t.Fatalf("replacement read failed: %v", err)
			}
			// A deadline is a real read failure, not observer abandonment.
			deadline, stop := context.WithDeadline(t.Context(), time.Unix(0, 0))
			defer stop()
			err := tt.read(deadline)
			if err == nil {
				t.Fatal("expired deadline was hidden as cancellation")
			}
			wire, ok := apierror.Parse(err.Error())
			if !ok || wire.Code != "internal" {
				t.Fatalf("deadline error contract changed: %v", err)
			}
		})
	}
	_, err := s.AccountHoldingGroups(t.Context(), "invalid-id")
	if err == nil {
		t.Fatal("live invalid read succeeded")
	}
	wire, ok := apierror.Parse(err.Error())
	if !ok || wire.Code != "validation" {
		t.Fatalf("live error contract changed: %v", err)
	}
}
