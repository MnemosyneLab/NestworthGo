package domain

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// LiquiditySource is one current-valuation component fed into availability.
type LiquiditySource struct {
	Ref                 LiquiditySourceRef
	AccountID           AccountID
	ProductID           *ProductContractID
	DisplayName         string
	NativeCurrency      CurrencyCode
	CurrentNativeValue  *Money
	CurrentNativeAmount string // Exact valuation; CurrentNativeValue is its display projection.
	ValueAsOf           *time.Time
	PriceEvidence       *QuoteEvidenceView
	FXEvidence          *QuoteEvidenceView
	IncludeInNetWorth   bool
	Archived            bool
	Liability           bool
	QuantityZero        bool
	AccountType         AccountType
	TrackingMode        TrackingMode
	InstrumentType      *InstrumentType
	Managed             bool
	Contract            *ProductContract
	ExplicitPolicy      *LiquidityPolicy
}

type LiquidityQuery struct {
	AsOf                   time.Time
	Timezone               string
	BaseCurrency           CurrencyCode
	CustomHorizonOn        *string
	IncludeEarlyWithdrawal bool
}

// ConvertToBase converts a native amount using the same eligible current FX
// path as valuation. Conversion must be linear under a fixed FX snapshot.
// A nil result means the FX observation is missing.
type ConvertToBase func(amount Money) (*decimal.Decimal, bool, error)

type LiquidityRoute struct {
	netExact       *decimal.Decimal
	Kind           RouteKind
	EligibleOn     *string
	ReceiptOn      *string
	GrossNative    *Money
	FeeNative      *Money
	NetNative      *Money
	Status         CompletenessStatus
	AmountBasis    string
	ActionRequired string
	Assumptions    []LiquidityAssumption
	MissingReasons []string
}

type LiquidityBucketResult struct {
	netExact               *decimal.Decimal
	reserveExact           *decimal.Decimal
	nativeStatus           CompletenessStatus
	HorizonOn              string
	SelectedRoute          *LiquidityRoute
	NetNative              *Money
	NetBase                *decimal.Decimal
	AppliedReserveNative   *Money
	UnreservedNative       *Money
	ReserveShortfallNative *Money
	Status                 CompletenessStatus
	Reasons                []string
}

type LiquiditySourceResult struct {
	Ref                  LiquiditySourceRef
	SourceKey            string
	AccountID            AccountID
	ProductID            *ProductContractID
	DisplayName          string
	NativeCurrency       CurrencyCode
	CurrentNativeValue   *Money
	ValueAsOf            *time.Time
	PriceEvidence        *QuoteEvidenceView
	FXEvidence           *QuoteEvidenceView
	PolicyOrigin         PolicyOrigin
	Policy               *LiquidityPolicy
	ContractState        *ProductContractState
	DisplayState         ProductDisplayState
	ReservationRequested Money
	NormalRoute          *LiquidityRoute
	EarlyRoute           *LiquidityRoute
	BucketResults        []LiquidityBucketResult
	Reasons              []string
	Assumptions          []LiquidityAssumption
	Excluded             bool
	DueUnconfirmed       bool
}

type NativeCurrencyGroup struct {
	Currency                CurrencyCode
	Status                  CompletenessStatus
	FullAvailable           *Money
	KnownAvailableSubtotal  *Money
	AppliedReserveSubtotal  *Money
	FullUnreserved          *Money
	KnownUnreservedSubtotal *Money
}

type LiquidityBucket struct {
	HorizonOn               string
	Status                  CompletenessStatus
	FullAvailable           *Money
	KnownAvailableSubtotal  *Money
	AppliedReserveSubtotal  *Money
	FullUnreserved          *Money
	KnownUnreservedSubtotal *Money
	UnknownSourceCount      int
	ExcludedSourceCount     int
	EstimatedSourceCount    int
	NativeCurrencyGroups    []NativeCurrencyGroup
	Warnings                []string
}

type UnresolvedReservation struct {
	Reservation LiquidityReservation
	Reason      string
}

type LiquidityOverview struct {
	AsOf                   time.Time
	LocalDate              string
	Timezone               string
	BaseCurrency           CurrencyCode
	Assumptions            []LiquidityAssumption
	Buckets                []LiquidityBucket
	Sources                []LiquiditySourceResult
	UnresolvedReservations []UnresolvedReservation
}

func EvaluateLiquidity(query LiquidityQuery, sources []LiquiditySource, reservations []LiquidityReservation, convert ConvertToBase) (LiquidityOverview, error) {
	localDate, timezone, err := LocalCivilDate(query.AsOf, query.Timezone)
	if err != nil {
		return LiquidityOverview{}, err
	}
	horizons, err := LiquidityHorizons(localDate, query.CustomHorizonOn)
	if err != nil {
		return LiquidityOverview{}, err
	}
	overview := LiquidityOverview{
		AsOf:         query.AsOf,
		LocalDate:    localDate,
		Timezone:     timezone,
		BaseCurrency: query.BaseCurrency,
		Assumptions:  []LiquidityAssumption{AssumptionCurrentPricesAndFX, AssumptionAssetsOnly, AssumptionOutstandingDebtOmitted},
	}
	for _, source := range sources {
		if source.CurrentNativeAmount != "" {
			if _, err := ParseNativeAmount(source.CurrentNativeAmount); err != nil {
				return LiquidityOverview{}, err
			}
		}
	}
	activeBySource := map[string]Money{}
	for _, reservation := range reservations {
		if err := reservation.Validate(); err != nil {
			return LiquidityOverview{}, err
		}
		if !reservation.Active() {
			continue
		}
		key := reservation.Source.Key()
		found := false
		for _, source := range sources {
			if source.Ref.Key() == key && !source.Archived && !source.Liability && !source.QuantityZero && liquiditySourcePositiveOrUnknown(source) {
				found = true
				break
			}
		}
		if !found {
			overview.UnresolvedReservations = append(overview.UnresolvedReservations, UnresolvedReservation{Reservation: reservation, Reason: "source is missing, archived, or no longer positive"})
			continue
		}
		current, ok := activeBySource[key]
		if !ok {
			activeBySource[key] = reservation.Amount
			continue
		}
		sum, err := current.Add(reservation.Amount)
		if err != nil {
			return LiquidityOverview{}, err
		}
		activeBySource[key] = sum
	}
	results := make([]LiquiditySourceResult, 0, len(sources))
	for _, source := range sources {
		if err := source.Ref.Validate(); err != nil {
			return LiquidityOverview{}, err
		}
		result, err := evaluateSource(query, localDate, horizons, source, activeBySource[source.Ref.Key()], convert)
		if err != nil {
			return LiquidityOverview{}, err
		}
		results = append(results, result)
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].DisplayName != results[j].DisplayName {
			return results[i].DisplayName < results[j].DisplayName
		}
		return results[i].SourceKey < results[j].SourceKey
	})
	overview.Sources = results
	overview.Buckets = make([]LiquidityBucket, 0, len(horizons))
	for _, horizon := range horizons {
		bucket, err := aggregateBucket(horizon, query.BaseCurrency, results, convert)
		if err != nil {
			return LiquidityOverview{}, err
		}
		overview.Buckets = append(overview.Buckets, bucket)
	}
	if err := assertCumulativeMonotonic(overview.Buckets); err != nil {
		return LiquidityOverview{}, err
	}
	return overview, nil
}

func evaluateSource(query LiquidityQuery, today string, horizons []string, source LiquiditySource, requestedReserve Money, convert ConvertToBase) (LiquiditySourceResult, error) {
	result := LiquiditySourceResult{
		Ref:                source.Ref,
		SourceKey:          source.Ref.Key(),
		AccountID:          source.AccountID,
		ProductID:          source.ProductID,
		DisplayName:        source.DisplayName,
		NativeCurrency:     source.NativeCurrency,
		CurrentNativeValue: source.CurrentNativeValue,
		ValueAsOf:          source.ValueAsOf,
		PriceEvidence:      source.PriceEvidence,
		FXEvidence:         source.FXEvidence,
	}
	if requestedReserve.Currency() == "" && source.NativeCurrency != "" {
		zero, err := NewMoney(decimal.Zero, source.NativeCurrency)
		if err != nil {
			return LiquiditySourceResult{}, err
		}
		result.ReservationRequested = zero
	} else {
		result.ReservationRequested = requestedReserve
	}
	if source.Archived || source.Liability || source.QuantityZero {
		result.Excluded = true
		result.Reasons = append(result.Reasons, "source is archived, a liability, or has zero quantity")
		result.BucketResults = emptyBucketResults(horizons)
		return result, nil
	}
	policy, origin, assumptions, err := ResolveLiquidityPolicy(source)
	if err != nil && origin == PolicyOriginContract && source.Managed {
		result.PolicyOrigin = origin
		result.Assumptions = assumptions
		result.Reasons = append(result.Reasons, "managed product policy is missing")
		result.BucketResults = unknownBucketResults(horizons)
		return result, nil
	}
	if err != nil {
		return LiquiditySourceResult{}, err
	}
	result.PolicyOrigin = origin
	result.Policy = &policy
	result.Assumptions = append(result.Assumptions, assumptions...)
	if source.Contract != nil {
		state := source.Contract.State
		result.ContractState = &state
	}
	if policy.AccessKind == AccessExcluded {
		result.Excluded = true
		result.DisplayState = ProductDisplayLocked
		result.Reasons = append(result.Reasons, "explicitly excluded from short-term access")
		result.Assumptions = appendUniqueAssumption(result.Assumptions, AssumptionExcludedShortTerm)
		result.BucketResults = excludedBucketResults(horizons)
		return result, nil
	}
	normal, err := buildNormalRoute(today, source, policy)
	if err != nil {
		return LiquiditySourceResult{}, err
	}
	result.NormalRoute = normal
	if query.IncludeEarlyWithdrawal {
		early, err := buildEarlyRoute(today, source, policy)
		if err != nil {
			return LiquiditySourceResult{}, err
		}
		result.EarlyRoute = early
	}
	due := productReceiptUnconfirmed(source.Contract, policy, today, normal)
	result.DueUnconfirmed = due
	result.DisplayState = deriveDisplayState(source, policy, today, due, normal)
	if due {
		result.Assumptions = appendUniqueAssumption(result.Assumptions, AssumptionDueUnconfirmed)
		result.Reasons = append(result.Reasons, "due — receipt unconfirmed")
	}
	result.BucketResults = make([]LiquidityBucketResult, 0, len(horizons))
	for _, horizon := range horizons {
		bucket := selectRouteForHorizon(horizon, result, due, requestedReserve, convert)
		result.BucketResults = append(result.BucketResults, bucket)
	}
	return result, nil
}

// ProductAvailabilityState shares the overview's timing rules with product detail.
func ProductAvailabilityState(contract ProductContract, policy LiquidityPolicy, today string) (ProductDisplayState, error) {
	source := LiquiditySource{Managed: true, Contract: &contract}
	normal, err := buildNormalRoute(today, source, policy)
	if err != nil {
		return "", err
	}
	return deriveDisplayState(source, policy, today, productReceiptUnconfirmed(&contract, policy, today, normal), normal), nil
}

func productReceiptUnconfirmed(contract *ProductContract, policy LiquidityPolicy, today string, normal *LiquidityRoute) bool {
	return contract != nil && contract.State == ProductStateOpen &&
		(contract.MaturityOn != nil || policy.ReceiptOnOverride != nil) &&
		policy.AccessKind != AccessUnknown && policy.AccessKind != AccessExcluded &&
		normal != nil && normal.ReceiptOn != nil && compareCivilDates(*normal.ReceiptOn, today) <= 0
}

func deriveDisplayState(source LiquiditySource, policy LiquidityPolicy, today string, due bool, normal *LiquidityRoute) ProductDisplayState {
	if source.Contract != nil {
		switch source.Contract.State {
		case ProductStateSettled:
			return ProductDisplaySettled
		case ProductStateCancelled:
			return ProductDisplayCancelled
		}
	}
	if due {
		return ProductDisplayDueUnconfirmed
	}
	if policy.AccessKind == AccessExcluded {
		return ProductDisplayLocked
	}
	if normal != nil && normal.EligibleOn != nil && compareCivilDates(today, *normal.EligibleOn) < 0 {
		return ProductDisplayLocked
	}
	if policy.AccessKind == AccessUnknown {
		return ProductDisplayLocked
	}
	return ProductDisplayRedeemable
}

func buildNormalRoute(today string, source LiquiditySource, policy LiquidityPolicy) (*LiquidityRoute, error) {
	route := &LiquidityRoute{Kind: RouteNormal, Status: StatusComplete, AmountBasis: "current_value", ActionRequired: "none"}
	if policy.AccessKind == AccessUnknown {
		route.Status = StatusUnavailable
		route.MissingReasons = []string{"access policy is unknown"}
		route.Assumptions = []LiquidityAssumption{AssumptionUnknownAccess}
		return route, nil
	}
	eligible, assumptions, err := policy.actionEligibleOn(today, false, source.Contract)
	if err != nil {
		route.Status = StatusUnavailable
		route.MissingReasons = []string{"access timing is unknown"}
		route.Assumptions = assumptions
		return route, nil
	}
	route.EligibleOn = &eligible
	route.Assumptions = append(route.Assumptions, assumptions...)
	receipt, receiptAssumptions, err := policy.receiptOn(today, false, source.Contract)
	if err != nil {
		route.Status = StatusPartial
		route.MissingReasons = append(route.MissingReasons, "receipt timing is unknown")
		route.Assumptions = append(route.Assumptions, receiptAssumptions...)
		route.Assumptions = appendUniqueAssumption(route.Assumptions, AssumptionUnknownTiming)
		return route, nil
	}
	route.ReceiptOn = receipt
	route.Assumptions = append(route.Assumptions, receiptAssumptions...)
	gross, basis, err := normalGross(source, policy)
	if err != nil {
		return nil, err
	}
	if gross == nil {
		route.Status = StatusUnavailable
		route.MissingReasons = append(route.MissingReasons, "amount is unknown")
		if source.CurrentNativeValue == nil {
			route.Assumptions = appendUniqueAssumption(route.Assumptions, AssumptionMissingPrice)
		}
		return route, nil
	}
	route.GrossNative = gross
	route.AmountBasis = basis
	if source.Managed && source.Contract != nil && source.Contract.Kind == ProductTermDeposit {
		route.Assumptions = appendUniqueAssumption(route.Assumptions, AssumptionForecastInterest)
	}
	net, exact, feeUnknown, feeExceeds, err := applyLiquidityFee(source, policy, basis, *gross, policy.NormalExitFee)
	if err != nil {
		return nil, err
	}
	if feeUnknown {
		route.Status = StatusPartial
		route.MissingReasons = append(route.MissingReasons, "exit fee is unknown")
		route.Assumptions = appendUniqueAssumption(route.Assumptions, AssumptionUnknownFee)
		return route, nil
	}
	route.FeeNative = policy.NormalExitFee
	route.NetNative = net
	route.netExact = exact
	if feeExceeds {
		route.Assumptions = appendUniqueAssumption(route.Assumptions, AssumptionFeeExceedsGross)
	}
	if source.Managed {
		route.ActionRequired = "redeem"
	} else if source.Ref.Kind == SourceHolding {
		route.ActionRequired = "sell"
	} else {
		route.ActionRequired = "withdraw"
	}
	return route, nil
}

func buildEarlyRoute(today string, source LiquiditySource, policy LiquidityPolicy) (*LiquidityRoute, error) {
	if policy.EarlyKind == EarlyNotAllowed {
		return nil, nil
	}
	if source.Managed && source.Contract != nil && source.Contract.Kind == ProductTermDeposit && source.Contract.MaturityOn != nil {
		if compareCivilDates(today, *source.Contract.MaturityOn) >= 0 {
			return nil, nil
		}
	}
	route := &LiquidityRoute{Kind: RouteEarly, Status: StatusComplete, AmountBasis: "current_value", ActionRequired: "early_withdrawal"}

	eligible, assumptions, err := policy.actionEligibleOn(today, true, source.Contract)
	if err != nil {
		route.Status = StatusUnavailable
		route.Assumptions = assumptions
		return route, nil
	}
	if source.Managed && source.Contract != nil && source.Contract.Kind == ProductTermDeposit && source.Contract.MaturityOn != nil {
		if compareCivilDates(eligible, *source.Contract.MaturityOn) >= 0 {
			return nil, nil
		}
	}
	route.EligibleOn = &eligible
	if policy.EarlyKind == EarlyUnknown {
		route.Status = StatusUnavailable
		route.MissingReasons = []string{"early access is unknown"}
		route.Assumptions = []LiquidityAssumption{AssumptionUnknownEarly}
		return route, nil
	}

	receipt, receiptAssumptions, err := policy.receiptOn(today, true, source.Contract)
	if err != nil {
		route.Status = StatusPartial
		route.MissingReasons = []string{"early receipt timing is unknown"}
		route.Assumptions = append(assumptions, receiptAssumptions...)
		return route, nil
	}
	route.ReceiptOn = receipt
	route.Assumptions = append(route.Assumptions, receiptAssumptions...)
	gross, basis, err := earlyGross(source, policy)
	if err != nil {
		return nil, err
	}
	if gross == nil {
		route.Status = StatusUnavailable
		route.MissingReasons = append(route.MissingReasons, "early amount is unknown")
		return route, nil
	}
	route.GrossNative = gross
	route.AmountBasis = basis
	net, exact, feeUnknown, feeExceeds, err := applyLiquidityFee(source, policy, basis, *gross, policy.EarlyFee)
	if err != nil {
		return nil, err
	}
	if feeUnknown {
		route.Status = StatusPartial
		route.MissingReasons = append(route.MissingReasons, "early fee is unknown")
		route.Assumptions = appendUniqueAssumption(route.Assumptions, AssumptionUnknownFee)
		return route, nil
	}
	route.FeeNative = policy.EarlyFee
	route.NetNative = net
	route.netExact = exact
	if feeExceeds {
		route.Assumptions = appendUniqueAssumption(route.Assumptions, AssumptionFeeExceedsGross)
	}
	return route, nil
}

func normalGross(source LiquiditySource, policy LiquidityPolicy) (*Money, string, error) {
	if source.Managed && source.Contract != nil && source.Contract.Kind == ProductTermDeposit {
		gross, err := source.Contract.ForecastGrossProceeds()
		if err != nil {
			return nil, "", err
		}
		return gross, "principal_plus_unpaid_interest", nil
	}
	if source.CurrentNativeValue == nil {
		return nil, "current_value", nil
	}
	gross := *source.CurrentNativeValue
	if policy.AccessibleAmountCap != nil {
		if policy.AccessibleAmountCap.Amount().LessThan(gross.Amount()) {
			gross = *policy.AccessibleAmountCap
		}
	}
	return &gross, "current_value", nil
}

func earlyGross(source LiquiditySource, policy LiquidityPolicy) (*Money, string, error) {
	if policy.EarlyAmountMode != nil && *policy.EarlyAmountMode == EarlyAmountFixedGross {
		if policy.EarlyGrossAmount == nil {
			return nil, "fixed_gross", nil
		}
		gross := *policy.EarlyGrossAmount
		if !source.Managed && policy.AccessibleAmountCap != nil && policy.AccessibleAmountCap.Amount().LessThan(gross.Amount()) {
			gross = *policy.AccessibleAmountCap
		}
		return &gross, "fixed_gross", nil
	}
	if source.CurrentNativeValue == nil {
		return nil, "current_value", nil
	}
	gross := *source.CurrentNativeValue
	if !source.Managed && policy.AccessibleAmountCap != nil && policy.AccessibleAmountCap.Amount().LessThan(gross.Amount()) {
		gross = *policy.AccessibleAmountCap
	}
	return &gross, "current_value", nil
}

func selectRouteForHorizon(horizon string, source LiquiditySourceResult, due bool, requestedReserve Money, convert ConvertToBase) LiquidityBucketResult {
	result := LiquidityBucketResult{HorizonOn: horizon, Status: StatusComplete}
	candidates := make([]*LiquidityRoute, 0, 2)
	if source.NormalRoute != nil && !due {
		candidates = append(candidates, source.NormalRoute)
	}
	if source.EarlyRoute != nil {
		candidates = append(candidates, source.EarlyRoute)
	}
	var selected *LiquidityRoute
	unknownEligible := false
	for _, candidate := range candidates {
		if candidate.EligibleOn != nil && *candidate.EligibleOn > horizon {
			continue
		}
		if candidate.ReceiptOn == nil {
			if candidate.Status != StatusComplete {
				unknownEligible = true
			}
			continue
		}
		if compareCivilDates(*candidate.ReceiptOn, horizon) > 0 {
			continue
		}
		if candidate.NetNative == nil {
			unknownEligible = true
			continue
		}
		if selected == nil {
			selected = candidate
			continue
		}
		if routeNativeExact(candidate).GreaterThan(routeNativeExact(selected)) {
			selected = candidate
			continue
		}
		if routeNativeExact(candidate).Equal(routeNativeExact(selected)) {
			if candidate.Kind == RouteNormal && selected.Kind != RouteNormal {
				selected = candidate
				continue
			}
			if candidate.ReceiptOn != nil && selected.ReceiptOn != nil && compareCivilDates(*candidate.ReceiptOn, *selected.ReceiptOn) < 0 && candidate.Kind == selected.Kind {
				selected = candidate
			}
		}
	}
	if selected == nil {
		if unknownEligible {
			result.Status = StatusPartial
			result.Reasons = append(result.Reasons, "an eligible route has unknown proceeds")
		}
		return result
	}
	copyRoute := *selected
	result.SelectedRoute = &copyRoute
	result.NetNative = selected.NetNative
	exactNet := routeNativeExact(selected)
	result.netExact = &exactNet
	if requestedReserve.Currency() != "" && selected.NetNative != nil && requestedReserve.Currency() != selected.NetNative.Currency() {
		result.Status = StatusPartial
		result.Reasons = append(result.Reasons, "reservation currency does not match the source")
		zero, _ := NewMoney(decimal.Zero, selected.NetNative.Currency())
		unreserved := *selected.NetNative
		result.AppliedReserveNative = &zero
		result.UnreservedNative = &unreserved
		result.ReserveShortfallNative = &zero
	} else if requestedReserve.Currency() != "" && selected.NetNative != nil {
		applied, unreserved, shortfall, exactReserve := applyLiquidityReservation(exactNet, selected.NetNative.Currency(), requestedReserve)
		result.reserveExact = &exactReserve
		result.AppliedReserveNative = &applied
		result.UnreservedNative = &unreserved
		result.ReserveShortfallNative = &shortfall
	} else if selected.NetNative != nil {
		zero, _ := NewMoney(decimal.Zero, selected.NetNative.Currency())
		unreserved := *selected.NetNative
		result.AppliedReserveNative = &zero
		result.UnreservedNative = &unreserved
		result.ReserveShortfallNative = &zero
	}

	if unknownEligible {
		result.Status = StatusPartial
		result.Reasons = append(result.Reasons, "another eligible alternative has unknown proceeds")
	}
	if selected.Status != StatusComplete && result.Status == StatusComplete {
		result.Status = selected.Status
	}
	result.nativeStatus = result.Status
	if convert != nil && selected.NetNative != nil {
		if converted, ok, err := convertLiquidityExact(convert, exactNet, selected.NetNative.Currency()); err == nil && ok {
			result.NetBase = converted
		} else {
			result.Status = StatusPartial
			result.Reasons = append(result.Reasons, "base FX is missing")
		}
	}
	return result
}

func aggregateBucket(horizon string, base CurrencyCode, sources []LiquiditySourceResult, convert ConvertToBase) (LiquidityBucket, error) {
	bucket := LiquidityBucket{HorizonOn: horizon, Status: StatusComplete}
	type nativeAcc struct {
		knownAvailable  decimal.Decimal
		knownReserve    decimal.Decimal
		knownUnreserved decimal.Decimal
		complete        bool
		hasKnown        bool
		unknown         bool
	}
	native := map[CurrencyCode]*nativeAcc{}
	baseKnown := decimal.Zero
	baseReserve := decimal.Zero
	baseUnreserved := decimal.Zero
	baseComplete := true
	baseHasKnown := false
	baseUnknown := false
	baseFXMissing := false
	for _, source := range sources {
		if source.Excluded {
			bucket.ExcludedSourceCount++
			continue
		}
		var row *LiquidityBucketResult
		for i := range source.BucketResults {
			if source.BucketResults[i].HorizonOn == horizon {
				row = &source.BucketResults[i]
				break
			}
		}
		if row == nil {
			continue
		}
		acc := native[source.NativeCurrency]
		if acc == nil {
			acc = &nativeAcc{complete: true}
			native[source.NativeCurrency] = acc
		}
		if source.PolicyOrigin == PolicyOriginAssumed {
			bucket.EstimatedSourceCount++
		}
		if row.NetNative == nil {
			if row.Status == StatusComplete {
				// No eligible route is a known zero only when the row proves it.
				continue
			}
			bucket.UnknownSourceCount++
			acc.unknown = true
			acc.complete = false
			baseUnknown = true
			baseComplete = false
			continue
		}
		acc.hasKnown = true
		acc.knownAvailable = acc.knownAvailable.Add(bucketNativeExact(row))
		if row.AppliedReserveNative != nil {
			acc.knownReserve = acc.knownReserve.Add(bucketReserveExact(row))
		}
		if row.UnreservedNative != nil {
			acc.knownUnreserved = acc.knownUnreserved.Add(bucketNativeExact(row).Sub(bucketReserveExact(row)))
		}
		if !nativeResultComplete(*row) {
			acc.complete = false
			baseComplete = false
			if row.Status == StatusUnavailable || row.Status == StatusPartial {
				acc.unknown = true
				baseUnknown = true
			}
		}
		baseHasKnown = true
		baseKnown = baseKnown.Add(bucketNativeExact(row))
		if row.AppliedReserveNative != nil {
			baseReserve = baseReserve.Add(bucketReserveExact(row))
		}
		if row.UnreservedNative != nil {
			baseUnreserved = baseUnreserved.Add(bucketNativeExact(row).Sub(bucketReserveExact(row)))
		}
		if convert != nil {
			if converted, ok, err := convertLiquidityExact(convert, bucketNativeExact(row), row.NetNative.Currency()); err != nil {
				return LiquidityBucket{}, err
			} else if !ok {
				baseFXMissing = true
				baseComplete = false
			} else if row.NetNative.Currency() != base {
				// Converted values replace native sums for the base total.
				_ = converted
			}
		}
	}
	groups := make([]NativeCurrencyGroup, 0, len(native))
	for currency, acc := range native {
		group := NativeCurrencyGroup{Currency: currency, Status: StatusComplete}
		if acc.hasKnown {
			available, err := displayMoney(acc.knownAvailable, currency)
			if err != nil {
				return LiquidityBucket{}, err
			}
			reserve, err := displayMoney(acc.knownReserve, currency)
			if err != nil {
				return LiquidityBucket{}, err
			}
			unreservedAmount := available.Amount().Sub(reserve.Amount())
			if unreservedAmount.IsNegative() {
				unreservedAmount = decimal.Zero
			}
			unreserved, err := displayMoney(unreservedAmount, currency)
			if err != nil {
				return LiquidityBucket{}, err
			}
			group.KnownAvailableSubtotal = &available
			group.AppliedReserveSubtotal = &reserve
			group.KnownUnreservedSubtotal = &unreserved
			if acc.complete && !acc.unknown {
				group.FullAvailable = &available
				group.FullUnreserved = &unreserved
			} else {
				group.Status = StatusPartial
			}
		} else if acc.unknown {
			group.Status = StatusUnavailable
		} else {
			zero, err := NewMoney(decimal.Zero, currency)
			if err != nil {
				return LiquidityBucket{}, err
			}
			group.FullAvailable = &zero
			group.KnownAvailableSubtotal = &zero
			group.AppliedReserveSubtotal = &zero
			group.FullUnreserved = &zero
			group.KnownUnreservedSubtotal = &zero
		}
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Currency < groups[j].Currency })
	bucket.NativeCurrencyGroups = groups
	sameCurrency := len(groups) == 1 && groups[0].Currency == base
	multiNative := false
	for _, group := range groups {
		if group.Currency != base && !native[group.Currency].knownAvailable.IsZero() {
			multiNative = true
		}
	}
	if convert == nil && !sameCurrency && multiNative {
		baseFXMissing = true
		baseComplete = false
	}
	if convert != nil {
		sumAvailable := decimal.Zero
		sumReserve := decimal.Zero
		convertedOK := true
		hasConvertedAmount := false
		for _, source := range sources {
			if source.Excluded {
				continue
			}
			var row *LiquidityBucketResult
			for i := range source.BucketResults {
				if source.BucketResults[i].HorizonOn == horizon {
					row = &source.BucketResults[i]
					break
				}
			}
			if row == nil || row.NetNative == nil {
				continue
			}
			converted, ok, err := convertLiquidityExact(convert, bucketNativeExact(row), row.NetNative.Currency())
			if err != nil {
				return LiquidityBucket{}, err
			}
			if !ok || converted == nil {
				convertedOK = false
				continue
			}
			hasConvertedAmount = true
			sumAvailable = sumAvailable.Add(*converted)
			if row.AppliedReserveNative != nil {
				convertedReserve, ok, err := convertLiquidityExact(convert, bucketReserveExact(row), row.AppliedReserveNative.Currency())
				if err != nil {
					return LiquidityBucket{}, err
				}
				if ok && convertedReserve != nil {
					sumReserve = sumReserve.Add(*convertedReserve)
				}
			}
		}
		if hasConvertedAmount || (!baseHasKnown && !baseUnknown) {
			available, err := displayMoney(sumAvailable, base)
			if err != nil {
				return LiquidityBucket{}, err
			}
			reserve, err := displayMoney(sumReserve, base)
			if err != nil {
				return LiquidityBucket{}, err
			}
			unreservedAmount := available.Amount().Sub(reserve.Amount())
			if unreservedAmount.IsNegative() {
				unreservedAmount = decimal.Zero
			}
			unreserved, err := displayMoney(unreservedAmount, base)
			if err != nil {
				return LiquidityBucket{}, err
			}
			bucket.KnownAvailableSubtotal = &available
			bucket.AppliedReserveSubtotal = &reserve
			bucket.KnownUnreservedSubtotal = &unreserved
			if convertedOK && baseComplete && !baseFXMissing && !baseUnknown {
				bucket.FullAvailable = &available
				bucket.FullUnreserved = &unreserved
				bucket.Status = StatusComplete
			} else if baseUnknown && !baseHasKnown && !convertedOK {
				bucket.Status = StatusUnavailable
			} else {
				bucket.Status = StatusPartial
			}
		} else if baseUnknown && !baseHasKnown {
			bucket.Status = StatusUnavailable
		} else {
			bucket.Status = StatusPartial
		}
		if baseFXMissing {
			bucket.Warnings = append(bucket.Warnings, "missing base FX")
			if bucket.Status == StatusComplete {
				bucket.Status = StatusPartial
			}
		}
		return bucket, nil
	}
	if sameCurrency {
		group := groups[0]
		bucket.KnownAvailableSubtotal = group.KnownAvailableSubtotal
		bucket.AppliedReserveSubtotal = group.AppliedReserveSubtotal
		bucket.KnownUnreservedSubtotal = group.KnownUnreservedSubtotal
		bucket.FullAvailable = group.FullAvailable
		bucket.FullUnreserved = group.FullUnreserved
		bucket.Status = group.Status
		if !baseHasKnown && !baseUnknown {
			zero, err := NewMoney(decimal.Zero, base)
			if err != nil {
				return LiquidityBucket{}, err
			}
			bucket.FullAvailable = &zero
			bucket.KnownAvailableSubtotal = &zero
			bucket.AppliedReserveSubtotal = &zero
			bucket.FullUnreserved = &zero
			bucket.KnownUnreservedSubtotal = &zero
			bucket.Status = StatusComplete
		}
		if baseUnknown && !baseHasKnown {
			bucket.Status = StatusUnavailable
			bucket.FullAvailable = nil
			bucket.FullUnreserved = nil
		}
		return bucket, nil
	}
	if baseHasKnown {
		bucket.Status = StatusPartial
		if sameCurrency {
			bucket.KnownAvailableSubtotal = groups[0].KnownAvailableSubtotal
		}
	} else if baseUnknown {
		bucket.Status = StatusUnavailable
	} else {
		zero, err := NewMoney(decimal.Zero, base)
		if err != nil {
			return LiquidityBucket{}, err
		}
		bucket.FullAvailable = &zero
		bucket.KnownAvailableSubtotal = &zero
		bucket.AppliedReserveSubtotal = &zero
		bucket.FullUnreserved = &zero
		bucket.KnownUnreservedSubtotal = &zero
	}
	return bucket, nil
}

func displayMoney(value decimal.Decimal, currency CurrencyCode) (Money, error) {
	return NewMoney(value, currency)
}

func emptyBucketResults(horizons []string) []LiquidityBucketResult {
	results := make([]LiquidityBucketResult, 0, len(horizons))
	for _, horizon := range horizons {
		results = append(results, LiquidityBucketResult{HorizonOn: horizon, Status: StatusComplete, Reasons: []string{"excluded"}})
	}
	return results
}

func excludedBucketResults(horizons []string) []LiquidityBucketResult {
	return emptyBucketResults(horizons)
}

func unknownBucketResults(horizons []string) []LiquidityBucketResult {
	results := make([]LiquidityBucketResult, 0, len(horizons))
	for _, horizon := range horizons {
		results = append(results, LiquidityBucketResult{HorizonOn: horizon, Status: StatusUnavailable, Reasons: []string{"unknown"}})
	}
	return results
}

func appendUniqueAssumption(values []LiquidityAssumption, next LiquidityAssumption) []LiquidityAssumption {
	for _, existing := range values {
		if existing == next {
			return values
		}
	}
	return append(values, next)
}

func assertCumulativeMonotonic(buckets []LiquidityBucket) error {
	var prevAvailable *decimal.Decimal
	var prevUnreserved *decimal.Decimal
	for _, bucket := range buckets {
		if bucket.KnownAvailableSubtotal != nil {
			amount := bucket.KnownAvailableSubtotal.Amount()
			if prevAvailable != nil && amount.LessThan(*prevAvailable) {
				return validation("availability", "cumulative known available amounts cannot decrease")
			}
			prevAvailable = &amount
		}
		if bucket.KnownUnreservedSubtotal != nil {
			amount := bucket.KnownUnreservedSubtotal.Amount()
			if prevUnreserved != nil && amount.LessThan(*prevUnreserved) {
				return validation("availability", "cumulative known unreserved amounts cannot decrease")
			}
			prevUnreserved = &amount
		}
	}
	return nil
}

// Native completeness is captured before base FX conversion.
func nativeResultComplete(row LiquidityBucketResult) bool {
	if row.nativeStatus != "" {
		return row.nativeStatus == StatusComplete
	}
	return row.Status == StatusComplete
}
