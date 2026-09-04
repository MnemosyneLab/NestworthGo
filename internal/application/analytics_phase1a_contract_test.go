package application

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func analyticsPhase1aAccount(householdID domain.HouseholdID, currency domain.CurrencyCode, tracking domain.TrackingMode, role domain.BalanceSheetRole) domain.Account {
	return domain.Account{ID: domain.NewAccountID(), HouseholdID: householdID, Name: "Analysis account", AccountType: domain.TypeBankAccount, BalanceSheetRole: role, TrackingMode: tracking, DefaultCurrency: currency, IncludeInNetWorth: true}
}

func analyticsPhase1aItem(t *testing.T, accountID domain.AccountID, currency, native, base string, holdingID *domain.HoldingID, instrumentID *domain.InstrumentID, quoteID, fxID string) domain.DailyValuationSnapshotItem {
	t.Helper()
	baseAmount := mustMoney(t, base, "CNY")
	item := domain.DailyValuationSnapshotItem{ID: domain.NewDailyValuationSnapshotItemID(), AccountID: accountID, HoldingID: holdingID, InstrumentID: instrumentID, NativeAmount: native, NativeCurrency: domain.CurrencyCode(currency), BaseAmount: &baseAmount, BaseAmountExact: base, Complete: true}
	if quoteID != "" {
		item.QuoteID = &quoteID
	}
	if fxID != "" {
		item.FXQuoteID = &fxID
	}
	return item
}

func analyticsPhase1aSnapshot(date string, items ...domain.DailyValuationSnapshotItem) domain.DailyValuationSnapshot {
	parsed, _ := time.Parse("2006-01-02", date)
	return domain.DailyValuationSnapshot{ID: domain.NewDailyValuationSnapshotID(), LocalDate: date, CutoffAt: parsed.Add(23*time.Hour + 59*time.Minute), Currency: "CNY", Complete: true, Items: items}
}

func analyticsPhase1aOrigin(t *testing.T, householdID domain.HouseholdID, timezone string) domain.HistoryOrigin {
	t.Helper()
	origin, err := domain.NewHistoryOrigin(householdID, timezone, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return origin
}

func analyticsPhase1aEffect(t *testing.T, activityID domain.ActivityID, sequence int, accountID domain.AccountID, amount, currency string, direction domain.EffectDirection, target domain.EffectTarget, classification domain.ActivityClassification) domain.ActivityEffect {
	t.Helper()
	money := mustMoney(t, amount, currency)
	return domain.ActivityEffect{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: sequence, Direction: direction, Target: target, Classification: classification, AccountID: &accountID, Money: &money}
}

func analyticsPhase1aBucket(day domain.ComponentDay, bucket domain.AttributionBucket) decimal.Decimal {
	value, ok := day.AssetBuckets[bucket]
	if !ok {
		return decimal.Zero
	}
	return value.Amount()
}

func analyticsPhase1aFindDay(t *testing.T, result domain.PeriodAnalysisResult, component domain.ComponentID) domain.ComponentDay {
	t.Helper()
	for _, day := range result.Days {
		if day.Component.Key() == component.Key() {
			return day
		}
	}
	t.Fatalf("component %s not found", component.Key())
	return domain.ComponentDay{}
}

func analyticsPhase1aBaseQuery(valuation domain.Valuation) domain.AnalysisQuery {
	return domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-02", To: "2026-08-02", Valuation: valuation, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
}

func TestAnalyticsPhase1aReviewCases01And02And17bIdentity(t *testing.T) {
	householdID := domain.NewHouseholdID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	from := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	to := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, from.ID, "CNY", "100", "100", nil, nil, "", ""), analyticsPhase1aItem(t, to.ID, "CNY", "0", "0", nil, nil, "", ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, from.ID, "CNY", "0", "0", nil, nil, "", ""), analyticsPhase1aItem(t, to.ID, "CNY", "100", "100", nil, nil, "", ""))
	activityID := domain.NewActivityID()
	transfer := domain.Activity{ID: activityID, Kind: domain.ActivityCashTransfer, Reason: domain.ReasonOther, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{
		analyticsPhase1aEffect(t, activityID, 1, from.ID, "100", "CNY", domain.EffectRemoved, domain.EffectTargetAccountValue, domain.ClassificationInternalTransfer),
		analyticsPhase1aEffect(t, activityID, 2, to.ID, "100", "CNY", domain.EffectAdded, domain.EffectTargetAccountValue, domain.ClassificationInternalTransfer),
	}}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: from}, {Account: to}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{transfer}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range result.Days {
		if day.Status != domain.CompletenessOK || day.Residual != nil || !analyticsPhase1aBucket(day, domain.BucketExternalFlow).IsZero() {
			t.Fatalf("household transfer was not neutral: %+v", day)
		}
	}
	reviewAssertPeriodIdentity(t, result, "100")
	accountQuery := analyticsPhase1aBaseQuery(domain.ValuationBase)
	accountQuery.Scope = domain.AnalysisScope{Kind: domain.ScopeAccount, ID: to.ID.String()}
	accountInput := input
	accountInput.Portfolio.Accounts = []domain.AccountRecord{{Account: to}}
	accountResult, err := ComputeAnalysis(accountInput, accountQuery)
	if err != nil {
		t.Fatal(err)
	}
	if got := analyticsPhase1aBucket(accountResult.Days[0], domain.BucketExternalFlow); !got.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("account rewrite external flow = %s, want 100", got)
	}
	reviewAssertIdentity(t, accountResult.Days[0], "100")

	debt := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleLiability)
	debtPrevious := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, debt.ID, "CNY", "0", "0", nil, nil, "", ""))
	debtCurrent := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, debt.ID, "CNY", "500", "500", nil, nil, "", ""))
	interestID := domain.NewActivityID()
	interest := domain.Activity{ID: interestID, Kind: domain.ActivityValueUpdate, Reason: domain.ReasonInterest, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{analyticsPhase1aEffect(t, interestID, 1, debt.ID, "500", "CNY", domain.EffectAdded, domain.EffectTargetAccountValue, domain.ClassificationRemeasurement)}}
	debtInput := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: debt}}}, Snapshots: []domain.DailyValuationSnapshot{debtPrevious, debtCurrent}, Activities: []domain.Activity{interest}}
	debtResult, err := ComputeAnalysis(debtInput, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	debtDay := debtResult.Days[0]
	if got := analyticsPhase1aBucket(debtDay, domain.BucketLiabilityImpact); !got.Equal(decimal.NewFromInt(-500)) || debtDay.Residual != nil || debtDay.Status != domain.CompletenessOK {
		t.Fatalf("capitalized liability interest = %+v", debtDay)
	}
	reviewAssertIdentity(t, debtDay, "-500")
}

func TestAnalyticsPhase1aReviewCases09And26And32FXPaths(t *testing.T) {
	householdID := domain.NewHouseholdID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Type: domain.InstrumentStock, QuoteCurrency: "USD", Name: "USD asset"}
	openFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: func() domain.FxRate { value, _ := domain.ParseFxRate("7"); return value }(), QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	eventFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: func() domain.FxRate { value, _ := domain.ParseFxRate("7.1"); return value }(), QuotedAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}
	closeFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: func() domain.FxRate { value, _ := domain.ParseFxRate("7.2"); return value }(), QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	openPrice := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "USD", QuotedAt: openFX.QuotedAt}
	closePrice := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "110"), Currency: "USD", QuotedAt: closeFX.QuotedAt}
	openFXID, closeFXID := openFX.ID.String(), closeFX.ID.String()
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", "100", "700", nil, nil, "", openFXID))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", "110", "792", nil, nil, "", closeFXID))
	activityID := domain.NewActivityID()
	income := domain.Activity{ID: activityID, Kind: domain.ActivityCashIn, Reason: domain.ReasonIncome, EffectiveAt: eventFX.QuotedAt, EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{analyticsPhase1aEffect(t, activityID, 1, account.ID, "10", "USD", domain.EffectAdded, domain.EffectTargetAccountCash, domain.ClassificationIncome)}}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{income}, FXQuotes: []domain.FXQuote{openFX, eventFX, closeFX}, InstrumentQuotes: []domain.InstrumentQuote{openPrice, closePrice}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	cashDay := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: account.ID, Currency: "USD", Cash: true})
	if got := analyticsPhase1aBucket(cashDay, domain.BucketIncome); !got.Equal(decimal.NewFromInt(71)) {
		t.Fatalf("cash income = %s, want 71", got)
	}
	if got := analyticsPhase1aBucket(cashDay, domain.BucketFXImpact); !got.Equal(decimal.NewFromInt(21)) {
		t.Fatalf("cash FX = %s, want 21", got)
	}
	reviewAssertIdentity(t, cashDay, "792")
	previous.Items[0] = analyticsPhase1aItem(t, account.ID, "USD", "100", "700", &holdingID, &instrumentID, openPrice.ID.String(), openFXID)
	current.Items[0] = analyticsPhase1aItem(t, account.ID, "USD", "110", "792", &holdingID, &instrumentID, closePrice.ID.String(), closeFXID)
	input.Snapshots = []domain.DailyValuationSnapshot{previous, current}
	input.Activities = nil
	result, err = ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	holdingDay := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: account.ID, HoldingID: &holdingID, InstrumentID: &instrumentID, Currency: "USD"})
	if got := analyticsPhase1aBucket(holdingDay, domain.BucketPriceChange); !got.Equal(decimal.NewFromInt(70)) {
		t.Fatalf("holding price = %s, want 70", got)
	}
	if got := analyticsPhase1aBucket(holdingDay, domain.BucketFXImpact); !got.Equal(decimal.NewFromInt(22)) {
		t.Fatalf("holding FX = %s, want 22", got)
	}
	reviewAssertIdentity(t, holdingDay, "792")
}

func TestAnalyticsPhase1aReviewCases23And27And34And36Boundaries(t *testing.T) {
	householdID := domain.NewHouseholdID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Type: domain.InstrumentStock, QuoteCurrency: "CNY"}
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "400"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", "40000", "40000", &holdingID, &instrumentID, openQuote.ID.String(), ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", "40000", "40000", &holdingID, &instrumentID, closeQuote.ID.String(), ""))
	activityID := domain.NewActivityID()
	quantity := mustQuantity(t, "300")
	activity := domain.Activity{ID: activityID, Kind: domain.ActivityPositionTransfer, Reason: domain.ReasonReconciliation, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Direction: domain.EffectAdded, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationRemeasurement, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity}}}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}, InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if day.Status != domain.CompletenessOK || day.Residual != nil || !analyticsPhase1aBucket(day, domain.BucketExternalFlow).IsZero() || !analyticsPhase1aBucket(day, domain.BucketPriceChange).IsZero() {
		t.Fatalf("4:1 split was treated as economic change: %+v", day)
	}
	reviewAssertIdentity(t, day, "40000")
	input.InstrumentQuotes = []domain.InstrumentQuote{closeQuote}
	previous.Items[0].NativeAmount = "40000"
	current.Items[0].NativeAmount = "41000"
	current.Items[0].BaseAmountExact = "41000"
	updatedBase := mustMoney(t, "41000", "CNY")
	current.Items[0].BaseAmount = &updatedBase
	input.Snapshots = []domain.DailyValuationSnapshot{previous, current}
	result, err = ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	missing := result.Days[0]
	if missing.Status != domain.CompletenessPartial || missing.Residual == nil || !analyticsPhase1aBucket(missing, domain.BucketPriceChange).IsZero() {
		t.Fatalf("missing quote was fabricated: %+v", missing)
	}
	reviewAssertIdentity(t, missing, "41000")
	if got := activityLocalDate(domain.Activity{EffectiveAt: time.Date(2026, 3, 8, 16, 30, 0, 0, time.UTC)}, "America/Los_Angeles"); got != "2026-03-08" {
		t.Fatalf("DST local date = %s", got)
	}
	first := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	second := analyticsPhase1aAccount(householdID, "USD", domain.TrackingBalance, domain.RoleAsset)
	nativeInput := input
	nativeInput.Portfolio.Accounts = []domain.AccountRecord{{Account: first}, {Account: second}}
	nativeInput.Portfolio.Instruments = nil
	nativeInput.Portfolio.Holdings = nil
	nativeInput.Snapshots = []domain.DailyValuationSnapshot{analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, first.ID, "CNY", "1", "1", nil, nil, "", ""), analyticsPhase1aItem(t, second.ID, "USD", "1", "7", nil, nil, "", "")), analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, first.ID, "CNY", "1", "1", nil, nil, "", ""), analyticsPhase1aItem(t, second.ID, "USD", "1", "7", nil, nil, "", ""))}
	if _, err := ComputeAnalysis(nativeInput, analyticsPhase1aBaseQuery(domain.ValuationNative)); err == nil {
		t.Fatal("native multi-currency analysis was accepted")
	}
}

func TestAnalyticsPhase1aReviewCase35InKindTransferHasNoPriceAndIsScopeRelative(t *testing.T) {
	householdID := domain.NewHouseholdID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	source := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	receiver := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID := domain.NewInstrumentID()
	fromHolding, toHolding := domain.NewHoldingID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Type: domain.InstrumentStock, QuoteCurrency: "CNY"}
	openingQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, source.ID, "CNY", "100", "100", &fromHolding, &instrumentID, openingQuote.ID.String(), ""), analyticsPhase1aItem(t, receiver.ID, "CNY", "0", "0", &toHolding, &instrumentID, "", ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, source.ID, "CNY", "0", "0", &fromHolding, &instrumentID, quote.ID.String(), ""), analyticsPhase1aItem(t, receiver.ID, "CNY", "100", "100", &toHolding, &instrumentID, quote.ID.String(), ""))
	activityID := domain.NewActivityID()
	quantity := mustQuantity(t, "1")
	activity := domain.Activity{ID: activityID, Kind: domain.ActivityPositionTransfer, Reason: domain.ReasonOther, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{
		{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Direction: domain.EffectRemoved, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationInternalTransfer, HoldingID: &fromHolding, InstrumentID: &instrumentID, Quantity: &quantity},
		{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 2, Direction: domain.EffectAdded, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationInternalTransfer, HoldingID: &toHolding, InstrumentID: &instrumentID, Quantity: &quantity},
	}}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: source}, {Account: receiver}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: fromHolding, AccountID: source.ID, InstrumentID: instrumentID}, {ID: toHolding, AccountID: receiver.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}, InstrumentQuotes: []domain.InstrumentQuote{openingQuote, quote}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	for _, holding := range []domain.HoldingID{fromHolding, toHolding} {
		day := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: map[domain.HoldingID]domain.AccountID{fromHolding: source.ID, toHolding: receiver.ID}[holding], HoldingID: &holding, InstrumentID: &instrumentID, Currency: "CNY"})
		if day.Status != domain.CompletenessOK || day.Residual != nil || !analyticsPhase1aBucket(day, domain.BucketPriceChange).IsZero() {
			t.Fatalf("household in-kind transfer changed value: %+v", day)
		}
	}
	reviewAssertPeriodIdentity(t, result, "100")
	accountQuery := analyticsPhase1aBaseQuery(domain.ValuationBase)
	accountQuery.Scope = domain.AnalysisScope{Kind: domain.ScopeAccount, ID: receiver.ID.String()}
	accountInput := input
	accountInput.Portfolio.Accounts = []domain.AccountRecord{{Account: receiver}}
	accountResult, err := ComputeAnalysis(accountInput, accountQuery)
	if err != nil {
		t.Fatal(err)
	}
	receivingDay := accountResult.Days[0]
	if got := analyticsPhase1aBucket(receivingDay, domain.BucketExternalFlow); !got.Equal(decimal.NewFromInt(100)) || !analyticsPhase1aBucket(receivingDay, domain.BucketPriceChange).IsZero() {
		t.Fatalf("receiving account transfer = %+v", receivingDay)
	}
	reviewAssertIdentity(t, receivingDay, "100")
}

func TestAnalyticsPhase1aReviewCases33And36QuantityErrorKeepsFormulaDriversAndShowsResidual(t *testing.T) {
	householdID := domain.NewHouseholdID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Type: domain.InstrumentStock, QuoteCurrency: "USD"}
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "USD", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "110"), Currency: "USD", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	openFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: func() domain.FxRate { value, _ := domain.ParseFxRate("7"); return value }(), QuotedAt: openQuote.QuotedAt}
	closeFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: func() domain.FxRate { value, _ := domain.ParseFxRate("7.2"); return value }(), QuotedAt: closeQuote.QuotedAt}
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", "100", "700", &holdingID, &instrumentID, openQuote.ID.String(), openFX.ID.String()))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", "220", "1584", &holdingID, &instrumentID, closeQuote.ID.String(), closeFX.ID.String()))
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, InstrumentQuotes: []domain.InstrumentQuote{openQuote, closeQuote}, FXQuotes: []domain.FXQuote{openFX, closeFX}}
	query := analyticsPhase1aBaseQuery(domain.ValuationBase)
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if day.Status != domain.CompletenessPartial || day.Residual == nil || !analyticsPhase1aBucket(day, domain.BucketPriceChange).Equal(decimal.NewFromInt(70)) || !analyticsPhase1aBucket(day, domain.BucketFXImpact).Equal(decimal.NewFromInt(22)) {
		t.Fatalf("base quantity error was absorbed by drivers: %+v", day)
	}
	query.Valuation = domain.ValuationNative
	nativeResult, err := ComputeAnalysis(input, query)
	if err != nil {
		t.Fatal(err)
	}
	nativeDay := nativeResult.Days[0]
	if nativeDay.Status != domain.CompletenessPartial || nativeDay.Residual == nil || !analyticsPhase1aBucket(nativeDay, domain.BucketPriceChange).Equal(decimal.NewFromInt(10)) || !analyticsPhase1aBucket(nativeDay, domain.BucketFXImpact).IsZero() {
		t.Fatalf("native quantity error was not explicit: %+v", nativeDay)
	}
	reviewAssertIdentity(t, day, "1584")
	reviewAssertIdentity(t, nativeDay, "220")
}

func TestAnalyticsPhase1aReviewCase17CashInterestIsSignedSpending(t *testing.T) {
	householdID := domain.NewHouseholdID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", "1000", "1000", nil, nil, "", ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", "500", "500", nil, nil, "", ""))
	activityID := domain.NewActivityID()
	activity := domain.Activity{ID: activityID, Kind: domain.ActivityCashOut, Reason: domain.ReasonInterest, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{analyticsPhase1aEffect(t, activityID, 1, account.ID, "500", "CNY", domain.EffectRemoved, domain.EffectTargetAccountValue, domain.ClassificationExternalOutflow)}}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if got := analyticsPhase1aBucket(day, domain.BucketSpending); !got.Equal(decimal.NewFromInt(-500)) || !analyticsPhase1aBucket(day, domain.BucketFee).IsZero() || day.Residual != nil {
		t.Fatalf("cash interest was not signed spending: %+v", day)
	}
	reviewAssertIdentity(t, day, "500")
}

func TestAnalyticsPhase1aReviewCase04SameDayBuyUsesTradePath(t *testing.T) {
	householdID := domain.NewHouseholdID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Type: domain.InstrumentStock, QuoteCurrency: "USD"}
	openFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: func() domain.FxRate { value, _ := domain.ParseFxRate("7"); return value }(), QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	eventFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: func() domain.FxRate { value, _ := domain.ParseFxRate("7.1"); return value }(), QuotedAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}
	closeFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: func() domain.FxRate { value, _ := domain.ParseFxRate("7.2"); return value }(), QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "110"), Currency: "USD", QuotedAt: closeFX.QuotedAt}
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "USD", "100", "700", nil, nil, "", openFX.ID.String()), analyticsPhase1aItem(t, account.ID, "USD", "0", "0", &holdingID, &instrumentID, "", ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "USD", "0", "0", nil, nil, "", closeFX.ID.String()), analyticsPhase1aItem(t, account.ID, "USD", "110", "792", &holdingID, &instrumentID, closeQuote.ID.String(), closeFX.ID.String()))
	activityID := domain.NewActivityID()
	quantity := mustQuantity(t, "1")
	gross := mustMoney(t, "100", "USD")
	unitPrice := mustUnitPrice(t, "100")
	activity := domain.Activity{ID: activityID, Kind: domain.ActivityBuy, Reason: domain.ReasonPrincipal, EffectiveAt: eventFX.QuotedAt, EffectiveLocalDate: "2026-08-02", TradeDetail: &domain.TradeDetail{Side: domain.TradeBuy, InstrumentID: instrumentID, HoldingID: holdingID, Quantity: quantity, Gross: gross, UnitPrice: unitPrice}, Effects: []domain.ActivityEffect{
		{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Direction: domain.EffectAdded, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationTradePrincipal, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity},
		{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 2, Direction: domain.EffectRemoved, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationTradePrincipal, AccountID: &account.ID, Money: &gross},
	}}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}, InstrumentQuotes: []domain.InstrumentQuote{closeQuote}, FXQuotes: []domain.FXQuote{openFX, eventFX, closeFX}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	holdingDay := analyticsPhase1aFindDay(t, result, domain.ComponentID{AccountID: account.ID, HoldingID: &holdingID, InstrumentID: &instrumentID, Currency: "USD"})
	if got := analyticsPhase1aBucket(holdingDay, domain.BucketPriceChange); !got.Equal(decimal.NewFromInt(71)) {
		t.Fatalf("same-day buy price = %s, want 71", got)
	}
	if got := analyticsPhase1aBucket(holdingDay, domain.BucketFXImpact); !got.Equal(decimal.NewFromInt(11)) {
		t.Fatalf("same-day buy FX = %s, want 11", got)
	}
	reviewAssertPeriodIdentity(t, result, "792")
}

func TestAnalyticsPhase1aReviewCase38GiftIsIncomeNotExternalFlow(t *testing.T) {
	householdID := domain.NewHouseholdID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	account := analyticsPhase1aAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	previous := analyticsPhase1aSnapshot("2026-08-01", analyticsPhase1aItem(t, account.ID, "CNY", "100", "100", nil, nil, "", ""))
	current := analyticsPhase1aSnapshot("2026-08-02", analyticsPhase1aItem(t, account.ID, "CNY", "200", "200", nil, nil, "", ""))
	activityID := domain.NewActivityID()
	activity := domain.Activity{ID: activityID, Kind: domain.ActivityCashIn, Reason: domain.ReasonGift, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{analyticsPhase1aEffect(t, activityID, 1, account.ID, "100", "CNY", domain.EffectAdded, domain.EffectTargetAccountValue, domain.ClassificationExternalInflow)}}
	input := AnalysisInputs{Origin: analyticsPhase1aOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}}
	result, err := ComputeAnalysis(input, analyticsPhase1aBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	day := result.Days[0]
	if got := analyticsPhase1aBucket(day, domain.BucketIncome); !got.Equal(decimal.NewFromInt(100)) || !analyticsPhase1aBucket(day, domain.BucketExternalFlow).IsZero() || day.Residual != nil {
		t.Fatalf("gift classification = %+v", day)
	}
	reviewAssertIdentity(t, day, "200")
}
