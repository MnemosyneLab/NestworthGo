package history_test

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestCashTransferLiabilityRejectedThroughWails(t *testing.T) {
	fx := newFixture(t)
	ctx := t.Context()
	if _, err := fx.service.RecordChange(ctx, history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "1000", Currency: "USD", Reason: "contribution"}); err != nil {
		t.Fatal(err)
	}
	valid := history.ChangeCommandRequest{Kind: history.ChangeCashTransfer, FromAccountID: fx.checkingID, ToAccountID: fx.savingsID, Sent: "100", SentCurrency: "USD", Received: "100", ReceivedCurrency: "USD"}
	original, err := fx.service.RecordChange(ctx, valid)
	if err != nil {
		t.Fatal(err)
	}
	before, err := fx.app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, direction := range []struct{ name, from, to, field string }{
		{"asset_to_liability", fx.checkingID, fx.cardID, "toAccountId"},
		{"liability_to_asset", fx.cardID, fx.savingsID, "fromAccountId"},
	} {
		t.Run(direction.name, func(t *testing.T) {
			invalid := valid
			invalid.FromAccountID, invalid.ToAccountID = direction.from, direction.to
			for name, run := range map[string]func() error{
				"preview":     func() error { _, err := fx.service.PreviewChange(ctx, invalid); return err },
				"record":      func() error { _, err := fx.service.RecordChange(ctx, invalid); return err },
				"preview_fix": func() error { _, err := fx.service.PreviewFixChange(ctx, original.Activity.ID, invalid); return err },
				"fix":         func() error { _, err := fx.service.FixChange(ctx, original.Activity.ID, invalid); return err },
			} {
				t.Run(name, func(t *testing.T) {
					err := run()
					if err == nil {
						t.Fatal("liability cash transfer accepted")
					}
					failure, ok := apierror.Parse(err.Error())
					if !ok || failure.Code != "invalid_change" || failure.Field != direction.field {
						t.Fatalf("wire error = %v", err)
					}
					after, err := fx.app.ListActivities(ctx, 100)
					if err != nil || len(after) != len(before) {
						t.Fatalf("rejection changed history: %d -> %d, %v", len(before), len(after), err)
					}
				})
			}
		})
	}
}
