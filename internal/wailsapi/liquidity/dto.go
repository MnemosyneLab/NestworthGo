package liquidity

import (
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type LiquidityOverviewDTO struct {
	AsOf                   string                     `json:"asOf"`
	LocalDate              string                     `json:"localDate"`
	Timezone               string                     `json:"timezone"`
	BaseCurrency           string                     `json:"baseCurrency"`
	Assumptions            []string                   `json:"assumptions"`
	Buckets                []LiquidityBucketDTO       `json:"buckets"`
	Sources                []LiquiditySourceDTO       `json:"sources"`
	UnresolvedReservations []UnresolvedReservationDTO `json:"unresolvedReservations"`
}

type LiquidityBucketDTO struct {
	HorizonOn               string                   `json:"horizonOn"`
	Status                  string                   `json:"status"`
	FullAvailable           *wire.MoneyView          `json:"fullAvailable"`
	KnownAvailableSubtotal  *wire.MoneyView          `json:"knownAvailableSubtotal"`
	AppliedReserveSubtotal  *wire.MoneyView          `json:"appliedReserveSubtotal"`
	FullUnreserved          *wire.MoneyView          `json:"fullUnreserved"`
	KnownUnreservedSubtotal *wire.MoneyView          `json:"knownUnreservedSubtotal"`
	UnknownSourceCount      int                      `json:"unknownSourceCount"`
	ExcludedSourceCount     int                      `json:"excludedSourceCount"`
	EstimatedSourceCount    int                      `json:"estimatedSourceCount"`
	NativeCurrencyGroups    []NativeCurrencyGroupDTO `json:"nativeCurrencyGroups"`
	Warnings                []string                 `json:"warnings"`
}

type NativeCurrencyGroupDTO struct {
	Currency                string          `json:"currency"`
	Status                  string          `json:"status"`
	FullAvailable           *wire.MoneyView `json:"fullAvailable"`
	KnownAvailableSubtotal  *wire.MoneyView `json:"knownAvailableSubtotal"`
	AppliedReserveSubtotal  *wire.MoneyView `json:"appliedReserveSubtotal"`
	FullUnreserved          *wire.MoneyView `json:"fullUnreserved"`
	KnownUnreservedSubtotal *wire.MoneyView `json:"knownUnreservedSubtotal"`
}

type LiquiditySourceDTO struct {
	SourceRef            SourceRefDTO           `json:"sourceRef"`
	SourceKey            string                 `json:"sourceKey"`
	AccountID            string                 `json:"accountId"`
	ProductID            *string                `json:"productId"`
	DisplayName          string                 `json:"displayName"`
	NativeCurrency       string                 `json:"nativeCurrency"`
	CurrentNativeValue   *wire.MoneyView        `json:"currentNativeValue"`
	ValueAsOf            *string                `json:"valueAsOf"`
	PriceEvidence        *wire.QuoteEvidenceDTO `json:"priceEvidence"`
	FXEvidence           *wire.QuoteEvidenceDTO `json:"fxEvidence"`
	PolicyOrigin         string                 `json:"policyOrigin"`
	Policy               *PolicyDTO             `json:"policy"`
	ContractState        *string                `json:"contractState"`
	DisplayState         string                 `json:"displayState"`
	ReservationRequested wire.MoneyView         `json:"reservationRequested"`
	NormalRoute          *RouteDTO              `json:"normalRoute"`
	EarlyRoute           *RouteDTO              `json:"earlyRoute"`
	BucketResults        []BucketResultDTO      `json:"bucketResults"`
	Reasons              []string               `json:"reasons"`
	Assumptions          []string               `json:"assumptions"`
	Excluded             bool                   `json:"excluded"`
	DueUnconfirmed       bool                   `json:"dueUnconfirmed"`
}

type RouteDTO struct {
	Kind           string          `json:"kind"`
	EligibleOn     *string         `json:"eligibleOn"`
	ReceiptOn      *string         `json:"receiptOn"`
	GrossNative    *wire.MoneyView `json:"grossNative"`
	FeeNative      *wire.MoneyView `json:"feeNative"`
	NetNative      *wire.MoneyView `json:"netNative"`
	Status         string          `json:"status"`
	AmountBasis    string          `json:"amountBasis"`
	ActionRequired string          `json:"actionRequired"`
	Assumptions    []string        `json:"assumptions"`
	MissingReasons []string        `json:"missingReasons"`
}

type BucketResultDTO struct {
	HorizonOn              string          `json:"horizonOn"`
	SelectedRoute          *RouteDTO       `json:"selectedRoute"`
	NetNative              *wire.MoneyView `json:"netNative"`
	NetBase                *wire.MoneyView `json:"netBase"`
	AppliedReserveNative   *wire.MoneyView `json:"appliedReserveNative"`
	UnreservedNative       *wire.MoneyView `json:"unreservedNative"`
	ReserveShortfallNative *wire.MoneyView `json:"reserveShortfallNative"`
	Status                 string          `json:"status"`
	Reasons                []string        `json:"reasons"`
}

type UnresolvedReservationDTO struct {
	Reservation ReservationDTO `json:"reservation"`
	Reason      string         `json:"reason"`
}

type PolicyDTO struct {
	ID                  string          `json:"id"`
	SourceRef           SourceRefDTO    `json:"sourceRef"`
	AccessKind          string          `json:"accessKind"`
	UnlockOn            *string         `json:"unlockOn"`
	SettlementDays      *int            `json:"settlementDays"`
	DayBasis            *string         `json:"dayBasis"`
	ReceiptOnOverride   *string         `json:"receiptOnOverride"`
	AccessibleAmountCap *wire.MoneyView `json:"accessibleAmountCap"`
	NormalExitFee       *wire.MoneyView `json:"normalExitFee"`
	EarlyKind           string          `json:"earlyKind"`
	EarlySettlementDays *int            `json:"earlySettlementDays"`
	EarlyDayBasis       *string         `json:"earlyDayBasis"`
	EarlyFee            *wire.MoneyView `json:"earlyFee"`
	EarlyAmountMode     *string         `json:"earlyAmountMode"`
	EarlyGrossAmount    *wire.MoneyView `json:"earlyGrossAmount"`
	ConfirmedAt         *string         `json:"confirmedAt"`
	Note                *string         `json:"note"`
	Revision            int             `json:"revision"`
}

type ReservationDTO struct {
	ID         string         `json:"id"`
	SourceRef  SourceRefDTO   `json:"sourceRef"`
	Label      string         `json:"label"`
	Amount     wire.MoneyView `json:"amount"`
	Currency   string         `json:"currency"`
	Revision   int            `json:"revision"`
	CreatedAt  string         `json:"createdAt"`
	UpdatedAt  string         `json:"updatedAt"`
	ReleasedAt *string        `json:"releasedAt"`
}

type ProductDTO struct {
	ID                    string          `json:"id"`
	AccountID             string          `json:"accountId"`
	HoldingID             string          `json:"holdingId"`
	InstrumentID          string          `json:"instrumentId"`
	Kind                  string          `json:"kind"`
	Name                  string          `json:"name"`
	Note                  *string         `json:"note"`
	Currency              string          `json:"currency"`
	Principal             wire.MoneyView  `json:"principal"`
	CurrentValue          *wire.MoneyView `json:"currentValue"`
	CurrentCostBasis      *wire.MoneyView `json:"currentCostBasis"`
	StartOn               string          `json:"startOn"`
	MaturityOn            *string         `json:"maturityOn"`
	InterestMode          string          `json:"interestMode"`
	AnnualRate            *string         `json:"annualRate"`
	MaturityInterest      *wire.MoneyView `json:"maturityInterest"`
	InterestPaidThroughOn *string         `json:"interestPaidThroughOn"`
	State                 string          `json:"state"`
	DisplayState          string          `json:"displayState"`
	Policy                PolicyDTO       `json:"policy"`
	Revision              int             `json:"revision"`
	PredecessorID         *string         `json:"predecessorId"`
	SuccessorID           *string         `json:"successorId"`
	NextAction            string          `json:"nextAction"`
}

type ProductDetailDTO struct {
	Product          ProductDTO        `json:"product"`
	Reservations     []ReservationDTO  `json:"reservations"`
	PermittedActions []string          `json:"permittedActions"`
	DisabledReasons  map[string]string `json:"disabledReasons"`
}

type ProductOperationPreviewDTO struct {
	Kind                string                  `json:"kind"`
	NormalizedJSON      string                  `json:"normalizedJson"`
	PayloadSHA256       string                  `json:"payloadSha256"`
	ReviewedStateHash   string                  `json:"reviewedStateHash"`
	LocalDate           string                  `json:"localDate"`
	Timezone            string                  `json:"timezone"`
	Warnings            []string                `json:"warnings"`
	Assumptions         []string                `json:"assumptions"`
	MissingFields       []string                `json:"missingFields"`
	Activities          []wire.ChangePreviewDTO `json:"activities"`
	CashBefore          []wire.MoneyView        `json:"cashBefore"`
	CashAfter           []wire.MoneyView        `json:"cashAfter"`
	ProductBefore       *wire.MoneyView         `json:"productBefore"`
	ProductAfter        *wire.MoneyView         `json:"productAfter"`
	NetWorthKnown       bool                    `json:"netWorthKnown"`
	ReservationReleases []string                `json:"reservationReleases"`
	DraftProductIDs     []string                `json:"draftProductIds"`
}

type ProductOperationReceiptDTO struct {
	OperationID string           `json:"operationId"`
	Kind        string           `json:"kind"`
	ProductIDs  []string         `json:"productIds"`
	ActivityIDs []string         `json:"activityIds"`
	QuoteIDs    []string         `json:"quoteIds"`
	CashAfter   []wire.MoneyView `json:"cashAfter"`
	Replayed    bool             `json:"replayed"`
	CreatedAt   string           `json:"createdAt"`
}

type ProductOperationDTO struct {
	ID                  string  `json:"id"`
	Kind                string  `json:"kind"`
	EffectiveAt         string  `json:"effectiveAt"`
	CreatedAt           string  `json:"createdAt"`
	ReversesOperationID *string `json:"reversesOperationId"`
}

type ProductOperationPageDTO struct {
	Operations []ProductOperationDTO `json:"operations"`
	Next       *string               `json:"next"`
}

func fromOverview(value domain.LiquidityOverview) LiquidityOverviewDTO {
	dto := LiquidityOverviewDTO{
		AsOf: wire.FormatTime(value.AsOf), LocalDate: value.LocalDate, Timezone: value.Timezone,
		BaseCurrency: value.BaseCurrency.String(), Assumptions: assumptionStrings(value.Assumptions),
		Buckets:                make([]LiquidityBucketDTO, 0, len(value.Buckets)),
		Sources:                make([]LiquiditySourceDTO, 0, len(value.Sources)),
		UnresolvedReservations: make([]UnresolvedReservationDTO, 0, len(value.UnresolvedReservations)),
	}
	for _, bucket := range value.Buckets {
		dto.Buckets = append(dto.Buckets, fromBucket(bucket, value.BaseCurrency))
	}
	for _, source := range value.Sources {
		dto.Sources = append(dto.Sources, fromSource(source, value.BaseCurrency))
	}
	for _, unresolved := range value.UnresolvedReservations {
		dto.UnresolvedReservations = append(dto.UnresolvedReservations, UnresolvedReservationDTO{
			Reservation: fromReservation(unresolved.Reservation), Reason: unresolved.Reason,
		})
	}
	return dto
}

func fromBucket(value domain.LiquidityBucket, base domain.CurrencyCode) LiquidityBucketDTO {
	dto := LiquidityBucketDTO{
		HorizonOn: value.HorizonOn, Status: string(value.Status),
		FullAvailable:           wire.FromMoneyPtr(value.FullAvailable),
		KnownAvailableSubtotal:  wire.FromMoneyPtr(value.KnownAvailableSubtotal),
		AppliedReserveSubtotal:  wire.FromMoneyPtr(value.AppliedReserveSubtotal),
		FullUnreserved:          wire.FromMoneyPtr(value.FullUnreserved),
		KnownUnreservedSubtotal: wire.FromMoneyPtr(value.KnownUnreservedSubtotal),
		UnknownSourceCount:      value.UnknownSourceCount, ExcludedSourceCount: value.ExcludedSourceCount,
		EstimatedSourceCount: value.EstimatedSourceCount, Warnings: emptyStrings(value.Warnings),
		NativeCurrencyGroups: make([]NativeCurrencyGroupDTO, 0, len(value.NativeCurrencyGroups)),
	}
	_ = base
	for _, group := range value.NativeCurrencyGroups {
		dto.NativeCurrencyGroups = append(dto.NativeCurrencyGroups, NativeCurrencyGroupDTO{
			Currency: group.Currency.String(), Status: string(group.Status),
			FullAvailable:           wire.FromMoneyPtr(group.FullAvailable),
			KnownAvailableSubtotal:  wire.FromMoneyPtr(group.KnownAvailableSubtotal),
			AppliedReserveSubtotal:  wire.FromMoneyPtr(group.AppliedReserveSubtotal),
			FullUnreserved:          wire.FromMoneyPtr(group.FullUnreserved),
			KnownUnreservedSubtotal: wire.FromMoneyPtr(group.KnownUnreservedSubtotal),
		})
	}
	return dto
}

func fromSource(value domain.LiquiditySourceResult, base domain.CurrencyCode) LiquiditySourceDTO {
	dto := LiquiditySourceDTO{
		SourceRef: fromSourceRef(value.Ref), SourceKey: value.SourceKey, AccountID: value.AccountID.String(),
		DisplayName: value.DisplayName, NativeCurrency: value.NativeCurrency.String(),
		CurrentNativeValue: wire.FromMoneyPtr(value.CurrentNativeValue),
		ValueAsOf:          wire.FormatTimePtr(value.ValueAsOf), PolicyOrigin: string(value.PolicyOrigin),
		PriceEvidence: wire.FromQuoteEvidence(value.PriceEvidence), FXEvidence: wire.FromQuoteEvidence(value.FXEvidence),
		ReservationRequested: wire.FromMoney(value.ReservationRequested),
		NormalRoute:          fromRoute(value.NormalRoute), EarlyRoute: fromRoute(value.EarlyRoute),
		BucketResults: make([]BucketResultDTO, 0, len(value.BucketResults)), Reasons: emptyStrings(value.Reasons),
		DisplayState: string(value.DisplayState), Assumptions: assumptionStrings(value.Assumptions),
		Excluded: value.Excluded, DueUnconfirmed: value.DueUnconfirmed,
	}
	if value.ProductID != nil {
		id := value.ProductID.String()
		dto.ProductID = &id
	}
	if value.Policy != nil {
		policy := fromPolicy(*value.Policy)
		dto.Policy = &policy
	}
	if value.ContractState != nil {
		state := string(*value.ContractState)
		dto.ContractState = &state
	}
	for _, result := range value.BucketResults {
		dto.BucketResults = append(dto.BucketResults, fromBucketResult(result, base))
	}
	return dto
}

func fromBucketResult(value domain.LiquidityBucketResult, base domain.CurrencyCode) BucketResultDTO {
	return BucketResultDTO{
		HorizonOn: value.HorizonOn, SelectedRoute: fromRoute(value.SelectedRoute),
		NetNative: wire.FromMoneyPtr(value.NetNative), NetBase: fromBaseAmount(value.NetBase, base),
		AppliedReserveNative:   wire.FromMoneyPtr(value.AppliedReserveNative),
		UnreservedNative:       wire.FromMoneyPtr(value.UnreservedNative),
		ReserveShortfallNative: wire.FromMoneyPtr(value.ReserveShortfallNative),
		Status:                 string(value.Status), Reasons: emptyStrings(value.Reasons),
	}
}

func fromRoute(value *domain.LiquidityRoute) *RouteDTO {
	if value == nil {
		return nil
	}
	return &RouteDTO{
		Kind: string(value.Kind), EligibleOn: value.EligibleOn, ReceiptOn: value.ReceiptOn,
		GrossNative: wire.FromMoneyPtr(value.GrossNative), FeeNative: wire.FromMoneyPtr(value.FeeNative),
		NetNative: wire.FromMoneyPtr(value.NetNative), Status: string(value.Status),
		AmountBasis: value.AmountBasis, ActionRequired: value.ActionRequired,
		Assumptions: assumptionStrings(value.Assumptions), MissingReasons: emptyStrings(value.MissingReasons),
	}
}

func fromSourceRef(value domain.LiquiditySourceRef) SourceRefDTO {
	dto := SourceRefDTO{Kind: string(value.Kind), AccountID: value.AccountID.String()}
	if value.HoldingID != nil {
		id := value.HoldingID.String()
		dto.HoldingID = &id
	}
	if value.Currency != nil {
		currency := value.Currency.String()
		dto.Currency = &currency
	}
	return dto
}

func fromPolicy(value domain.LiquidityPolicy) PolicyDTO {
	dto := PolicyDTO{
		ID: value.ID.String(), SourceRef: fromSourceRef(value.Source), AccessKind: string(value.AccessKind),
		UnlockOn: value.UnlockOn, SettlementDays: value.SettlementDays, ReceiptOnOverride: value.ReceiptOnOverride,
		AccessibleAmountCap: wire.FromMoneyPtr(value.AccessibleAmountCap), NormalExitFee: wire.FromMoneyPtr(value.NormalExitFee),
		EarlyKind: string(value.EarlyKind), EarlySettlementDays: value.EarlySettlementDays, EarlyFee: wire.FromMoneyPtr(value.EarlyFee),
		EarlyGrossAmount: wire.FromMoneyPtr(value.EarlyGrossAmount), ConfirmedAt: wire.FormatTimePtr(value.ConfirmedAt),
		Note: value.Note, Revision: value.Revision,
	}
	if value.DayBasis != nil {
		basis := string(*value.DayBasis)
		dto.DayBasis = &basis
	}
	if value.EarlyDayBasis != nil {
		basis := string(*value.EarlyDayBasis)
		dto.EarlyDayBasis = &basis
	}
	if value.EarlyAmountMode != nil {
		mode := string(*value.EarlyAmountMode)
		dto.EarlyAmountMode = &mode
	}
	return dto
}

func fromReservation(value domain.LiquidityReservation) ReservationDTO {
	return ReservationDTO{
		ID: value.ID.String(), SourceRef: fromSourceRef(value.Source), Label: value.Label,
		Amount: wire.FromMoney(value.Amount), Currency: value.Currency.String(), Revision: value.Revision,
		CreatedAt: wire.FormatTime(value.CreatedAt), UpdatedAt: wire.FormatTime(value.UpdatedAt),
		ReleasedAt: wire.FormatTimePtr(value.ReleasedAt),
	}
}

func fromProductDetail(value application.ProductDetail) ProductDetailDTO {
	product := ProductDTO{
		ID: value.Contract.ID.String(), AccountID: value.Contract.AccountID.String(),
		HoldingID: value.Contract.HoldingID.String(), InstrumentID: value.Contract.InstrumentID.String(),
		Kind: string(value.Contract.Kind), Name: value.Contract.Name, Note: value.Contract.Note,
		Currency: value.Contract.Currency.String(), Principal: wire.FromMoney(value.Contract.Principal),
		CurrentValue: wire.FromMoneyPtr(value.CurrentValue), CurrentCostBasis: wire.FromMoneyPtr(value.CurrentCostBasis),
		StartOn: value.Contract.StartOn, MaturityOn: value.Contract.MaturityOn, InterestMode: string(value.Contract.InterestMode),
		MaturityInterest: wire.FromMoneyPtr(value.Contract.MaturityInterest), InterestPaidThroughOn: value.Contract.InterestPaidThroughOn,
		State: string(value.Contract.State), DisplayState: string(value.DisplayState), Policy: fromPolicy(value.Policy),
		Revision: value.Contract.Revision, NextAction: nextProductAction(value),
	}
	if value.Contract.AnnualRate != nil {
		rate := value.Contract.AnnualRate.Canonical()
		product.AnnualRate = &rate
	}
	if value.PredecessorID != nil {
		id := value.PredecessorID.String()
		product.PredecessorID = &id
	}
	if value.SuccessorID != nil {
		id := value.SuccessorID.String()
		product.SuccessorID = &id
	}
	reservations := make([]ReservationDTO, 0, len(value.Reservations))
	for _, reservation := range value.Reservations {
		reservations = append(reservations, fromReservation(reservation))
	}
	reasons := value.DisabledReasons
	if reasons == nil {
		reasons = map[string]string{}
	}
	return ProductDetailDTO{
		Product: product, Reservations: reservations, PermittedActions: emptyStrings(value.PermittedActions), DisabledReasons: reasons,
	}
}

func fromPreview(value application.ProductOperationPreview) ProductOperationPreviewDTO {
	activities := make([]wire.ChangePreviewDTO, 0, len(value.Activities))
	for _, preview := range value.Activities {
		activities = append(activities, wire.FromChangePreview(preview))
	}
	releases := make([]string, 0, len(value.ReservationReleases))
	for _, id := range value.ReservationReleases {
		releases = append(releases, id.String())
	}
	drafts := make([]string, 0, len(value.DraftProductIDs))
	for _, id := range value.DraftProductIDs {
		drafts = append(drafts, id.String())
	}
	return ProductOperationPreviewDTO{
		Kind: string(value.Kind), NormalizedJSON: value.NormalizedJSON, PayloadSHA256: value.PayloadSHA256,
		ReviewedStateHash: value.ReviewedStateHash, LocalDate: value.LocalDate, Timezone: value.Timezone,
		Warnings: emptyStrings(value.Warnings), Assumptions: emptyStrings(value.Assumptions), MissingFields: emptyStrings(value.MissingFields),
		Activities: activities, CashBefore: fromMoneyList(value.CashBefore), CashAfter: fromMoneyList(value.CashAfter),
		ProductBefore: wire.FromMoneyPtr(value.ProductBefore), ProductAfter: wire.FromMoneyPtr(value.ProductAfter),
		NetWorthKnown: value.NetWorthKnown, ReservationReleases: releases, DraftProductIDs: drafts,
	}
}

func fromReceipt(value application.ProductOperationReceipt) ProductOperationReceiptDTO {
	productIDs := make([]string, 0, len(value.ProductIDs))
	for _, id := range value.ProductIDs {
		productIDs = append(productIDs, id.String())
	}
	activityIDs := make([]string, 0, len(value.ActivityIDs))
	for _, id := range value.ActivityIDs {
		activityIDs = append(activityIDs, id.String())
	}
	quoteIDs := make([]string, 0, len(value.QuoteIDs))
	for _, id := range value.QuoteIDs {
		quoteIDs = append(quoteIDs, id.String())
	}
	return ProductOperationReceiptDTO{
		OperationID: value.OperationID.String(), Kind: string(value.Kind), ProductIDs: productIDs,
		ActivityIDs: activityIDs, QuoteIDs: quoteIDs, CashAfter: fromMoneyList(value.CashAfter),
		Replayed: value.Replayed, CreatedAt: wire.FormatTime(value.CreatedAt),
	}
}

func fromOperation(value domain.ProductOperation) ProductOperationDTO {
	dto := ProductOperationDTO{ID: value.ID.String(), Kind: string(value.Kind), EffectiveAt: wire.FormatTime(value.EffectiveAt), CreatedAt: wire.FormatTime(value.CreatedAt)}
	if value.ReversesOperationID != nil {
		id := value.ReversesOperationID.String()
		dto.ReversesOperationID = &id
	}
	return dto
}

func fromMoneyList(values []domain.Money) []wire.MoneyView {
	result := make([]wire.MoneyView, 0, len(values))
	for _, value := range values {
		result = append(result, wire.FromMoney(value))
	}
	return result
}

func fromBaseAmount(value *decimal.Decimal, currency domain.CurrencyCode) *wire.MoneyView {
	if value == nil {
		return nil
	}
	return &wire.MoneyView{Amount: value.String(), Currency: currency.String()}
}

func assumptionStrings(values []domain.LiquidityAssumption) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func emptyStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nextProductAction(value application.ProductDetail) string {
	if value.DisplayState == domain.ProductDisplayDueUnconfirmed {
		return "confirm_receipt"
	}
	if len(value.PermittedActions) == 0 {
		return ""
	}
	return value.PermittedActions[0]
}
