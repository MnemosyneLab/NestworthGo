package application

import (
	"errors"
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestCashPlanExpiryAndImmutableReason(t *testing.T) {
	s, ctx, bootstrap, _ := newOnboardedService(t, "cash-plan", []string{"Owner"})
	record, err := s.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	reason := "Statement"
	plan, err := s.PreviewCashBalance(ctx, record.Account.ID, "100", "USD", &reason)
	if err != nil {
		t.Fatal(err)
	}
	reason = "Edited caller memory"
	plan.expiresAt = time.Now().Add(-time.Second)
	_, err = s.ConfirmCashBalance(ctx, plan)
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrStalePreview {
		t.Fatalf("expired: %v", err)
	}

	plan, err = s.PreviewCashBalance(ctx, record.Account.ID, "100", "USD", &reason)
	if err != nil {
		t.Fatal(err)
	}
	reason = "Later edit"
	value, err := s.ConfirmCashBalance(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if value.ActivityNote == nil || *value.ActivityNote != "Edited caller memory" {
		t.Fatalf("note: %+v", value)
	}
	plan.expiresAt = time.Now().Add(-time.Hour)
	replay, err := s.ConfirmCashBalance(ctx, plan)
	if err != nil || replay.ID != value.ID {
		t.Fatalf("expired committed retry: %+v %v", replay, err)
	}
}
