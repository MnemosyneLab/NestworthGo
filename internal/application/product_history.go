package application

import (
	"errors"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type ProductUndoDisabledReason string

const (
	UndoNoActiveOperation           ProductUndoDisabledReason = "no_active_operation"
	UndoValuationRequiresCorrection ProductUndoDisabledReason = "valuation_requires_correction"
	UndoLaterProductOperation       ProductUndoDisabledReason = "later_product_operation"
	UndoContractChanged             ProductUndoDisabledReason = "contract_changed"
	UndoActiveReservation           ProductUndoDisabledReason = "active_reservation"
	UndoReservationChanged          ProductUndoDisabledReason = "reservation_changed"
	UndoArchivedTarget              ProductUndoDisabledReason = "archived_target"
	UndoInsufficientBalance         ProductUndoDisabledReason = "insufficient_balance"
	UndoInsufficientQuantity        ProductUndoDisabledReason = "insufficient_quantity"
	UndoUnsafe                      ProductUndoDisabledReason = "unsafe_undo"
)

type ProductUndoAvailability struct {
	Operation *domain.ProductOperation
	Allowed   bool
	Reason    *ProductUndoDisabledReason
}

// Retain the mutation's public error identity while tagging a known business refusal.
type productUndoRefusal struct {
	reason ProductUndoDisabledReason
	cause  error
}

func (e *productUndoRefusal) Error() string { return e.cause.Error() }
func (e *productUndoRefusal) Unwrap() error { return e.cause }
func undoRefusal(reason ProductUndoDisabledReason, cause error) error {
	return &productUndoRefusal{reason, cause}
}

func (s *Service) productUndoAvailability(snapshot domain.ProductOperationHistorySnapshot) (ProductUndoAvailability, error) {
	result := ProductUndoAvailability{Operation: snapshot.Candidate}
	reason := UndoNoActiveOperation
	if snapshot.Candidate == nil {
		result.Reason = &reason
		return result, nil
	}
	if snapshot.Candidate.Kind == domain.ProductOpValueObservation {
		reason = UndoValuationRequiresCorrection
		result.Reason = &reason
		return result, nil
	}
	if snapshot.UndoEvidence == nil {
		return ProductUndoAvailability{}, &domain.Error{Code: domain.ErrIntegrity, Message: "undo evidence is missing"}
	}
	state, err := s.changeStateFrom(snapshot.Portfolio.Origin, snapshot.Portfolio)
	if err != nil {
		return ProductUndoAvailability{}, err
	}
	_, err = planUndoEvidence(snapshot.Portfolio.Origin, state, *snapshot.UndoEvidence, domain.NewProductOperationID(), state.Now, state.Now, false)
	if err == nil {
		result.Allowed = true
		return result, nil
	}
	var refusal *productUndoRefusal
	var business *domain.Error
	if errors.As(err, &refusal) {
		reason = refusal.reason
	} else if errors.As(err, &business) {
		switch business.Code {
		case domain.ErrInsufficientBalance:
			reason = UndoInsufficientBalance
		case domain.ErrInsufficientQuantity:
			reason = UndoInsufficientQuantity
		case domain.ErrAlreadyUndone:
			reason = UndoUnsafe
		default:
			return ProductUndoAvailability{}, err
		}
	} else {
		return ProductUndoAvailability{}, err
	}
	result.Reason = &reason
	return result, nil
}

// Match SQLite's final active-target guard, scoped to product inverse planning.
// Replay and unrelated historical correction semantics remain independent.
func validateProductUndoTargets(state domain.ChangeState, effects []domain.ActivityEffect) error {
	for _, effect := range effects {
		switch effect.Target {
		case domain.EffectTargetAccountValue, domain.EffectTargetAccountCash:
			if effect.AccountID == nil {
				return &domain.Error{Code: domain.ErrIntegrity, Message: "Account effect is missing its Account"}
			}
			account, ok := state.Accounts[*effect.AccountID]
			if !ok {
				return &domain.Error{Code: domain.ErrNotFound, Field: "accountId", Message: "Account was not found"}
			}
			if account.Archived {
				return undoRefusal(UndoArchivedTarget, &domain.Error{Code: domain.ErrConflict, Field: "accountId", Message: "Account is archived"})
			}
		case domain.EffectTargetHoldingQuantity, domain.EffectTargetHoldingCost:
			if effect.HoldingID == nil {
				return &domain.Error{Code: domain.ErrIntegrity, Message: "Holding effect is missing its Holding"}
			}
			holding, ok := state.Holdings[*effect.HoldingID]
			if !ok {
				return &domain.Error{Code: domain.ErrNotFound, Field: "holdingId", Message: "Holding was not found"}
			}
			if holding.Archived {
				return undoRefusal(UndoArchivedTarget, &domain.Error{Code: domain.ErrConflict, Field: "holdingId", Message: "Holding, its Account, or Instrument is archived"})
			}
		}
	}
	return nil
}
