package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReconciliationTarget names one current ledger endpoint and its desired value.
// An account target is a balance in an explicit currency; a holding target is
// a quantity. UnitCost is required only when a holding's quantity increases.
type ReconciliationTarget struct {
	AccountID       *domain.AccountID
	HoldingID       *domain.HoldingID
	TargetBalance   *domain.Money
	TargetQuantity  *domain.Quantity
	TargetTotalCost *domain.Money
	UnitCost        *domain.UnitPrice
	Note            *string
}

// BuildReconciliationCommands resolves current targets to exact deltas. Call it
// inside PreviewChangesGuardedWith so the snapshot and preview share one read
// gate. Keep the returned commands for RecordChangesGuarded; rebuilding them at
// commit time would change the user's approved amounts or effective time.
func (s *Service) BuildReconciliationCommands(ctx context.Context, targets []ReconciliationTarget) ([]any, error) {
	if len(targets) == 0 || len(targets) > maxChangeBatchSize {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "targets", Message: "provide 1 to 100 targets"}
	}
	origin, snapshot, err := s.loadChangeContext(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.changeStateFrom(origin, snapshot)
	if err != nil {
		return nil, err
	}
	commands := make([]any, 0, len(targets))
	seen := make(map[string]bool, len(targets))
	var costs *costBasisReplayContext
	for index, target := range targets {
		if (target.AccountID == nil) == (target.HoldingID == nil) ||
			(target.AccountID != nil && (target.TargetBalance == nil || target.TargetQuantity != nil || target.TargetTotalCost != nil || target.UnitCost != nil)) ||
			(target.HoldingID != nil && (target.TargetBalance != nil || (target.TargetQuantity == nil && target.TargetTotalCost == nil))) {
			return nil, reconciliationTargetError(index, &domain.Error{Code: domain.ErrValidation, Message: "provide an account balance or a holding quantity and/or total cost"})
		}
		if target.AccountID != nil {
			command, key, buildErr := s.reconciliationBalanceCommand(snapshot, state, target)
			if buildErr != nil {
				return nil, reconciliationTargetError(index, buildErr)
			}
			if seen[key] {
				return nil, reconciliationTargetError(index, &domain.Error{Code: domain.ErrValidation, Message: "endpoint is repeated"})
			}
			seen[key] = true
			commands = append(commands, command)
			continue
		}
		if costs == nil {
			costs = newCostBasisReplayContext(s.repository, snapshot.Holdings)
		}
		holdingCommands, key, buildErr := s.reconciliationHoldingCommand(ctx, state, costs, target)
		if buildErr != nil {
			return nil, reconciliationTargetError(index, buildErr)
		}
		if seen[key] {
			return nil, reconciliationTargetError(index, &domain.Error{Code: domain.ErrValidation, Message: "endpoint is repeated"})
		}
		seen[key] = true
		if len(commands)+len(holdingCommands) > maxChangeBatchSize {
			return nil, reconciliationTargetError(index, &domain.Error{Code: domain.ErrValidation, Message: "targets require more than 100 atomic changes"})
		}
		commands = append(commands, holdingCommands...)
	}
	return commands, nil
}

func (s *Service) reconciliationBalanceCommand(snapshot domain.PortfolioSnapshot, state domain.ChangeState, target ReconciliationTarget) (any, string, error) {
	accountID, err := domain.ParseAccountID(target.AccountID.String())
	if err != nil {
		return nil, "", err
	}
	account, ok := state.Accounts[accountID]
	if !ok {
		return nil, "", &domain.Error{Code: domain.ErrNotFound, Field: "accountId", Message: "Account was not found"}
	}
	if account.Archived {
		return nil, "", &domain.Error{Code: domain.ErrConflict, Field: "accountId", Message: "Account is archived"}
	}
	if target.UnitCost != nil {
		return nil, "", &domain.Error{Code: domain.ErrValidation, Field: "unitCost", Message: "unit cost applies only to an added holding quantity"}
	}
	requested := *target.TargetBalance
	if _, err := domain.ParseSupportedCurrency(requested.Currency().String()); err != nil {
		return nil, "", err
	}
	if _, err := domain.ParseMoney(requested.CanonicalAmount(), requested.Currency()); err != nil {
		return nil, "", err
	}
	key := "account:" + accountID.String() + ":" + requested.Currency().String()
	current := account.Current
	if account.Mode == domain.TrackingHoldings {
		current, err = currentCashAmount(state, accountID, requested.Currency())
		if err != nil {
			return nil, "", err
		}
	} else if requested.Currency() != account.Currency {
		return nil, "", &domain.Error{Code: domain.ErrValidation, Field: "targetBalance", Message: "currency must match the Account"}
	} else {
		// changeStateFrom uses zero as a general fallback. For a balance
		// reconciliation, a missing Account Value is unknown, not zero.
		known := false
		for _, record := range snapshot.Accounts {
			if record.Account.ID == accountID {
				known = record.LatestValue != nil
				break
			}
		}
		if !known {
			return nil, "", &domain.Error{Code: domain.ErrUnavailable, Field: "targetBalance", Message: "current Account balance is unavailable"}
		}
	}
	delta, added, err := differenceMoney(requested, current)
	if err != nil {
		return nil, "", err
	}
	if delta.IsZero() {
		return nil, "", &domain.Error{Code: domain.ErrNoChange, Field: "targetBalance", Message: "balance already matches the target"}
	}
	if account.Mode != domain.TrackingHoldings {
		return domain.ValueUpdateInput{HouseholdID: state.HouseholdID, AccountID: accountID, NewValue: requested, Reason: domain.ReasonReconciliation, EffectiveAt: state.Now, Note: target.Note}, key, nil
	}
	if added {
		return domain.MoneyAddedInput{HouseholdID: state.HouseholdID, AccountID: accountID, Amount: delta, Reason: domain.ReasonReconciliation, EffectiveAt: state.Now, Note: target.Note}, key, nil
	}
	return domain.MoneyRemovedInput{HouseholdID: state.HouseholdID, AccountID: accountID, Amount: delta, Reason: domain.ReasonReconciliation, EffectiveAt: state.Now, Note: target.Note}, key, nil
}

func (s *Service) reconciliationHoldingCommand(ctx context.Context, state domain.ChangeState, costs *costBasisReplayContext, target ReconciliationTarget) ([]any, string, error) {
	holdingID, err := domain.ParseHoldingID(target.HoldingID.String())
	if err != nil {
		return nil, "", err
	}
	holding, ok := state.Holdings[holdingID]
	if !ok {
		return nil, "", &domain.Error{Code: domain.ErrNotFound, Field: "holdingId", Message: "Holding was not found"}
	}
	if holding.Archived {
		return nil, "", &domain.Error{Code: domain.ErrConflict, Field: "holdingId", Message: "Holding is archived"}
	}
	if err := s.rejectManagedHolding(ctx, holdingID); err != nil {
		return nil, "", err
	}
	requested := holding.Current
	if target.TargetQuantity != nil {
		requested = *target.TargetQuantity
		if _, err := domain.ParseQuantity(requested.Canonical()); err != nil {
			return nil, "", err
		}
	}
	if target.TargetTotalCost != nil && target.UnitCost != nil {
		return nil, "", &domain.Error{Code: domain.ErrValidation, Field: "unitCost", Message: "unitCost and totalCost cannot be supplied together"}
	}
	difference := requested.Decimal().Sub(holding.Current.Decimal())
	added := difference.IsPositive()
	if added && target.UnitCost == nil && target.TargetTotalCost == nil {
		return nil, "", &domain.Error{Code: domain.ErrCostBasisRequired, Field: "unitCost", Message: "an explicit per-unit cost is required for added quantity"}
	}
	if difference.IsZero() && target.UnitCost != nil {
		return nil, "", &domain.Error{Code: domain.ErrInvalidChange, Field: "unitCost", Message: "cost-only reconciliation requires totalCost"}
	}
	if !added && target.UnitCost != nil {
		return nil, "", &domain.Error{Code: domain.ErrValidation, Field: "unitCost", Message: "unit cost cannot be changed by reducing quantity"}
	}
	if target.UnitCost != nil {
		if _, err := domain.ParseUnitPrice(target.UnitCost.Canonical()); err != nil {
			return nil, "", err
		}
	}
	lot, err := costs.replay(ctx, holdingID, nil)
	if err != nil {
		return nil, "", err
	}
	if !lot.Current.Quantity.Decimal().Equal(holding.Current.Decimal()) {
		return nil, "", &domain.Error{Code: domain.ErrCostBasisRequired, Field: "holdingId", Message: "current quantity has no matching cost-basis history"}
	}
	commands := make([]any, 0, 2)
	var desiredCost *domain.UnitPrice
	if target.TargetTotalCost != nil {
		total := *target.TargetTotalCost
		if total.Currency() != holding.Currency {
			return nil, "", &domain.Error{Code: domain.ErrValidation, Field: "totalCost", Message: "total cost currency must match the Holding's Instrument currency"}
		}
		if requested.IsZero() {
			if !total.IsZero() {
				return nil, "", &domain.Error{Code: domain.ErrValidation, Field: "totalCost", Message: "a zero-quantity Holding must have zero total cost"}
			}
		} else {
			unit, unitErr := domain.NewUnitPrice(total.Amount().Div(requested.Decimal()).RoundBank(8))
			if unitErr != nil {
				return nil, "", &domain.Error{Code: domain.ErrValidation, Field: "totalCost", Message: "total cost requires an unsupported per-unit cost"}
			}
			actual, amountErr := domain.NewMoney(unit.Decimal().Mul(requested.Decimal()), holding.Currency)
			if amountErr != nil || !actual.Amount().Equal(total.Amount()) {
				return nil, "", &domain.Error{Code: domain.ErrValidation, Field: "totalCost", Message: "total cost cannot be represented at the supported cost precision"}
			}
			desiredCost = &unit
		}
	}
	if !difference.IsZero() {
		amount, amountErr := domain.NewQuantity(difference.Abs())
		if amountErr != nil {
			return nil, "", amountErr
		}
		incomingCost := target.UnitCost
		if added && desiredCost != nil {
			incomingCost = desiredCost
		}
		commands = append(commands, domain.PositionAdjustmentInput{HouseholdID: state.HouseholdID, HoldingID: holdingID, Quantity: amount, Added: added, UnitCost: incomingCost, EffectiveAt: state.Now, Note: target.Note})
	}
	if desiredCost != nil {
		currentCost := lot.Current.AverageUnitCost
		if !difference.IsZero() || !currentCost.Decimal().Equal(desiredCost.Decimal()) {
			commands = append(commands, domain.PositionCostAdjustmentInput{HouseholdID: state.HouseholdID, HoldingID: holdingID, UnitCost: *desiredCost, EffectiveAt: state.Now, Note: target.Note})
		}
	}
	if len(commands) == 0 {
		return nil, "", &domain.Error{Code: domain.ErrNoChange, Field: "targetQuantity", Message: "Holding quantity and cost already match the target"}
	}
	return commands, "holding:" + holdingID.String(), nil
}

func reconciliationTargetError(index int, err error) error {
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		return err
	}
	field := fmt.Sprintf("targets[%d]", index)
	if domainErr.Field != "" {
		field += "." + domainErr.Field
	}
	return &domain.Error{Code: domainErr.Code, Field: field, Message: domainErr.Message}
}
