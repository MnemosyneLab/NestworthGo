package application

import (
	"context"
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

func (s *Service) CreateInstrument(ctx context.Context, input InstrumentInput) (domain.Instrument, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.Instrument{}, err
	}
	if bootstrap.Household == nil {
		return domain.Instrument{}, onboardingRequired()
	}
	instrument, err := newInstrumentFromInput(bootstrap.Household.ID, input, s.now())
	if err != nil {
		return domain.Instrument{}, err
	}
	if instrument.LogoAssetID != nil {
		if err := s.requireAssetHousehold(ctx, bootstrap.Household.ID, *instrument.LogoAssetID); err != nil {
			return domain.Instrument{}, err
		}
	}
	origin, err := s.repository.HistoryOrigin(ctx, bootstrap.Household.ID)
	if err != nil {
		return domain.Instrument{}, err
	}
	if origin != nil {
		observation := domain.InstrumentPreferenceObservation{ID: domain.NewInstrumentPreferenceObservationID(), InstrumentID: instrument.ID, SourceKind: instrument.QuoteSource, EffectiveAt: instrument.CreatedAt, CreatedAt: instrument.CreatedAt}
		if err := s.repository.CreateInstrumentWithObservation(ctx, instrument, observation); err != nil {
			return domain.Instrument{}, safePortfolioError(err)
		}
		return instrument, nil
	}
	if err := s.repository.CreateInstrument(ctx, instrument); err != nil {
		return domain.Instrument{}, safePortfolioError(err)
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

func (s *Service) UpdateInstrument(ctx context.Context, id domain.InstrumentID, input InstrumentInput) (domain.Instrument, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.Instrument{}, err
	}
	if bootstrap.Household == nil {
		return domain.Instrument{}, onboardingRequired()
	}
	current, err := s.repository.Instrument(ctx, bootstrap.Household.ID, id)
	if err != nil {
		return domain.Instrument{}, safePortfolioError(err)
	}
	if !input.Replace {
		mergeInstrumentInput(&input, current)
	}
	updated, err := newInstrumentFromInput(current.HouseholdID, input, s.now())
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
			return domain.Instrument{}, safePortfolioError(err)
		}
	} else if err := s.repository.UpdateInstrument(ctx, updated); err != nil {
		return domain.Instrument{}, safePortfolioError(err)
	}
	return updated, nil
}

func (s *Service) ArchiveInstrument(ctx context.Context, id domain.InstrumentID, archived bool) error {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if bootstrap.Household == nil {
		return onboardingRequired()
	}
	return safePortfolioError(s.repository.SetInstrumentArchive(ctx, bootstrap.Household.ID, id, archived, s.now()))
}

func (s *Service) SetInstrumentLogo(ctx context.Context, id domain.InstrumentID, assetID domain.MediaAssetID) error {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if bootstrap.Household == nil {
		return onboardingRequired()
	}
	if err := s.requireAssetHousehold(ctx, bootstrap.Household.ID, assetID); err != nil {
		return err
	}
	return safePortfolioError(s.repository.SetInstrumentLogo(ctx, bootstrap.Household.ID, id, assetID))
}

func (s *Service) SetInstrumentQuoteSource(ctx context.Context, id domain.InstrumentID, source string) error {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if bootstrap.Household == nil {
		return onboardingRequired()
	}
	parsed, err := domain.ParseQuoteSourceKind(source)
	if err != nil {
		return err
	}
	if origin, originErr := s.repository.HistoryOrigin(ctx, bootstrap.Household.ID); originErr != nil {
		return originErr
	} else if origin != nil {
		instrument, instrumentErr := s.repository.Instrument(ctx, bootstrap.Household.ID, id)
		if instrumentErr != nil {
			return safePortfolioError(instrumentErr)
		}
		instrument.QuoteSource = parsed
		observation, observationErr := s.instrumentPreferenceObservation(ctx, instrument)
		if observationErr != nil {
			return observationErr
		}
		return safePortfolioError(s.repository.SetInstrumentQuoteSourceWithObservation(ctx, bootstrap.Household.ID, id, parsed, observation))
	}
	return safePortfolioError(s.repository.SetInstrumentQuoteSource(ctx, bootstrap.Household.ID, id, parsed))
}

type HoldingInput struct {
	AccountID    string
	InstrumentID string
	Quantity     string
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
	holding, err := domain.NewHoldingForAccount(account.Account, instrument, quantity, input.Note, input.SortOrder, s.now())
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
		state := domain.ChangeState{HouseholdID: origin.HouseholdID, OriginAt: origin.StartedAt, Timezone: origin.Timezone, Now: s.now(), Accounts: make(map[domain.AccountID]domain.ChangeAccountState), Cash: make(map[domain.AccountID]map[domain.CurrencyCode]domain.Money), Holdings: map[domain.HoldingID]domain.ChangeHoldingState{holding.ID: {ID: holding.ID, AccountID: holding.AccountID, InstrumentID: holding.InstrumentID, InstrumentName: instrument.Name, Currency: instrument.QuoteCurrency, Current: zero}}}
		preview, previewErr := domain.PreviewChange(state, domain.PositionAdjustmentInput{HouseholdID: origin.HouseholdID, HoldingID: holding.ID, Quantity: quantity, Added: true, EffectiveAt: s.now()})
		if previewErr != nil {
			return domain.Holding{}, previewErr
		}
		if err := s.repository.CreateHoldingWithActivity(ctx, created, domain.ActivityCommit{Activity: preview.Activity, Effects: preview.Effects, Resulting: preview.Resulting}, s.now()); err != nil {
			return domain.Holding{}, safePortfolioError(err)
		}
		return holding, nil
	}
	if err := s.repository.CreateHolding(ctx, holding); err != nil {
		return domain.Holding{}, safePortfolioError(err)
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

func (s *Service) UpdateHoldingQuantity(ctx context.Context, id domain.HoldingID, quantity string) (domain.Holding, error) {
	return s.UpdateHolding(ctx, id, HoldingUpdateInput{Quantity: quantity, QuantitySet: true})
}

func (s *Service) UpdateHolding(ctx context.Context, id domain.HoldingID, input HoldingUpdateInput) (domain.Holding, error) {
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
		current = current.ReplaceQuantity(quantity, s.now())
	} else {
		current.UpdatedAt = normalizeNow(s.now())
	}
	if input.NoteSet {
		current.Note = input.Note
	}
	if input.SortOrderSet {
		current.SortOrder = input.SortOrder
	}
	if err := s.repository.UpdateHolding(ctx, current); err != nil {
		return domain.Holding{}, safePortfolioError(err)
	}
	return current, nil
}

func (s *Service) ArchiveHolding(ctx context.Context, id domain.HoldingID, archived bool) error {
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
	return safePortfolioError(s.repository.SetHoldingArchive(ctx, household.ID, id, archived, s.now()))
}

func (s *Service) AppendAccountCashValue(ctx context.Context, accountID domain.AccountID, amount, currency, effectiveAt string) (domain.AccountCashValue, error) {
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
	parsedCurrency, err := domain.ParseCurrency(currency)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	money, err := domain.ParseMoney(amount, parsedCurrency)
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	when, err := parsePortfolioTimestamp(effectiveAt, s.now())
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	value, err := domain.NewAccountCashValue(record.Account, money, when, s.now())
	if err != nil {
		return domain.AccountCashValue{}, err
	}
	if origin, originErr := s.repository.HistoryOrigin(ctx, household.ID); originErr != nil {
		return domain.AccountCashValue{}, originErr
	} else if origin != nil {
		state, stateErr := s.changeState(ctx)
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
		preview, commitErr := s.RecordChange(ctx, command)
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
		return domain.AccountCashValue{}, safePortfolioError(err)
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
	when, err := parsePortfolioTimestamp(quotedAt, s.now())
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	quote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceManual, QuotedAt: when, Delayed: delayed}, s.now())
	if err != nil {
		return domain.InstrumentQuote{}, err
	}
	if err := s.repository.AppendInstrumentQuoteAndSelectManual(ctx, quote); err != nil {
		return domain.InstrumentQuote{}, safePortfolioError(err)
	}
	return quote, nil
}

func (s *Service) SaveManualInstrumentQuote(ctx context.Context, instrumentID domain.InstrumentID, unitPrice, quotedAt string) (domain.InstrumentQuote, error) {
	return s.AppendManualInstrumentQuote(ctx, instrumentID, unitPrice, quotedAt, false)
}

func (s *Service) SetFXPreference(ctx context.Context, currencyA, currencyB, source string) (domain.FXPreference, error) {
	_, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return domain.FXPreference{}, err
	}
	if household == nil {
		return domain.FXPreference{}, onboardingRequired()
	}
	a, err := domain.ParseCurrency(currencyA)
	if err != nil {
		return domain.FXPreference{}, err
	}
	b, err := domain.ParseCurrency(currencyB)
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
	preference, err := domain.NewFXPreference(household.ID, a, b, parsedSource, s.now())
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
		return domain.FXPreference{}, safePortfolioError(saveErr)
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
	_, household, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return domain.FXQuote{}, err
	}
	if household == nil {
		return domain.FXQuote{}, onboardingRequired()
	}
	base, err := domain.ParseCurrency(baseCurrency)
	if err != nil {
		return domain.FXQuote{}, err
	}
	quoteCurrencyCode, err := domain.ParseCurrency(quoteCurrency)
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
	when, err := parsePortfolioTimestamp(quotedAt, s.now())
	if err != nil {
		return domain.FXQuote{}, err
	}
	fxQuote, err := domain.NewFXQuote(domain.FXQuoteInput{HouseholdID: household.ID, BaseCurrency: base, QuoteCurrency: quoteCurrencyCode, Rate: parsedRate, SourceKind: domain.QuoteSourceManual, QuotedAt: when}, s.now())
	if err != nil {
		return domain.FXQuote{}, err
	}
	if err := s.repository.AppendFXQuoteAndSelectManual(ctx, fxQuote); err != nil {
		return domain.FXQuote{}, safePortfolioError(err)
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
	quoteCurrency, err := domain.ParseCurrency(input.QuoteCurrency)
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
	return domain.NewInstrument(domain.InstrumentInput{
		HouseholdID: householdID, Name: input.Name, Type: instrumentType, QuoteCurrency: quoteCurrency,
		Symbol: optionalText(input.Symbol), MarketCode: optionalText(input.MarketCode), CountryCode: optionalText(input.CountryCode), ISIN: optionalText(input.ISIN), Note: input.Note,
		LogoAssetID: logoAssetID, SortOrder: input.SortOrder, QuoteSource: domain.QuoteSourceKind(input.QuoteSource), ProviderKey: optionalText(input.ProviderKey), ProviderSymbol: optionalText(input.ProviderSymbol),
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
		return safePortfolioError(err)
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

func safePortfolioError(err error) error {
	if err == nil {
		return nil
	}
	return err
}

func normalizeNow(value time.Time) time.Time { return value.UTC().Truncate(time.Millisecond) }
