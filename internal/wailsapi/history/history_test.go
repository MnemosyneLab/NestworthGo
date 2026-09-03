package history_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type fixture struct {
	app          *application.Service
	service      *history.Service
	checkingID   string
	savingsID    string
	brokerageID  string
	brokerage2ID string
	cardID       string
	instrumentID string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	app := wailstest.NewService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"}, Timezone: "UTC",
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	bootstrap, err := household.NewService(app).Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	memberID := bootstrap.Members[0].ID
	accountService := account.NewService(app)
	create := func(name, primary, secondary, mode string, includeInPortfolio bool, initial string) string {
		record, err := accountService.CreateAccount(ctx, account.CreateAccountRequest{
			Name: name, AccountType: primary, BalanceSheetRole: secondary, TrackingMode: mode,
			DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: includeInPortfolio,
			OwnerIDs: []string{memberID}, InitialAmount: initial,
		})
		if err != nil {
			t.Fatalf("CreateAccount(%s): %v", name, err)
		}
		return record.Account.ID
	}
	checkingID := create("Checking", "bank_account", "asset", "balance", false, "0")
	savingsID := create("Savings", "bank_account", "asset", "balance", false, "0")
	brokerageID := create("Brokerage", "brokerage", "asset", "holdings", true, "")
	brokerage2ID := create("Brokerage2", "brokerage", "asset", "holdings", true, "")
	cardID := create("Card", "credit_card", "liability", "balance", false, "0")

	instrumentDTO, err := instrument.NewService(app).CreateInstrument(ctx, instrument.InstrumentRequest{
		Name: "NVIDIA", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}

	return fixture{
		app: app, service: history.NewService(app), checkingID: checkingID, savingsID: savingsID,
		brokerageID: brokerageID, brokerage2ID: brokerage2ID, cardID: cardID, instrumentID: instrumentDTO.ID,
	}
}

func TestHistoryOriginStartedByOnboarding(t *testing.T) {
	fx := newFixture(t)
	started, err := fx.service.HistoryStarted(context.Background())
	if err != nil {
		t.Fatalf("HistoryStarted: %v", err)
	}
	if !started {
		t.Fatal("HistoryStarted = false, want true (onboarding supplied a timezone)")
	}
	origin, err := fx.service.HistoryOrigin(context.Background())
	if err != nil {
		t.Fatalf("HistoryOrigin: %v", err)
	}
	if origin == nil || origin.Timezone != "UTC" {
		t.Fatalf("origin = %+v, want UTC", origin)
	}
}

// TestChangeCommandUnionRoundTripsEveryKind verifies that the
// HistoryService change-command union round-trips every change kind
// currently supported by domain.PreviewChange. It exercises all eleven
// domain.PreviewChange input kinds through history.Service.RecordChange.
func TestChangeCommandUnionRoundTripsEveryKind(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	record := func(name string, request history.ChangeCommandRequest) history.ChangeCommandRequest {
		preview, err := fx.service.RecordChange(ctx, request)
		if err != nil {
			t.Fatalf("RecordChange(%s): %v", name, err)
		}
		if preview.Activity.ID == "" {
			t.Fatalf("RecordChange(%s): empty activity ID in result", name)
		}
		encoded, err := json.Marshal(preview)
		if err != nil {
			t.Fatalf("RecordChange(%s): json.Marshal(ChangePreviewDTO): %v", name, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("RecordChange(%s): json.Unmarshal: %v", name, err)
		}
		if len(decoded) == 0 {
			t.Fatalf("RecordChange(%s): DTO round-trip produced an empty object", name)
		}
		return request
	}

	record("money_added", history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "1000", Currency: "USD", Reason: "contribution",
	})
	record("money_removed", history.ChangeCommandRequest{
		Kind: history.ChangeMoneyRemoved, AccountID: fx.checkingID, Amount: "200", Currency: "USD", Reason: "expense",
	})
	record("value_update", history.ChangeCommandRequest{
		Kind: history.ChangeValueUpdate, AccountID: fx.checkingID, NewValue: "900", NewValueCurrency: "USD", Reason: "reconciliation",
	})
	record("cash_transfer", history.ChangeCommandRequest{
		Kind: history.ChangeCashTransfer, FromAccountID: fx.checkingID, ToAccountID: fx.savingsID,
		Sent: "300", SentCurrency: "USD", Received: "300", ReceivedCurrency: "USD",
	})
	record("money_added_brokerage_cash", history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: fx.brokerageID, Amount: "5000", Currency: "USD", Reason: "contribution",
	})
	record("fx_conversion", history.ChangeCommandRequest{
		Kind: history.ChangeFXConversion, AccountID: fx.brokerageID,
		Sold: "1000", SoldCurrency: "USD", Bought: "920", BoughtCurrency: "EUR",
	})
	tradePreview, err := fx.service.RecordChange(ctx, history.ChangeCommandRequest{
		Kind: history.ChangeTrade, SettlementAccountID: fx.brokerageID, InstrumentID: fx.instrumentID,
		Side: "buy", Quantity: "10", Gross: "1000", GrossCurrency: "USD", Fee: "5", FeeCurrency: "USD",
	})
	if err != nil {
		t.Fatalf("RecordChange(trade): %v", err)
	}
	if tradePreview.Activity.TradeDetail == nil {
		t.Fatalf("trade activity has no TradeDetail: %+v", tradePreview.Activity)
	}
	holdingAID := tradePreview.Activity.TradeDetail.HoldingID

	dividendPreview, err := fx.service.RecordChange(ctx, history.ChangeCommandRequest{
		Kind: history.ChangeCashDividend, HoldingID: holdingAID, Amount: "15", Currency: "USD",
	})
	if err != nil {
		t.Fatalf("RecordChange(cash_dividend): %v", err)
	}
	if dividendPreview.Activity.DividendDetail == nil || dividendPreview.Activity.DividendDetail.HoldingID != holdingAID {
		t.Fatalf("dividend activity has no DividendDetail: %+v", dividendPreview.Activity)
	}

	holdingBDTO, err := holding.NewService(fx.app).CreateHolding(ctx, holding.CreateHoldingRequest{
		AccountID: fx.brokerage2ID, InstrumentID: fx.instrumentID, Quantity: "0",
	})
	if err != nil {
		t.Fatalf("CreateHolding (second holding, zero quantity): %v", err)
	}

	record("position_transfer", history.ChangeCommandRequest{
		Kind: history.ChangePositionTransfer, FromHoldingID: holdingAID, ToHoldingID: holdingBDTO.ID, Quantity: "3",
	})
	record("position_adjustment", history.ChangeCommandRequest{
		Kind: history.ChangePositionAdjustment, HoldingID: holdingBDTO.ID, Quantity: "2", Added: true, UnitCost: "50",
	})
	record("debt_draw", history.ChangeCommandRequest{
		Kind: history.ChangeDebtDraw, DebtAccountID: fx.cardID, CashAccountID: fx.checkingID, Principal: "300", PrincipalCurrency: "USD",
	})
	record("debt_payment", history.ChangeCommandRequest{
		Kind: history.ChangeDebtPayment, DebtAccountID: fx.cardID, CashAccountID: fx.checkingID,
		Principal: "100", PrincipalCurrency: "USD", InterestOrFee: "10", InterestOrFeeCurrency: "USD",
	})

	activities, err := fx.service.ListActivities(ctx, 0)
	if err != nil {
		t.Fatalf("ListActivities: %v", err)
	}
	// 12 recorded activities: the eleven domain.PreviewChange command kinds
	// plus one extra money_added call (funding the Brokerage cash
	// sub-ledger) needed to make the fx_conversion/trade commands valid.
	if len(activities) != 12 {
		t.Fatalf("ListActivities returned %d activities, want 12", len(activities))
	}
	var listedTrade *wire.ActivityDTO
	var listedDividend *wire.ActivityDTO
	for index := range activities {
		if activities[index].TradeDetail != nil {
			listedTrade = &activities[index]
		}
		if activities[index].DividendDetail != nil {
			listedDividend = &activities[index]
		}
	}
	if listedTrade == nil || listedTrade.TradeDetail.HoldingID != holdingAID {
		t.Fatalf("ListActivities omitted TradeDetail on the buy: %+v", activities)
	}
	if listedDividend == nil || listedDividend.DividendDetail.HoldingID != holdingAID {
		t.Fatalf("ListActivities omitted DividendDetail: %+v", activities)
	}
}

func TestPreviewChangeDoesNotCommit(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	if _, err := fx.service.PreviewChange(ctx, history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "1000", Currency: "USD", Reason: "contribution",
	}); err != nil {
		t.Fatalf("PreviewChange: %v", err)
	}
	activities, err := fx.service.ListActivities(ctx, 0)
	if err != nil {
		t.Fatalf("ListActivities: %v", err)
	}
	if len(activities) != 0 {
		t.Fatalf("PreviewChange committed %d activities, want 0", len(activities))
	}
}

func TestUndoChange(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	recorded, err := fx.service.RecordChange(ctx, history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "1000", Currency: "USD", Reason: "contribution",
	})
	if err != nil {
		t.Fatalf("RecordChange: %v", err)
	}
	undone, err := fx.service.UndoChange(ctx, recorded.Activity.ID)
	if err != nil {
		t.Fatalf("UndoChange: %v", err)
	}
	if undone.Activity.Kind != "reversal" {
		t.Fatalf("undone.Activity.Kind = %q, want reversal", undone.Activity.Kind)
	}
	if undone.Activity.ReversesActivityID == nil || *undone.Activity.ReversesActivityID != recorded.Activity.ID {
		t.Fatalf("undone.Activity.ReversesActivityID = %v, want %q", undone.Activity.ReversesActivityID, recorded.Activity.ID)
	}
	loaded, err := fx.service.Activity(ctx, recorded.Activity.ID)
	if err != nil {
		t.Fatalf("Activity(original): %v", err)
	}
	if loaded.ID != recorded.Activity.ID || len(loaded.Effects) != 1 || loaded.Effects[0].AccountID == nil || *loaded.Effects[0].AccountID != fx.checkingID {
		t.Fatalf("Activity(original) = %+v, want the original activity with its Checking effect", loaded)
	}
	_, err = fx.service.UndoChange(ctx, recorded.Activity.ID)
	if err == nil {
		t.Fatal("second UndoChange on the same activity should fail")
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok || wireErr.Code != "already_undone" {
		t.Fatalf("err = %v, want already_undone", err)
	}
}

func TestFixChangeReplacesWithCorrectionGroup(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	recorded, err := fx.service.RecordChange(ctx, history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "1000", Currency: "USD", Reason: "contribution",
	})
	if err != nil {
		t.Fatalf("RecordChange: %v", err)
	}
	fixed, err := fx.service.FixChange(ctx, recorded.Activity.ID, history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "1200", Currency: "USD", Reason: "contribution",
	})
	if err != nil {
		t.Fatalf("FixChange: %v", err)
	}
	if fixed.Activity.CorrectionGroupID == nil {
		t.Fatalf("fixed.Activity.CorrectionGroupID is nil, want a correction group")
	}
	if len(fixed.Resulting) != 1 || fixed.Resulting[0].Amount != "1200" {
		t.Fatalf("Resulting = %+v, want 1200", fixed.Resulting)
	}
}

// TestPreviewFixChangeMatchesFixChange is a regression test for the Fix
// form's "Preview" step: it must call PreviewFixChange, not plain
// PreviewChange (which double-counts the original Activity's effect,
// since it does not know a Fix is in progress), and PreviewFixChange
// must not commit anything.
func TestPreviewFixChangeMatchesFixChange(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	recorded, err := fx.service.RecordChange(ctx, history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "1000", Currency: "USD", Reason: "contribution",
	})
	if err != nil {
		t.Fatalf("RecordChange: %v", err)
	}
	replacement := history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "1200", Currency: "USD", Reason: "contribution",
	}

	before, err := fx.service.ListActivities(ctx, 100)
	if err != nil {
		t.Fatalf("ListActivities before preview: %v", err)
	}
	preview, err := fx.service.PreviewFixChange(ctx, recorded.Activity.ID, replacement)
	if err != nil {
		t.Fatalf("PreviewFixChange: %v", err)
	}
	if len(preview.Resulting) != 1 || preview.Resulting[0].Amount != "1200" {
		t.Fatalf("PreviewFixChange.Resulting = %+v, want 1200 (checking started at 0)", preview.Resulting)
	}
	after, err := fx.service.ListActivities(ctx, 100)
	if err != nil {
		t.Fatalf("ListActivities after preview: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("PreviewFixChange must not commit anything; activity count went from %d to %d", len(before), len(after))
	}

	fixed, err := fx.service.FixChange(ctx, recorded.Activity.ID, replacement)
	if err != nil {
		t.Fatalf("FixChange: %v", err)
	}
	if fixed.Resulting[0].Amount != preview.Resulting[0].Amount {
		t.Fatalf("FixChange.Resulting = %+v, want it to match PreviewFixChange's %+v", fixed.Resulting, preview.Resulting)
	}
}

func TestChangeCommandUnsupportedKind(t *testing.T) {
	fx := newFixture(t)
	_, err := fx.service.RecordChange(context.Background(), history.ChangeCommandRequest{Kind: "not_a_kind"})
	if err == nil {
		t.Fatal("want an error for an unsupported change kind")
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok || wireErr.Code != "invalid_change" {
		t.Fatalf("err = %v, want invalid_change", err)
	}
}

func TestChangeCommandInvalidAccountID(t *testing.T) {
	fx := newFixture(t)
	_, err := fx.service.RecordChange(context.Background(), history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: "not-a-uuid", Amount: "1000", Currency: "USD",
	})
	wireErr, ok := apierror.Parse(err.Error())
	if !ok || wireErr.Code != "validation" {
		t.Fatalf("err = %v, want validation", err)
	}
}

func TestListActivityPageDateValidation(t *testing.T) {
	fx := newFixture(t)
	_, err := fx.service.ListActivityPage(context.Background(), history.ActivityQueryRequest{FromLocalDate: "not-a-date"})
	if err == nil {
		t.Fatal("want a validation error for a malformed date")
	}
}

func TestDailySnapshotStateAndBuild(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	bootstrap, err := household.NewService(fx.app).Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	state, err := fx.service.DailySnapshotState(ctx, bootstrap.Household.ID)
	if err != nil {
		t.Fatalf("DailySnapshotState: %v", err)
	}
	if state.HouseholdID != bootstrap.Household.ID {
		t.Fatalf("HouseholdID = %q, want %q", state.HouseholdID, bootstrap.Household.ID)
	}
	// BuildDailyValuationSnapshot only accepts an already-closed local day
	// strictly after the Starting point. Since this test cannot control
	// application.Service's clock from outside the application package
	// (setClock is unexported; internal/application's own tests use it,
	// but this is an external wailsapi test), every candidate date is
	// either "today" (not yet closed) or before the Starting point (which
	// this fixture set to "now" via onboarding). This test therefore
	// verifies the adapter's error-wrapping wiring for "today" rather than
	// a happy-path build; the happy path is exercised by
	// internal/application's own BuildDailyValuationSnapshot tests, which
	// this migration must not duplicate or weaken.
	today := time.Now().UTC().Format("2006-01-02")
	_, _, err = fx.service.BuildDailyValuationSnapshot(ctx, today)
	if err == nil {
		t.Fatal("want an error for a not-yet-closed local day")
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok || wireErr.Code != "invalid_change_time" {
		t.Fatalf("err = %v, want invalid_change_time", err)
	}
}

func TestRecordChangeMutationIDIsIdempotent(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	mutationID := "11111111-1111-4111-8111-111111111111"
	request := history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: fx.checkingID, Amount: "25", Currency: "USD", Reason: "contribution", MutationID: mutationID,
	}
	first, err := fx.service.RecordChange(ctx, request)
	if err != nil {
		t.Fatalf("first RecordChange: %v", err)
	}
	replay, err := fx.service.RecordChange(ctx, request)
	if err != nil {
		t.Fatalf("replay RecordChange: %v", err)
	}
	if replay.Activity.ID != first.Activity.ID {
		t.Fatalf("replay activity ID = %s, want %s", replay.Activity.ID, first.Activity.ID)
	}
	conflict := request
	conflict.Amount = "50"
	_, err = fx.service.RecordChange(ctx, conflict)
	if err == nil {
		t.Fatal("expected conflict for same mutation ID and different payload")
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok || wireErr.Code != "conflict" || wireErr.Field != "mutationId" {
		t.Fatalf("err = %v, want conflict mutationId", err)
	}
}
