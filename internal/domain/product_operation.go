package domain

import (
	"strings"
	"time"
)

type ProductOperationKind string

const (
	ProductOpOpen             ProductOperationKind = "open"
	ProductOpRecordExisting   ProductOperationKind = "record_existing"
	ProductOpReceiveInterest  ProductOperationKind = "receive_interest"
	ProductOpSettle           ProductOperationKind = "settle"
	ProductOpRenew            ProductOperationKind = "renew"
	ProductOpUndo             ProductOperationKind = "undo"
	ProductOpValueObservation ProductOperationKind = "value_observation"
)

func ParseProductOperationKind(value string) (ProductOperationKind, error) {
	kind := ProductOperationKind(strings.TrimSpace(value))
	switch kind {
	case ProductOpOpen, ProductOpRecordExisting, ProductOpReceiveInterest, ProductOpSettle, ProductOpRenew, ProductOpUndo, ProductOpValueObservation:
		return kind, nil
	default:
		return "", validation("kind", "is not a supported product operation")
	}
}

type ProductOperationRole string

const (
	ProductRoleOpened    ProductOperationRole = "opened"
	ProductRoleSettled   ProductOperationRole = "settled"
	ProductRoleIncome    ProductOperationRole = "income"
	ProductRoleReopened  ProductOperationRole = "reopened"
	ProductRoleCancelled ProductOperationRole = "cancelled"
	ProductRoleValued    ProductOperationRole = "valued"
)

func ParseProductOperationRole(value string) (ProductOperationRole, error) {
	role := ProductOperationRole(strings.TrimSpace(value))
	switch role {
	case ProductRoleOpened, ProductRoleSettled, ProductRoleIncome, ProductRoleReopened, ProductRoleCancelled, ProductRoleValued:
		return role, nil
	default:
		return "", validation("role", "is not a supported product operation role")
	}
}

type ProductActivityPurpose string

const (
	ProductPurposeAcquisition      ProductActivityPurpose = "acquisition"
	ProductPurposeExistingPosition ProductActivityPurpose = "existing_position"
	ProductPurposeRedemption       ProductActivityPurpose = "redemption"
	ProductPurposeInterest         ProductActivityPurpose = "interest"
	ProductPurposeReversal         ProductActivityPurpose = "reversal"
)

func ParseProductActivityPurpose(value string) (ProductActivityPurpose, error) {
	purpose := ProductActivityPurpose(strings.TrimSpace(value))
	switch purpose {
	case ProductPurposeAcquisition, ProductPurposeExistingPosition, ProductPurposeRedemption, ProductPurposeInterest, ProductPurposeReversal:
		return purpose, nil
	default:
		return "", validation("purpose", "is not a supported product activity purpose")
	}
}

// ProductActivityContext associates a ledger Activity with a managed product
// operation. Readers populate it by joining operation-activity links.
type ProductActivityContext struct {
	OperationID  ProductOperationID
	ProductID    ProductContractID
	Purpose      ProductActivityPurpose
	HoldingID    HoldingID
	InstrumentID InstrumentID
	ProductKind  ProductKind
}

type ProductOperation struct {
	ID                  ProductOperationID
	HouseholdID         HouseholdID
	Kind                ProductOperationKind
	PayloadSHA256       string
	RequestVersion      int
	RequestJSON         string
	ResultJSON          string
	EffectiveAt         time.Time
	CreatedAt           time.Time
	ReversesOperationID *ProductOperationID
}

type ProductOperationProduct struct {
	OperationID ProductOperationID
	ProductID   ProductContractID
	Role        ProductOperationRole
}

type ProductOperationActivity struct {
	OperationID ProductOperationID
	ActivityID  ActivityID
	Sequence    int
	Purpose     ProductActivityPurpose
	ProductID   ProductContractID
}

type ProductOperationReservation struct {
	OperationID         ProductOperationID
	ReservationID       LiquidityReservationID
	PreviousReleasedAt  *time.Time
	ResultingReleasedAt *time.Time
	ResultingRevision   int
}

type LiquiditySnapshot struct {
	Portfolio    PortfolioSnapshot
	Contracts    []ProductContract
	Policies     []LiquidityPolicy
	Reservations []LiquidityReservation
}

type ProductBundle struct {
	AsOf                   time.Time
	Operation              ProductOperation
	Instruments            []Instrument
	InstrumentObservations []InstrumentPreferenceObservation
	Holdings               []Holding
	Quotes                 []InstrumentQuote
	Activities             []ActivityCommit
	Contracts              []ProductContract
	Policies               []LiquidityPolicy
	ProductLinks           []ProductOperationProduct
	ActivityLinks          []ProductOperationActivity
	ReservationLinks       []ProductOperationReservation
}

type ProductOperationEvidence struct {
	Operation    ProductOperation
	Products     []ProductOperationProduct
	Activities   []ProductOperationActivity
	Reservations []ProductOperationReservation
}

const ProductRequestVersion = 1
const ManagedHoldingOpenQuantity = "1"
const ManagedHoldingClosedQuantity = "0"
