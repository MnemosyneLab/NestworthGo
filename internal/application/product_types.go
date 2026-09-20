package application

import (
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ProductTermsInput struct {
	Kind                  string  `json:"kind"`
	Name                  string  `json:"name"`
	Note                  *string `json:"note"`
	StartOn               string  `json:"startOn"`
	MaturityOn            *string `json:"maturityOn"`
	InterestMode          string  `json:"interestMode"`
	AnnualRate            *string `json:"annualRate"`
	AnnualRatePercent     *string `json:"annualRatePercent"`
	MaturityInterest      *string `json:"maturityInterest"`
	InterestPaidThroughOn *string `json:"interestPaidThroughOn"`
}

type ProductPolicyInput struct {
	AccessKind          string  `json:"accessKind"`
	UnlockOn            *string `json:"unlockOn"`
	SettlementDays      *int    `json:"settlementDays"`
	DayBasis            *string `json:"dayBasis"`
	ReceiptOnOverride   *string `json:"receiptOnOverride"`
	NormalExitFee       *string `json:"normalExitFee"`
	EarlyKind           string  `json:"earlyKind"`
	EarlySettlementDays *int    `json:"earlySettlementDays"`
	EarlyDayBasis       *string `json:"earlyDayBasis"`
	EarlyFee            *string `json:"earlyFee"`
	EarlyAmountMode     *string `json:"earlyAmountMode"`
	EarlyGrossAmount    *string `json:"earlyGrossAmount"`
	Note                *string `json:"note"`
}

type OpenProductCommand struct {
	AccountID   string             `json:"accountId"`
	Currency    string             `json:"currency"`
	Principal   string             `json:"principal"`
	OpeningFee  *string            `json:"openingFee"`
	EffectiveAt string             `json:"effectiveAt"`
	Terms       ProductTermsInput  `json:"terms"`
	Policy      ProductPolicyInput `json:"policy"`
}

type RecordExistingProductCommand struct {
	AccountID           string             `json:"accountId"`
	Currency            string             `json:"currency"`
	Principal           string             `json:"principal"`
	TotalCostBasis      string             `json:"totalCostBasis"`
	CurrentValue        string             `json:"currentValue"`
	CashExcludesProduct bool               `json:"cashExcludesProduct"`
	EffectiveAt         string             `json:"effectiveAt"`
	Terms               ProductTermsInput  `json:"terms"`
	Policy              ProductPolicyInput `json:"policy"`
}

type ReceiveInterestCommand struct {
	ProductID             string  `json:"productId"`
	Amount                string  `json:"amount"`
	EffectiveAt           string  `json:"effectiveAt"`
	InterestPaidThroughOn *string `json:"interestPaidThroughOn"`
	RemainingInterest     *string `json:"remainingInterest"`
}

type SettleProductCommand struct {
	ProductID             string   `json:"productId"`
	ReturnedPrincipal     *string  `json:"returnedPrincipal"`
	Interest              *string  `json:"interest"`
	GrossProceeds         *string  `json:"grossProceeds"`
	Fee                   *string  `json:"fee"`
	EffectiveAt           string   `json:"effectiveAt"`
	ReleaseReservationIDs []string `json:"releaseReservationIds"`
}

type RenewProductCommand struct {
	Settle     SettleProductCommand `json:"settle"`
	Principal  string               `json:"principal"`
	OpeningFee *string              `json:"openingFee"`
	Terms      ProductTermsInput    `json:"terms"`
	Policy     ProductPolicyInput   `json:"policy"`
}

type UndoProductCommand struct {
	OperationID string `json:"operationId"`
}

type ProductCommand struct {
	Kind            domain.ProductOperationKind   `json:"kind"`
	Open            *OpenProductCommand           `json:"open,omitempty"`
	RecordExisting  *RecordExistingProductCommand `json:"recordExisting,omitempty"`
	ReceiveInterest *ReceiveInterestCommand       `json:"receiveInterest,omitempty"`
	Settle          *SettleProductCommand         `json:"settle,omitempty"`
	Renew           *RenewProductCommand          `json:"renew,omitempty"`
	Undo            *UndoProductCommand           `json:"undo,omitempty"`
}

type ProductOperationPreview struct {
	Kind                domain.ProductOperationKind
	NormalizedJSON      string
	PayloadSHA256       string
	ReviewedStateHash   string
	LocalDate           string
	Timezone            string
	Warnings            []string
	Assumptions         []string
	MissingFields       []string
	Activities          []domain.ChangePreview
	CashBefore          []domain.Money
	CashAfter           []domain.Money
	ProductBefore       *domain.Money
	ProductAfter        *domain.Money
	NetWorthKnown       bool
	ReservationReleases []domain.LiquidityReservationID
	DraftProductIDs     []domain.ProductContractID
}

type ProductOperationReceipt struct {
	OperationID domain.ProductOperationID
	Kind        domain.ProductOperationKind
	ProductIDs  []domain.ProductContractID
	ActivityIDs []domain.ActivityID
	QuoteIDs    []domain.InstrumentQuoteID
	CashAfter   []domain.Money
	Replayed    bool
	CreatedAt   time.Time
}

type ProductOperationPage struct {
	Operations []domain.ProductOperation
	Next       *string
}

type ProductDetail struct {
	Contract         domain.ProductContract
	Policy           domain.LiquidityPolicy
	CurrentValue     *domain.Money
	CurrentCostBasis *domain.Money
	DisplayState     domain.ProductDisplayState
	Reservations     []domain.LiquidityReservation
	PredecessorID    *domain.ProductContractID
	SuccessorID      *domain.ProductContractID
	PermittedActions []string
	DisabledReasons  map[string]string
}

type LiquidityOverviewQuery struct {
	CustomHorizonOn        *string
	IncludeEarlyWithdrawal bool
}

type SavePolicyInput struct {
	Source           domain.LiquiditySourceRef
	ExpectedRevision int
	Policy           ProductPolicyInput
}

type SaveReservationInput struct {
	ID               *domain.LiquidityReservationID
	Source           domain.LiquiditySourceRef
	ExpectedRevision int
	Label            string
	Amount           string
}

type UpdateProductTermsInput struct {
	ProductID        domain.ProductContractID
	ExpectedRevision int
	Terms            ProductTermsInput
	Policy           ProductPolicyInput
}

type AppendProductValuationInput struct {
	ProductID  domain.ProductContractID
	Amount     string
	ObservedAt string
	MutationID string
}

type productReceiptEvidence struct {
	Receipt            ProductOperationReceipt       `json:"receipt"`
	BeforeContracts    []domain.ProductContract      `json:"beforeContracts"`
	AfterContracts     []domain.ProductContract      `json:"afterContracts"`
	BeforePolicies     []domain.LiquidityPolicy      `json:"beforePolicies"`
	AfterPolicies      []domain.LiquidityPolicy      `json:"afterPolicies"`
	BeforeReservations []domain.LiquidityReservation `json:"beforeReservations"`
	CommandSHA         string                        `json:"commandSha"`
	ReviewedStateHash  string                        `json:"reviewedStateHash"`
	LocalDate          string                        `json:"localDate"`
}
