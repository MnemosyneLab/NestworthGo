package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestRecordChangeWritesActivityEffectsProjectionAndClosedDayDirtyState(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/change.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	originNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := originNow
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Changes", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "10000", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = originNow.Add(48 * time.Hour)
	amount, _ := domain.ParseMoney("500", "CNY")
	preview, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: originNow.Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Activity.Kind != domain.ActivityCashIn || preview.Resulting[0].Amount != "10500" {
		t.Fatalf("recorded preview = %+v", preview)
	}
	var activities, effects, projections, replays int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activity_effects").Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM account_values WHERE projection_kind = 'event'").Scan(&projections); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM account_values WHERE projection_kind = 'replay'").Scan(&replays); err != nil {
		t.Fatal(err)
	}
	if activities != 1 || effects != 1 || projections != 1 || replays != 1 {
		t.Fatalf("persisted activity=%d effects=%d projections=%d replays=%d", activities, effects, projections, replays)
	}
	var dirty string
	if err := database.SQL.QueryRow("SELECT dirty_from FROM history_snapshot_state WHERE household_id = ?", bootstrap.Household.ID.String()).Scan(&dirty); err != nil {
		t.Fatal(err)
	}
	if dirty != "2026-01-02" {
		t.Fatalf("dirty_from = %q, want 2026-01-02", dirty)
	}
	if _, err := service.AppendAccountValue(ctx, account.Account.ID, "10500", "2026-01-04"); err == nil || err.(*domain.Error).Code != domain.ErrInvalidChangeTime {
		t.Fatalf("future/pre-origin append error = %v", err)
	}
}

func TestRecordChangeRequiresStartingPointAndDoesNotCallProviders(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/change-gate.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Gate", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	household, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	amount, _ := domain.ParseMoney("1", "CNY")
	_, err = service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: household.Household.ID, AccountID: domain.AccountID("00000000-0000-4000-8000-000000000001"), Amount: amount})
	if err == nil || err.(*domain.Error).Code != domain.ErrHistoryNotStarted {
		t.Fatalf("pre-history record error = %v", err)
	}
	var activities int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if activities != 0 {
		t.Fatalf("pre-history record wrote %d activities", activities)
	}
}

func TestRecordTradeUpdatesCashAndQuantityAndPersistsTradeDetail(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/trade.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Trading", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "100", "2026-01-01", false); err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "3"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "1000", "USD", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	var startingCost string
	if err := database.SQL.QueryRow("SELECT COALESCE(unit_cost, '') FROM history_origin_components WHERE holding_id = ?", holding.ID.String()).Scan(&startingCost); err != nil {
		t.Fatal(err)
	}
	if startingCost != "100" {
		t.Fatalf("persisted Starting Point unit cost = %q, want 100", startingCost)
	}
	gross, _ := domain.ParseMoney("200", "USD")
	fee, _ := domain.ParseMoney("5", "USD")
	// A client may identify the trade by its Account + Instrument pair. The
	// existing active Holding must be reused instead of attempting to create a
	// duplicate Holding during RecordChange.
	preview, err := service.RecordChange(ctx, domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "2"), Gross: gross, Fee: &fee, EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Resulting[0].Amount != "795" || preview.Resulting[1].Quantity != "5" {
		t.Fatalf("buy result = %+v", preview.Resulting)
	}
	var side, unitPrice, feeAmount string
	if err := database.SQL.QueryRow("SELECT side, unit_price, fee_amount FROM activity_trade_details WHERE activity_id = ?", preview.Activity.ID.String()).Scan(&side, &unitPrice, &feeAmount); err != nil {
		t.Fatal(err)
	}
	if side != "buy" || unitPrice != "100" || feeAmount != "5" {
		t.Fatalf("trade detail = %s %s %s", side, unitPrice, feeAmount)
	}
	var cash, quantity string
	if err := database.SQL.QueryRow("SELECT amount FROM account_cash_values WHERE account_id = ? AND projection_kind = 'event' ORDER BY created_at DESC LIMIT 1", account.Account.ID.String()).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT quantity FROM holding_quantity_values WHERE holding_id = ? AND projection_kind = 'event' ORDER BY created_at DESC LIMIT 1", holding.ID.String()).Scan(&quantity); err != nil {
		t.Fatal(err)
	}
	if cash != "795" || quantity != "5" {
		t.Fatalf("persisted buy cash=%s quantity=%s", cash, quantity)
	}

	sellGross, _ := domain.ParseMoney("120", "USD")
	sellFee, _ := domain.ParseMoney("2", "USD")
	clock = clock.Add(time.Hour)
	sell, err := service.RecordChange(ctx, domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeSell, SettlementAccountID: account.Account.ID, HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "1"), Gross: sellGross, Fee: &sellFee, EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	if sell.Resulting[0].Amount != "913" || sell.Resulting[1].Quantity != "4" {
		t.Fatalf("sell result = %+v", sell.Resulting)
	}
}

func TestRecordCashDividendPersistsDetailWithoutChangingQuantityOrCostBasis(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/dividend.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	ctx := context.Background()
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Dividends", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "100", "2026-01-01", false); err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "50", "USD", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := repository.ListCostBasisEvents(ctx, holding.ID)
	if err != nil {
		t.Fatal(err)
	}

	amount, _ := domain.ParseMoney("25", "USD")
	preview, err := service.RecordChange(ctx, domain.CashDividendInput{HouseholdID: bootstrap.Household.ID, HoldingID: holding.ID, Amount: amount, EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Activity.Kind != domain.ActivityCashDividend || preview.Activity.DividendDetail == nil || preview.Activity.DividendDetail.HoldingID != holding.ID {
		t.Fatalf("recorded dividend = %+v", preview.Activity)
	}
	if preview.Resulting[0].Amount != "75" || preview.Resulting[0].Currency != "USD" {
		t.Fatalf("resulting cash = %+v", preview.Resulting)
	}

	var persistedHolding, persistedAmount, persistedCurrency string
	if err := database.SQL.QueryRow("SELECT holding_id, amount, currency FROM activity_dividend_details WHERE activity_id = ?", preview.Activity.ID.String()).Scan(&persistedHolding, &persistedAmount, &persistedCurrency); err != nil {
		t.Fatal(err)
	}
	if persistedHolding != holding.ID.String() || persistedAmount != "25" || persistedCurrency != "USD" {
		t.Fatalf("persisted dividend detail = %s %s %s", persistedHolding, persistedAmount, persistedCurrency)
	}

	loaded, err := repository.Holding(ctx, holding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Quantity.Canonical() != "10" {
		t.Fatalf("holding quantity = %s, want 10", loaded.Quantity.Canonical())
	}
	afterEvents, err := repository.ListCostBasisEvents(ctx, holding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("cost basis events changed from %d to %d", len(beforeEvents), len(afterEvents))
	}

	undo, err := service.UndoChange(ctx, preview.Activity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.Activity.Kind != domain.ActivityReversal || undo.Resulting[0].Amount != "50" {
		t.Fatalf("undo = %+v", undo)
	}
}

func TestRecordFirstBuyCreatesHoldingAndCommitsAtomically(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/first-buy.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	clock := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "First Buy", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "First ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "1000", "USD", "2026-01-02"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	gross, _ := domain.ParseMoney("200", "USD")
	preview, err := service.RecordChange(ctx, domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "2"), Gross: gross, EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	holdings, err := service.ListHoldings(ctx, account.Account.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(holdings) != 1 || holdings[0].Quantity.Canonical() != "2" {
		t.Fatalf("first buy holdings = %+v", holdings)
	}
	if preview.Activity.TradeDetail == nil || preview.Activity.TradeDetail.HoldingID != holdings[0].ID {
		t.Fatalf("first buy trade detail = %+v, holding = %s", preview.Activity.TradeDetail, holdings[0].ID)
	}
	var cash string
	if err := database.SQL.QueryRow("SELECT amount FROM account_cash_values WHERE account_id = ? AND projection_kind = 'event' ORDER BY created_at DESC LIMIT 1", account.Account.ID.String()).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	if cash != "800" {
		t.Fatalf("first buy cash = %s, want 800", cash)
	}
	var activityCount, effectCount int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activity_effects").Scan(&effectCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 1 || effectCount != 2 {
		t.Fatalf("first buy evidence activities=%d effects=%d", activityCount, effectCount)
	}
}

func TestRecordTransfersUseNativeEndpointsAndCommitAtomically(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/transfer.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	clock := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Transfers", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerIDs := []domain.MemberID{bootstrap.Members[0].ID}
	fromAccount, err := service.CreateAccount(ctx, AccountInput{Name: "From", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1000", OwnerIDs: ownerIDs})
	if err != nil {
		t.Fatal(err)
	}
	toAccount, err := service.CreateAccount(ctx, AccountInput{Name: "To", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", OwnerIDs: ownerIDs})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "100", "2026-02-01", false); err != nil {
		t.Fatal(err)
	}
	fromBroker, err := service.CreateAccount(ctx, AccountInput{Name: "From Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: ownerIDs})
	if err != nil {
		t.Fatal(err)
	}
	toBroker, err := service.CreateAccount(ctx, AccountInput{Name: "To Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: ownerIDs})
	if err != nil {
		t.Fatal(err)
	}
	fromHolding, err := service.CreateHolding(ctx, HoldingInput{AccountID: fromBroker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "3"})
	if err != nil {
		t.Fatal(err)
	}
	toHolding, err := service.CreateHolding(ctx, HoldingInput{AccountID: toBroker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	cny, _ := domain.ParseMoney("125", "CNY")
	cashTransfer, err := service.RecordChange(ctx, domain.CashTransferInput{HouseholdID: bootstrap.Household.ID, FromAccountID: fromAccount.Account.ID, ToAccountID: toAccount.Account.ID, Sent: cny, Received: cny, EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	if cashTransfer.Effects[0].Target != domain.EffectTargetAccountValue || cashTransfer.Resulting[0].Amount != "875" || cashTransfer.Resulting[1].Amount != "225" {
		t.Fatalf("cash transfer = %+v", cashTransfer)
	}
	positionTransfer, err := service.RecordChange(ctx, domain.PositionTransferInput{HouseholdID: bootstrap.Household.ID, FromHoldingID: fromHolding.ID, ToHoldingID: toHolding.ID, Quantity: mustQuantity(t, "2"), EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	if positionTransfer.Resulting[0].Quantity != "1" || positionTransfer.Resulting[1].Quantity != "3" {
		t.Fatalf("position transfer = %+v", positionTransfer.Resulting)
	}
	var activities, effects int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activity_effects").Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if activities != 2 || effects != 4 {
		t.Fatalf("transfer evidence activities=%d effects=%d", activities, effects)
	}
}

func TestUndoAndFixKeepEvidenceAppendOnly(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/correction.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	clock := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Corrections", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1000", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	amount, _ := domain.ParseMoney("100", "CNY")
	original, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	undo, err := service.UndoChange(ctx, original.Activity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.Activity.Kind != domain.ActivityReversal || undo.Resulting[0].Amount != "1000" {
		t.Fatalf("undo = %+v", undo)
	}
	if _, err := service.UndoChange(ctx, original.Activity.ID); err == nil || err.(*domain.Error).Code != domain.ErrAlreadyUndone {
		t.Fatalf("second undo error = %v", err)
	}

	second, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	replacementAmount, _ := domain.ParseMoney("80", "CNY")
	replacement, err := service.FixChange(ctx, second.Activity.ID, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: replacementAmount, Reason: domain.ReasonIncome, EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Resulting[0].Amount != "1080" {
		t.Fatalf("replacement = %+v", replacement)
	}
	var activities, groups, reversals int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activity_correction_groups").Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities WHERE reverses_activity_id IS NOT NULL").Scan(&reversals); err != nil {
		t.Fatal(err)
	}
	if activities != 5 || groups != 1 || reversals != 2 {
		t.Fatalf("correction evidence activities=%d groups=%d reversals=%d", activities, groups, reversals)
	}
}

// TestPreviewFixChangeMatchesFixChangeWithoutCommitting is a regression
// test for a bug found during manual verification: the Fix
// form's "Preview" step called plain PreviewChange, which ignores the
// original Activity being replaced and previews the replacement command
// against the state *after* that original effect already applied —
// double-counting it, since Confirm (FixChange) correctly inverts the
// original effect first. PreviewFixChange must return the exact number
// FixChange will actually commit, and must not write anything.
func TestPreviewFixChangeMatchesFixChangeWithoutCommitting(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/preview-fix.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	clock := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "PreviewFix", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Checking", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "5000", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	amount, _ := domain.ParseMoney("1000", "USD")
	original, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	replacementAmount, _ := domain.ParseMoney("1200", "USD")
	replacementCommand := domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: replacementAmount, Reason: domain.ReasonIncome, EffectiveAt: clock}

	preview, err := service.PreviewFixChange(ctx, original.Activity.ID, replacementCommand)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Resulting[0].Amount != "6200" {
		t.Fatalf("PreviewFixChange resulting = %+v, want 6200 (5000 initial - 1000 inverted + 1200 replacement)", preview.Resulting)
	}

	var activitiesAfterPreview int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activitiesAfterPreview); err != nil {
		t.Fatal(err)
	}
	if activitiesAfterPreview != 1 {
		t.Fatalf("PreviewFixChange must not commit anything; activities = %d, want 1 (only the original)", activitiesAfterPreview)
	}

	committed, err := service.FixChange(ctx, original.Activity.ID, replacementCommand)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Resulting[0].Amount != preview.Resulting[0].Amount {
		t.Fatalf("FixChange resulting = %+v, want it to match PreviewFixChange's %+v", committed.Resulting, preview.Resulting)
	}
}

func TestAppendEffectiveStateAndPreferenceObservations(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/observations.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	clock := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Observations", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := service.AppendAccountStateObservation(ctx, domain.AccountStateObservation{AccountID: account.Account.ID, EffectiveAt: clock, IncludeInNetWorth: true, IncludeInLiquidAssets: true, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}}); err != nil {
		t.Fatal(err)
	}
	if err := service.AppendInstrumentPreferenceObservation(ctx, domain.InstrumentPreferenceObservation{InstrumentID: instrument.ID, SourceKind: domain.QuoteSourceManual, EffectiveAt: clock}); err != nil {
		t.Fatal(err)
	}
	if err := service.AppendFXPreferenceObservation(ctx, domain.FXPreferenceObservation{HouseholdID: bootstrap.Household.ID, CurrencyA: domain.CurrencyCode("USD"), CurrencyB: domain.CurrencyCode("CNY"), SourceKind: domain.QuoteSourceManual, EffectiveAt: clock}); err != nil {
		t.Fatal(err)
	}
	var accountStates, ownership, instrumentPreferences, fxPreferences int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM account_state_observations").Scan(&accountStates); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM account_state_ownership").Scan(&ownership); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM instrument_preference_observations").Scan(&instrumentPreferences); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM fx_preference_observations").Scan(&fxPreferences); err != nil {
		t.Fatal(err)
	}
	if accountStates != 1 || ownership != 1 || instrumentPreferences != 1 || fxPreferences != 1 {
		t.Fatalf("observation counts = account=%d ownership=%d instrument=%d fx=%d", accountStates, ownership, instrumentPreferences, fxPreferences)
	}
}

func TestHistoricalSnapshotUsesOriginAndActivitiesAndSkipsUnchangedRevision(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/snapshots.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	clock := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Snapshots", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1000", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	first, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-05-01")
	if err != nil || !appended || first.NetWorthAmount == nil || first.NetWorthAmount.CanonicalAmount() != "1000" {
		t.Fatalf("first snapshot=%+v appended=%v err=%v", first, appended, err)
	}
	_, appended, err = service.BuildDailyValuationSnapshot(ctx, "2026-05-01")
	if err != nil || appended {
		t.Fatalf("unchanged snapshot appended=%v err=%v", appended, err)
	}
	amount, _ := domain.ParseMoney("500", "CNY")
	if _, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	second, appended, err := service.BuildDailyValuationSnapshot(ctx, "2026-05-02")
	if err != nil || !appended || second.NetWorthAmount == nil || second.NetWorthAmount.CanonicalAmount() != "1500" {
		t.Fatalf("second snapshot=%+v appended=%v err=%v", second, appended, err)
	}
	var mayOne, mayTwo, revisions int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM daily_valuation_snapshots WHERE local_date = '2026-05-01'").Scan(&mayOne); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM daily_valuation_snapshots WHERE local_date = '2026-05-02'").Scan(&mayTwo); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM daily_valuation_snapshots WHERE local_date = '2026-05-02' AND revision = 1").Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if mayOne != 1 || mayTwo != 1 || revisions != 1 {
		t.Fatalf("snapshot revisions may1=%d may2=%d revision1=%d", mayOne, mayTwo, revisions)
	}
	trend, err := service.NetWorthTrend(ctx, domain.TrendAllTime)
	if err != nil || len(trend.Points) != 3 || trend.Points[0].Value == nil || trend.Points[0].Value.CanonicalAmount() != "1000" || trend.Points[1].Value == nil || trend.Points[1].Value.CanonicalAmount() != "1500" {
		t.Fatalf("trend = %+v err=%v", trend, err)
	}
}

func TestCompositeCashAcceptsForeignCurrencyBeforeAndAfterHistory(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/composite-cash.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "MooMoo", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	brokerage, err := service.CreateAccount(ctx, AccountInput{
		Name: "MooMoo SG", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "SGD", IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	simple, err := service.CreateAccount(ctx, AccountInput{
		Name: "DBS Savings", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "SGD", InitialAmount: "100",
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, brokerage.Account.ID, "12000", "SGD", "2026-08-01"); err != nil {
		t.Fatalf("history-before SGD cash: %v", err)
	}
	if _, err := service.AppendAccountCashValue(ctx, brokerage.Account.ID, "8500", "USD", "2026-08-01"); err != nil {
		t.Fatalf("history-before USD cash: %v", err)
	}
	if _, err := service.AppendAccountCashValue(ctx, brokerage.Account.ID, "3000", "CNY", "2026-08-01"); err != nil {
		t.Fatalf("history-before CNY cash: %v", err)
	}
	cashBefore, err := service.ListAccountCashValues(ctx, brokerage.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cashBefore) != 3 {
		t.Fatalf("history-before cash observations = %d, want 3", len(cashBefore))
	}
	clock = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	reconciled, err := service.AppendAccountCashValue(ctx, brokerage.Account.ID, "9000", "USD", "2026-08-03")
	if err != nil {
		t.Fatalf("history-after USD reconcile: %v", err)
	}
	if reconciled.Amount.CanonicalAmount() != "9000" || reconciled.Amount.Currency() != "USD" {
		t.Fatalf("reconciled cash = %+v", reconciled)
	}
	added, _ := domain.ParseMoney("200", "CNY")
	preview, err := service.RecordChange(ctx, domain.MoneyAddedInput{
		HouseholdID: bootstrap.Household.ID, AccountID: brokerage.Account.ID, Amount: added,
		Reason: domain.ReasonContribution, EffectiveAt: clock,
	})
	if err != nil {
		t.Fatalf("history-after CNY deposit: %v", err)
	}
	if preview.Activity.Kind != domain.ActivityCashIn || preview.Resulting[0].Currency != "CNY" || preview.Resulting[0].Amount != "3200" {
		t.Fatalf("CNY deposit preview = %+v", preview)
	}
	foreign, _ := domain.ParseMoney("10", "USD")
	_, err = service.RecordChange(ctx, domain.MoneyAddedInput{
		HouseholdID: bootstrap.Household.ID, AccountID: simple.Account.ID, Amount: foreign,
		Reason: domain.ReasonIncome, EffectiveAt: clock,
	})
	if err == nil {
		t.Fatal("simple account accepted a foreign-currency deposit")
	}
	if domainErr, ok := err.(*domain.Error); !ok || domainErr.Code != domain.ErrInvalidChange {
		t.Fatalf("simple foreign deposit error = %v, want invalid change", err)
	}
}

func TestMixedBankFirstFundBuyDecreasesCashAndShowsHolding(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "cmb-mixed-first-buy", []string{"Owner"})
	bank, err := service.CreateAccount(ctx, AccountInput{
		Name: "招商银行综合账户", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bank.Account.TrackingMode != domain.TrackingHoldings {
		t.Fatalf("tracking = %s, want holdings", bank.Account.TrackingMode)
	}
	if _, err := service.AppendAccountCashValue(ctx, bank.Account.ID, "10000", "CNY", "2026-08-01"); err != nil {
		t.Fatalf("opening CNY cash: %v", err)
	}
	fund, err := service.CreateInstrument(ctx, InstrumentInput{Name: "招银理财A", Type: "mutual_fund", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	tradeAt := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	setClock(tradeAt)
	gross, _ := domain.ParseMoney("2000", "CNY")
	preview, err := service.RecordChange(ctx, domain.TradeInput{
		HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: bank.Account.ID,
		InstrumentID: fund.ID, Quantity: mustQuantity(t, "100"), Gross: gross, EffectiveAt: tradeAt,
	})
	if err != nil {
		t.Fatalf("first fund buy: %v", err)
	}
	holdings, err := service.ListHoldings(ctx, bank.Account.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(holdings) != 1 || holdings[0].InstrumentID != fund.ID || holdings[0].Quantity.Canonical() != "100" {
		t.Fatalf("holdings after first buy = %+v", holdings)
	}
	if preview.Activity.TradeDetail == nil || preview.Activity.TradeDetail.HoldingID != holdings[0].ID {
		t.Fatalf("trade detail = %+v, holding = %s", preview.Activity.TradeDetail, holdings[0].ID)
	}

	valuations, err := service.AccountValuations(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var valuation *domain.AccountValuation
	for i := range valuations {
		if valuations[i].Account.ID == bank.Account.ID {
			valuation = &valuations[i]
			break
		}
	}
	if valuation == nil {
		t.Fatal("mixed bank valuation missing")
	}
	var cashAmount string
	var sawHolding bool
	for _, component := range valuation.Components {
		if component.InstrumentID == nil {
			if component.NativeCurrency == "CNY" {
				cashAmount = component.NativeAmount
			}
			continue
		}
		if *component.InstrumentID == fund.ID {
			sawHolding = true
			if component.Available {
				t.Fatal("missing fund quote must not be treated as an available market value")
			}
		}
	}
	if cashAmount != "8000" {
		t.Fatalf("CNY cash after first buy = %s, want 8000", cashAmount)
	}
	if !sawHolding {
		t.Fatal("fund holding was not visible on the mixed bank valuation")
	}
	if valuation.Complete {
		t.Fatal("valuation without a fund quote must stay incomplete")
	}
}

func mustQuantity(t *testing.T, value string) domain.Quantity {
	t.Helper()
	quantity, err := domain.ParseQuantity(value)
	if err != nil {
		t.Fatal(err)
	}
	return quantity
}
