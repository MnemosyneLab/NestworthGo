package application

import (
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestRepeatCommitChecksSourceEvenWithCurrentToken(t *testing.T) {
	s, ctx, bootstrap, _ := newOnboardedService(t, "repeat-source", []string{"Owner"})
	account, err := s.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "1000", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	origin, err := s.StartHistory(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	amount, _ := domain.ParseMoney("10", "USD")
	command := domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: origin.StartedAt}
	source, err := s.RecordChange(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoChange(ctx, source.Activity.ID); err != nil {
		t.Fatal(err)
	}
	// Obtain a current ordinary-command token, so the stale fence cannot mask
	// the source eligibility check inside the commit's write gate.
	_, token, err := s.PreviewChangeGuarded(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RecordRepeatChangeGuarded(ctx, source.Activity.ID, command, domain.NewMutationID().String(), strings.Repeat("ab", 32), token)
	if !hasDomainCode(err, domain.ErrAlreadyUndone) {
		t.Fatalf("current-token repeat of reversed source = %v", err)
	}
	activities, err := s.ListActivities(ctx, 100)
	if err != nil || len(activities) != 2 {
		t.Fatalf("unexpected writes %+v %v", activities, err)
	}
}
