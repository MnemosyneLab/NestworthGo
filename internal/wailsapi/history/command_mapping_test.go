package history

import (
	"fmt"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// commandKindActivity maps every ChangeCommandKind onto the ActivityKind
// PreviewChange records for that command. Keep this table in lockstep with
// frontend/src/features/history/activityToCommand.ts.
var commandKindActivity = []struct {
	kind     ChangeCommandKind
	activity domain.ActivityKind
}{
	{ChangeMoneyAdded, domain.ActivityCashIn},
	{ChangeMoneyRemoved, domain.ActivityCashOut},
	{ChangeCashTransfer, domain.ActivityCashTransfer},
	{ChangeFXConversion, domain.ActivityFXConversion},
	{ChangePositionTransfer, domain.ActivityPositionTransfer},
	{ChangePositionAdjustment, domain.ActivityPositionTransfer},
	{ChangeTrade, domain.ActivityBuy},
	{ChangeValueUpdate, domain.ActivityValueUpdate},
	{ChangeDebtDraw, domain.ActivityDebtDraw},
	{ChangeDebtPayment, domain.ActivityDebtPayment},
}

func TestToCommandAcceptsEveryChangeCommandKind(t *testing.T) {
	householdID := domain.NewHouseholdID()
	accountID := domain.NewAccountID().String()
	otherAccountID := domain.NewAccountID().String()
	holdingID := domain.NewHoldingID().String()
	otherHoldingID := domain.NewHoldingID().String()
	instrumentID := domain.NewInstrumentID().String()

	requests := map[ChangeCommandKind]ChangeCommandRequest{
		ChangeMoneyAdded:         {Kind: ChangeMoneyAdded, AccountID: accountID, Amount: "1", Currency: "USD"},
		ChangeMoneyRemoved:       {Kind: ChangeMoneyRemoved, AccountID: accountID, Amount: "1", Currency: "USD"},
		ChangeCashTransfer:       {Kind: ChangeCashTransfer, FromAccountID: accountID, ToAccountID: otherAccountID, Sent: "1", SentCurrency: "USD", Received: "1", ReceivedCurrency: "USD"},
		ChangeFXConversion:       {Kind: ChangeFXConversion, AccountID: accountID, Sold: "1", SoldCurrency: "USD", Bought: "1", BoughtCurrency: "CNY"},
		ChangePositionTransfer:   {Kind: ChangePositionTransfer, FromHoldingID: holdingID, ToHoldingID: otherHoldingID, Quantity: "1"},
		ChangePositionAdjustment: {Kind: ChangePositionAdjustment, HoldingID: holdingID, Quantity: "1", Added: true},
		ChangeTrade:              {Kind: ChangeTrade, SettlementAccountID: accountID, InstrumentID: instrumentID, Side: "buy", Quantity: "1", Gross: "1", GrossCurrency: "USD"},
		ChangeValueUpdate:        {Kind: ChangeValueUpdate, AccountID: accountID, NewValue: "1", NewValueCurrency: "USD"},
		ChangeDebtDraw:           {Kind: ChangeDebtDraw, DebtAccountID: accountID, CashAccountID: otherAccountID, Principal: "1", PrincipalCurrency: "USD"},
		ChangeDebtPayment:        {Kind: ChangeDebtPayment, DebtAccountID: accountID, CashAccountID: otherAccountID, Principal: "1", PrincipalCurrency: "USD"},
	}

	if len(requests) != len(commandKindActivity) {
		t.Fatalf("request fixtures = %d, mapping table = %d", len(requests), len(commandKindActivity))
	}
	for _, mapping := range commandKindActivity {
		request, ok := requests[mapping.kind]
		if !ok {
			t.Fatalf("no ToCommand fixture for %s", mapping.kind)
		}
		command, err := request.ToCommand(householdID)
		if err != nil {
			t.Fatalf("%s: %v", mapping.kind, err)
		}
		if command == nil {
			t.Fatalf("%s: ToCommand returned nil", mapping.kind)
		}
		if got := fmt.Sprintf("%T", command); got == "" {
			t.Fatalf("%s: empty command type", mapping.kind)
		}
	}
}

func TestCommandKindActivityTableCoversEveryKind(t *testing.T) {
	covered := map[ChangeCommandKind]bool{}
	for _, mapping := range commandKindActivity {
		if covered[mapping.kind] {
			t.Fatalf("duplicate mapping for %s", mapping.kind)
		}
		covered[mapping.kind] = true
	}
	for _, kind := range []ChangeCommandKind{
		ChangeMoneyAdded, ChangeMoneyRemoved, ChangeCashTransfer, ChangeFXConversion,
		ChangePositionTransfer, ChangePositionAdjustment, ChangeTrade, ChangeValueUpdate,
		ChangeDebtDraw, ChangeDebtPayment,
	} {
		if !covered[kind] {
			t.Fatalf("mapping table missing %s", kind)
		}
	}
}
