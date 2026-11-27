package storage

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"strings"
	"time"
)

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

func fixtureStartInstant(startDate, timezone string, now time.Time) (time.Time, error) {
	if strings.TrimSpace(startDate) == "" {
		return now, nil
	}
	location, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		return time.Time{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Field: "timezone", Message: "timezone must be a valid IANA timezone"}
	}
	day := strings.TrimSpace(startDate)
	if len(day) != len("2006-01-02") || day > now.In(location).Format("2006-01-02") {
		return time.Time{}, &domain.Error{Code: domain.ErrValidation, Field: "startDate", Message: "start date must be a valid day no later than today"}
	}
	label, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return time.Time{}, &domain.Error{Code: domain.ErrValidation, Field: "startDate", Message: "start date must be a valid day"}
	}
	return domain.ResolveCivilDateStart(label, location)
}

// BuildOrigin captures explicitly supplied synthetic baseline facts. It does
// not read, modify or infer any actual user's ledger.
func BuildOrigin(household domain.HouseholdID, snapshot domain.PortfolioSnapshot, timezone, startDate string, now time.Time, costOverrides map[domain.HoldingID]string, selectQuote func(domain.Instrument, []domain.InstrumentQuote) *domain.InstrumentQuote) (domain.HistoryOriginData, error) {
	startedAt, err := fixtureStartInstant(startDate, timezone, now)
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
	origin, err := domain.NewHistoryOrigin(household, timezone, startedAt, now)
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
				quote := selectQuote(instrument, snapshot.InstrumentQuotes)
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
