package application

import "github.com/waltwang/nestworth-go/internal/domain"

// prepareChangeHolding adds a draft zero-quantity Holding to the preview state
// for changes that introduce a position. The caller persists that Holding and
// the resulting Activity in one transaction after validating the preview.
func (s *Service) prepareChangeHolding(snapshot domain.PortfolioSnapshot, state domain.ChangeState, command any) (domain.ChangeState, any, *domain.Holding, error) {
	if input, ok := command.(domain.PositionImportInput); ok {
		return s.preparePositionImport(snapshot, state, input)
	}
	return s.prepareTradeHolding(snapshot, state, command)
}

func (s *Service) preparePositionImport(snapshot domain.PortfolioSnapshot, state domain.ChangeState, input domain.PositionImportInput) (domain.ChangeState, any, *domain.Holding, error) {
	if input.HouseholdID != state.HouseholdID {
		return state, nil, nil, &domain.Error{Code: domain.ErrInvalidChange, Field: "householdId", Message: "position belongs to a different Household"}
	}
	if input.Quantity.IsZero() {
		return state, nil, nil, &domain.Error{Code: domain.ErrInvalidChange, Field: "quantity", Message: "quantity must be greater than zero"}
	}
	if input.UnitCost == nil {
		return state, nil, nil, &domain.Error{Code: domain.ErrCostBasisRequired, Field: "unitCost", Message: "an existing position requires its acquisition cost per unit"}
	}
	account, ok := accountFromSnapshot(snapshot, input.AccountID)
	if !ok {
		return state, nil, nil, &domain.Error{Code: domain.ErrNotFound, Field: "accountId", Message: "Account was not found"}
	}
	instrument, ok := instrumentFromSnapshot(snapshot, input.InstrumentID)
	if !ok {
		return state, nil, nil, &domain.Error{Code: domain.ErrNotFound, Field: "instrumentId", Message: "Instrument was not found"}
	}
	if account.Account.HouseholdID != state.HouseholdID || instrument.HouseholdID != state.HouseholdID {
		return state, nil, nil, &domain.Error{Code: domain.ErrInvalidChange, Field: "householdId", Message: "Account and Instrument must belong to the active Household"}
	}
	if account.Account.ArchivedAt != nil {
		return state, nil, nil, &domain.Error{Code: domain.ErrValidation, Field: "accountId", Message: "account is archived"}
	}
	if instrument.ArchivedAt != nil {
		return state, nil, nil, &domain.Error{Code: domain.ErrValidation, Field: "instrumentId", Message: "instrument is archived"}
	}
	if input.Currency != instrument.QuoteCurrency {
		return state, nil, nil, &domain.Error{Code: domain.ErrValidation, Field: "currency", Message: "cost currency must match the Instrument quote currency"}
	}
	for _, existing := range snapshot.Holdings {
		if existing.ArchivedAt == nil && existing.AccountID == input.AccountID && existing.InstrumentID == input.InstrumentID {
			return state, nil, nil, &domain.Error{Code: domain.ErrConflict, Field: "instrumentId", Message: "an active position already exists for this Account and Instrument"}
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
		CostBasisAvailable: false,
	}
	adjustment := domain.PositionAdjustmentInput{
		HouseholdID: input.HouseholdID, HoldingID: holding.ID, Quantity: input.Quantity,
		Added: true, UnitCost: input.UnitCost, EffectiveAt: input.EffectiveAt, Note: input.Note,
	}
	return state, adjustment, &holding, nil
}
