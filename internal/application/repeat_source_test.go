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

func TestRepeatSourceClosedReasonMatrix(t *testing.T) {
	account := domain.NewAccountID()
	holding := domain.NewHoldingID()
	money, err := domain.ParseMoney("0.0001", "JPY")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		kind   domain.ActivityKind
		reason domain.ActivityReason
	}{
		{domain.ActivityCashIn, domain.ReasonIncome}, {domain.ActivityCashIn, domain.ReasonInterest},
		{domain.ActivityCashIn, domain.ReasonContribution}, {domain.ActivityCashIn, domain.ReasonGift}, {domain.ActivityCashIn, domain.ReasonOther},
		{domain.ActivityCashOut, domain.ReasonExpense}, {domain.ActivityCashOut, domain.ReasonFee},
		{domain.ActivityCashOut, domain.ReasonTax}, {domain.ActivityCashOut, domain.ReasonInterest}, {domain.ActivityCashOut, domain.ReasonOther},
	} {
		a := domain.Activity{Kind: test.kind, Reason: test.reason, Effects: []domain.ActivityEffect{{AccountID: &account, Money: &money}}}
		if !ordinaryRepeatSource(a) {
			t.Fatalf("ordinary source rejected: %s %s", test.kind, test.reason)
		}
		a.Effects[0].HoldingID = &holding
		if ordinaryRepeatSource(a) || repeatSourceReason(a) != "holding_attribution" {
			t.Fatalf("holding attribution accepted: %s %s", test.kind, test.reason)
		}
	}
	for _, test := range []struct {
		kind   domain.ActivityKind
		reason domain.ActivityReason
	}{
		{domain.ActivityCashIn, domain.ReasonReconciliation}, {domain.ActivityCashOut, domain.ReasonReconciliation},
		{domain.ActivityCashIn, domain.ReasonFee}, {domain.ActivityCashOut, domain.ReasonGift},
		{domain.ActivityCashDividend, domain.ReasonIncome}, {domain.ActivityCashTransfer, domain.ReasonOther},
		{domain.ActivityBuy, domain.ReasonOther}, {domain.ActivityDebtPayment, domain.ReasonInterest},
	} {
		a := domain.Activity{Kind: test.kind, Reason: test.reason, Effects: []domain.ActivityEffect{{AccountID: &account, Money: &money}}}
		if ordinaryRepeatSource(a) {
			t.Fatalf("excluded source accepted: %s %s", test.kind, test.reason)
		}
	}
	ordinary := domain.Activity{Kind: domain.ActivityCashIn, Reason: domain.ReasonIncome, Effects: []domain.ActivityEffect{{AccountID: &account, Money: &money}}}
	withDividend := ordinary
	withDividend.DividendDetail = &domain.DividendDetail{}
	withTrade := ordinary
	withTrade.TradeDetail = &domain.TradeDetail{}
	withProduct := ordinary
	withProduct.ProductContext = &domain.ProductActivityContext{}
	for _, a := range []domain.Activity{withDividend, withTrade, withProduct} {
		if ordinaryRepeatSource(a) {
			t.Fatal("specialized source accepted as ordinary cash")
		}
	}
	for _, reason := range []domain.ActivityReason{domain.ReasonFee, domain.ReasonTax} {
		attributed := domain.Activity{Kind: domain.ActivityCashOut, Reason: reason, DividendDetail: &domain.DividendDetail{HoldingID: holding}, Effects: []domain.ActivityEffect{{AccountID: &account, Money: &money}}}
		if ordinaryRepeatSource(attributed) || repeatSourceReason(attributed) != "holding_attribution" {
			t.Fatalf("holding-linked %s reason = %q", reason, repeatSourceReason(attributed))
		}
	}
	dividend := domain.Activity{Kind: domain.ActivityCashDividend, Reason: domain.ReasonIncome, DividendDetail: &domain.DividendDetail{HoldingID: holding}, Effects: []domain.ActivityEffect{{AccountID: &account, Money: &money}}}
	if repeatSourceReason(dividend) != "specialized" {
		t.Fatalf("cash dividend reason = %q", repeatSourceReason(dividend))
	}
}
