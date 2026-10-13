package domain

import (
	"errors"
	"testing"
)

func TestCashTransferRejectsLiabilityEndpoints(t *testing.T) {
	for _, direction := range []struct {
		name                       string
		fromLiability, toLiability bool
		field                      string
	}{
		{"asset_to_liability", false, true, "toAccountId"},
		{"liability_to_asset", true, false, "fromAccountId"},
		{"liability_to_liability", true, true, "fromAccountId"},
	} {
		t.Run(direction.name, func(t *testing.T) {
			state, source, _, _, _ := changeTestState(t)
			destination := AccountID(newID())
			current, _ := ParseMoney("500", "CNY")
			state.Accounts[destination] = ChangeAccountState{ID: destination, Name: "Destination", Currency: "CNY", Mode: TrackingBalance, Liability: direction.toLiability, Current: current}
			from := state.Accounts[source]
			from.Liability = direction.fromLiability
			state.Accounts[source] = from
			amount, _ := ParseMoney("100", "CNY")
			preview, err := PreviewChange(state, CashTransferInput{HouseholdID: state.HouseholdID, FromAccountID: source, ToAccountID: destination, Sent: amount, Received: amount})
			var failure *Error
			if !errors.As(err, &failure) || failure.Code != ErrInvalidChange || failure.Field != direction.field {
				t.Fatalf("error = %v, want invalid_change at %s", err, direction.field)
			}
			if len(preview.Effects) != 0 || len(preview.Resulting) != 0 {
				t.Fatalf("rejected transfer returned effects: %+v", preview)
			}
		})
	}
}

func TestCashTransferAssetsConservePrincipalAndSeparateFee(t *testing.T) {
	for _, mode := range []TrackingMode{TrackingBalance, TrackingHoldings} {
		t.Run(string(mode), func(t *testing.T) {
			state, source, _, _, _ := changeTestState(t)
			destination := AccountID(newID())
			zero, _ := ParseMoney("0", "CNY")
			state.Accounts[destination] = ChangeAccountState{ID: destination, Name: "Asset destination", Currency: "CNY", Mode: mode, Current: zero}
			state.Cash[destination] = map[CurrencyCode]Money{"CNY": zero}
			amount, _ := ParseMoney("100", "CNY")
			fee, _ := ParseMoney("2", "CNY")
			preview, err := PreviewChange(state, CashTransferInput{HouseholdID: state.HouseholdID, FromAccountID: source, ToAccountID: destination, Sent: amount, Received: amount, Fee: &fee})
			if err != nil {
				t.Fatal(err)
			}
			if preview.Resulting[0].Amount != "9898" || preview.Resulting[1].Amount != "100" || len(preview.Effects) != 3 || preview.Effects[2].Classification != ClassificationFee {
				t.Fatalf("asset transfer result = %+v", preview)
			}
		})
	}
}
