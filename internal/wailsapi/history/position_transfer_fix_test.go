package history_test

import (
	"context"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
)

func TestPositionTransferFixFromStoredActivityUsesExistingHolding(t *testing.T) {
	for _, changeTarget := range []bool{false, true} {
		name := "quantity_and_note"
		if changeTarget {
			name = "different_existing_target"
		}
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t) // Real application and temp-file SQLite, no repository mocks.
			ctx := context.Background()
			if _, err := fx.service.RecordChange(ctx, history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.brokerageID, Amount: "200", Currency: "USD", Reason: "contribution"}); err != nil {
				t.Fatal(err)
			}
			bought, err := fx.service.RecordChange(ctx, history.ChangeCommandRequest{Kind: history.ChangeTrade, SettlementAccountID: fx.brokerageID, InstrumentID: fx.instrumentID, Side: "buy", Quantity: "10", Gross: "100", GrossCurrency: "USD"})
			if err != nil {
				t.Fatal(err)
			}
			sourceID := bought.Activity.TradeDetail.HoldingID
			b, err := household.NewService(fx.app).Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			other, err := account.NewService(fx.app).CreateAccount(ctx, account.CreateAccountRequest{Name: "Other existing destination", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []string{b.Members[0].ID}, IncludeInNetWorth: true})
			if err != nil {
				t.Fatal(err)
			}
			otherHolding, err := holding.NewService(fx.app).CreateHolding(ctx, holding.CreateHoldingRequest{AccountID: other.Account.ID, InstrumentID: fx.instrumentID, Quantity: "0"})
			if err != nil {
				t.Fatal(err)
			}
			recorded, err := fx.service.RecordChange(ctx, history.ChangeCommandRequest{Kind: history.ChangePositionTransfer, FromHoldingID: sourceID, ToAccountID: fx.brokerage2ID, Quantity: "2"})
			if err != nil {
				t.Fatal(err)
			}
			original, err := fx.service.Activity(ctx, recorded.Activity.ID)
			if err != nil {
				t.Fatal(err)
			}
			// Reconstruct the Fix input from persisted effects, matching activityToInitialCommand.
			request := history.ChangeCommandRequest{Kind: history.ChangePositionTransfer}
			for _, effect := range original.Effects {
				switch effect.Role {
				case "transfer_from":
					request.FromHoldingID = *effect.HoldingID
					request.Quantity = *effect.Quantity
				case "transfer_to":
					request.ToHoldingID = *effect.HoldingID
				}
			}
			if request.FromHoldingID != sourceID || request.ToHoldingID == "" || request.Quantity != "2" {
				t.Fatalf("incomplete stored activity reconstruction: %+v", request)
			}
			originalTargetID := request.ToHoldingID
			note := "Corrected transfer"
			request.Quantity = "3"
			request.Note = &note
			if changeTarget {
				request.ToHoldingID = otherHolding.ID
			}
			// Account-based creation is deliberately unsupported during historical Fix.
			broken := request
			broken.ToHoldingID = ""
			broken.ToAccountID = fx.brokerage2ID
			_, err = fx.service.PreviewFixChange(ctx, original.ID, broken)
			if err == nil {
				t.Fatal("historical correction unexpectedly accepted account-only destination")
			}
			wireErr, ok := apierror.Parse(err.Error())
			if !ok || wireErr.Code != "cannot_fix_change" || wireErr.Field != "toHoldingId" {
				t.Fatalf("wrong contract error: %v", err)
			}
			preview, err := fx.service.PreviewFixChange(ctx, original.ID, request)
			if err != nil {
				t.Fatal(err)
			}
			fixed, err := fx.service.FixChange(ctx, original.ID, request)
			if err != nil {
				t.Fatal(err)
			}
			if fixed.Activity.CorrectionGroupID == nil || fixed.Activity.EffectiveAt != original.EffectiveAt || fixed.Activity.Note == nil || *fixed.Activity.Note != note {
				t.Fatalf("bad correction metadata: %+v", fixed.Activity)
			}
			if len(preview.Resulting) != len(fixed.Resulting) {
				t.Fatal("preview and commit disagree")
			}
			for i := range preview.Resulting {
				if preview.Resulting[i].Quantity != fixed.Resulting[i].Quantity {
					t.Fatal("preview and commit quantities disagree")
				}
			}
			actual, err := holding.NewService(fx.app).HoldingsByAccounts(ctx, []string{fx.brokerageID, fx.brokerage2ID, other.Account.ID})
			if err != nil {
				t.Fatal(err)
			}
			quantities := map[string]string{}
			for _, rows := range actual {
				for _, row := range rows {
					quantities[row.ID] = row.Quantity
				}
			}
			wantOriginal, wantOther := "3", "0"
			if changeTarget {
				wantOriginal, wantOther = "0", "3"
			}
			if quantities[sourceID] != "7" || quantities[originalTargetID] != wantOriginal || quantities[otherHolding.ID] != wantOther || len(quantities) != 3 {
				t.Fatalf("wrong persisted holdings after Fix: %+v", quantities)
			}
			t.Logf("Fix from stored transfer: source=%s oldTarget=%s alternateTarget=%s; note and original effective instant retained", quantities[sourceID], quantities[originalTargetID], quantities[otherHolding.ID])
		})
	}
}
