package liquidity

import (
	"strings"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// ProductCommandRequest is the tagged union the frontend submits for every
// product lifecycle action. Exactly one payload must match Kind.
type ProductCommandRequest struct {
	Kind            string                                    `json:"kind"`
	Open            *application.OpenProductCommand           `json:"open,omitempty"`
	RecordExisting  *application.RecordExistingProductCommand `json:"recordExisting,omitempty"`
	ReceiveInterest *application.ReceiveInterestCommand       `json:"receiveInterest,omitempty"`
	Settle          *application.SettleProductCommand         `json:"settle,omitempty"`
	Renew           *application.RenewProductCommand          `json:"renew,omitempty"`
	Undo            *application.UndoProductCommand           `json:"undo,omitempty"`
}

func (r ProductCommandRequest) toApplication() (application.ProductCommand, error) {
	kind, err := domain.ParseProductOperationKind(r.Kind)
	if err != nil {
		return application.ProductCommand{}, err
	}
	command := application.ProductCommand{Kind: kind}
	filled := 0
	switch kind {
	case domain.ProductOpOpen:
		if r.Open == nil {
			return application.ProductCommand{}, &domain.Error{Code: domain.ErrValidation, Field: "open", Message: "open payload is required"}
		}
		command.Open = r.Open
		filled++
	case domain.ProductOpRecordExisting:
		if r.RecordExisting == nil {
			return application.ProductCommand{}, &domain.Error{Code: domain.ErrValidation, Field: "recordExisting", Message: "record-existing payload is required"}
		}
		command.RecordExisting = r.RecordExisting
		filled++
	case domain.ProductOpReceiveInterest:
		if r.ReceiveInterest == nil {
			return application.ProductCommand{}, &domain.Error{Code: domain.ErrValidation, Field: "receiveInterest", Message: "interest payload is required"}
		}
		command.ReceiveInterest = r.ReceiveInterest
		filled++
	case domain.ProductOpSettle:
		if r.Settle == nil {
			return application.ProductCommand{}, &domain.Error{Code: domain.ErrValidation, Field: "settle", Message: "settle payload is required"}
		}
		command.Settle = r.Settle
		filled++
	case domain.ProductOpRenew:
		if r.Renew == nil {
			return application.ProductCommand{}, &domain.Error{Code: domain.ErrValidation, Field: "renew", Message: "renew payload is required"}
		}
		command.Renew = r.Renew
		filled++
	case domain.ProductOpUndo:
		if r.Undo == nil {
			return application.ProductCommand{}, &domain.Error{Code: domain.ErrValidation, Field: "undo", Message: "undo payload is required"}
		}
		command.Undo = r.Undo
		filled++
	default:
		return application.ProductCommand{}, &domain.Error{Code: domain.ErrValidation, Field: "kind", Message: "is not a supported product operation"}
	}
	if extra := extraProductPayloads(r, kind); extra > 0 || filled != 1 {
		return application.ProductCommand{}, &domain.Error{Code: domain.ErrValidation, Field: "kind", Message: "exactly one payload must match kind"}
	}
	return command, nil
}

func extraProductPayloads(r ProductCommandRequest, kind domain.ProductOperationKind) int {
	count := 0
	if r.Open != nil && kind != domain.ProductOpOpen {
		count++
	}
	if r.RecordExisting != nil && kind != domain.ProductOpRecordExisting {
		count++
	}
	if r.ReceiveInterest != nil && kind != domain.ProductOpReceiveInterest {
		count++
	}
	if r.Settle != nil && kind != domain.ProductOpSettle {
		count++
	}
	if r.Renew != nil && kind != domain.ProductOpRenew {
		count++
	}
	if r.Undo != nil && kind != domain.ProductOpUndo {
		count++
	}
	return count
}

type OverviewRequest struct {
	CustomHorizonOn        *string `json:"customHorizonOn,omitempty"`
	IncludeEarlyWithdrawal bool    `json:"includeEarlyWithdrawal"`
}

type ListProductsRequest struct {
	AccountID     *string `json:"accountId,omitempty"`
	IncludeClosed bool    `json:"includeClosed"`
}

type ListOperationsRequest struct {
	ProductID string `json:"productId"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type SourceRefDTO struct {
	Kind      string  `json:"kind"`
	AccountID string  `json:"accountId"`
	HoldingID *string `json:"holdingId,omitempty"`
	Currency  *string `json:"currency,omitempty"`
}

func (r SourceRefDTO) toDomain() (domain.LiquiditySourceRef, error) {
	kind, err := domain.ParseLiquiditySourceKind(r.Kind)
	if err != nil {
		return domain.LiquiditySourceRef{}, err
	}
	accountID, err := domain.ParseAccountID(strings.TrimSpace(r.AccountID))
	if err != nil {
		return domain.LiquiditySourceRef{}, err
	}
	ref := domain.LiquiditySourceRef{Kind: kind, AccountID: accountID}
	if r.HoldingID != nil && strings.TrimSpace(*r.HoldingID) != "" {
		holdingID, err := domain.ParseHoldingID(*r.HoldingID)
		if err != nil {
			return domain.LiquiditySourceRef{}, err
		}
		ref.HoldingID = &holdingID
	}
	if r.Currency != nil && strings.TrimSpace(*r.Currency) != "" {
		currency, err := domain.ParseCurrency(*r.Currency)
		if err != nil {
			return domain.LiquiditySourceRef{}, err
		}
		ref.Currency = &currency
	}
	return ref, nil
}

type SavePolicyRequest struct {
	Source           SourceRefDTO                   `json:"sourceRef"`
	ExpectedRevision int                            `json:"expectedRevision"`
	Policy           application.ProductPolicyInput `json:"policy"`
}

type ResetPolicyRequest struct {
	Source           SourceRefDTO `json:"sourceRef"`
	ExpectedRevision int          `json:"expectedRevision"`
}

type SaveReservationRequest struct {
	ID               *string      `json:"id,omitempty"`
	Source           SourceRefDTO `json:"sourceRef"`
	ExpectedRevision int          `json:"expectedRevision"`
	Label            string       `json:"label"`
	Amount           string       `json:"amount"`
}

type ReleaseReservationRequest struct {
	ID               string `json:"id"`
	ExpectedRevision int    `json:"expectedRevision"`
}

type UpdateProductTermsRequest struct {
	ProductID        string                         `json:"productId"`
	ExpectedRevision int                            `json:"expectedRevision"`
	Terms            application.ProductTermsInput  `json:"terms"`
	Policy           application.ProductPolicyInput `json:"policy"`
}

type AppendProductValuationRequest struct {
	ProductID  string `json:"productId"`
	Amount     string `json:"amount"`
	ObservedAt string `json:"observedAt"`
	MutationID string `json:"mutationId"`
}

type RecordProductOperationRequest struct {
	Command           ProductCommandRequest `json:"command"`
	MutationID        string                `json:"mutationId"`
	ReviewedStateHash string                `json:"reviewedStateHash"`
}
