package history_test

import (
	"context"
	"errors"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"sync"
	"testing"
)

func TestRepeatFreshGuardedConfirmation(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	origin, err := fx.service.HistoryOrigin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	request := history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "100", Currency: "USD", Reason: "income", EffectiveAt: origin.StartedAt, MutationID: "old-identity-is-ignored", HoldingID: fx.instrumentID}
	plan, err := fx.service.PreviewRepeatChange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	page, err := fx.service.ListActivityPage(ctx, history.ActivityQueryRequest{})
	if err != nil || len(page.Activities) != 0 {
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
	if err != nil || len(page.Activities) != 1 {
		t.Fatalf("result %+v %v", page, err)
	}
	second, err := fx.service.PreviewRepeatChange(ctx, request)
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
	stale, err := fx.service.PreviewRepeatChange(ctx, request)
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
	if _, err := fx.service.PreviewRepeatChange(ctx, request); err == nil {
		t.Fatal("archived accepted")
	}
	request.AccountID = fx.savingsID
	request.Currency = "EUR"
	if _, err := fx.service.PreviewRepeatChange(ctx, request); err == nil {
		t.Fatal("changed currency accepted")
	}
	request.AccountID = "00000000-0000-0000-0000-000000000001"
	request.Currency = "USD"
	if _, err := fx.service.PreviewRepeatChange(ctx, request); err == nil {
		t.Fatal("missing account accepted")
	}
	if _, err := history.NewService(fx.app).ConfirmRepeatChange(ctx, plan.PlanID); err == nil {
		t.Fatal("unavailable process plan accepted")
	}
}

func TestRepeatRejectsSpecializedCommands(t *testing.T) {
	fx := newFixture(t)
	for _, request := range []history.ChangeCommandRequest{
		{Kind: history.ChangeMoneyAdded, Reason: "reconciliation"}, {Kind: history.ChangeMoneyAdded, Reason: "interest"}, {Kind: history.ChangeMoneyRemoved, Reason: "fee"}, {Kind: history.ChangeTrade, Reason: "income"}, {Kind: history.ChangeCashDividend, Reason: "income"}, {Kind: history.ChangeCashTransfer, Reason: "expense"}, {Kind: history.ChangeValueUpdate, Reason: "income"}, {Kind: history.ChangeFXConversion, Reason: "income"}, {Kind: history.ChangeDebtPayment, Reason: "expense"},
	} {
		if _, err := fx.service.PreviewRepeatChange(context.Background(), request); err == nil {
			t.Fatalf("accepted %+v", request)
		}
	}
}
