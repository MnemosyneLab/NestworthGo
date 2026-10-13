package application

import (
	"context"
	"errors"
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

type repeatStatusSpy struct {
	Repository
	calls    map[domain.HouseholdID][]domain.ActivityID
	reversed map[domain.ActivityID]bool
	err      error
	cancel   context.CancelFunc
}

func (r *repeatStatusSpy) ActivitiesWithReversals(ctx context.Context, h domain.HouseholdID, ids []domain.ActivityID) (map[domain.ActivityID]bool, error) {
	r.calls[h] = append(r.calls[h], ids...)
	if r.cancel != nil {
		r.cancel()
	}
	return r.reversed, r.err
}
func (r *repeatStatusSpy) ActivityHasReversal(context.Context, domain.HouseholdID, domain.ActivityID) (bool, error) {
	panic("list hints must use batch reads")
}
func TestRepeatSourceStatusesBatch(t *testing.T) {
	account := domain.NewAccountID()
	money, _ := domain.ParseMoney("1", "USD")
	h := domain.NewHouseholdID()
	activities := make([]domain.Activity, 100)
	for i := range activities {
		activities[i] = domain.Activity{ID: domain.NewActivityID(), HouseholdID: h, Kind: domain.ActivityCashIn, Reason: domain.ReasonIncome, Effects: []domain.ActivityEffect{{AccountID: &account, Money: &money}}}
	}
	spy := &repeatStatusSpy{calls: make(map[domain.HouseholdID][]domain.ActivityID), reversed: map[domain.ActivityID]bool{activities[0].ID: true}}
	s := NewService(spy)
	got, err := s.RepeatSourceStatuses(t.Context(), append(activities, activities[0]))
	if err != nil || len(spy.calls) != 1 || len(spy.calls[h]) != 100 || got[activities[0].ID] != "reversed" || got[activities[1].ID] != "eligible" {
		t.Fatalf("batch: %v %v calls=%v", got, err, spy.calls)
	}
	other := activities[1]
	other.ID = domain.NewActivityID()
	other.HouseholdID = domain.NewHouseholdID()
	spy.calls = make(map[domain.HouseholdID][]domain.ActivityID)
	_, err = s.RepeatSourceStatuses(t.Context(), []domain.Activity{activities[1], other})
	if err != nil || len(spy.calls) != 2 {
		t.Fatalf("household grouping: %v %v", spy.calls, err)
	}
	excluded := []domain.Activity{}
	for _, reason := range []string{"corrected", "product", "specialized", "unsupported_reason", "holding_attribution", "not_single_account"} {
		a := activities[1]
		a.ID = domain.NewActivityID()
		switch reason {
		case "corrected":
			id := domain.NewActivityCorrectionGroupID()
			a.CorrectionGroupID = &id
		case "product":
			a.ProductContext = &domain.ProductActivityContext{}
		case "specialized":
			a.TradeDetail = &domain.TradeDetail{}
		case "unsupported_reason":
			a.Reason = domain.ReasonReconciliation
		case "holding_attribution":
			id := domain.NewHoldingID()
			a.Effects = append([]domain.ActivityEffect{}, a.Effects...)
			a.Effects[0].HoldingID = &id
		case "not_single_account":
			a.Effects = nil
		}
		excluded = append(excluded, a)
	}
	spy.calls = make(map[domain.HouseholdID][]domain.ActivityID)
	got, err = s.RepeatSourceStatuses(t.Context(), excluded)
	if err != nil || len(spy.calls) != 0 {
		t.Fatalf("excluded reads: %v %v", spy.calls, err)
	}
	for _, a := range excluded {
		if got[a.ID] != repeatSourceReason(a) {
			t.Fatalf("status changed: %v", got)
		}
	}
	spy.err = errors.New("synthetic repository failure")
	got, err = s.RepeatSourceStatuses(t.Context(), activities)
	if err == nil || got != nil {
		t.Fatalf("error: %v %v", got, err)
	}
	spy.err = nil
	ctx, cancel := context.WithCancel(t.Context())
	spy.cancel = cancel
	got, err = s.RepeatSourceStatuses(ctx, activities)
	if !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("cancel: %v %v", got, err)
	}
}
