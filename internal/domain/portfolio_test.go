package domain

import (
	"testing"
	"time"
)

func TestInstrumentProviderBindingAndPortfolioRelationships(t *testing.T) {
	householdID := HouseholdID(newID())
	if _, err := NewInstrument(InstrumentInput{HouseholdID: householdID, Name: "QQQ", Type: InstrumentETF, QuoteCurrency: CurrencyCode("USD"), QuoteSource: QuoteSourceProvider}, time.Now()); err == nil {
		t.Fatal("provider instrument without a complete binding succeeded")
	}
	key, symbol := "yahoo_finance", "qqq"
	instrument, err := NewInstrument(InstrumentInput{HouseholdID: householdID, Name: "QQQ", Type: InstrumentETF, QuoteCurrency: CurrencyCode("USD"), CountryCode: stringPtr("us"), QuoteSource: QuoteSourceProvider, ProviderKey: &key, ProviderSymbol: &symbol}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if instrument.ProviderSymbol == nil || *instrument.ProviderSymbol != "QQQ" || instrument.CountryCode == nil || *instrument.CountryCode != "US" {
		t.Fatalf("instrument metadata was not normalized: %+v", instrument)
	}
	account, _, initial, err := NewAccount(AccountInput{HouseholdID: householdID, Name: "Brokerage", AccountType: TypeBrokerage, BalanceSheetRole: RoleAsset, TrackingMode: TrackingHoldings, DefaultCurrency: CurrencyCode("SGD"), Ownership: []OwnershipShare{{MemberID: MemberID(newID()), ShareBPS: TotalOwnershipBPS}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if initial != nil {
		t.Fatal("Holdings Account returned an initial Account Value")
	}
	quantity, err := ParseQuantity("3")
	if err != nil {
		t.Fatal(err)
	}
	holding, err := NewHoldingForAccount(account, instrument, quantity, nil, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if holding.Quantity.Canonical() != "3" {
		t.Fatalf("holding quantity = %s", holding.Quantity.Canonical())
	}
	otherHousehold := instrument
	otherHousehold.HouseholdID = HouseholdID(newID())
	if _, err := NewHoldingForAccount(account, otherHousehold, quantity, nil, 0, time.Now()); err == nil {
		t.Fatal("cross-Household holding succeeded")
	}
}

func TestHoldingsAccountRejectsFakeInitialAmountAndCashUsesHoldingsMode(t *testing.T) {
	householdID := HouseholdID(newID())
	memberID := MemberID(newID())
	_, _, _, err := NewAccount(AccountInput{HouseholdID: householdID, Name: "Brokerage", AccountType: TypeBrokerage, BalanceSheetRole: RoleAsset, TrackingMode: TrackingHoldings, DefaultCurrency: CurrencyCode("CNY"), Ownership: []OwnershipShare{{MemberID: memberID, ShareBPS: TotalOwnershipBPS}}, InitialAmount: "1"}, time.Now())
	if err == nil {
		t.Fatal("Holdings Account accepted an initial amount")
	}
	account, _, _, err := NewAccount(AccountInput{HouseholdID: householdID, Name: "Brokerage", AccountType: TypeBrokerage, BalanceSheetRole: RoleAsset, TrackingMode: TrackingHoldings, DefaultCurrency: CurrencyCode("CNY"), Ownership: []OwnershipShare{{MemberID: memberID, ShareBPS: TotalOwnershipBPS}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	money, err := ParseMoney("0", CurrencyCode("USD"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAccountCashValue(account, money, time.Now(), time.Now()); err != nil {
		t.Fatalf("zero cash observation failed: %v", err)
	}
	value, err := ParseMoney("1", CurrencyCode("USD"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAccountValue(account, value, time.Now(), time.Now()); err == nil {
		t.Fatal("Holdings Account accepted a legacy Account Value")
	}
}

func TestQuoteContractsNormalizeSourceAndFXPair(t *testing.T) {
	householdID := HouseholdID(newID())
	instrument, err := NewInstrument(InstrumentInput{HouseholdID: householdID, Name: "Fund", Type: InstrumentETF, QuoteCurrency: CurrencyCode("USD")}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	price, err := ParseUnitPrice("10")
	if err != nil {
		t.Fatal(err)
	}
	quote, err := NewInstrumentQuote(instrument, InstrumentQuoteInput{UnitPrice: price, SourceKind: QuoteSourceManual, QuotedAt: time.Now()}, time.Now())
	if err != nil || quote.SourceKey != string(QuoteSourceManual) {
		t.Fatalf("manual quote = %+v, err = %v", quote, err)
	}
	rate, err := ParseFxRate("1.2")
	if err != nil {
		t.Fatal(err)
	}
	fx, err := NewFXQuote(FXQuoteInput{HouseholdID: householdID, BaseCurrency: CurrencyCode("USD"), QuoteCurrency: CurrencyCode("CNY"), Rate: rate, SourceKind: QuoteSourceProvider, SourceKey: "yahoo_finance", QuotedAt: time.Now()}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if fx.BaseCurrency != CurrencyCode("USD") || fx.QuoteCurrency != CurrencyCode("CNY") {
		t.Fatalf("FX orientation changed: %+v", fx)
	}
	preference, err := NewFXPreference(householdID, CurrencyCode("CNY"), CurrencyCode("USD"), QuoteSourceProvider, time.Now())
	if err != nil || preference.CurrencyA != CurrencyCode("CNY") || preference.CurrencyB != CurrencyCode("USD") {
		t.Fatalf("FX preference = %+v, err = %v", preference, err)
	}
	if _, err := NewFXQuote(FXQuoteInput{HouseholdID: householdID, BaseCurrency: CurrencyCode("CNY"), QuoteCurrency: CurrencyCode("CNY"), Rate: rate}, time.Now()); err == nil {
		t.Fatal("same-currency FX quote succeeded")
	}
}
