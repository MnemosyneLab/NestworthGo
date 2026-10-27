package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func TestLiabilityPurchaseRefundAndRepaymentPersistence(t *testing.T) {
	s, _, household, bank, _, debt, _ := cashTransferLiabilityFixture(t)
	ctx := t.Context()
	amount := mustMoney(t, "100", "CNY")
	commands := []any{
		domain.MoneyAddedInput{HouseholdID: household, AccountID: debt, Amount: amount, Reason: domain.ReasonCardPurchase, EffectiveAt: s.clock()},
		domain.MoneyRemovedInput{HouseholdID: household, AccountID: debt, Amount: amount, Reason: domain.ReasonCardRefund, EffectiveAt: s.clock()},
		domain.DebtPaymentInput{HouseholdID: household, DebtAccountID: debt, CashAccountID: bank, Principal: amount, EffectiveAt: s.clock()},
	}
	for i, command := range commands {
		preview, err := s.PreviewChange(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		committed, err := s.RecordChange(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := s.repository.Activity(ctx, household, committed.Activity.ID)
		if err != nil {
			t.Fatal(err)
		}
		if saved.Reason != preview.Activity.Reason || len(saved.Effects) != len(preview.Effects) {
			t.Fatalf("preview/commit mismatch: %+v", saved)
		}
		if i == 0 && saved.Effects[0].Classification != domain.ClassificationExternalOutflow {
			t.Fatal("purchase became income")
		}
		if i == 2 {
			for _, effect := range saved.Effects {
				if effect.Classification != domain.ClassificationDebtPrincipal {
					t.Fatal("repayment became expense")
				}
			}
		}
	}
	state, err := s.changeState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Accounts[debt].Current.CanonicalAmount() != "400" || state.Accounts[bank].Current.CanonicalAmount() != "900" {
		t.Fatalf("wrong balances: %+v", state.Accounts)
	}
	tooMuch := mustMoney(t, "401", "CNY")
	if _, err = s.RecordChange(ctx, domain.MoneyRemovedInput{HouseholdID: household, AccountID: debt, Amount: tooMuch, Reason: domain.ReasonCardRefund, EffectiveAt: s.clock()}); err == nil {
		t.Fatal("refund exceeded debt")
	}
	if _, err = s.RecordChange(ctx, domain.DebtPaymentInput{HouseholdID: household, DebtAccountID: debt, CashAccountID: bank, Principal: tooMuch, EffectiveAt: s.clock()}); err == nil {
		t.Fatal("repayment exceeded debt")
	}
}

func TestLiabilityPurchaseAndRefundAttribution(t *testing.T) {
	for _, refund := range []bool{false, true} {
		t.Run(map[bool]string{false: "purchase", true: "refund"}[refund], func(t *testing.T) {
			household := domain.NewHouseholdID()
			account := analysisAccount(household, "CNY", domain.TrackingBalance, domain.RoleLiability)
			account.AccountType = domain.TypeCreditCard
			before, after, reason, direction, classification := "100", "200", domain.ReasonCardPurchase, domain.EffectAdded, domain.ClassificationExternalOutflow
			if refund {
				before, after, reason, direction, classification = "200", "100", domain.ReasonCardRefund, domain.EffectRemoved, domain.ClassificationExternalInflow
			}
			id := domain.NewActivityID()
			money := mustMoney(t, "100", "CNY")
			activity := domain.Activity{ID: id, Kind: domain.ActivityCashIn, Reason: reason, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{{ID: domain.NewActivityEffectID(), ActivityID: id, Sequence: 1, Role: domain.EffectRoleAmount, Target: domain.EffectTargetAccountValue, Direction: direction, Classification: classification, AccountID: &account.ID, Money: &money}}}
			if refund {
				activity.Kind = domain.ActivityCashOut
			}
			input := AnalysisInputs{Origin: analysisOrigin(t, household, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: household, BaseCurrency: "CNY"}, Accounts: []domain.AccountRecord{{Account: account}}}, Snapshots: []domain.DailyValuationSnapshot{analysisSnapshot("2026-08-01", analysisItem(t, account.ID, "CNY", before, before, nil, nil, "", "")), analysisSnapshot("2026-08-02", analysisItem(t, account.ID, "CNY", after, after, nil, nil, "", ""))}, Activities: []domain.Activity{activity}}
			result, err := ComputeAnalysis(input, domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true})
			if err != nil {
				t.Fatal(err)
			}
			expected := "-100"
			if refund {
				expected = "100"
			}
			if len(result.Days) != 1 || result.Days[0].AssetBuckets[domain.BucketSpending].CanonicalAmount() != expected {
				t.Fatalf("wrong spending bucket: %+v", result.Days)
			}
			if !result.Days[0].AssetBuckets[domain.BucketIncome].Amount().IsZero() {
				t.Fatal("card activity became income")
			}
		})
	}
}
