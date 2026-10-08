package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// ProductTermsMutation is private local idempotency evidence, not a financial
// lifecycle operation. SQLite snapshots retain it with the contract/policy.
type ProductTermsMutation struct {
	ID             ProductOperationID  `json:"id"`
	HouseholdID    HouseholdID         `json:"householdId"`
	ProductID      ProductContractID   `json:"productId"`
	PayloadSHA256  string              `json:"payloadSha256"`
	CommandJSON    json.RawMessage     `json:"command"`
	EvidenceSHA256 string              `json:"evidenceSha256,omitempty"`
	Receipt        ProductTermsReceipt `json:"receipt"`
	CreatedAt      time.Time           `json:"createdAt"`
}

// NewProductTermsMutation seals the exact normalized command and immutable
// recorded-time result. This detects inconsistent local evidence, not a
// malicious rewrite of every fact and checksum in an unauthenticated database.
func NewProductTermsMutation(id ProductOperationID, household HouseholdID, command json.RawMessage, receipt ProductTermsReceipt, createdAt time.Time) (ProductTermsMutation, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, command); err != nil {
		return ProductTermsMutation{}, err
	}
	m := ProductTermsMutation{ID: id, HouseholdID: household, ProductID: receipt.Contract.ID, CommandJSON: append(json.RawMessage(nil), compact.Bytes()...), Receipt: receipt, CreatedAt: createdAt}
	m.PayloadSHA256 = productTermsHash(compact.Bytes())
	digest, err := m.evidenceHash()
	if err != nil {
		return ProductTermsMutation{}, err
	}
	m.EvidenceSHA256 = digest
	return m, m.Validate()
}
func productTermsHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (m ProductTermsMutation) evidenceHash() (string, error) {
	m.EvidenceSHA256 = ""
	raw, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return productTermsHash(raw), nil
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
	for _, money := range []*Money{r.Policy.AccessibleAmountCap, r.Policy.NormalExitFee, r.Policy.EarlyFee, r.Policy.EarlyGrossAmount} {
		if money != nil && money.Currency() != r.Contract.Currency {
			return &Error{Code: ErrValidation, Message: "product policy receipt amount must use the product currency"}
		}
	}
	if r.MutationID != m.ID || r.Contract.ID != m.ProductID || r.Contract.HouseholdID != m.HouseholdID || r.Policy.HouseholdID != m.HouseholdID || r.Policy.Source.Key() != HoldingSourceRef(r.Contract.AccountID, r.Contract.HoldingID).Key() || !r.RecordedAt.Equal(m.CreatedAt) || !r.Contract.UpdatedAt.Equal(m.CreatedAt) || r.CurrentValue != nil && r.CurrentValue.Currency() != r.Contract.Currency {
		return &Error{Code: ErrValidation, Message: "product terms receipt identity is inconsistent"}
	}

	var command struct {
		ProductID        string `json:"productId"`
		ExpectedRevision int    `json:"expectedRevision"`
	}
	var compact bytes.Buffer
	if json.Compact(&compact, m.CommandJSON) != nil || json.Unmarshal(compact.Bytes(), &command) != nil || command.ProductID != m.ProductID.String() || command.ExpectedRevision != r.Contract.Revision-1 || productTermsHash(compact.Bytes()) != m.PayloadSHA256 || r.Replayed {
		return &Error{Code: ErrValidation, Message: "product terms command evidence is inconsistent"}
	}
	bound, err := m.evidenceHash()
	if err != nil {
		return err
	}
	if m.EvidenceSHA256 == "" || bound != m.EvidenceSHA256 {
		return &Error{Code: ErrValidation, Message: "product terms command and result binding is inconsistent"}
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

type productTermsPolicyJSON struct {
	LiquidityPolicy
	AccessibleAmountCap *productReceiptMoney `json:"AccessibleAmountCap"`
	NormalExitFee       *productReceiptMoney `json:"NormalExitFee"`
	EarlyFee            *productReceiptMoney `json:"EarlyFee"`
	EarlyGrossAmount    *productReceiptMoney `json:"EarlyGrossAmount"`
}

func receiptMoneyPtr(m *Money) *productReceiptMoney {
	if m == nil {
		return nil
	}
	v := receiptMoney(*m)
	return &v
}
func termsPolicyJSON(p LiquidityPolicy) productTermsPolicyJSON {
	return productTermsPolicyJSON{p, receiptMoneyPtr(p.AccessibleAmountCap), receiptMoneyPtr(p.NormalExitFee), receiptMoneyPtr(p.EarlyFee), receiptMoneyPtr(p.EarlyGrossAmount)}
}
func (p productTermsPolicyJSON) policy() (LiquidityPolicy, error) {
	result := p.LiquidityPolicy
	for _, field := range []struct {
		raw    *productReceiptMoney
		target **Money
	}{{p.AccessibleAmountCap, &result.AccessibleAmountCap}, {p.NormalExitFee, &result.NormalExitFee}, {p.EarlyFee, &result.EarlyFee}, {p.EarlyGrossAmount, &result.EarlyGrossAmount}} {
		*field.target = nil
		if field.raw != nil {
			v, err := field.raw.money()
			if err != nil {
				return LiquidityPolicy{}, err
			}
			*field.target = &v
		}
	}
	return result, nil
}
func (r ProductTermsReceipt) MarshalJSON() ([]byte, error) {
	type alias ProductTermsReceipt
	return json.Marshal(struct {
		alias
		CurrentValue *productReceiptMoney   `json:"currentValue"`
		Policy       productTermsPolicyJSON `json:"policy"`
	}{alias(r), receiptMoneyPtr(r.CurrentValue), termsPolicyJSON(r.Policy)})
}
func (r *ProductTermsReceipt) UnmarshalJSON(raw []byte) error {
	type alias ProductTermsReceipt
	var p struct {
		alias
		CurrentValue *productReceiptMoney   `json:"currentValue"`
		Policy       productTermsPolicyJSON `json:"policy"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	*r = ProductTermsReceipt(p.alias)
	if p.CurrentValue != nil {
		v, err := p.CurrentValue.money()
		if err != nil {
			return err
		}
		r.CurrentValue = &v
	}
	policy, err := p.Policy.policy()
	if err != nil {
		return err
	}
	r.Policy = policy
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
