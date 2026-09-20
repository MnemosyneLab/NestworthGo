package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *Service) rejectManagedHolding(ctx context.Context, holdingID domain.HoldingID) error {
	product, err := s.repository.ProductByHolding(ctx, holdingID)
	if err != nil {
		return err
	}
	if product != nil {
		return domain.ManagedPositionConflict("holdingId", "this holding is managed by a product contract; use the product operation")
	}
	return nil
}

func (s *Service) rejectManagedInstrument(ctx context.Context, instrumentID domain.InstrumentID) error {
	product, err := s.repository.ProductByInstrument(ctx, instrumentID)
	if err != nil {
		return err
	}
	if product != nil {
		return domain.ManagedPositionConflict("instrumentId", "this instrument is managed by a product contract; use product detail")
	}
	return nil
}

func (s *Service) rejectManagedCommand(ctx context.Context, command any) error {
	switch input := command.(type) {
	case domain.TradeInput:
		if input.HoldingID != "" {
			if err := s.rejectManagedHolding(ctx, input.HoldingID); err != nil {
				return err
			}
		}
		if input.InstrumentID != "" {
			return s.rejectManagedInstrument(ctx, input.InstrumentID)
		}
	case domain.PositionTransferInput:
		if err := s.rejectManagedHolding(ctx, input.FromHoldingID); err != nil {
			return err
		}
		return s.rejectManagedHolding(ctx, input.ToHoldingID)
	case domain.PositionAdjustmentInput:
		return s.rejectManagedHolding(ctx, input.HoldingID)
	case domain.CashDividendInput:
		return s.rejectManagedHolding(ctx, input.HoldingID)
	case domain.MoneyRemovedInput:
		if input.HoldingID != nil {
			return s.rejectManagedHolding(ctx, *input.HoldingID)
		}
	}
	return nil
}

func (s *Service) rejectArchiveAccountWithProducts(ctx context.Context, householdID domain.HouseholdID, accountID domain.AccountID) error {
	account := accountID
	products, err := s.repository.ListProducts(ctx, householdID, &account, false)
	if err != nil {
		return err
	}
	if len(products) > 0 {
		return domain.ManagedPositionConflict("accountId", "an account with open managed contracts cannot be archived")
	}
	return nil
}

func (s *Service) rejectManagedActivity(ctx context.Context, activityID domain.ActivityID) error {
	context, err := s.repository.ProductActivityContext(ctx, activityID)
	if err != nil {
		return err
	}
	if context != nil {
		return domain.ManagedPositionConflict("activityId", "use grouped product undo instead of reversing a child activity")
	}
	return nil
}
