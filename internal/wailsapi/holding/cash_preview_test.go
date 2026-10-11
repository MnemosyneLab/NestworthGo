package holding_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestCashPreviewConfirmation(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "baseline", true: "history"}[started], func(t *testing.T) {
			fx := onboardedFixture(t)
			ctx := context.Background()
			s := holding.NewService(fx.app)
			if _, err := s.AppendAccountCashValue(ctx, fx.accountID, "100", "USD", ""); err != nil {
				t.Fatal(err)
			}
			if started {
				if _, err := history.NewService(fx.app).StartHistory(ctx, "UTC"); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := s.ListAccountCashValues(ctx, fx.accountID)
			reason := "Statement balance"
			var note *string
			if started {
				note = &reason
			}
			preview, err := s.PreviewCashBalance(ctx, fx.accountID, "80", "USD", note)
			if err != nil {
				t.Fatal(err)
			}
			if preview.Current != "100" || preview.Target != "80" || preview.Delta != "-20" || preview.EffectiveAt == "" {
				t.Fatalf("preview: %+v", preview)
			}
			values, _ := s.ListAccountCashValues(ctx, fx.accountID)
			if len(values) != len(before) {
				t.Fatal("preview wrote")
			}
			var confirmedAt time.Time
			if !started {
				at, err := time.Parse(time.RFC3339Nano, preview.EffectiveAt)
				if err != nil {
					t.Fatal(err)
				}
				confirmedAt = at.Add(time.Minute)
				fx.app.SetClock(func() time.Time { return confirmedAt })
			}
			var wg sync.WaitGroup
			for i := 0; i < 2; i++ {
				wg.Go(func() {
					if _, err := s.ConfirmCashBalance(ctx, preview.PlanID); err != nil {
						t.Error(err)
					}
				})
			}
			wg.Wait()
			values, _ = s.ListAccountCashValues(ctx, fx.accountID)
			if len(values) != len(before)+1 || values[0].Amount.Amount != "80" {
				t.Fatalf("values: %+v", values)
			}
			if !started && values[0].CreatedAt != wire.FormatTime(confirmedAt) {
				t.Fatalf("baseline audit time: %+v", values[0])
			}
			if _, err := s.ConfirmCashBalance(ctx, preview.PlanID); err != nil {
				t.Fatal("replay", err)
			}
			stale, err := s.PreviewCashBalance(ctx, fx.accountID, "120", "USD", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.AppendAccountCashValue(ctx, fx.accountID, "90", "USD", ""); err != nil {
				t.Fatal(err)
			}
			_, err = s.ConfirmCashBalance(ctx, stale.PlanID)
			var wire *apierror.WireError
			if !errors.As(err, &wire) || wire.Code != "stale_preview" {
				t.Fatalf("stale: %v", err)
			}
			fresh, err := s.PreviewCashBalance(ctx, fx.accountID, "120", "USD", nil)
			if err != nil {
				t.Fatal(err)
			}
			if fresh.Current != "90" || fresh.Delta != "30" {
				t.Fatalf("fresh %+v", fresh)
			}
			if err := account.NewService(fx.app).ArchiveAccount(ctx, fx.accountID, true); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ConfirmCashBalance(ctx, fresh.PlanID); err == nil {
				t.Fatal("archived account accepted")
			}
			if _, err := s.PreviewCashBalance(ctx, fx.accountID, "140", "USD", nil); err == nil {
				t.Fatal("archived preview accepted")
			}
		})
	}
}
