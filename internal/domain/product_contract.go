package domain

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ProductContractID identifies one managed term deposit or locked product.
type ProductContractID string
type LiquidityPolicyID string
type LiquidityReservationID string
type ProductOperationID string

func NewProductContractID() ProductContractID { return ProductContractID(newID()) }
func NewLiquidityPolicyID() LiquidityPolicyID { return LiquidityPolicyID(newID()) }
func NewLiquidityReservationID() LiquidityReservationID {
	return LiquidityReservationID(newID())
}
func NewProductOperationID() ProductOperationID { return ProductOperationID(newID()) }

func (id ProductContractID) String() string      { return string(id) }
func (id LiquidityPolicyID) String() string      { return string(id) }
func (id LiquidityReservationID) String() string { return string(id) }
func (id ProductOperationID) String() string     { return string(id) }

func ParseProductContractID(value string) (ProductContractID, error) {
	return parseID[ProductContractID](value, "productId")
}
func ParseLiquidityPolicyID(value string) (LiquidityPolicyID, error) {
	return parseID[LiquidityPolicyID](value, "policyId")
}
func ParseLiquidityReservationID(value string) (LiquidityReservationID, error) {
	return parseID[LiquidityReservationID](value, "reservationId")
}
func ParseProductOperationID(value string) (ProductOperationID, error) {
	return parseID[ProductOperationID](value, "operationId")
}

type ProductKind string

const (
	ProductTermDeposit   ProductKind = "term_deposit"
	ProductLockedProduct ProductKind = "locked_product"
)

func ParseProductKind(value string) (ProductKind, error) {
	kind := ProductKind(strings.TrimSpace(value))
	switch kind {
	case ProductTermDeposit, ProductLockedProduct:
		return kind, nil
	default:
		return "", validation("kind", "must be term_deposit or locked_product")
	}
}

func AllProductKinds() []ProductKind {
	return []ProductKind{ProductTermDeposit, ProductLockedProduct}
}

type ProductContractState string

const (
	ProductStateOpen      ProductContractState = "open"
	ProductStateSettled   ProductContractState = "settled"
	ProductStateCancelled ProductContractState = "cancelled"
)

func ParseProductContractState(value string) (ProductContractState, error) {
	state := ProductContractState(strings.TrimSpace(value))
	switch state {
	case ProductStateOpen, ProductStateSettled, ProductStateCancelled:
		return state, nil
	default:
		return "", validation("state", "must be open, settled, or cancelled")
	}
}

type InterestMode string

const (
	InterestNone                  InterestMode = "none"
	InterestManualMaturityAmount  InterestMode = "manual_maturity_amount"
	InterestSimpleAct365          InterestMode = "simple_act_365"
	InterestSimpleAct360          InterestMode = "simple_act_360"
)

func ParseInterestMode(value string) (InterestMode, error) {
	mode := InterestMode(strings.TrimSpace(value))
	switch mode {
	case InterestNone, InterestManualMaturityAmount, InterestSimpleAct365, InterestSimpleAct360:
		return mode, nil
	default:
		return "", validation("interestMode", "is not supported")
	}
}

func AllInterestModes() []InterestMode {
	return []InterestMode{InterestNone, InterestManualMaturityAmount, InterestSimpleAct365, InterestSimpleAct360}
}

type ProductDisplayState string

const (
	ProductDisplayLocked          ProductDisplayState = "locked"
	ProductDisplayRedeemable      ProductDisplayState = "redeemable"
	ProductDisplayDueUnconfirmed  ProductDisplayState = "due_unconfirmed"
	ProductDisplaySettled         ProductDisplayState = "settled"
	ProductDisplayCancelled       ProductDisplayState = "cancelled"
)

// AnnualRate is a canonical decimal ratio in [0, 1] with at most eight
// fractional digits. Form input may supply a percent string such as "2.5".
type AnnualRate struct{ value decimal.Decimal }

func ParseAnnualRateRatio(value string) (AnnualRate, error) {
	trimmed := strings.TrimSpace(value)
	parsed, err := decimal.NewFromString(trimmed)
	if err != nil {
		return AnnualRate{}, validation("annualRate", "must be a canonical decimal ratio")
	}
	return NewAnnualRate(parsed)
}

func ParseAnnualRatePercent(value string) (AnnualRate, error) {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimSuffix(trimmed, "%")
	parsed, err := decimal.NewFromString(trimmed)
	if err != nil {
		return AnnualRate{}, validation("annualRate", "must be a percent between 0 and 100")
	}
	if parsed.IsNegative() || parsed.GreaterThan(decimal.NewFromInt(100)) {
		return AnnualRate{}, validation("annualRate", "must be a percent between 0 and 100")
	}
	return NewAnnualRate(parsed.Div(decimal.NewFromInt(100)))
}

func NewAnnualRate(value decimal.Decimal) (AnnualRate, error) {
	if value.IsNegative() || value.GreaterThan(decimal.NewFromInt(1)) {
		return AnnualRate{}, validation("annualRate", "must be a ratio between 0 and 1")
	}
	canonical, err := decimal.NewFromString(canonicalDecimal(value))
	if err != nil {
		return AnnualRate{}, validation("annualRate", "must be a canonical decimal ratio")
	}
	scale, integerDigits := decimalShape(canonical)
	if integerDigits > 1 || scale > 8 {
		return AnnualRate{}, &Error{Code: ErrDecimalOverflow, Field: "annualRate", Message: "must have at most eight fractional digits"}
	}
	return AnnualRate{value: canonical}, nil
}

func (r AnnualRate) Decimal() decimal.Decimal { return r.value }
func (r AnnualRate) Canonical() string {
	if r.value.IsZero() {
		return "0"
	}
	return r.value.String()
}

// ProductContract is the persisted metadata for one managed product. Principal,
// owner account, currency, and original start date are immutable after create.
type ProductContract struct {
	ID                   ProductContractID
	HouseholdID          HouseholdID
	AccountID            AccountID
	HoldingID            HoldingID
	InstrumentID         InstrumentID
	Kind                 ProductKind
	Name                 string
	Note                 *string
	Currency             CurrencyCode
	Principal            Money
	StartOn              string
	MaturityOn           *string
	InterestMode         InterestMode
	AnnualRate           *AnnualRate
	MaturityInterest     *Money
	InterestPaidThroughOn *string
	RenewedFromID        *ProductContractID
	State                ProductContractState
	OpenedOperationID    ProductOperationID
	ClosedOperationID    *ProductOperationID
	Revision             int
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type ProductContractInput struct {
	HouseholdID           HouseholdID
	AccountID             AccountID
	HoldingID             HoldingID
	InstrumentID          InstrumentID
	Kind                  ProductKind
	Name                  string
	Note                  *string
	Currency              CurrencyCode
	Principal             Money
	StartOn               string
	MaturityOn            *string
	InterestMode          InterestMode
	AnnualRate            *AnnualRate
	MaturityInterest      *Money
	InterestPaidThroughOn *string
	RenewedFromID         *ProductContractID
	State                 ProductContractState
	OpenedOperationID     ProductOperationID
	ClosedOperationID     *ProductOperationID
	Revision              int
}

func NewProductContract(input ProductContractInput, now time.Time) (ProductContract, error) {
	contract := ProductContract{
		ID: NewProductContractID(), HouseholdID: input.HouseholdID, AccountID: input.AccountID,
		HoldingID: input.HoldingID, InstrumentID: input.InstrumentID, Kind: input.Kind,
		Name: input.Name, Note: input.Note, Currency: input.Currency, Principal: input.Principal,
		StartOn: input.StartOn, MaturityOn: input.MaturityOn, InterestMode: input.InterestMode,
		AnnualRate: input.AnnualRate, MaturityInterest: input.MaturityInterest,
		InterestPaidThroughOn: input.InterestPaidThroughOn, RenewedFromID: input.RenewedFromID,
		State: input.State, OpenedOperationID: input.OpenedOperationID, ClosedOperationID: input.ClosedOperationID,
		Revision: input.Revision, CreatedAt: normalizeTime(now), UpdatedAt: normalizeTime(now),
	}
	if contract.Revision == 0 {
		contract.Revision = 1
	}
	if contract.State == "" {
		contract.State = ProductStateOpen
	}
	if err := contract.Validate(); err != nil {
		return ProductContract{}, err
	}
	return contract, nil
}

func (c ProductContract) Validate() error {
	if _, err := ParseHouseholdID(c.HouseholdID.String()); err != nil {
		return validation("householdId", "must be a lowercase UUID")
	}
	if _, err := ParseAccountID(c.AccountID.String()); err != nil {
		return validation("accountId", "must be a lowercase UUID")
	}
	if _, err := ParseHoldingID(c.HoldingID.String()); err != nil {
		return validation("holdingId", "must be a lowercase UUID")
	}
	if _, err := ParseInstrumentID(c.InstrumentID.String()); err != nil {
		return validation("instrumentId", "must be a lowercase UUID")
	}
	kind, err := ParseProductKind(string(c.Kind))
	if err != nil {
		return err
	}
	if _, err := validateName("name", c.Name); err != nil {
		return err
	}
	if _, err := validateNote("note", c.Note); err != nil {
		return err
	}
	if c.Principal.IsZero() {
		return validation("principal", "must be greater than zero")
	}
	if c.Principal.Currency() != c.Currency {
		return validation("principal", "must use the contract currency")
	}
	startOn, err := ParseCivilDate("startOn", c.StartOn)
	if err != nil {
		return err
	}
	c.StartOn = startOn
	maturity, err := optionalCivilDate("maturityOn", c.MaturityOn)
	if err != nil {
		return err
	}
	if kind == ProductTermDeposit {
		if maturity == nil {
			return validation("maturityOn", "is required for a term deposit")
		}
		if compareCivilDates(startOn, *maturity) >= 0 {
			return validation("maturityOn", "must be after startOn")
		}
	} else if maturity != nil && compareCivilDates(startOn, *maturity) >= 0 {
		return validation("maturityOn", "must be after startOn")
	}
	mode, err := ParseInterestMode(string(c.InterestMode))
	if err != nil {
		return err
	}
	if err := validateInterestFields(kind, mode, c.AnnualRate, c.MaturityInterest, c.InterestPaidThroughOn, startOn, maturity, c.Currency); err != nil {
		return err
	}
	if _, err := ParseProductContractState(string(c.State)); err != nil {
		return err
	}
	if c.Revision < 1 {
		return validation("revision", "must be a positive integer")
	}
	if _, err := ParseProductOperationID(c.OpenedOperationID.String()); err != nil {
		return validation("openedOperationId", "must be a lowercase UUID")
	}
	if c.ClosedOperationID != nil {
		if _, err := ParseProductOperationID(c.ClosedOperationID.String()); err != nil {
			return validation("closedOperationId", "must be a lowercase UUID")
		}
	}
	if c.RenewedFromID != nil {
		if _, err := ParseProductContractID(c.RenewedFromID.String()); err != nil {
			return validation("renewedFromId", "must be a lowercase UUID")
		}
		if *c.RenewedFromID == c.ID {
			return validation("renewedFromId", "must not reference the same contract")
		}
	}
	if c.State == ProductStateOpen && c.ClosedOperationID != nil {
		return validation("closedOperationId", "must be empty while the contract is open")
	}
	if (c.State == ProductStateSettled || c.State == ProductStateCancelled) && c.ClosedOperationID == nil {
		return validation("closedOperationId", "is required after settlement or cancellation")
	}
	return nil
}

func validateInterestFields(kind ProductKind, mode InterestMode, rate *AnnualRate, maturityInterest *Money, paidThrough *string, startOn string, maturity *string, currency CurrencyCode) error {
	switch mode {
	case InterestNone:
		if rate != nil {
			return validation("annualRate", "must be empty when interest mode is none")
		}
		if maturityInterest != nil {
			return validation("maturityInterest", "must be empty when interest mode is none")
		}
		if paidThrough != nil {
			return validation("interestPaidThroughOn", "must be empty when interest mode is none")
		}
	case InterestManualMaturityAmount:
		if rate != nil {
			return validation("annualRate", "must be empty for a manual maturity-interest amount")
		}
		if maturityInterest == nil {
			return validation("maturityInterest", "is required for a manual maturity-interest amount")
		}
		if maturityInterest.Currency() != currency {
			return validation("maturityInterest", "must use the contract currency")
		}
		if paidThrough != nil {
			return validation("interestPaidThroughOn", "must be empty for a manual maturity-interest amount")
		}
	case InterestSimpleAct365, InterestSimpleAct360:
		if kind != ProductTermDeposit {
			return validation("interestMode", "simple interest is only supported for term deposits")
		}
		if rate == nil {
			return validation("annualRate", "is required for simple interest")
		}
		if maturityInterest != nil {
			return validation("maturityInterest", "must be empty for simple interest")
		}
		if paidThrough == nil {
			return validation("interestPaidThroughOn", "is required for simple interest")
		}
		paid, err := ParseCivilDate("interestPaidThroughOn", *paidThrough)
		if err != nil {
			return err
		}
		if maturity == nil {
			return validation("maturityOn", "is required for simple interest")
		}
		if compareCivilDates(paid, startOn) < 0 || compareCivilDates(paid, *maturity) > 0 {
			return validation("interestPaidThroughOn", "must fall on or between startOn and maturityOn")
		}
	}
	return nil
}

func optionalCivilDate(field string, value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := ParseCivilDate(field, *value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// EstimatedUnpaidMaturityInterest returns forecast unpaid interest. It never
// changes net worth, cost basis, or ledger facts.
func (c ProductContract) EstimatedUnpaidMaturityInterest() (*Money, error) {
	switch c.InterestMode {
	case InterestNone:
		zero, err := NewMoney(decimal.Zero, c.Currency)
		if err != nil {
			return nil, err
		}
		return &zero, nil
	case InterestManualMaturityAmount:
		if c.MaturityInterest == nil {
			return nil, validation("maturityInterest", "is required")
		}
		copy := *c.MaturityInterest
		return &copy, nil
	case InterestSimpleAct365, InterestSimpleAct360:
		if c.AnnualRate == nil || c.InterestPaidThroughOn == nil || c.MaturityOn == nil {
			return nil, validation("interestMode", "simple interest is missing required fields")
		}
		days, err := CivilDaysBetween(*c.InterestPaidThroughOn, *c.MaturityOn)
		if err != nil {
			return nil, err
		}
		basis := 365
		if c.InterestMode == InterestSimpleAct360 {
			basis = 360
		}
		interest, err := SimpleInterest(c.Principal, *c.AnnualRate, days, basis)
		if err != nil {
			return nil, err
		}
		return &interest, nil
	default:
		return nil, validation("interestMode", "is not supported")
	}
}

// SimpleInterest computes principal * rate * days / basis, rounding once to
// four decimal places half away from zero. Tests must not use the production
// evaluator as their expected-value source; Fixture D is computed by hand.
func SimpleInterest(principal Money, rate AnnualRate, days, basis int) (Money, error) {
	if days < 0 {
		return Money{}, validation("days", "must not be negative")
	}
	if basis != 360 && basis != 365 {
		return Money{}, validation("dayBasis", "must be 360 or 365")
	}
	if days == 0 || rate.value.IsZero() || principal.IsZero() {
		return NewMoney(decimal.Zero, principal.Currency())
	}
	raw := principal.Amount().Mul(rate.value).Mul(decimal.NewFromInt(int64(days))).Div(decimal.NewFromInt(int64(basis)))
	rounded := raw.Round(4)
	if rounded.IsNegative() || rounded.GreaterThan(maxMoney) {
		return Money{}, &Error{Code: ErrDecimalOverflow, Field: "interest", Message: "interest is outside the supported range"}
	}
	return ParseMoney(canonicalDecimal(rounded), principal.Currency())
}

func (c ProductContract) ForecastGrossProceeds() (*Money, error) {
	if c.Kind == ProductLockedProduct {
		return nil, nil
	}
	interest, err := c.EstimatedUnpaidMaturityInterest()
	if err != nil {
		return nil, err
	}
	if interest == nil {
		return nil, nil
	}
	total, err := c.Principal.Add(*interest)
	if err != nil {
		return nil, err
	}
	return &total, nil
}

func ProductAccountEligible(account Account) error {
	if account.ArchivedAt != nil {
		return &Error{Code: ErrConflict, Field: "accountId", Message: "account is archived"}
	}
	if account.IsLiability() {
		return validation("accountId", "products require an asset-side account")
	}
	if account.TrackingMode != TrackingHoldings {
		return validation("accountId", "product details require a Holdings Account")
	}
	if account.AccountType == TypeCashOnHand {
		return validation("accountId", "cash on hand cannot hold products")
	}
	return nil
}

func ManagedPositionConflict(field, message string) error {
	if message == "" {
		message = "this position is managed by a product contract"
	}
	return &Error{Code: ErrManagedPosition, Field: field, Message: message}
}
