package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// loadChangeContext fetches the immutable Starting point plus one full
// portfolio snapshot so a request never reads the snapshot twice.
func (s *Service) loadChangeContext(ctx context.Context) (*domain.HistoryOrigin, domain.PortfolioSnapshot, error) {
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return nil, domain.PortfolioSnapshot{}, err
	}
	if origin == nil {
		return nil, domain.PortfolioSnapshot{}, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before recording a change"}
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, domain.PortfolioSnapshot{}, err
	}
	return origin, snapshot, nil
}

func (s *Service) changeState(ctx context.Context) (domain.ChangeState, error) {
	origin, snapshot, err := s.loadChangeContext(ctx)
	if err != nil {
		return domain.ChangeState{}, err
	}
	return s.changeStateFrom(origin, snapshot)
}

// changeStateFrom builds ChangeState from an already-loaded Starting point and
// portfolio snapshot.
func (s *Service) changeStateFrom(origin *domain.HistoryOrigin, snapshot domain.PortfolioSnapshot) (domain.ChangeState, error) {
	if origin == nil {
		return domain.ChangeState{}, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before recording a change"}
	}
	state := domain.ChangeState{HouseholdID: origin.HouseholdID, OriginAt: origin.StartedAt, Timezone: origin.Timezone, Now: s.clock(), Accounts: make(map[domain.AccountID]domain.ChangeAccountState), Cash: make(map[domain.AccountID]map[domain.CurrencyCode]domain.Money), Holdings: make(map[domain.HoldingID]domain.ChangeHoldingState), Instruments: make(map[domain.InstrumentID]domain.Instrument)}
	for _, record := range snapshot.Accounts {
		account := record.Account
		current, parseErr := domain.ParseMoney("0", account.DefaultCurrency)
		if parseErr != nil {
			return domain.ChangeState{}, parseErr
		}
		if record.LatestValue != nil {
			current = record.LatestValue.Amount
		}
		state.Accounts[account.ID] = domain.ChangeAccountState{ID: account.ID, Name: account.Name, Currency: account.DefaultCurrency, Mode: account.TrackingMode, Liability: account.PrimaryCategory.IsLiability(), Archived: account.ArchivedAt != nil, Current: current}
	}
	for _, value := range snapshot.CashValues {
		if state.Cash[value.AccountID] == nil {
			state.Cash[value.AccountID] = make(map[domain.CurrencyCode]domain.Money)
		}
		state.Cash[value.AccountID][value.Amount.Currency()] = value.Amount
	}
	instruments := make(map[domain.InstrumentID]domain.Instrument, len(snapshot.Instruments))
	for _, instrument := range snapshot.Instruments {
		instruments[instrument.ID] = instrument
	}
	state.Instruments = instruments
	for _, holding := range snapshot.Holdings {
		instrument := instruments[holding.InstrumentID]
		state.Holdings[holding.ID] = domain.ChangeHoldingState{ID: holding.ID, AccountID: holding.AccountID, InstrumentID: holding.InstrumentID, InstrumentName: instrument.Name, Currency: instrument.QuoteCurrency, Archived: holding.ArchivedAt != nil || instrument.ArchivedAt != nil || state.Accounts[holding.AccountID].Archived, Current: holding.Quantity, CostBasisAvailable: false}
	}
	return state, nil
}

func (s *Service) PreviewChange(ctx context.Context, command any) (domain.ChangePreview, error) {
	state, err := s.changeState(ctx)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	return domain.PreviewChange(state, command)
}

// RecordChange re-loads the current state before building and committing the
// effects. A previous preview is never accepted as write authorization.
func (s *Service) RecordChange(ctx context.Context, command any) (domain.ChangePreview, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	return s.recordChangeLocked(ctx, command)
}

// recordChangeLocked re-loads the current state before building and committing
// the effects; the caller must hold s.changeMu. A previous preview is never
// accepted as write authorization.
func (s *Service) recordChangeLocked(ctx context.Context, command any) (domain.ChangePreview, error) {
	origin, snapshot, err := s.loadChangeContext(ctx)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	state, err := s.changeStateFrom(origin, snapshot)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	if trade, ok := command.(domain.TradeInput); ok && trade.HoldingID == "" {
		if trade.Side != domain.TradeBuy {
			return domain.ChangePreview{}, &domain.Error{Code: domain.ErrInvalidTrade, Field: "holdingId", Message: "a Holding is required when selling"}
		}
		accountRecord, accountOK := accountFromSnapshot(snapshot, trade.SettlementAccountID)
		if !accountOK {
			return domain.ChangePreview{}, &domain.Error{Code: domain.ErrNotFound, Field: "settlementAccountId", Message: "Account was not found"}
		}
		instrument, instrumentOK := instrumentFromSnapshot(snapshot, trade.InstrumentID)
		if !instrumentOK {
			return domain.ChangePreview{}, &domain.Error{Code: domain.ErrNotFound, Field: "instrumentId", Message: "Instrument was not found"}
		}
		zero, zeroErr := domain.ParseQuantity("0")
		if zeroErr != nil {
			return domain.ChangePreview{}, zeroErr
		}
		holding, holdingErr := domain.NewHoldingForAccount(accountRecord.Account, instrument, zero, nil, 0, s.clock())
		if holdingErr != nil {
			return domain.ChangePreview{}, holdingErr
		}
		state.Holdings[holding.ID] = domain.ChangeHoldingState{ID: holding.ID, AccountID: holding.AccountID, InstrumentID: holding.InstrumentID, InstrumentName: instrument.Name, Currency: instrument.QuoteCurrency, Current: zero, CostBasisAvailable: false}
		trade.HoldingID = holding.ID
		preview, previewErr := domain.PreviewChange(state, trade)
		if previewErr != nil {
			return domain.ChangePreview{}, previewErr
		}
		if commitErr := s.repository.CreateHoldingWithActivity(ctx, holding, domain.ActivityCommit{Activity: preview.Activity, Effects: preview.Effects, Resulting: preview.Resulting}, s.clock()); commitErr != nil {
			return domain.ChangePreview{}, commitErr
		}
		return preview, nil
	}
	return s.commitChangeLocked(ctx, state, command)
}

// commitChangeLocked previews the command against the given state and commits
// it; the caller must hold s.changeMu.
func (s *Service) commitChangeLocked(ctx context.Context, state domain.ChangeState, command any) (domain.ChangePreview, error) {
	preview, err := domain.PreviewChange(state, command)
	if err != nil {
		return domain.ChangePreview{}, err
	}
	if err := s.repository.CommitActivity(ctx, preview.Activity, preview.Effects, preview.Resulting, s.clock()); err != nil {
		return domain.ChangePreview{}, err
	}
	return preview, nil
}

func (s *Service) CommitChange(ctx context.Context, command any) (domain.ChangePreview, error) {
	return s.RecordChange(ctx, command)
}

func (s *Service) HistoryMutationAllowed(ctx context.Context) error {
	started, err := s.HistoryStarted(ctx)
	if err != nil {
		return err
	}
	if !started {
		return &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before recording a change"}
	}
	return nil
}

func currentCashAmount(state domain.ChangeState, accountID domain.AccountID, currency domain.CurrencyCode) (domain.Money, error) {
	if values := state.Cash[accountID]; values != nil {
		if value, ok := values[currency]; ok {
			return value, nil
		}
	}
	return domain.ParseMoney("0", currency)
}

func differenceMoney(newValue, current domain.Money) (domain.Money, bool, error) {
	delta := newValue.Amount().Sub(current.Amount())
	if delta.IsZero() {
		return domain.Money{}, false, nil
	}
	value, err := domain.NewMoney(delta.Abs(), newValue.Currency())
	return value, !delta.IsNegative(), err
}
