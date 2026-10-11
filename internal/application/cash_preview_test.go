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

func TestCashPreviewRejectsLaterEffectiveWinningObservation(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "future-baseline", true: "history-clock-rollback"}[started], func(t *testing.T) {
			s, ctx, bootstrap, setClock := newOnboardedService(t, "cash-order", []string{"Owner"})
			account, err := s.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			original := s.clock()
			later := original.Add(48 * time.Hour)
			target, winning := "80", "500"
			if started {
				if _, err := s.StartHistory(ctx, "Asia/Shanghai"); err != nil {
					t.Fatal(err)
				}
				setClock(later)
				amount, _ := domain.ParseMoney("100", "CNY")
				if _, err := s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: later}); err != nil {
					t.Fatal(err)
				}
				setClock(original.Add(24 * time.Hour))
				target, winning = "40", "100"
			} else {
				if _, err := s.AppendAccountCashValue(ctx, account.Account.ID, "500", "CNY", later.Format(time.RFC3339Nano)); err != nil {
					t.Fatal(err)
				}
			}
			before, err := s.repository.ListAccountCashValues(ctx, account.Account.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.PreviewCashBalance(ctx, account.Account.ID, target, "CNY", nil); !hasDomainCode(err, domain.ErrCashBalanceTimeConflict) {
				t.Fatalf("later winner preview = %v", err)
			}
			after, err := s.repository.ListAccountCashValues(ctx, account.Account.ID)
			if err != nil || len(after) != len(before) || after[0].Amount.CanonicalAmount() != winning {
				t.Fatalf("preview modified cash %+v %v", after, err)
			}
			setClock(later.Add(time.Minute))
			plan, err := s.PreviewCashBalance(ctx, account.Account.ID, target, "CNY", nil)
			if err != nil {
				t.Fatal(err)
			}
			if started && plan.Timezone != "Asia/Shanghai" {
				t.Fatalf("origin timezone: %s", plan.Timezone)
			}
			result, err := s.ConfirmCashBalance(ctx, plan)
			if err != nil || result.Amount.CanonicalAmount() != target {
				t.Fatalf("corrected clock confirmation %+v %v", result, err)
			}
			snapshot, _, err := s.portfolioSnapshot(ctx)
			if err != nil || len(snapshot.CashValues) != 1 || snapshot.CashValues[0].Amount.CanonicalAmount() != target {
				t.Fatalf("winning cash after confirmation %+v %v", snapshot.CashValues, err)
			}
		})
	}
}

func TestCashConfirmationRechecksLaterWinnerUnderWriteGate(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "baseline", true: "history"}[started], func(t *testing.T) {
			s, ctx, bootstrap, _ := newOnboardedService(t, "cash-confirm-order", []string{"Owner"})
			account, err := s.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			if started {
				if _, err := s.StartHistory(ctx, "UTC"); err != nil {
					t.Fatal(err)
				}
			}
			plan, err := s.PreviewCashBalance(ctx, account.Account.ID, "80", "CNY", nil)
			if err != nil {
				t.Fatal(err)
			}
			// Synthetic out-of-band row leaves the preview revision unchanged, proving
			// confirmation checks ordering itself rather than only the stale fence.
			amount, _ := domain.ParseMoney("500", "CNY")
			later := plan.EffectiveAt.Add(48 * time.Hour)
			row, err := domain.NewAccountCashValue(account.Account, amount, later, later)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.repository.AppendAccountCashValue(ctx, row); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ConfirmCashBalance(ctx, plan); !hasDomainCode(err, domain.ErrCashBalanceTimeConflict) {
				t.Fatalf("confirmation = %v", err)
			}
			values, err := s.repository.ListAccountCashValues(ctx, account.Account.ID)
			if err != nil || len(values) != 1 || values[0].ID != row.ID {
				t.Fatalf("confirmation wrote %+v %v", values, err)
			}
		})
	}
}

func TestCashPreviewRejectsWinningRowWithLaterAuditTime(t *testing.T) {
	s, ctx, bootstrap, _ := newOnboardedService(t, "cash-audit-order", []string{"Owner"})
	account, err := s.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	amount, _ := domain.ParseMoney("500", "CNY")
	now := s.clock()
	row, err := domain.NewAccountCashValue(account.Account, amount, now, now.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.repository.AppendAccountCashValue(ctx, row); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PreviewCashBalance(ctx, account.Account.ID, "80", "CNY", nil); !hasDomainCode(err, domain.ErrCashBalanceTimeConflict) {
		t.Fatalf("later audit winner preview = %v", err)
	}
}
