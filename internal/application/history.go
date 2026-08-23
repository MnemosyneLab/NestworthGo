package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// HistoryOrigin returns the immutable Starting point, if one has been
// confirmed. A nil result is a safe read-only pre-history state.
func (s *Service) HistoryOrigin(ctx context.Context) (*domain.HistoryOrigin, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if bootstrap.Household == nil {
		return nil, nil
	}
	return s.repository.HistoryOrigin(ctx, bootstrap.Household.ID)
}

func (s *Service) HistoryStarted(ctx context.Context) (bool, error) {
	origin, err := s.HistoryOrigin(ctx)
	return origin != nil, err
}

// StartHistory captures the selected v0.1.2 state as one immutable Starting
// point. The repository owns one transaction and returns an existing origin
// on an idempotent retry.
func (s *Service) StartHistory(ctx context.Context, timezone string) (domain.HistoryOrigin, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	if bootstrap.Household == nil {
		return domain.HistoryOrigin{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	now := s.now()
	origin, err := domain.NewHistoryOrigin(bootstrap.Household.ID, timezone, now, now)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	data := domain.HistoryOriginData{Origin: origin}
	for _, record := range snapshot.Accounts {
		account := record.Account
		data.AccountStates = append(data.AccountStates, domain.HistoryOriginAccountState{OriginID: origin.ID, AccountID: account.ID, ArchivedAt: account.ArchivedAt, IncludeInNetWorth: account.IncludeInNetWorth, IncludeInInvestment: account.IncludeInInvestment, IncludeInLiquidAssets: account.IncludeInLiquidAssets, CreatedAt: now})
		for _, share := range record.Ownership.Shares() {
			data.Ownership = append(data.Ownership, domain.HistoryOriginOwnership{OriginID: origin.ID, AccountID: account.ID, MemberID: share.MemberID, ShareBPS: share.ShareBPS})
		}
		if record.LatestValue != nil {
			value := record.LatestValue.Amount
			accountID := account.ID
			data.Components = append(data.Components, domain.HistoryOriginComponent{ID: domain.NewHistoryOriginComponentID(), OriginID: origin.ID, Kind: domain.HistoryOriginAccountValue, AccountID: &accountID, Amount: &value, CreatedAt: now})
		}
	}
	for _, value := range snapshot.CashValues {
		accountID := value.AccountID
		amount := value.Amount
		data.Components = append(data.Components, domain.HistoryOriginComponent{ID: domain.NewHistoryOriginComponentID(), OriginID: origin.ID, Kind: domain.HistoryOriginAccountCash, AccountID: &accountID, Amount: &amount, CreatedAt: now})
	}
	for _, holding := range snapshot.Holdings {
		accountID, holdingID, instrumentID, quantity := holding.AccountID, holding.ID, holding.InstrumentID, holding.Quantity
		data.Components = append(data.Components, domain.HistoryOriginComponent{ID: domain.NewHistoryOriginComponentID(), OriginID: origin.ID, Kind: domain.HistoryOriginHoldingQuantity, AccountID: &accountID, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity, CreatedAt: now})
	}
	for _, instrument := range snapshot.Instruments {
		data.InstrumentPreferences = append(data.InstrumentPreferences, domain.HistoryOriginInstrumentPreference{OriginID: origin.ID, InstrumentID: instrument.ID, SourceKind: instrument.QuoteSource, CreatedAt: now})
	}
	for _, preference := range snapshot.FXPreferences {
		data.FXPreferences = append(data.FXPreferences, domain.HistoryOriginFXPreference{OriginID: origin.ID, CurrencyA: preference.CurrencyA, CurrencyB: preference.CurrencyB, SourceKind: preference.SourceKind, CreatedAt: now})
	}
	return s.repository.StartHistory(ctx, data)
}
