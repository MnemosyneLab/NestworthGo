package domain

import (
	"encoding/hex"
	"time"
)

// MobileQuoteReceipt is private durable recovery evidence. Exact facts and the
// receipt are inserted by one repository transaction; it is never a setting.
type MobileQuoteReceipt struct {
	OperationID    string       `json:"operationId"`
	HouseholdID    HouseholdID  `json:"householdId"`
	DeviceID       string       `json:"deviceId"`
	PayloadHash    string       `json:"payloadHash"`
	Kind           string       `json:"kind"`
	FactID         string       `json:"factId"`
	InstrumentID   InstrumentID `json:"instrumentId,omitempty"`
	Currency       string       `json:"currency"`
	QuoteCurrency  string       `json:"quoteCurrency,omitempty"`
	Value          string       `json:"value"`
	QuotedAt       time.Time    `json:"quotedAt"`
	EffectiveDate  string       `json:"effectiveDate,omitempty"`
	RecordedAt     time.Time    `json:"recordedAt"`
	DerivedPending bool         `json:"derivedPending,omitempty"`
}

func (m MobileQuoteReceipt) Validate() error {
	if _, err := ParseMutationID(m.OperationID); err != nil {
		return err
	}
	if _, err := ParseHouseholdID(m.HouseholdID.String()); err != nil {
		return err
	}
	if hash, err := hex.DecodeString(m.PayloadHash); err != nil || len(hash) != 32 {
		return validation("payloadHash", "invalid receipt digest")
	}
	if m.DeviceID == "" || len(m.DeviceID) > 100 || m.QuotedAt.IsZero() || m.RecordedAt.IsZero() {
		return validation("receipt", "missing receipt evidence")
	}
	if _, err := ParseSupportedCurrency(m.Currency); err != nil {
		return err
	}
	switch m.Kind {
	case "instrument":
		if _, err := ParseInstrumentID(m.InstrumentID.String()); err != nil {
			return err
		}
		if _, err := ParseInstrumentQuoteID(m.FactID); err != nil {
			return err
		}
		_, err := ParseUnitPrice(m.Value)
		return err
	case "fx":
		if _, err := ParseFXQuoteID(m.FactID); err != nil {
			return err
		}
		if _, err := ParseSupportedCurrency(m.QuoteCurrency); err != nil {
			return err
		}
		_, err := ParseFxRate(m.Value)
		return err
	}
	return validation("kind", "invalid quote receipt")
}
