package domain

import (
	"testing"
	"time"
)

func reviewFixture(t *testing.T) (PortfolioSnapshot, Instrument) {
	t.Helper()
	household := &Household{ID: NewHouseholdID()}
	instrument := Instrument{ID: NewInstrumentID(), HouseholdID: household.ID, Name: "Synthetic fund", Type: InstrumentMutualFund, QuoteCurrency: "USD", QuoteSource: QuoteSourceAgent}
	account := Account{ID: NewAccountID(), TrackingMode: TrackingHoldings}
	quantity, _ := ParseQuantity("10")
	return PortfolioSnapshot{Household: household, Instruments: []Instrument{instrument}, Accounts: []AccountRecord{{Account: account}}, Holdings: []Holding{{ID: NewHoldingID(), AccountID: account.ID, InstrumentID: instrument.ID, Quantity: quantity}}}, instrument
}

func reviewQuote(t *testing.T, i Instrument, date, value string) InstrumentQuote {
	t.Helper()
	price, err := ParseUnitPrice(value)
	if err != nil {
		t.Fatal(err)
	}
	when, err := time.Parse(time.DateOnly, date)
	if err != nil {
		t.Fatal(err)
	}
	return InstrumentQuote{ID: NewInstrumentQuoteID(), InstrumentID: i.ID, UnitPrice: price, Currency: i.QuoteCurrency, SourceKind: QuoteSourceAgent, SourceKey: "agent", QuotedAt: when, CreatedAt: when, ValueEffectiveAt: when, EffectiveDate: date, ObservationKind: "close", PriceBasis: "agent_unit_nav_v1", TimestampBasis: "date_label"}
}
