package domain

import "time"

// PositionImportInput records an existing position already held outside the
// ledger. Quantity is the opening quantity and UnitCost is its acquisition
// cost per unit in the instrument's quote currency. It does not move cash.
type PositionImportInput struct {
	HouseholdID  HouseholdID
	AccountID    AccountID
	InstrumentID InstrumentID
	Quantity     Quantity
	UnitCost     *UnitPrice
	Currency     CurrencyCode
	EffectiveAt  time.Time
	Note         *string
}
