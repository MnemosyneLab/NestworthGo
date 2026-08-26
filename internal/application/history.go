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

// StartHistory captures the selected current state as one immutable Starting
// point. The repository owns one transaction and returns an existing origin
// on an idempotent retry.
func (s *Service) StartHistory(ctx context.Context, timezone string) (domain.HistoryOrigin, error) {
	return s.StartHistoryWithCosts(ctx, timezone, nil)
}

// StartHistoryWithCosts captures the selected current state with optional
// per-Holding cost overrides. A missing override keeps the selected current
// quote as the default, preserving the original StartHistory contract.
func (s *Service) StartHistoryWithCosts(ctx context.Context, timezone string, costOverrides map[domain.HoldingID]string) (domain.HistoryOrigin, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	if bootstrap.Household == nil {
		return domain.HistoryOrigin{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	now := s.clock()
	origin, err := domain.NewHistoryOrigin(bootstrap.Household.ID, timezone, now, now)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	data := domain.HistoryOriginData{Origin: origin}
	instrumentByID := make(map[domain.InstrumentID]domain.Instrument, len(snapshot.Instruments))
	for _, instrument := range snapshot.Instruments {
		instrumentByID[instrument.ID] = instrument
	}
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
		var unitCost *domain.UnitPrice
		if !quantity.IsZero() {
			instrument, ok := instrumentByID[instrumentID]
			if !ok {
				return domain.HistoryOrigin{}, &domain.Error{Code: domain.ErrIntegrity, Field: "instrumentId", Message: "Holding Instrument was not found"}
			}
			quote := selectInstrumentQuote(instrument, snapshot.InstrumentQuotes)
			if quote == nil {
				return domain.HistoryOrigin{}, &domain.Error{Code: domain.ErrCostBasisRequired, Field: "unitCost", Message: "a selected Instrument quote is required to start a positive Holding"}
			}
			cost := quote.UnitPrice
			if override, ok := costOverrides[holding.ID]; ok {
				parsed, parseErr := domain.ParseUnitPrice(override)
				if parseErr != nil {
					return domain.HistoryOrigin{}, parseErr
				}
				cost = parsed
			}
			unitCost = &cost
		}
		data.Components = append(data.Components, domain.HistoryOriginComponent{ID: domain.NewHistoryOriginComponentID(), OriginID: origin.ID, Kind: domain.HistoryOriginHoldingQuantity, AccountID: &accountID, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity, UnitCost: unitCost, CreatedAt: now})
	}
	for _, instrument := range snapshot.Instruments {
		data.InstrumentPreferences = append(data.InstrumentPreferences, domain.HistoryOriginInstrumentPreference{OriginID: origin.ID, InstrumentID: instrument.ID, SourceKind: instrument.QuoteSource, CreatedAt: now})
	}
	for _, preference := range snapshot.FXPreferences {
		data.FXPreferences = append(data.FXPreferences, domain.HistoryOriginFXPreference{OriginID: origin.ID, CurrencyA: preference.CurrencyA, CurrencyB: preference.CurrencyB, SourceKind: preference.SourceKind, CreatedAt: now})
	}
	return s.repository.StartHistory(ctx, data)
}

// StartingPointDraft returns positive active Holdings and their selected
// current quote defaults in one portfolio read for the Starting Point form.
func (s *Service) StartingPointDraft(ctx context.Context) ([]domain.StartingPointHoldingView, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if bootstrap.Household == nil {
		return []domain.StartingPointHoldingView{}, nil
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	instruments := make(map[domain.InstrumentID]domain.Instrument, len(snapshot.Instruments))
	for _, instrument := range snapshot.Instruments {
		instruments[instrument.ID] = instrument
	}
	draft := make([]domain.StartingPointHoldingView, 0)
	for _, holding := range snapshot.Holdings {
		if holding.ArchivedAt != nil || holding.Quantity.IsZero() {
			continue
		}
		instrument, ok := instruments[holding.InstrumentID]
		if !ok || instrument.ArchivedAt != nil {
			continue
		}
		unitCost := ""
		if quote := selectInstrumentQuote(instrument, snapshot.InstrumentQuotes); quote != nil {
			unitCost = quote.UnitPrice.Canonical()
		}
		draft = append(draft, domain.StartingPointHoldingView{HoldingID: holding.ID, InstrumentID: holding.InstrumentID, InstrumentName: instrument.Name, Currency: instrument.QuoteCurrency, Quantity: holding.Quantity.Canonical(), UnitCost: unitCost})
	}
	return draft, nil
}
