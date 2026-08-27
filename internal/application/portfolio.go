package application

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// InstrumentInput is the presentation-neutral command used by Instrument
// management. Provider refresh remains an explicit Service action; this
// command only persists the validated binding metadata.
type InstrumentInput struct {
	// Replace makes UpdateInstrument interpret this as the complete form
	// state. When false, zero values retain the existing field for callers that
	// intentionally perform a partial programmatic update.
	Replace        bool
	Name           string
	Type           string
	QuoteCurrency  string
	Symbol         string
	MarketCode     string
	CountryCode    string
	ISIN           string
	Note           *string
	LogoAssetID    string
	SortOrder      int
	QuoteSource    string
	ProviderKey    string
	ProviderSymbol string
}

// QuoteHistoryQuery keeps local quote-history reads bounded and deterministic
// without exposing repository SQL details to the UI.
type QuoteHistoryQuery struct {
	From       *time.Time
	To         *time.Time
	SourceKind *domain.QuoteSourceKind
	CurrencyA  domain.CurrencyCode
	CurrencyB  domain.CurrencyCode
	Limit      int
	Ascending  bool
}

func (s *Service) CreateInstrument(ctx context.Context, input InstrumentInput) (domain.Instrument, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.Instrument{}, err
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	instrument, err := newInstrumentFromInput(household.ID, input, s.clock())
	if err != nil {
		return domain.Instrument{}, err
	}
	if instrument.LogoAssetID != nil {
		if err := s.requireAssetHousehold(ctx, household.ID, *instrument.LogoAssetID); err != nil {
			return domain.Instrument{}, err
		}
	}
	origin, err := s.repository.HistoryOrigin(ctx, household.ID)
	if err != nil {
		return domain.Instrument{}, err
	}
	if origin != nil {
		observation := domain.InstrumentPreferenceObservation{ID: domain.NewInstrumentPreferenceObservationID(), InstrumentID: instrument.ID, SourceKind: instrument.QuoteSource, EffectiveAt: instrument.CreatedAt, CreatedAt: instrument.CreatedAt}
		if err := s.repository.CreateInstrumentWithObservation(ctx, instrument, observation); err != nil {
			return domain.Instrument{}, err
		}
		return instrument, nil
	}
	if err := s.repository.CreateInstrument(ctx, instrument); err != nil {
		return domain.Instrument{}, err
	}
	return instrument, nil
}

func (s *Service) ListInstruments(ctx context.Context, includeArchived bool) ([]domain.Instrument, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if bootstrap.Household == nil {
		return []domain.Instrument{}, nil
	}
	return s.repository.ListInstruments(ctx, bootstrap.Household.ID, includeArchived)
}

// CurrentInstrumentQuote returns the quote selected by the Instrument's
// current source preference. It is used to prefill local capture forms; the
// mutation boundary parses and validates the submitted value again.
func (s *Service) CurrentInstrumentQuote(ctx context.Context, id domain.InstrumentID) (*domain.InstrumentQuote, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	instrument, err := s.repository.Instrument(ctx, household.ID, id)
	if err != nil {
		return nil, err
	}
	quotes, err := s.repository.ListInstrumentQuotes(ctx, id)
	if err != nil {
		return nil, err
	}
	return selectInstrumentQuote(instrument, quotes), nil
}

// InstrumentQuoteHistory returns the locally persisted quote facts for one
// Instrument. It never refreshes a provider or performs network I/O.
func (s *Service) InstrumentQuoteHistory(ctx context.Context, id domain.InstrumentID, queries ...QuoteHistoryQuery) ([]domain.InstrumentQuote, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if bootstrap.Household == nil {
		return []domain.InstrumentQuote{}, nil
	}
	if _, err := s.repository.Instrument(ctx, bootstrap.Household.ID, id); err != nil {
		return nil, err
	}
	quotes, err := s.repository.ListInstrumentQuotes(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(queries) == 0 {
		return quotes, nil
	}
	return filterInstrumentQuoteHistory(quotes, queries[0]), nil
}

// FXQuoteHistory returns all locally persisted FX quote facts for the current
// Household. It never refreshes a provider or performs network I/O.
func (s *Service) FXQuoteHistory(ctx context.Context, queries ...QuoteHistoryQuery) ([]domain.FXQuote, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if bootstrap.Household == nil {
		return []domain.FXQuote{}, nil
	}
	quotes, err := s.repository.ListFXQuotes(ctx, bootstrap.Household.ID)
	if err != nil {
		return nil, err
	}
	if len(queries) == 0 {
		return quotes, nil
	}
	return filterFXQuoteHistory(quotes, queries[0]), nil
}

func filterInstrumentQuoteHistory(quotes []domain.InstrumentQuote, query QuoteHistoryQuery) []domain.InstrumentQuote {
	filtered := make([]domain.InstrumentQuote, 0, len(quotes))
	for _, quote := range quotes {
		if query.SourceKind != nil && quote.SourceKind != *query.SourceKind {
			continue
		}
		if query.From != nil && quote.QuotedAt.Before(*query.From) {
			continue
		}
		if query.To != nil && quote.QuotedAt.After(*query.To) {
			continue
		}
		filtered = append(filtered, quote)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return quoteLater(filtered[i].QuotedAt, filtered[i].CreatedAt, filtered[i].ID.String(), filtered[j].QuotedAt, filtered[j].CreatedAt, filtered[j].ID.String()) != query.Ascending
	})
	if query.Limit > 0 && len(filtered) > query.Limit {
		filtered = filtered[:query.Limit]
	}
	return filtered
}

func filterFXQuoteHistory(quotes []domain.FXQuote, query QuoteHistoryQuery) []domain.FXQuote {
	filtered := make([]domain.FXQuote, 0, len(quotes))
	var pairA, pairB domain.CurrencyCode
	if query.CurrencyA != "" || query.CurrencyB != "" {
		var err error
		pairA, pairB, err = domain.NormalizeFXPair(query.CurrencyA, query.CurrencyB)
		if err != nil {
			return filtered
		}
	}
	for _, quote := range quotes {
		if pairA != "" {
			quoteA, quoteB, err := domain.NormalizeFXPair(quote.BaseCurrency, quote.QuoteCurrency)
			if err != nil || quoteA != pairA || quoteB != pairB {
				continue
			}
		}
		if query.SourceKind != nil && quote.SourceKind != *query.SourceKind {
			continue
		}
		if query.From != nil && quote.QuotedAt.Before(*query.From) {
			continue
		}
		if query.To != nil && quote.QuotedAt.After(*query.To) {
			continue
		}
		filtered = append(filtered, quote)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return quoteLater(filtered[i].QuotedAt, filtered[i].CreatedAt, filtered[i].ID.String(), filtered[j].QuotedAt, filtered[j].CreatedAt, filtered[j].ID.String()) != query.Ascending
	})
	if query.Limit > 0 && len(filtered) > query.Limit {
		filtered = filtered[:query.Limit]
	}
	return filtered
}

// CurrentFXQuote selects the latest local quote for the configured source of
// one currency pair. The returned quote keeps its stored orientation; callers
// must display the BaseCurrency -> QuoteCurrency direction alongside Rate.
func (s *Service) CurrentFXQuote(ctx context.Context, currencyA, currencyB domain.CurrencyCode) (*domain.FXQuote, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	a, b, pairErr := domain.NormalizeFXPair(currencyA, currencyB)
	if pairErr != nil {
		return nil, pairErr
	}
	preferences, err := s.repository.ListFXPreferences(ctx, household.ID)
	if err != nil {
		return nil, err
	}
	var preference *domain.FXPreference
	for index := range preferences {
		if preferences[index].CurrencyA == a && preferences[index].CurrencyB == b {
			preference = &preferences[index]
			break
		}
	}
	if preference == nil {
		return nil, nil
	}
	quotes, err := s.repository.ListFXQuotes(ctx, household.ID)
	if err != nil {
		return nil, err
	}
	var selected *domain.FXQuote
	for index := range quotes {
		quote := &quotes[index]
		quoteA, quoteB, normalizeErr := domain.NormalizeFXPair(quote.BaseCurrency, quote.QuoteCurrency)
		if normalizeErr != nil || quote.SourceKind != preference.SourceKind || quoteA != a || quoteB != b {
			continue
		}
		if selected == nil || quoteLater(quote.QuotedAt, quote.CreatedAt, quote.ID.String(), selected.QuotedAt, selected.CreatedAt, selected.ID.String()) {
			selected = quote
		}
	}
	return selected, nil
}

func (s *Service) UpdateInstrument(ctx context.Context, id domain.InstrumentID, input InstrumentInput) (domain.Instrument, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.Instrument{}, err
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	current, err := s.repository.Instrument(ctx, household.ID, id)
	if err != nil {
		return domain.Instrument{}, err
	}
	if !input.Replace {
		mergeInstrumentInput(&input, current)
	}
	updated, err := newInstrumentFromInput(current.HouseholdID, input, s.clock())
	if err != nil {
		return domain.Instrument{}, err
	}
	updated.ID = current.ID
	updated.CreatedAt = current.CreatedAt
	updated.ArchivedAt = current.ArchivedAt
	if updated.LogoAssetID != nil {
		if err := s.requireAssetHousehold(ctx, current.HouseholdID, *updated.LogoAssetID); err != nil {
			return domain.Instrument{}, err
		}
	}
	preferenceObservation, observationErr := s.instrumentPreferenceObservation(ctx, updated)
	if observationErr != nil {
		return domain.Instrument{}, observationErr
	}
	if preferenceObservation.ID != "" && updated.QuoteSource != current.QuoteSource {
		if err := s.repository.UpdateInstrumentWithObservation(ctx, updated, preferenceObservation); err != nil {
			return domain.Instrument{}, err
		}
	} else if err := s.repository.UpdateInstrument(ctx, updated); err != nil {
		return domain.Instrument{}, err
	}
	return updated, nil
}

func (s *Service) ArchiveInstrument(ctx context.Context, id domain.InstrumentID, archived bool) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	return s.repository.SetInstrumentArchive(ctx, household.ID, id, archived, s.clock())
}

func (s *Service) SetInstrumentLogo(ctx context.Context, id domain.InstrumentID, assetID domain.MediaAssetID) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	if err := s.requireAssetHousehold(ctx, household.ID, assetID); err != nil {
		return err
	}
	return s.repository.SetInstrumentLogo(ctx, household.ID, id, assetID, s.clock())
}

func (s *Service) SetInstrumentQuoteSource(ctx context.Context, id domain.InstrumentID, source string) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	parsed, err := domain.ParseQuoteSourceKind(source)
	if err != nil {
		return err
	}
	if origin, originErr := s.repository.HistoryOrigin(ctx, household.ID); originErr != nil {
		return originErr
	} else if origin != nil {
		instrument, instrumentErr := s.repository.Instrument(ctx, household.ID, id)
		if instrumentErr != nil {
			return instrumentErr
		}
		instrument.QuoteSource = parsed
		observation, observationErr := s.instrumentPreferenceObservation(ctx, instrument)
		if observationErr != nil {
			return observationErr
		}
		return s.repository.SetInstrumentQuoteSourceWithObservation(ctx, household.ID, id, parsed, observation)
	}
	return s.repository.SetInstrumentQuoteSource(ctx, household.ID, id, parsed, s.clock())
}

type HoldingInput struct {
	AccountID    string
	InstrumentID string
	Quantity     string
	UnitCost     string
	Note         *string
	SortOrder    int
}

type HoldingUpdateInput struct {
	Quantity     string
	QuantitySet  bool
	Note         *string
	NoteSet      bool
	SortOrder    int
	SortOrderSet bool
}

func (s *Service) CreateHolding(ctx context.Context, input HoldingInput) (domain.Holding, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	accountID, err := domain.ParseAccountID(input.AccountID)
	if err != nil {
		return domain.Holding{}, err
	}
	instrumentID, err := domain.ParseInstrumentID(input.InstrumentID)
	if err != nil {
		return domain.Holding{}, err
	}
	quantity, err := domain.ParseQuantity(input.Quantity)
	if err != nil {
		return domain.Holding{}, err
	}
	snapshot, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return domain.Holding{}, err
	}
	account, ok := accountFromSnapshot(snapshot, accountID)
	if !ok {
		return domain.Holding{}, notFound("account")
	}
	instrument, ok := instrumentFromSnapshot(snapshot, instrumentID)
	if !ok {
		return domain.Holding{}, notFound("instrument")
	}
	if account.Account.ArchivedAt != nil {
		return domain.Holding{}, &domain.Error{Code: domain.ErrValidation, Field: "accountId", Message: "account is archived"}
	}
	if instrument.ArchivedAt != nil {
		return domain.Holding{}, &domain.Error{Code: domain.ErrValidation, Field: "instrumentId", Message: "instrument is archived"}
	}
	holding, err := domain.NewHoldingForAccount(account.Account, instrument, quantity, input.Note, input.SortOrder, s.clock())
	if err != nil {
		return domain.Holding{}, err
	}
	if household == nil {
		return domain.Holding{}, onboardingRequired()
	}
	origin, originErr := s.repository.HistoryOrigin(ctx, household.ID)
	if originErr != nil {
		return domain.Holding{}, originErr
	}
	if origin != nil && !quantity.IsZero() {
		zero, zeroErr := domain.ParseQuantity("0")
		if zeroErr != nil {
			return domain.Holding{}, zeroErr
		}
		created := holding
		created.Quantity = zero
		var unitCost *domain.UnitPrice
		if quote := selectInstrumentQuote(instrument, snapshot.InstrumentQuotes); quote != nil {
			cost := quote.UnitPrice
			unitCost = &cost
		}
		if strings.TrimSpace(input.UnitCost) != "" {
			cost, costErr := domain.ParseUnitPrice(input.UnitCost)
			if costErr != nil {
				return domain.Holding{}, costErr
			}
			unitCost = &cost
		}
		state := domain.ChangeState{HouseholdID: origin.HouseholdID, OriginAt: origin.StartedAt, Timezone: origin.Timezone, Now: s.clock(), Accounts: make(map[domain.AccountID]domain.ChangeAccountState), Cash: make(map[domain.AccountID]map[domain.CurrencyCode]domain.Money), Holdings: map[domain.HoldingID]domain.ChangeHoldingState{holding.ID: {ID: holding.ID, AccountID: holding.AccountID, InstrumentID: holding.InstrumentID, InstrumentName: instrument.Name, Currency: instrument.QuoteCurrency, Current: zero, CostBasisAvailable: false}}}
		preview, previewErr := domain.PreviewChange(state, domain.PositionAdjustmentInput{HouseholdID: origin.HouseholdID, HoldingID: holding.ID, Quantity: quantity, Added: true, UnitCost: unitCost, EffectiveAt: s.clock()})
		if previewErr != nil {
			return domain.Holding{}, previewErr
		}
		if err := s.repository.CreateHoldingWithActivity(ctx, created, domain.ActivityCommit{Activity: preview.Activity, Effects: preview.Effects, Resulting: preview.Resulting}, s.clock()); err != nil {
			return domain.Holding{}, err
		}
		return holding, nil
	}
	if err := s.repository.CreateHolding(ctx, holding); err != nil {
		return domain.Holding{}, err
	}
	return holding, nil
}

func (s *Service) ListHoldings(ctx context.Context, accountID domain.AccountID, includeArchived bool) ([]domain.Holding, error) {
	_, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if household == nil {
		return []domain.Holding{}, nil
	}
	return s.repository.ListHoldings(ctx, accountID, includeArchived)
}

// HoldingsByAccounts loads the holdings of several accounts in one repository
// round trip and groups them by account, replacing per-account ListHoldings
// loops in presentation code. Archived holdings are included.
func (s *Service) HoldingsByAccounts(ctx context.Context, accountIDs []domain.AccountID) (map[domain.AccountID][]domain.Holding, error) {
	grouped := make(map[domain.AccountID][]domain.Holding, len(accountIDs))
	if len(accountIDs) == 0 {
		return grouped, nil
	}
	holdings, err := s.repository.ListHoldingsByAccounts(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	for _, holding := range holdings {
		grouped[holding.AccountID] = append(grouped[holding.AccountID], holding)
	}
	return grouped, nil
}

func (s *Service) UpdateHoldingQuantity(ctx context.Context, id domain.HoldingID, quantity string) (domain.Holding, error) {
	return s.UpdateHolding(ctx, id, HoldingUpdateInput{Quantity: quantity, QuantitySet: true})
}

func (s *Service) UpdateHolding(ctx context.Context, id domain.HoldingID, input HoldingUpdateInput) (domain.Holding, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	snapshot, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return domain.Holding{}, err
	}
	if household == nil {
		return domain.Holding{}, onboardingRequired()
	}
	current, ok := holdingFromSnapshot(snapshot, id)
	if !ok {
		return domain.Holding{}, notFound("holding")
	}
	if current.ArchivedAt != nil {
		return domain.Holding{}, &domain.Error{Code: domain.ErrValidation, Field: "holdingId", Message: "holding is archived"}
	}
	if input.QuantitySet || input.Quantity != "" {
		if origin, originErr := s.repository.HistoryOrigin(ctx, household.ID); originErr != nil {
			return domain.Holding{}, originErr
		} else if origin != nil {
			return domain.Holding{}, &domain.Error{Code: domain.ErrConflict, Message: "Holding quantity changes must be recorded as a change"}
		}
		quantity, parseErr := domain.ParseQuantity(input.Quantity)
		if parseErr != nil {
			return domain.Holding{}, parseErr
		}
		current = current.ReplaceQuantity(quantity, s.clock())
	} else {
		current.UpdatedAt = normalizeNow(s.clock())
	}
	if input.NoteSet {
		current.Note = input.Note
	}
	if input.SortOrderSet {
		current.SortOrder = input.SortOrder
	}
	if err := s.repository.UpdateHolding(ctx, current); err != nil {
		return domain.Holding{}, err
	}
	return current, nil
}

func (s *Service) ArchiveHolding(ctx context.Context, id domain.HoldingID, archived bool) error {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	snapshot, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return err
	}
	if household == nil {
		return onboardingRequired()
	}
	origin, originErr := s.repository.HistoryOrigin(ctx, household.ID)
	if originErr != nil {
		return originErr
	}
	if archived && origin != nil {
		current, found := holdingFromSnapshot(snapshot, id)
		if !found {
			return &domain.Error{Code: domain.ErrNotFound, Message: "holding was not found"}
		}
		if !current.Quantity.IsZero() {
			return &domain.Error{Code: domain.ErrConflict, Message: "a Holding must have zero quantity before it is archived"}
		}
	}
	return s.repository.SetHoldingArchive(ctx, household.ID, id, archived, s.clock())
}

func (s *Service) AppendAccountCashValue(ctx context.Context, accountID domain.AccountID, amount, currency, effectiveAt string) (domain.AccountCashValue, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	snapshot, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	if household == nil {
		return domain.AccountCashValue{}, onboardingRequired()
	}
	record, ok := accountFromSnapshot(snapshot, accountID)
	if !ok {
		return domain.AccountCashValue{}, notFound("account")
	}
	if record.Account.ArchivedAt != nil {
		return domain.AccountCashValue{}, &domain.Error{Code: domain.ErrValidation, Field: "accountId", Message: "account is archived"}
	}
	parsedCurrency, err := domain.ParseSupportedCurrency(currency)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	money, err := domain.ParseMoney(amount, parsedCurrency)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	when, err := parsePortfolioTimestamp(effectiveAt, s.clock())
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	value, err := domain.NewAccountCashValue(record.Account, money, when, s.clock())
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	if origin, originErr := s.repository.HistoryOrigin(ctx, household.ID); originErr != nil {
		return domain.AccountCashValue{}, originErr
	} else if origin != nil {
		state, stateErr := s.changeStateFrom(origin, snapshot)
		if stateErr != nil {
			return domain.AccountCashValue{}, stateErr
		}
		current, currentErr := currentCashAmount(state, accountID, parsedCurrency)
		if currentErr != nil {
			return domain.AccountCashValue{}, currentErr
		}
		delta, added, differenceErr := differenceMoney(money, current)
		if differenceErr != nil {
			return domain.AccountCashValue{}, differenceErr
		}
		if delta.IsZero() {
			return domain.AccountCashValue{}, &domain.Error{Code: domain.ErrNoChange, Message: "the new cash value is unchanged"}
		}
		var command any
		if added {
			command = domain.MoneyAddedInput{HouseholdID: household.ID, AccountID: accountID, Amount: delta, Reason: domain.ReasonReconciliation, EffectiveAt: when}
		} else {
			command = domain.MoneyRemovedInput{HouseholdID: household.ID, AccountID: accountID, Amount: delta, Reason: domain.ReasonReconciliation, EffectiveAt: when}
		}
		preview, commitErr := s.commitChangeLocked(ctx, state, command)
		if commitErr != nil {
			return domain.AccountCashValue{}, commitErr
		}
		resultMoney, parseErr := domain.ParseMoney(preview.Resulting[0].Amount, preview.Resulting[0].Currency)
		if parseErr != nil {
			return domain.AccountCashValue{}, parseErr
		}
		return domain.NewAccountCashValue(record.Account, resultMoney, preview.Activity.EffectiveAt, preview.Activity.CreatedAt)
	}
	if err := s.repository.AppendAccountCashValue(ctx, value); err != nil {
		return domain.AccountCashValue{}, err
	}
	return value, nil
}

func (s *Service) ListAccountCashValues(ctx context.Context, accountID domain.AccountID) ([]domain.AccountCashValue, error) {
	_, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if household == nil {
		return []domain.AccountCashValue{}, nil
	}
	return s.repository.ListAccountCashValues(ctx, accountID)
}

func (s *Service) AppendManualInstrumentQuote(ctx context.Context, instrumentID domain.InstrumentID, unitPrice, quotedAt string, delayed bool) (domain.InstrumentQuote, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	snapshot, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	if household == nil {
		return domain.InstrumentQuote{}, onboardingRequired()
	}
	instrument, ok := instrumentFromSnapshot(snapshot, instrumentID)
	if !ok {
		return domain.InstrumentQuote{}, notFound("instrument")
	}
	if instrument.ArchivedAt != nil {
		return domain.InstrumentQuote{}, &domain.Error{Code: domain.ErrValidation, Field: "instrumentId", Message: "instrument is archived"}
	}
	price, err := domain.ParseUnitPrice(unitPrice)
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	when, err := parsePortfolioTimestamp(quotedAt, s.clock())
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	quote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceManual, QuotedAt: when, Delayed: delayed}, s.clock())
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	if err := s.repository.AppendInstrumentQuoteAndSelectManual(ctx, quote); err != nil {
		return domain.InstrumentQuote{}, err
	}
	return quote, nil
}

func (s *Service) SaveManualInstrumentQuote(ctx context.Context, instrumentID domain.InstrumentID, unitPrice, quotedAt string) (domain.InstrumentQuote, error) {
	return s.AppendManualInstrumentQuote(ctx, instrumentID, unitPrice, quotedAt, false)
}

func (s *Service) SetFXPreference(ctx context.Context, currencyA, currencyB, source string) (domain.FXPreference, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	_, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return domain.FXPreference{}, err
	}
	if household == nil {
		return domain.FXPreference{}, onboardingRequired()
	}
	a, err := domain.ParseSupportedCurrency(currencyA)
	if err != nil {
		return domain.FXPreference{}, err
	}
	b, err := domain.ParseSupportedCurrency(currencyB)
	if err != nil {
		return domain.FXPreference{}, err
	}
	if a != household.BaseCurrency && b != household.BaseCurrency {
		return domain.FXPreference{}, &domain.Error{Code: domain.ErrValidation, Field: "currencyPair", Message: "currency pair must include the Household base currency"}
	}
	parsedSource, err := domain.ParseQuoteSourceKind(source)
	if err != nil {
		return domain.FXPreference{}, err
	}
	preference, err := domain.NewFXPreference(household.ID, a, b, parsedSource, s.clock())
	if err != nil {
		return domain.FXPreference{}, err
	}
	observation, observationErr := s.fxPreferenceObservation(ctx, preference)
	if observationErr != nil {
		return domain.FXPreference{}, observationErr
	}
	var saveErr error
	if observation.ID != "" {
		saveErr = s.repository.SetFXPreferenceWithObservation(ctx, preference, observation)
	} else {
		saveErr = s.repository.SetFXPreference(ctx, preference)
	}
	if saveErr != nil {
		return domain.FXPreference{}, saveErr
	}
	return preference, nil
}

func (s *Service) ListFXPreferences(ctx context.Context) ([]domain.FXPreference, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if bootstrap.Household == nil {
		return []domain.FXPreference{}, nil
	}
	return s.repository.ListFXPreferences(ctx, bootstrap.Household.ID)
}

func (s *Service) AppendManualFXQuote(ctx context.Context, baseCurrency, quoteCurrency, rate, quotedAt string) (domain.FXQuote, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	_, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return domain.FXQuote{}, err
	}
	if household == nil {
		return domain.FXQuote{}, onboardingRequired()
	}
	base, err := domain.ParseSupportedCurrency(baseCurrency)
	if err != nil {
		return domain.FXQuote{}, err
	}
	quoteCurrencyCode, err := domain.ParseSupportedCurrency(quoteCurrency)
	if err != nil {
		return domain.FXQuote{}, err
	}
	if base != household.BaseCurrency && quoteCurrencyCode != household.BaseCurrency {
		return domain.FXQuote{}, &domain.Error{Code: domain.ErrValidation, Field: "currencyPair", Message: "currency pair must include the Household base currency"}
	}
	parsedRate, err := domain.ParseFxRate(rate)
	if err != nil {
		return domain.FXQuote{}, err
	}
	when, err := parsePortfolioTimestamp(quotedAt, s.clock())
	if err != nil {
		return domain.FXQuote{}, err
	}
	fxQuote, err := domain.NewFXQuote(domain.FXQuoteInput{HouseholdID: household.ID, BaseCurrency: base, QuoteCurrency: quoteCurrencyCode, Rate: parsedRate, SourceKind: domain.QuoteSourceManual, QuotedAt: when}, s.clock())
	if err != nil {
		return domain.FXQuote{}, err
	}
	if err := s.repository.AppendFXQuoteAndSelectManual(ctx, fxQuote); err != nil {
		return domain.FXQuote{}, err
	}
	return fxQuote, nil
}

func (s *Service) SaveManualFXQuote(ctx context.Context, baseCurrency, quoteCurrency, rate, quotedAt string) (domain.FXQuote, error) {
	return s.AppendManualFXQuote(ctx, baseCurrency, quoteCurrency, rate, quotedAt)
}

func (s *Service) portfolioSnapshot(ctx context.Context) (domain.PortfolioSnapshot, *domain.Household, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.PortfolioSnapshot{}, nil, err
	}
	return snapshot, snapshot.Household, nil
}

func newInstrumentFromInput(householdID domain.HouseholdID, input InstrumentInput, now time.Time) (domain.Instrument, error) {
	quoteCurrency, err := domain.ParseSupportedCurrency(input.QuoteCurrency)
	if err != nil {
		return domain.Instrument{}, err
	}
	instrumentType, err := domain.ParseInstrumentType(input.Type)
	if err != nil {
		return domain.Instrument{}, err
	}
	logoAssetID, err := parseOptionalMediaID(input.LogoAssetID)
	if err != nil {
		return domain.Instrument{}, err
	}
	quoteSourceValue := input.QuoteSource
	if quoteSourceValue == "" {
		quoteSourceValue = string(domain.QuoteSourceManual)
	}
	quoteSource, err := domain.ParseQuoteSourceKind(quoteSourceValue)
	if err != nil {
		return domain.Instrument{}, err
	}
	return domain.NewInstrument(domain.InstrumentInput{
		HouseholdID: householdID, Name: input.Name, Type: instrumentType, QuoteCurrency: quoteCurrency,
		Symbol: optionalText(input.Symbol), MarketCode: optionalText(input.MarketCode), CountryCode: optionalText(input.CountryCode), ISIN: optionalText(input.ISIN), Note: input.Note,
		LogoAssetID: logoAssetID, SortOrder: input.SortOrder, QuoteSource: quoteSource, ProviderKey: optionalText(input.ProviderKey), ProviderSymbol: optionalText(input.ProviderSymbol),
	}, now)
}

func mergeInstrumentInput(input *InstrumentInput, current domain.Instrument) {
	if input.Name == "" {
		input.Name = current.Name
	}
	if input.Type == "" {
		input.Type = string(current.Type)
	}
	if input.QuoteCurrency == "" {
		input.QuoteCurrency = current.QuoteCurrency.String()
	}
	if input.Symbol == "" {
		input.Symbol = pointerText(current.Symbol)
	}
	if input.MarketCode == "" {
		input.MarketCode = pointerText(current.MarketCode)
	}
	if input.CountryCode == "" {
		input.CountryCode = pointerText(current.CountryCode)
	}
	if input.ISIN == "" {
		input.ISIN = pointerText(current.ISIN)
	}
	if input.Note == nil {
		input.Note = current.Note
	}
	if input.LogoAssetID == "" {
		input.LogoAssetID = pointerMediaID(current.LogoAssetID)
	}
	if input.QuoteSource == "" {
		input.QuoteSource = string(current.QuoteSource)
	}
	if input.ProviderKey == "" {
		input.ProviderKey = pointerText(current.ProviderKey)
	}
	if input.ProviderSymbol == "" {
		input.ProviderSymbol = pointerText(current.ProviderSymbol)
	}
	if input.SortOrder == 0 {
		input.SortOrder = current.SortOrder
	}
}

func accountFromSnapshot(snapshot domain.PortfolioSnapshot, id domain.AccountID) (domain.AccountRecord, bool) {
	for _, record := range snapshot.Accounts {
		if record.Account.ID == id {
			return record, true
		}
	}
	return domain.AccountRecord{}, false
}

func instrumentFromSnapshot(snapshot domain.PortfolioSnapshot, id domain.InstrumentID) (domain.Instrument, bool) {
	for _, instrument := range snapshot.Instruments {
		if instrument.ID == id {
			return instrument, true
		}
	}
	return domain.Instrument{}, false
}

func holdingFromSnapshot(snapshot domain.PortfolioSnapshot, id domain.HoldingID) (domain.Holding, bool) {
	for _, holding := range snapshot.Holdings {
		if holding.ID == id {
			return holding, true
		}
	}
	return domain.Holding{}, false
}

func parsePortfolioTimestamp(value string, fallback time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed.UTC(), nil
	}
	return time.Time{}, &domain.Error{Code: domain.ErrValidation, Field: "quotedAt", Message: "must use an RFC 3339 timestamp or YYYY-MM-DD"}
}

func optionalText(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func pointerText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func parseOptionalMediaID(value string) (*domain.MediaAssetID, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	id, err := domain.ParseMediaAssetID(value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func pointerMediaID(value *domain.MediaAssetID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func (s *Service) requireAssetHousehold(ctx context.Context, householdID domain.HouseholdID, assetID domain.MediaAssetID) error {
	asset, err := s.repository.MediaAsset(ctx, householdID, assetID)
	if err != nil {
		return err
	}
	if asset.HouseholdID != householdID {
		return &domain.Error{Code: domain.ErrValidation, Field: "logoAssetId", Message: "media asset belongs to another Household"}
	}
	return nil
}

func onboardingRequired() error {
	return &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
}

func notFound(entity string) error {
	return &domain.Error{Code: domain.ErrNotFound, Message: entity + " was not found"}
}
func normalizeNow(value time.Time) time.Time { return value.UTC().Truncate(time.Millisecond) }
