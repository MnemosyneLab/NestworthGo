package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type LiquiditySourceKind string

const (
	SourceAccountValue LiquiditySourceKind = "account_value"
	SourceAccountCash  LiquiditySourceKind = "account_cash"
	SourceHolding      LiquiditySourceKind = "holding"
)

func ParseLiquiditySourceKind(value string) (LiquiditySourceKind, error) {
	kind := LiquiditySourceKind(strings.TrimSpace(value))
	switch kind {
	case SourceAccountValue, SourceAccountCash, SourceHolding:
		return kind, nil
	default:
		return "", validation("sourceKind", "must be account_value, account_cash, or holding")
	}
}

// LiquiditySourceRef is the typed identity of one availability component.
// Keys are derived in Go; callers must not concatenate frontend strings for SQL.
type LiquiditySourceRef struct {
	Kind      LiquiditySourceKind
	AccountID AccountID
	HoldingID *HoldingID
	Currency  *CurrencyCode
}

func (r LiquiditySourceRef) Key() string {
	switch r.Kind {
	case SourceAccountValue:
		return "account_value:" + r.AccountID.String()
	case SourceAccountCash:
		currency := ""
		if r.Currency != nil {
			currency = r.Currency.String()
		}
		return "account_cash:" + r.AccountID.String() + ":" + currency
	case SourceHolding:
		holding := ""
		if r.HoldingID != nil {
			holding = r.HoldingID.String()
		}
		return "holding:" + holding
	default:
		return string(r.Kind)
	}
}

func (r LiquiditySourceRef) Validate() error {
	kind, err := ParseLiquiditySourceKind(string(r.Kind))
	if err != nil {
		return err
	}
	if _, err := ParseAccountID(r.AccountID.String()); err != nil {
		return validation("accountId", "must be a lowercase UUID")
	}
	switch kind {
	case SourceAccountValue:
		if r.HoldingID != nil {
			return validation("holdingId", "must be empty for an account_value source")
		}
		if r.Currency != nil {
			return validation("currency", "must be empty for an account_value source")
		}
	case SourceAccountCash:
		if r.HoldingID != nil {
			return validation("holdingId", "must be empty for an account_cash source")
		}
		if r.Currency == nil {
			return validation("currency", "is required for an account_cash source")
		}
		if _, err := ParseCurrency(r.Currency.String()); err != nil {
			return err
		}
	case SourceHolding:
		if r.HoldingID == nil {
			return validation("holdingId", "is required for a holding source")
		}
		if _, err := ParseHoldingID(r.HoldingID.String()); err != nil {
			return err
		}
	}
	return nil
}

func AccountValueSourceRef(accountID AccountID) LiquiditySourceRef {
	return LiquiditySourceRef{Kind: SourceAccountValue, AccountID: accountID}
}

func AccountCashSourceRef(accountID AccountID, currency CurrencyCode) LiquiditySourceRef {
	code := currency
	return LiquiditySourceRef{Kind: SourceAccountCash, AccountID: accountID, Currency: &code}
}

func HoldingSourceRef(accountID AccountID, holdingID HoldingID) LiquiditySourceRef {
	id := holdingID
	return LiquiditySourceRef{Kind: SourceHolding, AccountID: accountID, HoldingID: &id}
}

type AccessKind string

const (
	AccessOnRequest AccessKind = "on_request"
	AccessOnDate    AccessKind = "on_date"
	AccessUnknown   AccessKind = "unknown"
	AccessExcluded  AccessKind = "excluded"
)

func ParseAccessKind(value string) (AccessKind, error) {
	kind := AccessKind(strings.TrimSpace(value))
	switch kind {
	case AccessOnRequest, AccessOnDate, AccessUnknown, AccessExcluded:
		return kind, nil
	default:
		return "", validation("accessKind", "is not supported")
	}
}

type DayBasis string

const (
	DayBasisCalendar DayBasis = "calendar"
	DayBasisWeekdays DayBasis = "weekdays"
)

func ParseDayBasis(value string) (DayBasis, error) {
	basis := DayBasis(strings.TrimSpace(value))
	switch basis {
	case DayBasisCalendar, DayBasisWeekdays:
		return basis, nil
	default:
		return "", validation("dayBasis", "must be calendar or weekdays")
	}
}

type EarlyKind string

const (
	EarlyNotAllowed EarlyKind = "not_allowed"
	EarlyAllowed    EarlyKind = "allowed"
	EarlyUnknown    EarlyKind = "unknown"
)

func ParseEarlyKind(value string) (EarlyKind, error) {
	kind := EarlyKind(strings.TrimSpace(value))
	switch kind {
	case EarlyNotAllowed, EarlyAllowed, EarlyUnknown:
		return kind, nil
	default:
		return "", validation("earlyKind", "is not supported")
	}
}

type EarlyAmountMode string

const (
	EarlyAmountCurrentValue EarlyAmountMode = "current_value"
	EarlyAmountFixedGross   EarlyAmountMode = "fixed_gross"
)

func ParseEarlyAmountMode(value string) (EarlyAmountMode, error) {
	mode := EarlyAmountMode(strings.TrimSpace(value))
	switch mode {
	case EarlyAmountCurrentValue, EarlyAmountFixedGross:
		return mode, nil
	default:
		return "", validation("earlyAmountMode", "is not supported")
	}
}

type PolicyOrigin string

const (
	PolicyOriginAssumed  PolicyOrigin = "assumed"
	PolicyOriginExplicit PolicyOrigin = "explicit"
	PolicyOriginContract PolicyOrigin = "contract"
)

type CompletenessStatus string

const (
	StatusComplete    CompletenessStatus = "complete"
	StatusPartial     CompletenessStatus = "partial"
	StatusUnavailable CompletenessStatus = "unavailable"
)

type RouteKind string

const (
	RouteNormal RouteKind = "normal"
	RouteEarly  RouteKind = "early"
)

type LiquidityPolicy struct {
	ID                  LiquidityPolicyID
	HouseholdID         HouseholdID
	Source              LiquiditySourceRef
	AccessKind          AccessKind
	UnlockOn            *string
	SettlementDays      *int
	DayBasis            *DayBasis
	ReceiptOnOverride   *string
	AccessibleAmountCap *Money
	NormalExitFee       *Money
	EarlyKind           EarlyKind
	EarlySettlementDays *int
	EarlyDayBasis       *DayBasis
	EarlyFee            *Money
	EarlyAmountMode     *EarlyAmountMode
	EarlyGrossAmount    *Money
	ConfirmedAt         *time.Time
	Note                *string
	Revision            int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (p LiquidityPolicy) Validate(managed bool) error {
	if _, err := ParseHouseholdID(p.HouseholdID.String()); err != nil {
		return validation("householdId", "must be a lowercase UUID")
	}
	if err := p.Source.Validate(); err != nil {
		return err
	}
	access, err := ParseAccessKind(string(p.AccessKind))
	if err != nil {
		return err
	}
	if access == AccessOnDate {
		if p.UnlockOn == nil {
			return validation("unlockOn", "is required for on_date access")
		}
		if _, err := ParseCivilDate("unlockOn", *p.UnlockOn); err != nil {
			return err
		}
	}
	if p.UnlockOn != nil && access != AccessOnDate && access != AccessOnRequest {
		if _, err := ParseCivilDate("unlockOn", *p.UnlockOn); err != nil {
			return err
		}
	}
	if p.SettlementDays != nil {
		if *p.SettlementDays < 0 || *p.SettlementDays > 365 {
			return validation("settlementDays", "must be between 0 and 365")
		}
		if p.DayBasis == nil {
			return validation("dayBasis", "is required when settlementDays is set")
		}
		if _, err := ParseDayBasis(string(*p.DayBasis)); err != nil {
			return err
		}
	}
	if p.ReceiptOnOverride != nil {
		if _, err := ParseCivilDate("receiptOnOverride", *p.ReceiptOnOverride); err != nil {
			return err
		}
	}
	if managed && p.AccessibleAmountCap != nil {
		return validation("accessibleAmountCap", "is not accepted for a managed product")
	}
	if p.AccessibleAmountCap != nil && p.Source.Currency != nil && p.AccessibleAmountCap.Currency() != *p.Source.Currency {
		return validation("accessibleAmountCap", "must use the source currency")
	}
	if p.NormalExitFee != nil && p.Source.Currency != nil && p.NormalExitFee.Currency() != *p.Source.Currency {
		return validation("normalExitFee", "must use the source currency")
	}
	early, err := ParseEarlyKind(string(p.EarlyKind))
	if err != nil {
		return err
	}
	if early == EarlyAllowed {
		if p.EarlySettlementDays == nil || p.EarlyDayBasis == nil || p.EarlyFee == nil || p.EarlyAmountMode == nil {
			return validation("earlyKind", "allowed early access requires settlement, fee, and amount fields")
		}
		if *p.EarlySettlementDays < 0 || *p.EarlySettlementDays > 365 {
			return validation("earlySettlementDays", "must be between 0 and 365")
		}
		if _, err := ParseDayBasis(string(*p.EarlyDayBasis)); err != nil {
			return err
		}
		if _, err := ParseEarlyAmountMode(string(*p.EarlyAmountMode)); err != nil {
			return err
		}
		if *p.EarlyAmountMode == EarlyAmountFixedGross && p.EarlyGrossAmount == nil {
			return validation("earlyGrossAmount", "is required for a fixed-gross early amount")
		}
	}
	if p.Revision < 1 {
		return validation("revision", "must be a positive integer")
	}
	if _, err := validateNote("note", p.Note); err != nil {
		return err
	}
	return nil
}

type LiquidityReservation struct {
	ID          LiquidityReservationID
	HouseholdID HouseholdID
	Source      LiquiditySourceRef
	Label       string
	Amount      Money
	Currency    CurrencyCode
	Revision    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ReleasedAt  *time.Time
}

func (r LiquidityReservation) Validate() error {
	if _, err := ParseHouseholdID(r.HouseholdID.String()); err != nil {
		return validation("householdId", "must be a lowercase UUID")
	}
	if err := r.Source.Validate(); err != nil {
		return err
	}
	if _, err := validateName("label", r.Label); err != nil {
		return err
	}
	if r.Amount.IsZero() {
		return validation("amount", "must be greater than zero")
	}
	if r.Amount.Currency() != r.Currency {
		return validation("currency", "must match the reservation amount")
	}
	if r.Source.Currency != nil && r.Currency != *r.Source.Currency {
		return validation("currency", "must match the source currency")
	}
	if r.Revision < 1 {
		return validation("revision", "must be a positive integer")
	}
	return nil
}

func (r LiquidityReservation) Active() bool { return r.ReleasedAt == nil }

type LiquidityAssumption string

const (
	AssumptionBankCashToday          LiquidityAssumption = "assumed_bank_cash_today"
	AssumptionWeekdaysNoHolidays     LiquidityAssumption = "weekdays_no_holidays"
	AssumptionCurrentPricesAndFX     LiquidityAssumption = "current_prices_and_fx"
	AssumptionForecastInterest       LiquidityAssumption = "future_interest_is_forecast"
	AssumptionExcludedShortTerm      LiquidityAssumption = "excluded_from_short_term"
	AssumptionDueUnconfirmed         LiquidityAssumption = "due_unconfirmed"
	AssumptionFeeExceedsGross        LiquidityAssumption = "fee_exceeds_gross"
	AssumptionMissingFX              LiquidityAssumption = "missing_fx"
	AssumptionMissingPrice           LiquidityAssumption = "missing_price"
	AssumptionUnknownAccess          LiquidityAssumption = "unknown_access"
	AssumptionUnknownFee             LiquidityAssumption = "unknown_fee"
	AssumptionUnknownTiming          LiquidityAssumption = "unknown_timing"
	AssumptionUnknownEarly           LiquidityAssumption = "unknown_early_access"
	AssumptionRestrictedAccount      LiquidityAssumption = "account_restrictions_may_apply"
	AssumptionOutstandingDebtOmitted LiquidityAssumption = "outstanding_debt_not_subtracted"
	AssumptionAssetsOnly             LiquidityAssumption = "based_on_current_assets"
)

func DefaultLiquidityPolicy(accountType AccountType, tracking TrackingMode, sourceKind LiquiditySourceKind, managed bool) (LiquidityPolicy, PolicyOrigin, []LiquidityAssumption) {
	policy := LiquidityPolicy{AccessKind: AccessUnknown, EarlyKind: EarlyNotAllowed, Revision: 1}
	if managed {
		policy.AccessKind = AccessUnknown
		return policy, PolicyOriginContract, []LiquidityAssumption{AssumptionUnknownAccess}
	}
	zero := 0
	calendar := DayBasisCalendar
	switch {
	case accountType == TypeCashOnHand && sourceKind != SourceHolding:
		policy.AccessKind = AccessOnRequest
		policy.SettlementDays = &zero
		policy.DayBasis = &calendar
		policy.EarlyKind = EarlyNotAllowed
		return policy, PolicyOriginAssumed, nil
	case accountType == TypeBankAccount && (sourceKind == SourceAccountValue || sourceKind == SourceAccountCash):
		policy.AccessKind = AccessOnRequest
		policy.SettlementDays = &zero
		policy.DayBasis = &calendar
		policy.EarlyKind = EarlyNotAllowed
		return policy, PolicyOriginAssumed, []LiquidityAssumption{AssumptionBankCashToday}
	case accountType == TypeProperty || accountType == TypeVehicle || accountType == TypeCollectible:
		policy.AccessKind = AccessExcluded
		policy.EarlyKind = EarlyNotAllowed
		return policy, PolicyOriginAssumed, []LiquidityAssumption{AssumptionExcludedShortTerm}
	default:
		policy.AccessKind = AccessUnknown
		policy.EarlyKind = EarlyUnknown
		assumptions := []LiquidityAssumption{AssumptionUnknownAccess}
		if restrictedAccountType(accountType) {
			assumptions = append(assumptions, AssumptionRestrictedAccount)
		}
		return policy, PolicyOriginAssumed, assumptions
	}
}

func restrictedAccountType(accountType AccountType) bool {
	switch accountType {
	case TypeBrokerage, TypeInvestmentAccount, TypeCryptoExchange, TypeDigitalWallet, TypePension, TypeInsurancePolicy, TypeOther:
		return true
	default:
		return false
	}
}

func ResolveLiquidityPolicy(source LiquiditySource) (LiquidityPolicy, PolicyOrigin, []LiquidityAssumption, error) {
	if source.Managed {
		if source.ExplicitPolicy == nil {
			return LiquidityPolicy{}, PolicyOriginContract, []LiquidityAssumption{AssumptionUnknownAccess}, validation("policy", "a managed product requires an explicit contract policy")
		}
		policy := *source.ExplicitPolicy
		if err := policy.Validate(true); err != nil {
			return LiquidityPolicy{}, PolicyOriginContract, nil, err
		}
		return policy, PolicyOriginContract, nil, nil
	}
	if source.ExplicitPolicy != nil {
		policy := *source.ExplicitPolicy
		if err := policy.Validate(false); err != nil {
			return LiquidityPolicy{}, PolicyOriginExplicit, nil, err
		}
		assumptions := []LiquidityAssumption{AssumptionRestrictedAccount}
		return policy, PolicyOriginExplicit, assumptions, nil
	}
	policy, origin, assumptions := DefaultLiquidityPolicy(source.AccountType, source.TrackingMode, source.Ref.Kind, false)
	if restrictedAccountType(source.AccountType) && source.Ref.Kind == SourceHolding {
		policy.AccessKind = AccessUnknown
		origin = PolicyOriginAssumed
		assumptions = []LiquidityAssumption{AssumptionUnknownAccess, AssumptionRestrictedAccount}
	}
	attachAssumedZeroExitFee(&policy, origin, source.NativeCurrency)
	return policy, origin, assumptions, nil
}

// attachAssumedZeroExitFee is part of the assumed bank-cash / cash-on-hand
// policy, not a silent substitution for a missing user-entered fee. Explicit
// policies keep a nil fee as unknown.
func attachAssumedZeroExitFee(policy *LiquidityPolicy, origin PolicyOrigin, currency CurrencyCode) {
	if policy == nil || origin != PolicyOriginAssumed || policy.NormalExitFee != nil {
		return
	}
	if policy.AccessKind != AccessOnRequest || currency == "" {
		return
	}
	zero, err := NewMoney(decimal.Zero, currency)
	if err != nil {
		return
	}
	policy.NormalExitFee = &zero
}

func (p LiquidityPolicy) actionEligibleOn(today string, early bool, contract *ProductContract) (string, []LiquidityAssumption, error) {
	if early {
		eligible := today
		if contract != nil && contract.Kind == ProductLockedProduct && p.UnlockOn != nil {
			unlock, err := ParseCivilDate("unlockOn", *p.UnlockOn)
			if err != nil {
				return "", nil, err
			}
			if compareCivilDates(eligible, unlock) < 0 {
				eligible = unlock
			}
		}
		return eligible, nil, nil
	}
	if contract != nil && contract.MaturityOn != nil {
		maturity, err := ParseCivilDate("maturityOn", *contract.MaturityOn)
		if err != nil {
			return "", nil, err
		}
		if compareCivilDates(today, maturity) > 0 {
			return today, nil, nil
		}
		return maturity, nil, nil
	}
	switch p.AccessKind {
	case AccessOnRequest:
		eligible := today
		if p.UnlockOn != nil {
			unlock, err := ParseCivilDate("unlockOn", *p.UnlockOn)
			if err != nil {
				return "", nil, err
			}
			if compareCivilDates(eligible, unlock) < 0 {
				eligible = unlock
			}
		}
		return eligible, nil, nil
	case AccessOnDate:
		if p.UnlockOn == nil {
			return "", []LiquidityAssumption{AssumptionUnknownTiming}, validation("unlockOn", "is required")
		}
		unlock, err := ParseCivilDate("unlockOn", *p.UnlockOn)
		if err != nil {
			return "", nil, err
		}
		if compareCivilDates(today, unlock) > 0 {
			return today, nil, nil
		}
		return unlock, nil, nil
	default:
		return "", []LiquidityAssumption{AssumptionUnknownAccess}, fmt.Errorf("access is not scheduled")
	}
}

// ValidateProductDates checks the joint contract/policy invariants at write and restore boundaries.
func (p LiquidityPolicy) ValidateProductDates(kind ProductKind, maturity *string) error {
	if kind == ProductTermDeposit && (maturity == nil || p.UnlockOn == nil || *p.UnlockOn != *maturity) {
		return validation("unlockOn", "must equal deposit maturity")
	}
	if maturity != nil && p.UnlockOn != nil {
		if *p.UnlockOn > *maturity {
			return validation("unlockOn", "cannot follow maturity")
		}
	}
	if p.ReceiptOnOverride != nil {
		if maturity != nil && *p.ReceiptOnOverride < *maturity {
			return validation("receiptOnOverride", "cannot precede maturity")
		}
		if p.UnlockOn != nil && *p.ReceiptOnOverride < *p.UnlockOn {
			return validation("receiptOnOverride", "cannot precede unlock")
		}
	}
	return nil
}

func (p LiquidityPolicy) receiptOn(today string, early bool, contract *ProductContract) (*string, []LiquidityAssumption, error) {
	// A maturity is a scheduled receipt, not a fresh redemption request each day.
	if !early && contract != nil && contract.MaturityOn != nil {
		today = *contract.MaturityOn
	}
	if !early && p.ReceiptOnOverride != nil {
		override, err := ParseCivilDate("receiptOnOverride", *p.ReceiptOnOverride)
		if err != nil {
			return nil, nil, err
		}
		// An exact scheduled receipt stays fixed after its date passes. Validate
		// against contractual eligibility, not a new request made today.
		anchor := override
		if contract != nil && contract.MaturityOn != nil {
			anchor = *contract.MaturityOn
		}
		eligible, _, err := p.actionEligibleOn(anchor, false, contract)
		if err == nil && compareCivilDates(override, eligible) < 0 {
			return nil, nil, validation("receiptOnOverride", "cannot precede action eligibility")
		}
		return &override, nil, nil
	}
	eligible, assumptions, err := p.actionEligibleOn(today, early, contract)
	if err != nil {
		return nil, assumptions, err
	}
	days := 0
	basis := DayBasisCalendar
	if early {
		if p.EarlySettlementDays == nil || p.EarlyDayBasis == nil {
			return nil, []LiquidityAssumption{AssumptionUnknownTiming}, validation("earlySettlementDays", "is required")
		}
		days = *p.EarlySettlementDays
		basis = *p.EarlyDayBasis
	} else if p.SettlementDays == nil {
		return nil, []LiquidityAssumption{AssumptionUnknownTiming}, validation("settlementDays", "is unknown")
	} else {
		days = *p.SettlementDays
		if p.DayBasis != nil {
			basis = *p.DayBasis
		}
	}
	receipt, err := SettlementReceiptOn(eligible, days, basis)
	if err != nil {
		return nil, nil, err
	}
	assumptions = nil
	if basis == DayBasisWeekdays {
		assumptions = []LiquidityAssumption{AssumptionWeekdaysNoHolidays}
	}
	return &receipt, assumptions, nil
}
