package domain

import (
	"testing"
	"time"
)

func changeTestState(t *testing.T) (ChangeState, AccountID, AccountID, HoldingID, HoldingID) {
	t.Helper()
	household := HouseholdID(newID())
	owner := AccountID(newID())
	broker := AccountID(newID())
	qqq := HoldingID(newID())
	qqqDestination := HoldingID(newID())
	qqqInstrument := InstrumentID(newID())
	cny, err := ParseCurrency("CNY")
	if err != nil {
		t.Fatal(err)
	}
	usd, err := ParseCurrency("USD")
	if err != nil {
		t.Fatal(err)
	}
	ownerMoney, err := ParseMoney("10000", cny)
	if err != nil {
		t.Fatal(err)
	}
	brokerMoney, err := ParseMoney("1000", usd)
	if err != nil {
		t.Fatal(err)
	}
	zero, _ := ParseQuantity("0")
	three, _ := ParseQuantity("3")
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	return ChangeState{
		HouseholdID: household, OriginAt: now.Add(-24 * time.Hour), Timezone: "Asia/Shanghai", Now: now,
		Accounts: map[AccountID]ChangeAccountState{
			owner:  {ID: owner, Name: "Family Cash", Currency: cny, Mode: TrackingBalance, Current: ownerMoney},
			broker: {ID: broker, Name: "Broker Cash", Currency: usd, Mode: TrackingHoldings, Current: brokerMoney},
		},
		Cash: map[AccountID]map[CurrencyCode]Money{broker: {usd: brokerMoney}},
		Holdings: map[HoldingID]ChangeHoldingState{
			qqq:            {ID: qqq, AccountID: broker, InstrumentID: qqqInstrument, InstrumentName: "QQQ", Currency: usd, Current: three},
			qqqDestination: {ID: qqqDestination, AccountID: broker, InstrumentID: qqqInstrument, InstrumentName: "QQQ", Currency: usd, Current: zero},
		},
	}, owner, broker, qqq, qqqDestination
}

func TestPreviewMoneyAddedAndRemovedUsesTypedEffects(t *testing.T) {
	state, _, _, _, _ := changeTestState(t)
	amount, _ := ParseMoney("500", state.Accounts[findAccount(state, "Family Cash")].Currency)
	preview, err := PreviewChange(state, MoneyAddedInput{HouseholdID: state.HouseholdID, AccountID: findAccount(state, "Family Cash"), Amount: amount, Reason: ReasonIncome})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Activity.Kind != ActivityCashIn || preview.Effects[0].Classification != ClassificationIncome || preview.Resulting[0].Amount != "10500" {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.Effects[0].Target != EffectTargetAccountValue || preview.Effects[0].Money == nil || preview.Effects[0].Quantity != nil {
		t.Fatalf("effect = %+v", preview.Effects[0])
	}
}

func TestPreviewCrossCurrencyTransferDerivesExactRateAndRejectsInsufficientCash(t *testing.T) {
	state, _, broker, _, _ := changeTestState(t)
	cny, _ := ParseCurrency("CNY")
	usd, _ := ParseCurrency("USD")
	sent, _ := ParseMoney("100", usd)
	received, _ := ParseMoney("690", cny)
	destination := AccountID(newID())
	destinationMoney, _ := ParseMoney("0", cny)
	state.Accounts[destination] = ChangeAccountState{ID: destination, Name: "CNY Cash", Currency: cny, Mode: TrackingHoldings, Current: destinationMoney}
	state.Cash[destination] = map[CurrencyCode]Money{cny: destinationMoney}
	preview, err := PreviewChange(state, CashTransferInput{HouseholdID: state.HouseholdID, FromAccountID: broker, ToAccountID: destination, Sent: sent, Received: received})
	if err != nil {
		t.Fatal(err)
	}
	if preview.DerivedRate == nil || preview.DerivedRate.Canonical() != "6.9" || preview.Activity.TransactionFXRate == nil {
		t.Fatalf("rate = %+v", preview)
	}
	tooMuch, _ := ParseMoney("1001", usd)
	_, err = PreviewChange(state, CashTransferInput{HouseholdID: state.HouseholdID, FromAccountID: broker, ToAccountID: destination, Sent: tooMuch, Received: received})
	if err == nil || err.(*Error).Code != ErrInsufficientBalance {
		t.Fatalf("too much transfer error = %v", err)
	}
}

func TestPreviewFXConversionUpdatesTwoCurrenciesAtomically(t *testing.T) {
	state, _, broker, _, _ := changeTestState(t)
	usd, _ := ParseCurrency("USD")
	sgd, _ := ParseCurrency("SGD")
	sold, _ := ParseMoney("100", usd)
	bought, _ := ParseMoney("135", sgd)
	fee, _ := ParseMoney("1", usd)
	preview, err := PreviewChange(state, FXConversionInput{HouseholdID: state.HouseholdID, AccountID: broker, Sold: sold, Bought: bought, Fee: &fee, EffectiveAt: state.Now})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Activity.Kind != ActivityFXConversion || preview.DerivedRate == nil || preview.DerivedRate.Canonical() != "1.35" {
		t.Fatalf("conversion preview = %+v", preview)
	}
	if len(preview.Effects) != 3 || preview.Effects[0].Direction != EffectRemoved || preview.Effects[1].Direction != EffectAdded || preview.Effects[2].Role != EffectRoleFee {
		t.Fatalf("conversion effects = %+v", preview.Effects)
	}
	if preview.Resulting[0].Currency != usd || preview.Resulting[0].Amount != "899" || preview.Resulting[1].Currency != sgd || preview.Resulting[1].Amount != "135" {
		t.Fatalf("conversion result = %+v", preview.Resulting)
	}
}

func TestPreviewMoneyAddedAllowsForeignCurrencyOnHoldingsAndRejectsOnSimple(t *testing.T) {
	state, _, broker, _, _ := changeTestState(t)
	cny, _ := ParseCurrency("CNY")
	usd, _ := ParseCurrency("USD")
	foreign, _ := ParseMoney("250", cny)
	preview, err := PreviewChange(state, MoneyAddedInput{HouseholdID: state.HouseholdID, AccountID: broker, Amount: foreign, Reason: ReasonContribution})
	if err != nil {
		t.Fatalf("holdings foreign cash: %v", err)
	}
	if preview.Activity.Kind != ActivityCashIn || preview.Effects[0].Target != EffectTargetAccountCash {
		t.Fatalf("holdings preview = %+v", preview)
	}
	if preview.Resulting[0].Currency != cny || preview.Resulting[0].Amount != "250" {
		t.Fatalf("holdings resulting = %+v", preview.Resulting)
	}

	simple := findAccount(state, "Family Cash")
	foreignUSD, _ := ParseMoney("50", usd)
	_, err = PreviewChange(state, MoneyAddedInput{HouseholdID: state.HouseholdID, AccountID: simple, Amount: foreignUSD, Reason: ReasonIncome})
	if err == nil || err.(*Error).Code != ErrInvalidChange {
		t.Fatalf("simple foreign cash error = %v, want invalid change", err)
	}

	removed, _ := ParseMoney("100", cny)
	_, err = PreviewChange(state, MoneyRemovedInput{HouseholdID: state.HouseholdID, AccountID: broker, Amount: removed, Reason: ReasonExpense})
	if err == nil || err.(*Error).Code != ErrInsufficientBalance {
		t.Fatalf("removing missing CNY cash error = %v, want insufficient balance", err)
	}
}

func TestPreviewCashDividendCreditsHoldingsCashWithoutChangingQuantity(t *testing.T) {
	state, _, broker, qqq, _ := changeTestState(t)
	usd, _ := ParseCurrency("USD")
	cny, _ := ParseCurrency("CNY")
	amount, _ := ParseMoney("12.5", usd)
	preview, err := PreviewChange(state, CashDividendInput{HouseholdID: state.HouseholdID, HoldingID: qqq, Amount: amount, EffectiveAt: state.Now})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Activity.Kind != ActivityCashDividend || preview.Activity.Reason != ReasonIncome {
		t.Fatalf("activity = %+v", preview.Activity)
	}
	if preview.Activity.DividendDetail == nil || preview.Activity.DividendDetail.HoldingID != qqq || preview.Activity.DividendDetail.Amount.CanonicalAmount() != "12.5" {
		t.Fatalf("dividend detail = %+v", preview.Activity.DividendDetail)
	}
	if len(preview.Effects) != 1 || preview.Effects[0].Target != EffectTargetAccountCash || preview.Effects[0].Classification != ClassificationIncome || preview.Effects[0].HoldingID != nil {
		t.Fatalf("effect = %+v", preview.Effects[0])
	}
	if preview.Resulting[0].Amount != "1012.5" || preview.Resulting[0].Currency != usd {
		t.Fatalf("resulting = %+v", preview.Resulting)
	}
	if state.Holdings[qqq].Current.Canonical() != "3" {
		t.Fatalf("quantity mutated in preview state = %s", state.Holdings[qqq].Current.Canonical())
	}

	foreign, _ := ParseMoney("80", cny)
	foreignPreview, err := PreviewChange(state, CashDividendInput{HouseholdID: state.HouseholdID, HoldingID: qqq, Amount: foreign, EffectiveAt: state.Now})
	if err != nil {
		t.Fatalf("foreign dividend: %v", err)
	}
	if foreignPreview.Resulting[0].Currency != cny || foreignPreview.Resulting[0].Amount != "80" {
		t.Fatalf("foreign resulting = %+v", foreignPreview.Resulting)
	}

	zero, _ := ParseMoney("0", usd)
	if _, err := PreviewChange(state, CashDividendInput{HouseholdID: state.HouseholdID, HoldingID: qqq, Amount: zero}); err == nil || err.(*Error).Code != ErrInvalidChange {
		t.Fatalf("zero amount error = %v", err)
	}

	simple := findAccount(state, "Family Cash")
	orphaned := HoldingID(newID())
	state.Holdings[orphaned] = ChangeHoldingState{ID: orphaned, AccountID: simple, InstrumentID: state.Holdings[qqq].InstrumentID, InstrumentName: "Cash Stock", Currency: cny, Current: state.Holdings[qqq].Current}
	if _, err := PreviewChange(state, CashDividendInput{HouseholdID: state.HouseholdID, HoldingID: orphaned, Amount: amount}); err == nil || err.(*Error).Code != ErrInvalidChange {
		t.Fatalf("simple-account dividend error = %v", err)
	}

	archived := HoldingID(newID())
	state.Holdings[archived] = ChangeHoldingState{ID: archived, AccountID: broker, InstrumentID: state.Holdings[qqq].InstrumentID, InstrumentName: "QQQ", Currency: usd, Archived: true, Current: state.Holdings[qqq].Current}
	if _, err := PreviewChange(state, CashDividendInput{HouseholdID: state.HouseholdID, HoldingID: archived, Amount: amount}); err == nil || err.(*Error).Code != ErrConflict {
		t.Fatalf("archived holding error = %v", err)
	}

	if _, err := PreviewChange(state, CashDividendInput{HouseholdID: state.HouseholdID, HoldingID: HoldingID(newID()), Amount: amount}); err == nil || err.(*Error).Code != ErrNotFound {
		t.Fatalf("missing holding error = %v", err)
	}
}

func TestPreviewFXConversionRejectsMissingSoldCash(t *testing.T) {
	state, _, broker, _, _ := changeTestState(t)
	usd, _ := ParseCurrency("USD")
	sgd, _ := ParseCurrency("SGD")
	sold, _ := ParseMoney("1001", usd)
	bought, _ := ParseMoney("1351.35", sgd)
	_, err := PreviewChange(state, FXConversionInput{HouseholdID: state.HouseholdID, AccountID: broker, Sold: sold, Bought: bought, EffectiveAt: state.Now})
	if err == nil || err.(*Error).Code != ErrInsufficientBalance {
		t.Fatalf("missing sold cash error = %v", err)
	}
}

func TestPreviewSameCurrencyTransferSupportsBalanceAccounts(t *testing.T) {
	state, owner, _, _, _ := changeTestState(t)
	destination := AccountID(newID())
	cny := state.Accounts[owner].Currency
	zero, _ := ParseMoney("0", cny)
	state.Accounts[destination] = ChangeAccountState{ID: destination, Name: "Reserve", Currency: cny, Mode: TrackingBalance, Current: zero}
	sent, _ := ParseMoney("125", cny)
	preview, err := PreviewChange(state, CashTransferInput{HouseholdID: state.HouseholdID, FromAccountID: owner, ToAccountID: destination, Sent: sent, Received: sent})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Effects[0].Target != EffectTargetAccountValue || preview.Effects[1].Target != EffectTargetAccountValue || preview.Resulting[0].Amount != "9875" || preview.Resulting[1].Amount != "125" {
		t.Fatalf("balance transfer = %+v", preview)
	}
}

func TestDebtDrawUsesDistinctNonLiabilityCashEndpoint(t *testing.T) {
	state, owner, _, _, _ := changeTestState(t)
	principal, _ := ParseMoney("100", "CNY")
	debtID := AccountID(newID())
	liabilityCashID := AccountID(newID())
	state.Accounts[debtID] = ChangeAccountState{ID: debtID, Name: "Debt", Currency: principal.Currency(), Mode: TrackingBalance, Liability: true, Current: principal}
	state.Accounts[liabilityCashID] = ChangeAccountState{ID: liabilityCashID, Name: "Other debt", Currency: principal.Currency(), Mode: TrackingBalance, Liability: true, Current: principal}

	for name, input := range map[string]DebtDrawInput{
		"same endpoint":  {HouseholdID: state.HouseholdID, DebtAccountID: debtID, CashAccountID: debtID, Principal: principal},
		"liability cash": {HouseholdID: state.HouseholdID, DebtAccountID: debtID, CashAccountID: liabilityCashID, Principal: principal},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := PreviewChange(state, input); err == nil {
				t.Fatal("invalid debt endpoint was accepted")
			}
		})
	}

	state.Accounts[owner] = ChangeAccountState{ID: owner, Name: "Cash", Currency: principal.Currency(), Mode: TrackingBalance, Current: principal}
	preview, err := PreviewChange(state, DebtDrawInput{HouseholdID: state.HouseholdID, DebtAccountID: debtID, CashAccountID: owner, Principal: principal})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Effects) != 2 || preview.Effects[0].Target != EffectTargetAccountValue || preview.Effects[1].Target != EffectTargetAccountValue || preview.Resulting[0].Amount != "200" || preview.Resulting[1].Amount != "200" {
		t.Fatalf("valid debt draw = %+v", preview)
	}
}

func TestPreviewTradeValueUpdateAndNoChange(t *testing.T) {
	state, _, broker, qqq, _ := changeTestState(t)
	quantity, _ := ParseQuantity("2")
	gross, _ := ParseMoney("200", state.Accounts[broker].Currency)
	fee, _ := ParseMoney("5", state.Accounts[broker].Currency)
	preview, err := PreviewChange(state, TradeInput{HouseholdID: state.HouseholdID, Side: TradeBuy, SettlementAccountID: broker, HoldingID: qqq, InstrumentID: state.Holdings[qqq].InstrumentID, Quantity: quantity, Gross: gross, Fee: &fee})
	if err != nil {
		t.Fatal(err)
	}
	if preview.DerivedUnitPrice == nil || preview.DerivedUnitPrice.Canonical() != "100" || preview.Resulting[0].Amount != "795" || len(preview.Effects) != 3 {
		t.Fatalf("trade preview = %+v", preview)
	}
	if preview.Activity.TradeDetail == nil || preview.Activity.TradeDetail.UnitPrice.Canonical() != "100" || preview.Activity.TradeDetail.Fee == nil {
		t.Fatalf("trade detail = %+v", preview.Activity.TradeDetail)
	}
	if preview.Activity.TradeDetail.HoldingID != qqq {
		t.Fatalf("trade resolved holding = %s, want %s", preview.Activity.TradeDetail.HoldingID, qqq)
	}
	updated, _ := ParseMoney("1000", state.Accounts[broker].Currency)
	_, err = PreviewChange(state, ValueUpdateInput{HouseholdID: state.HouseholdID, AccountID: broker, NewValue: updated})
	if err == nil || err.(*Error).Code != ErrInvalidChange {
		t.Fatalf("Holdings value update error = %v", err)
	}
	current := state.Accounts[findAccount(state, "Family Cash")].Current
	_, err = PreviewChange(state, ValueUpdateInput{HouseholdID: state.HouseholdID, AccountID: findAccount(state, "Family Cash"), NewValue: current})
	if err == nil || err.(*Error).Code != ErrNoChange {
		t.Fatalf("no-change error = %v", err)
	}
}

func TestResolveLocalDateTimeRejectsDSTGapAndAmbiguity(t *testing.T) {
	if _, err := ResolveLocalDateTime("2026-03-08", "02:30", "America/New_York"); err == nil || err.(*Error).Code != ErrInvalidChangeTime {
		t.Fatalf("DST gap error = %v", err)
	}
	if _, err := ResolveLocalDateTime("2026-11-01", "01:30", "America/New_York"); err == nil || err.(*Error).Code != ErrInvalidChangeTime {
		t.Fatalf("DST ambiguity error = %v", err)
	}
	resolved, err := ResolveLocalDateTime("2026-08-23", "20:00", "Asia/Shanghai")
	if err != nil || resolved.Location() != time.UTC || resolved.Format(time.RFC3339) != "2026-08-23T12:00:00Z" {
		t.Fatalf("resolved time = %v, error=%v", resolved, err)
	}
}

func TestPreviewPositionTransferRejectsDestinationOverflow(t *testing.T) {
	state, _, _, fromID, toID := changeTestState(t)
	maxQty, err := ParseQuantity("999999999999999999.99999999")
	if err != nil {
		t.Fatal(err)
	}
	holding := state.Holdings[toID]
	holding.Current = maxQty
	state.Holdings[toID] = holding
	quantity, err := ParseQuantity("0.00000001")
	if err != nil {
		t.Fatal(err)
	}
	_, err = PreviewChange(state, PositionTransferInput{HouseholdID: state.HouseholdID, FromHoldingID: fromID, ToHoldingID: toID, Quantity: quantity})
	if err == nil {
		t.Fatal("destination overflow was accepted")
	}
	if typed, ok := err.(*Error); !ok || typed.Code != ErrDecimalOverflow {
		t.Fatalf("overflow error = %v, want decimal_overflow", err)
	}
}

func findAccount(state ChangeState, name string) AccountID {
	for id, account := range state.Accounts {
		if account.Name == name {
			return id
		}
	}
	return ""
}
