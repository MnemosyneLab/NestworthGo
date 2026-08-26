package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historicalQuoteCacheKey struct{}
type historicalSnapshotBatchKey struct{}

type historicalQuoteCache struct {
	instrumentQuotes map[domain.InstrumentID][]domain.InstrumentQuote
	fxQuotes         []domain.FXQuote
}

// BuildDailyValuationSnapshot reconstructs a closed local day from the
// Starting point plus immutable Activities, then evaluates it with only quote
// observations at or before that day's cutoff.
func (s *Service) BuildDailyValuationSnapshot(ctx context.Context, localDate string) (domain.DailyValuationSnapshot, bool, error) {
	var origin *domain.HistoryOrigin
	var err error
	if batch, ok := ctx.Value(historicalSnapshotBatchKey{}).(*domain.HistoricalSnapshotBatch); ok && batch != nil {
		origin = &batch.Origin
	} else {
		origin, err = s.HistoryOrigin(ctx)
		if err != nil {
			return domain.DailyValuationSnapshot{}, false, err
		}
		if origin == nil {
			return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before building a snapshot"}
		}
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	parsedDate, err := time.ParseInLocation("2006-01-02", localDate, location)
	if err != nil || parsedDate.Format("2006-01-02") != localDate {
		return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrValidation, Field: "localDate", Message: "local date must use YYYY-MM-DD"}
	}
	if localDate >= s.clock().In(location).Format("2006-01-02") {
		return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "localDate", Message: "today is not a closed day"}
	}
	originDate := origin.StartedAt.In(location).Format("2006-01-02")
	if localDate < originDate {
		return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "localDate", Message: "snapshot date precedes the Starting point"}
	}
	nextLocal := parsedDate.AddDate(0, 0, 1).Format("2006-01-02")
	nextMidnight, err := domain.ResolveLocalDateTime(nextLocal, "00:00", origin.Timezone)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	cutoff := nextMidnight.Add(-time.Millisecond)
	portfolio, err := s.historicalPortfolioSnapshot(ctx, origin, cutoff)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	// PortfolioSnapshot intentionally reports investment totals only. For a
	// daily balance-sheet snapshot, value every account and sign liabilities.
	valuedAccounts, missing, err := NewValuationService(s.repository, func() time.Time { return cutoff }).ValueAccounts(portfolio)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	assets, liabilities := decimal.Zero, decimal.Zero
	complete := true
	items := make([]domain.DailyValuationSnapshotItem, 0)
	for _, account := range valuedAccounts {
		if !account.Complete {
			complete = false
		}
		if account.BaseValue != nil {
			amount, parseErr := decimal.NewFromString(account.BaseValue.Amount)
			if parseErr != nil {
				return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrIntegrity, Message: "historical base amount is invalid"}
			}
			if account.Account.PrimaryCategory.IsLiability() {
				liabilities = liabilities.Add(amount)
			} else {
				assets = assets.Add(amount)
			}
		}
		for _, component := range account.Components {
			item := domain.DailyValuationSnapshotItem{ID: domain.NewDailyValuationSnapshotItemID(), AccountID: account.Account.ID, HoldingID: component.HoldingID, NativeAmount: component.NativeAmount, NativeCurrency: component.NativeCurrency, Complete: component.Available, InstrumentID: component.InstrumentID, StateObservationID: component.StateObservationID, PreferenceObservationID: component.PreferenceObservationID, FXPreferenceObservationID: component.FXPreferenceObservationID}
			if component.PriceEvidence != nil && component.PriceEvidence.ObservationID != "" {
				quoteID := component.PriceEvidence.ObservationID
				item.QuoteID = &quoteID
			}
			if component.FXEvidence != nil && component.FXEvidence.ObservationID != "" {
				fxQuoteID := component.FXEvidence.ObservationID
				item.FXQuoteID = &fxQuoteID
			}
			if component.BaseAmount != nil {
				base, parseErr := domain.ParseMoney(component.BaseAmount.Amount, component.BaseAmount.Currency)
				if parseErr != nil {
					return domain.DailyValuationSnapshot{}, false, parseErr
				}
				item.BaseAmount = &base
			}
			if !component.Available {
				reason := missingReason(component, missing)
				item.MissingReason = &reason
			}
			items = append(items, item)
		}
	}
	assetsMoney, err := domain.NewMoney(assets, portfolio.Household.BaseCurrency)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	liabilitiesMoney, err := domain.NewMoney(liabilities, portfolio.Household.BaseCurrency)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	netWorthMoney, err := domain.NewMoney(assets.Sub(liabilities), portfolio.Household.BaseCurrency)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	hash := snapshotContentHash(localDate, cutoff, assetsMoney, liabilitiesMoney, netWorthMoney, items)
	snapshot := domain.DailyValuationSnapshot{ID: domain.NewDailyValuationSnapshotID(), HouseholdID: portfolio.Household.ID, LocalDate: localDate, CutoffAt: cutoff, ContentHash: hash, AssetsAmount: &assetsMoney, LiabilitiesAmount: &liabilitiesMoney, NetWorthAmount: &netWorthMoney, Currency: portfolio.Household.BaseCurrency, Complete: complete && len(missing) == 0, ComponentCount: len(items), MissingCount: len(missing), GenerationReason: "manual", CreatedAt: s.clock(), Items: items}
	appended, err := s.repository.SaveDailyValuationSnapshotAndMarkCompleted(ctx, snapshot, s.clock())
	return snapshot, appended, err
}

func (s *Service) historicalPortfolioSnapshot(ctx context.Context, origin *domain.HistoryOrigin, cutoff time.Time) (domain.PortfolioSnapshot, error) {
	var snapshot domain.PortfolioSnapshot
	var originData domain.HistoryOriginData
	var accountObservations []domain.AccountStateObservation
	var instrumentObservations []domain.InstrumentPreferenceObservation
	var instrumentStateObservations []domain.InstrumentStateObservation
	var holdingStateObservations []domain.HoldingStateObservation
	var fxObservations []domain.FXPreferenceObservation
	var activities []domain.Activity
	var batch *domain.HistoricalSnapshotBatch
	if value, ok := ctx.Value(historicalSnapshotBatchKey{}).(*domain.HistoricalSnapshotBatch); ok {
		batch = value
	}
	var err error
	if batch != nil {
		snapshot = batch.Portfolio
		originData = batch.OriginData
		accountObservations = batch.AccountStateObservations
		instrumentObservations = batch.InstrumentPreferenceFacts
		instrumentStateObservations = batch.InstrumentStateObservations
		holdingStateObservations = batch.HoldingStateObservations
		fxObservations = batch.FXPreferenceFacts
		activities = batch.Activities
	} else {
		snapshot, err = s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		originData, err = s.repository.HistoryOriginData(ctx, origin.ID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		accountObservations, err = s.repository.ListAccountStateObservations(ctx, origin.HouseholdID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		instrumentObservations, err = s.repository.ListInstrumentPreferenceObservations(ctx, origin.HouseholdID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		instrumentStateObservations, err = s.repository.ListInstrumentStateObservations(ctx, origin.HouseholdID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		holdingStateObservations, err = s.repository.ListHoldingStateObservations(ctx, origin.HouseholdID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		fxObservations, err = s.repository.ListFXPreferenceObservations(ctx, origin.HouseholdID)
		if err != nil {
			return domain.PortfolioSnapshot{}, err
		}
		activities, err = s.repository.ListActivitiesUntil(ctx, origin.HouseholdID, cutoff)
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
				record.Account.IncludeInInvestment = observation.IncludeInInvestment
				record.Account.IncludeInLiquidAssets = observation.IncludeInLiquidAssets
				record.Account.ArchivedAt = cloneTimePtr(observation.ArchivedAt)
				record.StateObservationID = &observation.ID
			} else {
				record.Account.IncludeInNetWorth = state.IncludeInNetWorth
				record.Account.IncludeInInvestment = state.IncludeInInvestment
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
	quoteCache, _ := ctx.Value(historicalQuoteCacheKey{}).(*historicalQuoteCache)
	for _, instrument := range snapshot.Instruments {
		var quotes []domain.InstrumentQuote
		var quoteErr error
		if quoteCache != nil {
			quotes = quoteCache.instrumentQuotes[instrument.ID]
		} else {
			quotes, quoteErr = s.repository.ListInstrumentQuotes(ctx, instrument.ID)
		}
		if quoteErr != nil {
			return domain.PortfolioSnapshot{}, quoteErr
		}
		for _, quote := range quotes {
			if !quote.QuotedAt.After(cutoff) {
				snapshot.InstrumentQuotes = append(snapshot.InstrumentQuotes, quote)
			}
		}
	}
	var fxQuotes []domain.FXQuote
	if quoteCache != nil {
		fxQuotes = quoteCache.fxQuotes
	} else {
		fxQuotes, err = s.repository.ListFXQuotes(ctx, origin.HouseholdID)
	}
	if err != nil {
		return domain.PortfolioSnapshot{}, err
	}
	for _, quote := range fxQuotes {
		if !quote.QuotedAt.After(cutoff) {
			snapshot.FXQuotes = append(snapshot.FXQuotes, quote)
		}
	}
	preferences := snapshot.FXPreferences
	if batch != nil {
		preferences = batch.FXPreferences
	} else {
		preferences, err = s.repository.ListFXPreferences(ctx, origin.HouseholdID)
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
	var selected *domain.AccountStateObservation
	for index := range observations {
		observation := &observations[index]
		if observation.AccountID != accountID || observation.EffectiveAt.After(cutoff) {
			continue
		}
		if selected == nil || effectiveObservationLater(observation.EffectiveAt, observation.CreatedAt, observation.ID.String(), selected.EffectiveAt, selected.CreatedAt, selected.ID.String()) {
			selected = observation
		}
	}
	return selected
}

func latestInstrumentObservation(observations []domain.InstrumentPreferenceObservation, instrumentID domain.InstrumentID, cutoff time.Time) *domain.InstrumentPreferenceObservation {
	var selected *domain.InstrumentPreferenceObservation
	for index := range observations {
		observation := &observations[index]
		if observation.InstrumentID != instrumentID || observation.EffectiveAt.After(cutoff) {
			continue
		}
		if selected == nil || effectiveObservationLater(observation.EffectiveAt, observation.CreatedAt, observation.ID.String(), selected.EffectiveAt, selected.CreatedAt, selected.ID.String()) {
			selected = observation
		}
	}
	return selected
}

func latestInstrumentStateObservation(observations []domain.InstrumentStateObservation, instrumentID domain.InstrumentID, cutoff time.Time) *domain.InstrumentStateObservation {
	var selected *domain.InstrumentStateObservation
	for index := range observations {
		observation := &observations[index]
		if observation.InstrumentID != instrumentID || observation.EffectiveAt.After(cutoff) {
			continue
		}
		if selected == nil || effectiveObservationLater(observation.EffectiveAt, observation.CreatedAt, observation.ID.String(), selected.EffectiveAt, selected.CreatedAt, selected.ID.String()) {
			selected = observation
		}
	}
	return selected
}

func latestHoldingStateObservation(observations []domain.HoldingStateObservation, holdingID domain.HoldingID, cutoff time.Time) *domain.HoldingStateObservation {
	var selected *domain.HoldingStateObservation
	for index := range observations {
		observation := &observations[index]
		if observation.HoldingID != holdingID || observation.EffectiveAt.After(cutoff) {
			continue
		}
		if selected == nil || effectiveObservationLater(observation.EffectiveAt, observation.CreatedAt, observation.ID.String(), selected.EffectiveAt, selected.CreatedAt, selected.ID.String()) {
			selected = observation
		}
	}
	return selected
}

func latestFXObservation(observations []domain.FXPreferenceObservation, currencyA, currencyB domain.CurrencyCode, cutoff time.Time) *domain.FXPreferenceObservation {
	var selected *domain.FXPreferenceObservation
	for index := range observations {
		observation := &observations[index]
		if observation.CurrencyA != currencyA || observation.CurrencyB != currencyB || observation.EffectiveAt.After(cutoff) {
			continue
		}
		if selected == nil || effectiveObservationLater(observation.EffectiveAt, observation.CreatedAt, observation.ID.String(), selected.EffectiveAt, selected.CreatedAt, selected.ID.String()) {
			selected = observation
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

func snapshotContentHash(localDate string, cutoff time.Time, assets, liabilities, netWorth domain.Money, items []domain.DailyValuationSnapshotItem) string {
	sort.Slice(items, func(i, j int) bool {
		return snapshotItemSortKey(items[i]) < snapshotItemSortKey(items[j])
	})
	hash := sha256.New()
	fmt.Fprintf(hash, "%s|%s|%s|%s|%s|%s", localDate, cutoff.UTC().Format(time.RFC3339Nano), assets.CanonicalAmount(), liabilities.CanonicalAmount(), netWorth.CanonicalAmount(), assets.Currency().String())
	for _, item := range items {
		fmt.Fprintf(hash, "|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%t|%s", item.AccountID.String(), snapshotItemHoldingIDString(item), snapshotItemInstrumentIDString(item), snapshotItemStateObservationIDString(item), snapshotItemPreferenceObservationIDString(item), snapshotItemFXPreferenceObservationIDString(item), item.NativeAmount, item.NativeCurrency.String(), snapshotItemBaseAmountString(item), snapshotItemQuoteIDString(item), snapshotItemFXQuoteIDString(item), item.Complete, snapshotItemMissingReasonString(item))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func snapshotItemSortKey(item domain.DailyValuationSnapshotItem) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%t|%s", item.AccountID.String(), snapshotItemHoldingIDString(item), snapshotItemInstrumentIDString(item), snapshotItemStateObservationIDString(item), snapshotItemPreferenceObservationIDString(item), snapshotItemFXPreferenceObservationIDString(item), item.NativeAmount, item.NativeCurrency.String(), snapshotItemBaseAmountString(item), snapshotItemQuoteIDString(item), snapshotItemFXQuoteIDString(item), item.Complete, snapshotItemMissingReasonString(item))
}

func snapshotItemHoldingIDString(item domain.DailyValuationSnapshotItem) string {
	if item.HoldingID == nil {
		return ""
	}
	return item.HoldingID.String()
}

func snapshotItemInstrumentIDString(item domain.DailyValuationSnapshotItem) string {
	if item.InstrumentID == nil {
		return ""
	}
	return item.InstrumentID.String()
}

func snapshotItemQuoteIDString(item domain.DailyValuationSnapshotItem) string {
	if item.QuoteID == nil {
		return ""
	}
	return *item.QuoteID
}

func snapshotItemStateObservationIDString(item domain.DailyValuationSnapshotItem) string {
	if item.StateObservationID == nil {
		return ""
	}
	return item.StateObservationID.String()
}

func snapshotItemPreferenceObservationIDString(item domain.DailyValuationSnapshotItem) string {
	if item.PreferenceObservationID == nil {
		return ""
	}
	return item.PreferenceObservationID.String()
}

func snapshotItemFXQuoteIDString(item domain.DailyValuationSnapshotItem) string {
	if item.FXQuoteID == nil {
		return ""
	}
	return *item.FXQuoteID
}

func snapshotItemFXPreferenceObservationIDString(item domain.DailyValuationSnapshotItem) string {
	if item.FXPreferenceObservationID == nil {
		return ""
	}
	return item.FXPreferenceObservationID.String()
}

func snapshotItemMissingReasonString(item domain.DailyValuationSnapshotItem) string {
	if item.MissingReason == nil {
		return ""
	}
	return *item.MissingReason
}

func missingReason(component domain.ValuationComponent, _ []domain.MissingInputView) string {
	if component.InstrumentID != nil {
		return "missing instrument price or FX rate"
	}
	return "missing account value or FX rate"
}

func snapshotItemBaseAmountString(item domain.DailyValuationSnapshotItem) string {
	if item.BaseAmount == nil {
		return ""
	}
	return item.BaseAmount.CanonicalAmount()
}

func (s *Service) RebuildHistoricalSnapshots(ctx context.Context, startDate, endDate string) (int, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return 0, &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "snapshot date range is invalid"}
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil || end.Before(start) {
		return 0, &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "snapshot date range is invalid"}
	}
	if end.Sub(start) > 30*24*time.Hour {
		return 0, &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "a snapshot rebuild is limited to 31 days"}
	}
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return 0, err
	}
	if origin == nil {
		return 0, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before building snapshots"}
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return 0, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	endLocal, err := time.ParseInLocation("2006-01-02", endDate, location)
	if err != nil {
		return 0, &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "snapshot date range is invalid"}
	}
	nextMidnight, err := domain.ResolveLocalDateTime(endLocal.AddDate(0, 0, 1).Format("2006-01-02"), "00:00", origin.Timezone)
	if err != nil {
		return 0, err
	}
	batch, err := s.repository.LoadHistoricalSnapshotBatch(ctx, origin.HouseholdID, nextMidnight.Add(-time.Millisecond))
	if err != nil {
		return 0, err
	}
	quoteCache := &historicalQuoteCache{instrumentQuotes: make(map[domain.InstrumentID][]domain.InstrumentQuote), fxQuotes: batch.FXQuoteFacts}
	for _, quote := range batch.InstrumentQuoteFacts {
		quoteCache.instrumentQuotes[quote.InstrumentID] = append(quoteCache.instrumentQuotes[quote.InstrumentID], quote)
	}
	ctx = context.WithValue(ctx, historicalQuoteCacheKey{}, quoteCache)
	ctx = context.WithValue(ctx, historicalSnapshotBatchKey{}, &batch)
	appended := 0
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return appended, err
		}
		_, changed, buildErr := s.BuildDailyValuationSnapshot(ctx, date.Format("2006-01-02"))
		if buildErr != nil {
			return appended, buildErr
		}
		if changed {
			appended++
		}
	}
	return appended, nil
}

func (s *Service) CompleteDailySnapshotRange(ctx context.Context, householdID domain.HouseholdID, targetDate string) error {
	return s.repository.CompleteDailySnapshotRange(ctx, householdID, targetDate, s.clock())
}

func (s *Service) DailySnapshotState(ctx context.Context, householdID domain.HouseholdID) (domain.DailySnapshotState, error) {
	return s.repository.DailySnapshotState(ctx, householdID)
}
