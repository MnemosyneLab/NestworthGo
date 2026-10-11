package history_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestRepeatFreshGuardedConfirmation(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	origin, err := fx.service.HistoryOrigin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	request := history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "100", Currency: "USD", Reason: "income", EffectiveAt: origin.StartedAt, MutationID: "old-identity-is-ignored", HoldingID: fx.instrumentID}
	sourceRequest := request
	sourceRequest.MutationID = ""
	sourceRequest.HoldingID = ""
	source, err := fx.service.RecordChange(ctx, sourceRequest)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := fx.service.PreviewRepeatChange(ctx, source.Activity.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	page, err := fx.service.ListActivityPage(ctx, history.ActivityQueryRequest{})
	if err != nil || len(page.Activities) != 1 {
		t.Fatalf("preview wrote: %+v %v", page, err)
	}
	var wg sync.WaitGroup
	var ids sync.Map
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			result, err := fx.service.ConfirmRepeatChange(ctx, plan.PlanID)
			if err != nil {
				t.Error(err)
				return
			}
			ids.Store(result.Activity.ID, true)
		})
	}
	wg.Wait()
	count := 0
	ids.Range(func(_, _ any) bool { count++; return true })
	if count != 1 {
		t.Fatal("double submit created multiple IDs")
	}
	page, err = fx.service.ListActivityPage(ctx, history.ActivityQueryRequest{})
	if err != nil || len(page.Activities) != 2 {
		t.Fatalf("result %+v %v", page, err)
	}
	second, err := fx.service.PreviewRepeatChange(ctx, source.Activity.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ConfirmRepeatChange(ctx, second.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Activity.ID == page.Activities[0].ID || result.Activity.CorrectionGroupID != nil || result.Activity.ReversesActivityID != nil {
		t.Fatalf("copied identity %+v", result.Activity)
	}
	stale, err := fx.service.PreviewRepeatChange(ctx, source.Activity.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := account.NewService(fx.app).ArchiveAccount(ctx, fx.checkingID, true); err != nil {
		t.Fatal(err)
	}
	_, err = fx.service.ConfirmRepeatChange(ctx, stale.PlanID)
	var wire *apierror.WireError
	if !errors.As(err, &wire) || wire.Code != "stale_preview" {
		t.Fatalf("stale %v", err)
	}
	if _, err := fx.service.PreviewRepeatChange(ctx, source.Activity.ID, request); err == nil {
		t.Fatal("archived accepted")
	}
	request.AccountID = fx.savingsID
	request.Currency = "EUR"
	if _, err := fx.service.PreviewRepeatChange(ctx, source.Activity.ID, request); err == nil {
		t.Fatal("changed currency accepted")
	}
	request.AccountID = "00000000-0000-0000-0000-000000000001"
	request.Currency = "USD"
	if _, err := fx.service.PreviewRepeatChange(ctx, source.Activity.ID, request); err == nil {
		t.Fatal("missing account accepted")
	}
	if _, err := history.NewService(fx.app).ConfirmRepeatChange(ctx, plan.PlanID); err == nil {
		t.Fatal("unavailable process plan accepted")
	}
}

func TestRepeatRejectsSpecializedCommands(t *testing.T) {
	fx := newFixture(t)
	for _, request := range []history.ChangeCommandRequest{
		{Kind: history.ChangeMoneyAdded, Reason: "reconciliation"}, {Kind: history.ChangeMoneyRemoved, Reason: "reconciliation"}, {Kind: history.ChangeMoneyAdded, Reason: "fee"}, {Kind: history.ChangeMoneyRemoved, Reason: "gift"}, {Kind: history.ChangeTrade, Reason: "income"}, {Kind: history.ChangeCashDividend, Reason: "income"}, {Kind: history.ChangeCashTransfer, Reason: "expense"}, {Kind: history.ChangeValueUpdate, Reason: "income"}, {Kind: history.ChangeFXConversion, Reason: "income"}, {Kind: history.ChangeDebtPayment, Reason: "expense"},
	} {
		if _, err := fx.service.PreviewRepeatChange(context.Background(), "00000000-0000-0000-0000-000000000001", request); err == nil {
			t.Fatalf("accepted %+v", request)
		}
	}
}

func TestRepeatRejectsReversedSourceOutsideFilteredPage(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	origin, err := fx.service.HistoryOrigin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	request := history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "100", Currency: "USD", Reason: "income", EffectiveAt: origin.StartedAt}
	source, err := fx.service.RecordChange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	page, err := fx.service.ListActivityPage(ctx, history.ActivityQueryRequest{Kinds: []string{"cash_in"}, Limit: 1})
	if err != nil || len(page.Activities) != 1 || !page.Activities[0].RepeatEligible {
		t.Fatalf("eligible page %+v %v", page, err)
	}
	pending, err := fx.service.PreviewRepeatChange(ctx, source.Activity.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.UndoChange(ctx, source.Activity.ID); err != nil {
		t.Fatal(err)
	}
	page, err = fx.service.ListActivityPage(ctx, history.ActivityQueryRequest{Kinds: []string{"cash_in"}, Limit: 1})
	if err != nil || len(page.Activities) != 1 || page.Activities[0].RepeatEligible || page.Activities[0].RepeatIneligibleReason != "reversed" {
		t.Fatalf("hidden reversal eligibility %+v %v", page, err)
	}
	if _, err := fx.service.PreviewRepeatChange(ctx, source.Activity.ID, request); err == nil {
		t.Fatal("reversed source preview accepted")
	}
	if _, err := fx.service.ConfirmRepeatChange(ctx, pending.PlanID); err == nil {
		t.Fatal("lifecycle change after preview committed")
	} else {
		var wire *apierror.WireError
		if !errors.As(err, &wire) || wire.Code != "stale_preview" {
			t.Fatalf("want stale preview, got %v", err)
		}
	}
	all, err := fx.service.ListActivityPage(ctx, history.ActivityQueryRequest{})
	if err != nil || len(all.Activities) != 2 {
		t.Fatalf("unexpected writes %+v %v", all, err)
	}
}

func TestRepeatReceiptReplaysAfterSourceIsReversed(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	origin, err := fx.service.HistoryOrigin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	request := history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "100", Currency: "USD", Reason: "income", EffectiveAt: origin.StartedAt}
	source, err := fx.service.RecordChange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := fx.service.PreviewRepeatChange(ctx, source.Activity.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := fx.service.ConfirmRepeatChange(ctx, plan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.UndoChange(ctx, source.Activity.ID); err != nil {
		t.Fatal(err)
	}
	replay, err := fx.service.ConfirmRepeatChange(ctx, plan.PlanID)
	if err != nil || replay.Activity.ID != committed.Activity.ID || replay.Activity.ID == source.Activity.ID {
		t.Fatalf("receipt replay %+v %v", replay, err)
	}
}

func TestRepeatInterestKeepsExistingClassification(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	origin, err := fx.service.HistoryOrigin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		kind         history.ChangeCommandKind
		amount, want string
	}{
		{history.ChangeMoneyAdded, "100", "external_inflow"},
		{history.ChangeMoneyRemoved, "10", "external_outflow"},
	} {
		request := history.ChangeCommandRequest{Kind: test.kind, AccountID: fx.checkingID, Amount: test.amount, Currency: "USD", Reason: "interest", EffectiveAt: origin.StartedAt}
		source, err := fx.service.RecordChange(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := fx.service.PreviewRepeatChange(ctx, source.Activity.ID, request)
		if err != nil {
			t.Fatal(err)
		}
		result, err := fx.service.ConfirmRepeatChange(ctx, plan.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		if result.Activity.ID == source.Activity.ID || result.Activity.Reason != "interest" || len(result.Activity.Effects) != 1 || result.Activity.Effects[0].Classification != test.want || source.Activity.Effects[0].Classification != test.want {
			t.Fatalf("interest classification changed: source=%+v repeated=%+v", source.Activity, result.Activity)
		}
	}
}

func TestRepeatValidatesSourceTypeAndKeepsExpenseFresh(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	origin, err := fx.service.HistoryOrigin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	income := history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "100", Currency: "USD", Reason: "income", EffectiveAt: origin.StartedAt}
	source, err := fx.service.RecordChange(ctx, income)
	if err != nil {
		t.Fatal(err)
	}
	expense := income
	expense.Kind = history.ChangeMoneyRemoved
	expense.Reason = "expense"
	if _, err := fx.service.PreviewRepeatChange(ctx, source.Activity.ID, expense); err == nil {
		t.Fatal("source type switch accepted")
	}
	if _, err := fx.service.PreviewRepeatChange(ctx, "00000000-0000-0000-0000-000000000001", income); err == nil {
		t.Fatal("missing source accepted")
	}
	reconciliation := income
	reconciliation.Reason = "reconciliation"
	specialized, err := fx.service.RecordChange(ctx, reconciliation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.PreviewRepeatChange(ctx, specialized.Activity.ID, income); err == nil {
		t.Fatal("specialized source accepted")
	}
	expense.Amount = "10"
	expenseSource, err := fx.service.RecordChange(ctx, expense)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := fx.service.PreviewRepeatChange(ctx, expenseSource.Activity.ID, expense)
	if err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ConfirmRepeatChange(ctx, plan.PlanID)
	if err != nil || result.Activity.ID == expenseSource.Activity.ID || result.Activity.Kind != "cash_out" || result.Activity.CorrectionGroupID != nil || result.Activity.ReversesActivityID != nil {
		t.Fatalf("fresh expense %+v %v", result, err)
	}
}
