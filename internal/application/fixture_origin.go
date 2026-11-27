package application

// Synthetic existing-history fixture construction. No normal desktop, mobile,
// or MCP entry point exposes these methods. They stage populated immutable
// origins for replay/accounting tests and synthetic QA seed commands, preserving
// the exact baseline financial facts of those fixtures.
//
// New ledgers use CompleteOnboarding, which atomically installs an empty origin.

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// CaptureSyntheticOrigin captures the selected current state as one immutable Starting
// point. The repository owns one transaction and returns an existing origin
// on an idempotent retry.
func (s *Service) CaptureSyntheticOrigin(ctx context.Context, timezone string) (domain.HistoryOrigin, error) {
	return s.CaptureSyntheticOriginWithCosts(ctx, timezone, nil)
}

func snapshotHasStartingAssets(snapshot domain.PortfolioSnapshot) bool {
	for _, record := range snapshot.Accounts {
		if record.LatestValue != nil && !record.LatestValue.Amount.IsZero() {
			return true
		}
	}
	for _, cash := range snapshot.CashValues {
		if !cash.Amount.IsZero() {
			return true
		}
	}
	for _, holding := range snapshot.Holdings {
		if !holding.Quantity.IsZero() {
			return true
		}
	}
	return false
}

// CaptureSyntheticOriginWithCosts captures the selected current state with optional
// per-Holding cost overrides. A missing override keeps the selected current
// quote as the default, preserving the synthetic fixture’s chosen opening facts.
func (s *Service) CaptureSyntheticOriginWithCosts(ctx context.Context, timezone string, costOverrides map[domain.HoldingID]string) (domain.HistoryOrigin, error) {
	return s.CaptureSyntheticOriginOnDate(ctx, timezone, "", costOverrides)
}

// CaptureSyntheticOriginOnDate allows a past empty starting point. Existing balances or
// positions can only be captured at the current instant, since they do not
// establish what was held on a date in the past.
func (s *Service) CaptureSyntheticOriginOnDate(ctx context.Context, timezone, startDate string, costOverrides map[domain.HoldingID]string) (domain.HistoryOrigin, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	defer unlock()
	data, err := s.prepareHistoryStart(ctx, timezone, startDate, costOverrides)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	return s.saveHistoryStart(ctx, data)
}

func (s *Service) prepareHistoryStart(ctx context.Context, timezone, startDate string, costOverrides map[domain.HoldingID]string) (domain.HistoryOriginData, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.HistoryOriginData{}, err
	}
	if bootstrap.Household == nil {
		return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	now := s.clock()
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.HistoryOriginData{}, err
	}
	startedAt, err := historyStartInstant(startDate, timezone, now)
	if err != nil {
		return domain.HistoryOriginData{}, err
	}
	if startedAt.Before(now) && snapshotHasStartingAssets(snapshot) {
		location, locationErr := time.LoadLocation(strings.TrimSpace(timezone))
		if locationErr != nil {
			return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Field: "timezone", Message: "timezone must be a valid IANA timezone"}
		}
		if strings.TrimSpace(startDate) != "" && strings.TrimSpace(startDate) == now.In(location).Format("2006-01-02") {
			startedAt = now
		} else {
			return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrConflict, Field: "startDate", Message: "a past start requires zero account balances, cash, and holdings"}
		}
	}
	origin, err := domain.NewHistoryOrigin(bootstrap.Household.ID, timezone, startedAt, now)
	if err != nil {
		return domain.HistoryOriginData{}, err
	}
	data := domain.HistoryOriginData{Origin: origin}
	instrumentByID := make(map[domain.InstrumentID]domain.Instrument, len(snapshot.Instruments))
	for _, instrument := range snapshot.Instruments {
		instrumentByID[instrument.ID] = instrument
	}
	for _, record := range snapshot.Accounts {
		account := record.Account
		data.AccountStates = append(data.AccountStates, domain.HistoryOriginAccountState{OriginID: origin.ID, AccountID: account.ID, ArchivedAt: account.ArchivedAt, IncludeInNetWorth: account.IncludeInNetWorth, IncludeInPortfolio: account.IncludeInPortfolio, IncludeInLiquidAssets: account.IncludeInLiquidAssets, CreatedAt: now})
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
				return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrIntegrity, Field: "instrumentId", Message: "Holding Instrument was not found"}
			}
			if override, ok := costOverrides[holding.ID]; ok {
				cost, err := domain.ParseUnitPrice(override)
				if err != nil {
					return domain.HistoryOriginData{}, err
				}
				unitCost = &cost
			} else {
				quote := selectInstrumentQuote(instrument, snapshot.InstrumentQuotes)
				if quote == nil {
					return domain.HistoryOriginData{}, &domain.Error{Code: domain.ErrCostBasisRequired, Field: "unitCost", Message: "a selected Instrument quote is required to start a positive Holding"}
				}
				unitCost = &quote.UnitPrice
			}
		}
		data.Components = append(data.Components, domain.HistoryOriginComponent{ID: domain.NewHistoryOriginComponentID(), OriginID: origin.ID, Kind: domain.HistoryOriginHoldingQuantity, AccountID: &accountID, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity, UnitCost: unitCost, CreatedAt: now})
	}
	for _, instrument := range snapshot.Instruments {
		data.InstrumentPreferences = append(data.InstrumentPreferences, domain.HistoryOriginInstrumentPreference{OriginID: origin.ID, InstrumentID: instrument.ID, SourceKind: instrument.QuoteSource, CreatedAt: now})
	}
	for _, preference := range snapshot.FXPreferences {
		data.FXPreferences = append(data.FXPreferences, domain.HistoryOriginFXPreference{OriginID: origin.ID, CurrencyA: preference.CurrencyA, CurrencyB: preference.CurrencyB, SourceKind: preference.SourceKind, CreatedAt: now})
	}
	return data, nil
}

func (s *Service) saveHistoryStart(ctx context.Context, data domain.HistoryOriginData) (domain.HistoryOrigin, error) {
	result, err := s.repository.CaptureSyntheticOrigin(ctx, data)
	if err == nil {
		s.invalidateAnalysis()
	}
	return result, err
}
