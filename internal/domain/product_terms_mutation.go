package domain

import (
	"encoding/hex"
	"encoding/json"
	"time"
)

// ProductTermsMutation is private local idempotency evidence, not a financial
// lifecycle operation. SQLite snapshots retain it with the contract/policy.
type ProductTermsMutation struct {
	ID            ProductOperationID  `json:"id"`
	HouseholdID   HouseholdID         `json:"householdId"`
	ProductID     ProductContractID   `json:"productId"`
	PayloadSHA256 string              `json:"payloadSha256"`
	Receipt       ProductTermsReceipt `json:"receipt"`
	CreatedAt     time.Time           `json:"createdAt"`
}

func (m ProductTermsMutation) Validate() error {
	if _, err := ParseProductOperationID(m.ID.String()); err != nil {
		return err
	}
	if _, err := ParseHouseholdID(m.HouseholdID.String()); err != nil {
		return err
	}
	if _, err := ParseProductContractID(m.ProductID.String()); err != nil {
		return err
	}
	digest, err := hex.DecodeString(m.PayloadSHA256)
	if err != nil || len(digest) != 32 || m.CreatedAt.IsZero() {
		return &Error{Code: ErrValidation, Message: "invalid product terms mutation evidence"}
	}
	if err := m.Receipt.Contract.Validate(); err != nil {
		return err
	}
	if err := m.Receipt.Policy.Validate(true); err != nil {
		return err
	}
	r := m.Receipt
	if r.MutationID != m.ID || r.Contract.ID != m.ProductID || r.Contract.HouseholdID != m.HouseholdID || r.Policy.HouseholdID != m.HouseholdID || r.Policy.Source.Key() != HoldingSourceRef(r.Contract.AccountID, r.Contract.HoldingID).Key() || !r.RecordedAt.Equal(m.CreatedAt) || !r.Contract.UpdatedAt.Equal(m.CreatedAt) || r.CurrentValue != nil && r.CurrentValue.Currency() != r.Contract.Currency {
		return &Error{Code: ErrValidation, Message: "product terms receipt identity is inconsistent"}
	}
	return nil
}

// ProductTermsReceipt captures the exact contract/policy result of one commit.
// CurrentValue is an unchanged recorded-time observation, not a fresh query.
type ProductTermsReceipt struct {
	MutationID       ProductOperationID  `json:"mutationId"`
	Contract         ProductContract     `json:"contract"`
	Policy           LiquidityPolicy     `json:"policy"`
	CurrentValue     *Money              `json:"currentValue"`
	DisplayState     ProductDisplayState `json:"displayState"`
	RecordedAt       time.Time           `json:"recordedAt"`
	Replayed         bool                `json:"replayed"`
	PermittedActions []string            `json:"permittedActions"`
	SuccessorID      *ProductContractID  `json:"successorId"`
}

// Receipt money has an explicit wire form; Money itself has private fields.
type productReceiptMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func receiptMoney(m Money) productReceiptMoney {
	return productReceiptMoney{m.CanonicalAmount(), m.Currency().String()}
}
func (m productReceiptMoney) money() (Money, error) {
	c, err := ParseCurrency(m.Currency)
	if err != nil {
		return Money{}, err
	}
	return ParseMoney(m.Amount, c)
}
func (r ProductTermsReceipt) MarshalJSON() ([]byte, error) {
	type alias ProductTermsReceipt
	var m *productReceiptMoney
	if r.CurrentValue != nil {
		v := receiptMoney(*r.CurrentValue)
		m = &v
	}
	return json.Marshal(struct {
		alias
		CurrentValue *productReceiptMoney `json:"currentValue"`
	}{alias(r), m})
}
func (r *ProductTermsReceipt) UnmarshalJSON(raw []byte) error {
	type alias ProductTermsReceipt
	var p struct {
		alias
		CurrentValue *productReceiptMoney `json:"currentValue"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	*r = ProductTermsReceipt(p.alias)
	if p.CurrentValue != nil {
		m, err := p.CurrentValue.money()
		if err != nil {
			return err
		}
		r.CurrentValue = &m
	}
	return nil
}

// ProductValuationReceipt is immutable evidence of one observed value.
type ProductValuationReceipt struct {
	OperationID ProductOperationID `json:"operationId"`
	ProductID   ProductContractID  `json:"productId"`
	QuoteID     InstrumentQuoteID  `json:"quoteId"`
	Amount      Money              `json:"amount"`
	ObservedAt  time.Time          `json:"observedAt"`
	RecordedAt  time.Time          `json:"recordedAt"`
	Replayed    bool               `json:"replayed"`
}

func (r ProductValuationReceipt) MarshalJSON() ([]byte, error) {
	type alias ProductValuationReceipt
	return json.Marshal(struct {
		alias
		Amount productReceiptMoney `json:"amount"`
	}{alias(r), receiptMoney(r.Amount)})
}
func (r *ProductValuationReceipt) UnmarshalJSON(raw []byte) error {
	type alias ProductValuationReceipt
	var p struct {
		alias
		Amount productReceiptMoney `json:"amount"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	m, err := p.Amount.money()
	if err != nil {
		return err
	}
	*r = ProductValuationReceipt(p.alias)
	r.Amount = m
	return r.Validate()
}
func (r ProductValuationReceipt) Validate() error {
	if _, err := ParseProductOperationID(r.OperationID.String()); err != nil {
		return err
	}
	if _, err := ParseProductContractID(r.ProductID.String()); err != nil {
		return err
	}
	if _, err := ParseInstrumentQuoteID(r.QuoteID.String()); err != nil {
		return err
	}
	if _, err := ParseCurrency(r.Amount.Currency().String()); err != nil {
		return err
	}
	if r.Amount.Amount().Sign() <= 0 || r.ObservedAt.IsZero() || r.RecordedAt.IsZero() || r.ObservedAt.After(r.RecordedAt) {
		return &Error{Code: ErrValidation, Message: "invalid product valuation receipt"}
	}
	return nil
}
