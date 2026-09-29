package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// preparePositionTransfer resolves the destination by account and instrument.
// When that account has no position yet, the returned zero-quantity Holding is
// persisted with the transfer Activity in the caller's transaction.
func (s *Service) preparePositionTransfer(ctx context.Context, snapshot domain.PortfolioSnapshot, state domain.ChangeState, input domain.PositionTransferInput) (domain.ChangeState, any, *domain.Holding, error) {
	if input.ToHoldingID != "" && input.ToAccountID != nil {
		return state, nil, nil, &domain.Error{Code: domain.ErrValidation, Field: "toAccountId", Message: "provide exactly one of toAccountId or toHoldingId"}
	}
	if input.ToHoldingID != "" {
		return state, input, nil, nil
	}
	if input.ToAccountID == nil {
		return state, nil, nil, &domain.Error{Code: domain.ErrValidation, Field: "toAccountId", Message: "provide toAccountId or toHoldingId"}
	}
	source, ok := state.Holdings[input.FromHoldingID]
	if !ok {
		return state, nil, nil, &domain.Error{Code: domain.ErrNotFound, Field: "fromHoldingId", Message: "source Holding was not found"}
	}
	if source.AccountID == *input.ToAccountID {
		return state, nil, nil, &domain.Error{Code: domain.ErrTransferMismatch, Field: "toAccountId", Message: "destination Account must differ from source Account"}
	}
	account, ok := accountFromSnapshot(snapshot, *input.ToAccountID)
	if !ok {
		return state, nil, nil, &domain.Error{Code: domain.ErrNotFound, Field: "toAccountId", Message: "destination Account was not found"}
	}
	if account.Account.HouseholdID != state.HouseholdID {
		return state, nil, nil, &domain.Error{Code: domain.ErrInvalidChange, Field: "toAccountId", Message: "destination Account belongs to a different Household"}
	}
	if account.Account.ArchivedAt != nil {
		return state, nil, nil, &domain.Error{Code: domain.ErrConflict, Field: "toAccountId", Message: "destination Account is archived"}
	}
	instrument, ok := instrumentFromSnapshot(snapshot, source.InstrumentID)
	if !ok {
		return state, nil, nil, &domain.Error{Code: domain.ErrNotFound, Field: "instrumentId", Message: "source Instrument was not found"}
	}
	if instrument.ArchivedAt != nil {
		return state, nil, nil, &domain.Error{Code: domain.ErrConflict, Field: "instrumentId", Message: "source Instrument is archived"}
	}
	if err := s.rejectManagedInstrument(ctx, source.InstrumentID); err != nil {
		return state, nil, nil, err
	}
	for _, existing := range snapshot.Holdings {
		if existing.ArchivedAt == nil && existing.AccountID == *input.ToAccountID && existing.InstrumentID == source.InstrumentID {
			if err := s.rejectManagedHolding(ctx, existing.ID); err != nil {
				return state, nil, nil, err
			}
			input.ToHoldingID = existing.ID
			input.ToAccountID = nil
			return state, input, nil, nil
		}
	}
	zero, err := domain.ParseQuantity("0")
	if err != nil {
		return state, nil, nil, err
	}
	holding, err := domain.NewHoldingForAccount(account.Account, instrument, zero, nil, 0, state.Now)
	if err != nil {
		return state, nil, nil, err
	}
	state.Holdings[holding.ID] = domain.ChangeHoldingState{
		ID: holding.ID, AccountID: holding.AccountID, InstrumentID: holding.InstrumentID,
		InstrumentName: instrument.Name, Currency: instrument.QuoteCurrency, Current: zero,
	}
	input.ToHoldingID = holding.ID
	input.ToAccountID = nil
	return state, input, &holding, nil
}
