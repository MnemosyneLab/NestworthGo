package history

import (
	"fmt"
	"testing"
	"time"

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
	{ChangeCashDividend, domain.ActivityCashDividend},
	{ChangeCashTransfer, domain.ActivityCashTransfer},
	{ChangeFXConversion, domain.ActivityFXConversion},
	{ChangePositionTransfer, domain.ActivityPositionTransfer},
	{ChangePositionAdjustment, domain.ActivityPositionTransfer},
	{ChangeTrade, domain.ActivityBuy},
	{ChangeValueUpdate, domain.ActivityValueUpdate},
	{ChangeDebtDraw, domain.ActivityDebtDraw},
	{ChangeDebtPayment, domain.ActivityDebtPayment},
}

func TestToCommandResolvesLocalDateTimeInHistoryOriginTimezone(t *testing.T) {
	householdID := domain.NewHouseholdID()
	accountID := domain.NewAccountID().String()
	request := ChangeCommandRequest{
		Kind: ChangeMoneyAdded, AccountID: accountID, Amount: "1", Currency: "USD", Reason: "other",
		EffectiveAt: "2026-01-01T00:00:00Z", EffectiveLocalDate: "2026-08-23", EffectiveLocalTime: "20:00",
	}
	command, err := request.ToCommand(householdID, "Asia/Shanghai")
	if err != nil {
		t.Fatalf("ToCommand: %v", err)
	}
	input, ok := command.(domain.MoneyAddedInput)
	if !ok {
		t.Fatalf("command = %T, want domain.MoneyAddedInput", command)
	}
	want := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	if !input.EffectiveAt.Equal(want) {
		t.Fatalf("EffectiveAt = %s, want %s", input.EffectiveAt, want)
	}
}

func TestToCommandRejectsPartialOrDSTInvalidLocalDateTime(t *testing.T) {
	householdID := domain.NewHouseholdID()
	base := ChangeCommandRequest{Kind: ChangeMoneyAdded, AccountID: domain.NewAccountID().String(), Amount: "1", Currency: "USD", Reason: "other"}
	for _, testCase := range []struct {
		name  string
		date  string
		clock string
	}{
		{name: "partial", date: "2026-08-23"},
		{name: "DST gap", date: "2026-03-08", clock: "02:30"},
		{name: "DST repeat", date: "2026-11-01", clock: "01:30"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := base
			request.EffectiveLocalDate = testCase.date
			request.EffectiveLocalTime = testCase.clock
			_, err := request.ToCommand(householdID, "America/New_York")
			if err == nil {
				t.Fatal("ToCommand error = nil")
			}
			if typed, ok := err.(*domain.Error); !ok || typed.Code != domain.ErrInvalidChangeTime {
				t.Fatalf("ToCommand error = %T %v, want ErrInvalidChangeTime", err, err)
			}
		})
	}
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
		ChangeCashDividend:       {Kind: ChangeCashDividend, HoldingID: holdingID, Amount: "1", Currency: "USD"},
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
		ChangeMoneyAdded, ChangeMoneyRemoved, ChangeCashDividend, ChangeCashTransfer, ChangeFXConversion,
		ChangePositionTransfer, ChangePositionAdjustment, ChangeTrade, ChangeValueUpdate,
		ChangeDebtDraw, ChangeDebtPayment,
	} {
		if !covered[kind] {
			t.Fatalf("mapping table missing %s", kind)
		}
	}
}
