package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// HistoricalReplay reconstructs a PortfolioSnapshot at a cutoff from the
// Starting point, immutable Activities, and observation facts. Rebuilds
// inject a batch so the replay does not re-query the same facts per day.
type HistoricalReplay struct {
	repository Repository
	batch      *domain.HistoricalSnapshotBatch
	quotes     *historicalQuoteCache
}

func (r HistoricalReplay) Snapshot(ctx context.Context, origin *domain.HistoryOrigin, cutoff time.Time) (domain.PortfolioSnapshot, error) {
	var snapshot domain.PortfolioSnapshot
	var originData domain.HistoryOriginData
	var accountObservations []domain.AccountStateObservation
	var instrumentObservations []domain.InstrumentPreferenceObservation
	var instrumentStateObservations []domain.InstrumentStateObservation
	var holdingStateObservations []domain.HoldingStateObservation
	var fxObservations []domain.FXPreferenceObservation
	var instrumentProviderBindings []domain.InstrumentProviderBindingRevision
	var activities []domain.Activity
	batch := r.batch
	var err error
	if batch != nil {
		snapshot = batch.Portfolio
		originData = batch.OriginData
		accountObservations = batch.AccountStateObservations
		instrumentObservations = batch.InstrumentPreferenceFacts
		instrumentProviderBindings = batch.InstrumentProviderBindingFacts
		instrumentStateObservations = batch.InstrumentStateObservations
		holdingStateObservations = batch.HoldingStateObservations
		fxObservations = batch.FXPreferenceFacts
		activities = batch.Activities
	} else {
		snapshot, err = r.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		originData, err = r.repository.HistoryOriginData(ctx, origin.ID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		accountObservations, err = r.repository.ListAccountStateObservations(ctx, origin.HouseholdID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		instrumentObservations, err = r.repository.ListInstrumentPreferenceObservations(ctx, origin.HouseholdID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		instrumentStateObservations, err = r.repository.ListInstrumentStateObservations(ctx, origin.HouseholdID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		holdingStateObservations, err = r.repository.ListHoldingStateObservations(ctx, origin.HouseholdID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		fxObservations, err = r.repository.ListFXPreferenceObservations(ctx, origin.HouseholdID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		activities, err = r.repository.ListActivitiesUntil(ctx, origin.HouseholdID, cutoff)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
	}
	if batch != nil {
		filtered := activities[:0]
		for _, activity := range activities {
			if !activity.EffectiveAt.After(cutoff) {
				filtered = append(filtered, activity)
			}
		}
		activities = filtered
	}
	values := make(map[domain.AccountID]domain.Money)
	cash := make(map[domain.AccountID]map[domain.CurrencyCode]domain.Money)
	quantities := make(map[domain.HoldingID]domain.Quantity)
	for _, component := range originData.Components {
		switch component.Kind {
		case domain.HistoryOriginAccountValue:
			if component.AccountID != nil && component.Amount != nil {
				values[*component.AccountID] = *component.Amount
			}
		case domain.HistoryOriginAccountCash:
			if component.AccountID != nil && component.Amount != nil {
				if cash[*component.AccountID] == nil {
					cash[*component.AccountID] = make(map[domain.CurrencyCode]domain.Money)
				}
				cash[*component.AccountID][component.Amount.Currency()] = *component.Amount
			}
		case domain.HistoryOriginHoldingQuantity:
			if component.HoldingID != nil && component.Quantity != nil {
				quantities[*component.HoldingID] = *component.Quantity
			}
		}
	}
	for _, activity := range activities {
		for _, effect := range activity.Effects {
			if effect.Money != nil && effect.AccountID != nil {
				if err := applyHistoricalMoney(values, cash, effect); err != nil {
					return domain.PortfolioSnapshot{}, err
				}
			}
			if effect.Quantity != nil && effect.HoldingID != nil {
				current := quantities[*effect.HoldingID]
				value := current.Decimal()
				if effect.Direction == domain.EffectAdded {
					value = value.Add(effect.Quantity.Decimal())
				} else {
					value = value.Sub(effect.Quantity.Decimal())
					if value.IsNegative() {
						return domain.PortfolioSnapshot{}, &domain.Error{Code: domain.ErrHistoryUpdateFailed, Field: "quantity", Message: "historical replay reached a negative quantity"}
					}
				}
				updated, updateErr := domain.NewQuantity(value)
				if updateErr != nil {
					return domain.PortfolioSnapshot{}, updateErr
				}
				quantities[*effect.HoldingID] = updated
			}
		}
	}

	originStates := make(map[domain.AccountID]domain.HistoryOriginAccountState, len(originData.AccountStates))
	originOwnership := make(map[domain.AccountID][]domain.OwnershipShare)
	originAccountIDs := make(map[domain.AccountID]struct{})
	originHoldingIDs := make(map[domain.HoldingID]struct{})
	originInstrumentIDs := make(map[domain.InstrumentID]struct{})
	for _, state := range originData.AccountStates {
		originStates[state.AccountID] = state
		originAccountIDs[state.AccountID] = struct{}{}
	}
	for _, share := range originData.Ownership {
		originOwnership[share.AccountID] = append(originOwnership[share.AccountID], domain.OwnershipShare{MemberID: share.MemberID, ShareBPS: share.ShareBPS})
	}
	for _, component := range originData.Components {
		if component.AccountID != nil {
			originAccountIDs[*component.AccountID] = struct{}{}
		}
		if component.HoldingID != nil {
			originHoldingIDs[*component.HoldingID] = struct{}{}
		}
		if component.InstrumentID != nil {
			originInstrumentIDs[*component.InstrumentID] = struct{}{}
		}
	}

	filteredAccounts := make([]domain.AccountRecord, 0, len(snapshot.Accounts))
	for _, record := range snapshot.Accounts {
		state, hasOriginState := originStates[record.Account.ID]
		_, wasPresentAtOrigin := originAccountIDs[record.Account.ID]
		if record.Account.CreatedAt.After(cutoff) && !wasPresentAtOrigin {
			continue
		}
		observation := latestAccountObservation(accountObservations, record.Account.ID, cutoff)
		if hasOriginState || observation != nil {
			if observation != nil {
				record.Account.IncludeInNetWorth = observation.IncludeInNetWorth
				record.Account.IncludeInPortfolio = observation.IncludeInPortfolio
				record.Account.IncludeInLiquidAssets = observation.IncludeInLiquidAssets
				record.Account.ArchivedAt = cloneTimePtr(observation.ArchivedAt)
				record.StateObservationID = &observation.ID
			} else {
				record.Account.IncludeInNetWorth = state.IncludeInNetWorth
				record.Account.IncludeInPortfolio = state.IncludeInPortfolio
				record.Account.IncludeInLiquidAssets = state.IncludeInLiquidAssets
				record.Account.ArchivedAt = cloneTimePtr(state.ArchivedAt)
			}
			if shares := accountOwnershipAtAsOf(originOwnership[record.Account.ID], record.Ownership.Shares(), observation); len(shares) > 0 {
				ownership, ownershipErr := domain.ParseOwnership(shares)
				if ownershipErr != nil {
					return domain.PortfolioSnapshot{}, ownershipErr
				}
				record.Ownership = ownership
			}
		} else if record.Account.ArchivedAt != nil && record.Account.ArchivedAt.After(cutoff) {
			record.Account.ArchivedAt = nil
		}
		if record.Account.ArchivedAt != nil && record.Account.ArchivedAt.After(cutoff) {
			record.Account.ArchivedAt = nil
		}
		if record.Account.TrackingMode != domain.TrackingHoldings {
			record.LatestValue = nil
			if value, ok := values[record.Account.ID]; ok {
				historical, valueErr := domain.NewAccountValue(record.Account, value, cutoff, cutoff)
				if valueErr != nil {
					return domain.PortfolioSnapshot{}, valueErr
				}
				record.LatestValue = &historical
			}
		}
		filteredAccounts = append(filteredAccounts, record)
	}
	snapshot.Accounts = filteredAccounts

	filteredInstruments := make([]domain.Instrument, 0, len(snapshot.Instruments))
	originInstrumentPreferences := make(map[domain.InstrumentID]domain.HistoryOriginInstrumentPreference, len(originData.InstrumentPreferences))
	for _, preference := range originData.InstrumentPreferences {
		originInstrumentPreferences[preference.InstrumentID] = preference
	}
	for _, instrument := range snapshot.Instruments {
		if instrument.CreatedAt.After(cutoff) {
			if _, wasPresentAtOrigin := originInstrumentIDs[instrument.ID]; !wasPresentAtOrigin {
				continue
			}
		}
		if observation := latestInstrumentStateObservation(instrumentStateObservations, instrument.ID, cutoff); observation != nil {
			instrument.ArchivedAt = cloneTimePtr(observation.ArchivedAt)
		} else if instrument.CreatedAt.After(origin.StartedAt) {
			instrument.ArchivedAt = nil
		} else if instrument.ArchivedAt != nil && instrument.ArchivedAt.After(cutoff) {
			instrument.ArchivedAt = nil
		}
		if baseline, ok := originInstrumentPreferences[instrument.ID]; ok {
			instrument.QuoteSource = baseline.SourceKind
		} else if instrument.CreatedAt.After(origin.StartedAt) {
			// A post-origin entity must never borrow today's mutable preference
			// when an older database has no creation baseline. Treat the missing
			// evidence conservatively as Manual rather than copying a later
			// Provider selection backward.
			instrument.QuoteSource = domain.QuoteSourceManual
		}
		if observation := latestInstrumentObservation(instrumentObservations, instrument.ID, cutoff); observation != nil {
			instrument.QuoteSource = observation.SourceKind
			instrument.PreferenceObservationID = &observation.ID
		} else {
			instrument.PreferenceObservationID = nil
		}
		if len(instrumentProviderBindings) > 0 && instrument.QuoteSource == domain.QuoteSourceProvider {
			binding := latestInstrumentProviderBindingRevision(instrumentProviderBindings, instrument.ID, cutoff)
			if binding == nil || !binding.Enabled {
				instrument.ProviderKey = nil
				instrument.ProviderSymbol = nil
				instrument.MarketCode = nil
				instrument.ProviderBindingRevision = 0
			} else {
				providerKey := binding.ProviderKey
				providerSymbol := binding.ProviderSymbol
				instrument.ProviderKey = &providerKey
				instrument.ProviderSymbol = &providerSymbol
				if binding.Market != "" {
					market := binding.Market
					instrument.MarketCode = &market
				} else {
					instrument.MarketCode = nil
				}
				instrument.ProviderBindingRevision = binding.BindingRevision
			}
		}
		filteredInstruments = append(filteredInstruments, instrument)
	}
	snapshot.Instruments = filteredInstruments
	instrumentIDs := make(map[domain.InstrumentID]struct{}, len(snapshot.Instruments))
	for _, instrument := range snapshot.Instruments {
		instrumentIDs[instrument.ID] = struct{}{}
	}

	filteredHoldings := make([]domain.Holding, 0, len(snapshot.Holdings))
	accountIDs := make(map[domain.AccountID]struct{}, len(snapshot.Accounts))
	for _, record := range snapshot.Accounts {
		accountIDs[record.Account.ID] = struct{}{}
	}
	for _, holding := range snapshot.Holdings {
		if holding.CreatedAt.After(cutoff) {
			if _, wasPresentAtOrigin := originHoldingIDs[holding.ID]; !wasPresentAtOrigin {
				continue
			}
		}
		if _, ok := accountIDs[holding.AccountID]; !ok {
			continue
		}
		if _, ok := instrumentIDs[holding.InstrumentID]; !ok {
			continue
		}
		if observation := latestHoldingStateObservation(holdingStateObservations, holding.ID, cutoff); observation != nil {
			holding.ArchivedAt = cloneTimePtr(observation.ArchivedAt)
		} else if holding.CreatedAt.After(origin.StartedAt) {
			holding.ArchivedAt = nil
		} else if holding.ArchivedAt != nil && holding.ArchivedAt.After(cutoff) {
			holding.ArchivedAt = nil
		}
		quantity := quantities[holding.ID]
		holding = holding.ReplaceQuantity(quantity, cutoff)
		filteredHoldings = append(filteredHoldings, holding)
	}
	snapshot.Holdings = filteredHoldings
	snapshot.CashValues = nil
	snapshot.InstrumentQuotes = nil
	snapshot.FXQuotes = nil
	snapshot.FXPreferences = nil
	for accountID, currencies := range cash {
		for _, value := range currencies {
			account, ok := accountRecordByID(snapshot.Accounts, accountID)
			if !ok {
				continue
			}
			cashValue, valueErr := domain.NewAccountCashValue(account.Account, value, cutoff, cutoff)
			if valueErr != nil {
				return domain.PortfolioSnapshot{}, valueErr
			}
			snapshot.CashValues = append(snapshot.CashValues, cashValue)
		}
	}
	quoteCache := r.quotes
	for _, instrument := range snapshot.Instruments {
		var quotes []domain.InstrumentQuote
		var quoteErr error
		if quoteCache != nil {
			quotes = quoteCache.instrumentQuotes[instrument.ID]
		} else {
			quotes, quoteErr = r.repository.ListInstrumentQuotes(ctx, instrument.ID)
		}
		if quoteErr != nil {
			return domain.PortfolioSnapshot{}, quoteErr
		}
		for _, quote := range quotes {
			// A correction may be fetched after the household day closed while
			// its economic effective time is historical. Keep all immutable facts
			// here; the historical valuation resolver applies the cutoff to
			// ValueEffectiveAt and rejects realtime/legacy observations.
			snapshot.InstrumentQuotes = append(snapshot.InstrumentQuotes, quote)
		}
	}
	var fxQuotes []domain.FXQuote
	if quoteCache != nil {
		fxQuotes = quoteCache.fxQuotes
	} else {
		fxQuotes, err = r.repository.ListFXQuotes(ctx, origin.HouseholdID)
	}
	if err != nil {
		return domain.PortfolioSnapshot{}, err
	}
	for _, quote := range fxQuotes {
		snapshot.FXQuotes = append(snapshot.FXQuotes, quote)
	}
	preferences := snapshot.FXPreferences
	if batch != nil {
		preferences = batch.FXPreferences
	} else {
		preferences, err = r.repository.ListFXPreferences(ctx, origin.HouseholdID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
	}
	originFXPreferences := make(map[string]domain.HistoryOriginFXPreference, len(originData.FXPreferences))
	for _, preference := range originData.FXPreferences {
		originFXPreferences[fxPreferenceKey(preference.CurrencyA, preference.CurrencyB)] = preference
	}
	currentFXPreferences := make(map[string]domain.FXPreference, len(preferences))
	for _, preference := range preferences {
		currentFXPreferences[fxPreferenceKey(preference.CurrencyA, preference.CurrencyB)] = preference
	}
	for key, baseline := range originFXPreferences {
		preference, ok := currentFXPreferences[key]
		if !ok {
			preference, err = domain.NewFXPreference(origin.HouseholdID, baseline.CurrencyA, baseline.CurrencyB, baseline.SourceKind, baseline.CreatedAt)
			if err != nil {
				return domain.PortfolioSnapshot{}, err
			}
		}
		preference.SourceKind = baseline.SourceKind
		if observation := latestFXObservation(fxObservations, baseline.CurrencyA, baseline.CurrencyB, cutoff); observation != nil {
			preference.SourceKind = observation.SourceKind
			preference.ObservationID = &observation.ID
		}
		snapshot.FXPreferences = append(snapshot.FXPreferences, preference)
		delete(currentFXPreferences, key)
	}
	for _, preference := range currentFXPreferences {
		observation := latestFXObservation(fxObservations, preference.CurrencyA, preference.CurrencyB, cutoff)
		if preference.CreatedAt.After(cutoff) && observation == nil {
			continue
		}
		if observation != nil {
			preference.SourceKind = observation.SourceKind
			preference.ObservationID = &observation.ID
		}
		snapshot.FXPreferences = append(snapshot.FXPreferences, preference)
	}
	return snapshot, nil
}

func latestAccountObservation(observations []domain.AccountStateObservation, accountID domain.AccountID, cutoff time.Time) *domain.AccountStateObservation {
	return latestObservation(observations, cutoff,
		func(observation *domain.AccountStateObservation) bool { return observation.AccountID == accountID },
		func(observation *domain.AccountStateObservation) time.Time { return observation.EffectiveAt },
		func(observation *domain.AccountStateObservation) time.Time { return observation.CreatedAt },
		func(observation *domain.AccountStateObservation) string { return observation.ID.String() },
	)
}

func latestInstrumentObservation(observations []domain.InstrumentPreferenceObservation, instrumentID domain.InstrumentID, cutoff time.Time) *domain.InstrumentPreferenceObservation {
	return latestObservation(observations, cutoff,
		func(observation *domain.InstrumentPreferenceObservation) bool {
			return observation.InstrumentID == instrumentID
		},
		func(observation *domain.InstrumentPreferenceObservation) time.Time { return observation.EffectiveAt },
		func(observation *domain.InstrumentPreferenceObservation) time.Time { return observation.CreatedAt },
		func(observation *domain.InstrumentPreferenceObservation) string { return observation.ID.String() },
	)
}

func latestInstrumentProviderBindingRevision(bindings []domain.InstrumentProviderBindingRevision, instrumentID domain.InstrumentID, cutoff time.Time) *domain.InstrumentProviderBindingRevision {
	var selected *domain.InstrumentProviderBindingRevision
	for index := range bindings {
		candidate := &bindings[index]
		if candidate.InstrumentID != instrumentID || candidate.EffectiveFrom.After(cutoff) {
			continue
		}
		if selected == nil || instrumentProviderBindingLater(*candidate, *selected) {
			selected = candidate
		}
	}
	return selected
}

func instrumentProviderBindingLater(candidate, selected domain.InstrumentProviderBindingRevision) bool {
	if !candidate.EffectiveFrom.Equal(selected.EffectiveFrom) {
		return candidate.EffectiveFrom.After(selected.EffectiveFrom)
	}
	if !candidate.CreatedAt.Equal(selected.CreatedAt) {
		return candidate.CreatedAt.After(selected.CreatedAt)
	}
	if candidate.BindingRevision != selected.BindingRevision {
		return candidate.BindingRevision > selected.BindingRevision
	}
	return candidate.ProviderKey > selected.ProviderKey
}

func latestInstrumentStateObservation(observations []domain.InstrumentStateObservation, instrumentID domain.InstrumentID, cutoff time.Time) *domain.InstrumentStateObservation {
	return latestObservation(observations, cutoff,
		func(observation *domain.InstrumentStateObservation) bool {
			return observation.InstrumentID == instrumentID
		},
		func(observation *domain.InstrumentStateObservation) time.Time { return observation.EffectiveAt },
		func(observation *domain.InstrumentStateObservation) time.Time { return observation.CreatedAt },
		func(observation *domain.InstrumentStateObservation) string { return observation.ID.String() },
	)
}

func latestHoldingStateObservation(observations []domain.HoldingStateObservation, holdingID domain.HoldingID, cutoff time.Time) *domain.HoldingStateObservation {
	return latestObservation(observations, cutoff,
		func(observation *domain.HoldingStateObservation) bool { return observation.HoldingID == holdingID },
		func(observation *domain.HoldingStateObservation) time.Time { return observation.EffectiveAt },
		func(observation *domain.HoldingStateObservation) time.Time { return observation.CreatedAt },
		func(observation *domain.HoldingStateObservation) string { return observation.ID.String() },
	)
}

func latestFXObservation(observations []domain.FXPreferenceObservation, currencyA, currencyB domain.CurrencyCode, cutoff time.Time) *domain.FXPreferenceObservation {
	return latestObservation(observations, cutoff,
		func(observation *domain.FXPreferenceObservation) bool {
			return observation.CurrencyA == currencyA && observation.CurrencyB == currencyB
		},
		func(observation *domain.FXPreferenceObservation) time.Time { return observation.EffectiveAt },
		func(observation *domain.FXPreferenceObservation) time.Time { return observation.CreatedAt },
		func(observation *domain.FXPreferenceObservation) string { return observation.ID.String() },
	)
}

func latestObservation[T any](
	items []T,
	cutoff time.Time,
	match func(*T) bool,
	effectiveAt func(*T) time.Time,
	createdAt func(*T) time.Time,
	id func(*T) string,
) *T {
	var selected *T
	for index := range items {
		item := &items[index]
		if !match(item) || effectiveAt(item).After(cutoff) {
			continue
		}
		if selected == nil || effectiveObservationLater(effectiveAt(item), createdAt(item), id(item), effectiveAt(selected), createdAt(selected), id(selected)) {
			selected = item
		}
	}
	return selected
}

func effectiveObservationLater(effectiveAt, createdAt time.Time, id string, otherEffectiveAt, otherCreatedAt time.Time, otherID string) bool {
	if !effectiveAt.Equal(otherEffectiveAt) {
		return effectiveAt.After(otherEffectiveAt)
	}
	if !createdAt.Equal(otherCreatedAt) {
		return createdAt.After(otherCreatedAt)
	}
	return id > otherID
}

func accountOwnershipAtAsOf(baseline, current []domain.OwnershipShare, observation *domain.AccountStateObservation) []domain.OwnershipShare {
	if observation != nil && len(observation.Ownership) > 0 {
		return observation.Ownership
	}
	if len(baseline) > 0 {
		return baseline
	}
	return current
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func fxPreferenceKey(currencyA, currencyB domain.CurrencyCode) string {
	return currencyA.String() + "|" + currencyB.String()
}

func applyHistoricalMoney(values map[domain.AccountID]domain.Money, cash map[domain.AccountID]map[domain.CurrencyCode]domain.Money, effect domain.ActivityEffect) error {
	amount := *effect.Money
	if effect.Target == domain.EffectTargetAccountValue {
		current := values[*effect.AccountID]
		return updateHistoricalMoney(values, *effect.AccountID, current, amount, effect.Direction)
	}
	if cash[*effect.AccountID] == nil {
		cash[*effect.AccountID] = make(map[domain.CurrencyCode]domain.Money)
	}
	current := cash[*effect.AccountID][amount.Currency()]
	return updateHistoricalMoney(cash[*effect.AccountID], amount.Currency(), current, amount, effect.Direction)
}

func updateHistoricalMoney[T comparable](values map[T]domain.Money, key T, current, amount domain.Money, direction domain.EffectDirection) error {
	value := current.Amount()
	if direction == domain.EffectAdded {
		value = value.Add(amount.Amount())
	} else {
		value = value.Sub(amount.Amount())
		if value.IsNegative() {
			return &domain.Error{Code: domain.ErrHistoryUpdateFailed, Field: "amount", Message: "historical replay reached a negative balance"}
		}
	}
	updated, err := domain.NewMoney(value, amount.Currency())
	if err != nil {
		return err
	}
	values[key] = updated
	return nil
}

func accountRecordByID(records []domain.AccountRecord, id domain.AccountID) (domain.AccountRecord, bool) {
	for _, record := range records {
		if record.Account.ID == id {
			return record, true
		}
	}
	return domain.AccountRecord{}, false
}
