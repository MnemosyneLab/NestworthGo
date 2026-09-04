package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type ScopeKind string

const (
	ScopeHousehold  ScopeKind = "household"
	ScopeAccount    ScopeKind = "account"
	ScopeCurrency   ScopeKind = "currency"
	ScopeInstrument ScopeKind = "instrument"
	ScopeAssetClass ScopeKind = "asset_class"
)

type Valuation string

const (
	ValuationBase   Valuation = "base"
	ValuationNative Valuation = "native"
)

type ReturnBasis string

const ReturnBasisInvestment ReturnBasis = "investment"

type AnalysisFilters struct {
	AccountID    *AccountID
	Currency     *CurrencyCode
	AssetClass   string
	InstrumentID *InstrumentID
	MemberID     *MemberID
}

type AnalysisScope struct {
	Kind ScopeKind
	ID   string
}
type AnalysisQuery struct {
	Scope       AnalysisScope
	From, To    LocalDate
	Valuation   Valuation
	Basis       ReturnBasis
	IncludeCash bool
	Filters     AnalysisFilters
}

func (s AnalysisScope) Validate() error {
	if !slices.Contains([]ScopeKind{ScopeHousehold, ScopeAccount, ScopeCurrency, ScopeInstrument, ScopeAssetClass}, s.Kind) {
		return validation("scope.kind", "is invalid")
	}
	if s.Kind == ScopeHousehold && s.ID != "" {
		return validation("scope.id", "must be empty for household scope")
	}
	if s.Kind != ScopeHousehold && s.ID == "" {
		return validation("scope.id", "is required")
	}
	if s.Kind == ScopeAccount {
		if _, err := ParseAccountID(s.ID); err != nil {
			return validation("scope.id", "must be a valid account ID")
		}
	}
	if s.Kind == ScopeInstrument {
		if _, err := ParseInstrumentID(s.ID); err != nil {
			return validation("scope.id", "must be a valid instrument ID")
		}
	}
	if s.Kind == ScopeCurrency {
		if _, err := ParseCurrency(s.ID); err != nil {
			return validation("scope.id", "must be a valid currency")
		}
	}
	return nil
}
func (q AnalysisQuery) Validate() error {
	if err := q.Scope.Validate(); err != nil {
		return err
	}
	if q.From == "" || q.To == "" || q.From > q.To {
		return validation("query.range", "must be a non-empty inclusive range")
	}
	for field, value := range map[string]LocalDate{"from": q.From, "to": q.To} {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil || parsed.Format("2006-01-02") != value {
			return validation("query."+field, "must use YYYY-MM-DD")
		}
	}
	if q.Valuation != ValuationBase && q.Valuation != ValuationNative {
		return validation("valuation", "is invalid")
	}
	if q.Basis != ReturnBasisInvestment {
		return validation("basis", "is invalid")
	}
	return nil
}

type ComponentID struct {
	AccountID    AccountID
	HoldingID    *HoldingID
	InstrumentID *InstrumentID
	Currency     CurrencyCode
	AssetClass   string
	Cash         bool
}
type Components []ComponentID

func (id ComponentID) Key() string {
	parts := []string{id.AccountID.String()}
	if id.HoldingID != nil {
		parts = append(parts, "holding:"+id.HoldingID.String())
	} else if id.Cash {
		parts = append(parts, "cash")
		if id.Currency != "" {
			parts = append(parts, "currency:"+id.Currency.String())
		}
	} else {
		parts = append(parts, "value")
	}
	if id.InstrumentID != nil {
		parts = append(parts, "instrument:"+id.InstrumentID.String())
	}
	return strings.Join(parts, "/")
}

type AnalysisUniverse struct {
	Accounts     []AccountID
	Instruments  []InstrumentID
	Currencies   []CurrencyCode
	AssetClasses []string
	Components
}
type InvestmentUniverse struct{ Components }
type ResolvedAnalysisContext struct {
	Universe            AnalysisUniverse
	InvestmentUniverse  InvestmentUniverse
	AnalysisDayTimezone string
}

type AttributionBucket string

const (
	BucketExternalFlow     AttributionBucket = "external_flow"
	BucketIncome           AttributionBucket = "income"
	BucketSpending         AttributionBucket = "spending"
	BucketDividendInterest AttributionBucket = "dividend_interest"
	BucketPriceChange      AttributionBucket = "price_change"
	BucketFXImpact         AttributionBucket = "fx_impact"
	BucketFee              AttributionBucket = "fee"
	BucketLiabilityImpact  AttributionBucket = "liability_impact"
	BucketAdjustment       AttributionBucket = "adjustment"
	BucketResidual         AttributionBucket = "residual"
)

type ReturnComponent string

const (
	ReturnPriceChange      ReturnComponent = "price_change"
	ReturnFXImpact         ReturnComponent = "fx_impact"
	ReturnDividendInterest ReturnComponent = "dividend_interest"
	ReturnInvestmentFee    ReturnComponent = "investment_fee"
)

type DietzCapitalFlow struct {
	Amount      SignedMoney
	EffectiveAt time.Time
}

// AttributedEffect keeps the three projections of one economic event
// orthogonal. A nil field means that the event does not contribute to that
// projection. PotentialDietzCapitalFlow preserves a capital leg that is
// internal to the current universe but may be external to a later group fold.
type AttributedEffect struct {
	AssetBucket               *AttributionBucket
	ReturnComponent           *ReturnComponent
	DietzCapitalFlow          *DietzCapitalFlow
	PotentialDietzCapitalFlow *DietzCapitalFlow
	Amount                    SignedMoney
	SourceEffect              ActivityEffect
	Component                 ComponentID
	RelatedHoldingID          *HoldingID
	RelatedInstrumentID       *InstrumentID
}

type Completeness string

const (
	CompletenessOK          Completeness = "ok"
	CompletenessPartial     Completeness = "partial"
	CompletenessUnavailable Completeness = "unavailable"
)

type Residual struct {
	Amount    SignedMoney
	Tolerance decimal.Decimal
	Visible   bool
}
type Tolerance struct {
	Amount         decimal.Decimal
	Currency       CurrencyCode
	BeginningValue Money
}

type ComponentDay struct {
	Date              LocalDate
	Component         ComponentID
	AssetBuckets      map[AttributionBucket]SignedMoney
	ReturnComponents  map[ReturnComponent]SignedMoney
	DietzFlow         SignedMoney
	DietzCapitalFlows []DietzCapitalFlow
	AttributedEffects []AttributedEffect
	BeginningValue    SignedMoney
	EndingValue       SignedMoney
	ReturnAmount      *SignedMoney
	InvestedCapital   *SignedMoney
	ReturnRate        *decimal.Decimal
	Status            Completeness
	Residual          *Residual
}
type DailyReturn struct {
	Date            LocalDate
	Amount          *SignedMoney
	InvestedCapital *SignedMoney
	Rate            *decimal.Decimal
	Status          Completeness
}
type RateCoverage struct{ RatedDays, TotalDays int }
type PeriodAnalysisResult struct {
	Query               AnalysisQuery
	AnalysisDayTimezone string
	Days                []ComponentDay
	DailyReturns        []DailyReturn
	Coverage            RateCoverage
	ReturnAmount        *SignedMoney
	// InvestedCapital is the InvestmentUniverse beginning on Query.From. It is
	// not the Dietz denominator and not the sum of daily beginning values.
	InvestedCapital *SignedMoney
	ReturnRate      *decimal.Decimal
	Status          Completeness
}

// Validate is deliberately structural: arithmetic and currency consistency belong to the engine.
func (d ComponentDay) Validate() error {
	if d.Date == "" {
		return validation("date", "is required")
	}
	if d.Status != CompletenessOK && d.Status != CompletenessPartial && d.Status != CompletenessUnavailable {
		return validation("status", "is invalid")
	}
	return nil
}
func (p PeriodAnalysisResult) Validate() error {
	if err := p.Query.Validate(); err != nil {
		return err
	}
	if p.Coverage.RatedDays < 0 || p.Coverage.TotalDays < 0 || p.Coverage.RatedDays > p.Coverage.TotalDays {
		return validation("coverage", "has invalid day counts")
	}
	for _, d := range p.Days {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func ParseScopeKind(v string) (ScopeKind, error) {
	k := ScopeKind(v)
	if !slices.Contains([]ScopeKind{ScopeHousehold, ScopeAccount, ScopeCurrency, ScopeInstrument, ScopeAssetClass}, k) {
		return "", fmt.Errorf("invalid scope kind: %s", v)
	}
	return k, nil
}
func ParseValuation(v string) (Valuation, error) {
	x := Valuation(v)
	if x != ValuationBase && x != ValuationNative {
		return "", fmt.Errorf("invalid valuation: %s", v)
	}
	return x, nil
}
func ParseReturnBasis(v string) (ReturnBasis, error) {
	if ReturnBasis(v) != ReturnBasisInvestment {
		return "", fmt.Errorf("invalid return basis: %s", v)
	}
	return ReturnBasisInvestment, nil
}
