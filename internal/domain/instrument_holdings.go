package domain

// HoldingAmounts contains independent native-currency values. A missing field
// is not a zero and does not suppress the other fields.
type HoldingAmounts struct {
	Quantity           string
	TotalCost          *MoneyView
	CurrentValue       *MoneyView
	UnrealizedGain     *SignedMoneyView
	CostMissingReason  string
	ValueMissingReason string
	GainMissingReason  string
}

type InstrumentHoldingMember struct {
	HoldingID   HoldingID
	AccountID   AccountID
	AccountName string
	Amounts     HoldingAmounts
}

type InstrumentHoldingsView struct {
	InstrumentID  InstrumentID
	Name          string
	Symbol        string
	QuoteCurrency CurrencyCode
	QuantityUnit  string
	Archived      bool
	Amounts       HoldingAmounts
	Holdings      []InstrumentHoldingMember
}
