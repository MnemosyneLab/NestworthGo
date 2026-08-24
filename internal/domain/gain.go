package domain

// SignedMoneyView is the display-safe representation of a signed gain or
// loss. It mirrors MoneyView while allowing losses to remain negative.
type SignedMoneyView struct {
	Amount   string
	Currency CurrencyCode
}

// HoldingGainView is a read model for one active Holding. Cost values are in
// the Instrument's quote currency. CurrentValue and UnrealizedGain are nil
// when the current quote or required FX input is unavailable; they are never
// replaced with a misleading zero.
type HoldingGainView struct {
	HoldingID          HoldingID
	AccountID          AccountID
	InstrumentID       InstrumentID
	InstrumentName     string
	InstrumentSymbol   string
	Quantity           string
	AverageCost        MoneyView
	TotalCost          MoneyView
	TotalCostBase      *MoneyView
	CurrentValue       *MoneyView
	CurrentValueBase   *MoneyView
	RealizedGain       SignedMoneyView
	UnrealizedGain     *SignedMoneyView
	UnrealizedGainBase *SignedMoneyView
	InstrumentMovement *SignedMoneyView
	CurrencyMovement   *SignedMoneyView
	Available          bool
	MissingReason      string
}

// AccountGainView groups the Holding gain read models for one account. The
// account totals are expressed in the Household base currency when every
// required current input is available.
type AccountGainView struct {
	AccountID      AccountID
	Holdings       []HoldingGainView
	TotalCost      *MoneyView
	CurrentValue   *MoneyView
	RealizedGain   *SignedMoneyView
	UnrealizedGain *SignedMoneyView
	Available      bool
	MissingReason  string
}

// GainScope limits a realized-gain period to one Account and/or Instrument.
// A nil field means that dimension is not filtered.
type GainScope struct {
	AccountID    *AccountID
	InstrumentID *InstrumentID
}

// LocalDate is the canonical YYYY-MM-DD value already used by activities and
// historical snapshots. It is an alias so existing string-based application
// callers remain source-compatible.
type LocalDate = string

type GainGroupView struct {
	Key           string
	Label         string
	Gain          SignedMoneyView
	Available     bool
	MissingReason string
}

// RealizedGainView groups realized gains by Instrument and Account for one
// inclusive local-date range. Missing FX affects only the affected group and
// is never represented as a zero gain.
type RealizedGainView struct {
	From          LocalDate
	To            LocalDate
	Currency      CurrencyCode
	ByInstrument  []GainGroupView
	ByAccount     []GainGroupView
	Available     bool
	MissingReason string
}

// StartingPointHoldingView is the editable, family-facing draft used before
// history begins. UnitCost is prefilled from the selected current quote and
// is intentionally kept as a canonical string until the application parses
// and validates it.
type StartingPointHoldingView struct {
	HoldingID      HoldingID
	InstrumentID   InstrumentID
	InstrumentName string
	Currency       CurrencyCode
	Quantity       string
	UnitCost       string
}
