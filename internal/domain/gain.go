package domain

// SignedMoneyView is the display-safe representation of a signed gain or
// loss. It mirrors MoneyView while allowing losses to remain negative.
type SignedMoneyView struct {
	Amount   string
	Currency CurrencyCode
}

// HoldingGainView is a read model for one active Holding. Cost values are in
// the Instrument's quote currency. Cost is nil when acquisition evidence does
// not cover the current quantity; an explicit zero cost remains available.
// CurrentValue and UnrealizedGain are nil when their inputs are unavailable; they are never
// replaced with a misleading zero. RealizedGain remains in the Instrument's
// quote currency and is derived from historical replay.
type HoldingGainView struct {
	HoldingID           HoldingID
	AccountID           AccountID
	InstrumentID        InstrumentID
	InstrumentName      string
	InstrumentShortName string
	InstrumentSymbol    string
	Quantity            string
	AverageCost         *MoneyView
	TotalCost           *MoneyView
	CostMissingReason   string
	TotalCostBase       *MoneyView
	CurrentValue        *MoneyView
	CurrentValueBase    *MoneyView
	RealizedGain        *SignedMoneyView
	UnrealizedGain      *SignedMoneyView
	UnrealizedGainBase  *SignedMoneyView
	InstrumentMovement  *SignedMoneyView
	CurrencyMovement    *SignedMoneyView
	Available           bool
	MissingReason       string
}

// AccountGainView groups the Holding gain read models for one account.
// RealizedGain is expressed in the Household base currency using FX available
// at each sale's effective time; it is independent of current quote inputs.
// CurrentValue, TotalCost, and UnrealizedGain use current inputs and can be
// unavailable while RealizedGain remains known, or vice versa.
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

// RealizedGainSelector filters realized sale events after cost basis has been
// replayed from the complete gain snapshot. A selector does not limit the
// replay inputs: prior acquisitions, archived holdings and transfer sources
// may still be needed to calculate a selected event's basis. NoMatch records
// contradictory scope/filter constraints, which produce an available empty
// result instead of widening to a broader scope.
//
// AssetClass is the instrument's current Type metadata, matching analysis
// universe classification. Currency is the realized sale event's native
// denomination, not account default or output currency.
type RealizedGainSelector struct {
	AccountID    *AccountID
	InstrumentID *InstrumentID
	Currency     *CurrencyCode
	AssetClass   string
	NoMatch      bool
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
	Total         *SignedMoneyView
	ByInstrument  []GainGroupView
	ByAccount     []GainGroupView
	Available     bool
	MissingReason string
}

// DividendIncomeView groups cash-dividend income by Instrument and Account
// for one inclusive local-date range. It is independent of realized sell
// gain: missing FX affects only the affected group and is never shown as a
// zero income amount for that missing observation.
type DividendIncomeView struct {
	From          LocalDate
	To            LocalDate
	Currency      CurrencyCode
	Total         *SignedMoneyView
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
