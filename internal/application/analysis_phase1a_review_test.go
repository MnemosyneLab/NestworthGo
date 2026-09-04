package application

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// Phase 1a review cases deliberately use domain.PreviewChange and
// domain.ApplyEffects to create the Activity evidence. This keeps the tests
// on the same write path that RecordChange uses instead of hand-authoring a
// shape the ledger could never persist.

func reviewChangeState(t *testing.T, householdID domain.HouseholdID, accounts ...domain.ChangeAccountState) domain.ChangeState {
	t.Helper()
	state := domain.ChangeState{
		HouseholdID: householdID,
		OriginAt:    time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Timezone:    "UTC",
		Now:         time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC),
		Accounts:    make(map[domain.AccountID]domain.ChangeAccountState, len(accounts)),
		Cash:        make(map[domain.AccountID]map[domain.CurrencyCode]domain.Money),
		Holdings:    make(map[domain.HoldingID]domain.ChangeHoldingState),
		Instruments: make(map[domain.InstrumentID]domain.Instrument),
	}
	for _, account := range accounts {
		state.Accounts[account.ID] = account
	}
	return state
}

func reviewAccountState(t *testing.T, account domain.Account, current string) domain.ChangeAccountState {
	t.Helper()
	money := mustMoney(t, current, account.DefaultCurrency.String())
	return domain.ChangeAccountState{ID: account.ID, Name: account.Name, Currency: account.DefaultCurrency, Mode: account.TrackingMode, Liability: account.IsLiability(), Current: money}
}

func reviewApplyChange(t *testing.T, state *domain.ChangeState, command any) domain.Activity {
	t.Helper()
	preview, err := domain.PreviewChange(*state, command)
	if err != nil {
		t.Fatalf("preview change: %v", err)
	}
	updated, _, err := domain.ApplyEffects(*state, preview.Effects)
	if err != nil {
		t.Fatalf("apply change: %v", err)
	}
	*state = updated
	return preview.Activity
}

func reviewExactItem(t *testing.T, accountID domain.AccountID, currency, native, base string, holdingID *domain.HoldingID, instrumentID *domain.InstrumentID, quoteID, fxID string) domain.DailyValuationSnapshotItem {
	t.Helper()
	exact, err := decimal.NewFromString(base)
	if err != nil {
		t.Fatal(err)
	}
	baseAmount, err := domain.NewMoney(exact, "CNY")
	if err != nil {
		t.Fatal(err)
	}
	item := domain.DailyValuationSnapshotItem{ID: domain.NewDailyValuationSnapshotItemID(), AccountID: accountID, HoldingID: holdingID, InstrumentID: instrumentID, NativeAmount: native, NativeCurrency: domain.CurrencyCode(currency), BaseAmount: &baseAmount, BaseAmountExact: base, Complete: true}
	if quoteID != "" {
		item.QuoteID = &quoteID
	}
	if fxID != "" {
		item.FXQuoteID = &fxID
	}
	return item
}

func reviewSignedSum(day domain.ComponentDay) decimal.Decimal {
	total := day.BeginningValue.Amount()
	for bucket, value := range day.AssetBuckets {
		if bucket != domain.BucketResidual {
			total = total.Add(value.Amount())
		}
	}
	if day.Residual != nil {
		total = total.Add(day.Residual.Amount.Amount())
	}
	return total
}

func reviewAssertPeriodIdentity(t *testing.T, result domain.PeriodAnalysisResult, expectedEnding string) {
	t.Helper()
	total := decimal.Zero
	for _, day := range result.Days {
		total = total.Add(reviewSignedSum(day))
	}
	expected := decimal.RequireFromString(expectedEnding).RoundBank(4)
	if !total.Equal(expected) {
		t.Fatalf("period identity = %s, want %s", total, expected)
	}
}

func reviewAssertIdentityApprox(t *testing.T, day domain.ComponentDay, expectedEnding string, tolerance decimal.Decimal) {
	t.Helper()
	expected := decimal.RequireFromString(expectedEnding)
	if difference := reviewSignedSum(day).Sub(expected).Abs(); difference.GreaterThan(tolerance) {
		t.Fatalf("identity for %s differs by %s, tolerance %s", day.Component.Key(), difference, tolerance)
	}
}

func reviewAssertIdentity(t *testing.T, day domain.ComponentDay, expectedEnding string) {
	t.Helper()
	expected := decimal.RequireFromString(expectedEnding).RoundBank(4)
	if got := reviewSignedSum(day); !got.Equal(expected) {
		t.Fatalf("identity for %s = %s, want %s (buckets=%+v residual=%+v)", day.Component.Key(), got, expected, day.AssetBuckets, day.Residual)
	}
}

func reviewPortfolio(household *domain.Household, accounts ...domain.Account) domain.PortfolioSnapshot {
	records := make([]domain.AccountRecord, 0, len(accounts))
	for _, account := range accounts {
		records = append(records, domain.AccountRecord{Account: account})
	}
	return domain.PortfolioSnapshot{Household: household, Accounts: records}
}

func TestAnalyticsPhase1aReviewCase03ScopeIntersection(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	from := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleAsset)
	to := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleAsset)
	state := reviewChangeState(t, h, reviewAccountState(t, from, "100"), reviewAccountState(t, to, "0"))
	activity := reviewApplyChange(t, &state, domain.CashTransferInput{HouseholdID: h, FromAccountID: from.ID, ToAccountID: to.ID, Sent: mustMoney(t, "100", "CNY"), Received: mustMoney(t, "100", "CNY"), EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, from.ID, "CNY", "100", "100", nil, nil, "", ""), analyticsPhase1aItem(t, to.ID, "CNY", "0", "0", nil, nil, "", ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, from.ID, "CNY", "0", "0", nil, nil, "", ""), analyticsPhase1aItem(t, to.ID, "CNY", "100", "100", nil, nil, "", ""))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, from, to), Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}}
	query := analyticsPhase1aBaseQuery(domain.ValuationBase)
	query.Filters.AccountID = &to.ID
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	day := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: to.ID, Currency: "CNY", Cash: true})
	if got := analyticsPhase1aBucket(day, domain.BucketExternalFlow); !got.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("filtered receiving leg external flow=%s, want 100", got)
	}
	if day.Status != domain.CompletenessOK || day.Residual != nil {
		t.Fatalf("filtered transfer incomplete: %+v", day)
	}
	reviewAssertIdentity(t, day, "100")
}

func TestAnalyticsPhase1aReviewCase08SameDayOpenClose(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(h, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: h, Type: domain.InstrumentStock, QuoteCurrency: "CNY", Name: "same-day asset"}
	quantity := mustQuantity(t, "1")
	state := reviewChangeState(t, h, reviewAccountState(t, account, "0"))
	state.Now = time.Date(2026, 8, 2, 23, 59, 0, 0, time.UTC)
	state.Cash[account.ID] = map[domain.CurrencyCode]domain.Money{"CNY": mustMoney(t, "100", "CNY")}
	state.Holdings[holdingID] = domain.ChangeHoldingState{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID, InstrumentName: instrument.Name, Currency: "CNY", Current: mustQuantity(t, "0")}
	state.Instruments[instrumentID] = instrument
	when := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	buy := reviewApplyChange(t, &state, domain.TradeInput{HouseholdID: h, Side: domain.TradeBuy, SettlementAccountID: account.ID, HoldingID: holdingID, InstrumentID: instrumentID, Quantity: quantity, Gross: mustMoney(t, "10", "CNY"), EffectiveAt: when})
	sell := reviewApplyChange(t, &state, domain.TradeInput{HouseholdID: h, Side: domain.TradeSell, SettlementAccountID: account.ID, HoldingID: holdingID, InstrumentID: instrumentID, Quantity: quantity, Gross: mustMoney(t, "11", "CNY"), EffectiveAt: when.Add(time.Minute)})
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "11"), Currency: "CNY", QuotedAt: when.Add(time.Hour)}
	prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", "100", "100", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "CNY", "0", "0", &holdingID, &instrumentID, "", ""))
	cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", "101", "101", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "CNY", "0", "0", &holdingID, &instrumentID, closeQuote.ID.String(), ""))
	p := reviewPortfolio(household, account)
	p.Instruments = []domain.Instrument{instrument}
	p.Holdings = []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: p, Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{buy, sell}, InstrumentQuotes: []domain.InstrumentQuote{closeQuote}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	holdingDay := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: account.ID, HoldingID: &holdingID, InstrumentID: &instrumentID, Currency: "CNY"})
	if !analyticsPhase1aBucket(holdingDay, domain.BucketExternalFlow).IsZero() || !analyticsPhase1aBucket(holdingDay, domain.BucketPriceChange).Equal(decimal.NewFromInt(1)) || holdingDay.Residual != nil {
		t.Fatalf("same-day open/close attribution=%+v", holdingDay)
	}
	reviewAssertPeriodIdentity(t, result, "101")
}

func TestAnalyticsPhase1aReviewCase10FXConversion(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(h, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	state := reviewChangeState(t, h, reviewAccountState(t, account, "0"))
	state.Cash[account.ID] = map[domain.CurrencyCode]domain.Money{"USD": mustMoney(t, "100", "USD"), "SGD": mustMoney(t, "0", "SGD")}
	activity := reviewApplyChange(t, &state, domain.FXConversionInput{HouseholdID: h, AccountID: account.ID, Sold: mustMoney(t, "100", "USD"), Bought: mustMoney(t, "140", "SGD"), EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
	usdOpen := reviewFXQuote(t, h, "USD", "7", time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC))
	usdEvent := reviewFXQuote(t, h, "USD", "7", time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC))
	usdClose := reviewFXQuote(t, h, "USD", "7.2", time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC))
	sgdEvent := reviewFXQuote(t, h, "SGD", "5", time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC))
	sgdClose := reviewFXQuote(t, h, "SGD", "5.1", time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC))
	prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", "100", "700", nil, nil, "", usdOpen.ID.String()), analyticsPhase1aItem(t, account.ID, "SGD", "0", "0", nil, nil, "", ""))
	cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", "0", "0", nil, nil, "", usdClose.ID.String()), analyticsPhase1aItem(t, account.ID, "SGD", "140", "714", nil, nil, "", sgdClose.ID.String()))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, account), Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{activity}, FXQuotes: []domain.FXQuote{usdOpen, usdEvent, usdClose, sgdEvent, sgdClose}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	usdDay := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: account.ID, Currency: "USD", Cash: true})
	if !analyticsPhase1aBucket(usdDay, domain.BucketExternalFlow).IsZero() || usdDay.Residual != nil {
		t.Fatalf("USD conversion leg was not internal: %+v", usdDay)
	}
	sgdDay := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: account.ID, Currency: "SGD", Cash: true})
	if got := analyticsPhase1aBucket(sgdDay, domain.BucketFXImpact); !got.Equal(decimal.NewFromInt(14)) {
		t.Fatalf("SGD FX impact=%s, want 14", got)
	}
	reviewAssertPeriodIdentity(t, result, "714")
}

func TestAnalyticsPhase1aReviewCases09And26ForeignHoldingFX(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(h, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: h, Type: domain.InstrumentStock, QuoteCurrency: "USD", Name: "foreign asset"}
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "USD", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "110"), Currency: "USD", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	openFX := reviewFXQuote(t, h, "USD", "7", openQuote.QuotedAt)
	closeFX := reviewFXQuote(t, h, "USD", "7.2", closeQuote.QuotedAt)
	prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", "100", "700", &holdingID, &instrumentID, openQuote.ID.String(), openFX.ID.String()))
	cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", "110", "792", &holdingID, &instrumentID, closeQuote.ID.String(), closeFX.ID.String()))
	p := reviewPortfolio(household, account)
	p.Instruments = []domain.Instrument{instrument}
	p.Holdings = []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: p, Snapshots: []domain.DailyValuationSnapshot{prev, cur}, InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote}, FXQuotes: []domain.FXQuote{openFX, closeFX}}

	baseResult, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	baseDay := baseResult.Days[0]
	if got := analyticsPhase1aBucket(baseDay, domain.BucketPriceChange); !got.Equal(decimal.NewFromInt(70)) {
		t.Fatalf("foreign holding base price=%s, want 70", got)
	}
	if got := analyticsPhase1aBucket(baseDay, domain.BucketFXImpact); !got.Equal(decimal.NewFromInt(22)) {
		t.Fatalf("foreign holding base FX=%s, want 22", got)
	}
	reviewAssertIdentity(t, baseDay, "792")

	nativeResult, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationNative))
	if err != nil {
		t.Fatal(err)
	}
	nativeDay := nativeResult.Days[0]
	if got := analyticsPhase1aBucket(nativeDay, domain.BucketPriceChange); !got.Equal(decimal.NewFromInt(10)) || !analyticsPhase1aBucket(nativeDay, domain.BucketFXImpact).IsZero() {
		t.Fatalf("foreign holding native attribution=%+v", nativeDay)
	}
	reviewAssertIdentity(t, nativeDay, "110")
}

func TestAnalyticsPhase1aReviewCase32TrackingBalanceCashFormula(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(h, "USD", domain.TrackingBalance, domain.RoleAsset)
	state := reviewChangeState(t, h, reviewAccountState(t, account, "100"))
	when := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	activity := reviewApplyChange(t, &state, domain.MoneyAddedInput{HouseholdID: h, AccountID: account.ID, Amount: mustMoney(t, "10", "USD"), Reason: domain.ReasonIncome, EffectiveAt: when})
	openFX := reviewFXQuote(t, h, "USD", "7", time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC))
	eventFX := reviewFXQuote(t, h, "USD", "7.1", when)
	closeFX := reviewFXQuote(t, h, "USD", "7.2", time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC))
	prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", "100", "700", nil, nil, "", openFX.ID.String()))
	cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", "110", "792", nil, nil, "", closeFX.ID.String()))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, account), Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{activity}, FXQuotes: []domain.FXQuote{openFX, eventFX, closeFX}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if got := analyticsPhase1aBucket(day, domain.BucketIncome); !got.Equal(decimal.NewFromInt(71)) || !analyticsPhase1aBucket(day, domain.BucketFXImpact).Equal(decimal.NewFromInt(21)) || day.Residual != nil || day.Status != domain.CompletenessOK {
		t.Fatalf("TrackingBalance case 32 attribution=%+v", day)
	}
	reviewAssertIdentity(t, day, "792")

	// Keep the classified event and formula drivers unchanged while making the
	// closing base observation inconsistent with the closing native amount.
	cur.Items[0].NativeAmount = "111"
	cur.Items[0].BaseAmountExact = "799.2"
	wrongBase := mustMoney(t, "799.2", "CNY")
	cur.Items[0].BaseAmount = &wrongBase
	input.Snapshots = []domain.DailyValuationSnapshot{prev, cur}
	result, err = ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	day = result.Days[0]
	if !analyticsPhase1aBucket(day, domain.BucketIncome).Equal(decimal.NewFromInt(71)) {
		t.Fatalf("wrong native amount changed income: %+v", day)
	}
	if !analyticsPhase1aBucket(day, domain.BucketFXImpact).Equal(decimal.NewFromInt(21)) || day.Residual == nil || !day.Residual.Amount.Amount().Equal(decimal.RequireFromString("7.2")) || day.Status != domain.CompletenessPartial {
		t.Fatalf("cash error was absorbed by FX: %+v", day)
	}
	reviewAssertIdentity(t, day, "799.2")
}

func TestAnalyticsPhase1aReviewDepositInterestIsWritable(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleAsset)
	state := reviewChangeState(t, h, reviewAccountState(t, account, "100"))
	activity := reviewApplyChange(t, &state, domain.MoneyAddedInput{HouseholdID: h, AccountID: account.ID, Amount: mustMoney(t, "10", "CNY"), Reason: domain.ReasonInterest, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
	prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", "100", "100", nil, nil, "", ""))
	cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", "110", "110", nil, nil, "", ""))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, account), Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{activity}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if activity.Reason != domain.ReasonInterest || !analyticsPhase1aBucket(day, domain.BucketDividendInterest).Equal(decimal.NewFromInt(10)) || !analyticsPhase1aBucket(day, domain.BucketIncome).IsZero() || day.Residual != nil {
		t.Fatalf("deposit interest write/classification=%+v", day)
	}
	reviewAssertIdentity(t, day, "110")
	found := false
	for _, reason := range domain.MoneyInReasons() {
		if reason == domain.ReasonInterest {
			found = true
		}
	}
	if !found {
		t.Fatal("MoneyInReasons does not expose interest")
	}
}

func reviewTradeScenario(t *testing.T) (AnalysisInputs, domain.Account, domain.Instrument, domain.HoldingID) {
	t.Helper()
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(h, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: h, Type: domain.InstrumentStock, QuoteCurrency: "CNY", Name: "scoped asset"}
	state := reviewChangeState(t, h, reviewAccountState(t, account, "0"))
	state.Cash[account.ID] = map[domain.CurrencyCode]domain.Money{"CNY": mustMoney(t, "100", "CNY")}
	state.Holdings[holdingID] = domain.ChangeHoldingState{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID, InstrumentName: instrument.Name, Currency: "CNY", Current: mustQuantity(t, "0")}
	activity := reviewApplyChange(t, &state, domain.TradeInput{HouseholdID: h, Side: domain.TradeBuy, SettlementAccountID: account.ID, HoldingID: holdingID, InstrumentID: instrumentID, Quantity: mustQuantity(t, "1"), Gross: mustMoney(t, "100", "CNY"), EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
	quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", "100", "100", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "CNY", "0", "0", &holdingID, &instrumentID, "", ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", "0", "0", nil, nil, "", ""), analyticsPhase1aItem(t, account.ID, "CNY", "100", "100", &holdingID, &instrumentID, quote.ID.String(), ""))
	p := reviewPortfolio(household, account)
	p.Instruments = []domain.Instrument{instrument}
	p.Holdings = []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}
	return AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: p, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}, InstrumentQuotes: []domain.InstrumentQuote{quote}}, account, instrument, holdingID
}

func TestAnalyticsPhase1aReviewCases13And14ScopeSemantics(t *testing.T) {
	input, account, instrument, holdingID := reviewTradeScenario(t)
	instrumentQuery := analyticsPhase1aBaseQuery(domain.ValuationBase)
	instrumentQuery.Scope = domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: instrument.ID.String()}
	instrumentResult, err := ComputeAnalysis(input, instrumentQuery)
	if err != nil {
		t.Fatal(err)
	}
	holdingDay := analyticsPhase1aFindDay(t, instrumentResult, domain.ComponentID{AccountID: account.ID, HoldingID: &holdingID, InstrumentID: &instrument.ID, Currency: "CNY"})
	if got := analyticsPhase1aBucket(holdingDay, domain.BucketExternalFlow); !got.Equal(decimal.NewFromInt(100)) || !analyticsPhase1aBucket(holdingDay, domain.BucketPriceChange).IsZero() || holdingDay.Residual != nil {
		t.Fatalf("instrument purchase scope=%+v", holdingDay)
	}
	reviewAssertIdentity(t, holdingDay, "100")

	accountQuery := analyticsPhase1aBaseQuery(domain.ValuationBase)
	accountQuery.Scope = domain.AnalysisScope{Kind: domain.ScopeAccount, ID: account.ID.String()}
	accountResult, err := ComputeAnalysis(input, accountQuery)
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range accountResult.Days {
		if !analyticsPhase1aBucket(day, domain.BucketExternalFlow).IsZero() || day.Residual != nil {
			t.Fatalf("account buy should stay internal: %+v", day)
		}
	}
	reviewAssertPeriodIdentity(t, accountResult, "100")
}

func TestAnalyticsPhase1aReviewCases16And17RealDebtPaymentPath(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	debt := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleLiability)
	cash := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleAsset)
	when := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	t.Run("case-15-principal-draw-is-neutral", func(t *testing.T) {
		state := reviewChangeState(t, h, reviewAccountState(t, debt, "0"), reviewAccountState(t, cash, "100000"))
		activity := reviewApplyChange(t, &state, domain.DebtDrawInput{HouseholdID: h, DebtAccountID: debt.ID, CashAccountID: cash.ID, Principal: mustMoney(t, "100000", "CNY"), EffectiveAt: when})
		prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, debt.ID, "CNY", "0", "0", nil, nil, "", ""), analyticsPhase1aItem(t, cash.ID, "CNY", "100000", "100000", nil, nil, "", ""))
		cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, debt.ID, "CNY", "100000", "100000", nil, nil, "", ""), analyticsPhase1aItem(t, cash.ID, "CNY", "200000", "200000", nil, nil, "", ""))
		input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, debt, cash), Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{activity}}
		result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
		if err != nil {
			t.Fatal(err)
		}
		for _, day := range result.Days {
			if day.Status != domain.CompletenessOK || day.Residual != nil || !analyticsPhase1aBucket(day, domain.BucketExternalFlow).IsZero() {
				t.Fatalf("principal draw was not neutral: %+v", day)
			}
		}
		reviewAssertPeriodIdentity(t, result, "100000")
	})

	t.Run("case-16-principal-repayment", func(t *testing.T) {
		state := reviewChangeState(t, h, reviewAccountState(t, debt, "10000"), reviewAccountState(t, cash, "10000"))
		activity := reviewApplyChange(t, &state, domain.DebtPaymentInput{HouseholdID: h, DebtAccountID: debt.ID, CashAccountID: cash.ID, Principal: mustMoney(t, "10000", "CNY"), EffectiveAt: when})
		prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, debt.ID, "CNY", "10000", "10000", nil, nil, "", ""), analyticsPhase1aItem(t, cash.ID, "CNY", "10000", "10000", nil, nil, "", ""))
		cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, debt.ID, "CNY", "0", "0", nil, nil, "", ""), analyticsPhase1aItem(t, cash.ID, "CNY", "0", "0", nil, nil, "", ""))
		input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, debt, cash), Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{activity}}
		result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
		if err != nil {
			t.Fatal(err)
		}
		reviewAssertPeriodIdentity(t, result, "0")
		for _, day := range result.Days {
			if day.Residual != nil || !analyticsPhase1aBucket(day, domain.BucketExternalFlow).IsZero() {
				t.Fatalf("principal repayment was not neutral: %+v", day)
			}
		}
	})

	t.Run("case-17-interest-is-spending", func(t *testing.T) {
		state := reviewChangeState(t, h, reviewAccountState(t, debt, "10000"), reviewAccountState(t, cash, "1500"))
		interest := mustMoney(t, "500", "CNY")
		activity := reviewApplyChange(t, &state, domain.DebtPaymentInput{HouseholdID: h, DebtAccountID: debt.ID, CashAccountID: cash.ID, Principal: mustMoney(t, "1000", "CNY"), InterestOrFee: &interest, EffectiveAt: when})
		prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, debt.ID, "CNY", "10000", "10000", nil, nil, "", ""), analyticsPhase1aItem(t, cash.ID, "CNY", "1500", "1500", nil, nil, "", ""))
		cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, debt.ID, "CNY", "9000", "9000", nil, nil, "", ""), analyticsPhase1aItem(t, cash.ID, "CNY", "0", "0", nil, nil, "", ""))
		input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, debt, cash), Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{activity}}
		result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
		if err != nil {
			t.Fatal(err)
		}
		cashDay := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: cash.ID, Currency: "CNY", Cash: true})
		if got := analyticsPhase1aBucket(cashDay, domain.BucketSpending); !got.Equal(decimal.NewFromInt(-500)) || !analyticsPhase1aBucket(cashDay, domain.BucketFee).IsZero() || cashDay.Residual != nil {
			t.Fatalf("real DebtPayment interest classification=%+v", cashDay)
		}
		reviewAssertPeriodIdentity(t, result, "-9000")
	})
}

func TestAnalyticsPhase1aReviewCase17bCapitalisedInterest(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	debt := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleLiability)
	state := reviewChangeState(t, h, reviewAccountState(t, debt, "10000"))
	activity := reviewApplyChange(t, &state, domain.ValueUpdateInput{HouseholdID: h, AccountID: debt.ID, NewValue: mustMoney(t, "10500", "CNY"), Reason: domain.ReasonInterest, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
	prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, debt.ID, "CNY", "10000", "10000", nil, nil, "", ""))
	cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, debt.ID, "CNY", "10500", "10500", nil, nil, "", ""))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, debt), Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{activity}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if got := analyticsPhase1aBucket(day, domain.BucketLiabilityImpact); !got.Equal(decimal.NewFromInt(-500)) || !analyticsPhase1aBucket(day, domain.BucketSpending).IsZero() || day.Residual != nil {
		t.Fatalf("capitalised interest classification=%+v", day)
	}
	reviewAssertIdentity(t, day, "-10500")
}

func TestAnalyticsPhase1aReviewCase24FractionalQuantity(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(h, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: h, Type: domain.InstrumentETF, QuoteCurrency: "CNY", Name: "fractional asset"}
	quantity := mustQuantity(t, "0.12345678")
	openPrice := mustUnitPrice(t, "10")
	closePrice := mustUnitPrice(t, "11")
	openNative := quantity.Decimal().Mul(openPrice.Decimal())
	closeNative := quantity.Decimal().Mul(closePrice.Decimal())
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: openPrice, Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: closePrice, Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	prev := analyticsPhase1aSnapshot("2026-08-01", reviewExactItem(t, account.ID, "CNY", openNative.String(), openNative.String(), &holdingID, &instrumentID, openQuote.ID.String(), ""))
	cur := analyticsPhase1aSnapshot("2026-08-02", reviewExactItem(t, account.ID, "CNY", closeNative.String(), closeNative.String(), &holdingID, &instrumentID, closeQuote.ID.String(), ""))
	p := reviewPortfolio(household, account)
	p.Instruments = []domain.Instrument{instrument}
	p.Holdings = []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: p, Snapshots: []domain.DailyValuationSnapshot{prev, cur}, InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote}}
	query := analyticsPhase1aBaseQuery(domain.ValuationNative)
	universe, err := resolveAnalysisUniverse(input, query)
	if err != nil {
		t.Fatal(err)
	}
	inferred, err := inferredQuantity(prev.Items[0], openQuoteForTest(universe, prev.Items[0], input.InstrumentQuotes))
	if err != nil || !inferred.Equal(quantity.Decimal()) {
		t.Fatalf("inferred fractional quantity=%s, want %s (err=%v)", inferred, quantity.Decimal(), err)
	}
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if day.Residual != nil || !analyticsPhase1aBucket(day, domain.BucketPriceChange).Equal(decimal.RequireFromString("0.1235")) {
		t.Fatalf("fractional quantity attribution=%+v", day)
	}
	// Quantity and BaseAmountExact retain the full value; the public AssetBucket
	// is a four-decimal Money view, so its identity is expected to differ only
	// within the currency-aware display tolerance. This proves inference is not
	// the source of the rounding loss and does not require schema v10 here.
	reviewAssertIdentityApprox(t, day, closeNative.String(), residualTolerance("CNY", openNative))
}

func openQuoteForTest(universe analysisUniverse, item domain.DailyValuationSnapshotItem, quotes []domain.InstrumentQuote) *domain.InstrumentQuote {
	return universe.quoteForItem(item, quotes, time.Date(2026, 8, 1, 23, 59, 0, 0, time.UTC))
}

func TestAnalyticsPhase1aReviewCase25ValueUpdateAndResidual(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleAsset)
	state := reviewChangeState(t, h, reviewAccountState(t, account, "100"))
	activity := reviewApplyChange(t, &state, domain.ValueUpdateInput{HouseholdID: h, AccountID: account.ID, NewValue: mustMoney(t, "110", "CNY"), Reason: domain.ReasonOther, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
	prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", "100", "100", nil, nil, "", ""))
	cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", "120", "120", nil, nil, "", ""))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, account), Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{activity}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if !analyticsPhase1aBucket(day, domain.BucketAdjustment).Equal(decimal.NewFromInt(10)) || day.Residual == nil || !day.Residual.Amount.Amount().Equal(decimal.NewFromInt(10)) || day.Status != domain.CompletenessPartial {
		t.Fatalf("adjustment/residual were not distinct: %+v", day)
	}
	reviewAssertIdentity(t, day, "120")
}

func TestAnalyticsPhase1aReviewCases28And29RealMoneyOutPath(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	for _, tc := range []struct {
		name   string
		reason domain.ActivityReason
		amount string
	}{
		{name: "case-28-bank-maintenance-fee", reason: domain.ReasonFee, amount: "10"},
		{name: "case-29-unassociated-tax", reason: domain.ReasonTax, amount: "5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleAsset)
			state := reviewChangeState(t, h, reviewAccountState(t, account, "100"))
			activity := reviewApplyChange(t, &state, domain.MoneyRemovedInput{HouseholdID: h, AccountID: account.ID, Amount: mustMoney(t, tc.amount, "CNY"), Reason: tc.reason, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
			ending := decimal.NewFromInt(100).Sub(decimal.RequireFromString(tc.amount))
			prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", "100", "100", nil, nil, "", ""))
			cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", ending.String(), ending.String(), nil, nil, "", ""))
			input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, account), Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{activity}}
			result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
			if err != nil {
				t.Fatal(err)
			}
			day := result.Days[0]
			if got := analyticsPhase1aBucket(day, domain.BucketFee); !got.Equal(ending.Sub(decimal.NewFromInt(100))) || day.Residual != nil {
				t.Fatalf("fee classification=%+v", day)
			}
			reviewAssertIdentity(t, day, ending.String())
		})
	}
}

func TestAnalyticsPhase1aReviewCase31IdentityAcrossScopes(t *testing.T) {
	h := domain.NewHouseholdID()
	household := &domain.Household{ID: h, BaseCurrency: "CNY"}
	asset := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleAsset)
	debt := analyticsPhase1aAccount(h, "CNY", domain.TrackingBalance, domain.RoleLiability)
	state := reviewChangeState(t, h, reviewAccountState(t, asset, "100"), reviewAccountState(t, debt, "50"))
	income := reviewApplyChange(t, &state, domain.MoneyAddedInput{HouseholdID: h, AccountID: asset.ID, Amount: mustMoney(t, "10", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)})
	capitalised := reviewApplyChange(t, &state, domain.ValueUpdateInput{HouseholdID: h, AccountID: debt.ID, NewValue: mustMoney(t, "55", "CNY"), Reason: domain.ReasonInterest, EffectiveAt: time.Date(2026, 8, 2, 11, 0, 0, 0, time.UTC)})
	prev := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, asset.ID, "CNY", "100", "100", nil, nil, "", ""), analyticsPhase1aItem(t, debt.ID, "CNY", "50", "50", nil, nil, "", ""))
	cur := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, asset.ID, "CNY", "110", "110", nil, nil, "", ""), analyticsPhase1aItem(t, debt.ID, "CNY", "55", "55", nil, nil, "", ""))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, h, "UTC"), Portfolio: reviewPortfolio(household, asset, debt), Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{income, capitalised}}
	for _, query := range []domain.AnalysisQuery{
		analyticsPhase1aBaseQuery(domain.ValuationBase),
		func() domain.AnalysisQuery {
			q := analyticsPhase1aBaseQuery(domain.ValuationBase)
			q.Scope = domain.AnalysisScope{Kind: domain.ScopeAccount, ID: asset.ID.String()}
			return q
		}(),
		func() domain.AnalysisQuery {
			q := analyticsPhase1aBaseQuery(domain.ValuationBase)
			q.Scope = domain.AnalysisScope{Kind: domain.ScopeAccount, ID: debt.ID.String()}
			return q
		}(),
	} {
		result, err := ComputeAnalysis(input, query)
		if err != nil {
			t.Fatal(err)
		}
		for _, day := range result.Days {
			if day.Component.AccountID == asset.ID {
				reviewAssertIdentity(t, day, "110")
			} else {
				reviewAssertIdentity(t, day, "-55")
			}
			if day.Status != domain.CompletenessOK || day.Residual != nil {
				t.Fatalf("identity fixture incomplete for %s: %+v", query.Scope.Kind, day)
			}
		}
	}
}

func TestAnalyticsPhase1aReviewCase40OriginTimezoneBoundaries(t *testing.T) {
	for _, tc := range []struct {
		date      string
		effective time.Time
		want      string
	}{
		{date: "2026-03-08", effective: time.Date(2026, 3, 9, 6, 30, 0, 0, time.UTC), want: "2026-03-08"},
		{date: "2026-11-01", effective: time.Date(2026, 11, 2, 7, 30, 0, 0, time.UTC), want: "2026-11-01"},
	} {
		if got := activityLocalDate(domain.Activity{EffectiveAt: tc.effective}, "America/Los_Angeles"); got != tc.want {
			t.Fatalf("effective %s assigned to %s, want %s", tc.effective, got, tc.want)
		}
	}
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	springStart, err := domain.ResolveLocalDateTime("2026-03-08", "00:00", loc.String())
	if err != nil {
		t.Fatal(err)
	}
	springEnd, err := domain.ResolveLocalDateTime("2026-03-09", "00:00", loc.String())
	if err != nil {
		t.Fatal(err)
	}
	fallStart, err := domain.ResolveLocalDateTime("2026-11-01", "00:00", loc.String())
	if err != nil {
		t.Fatal(err)
	}
	fallEnd, err := domain.ResolveLocalDateTime("2026-11-02", "00:00", loc.String())
	if err != nil {
		t.Fatal(err)
	}
	if got := springEnd.Sub(springStart); got != 23*time.Hour {
		t.Fatalf("spring Origin day length=%s, want 23h", got)
	}
	if got := fallEnd.Sub(fallStart); got != 25*time.Hour {
		t.Fatalf("fall Origin day length=%s, want 25h", got)
	}
}

func TestAnalyticsPhase1aReviewCase41CurrencyAwareTolerance(t *testing.T) {
	if got := residualTolerance("JPY", decimal.Zero); !got.Equal(decimal.NewFromInt(2)) {
		t.Fatalf("JPY tolerance=%s, want 2", got)
	}
	if got := residualTolerance("KRW", decimal.Zero); !got.Equal(decimal.NewFromInt(2)) {
		t.Fatalf("KRW tolerance=%s, want 2", got)
	}
	if got := residualTolerance("CNY", decimal.Zero); !got.Equal(decimal.RequireFromString("0.02")) {
		t.Fatalf("CNY tolerance=%s, want 0.02", got)
	}
	// BTC is not currently a supported currency. Phase 1a's documented
	// fallback is the legacy two-decimal display precision for unknown codes.
	if got := residualTolerance("BTC", decimal.Zero); !got.Equal(decimal.RequireFromString("0.02")) {
		t.Fatalf("unknown-currency tolerance=%s, want 0.02", got)
	}
}

func TestAnalyticsPhase1aReviewServiceLoadsClosedSnapshots(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "analysis-service-closed-snapshots", []string{"Owner"})
	account, err := service.CreateAccount(ctx, AccountInput{
		Name:               "Bank",
		AccountType:        "bank_account",
		BalanceSheetRole:   "asset",
		TrackingMode:       "balance",
		DefaultCurrency:    "CNY",
		IncludeInNetWorth:  true,
		IncludeInPortfolio: true,
		OwnerIDs:           []domain.MemberID{bootstrap.Members[0].ID},
		InitialAmount:      "100",
	})
	if err != nil {
		t.Fatal(err)
	}
	origin, err := service.StartHistory(ctx, "America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	if origin.Timezone != "America/Los_Angeles" {
		t.Fatalf("origin timezone=%q", origin.Timezone)
	}
	setClock(time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC))
	if _, err := service.RecordChange(ctx, domain.MoneyAddedInput{
		HouseholdID: bootstrap.Household.ID,
		AccountID:   account.Account.ID,
		Amount:      mustMoney(t, "10", "CNY"),
		Reason:      domain.ReasonIncome,
		EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	query := analyticsPhase1aBaseQuery(domain.ValuationBase)
	result, err := service.Analyze(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Days) != 1 {
		t.Fatalf("service analysis days=%d, want 1", len(result.Days))
	}
	day := result.Days[0]
	if got := analyticsPhase1aBucket(day, domain.BucketIncome); !got.Equal(decimal.NewFromInt(10)) || day.Status != domain.CompletenessOK || day.Residual != nil {
		t.Fatalf("service analysis did not use closed snapshots: %+v", day)
	}
	reviewAssertIdentity(t, day, "110")

	snapshots, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	seenAug1, seenAug2 := false, false
	for _, snapshot := range snapshots {
		seenAug1 = seenAug1 || snapshot.LocalDate == "2026-08-01"
		seenAug2 = seenAug2 || snapshot.LocalDate == "2026-08-02"
	}
	if !seenAug1 || !seenAug2 {
		t.Fatalf("service did not ensure closed-day snapshots: Aug1=%t Aug2=%t", seenAug1, seenAug2)
	}
}

func reviewFXQuote(t *testing.T, householdID domain.HouseholdID, native, rate string, quotedAt time.Time) domain.FXQuote {
	t.Helper()
	parsed, err := domain.ParseFxRate(rate)
	if err != nil {
		t.Fatal(err)
	}
	return domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: domain.CurrencyCode(native), QuoteCurrency: "CNY", Rate: parsed, QuotedAt: quotedAt}
}
