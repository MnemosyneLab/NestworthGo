package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func cashTransferLiabilityFixture(t *testing.T) (*Service, *sqlite.DB, domain.HouseholdID, domain.AccountID, domain.AccountID, domain.AccountID, domain.AccountID) {
	t.Helper()
	ctx, s, _, db, boot, _ := historicalInsertionFixture(t)
	create := func(name, kind, role, amount string) domain.AccountID {
		t.Helper()
		a, err := s.CreateAccount(ctx, AccountInput{Name: name, AccountType: kind, BalanceSheetRole: role, TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: amount, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		return a.Account.ID
	}
	bank := create("Synthetic Bank", "bank_account", "asset", "1000")
	savings := create("Synthetic Savings", "bank_account", "asset", "0")
	loan := create("Synthetic Loan", "loan", "liability", "500")
	otherLoan := create("Synthetic Other Loan", "loan", "liability", "0")
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	return s, db, boot.Household.ID, bank, savings, loan, otherLoan
}

func cashTransferFacts(t *testing.T, db *sqlite.DB) map[string]int {
	t.Helper()
	counts := make(map[string]int)
	for _, table := range []string{"activities", "activity_effects", "account_values", "account_cash_values", "holding_quantity_values", "activity_mutation_keys", "change_batch_mutation_keys"} {
		var count int
		if err := db.SQL.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		counts[table] = count
	}
	return counts
}

func TestCashTransferLiabilityRejectedAcrossApplicationWrites(t *testing.T) {
	for _, direction := range []string{"asset_to_liability", "liability_to_asset", "liability_to_liability"} {
		t.Run(direction, func(t *testing.T) {
			s, db, household, bank, savings, loan, otherLoan := cashTransferLiabilityFixture(t)
			ctx := t.Context()
			amount := mustMoney(t, "100", "CNY")
			valid := domain.CashTransferInput{HouseholdID: household, FromAccountID: bank, ToAccountID: savings, Sent: amount, Received: amount}
			original, err := s.RecordChange(ctx, valid)
			if err != nil {
				t.Fatal(err)
			}
			invalid := valid
			field := "fromAccountId"
			switch direction {
			case "asset_to_liability":
				invalid.ToAccountID, field = loan, "toAccountId"
			case "liability_to_asset":
				invalid.FromAccountID = loan
			case "liability_to_liability":
				invalid.FromAccountID, invalid.ToAccountID = loan, otherLoan
			}
			before, err := s.changeState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			facts := cashTransferFacts(t, db)
			operations := map[string]func() error{
				"preview":     func() error { _, err := s.PreviewChange(ctx, invalid); return err },
				"record":      func() error { _, err := s.RecordChange(ctx, invalid); return err },
				"preview_fix": func() error { _, err := s.PreviewFixChange(ctx, original.Activity.ID, invalid); return err },
				"fix":         func() error { _, err := s.FixChange(ctx, original.Activity.ID, invalid); return err },
				"preview_batch": func() error {
					_, _, err := s.PreviewChangesGuardedWith(ctx, func(context.Context) ([]any, error) { return []any{valid, invalid}, nil })
					return err
				},
				"commit_batch": func() error {
					// Failed write attempts invalidate earlier previews too. Obtain
					// a fresh token so commit reaches command validation.
					_, token, err := s.PreviewChangesGuardedWith(ctx, func(context.Context) ([]any, error) { return []any{valid}, nil })
					if err != nil {
						return err
					}
					_, err = s.RecordChangesGuarded(ctx, []any{valid, invalid}, domain.NewMutationID().String(), strings.Repeat("ab", 32), token)
					return err
				},
			}
			for name, run := range operations {
				t.Run(name, func(t *testing.T) {
					var failure *domain.Error
					if err := run(); !errors.As(err, &failure) || failure.Code != domain.ErrInvalidChange || !strings.HasSuffix(failure.Field, field) {
						t.Fatalf("error = %v, want invalid_change at %s", err, field)
					}
					if after := cashTransferFacts(t, db); !reflect.DeepEqual(after, facts) {
						t.Fatalf("rejection wrote facts: before=%v after=%v", facts, after)
					}
					after, err := s.changeState(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(before.Accounts, after.Accounts) || !reflect.DeepEqual(before.Cash, after.Cash) {
						t.Fatalf("rejection changed account projections")
					}
				})
			}
		})
	}
}

func TestCashTransferLegacyLiabilityRecordsRemainReadableAndRecoverable(t *testing.T) {
	for _, action := range []string{"reverse", "fix_to_debt_payment"} {
		t.Run(action, func(t *testing.T) {
			s, db, household, bank, savings, loan, _ := cashTransferLiabilityFixture(t)
			ctx := t.Context()
			amount := mustMoney(t, "100", "CNY")
			old, err := s.PreviewChange(ctx, domain.CashTransferInput{HouseholdID: household, FromAccountID: bank, ToAccountID: savings, Sent: amount, Received: amount})
			if err != nil {
				t.Fatal(err)
			}
			// Persist the exact pre-fix effect shape through the repository, as
			// an existing database would contain it. No new command bypass exists.
			old.Effects[1].AccountID = &loan
			old.Activity.Effects = old.Effects
			old.Resulting[1].AccountID, old.Resulting[1].Name, old.Resulting[1].Amount = &loan, "Synthetic Loan", "600"
			if err := sqlite.NewRepository(db).CommitActivity(ctx, old.Activity, old.Effects, old.Resulting, s.clock()); err != nil {
				t.Fatal(err)
			}
			loaded, err := s.Activity(ctx, old.Activity.ID)
			if err != nil || loaded.Kind != domain.ActivityCashTransfer || len(loaded.Effects) != 2 {
				t.Fatalf("old record read = %+v, %v", loaded, err)
			}
			before, err := s.Overview(ctx, domain.AccountFilter{})
			if err != nil || before.NetWorth.String() != "300" {
				t.Fatalf("old record overview = %+v, %v", before, err)
			}
			if action == "reverse" {
				if _, err := s.UndoChange(ctx, old.Activity.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				replacement := domain.DebtPaymentInput{HouseholdID: household, DebtAccountID: loan, CashAccountID: bank, Principal: amount}
				if _, err := s.PreviewFixChange(ctx, old.Activity.ID, replacement); err != nil {
					t.Fatal(err)
				}
				if _, err := s.FixChange(ctx, old.Activity.ID, replacement); err != nil {
					t.Fatal(err)
				}
			}
			after, err := s.Overview(ctx, domain.AccountFilter{})
			if err != nil || after.NetWorth.String() != "500" {
				t.Fatalf("recovered overview = %+v, %v", after, err)
			}
			wantBank, wantLoan := "1000", "500"
			if action != "reverse" {
				wantBank, wantLoan = "900", "400"
			}
			state, err := s.changeState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if state.Accounts[bank].Current.CanonicalAmount() != wantBank || state.Accounts[loan].Current.CanonicalAmount() != wantLoan {
				t.Fatalf("recovered bank=%s loan=%s", state.Accounts[bank].Current.CanonicalAmount(), state.Accounts[loan].Current.CanonicalAmount())
			}
			preserved, err := s.Activity(ctx, old.Activity.ID)
			if err != nil || !reflect.DeepEqual(preserved.Effects, loaded.Effects) {
				t.Fatalf("recovery rewrote original evidence: %+v, %v", preserved, err)
			}
		})
	}
}
