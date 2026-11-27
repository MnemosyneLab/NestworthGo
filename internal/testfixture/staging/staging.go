// Package staging populates explicitly synthetic baseline facts using the
// domain constructors and the isolated SQL fixture writer. It is not a
// financial mutation API and is never imported by a product package.
package staging

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/testfixture/storage"
	"strings"
	"time"
)

type Reader interface {
	Household(context.Context) (*domain.Household, error)
	ListMembers(context.Context, bool) ([]domain.Member, error)
	ListInstitutions(context.Context, bool) ([]domain.Institution, error)
	ListGroups(context.Context, bool) ([]domain.Group, error)
	ListAccountRecords(context.Context, domain.HouseholdID, domain.AccountFilter) ([]domain.AccountRecord, error)
	AccountRecord(context.Context, domain.HouseholdID, domain.AccountID) (domain.AccountRecord, error)
	Instrument(context.Context, domain.HouseholdID, domain.InstrumentID) (domain.Instrument, error)
	ReadPortfolioSnapshot(context.Context, domain.AccountFilter) (domain.PortfolioSnapshot, error)
	Path() string
}
type Stager struct {
	repository Reader
	writer     *storage.Writer
	clock      func() time.Time
}

func New(repo Reader, now func() time.Time) *Stager {
	return &Stager{repository: repo, writer: storage.ForPath(repo.Path()), clock: now}
}

type AccountInput struct {
	Name                     string
	AccountType              string
	BalanceSheetRole         string
	TrackingMode             string
	DefaultCurrency          string
	InstitutionID            string
	InstitutionIDSet         bool
	GroupID                  string
	GroupIDSet               bool
	Note                     *string
	NoteSet                  bool
	IconKey                  string
	IconKeySet               bool
	IncludeInNetWorth        bool
	IncludeInNetWorthSet     bool
	IncludeInPortfolio       bool
	IncludeInPortfolioSet    bool
	IncludeInLiquidAssets    bool
	IncludeInLiquidAssetsSet bool
	OpenedOn                 *string
	OpenedOnSet              bool
	ClosedOn                 *string
	ClosedOnSet              bool
	Ownership                []domain.OwnershipShare
	OwnerIDs                 []domain.MemberID
	OwnershipPercentages     []string
	InitialAmount            string
}
type InstrumentInput struct {
	// Replace makes UpdateInstrument interpret this as the complete form
	// state. When false, zero values retain the existing field for callers that
	// intentionally perform a partial programmatic update.
	Replace        bool
	Name           string
	ShortName      *string
	Type           string
	MetalTemplate  string
	QuantityUnit   string
	QuoteCurrency  string
	Symbol         string
	MarketCode     string
	CountryCode    string
	ISIN           string
	Note           *string
	IconKey        string
	SortOrder      int
	QuoteSource    string
	ProviderKey    string
	ProviderSymbol string
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
type Bootstrap struct {
	Household    *domain.Household
	Members      []domain.Member
	Institutions []domain.Institution
	Groups       []domain.Group
}

func (s *Stager) CreateAccount(ctx context.Context, input AccountInput) (domain.AccountRecord, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if bootstrap.Household == nil {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	primary, err := domain.ParseAccountType(input.AccountType)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	secondary, err := domain.ParseBalanceSheetRole(input.BalanceSheetRole)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	mode, err := domain.ParseTrackingMode(input.TrackingMode)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	currency, err := domain.ParseSupportedCurrency(strings.ToUpper(strings.TrimSpace(input.DefaultCurrency)))
	if err != nil {
		return domain.AccountRecord{}, err
	}
	var iconKey *string
	if strings.TrimSpace(input.IconKey) != "" {
		normalized, iconErr := normalizeIconKey(input.IconKey)
		if iconErr != nil {
			return domain.AccountRecord{}, iconErr
		}
		iconKey = &normalized
	}
	ownershipShares, err := resolveOwnership(input)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	accountInput := domain.AccountInput{HouseholdID: bootstrap.Household.ID, Name: input.Name, AccountType: primary, BalanceSheetRole: secondary, TrackingMode: mode, DefaultCurrency: currency, Note: input.Note, IconKey: iconKey, IncludeInNetWorth: input.IncludeInNetWorth, IncludeInPortfolio: input.IncludeInPortfolio, IncludeInLiquidAssets: input.IncludeInLiquidAssets, OpenedOn: input.OpenedOn, ClosedOn: input.ClosedOn, Ownership: ownershipShares, InitialAmount: input.InitialAmount}
	if input.InstitutionID != "" {
		id, parseErr := domain.ParseInstitutionID(input.InstitutionID)
		if parseErr != nil {
			return domain.AccountRecord{}, parseErr
		}
		accountInput.InstitutionID = &id
	}
	if input.GroupID != "" {
		id, parseErr := domain.ParseGroupID(input.GroupID)
		if parseErr != nil {
			return domain.AccountRecord{}, parseErr
		}
		accountInput.GroupID = &id
	}
	if input.InstitutionID != "" && !containsInstitution(bootstrap.Institutions, accountInput.InstitutionID) {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Field: "institutionId", Message: "institution is not an active reference"}
	}
	if input.GroupID != "" && !containsGroup(bootstrap.Groups, accountInput.GroupID) {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Field: "groupId", Message: "group is not an active reference"}
	}
	if err := validateOwnershipMembers(bootstrap.Members, accountInput.Ownership); err != nil {
		return domain.AccountRecord{}, err
	}
	account, ownership, initial, err := domain.NewAccount(accountInput, s.clock())
	if err != nil {
		return domain.AccountRecord{}, err
	}
	var value *domain.AccountValue
	if initial != nil {
		created, valueErr := domain.NewAccountValue(account, *initial, account.CreatedAt, account.CreatedAt)
		if valueErr != nil {
			return domain.AccountRecord{}, valueErr
		}
		value = &created
	}
	if err := s.writer.CreateAccount(ctx, account, ownership, value); err != nil {
		return domain.AccountRecord{}, err
	}
	return domain.AccountRecord{Account: account, Ownership: ownership, LatestValue: value}, nil
}

func (s *Stager) UpdateAccount(ctx context.Context, id domain.AccountID, input AccountInput) (domain.AccountRecord, error) {
	currentRecord, err := s.accountRecord(ctx, id)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	current := &currentRecord
	if current.LatestValue == nil && current.Account.TrackingMode != domain.TrackingHoldings {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Message: "account has no current value"}
	}
	if input.AccountType == "" {
		input.AccountType = current.Account.AccountType.String()
	}
	if input.BalanceSheetRole == "" {
		input.BalanceSheetRole = string(current.Account.BalanceSheetRole)
	}
	if input.TrackingMode == "" {
		input.TrackingMode = string(current.Account.TrackingMode)
	}
	if input.DefaultCurrency == "" {
		input.DefaultCurrency = current.Account.DefaultCurrency.String()
	}
	if input.Name == "" {
		input.Name = current.Account.Name
	}
	if input.InitialAmount == "" && current.LatestValue != nil {
		input.InitialAmount = current.LatestValue.Amount.CanonicalAmount()
	}
	// Omitted ownership (nil shares and no owner IDs) keeps the current
	// allocation. An explicit empty share list is not "leave unchanged"
	// and is not a household-wide even split; resolveOwnership rejects it.
	if input.Ownership == nil && len(input.OwnerIDs) == 0 {
		input.Ownership = current.Ownership.Shares()
	}
	if !input.InstitutionIDSet {
		if current.Account.InstitutionID != nil {
			input.InstitutionID = current.Account.InstitutionID.String()
		}
	}
	if !input.GroupIDSet {
		if current.Account.GroupID != nil {
			input.GroupID = current.Account.GroupID.String()
		}
	}
	if !input.NoteSet {
		input.Note = current.Account.Note
	}
	if !input.IconKeySet && current.Account.IconKey != nil {
		input.IconKey = *current.Account.IconKey
	}
	if !input.OpenedOnSet {
		input.OpenedOn = current.Account.OpenedOn
	}
	if !input.ClosedOnSet {
		input.ClosedOn = current.Account.ClosedOn
	}
	if !input.IncludeInNetWorthSet {
		input.IncludeInNetWorth = current.Account.IncludeInNetWorth
	}
	if !input.IncludeInPortfolioSet {
		input.IncludeInPortfolio = current.Account.IncludeInPortfolio
	}
	if !input.IncludeInLiquidAssetsSet {
		input.IncludeInLiquidAssets = current.Account.IncludeInLiquidAssets
	}
	var iconKey *string
	if strings.TrimSpace(input.IconKey) != "" {
		normalized, iconErr := normalizeIconKey(input.IconKey)
		if iconErr != nil {
			return domain.AccountRecord{}, iconErr
		}
		iconKey = &normalized
	}
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if bootstrap.Household == nil {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	primary, err := domain.ParseAccountType(input.AccountType)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	secondary, err := domain.ParseBalanceSheetRole(input.BalanceSheetRole)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	mode, err := domain.ParseTrackingMode(input.TrackingMode)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if mode != current.Account.TrackingMode {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Field: "trackingMode", Message: "tracking mode is immutable after account creation"}
	}
	if mode == domain.TrackingHoldings && (primary == domain.TypeCashOnHand) != (current.Account.AccountType == domain.TypeCashOnHand) {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Field: "accountType", Message: "cash-only multi-currency accounts cannot be converted to or from investment accounts"}
	}
	if secondary != current.Account.BalanceSheetRole {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Field: "balanceSheetRole", Message: "balance sheet role is immutable after account creation"}
	}
	currency, err := domain.ParseCurrency(input.DefaultCurrency)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if currency != current.Account.DefaultCurrency {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Field: "defaultCurrency", Message: "account currency is immutable after creation"}
	}
	var institutionID *domain.InstitutionID
	if input.InstitutionID != "" {
		parsed, parseErr := domain.ParseInstitutionID(input.InstitutionID)
		if parseErr != nil {
			return domain.AccountRecord{}, parseErr
		}
		institutionID = &parsed
		if input.InstitutionIDSet && !containsInstitution(bootstrap.Institutions, institutionID) {
			return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Field: "institutionId", Message: "institution is not an active reference"}
		}
	}
	var groupID *domain.GroupID
	if input.GroupID != "" {
		parsed, parseErr := domain.ParseGroupID(input.GroupID)
		if parseErr != nil {
			return domain.AccountRecord{}, parseErr
		}
		groupID = &parsed
		if input.GroupIDSet && !containsGroup(bootstrap.Groups, groupID) {
			return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Field: "groupId", Message: "group is not an active reference"}
		}
	}
	ownershipShares, err := resolveOwnership(input)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	account, ownership, _, err := domain.NewAccount(domain.AccountInput{HouseholdID: current.Account.HouseholdID, InstitutionID: institutionID, GroupID: groupID, Name: input.Name, AccountType: primary, BalanceSheetRole: secondary, TrackingMode: mode, DefaultCurrency: currency, Note: input.Note, IconKey: iconKey, IncludeInNetWorth: input.IncludeInNetWorth, IncludeInPortfolio: input.IncludeInPortfolio, IncludeInLiquidAssets: input.IncludeInLiquidAssets, OpenedOn: input.OpenedOn, ClosedOn: input.ClosedOn, SortOrder: current.Account.SortOrder, Ownership: ownershipShares, InitialAmount: input.InitialAmount}, s.clock())
	if err != nil {
		return domain.AccountRecord{}, err
	}
	// NewAccount supplies a creation-time default for nil icons. Updates must
	// retain the merged icon, including a previously cleared icon when omitted.
	account.IconKey = iconKey
	account.ID, account.CreatedAt, account.ArchivedAt = current.Account.ID, current.Account.CreatedAt, current.Account.ArchivedAt
	allMembers, err := s.repository.ListMembers(ctx, true)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if err := validateOwnershipMembersForUpdate(allMembers, ownership.Shares(), current.Ownership.Shares()); err != nil {
		return domain.AccountRecord{}, err
	}
	if err := s.writer.UpdateAccount(ctx, account, ownership); err != nil {
		return domain.AccountRecord{}, err
	}
	current.Account, current.Ownership = account, ownership
	return *current, nil
}

func (s *Stager) ArchiveAccount(ctx context.Context, id domain.AccountID, archived bool) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	records, err := s.repository.ListAccountRecords(ctx, household.ID, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return err
	}
	var current *domain.AccountRecord
	for index := range records {
		if records[index].Account.ID == id {
			current = &records[index]
			break
		}
	}
	if current == nil {
		return &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
	}
	now := s.clock()
	if archived {
		current.Account.ArchivedAt = &now
	} else {
		current.Account.ArchivedAt = nil
	}
	return s.writer.SetAccountArchive(ctx, household.ID, id, archived, now)
}

func (s *Stager) AppendAccountValue(ctx context.Context, accountID domain.AccountID, amount, effectiveAt string) (domain.AccountValue, error) {
	loaded, err := s.accountRecord(ctx, accountID)
	if err != nil {
		return domain.AccountValue{}, err
	}
	record := &loaded
	money, err := domain.ParseMoney(amount, record.Account.DefaultCurrency)
	if err != nil {
		return domain.AccountValue{}, err
	}
	when := s.clock()
	currentObservation := strings.TrimSpace(effectiveAt) == ""
	if !currentObservation {
		parsed, parseErr := time.Parse("2006-01-02", effectiveAt)
		if parseErr != nil {
			return domain.AccountValue{}, &domain.Error{Code: domain.ErrValidation, Field: "effectiveAt", Message: "must use YYYY-MM-DD"}
		}
		when = parsed.UTC()
	}
	value, err := domain.NewAccountValue(record.Account, money, when, s.clock())
	if err != nil {
		return domain.AccountValue{}, err
	}
	if latest := record.LatestValue; currentObservation && record.Account.TrackingMode == domain.TrackingBalance && latest != nil {
		// Match the latest-value query at storage precision. When both
		// timestamps tie, the newly inserted row wins by insertion order.
		if value.EffectiveAt.Before(latest.EffectiveAt) ||
			(value.EffectiveAt.Equal(latest.EffectiveAt) && value.CreatedAt.Before(latest.CreatedAt)) {
			return domain.AccountValue{}, &domain.Error{Code: domain.ErrCashBalanceTimeConflict, Field: "effectiveAt", Message: "a later balance observation would override this update; check the system clock and balance history"}
		}
	}
	if err := s.writer.AppendAccountValue(ctx, value); err != nil {
		return domain.AccountValue{}, err
	}
	return value, nil
}

func (s *Stager) CreateInstrument(ctx context.Context, input InstrumentInput) (domain.Instrument, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.Instrument{}, err
	}
	instrument, err := newInstrumentFromInput(household.ID, input, s.clock())
	if err != nil {
		return domain.Instrument{}, err
	}
	if err := s.writer.CreateInstrument(ctx, instrument); err != nil {
		return domain.Instrument{}, err
	}
	return instrument, nil
}

func (s *Stager) UpdateInstrument(ctx context.Context, id domain.InstrumentID, input InstrumentInput) (domain.Instrument, error) {
	return s.editInstrument(ctx, id, func(current domain.Instrument) (InstrumentInput, error) {
		// Omission remains compatible with older complete-form callers. Clearing is explicit.
		if input.ShortName == nil {
			value := current.ShortName
			input.ShortName = &value
		}
		if !input.Replace {
			mergeInstrumentInput(&input, current)
		}
		return input, nil
	})
}

func (s *Stager) editInstrument(ctx context.Context, id domain.InstrumentID, merge func(domain.Instrument) (InstrumentInput, error)) (domain.Instrument, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.Instrument{}, err
	}
	current, err := s.repository.Instrument(ctx, household.ID, id)
	if err != nil {
		return domain.Instrument{}, err
	}

	input, err := merge(current)
	if err != nil {
		return domain.Instrument{}, err
	}
	updated, err := newInstrumentFromInput(current.HouseholdID, input, s.clock())
	if err != nil {
		return domain.Instrument{}, err
	}
	if updated.MetalTemplate != current.MetalTemplate || updated.QuantityUnit != current.QuantityUnit || (current.MetalTemplate != "" && updated.Type != current.Type) {
		return domain.Instrument{}, &domain.Error{Code: domain.ErrValidation, Field: "quantityUnit", Message: "metal template and holding unit are immutable after creation"}
	}
	if updated.QuoteCurrency != current.QuoteCurrency {
		return domain.Instrument{}, &domain.Error{Code: domain.ErrValidation, Field: "quoteCurrency", Message: "instrument currency is immutable after creation"}
	}
	updated.ID = current.ID
	updated.CreatedAt = current.CreatedAt
	updated.ArchivedAt = current.ArchivedAt
	if err := s.writer.UpdateInstrument(ctx, updated); err != nil {
		return domain.Instrument{}, err
	}
	return updated, nil
}

func (s *Stager) SetInstrumentQuoteSource(ctx context.Context, id domain.InstrumentID, source string) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}

	parsed, err := domain.ParseQuoteSourceKind(source)
	if err != nil {
		return err
	}
	return s.writer.SetInstrumentQuoteSource(ctx, household.ID, id, parsed, s.clock())
}

func (s *Stager) CreateHolding(ctx context.Context, input HoldingInput) (domain.Holding, error) {
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
	if err := s.writer.CreateHolding(ctx, holding); err != nil {
		return domain.Holding{}, err
	}
	return holding, nil
}

func (s *Stager) UpdateHolding(ctx context.Context, id domain.HoldingID, input HoldingUpdateInput) (domain.Holding, error) {
	if input.QuantitySet || input.Quantity != "" {
		return domain.Holding{}, &domain.Error{Code: domain.ErrInvalidChange, Field: "quantity", Message: "UpdateHolding only supports metadata; use RecordChange for quantity changes"}
	}
	return s.updateHolding(ctx, id, input)
}

func (s *Stager) updateHolding(ctx context.Context, id domain.HoldingID, input HoldingUpdateInput) (domain.Holding, error) {
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
	current.UpdatedAt = normalizeNow(s.clock())
	if input.NoteSet {
		current.Note = input.Note
	}
	if input.SortOrderSet {
		current.SortOrder = input.SortOrder
	}
	if err := s.writer.UpdateHolding(ctx, current); err != nil {
		return domain.Holding{}, err
	}
	return current, nil
}

func (s *Stager) AppendAccountCashValue(ctx context.Context, accountID domain.AccountID, amount, currency, effectiveAt string) (domain.AccountCashValue, error) {
	// Stage exact synthetic cash facts for the test origin.
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
	if err := s.writer.AppendAccountCashValue(ctx, value); err != nil {
		return domain.AccountCashValue{}, err
	}
	return value, nil
}

func (s *Stager) SetFXPreference(ctx context.Context, currencyA, currencyB, source string) (domain.FXPreference, error) {
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
	parsedSource, err := domain.ParseQuoteSourceKind(source)
	if err != nil {
		return domain.FXPreference{}, err
	}
	preference, err := domain.NewFXPreference(household.ID, a, b, parsedSource, s.clock())
	if err != nil {
		return domain.FXPreference{}, err
	}
	if err := s.writer.SetFXPreference(ctx, preference); err != nil {
		return domain.FXPreference{}, err
	}
	return preference, nil
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
	quoteSourceValue := input.QuoteSource
	if quoteSourceValue == "" {
		quoteSourceValue = string(domain.QuoteSourceManual)
	}
	quoteSource, err := domain.ParseQuoteSourceKind(quoteSourceValue)
	if err != nil {
		return domain.Instrument{}, err
	}
	return domain.NewInstrument(domain.InstrumentInput{
		HouseholdID: householdID, Name: input.Name, ShortName: pointerText(input.ShortName), Type: instrumentType, QuoteCurrency: quoteCurrency, MetalTemplate: input.MetalTemplate, QuantityUnit: input.QuantityUnit,
		Symbol: optionalText(input.Symbol), MarketCode: optionalText(input.MarketCode), CountryCode: optionalText(input.CountryCode), ISIN: optionalText(input.ISIN), Note: input.Note,
		IconKey: optionalText(input.IconKey), SortOrder: input.SortOrder, QuoteSource: quoteSource, ProviderKey: optionalText(input.ProviderKey), ProviderSymbol: optionalText(input.ProviderSymbol),
	}, now)
}

func mergeInstrumentInput(input *InstrumentInput, current domain.Instrument) {
	if input.MetalTemplate == "" {
		input.MetalTemplate = current.MetalTemplate
	}
	if input.QuantityUnit == "" {
		input.QuantityUnit = current.QuantityUnit
	}
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
	if input.IconKey == "" {
		input.IconKey = pointerText(current.IconKey)
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

func notFound(entity string) error {
	return &domain.Error{Code: domain.ErrNotFound, Message: entity + " was not found"}
}

func onboardingRequired() error {
	return &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
}

func normalizeNow(value time.Time) time.Time { return value.UTC().Truncate(time.Millisecond) }

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

func resolveOwnership(input AccountInput) ([]domain.OwnershipShare, error) {
	if len(input.Ownership) > 0 {
		return input.Ownership, nil
	}
	// An empty owner list is invalid. It is not defaulted to every household
	// member. Checked owners with blank percentages still even-split among
	// those checked owners.
	if len(input.OwnerIDs) == 0 {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "ownership", Message: "at least one owner is required"}
	}
	if len(input.OwnershipPercentages) == 0 {
		ownership, err := domain.EqualOwnership(input.OwnerIDs)
		if err != nil {
			return nil, err
		}
		return ownership.Shares(), nil
	}
	if len(input.OwnershipPercentages) != len(input.OwnerIDs) {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "ownership", Message: "one percentage is required for each owner"}
	}
	shares := make([]domain.OwnershipShare, len(input.OwnerIDs))
	for index, value := range input.OwnershipPercentages {
		bps, err := domain.PercentToBasisPoints(value)
		if err != nil {
			return nil, err
		}
		shares[index] = domain.OwnershipShare{MemberID: input.OwnerIDs[index], ShareBPS: bps}
	}
	ownership, err := domain.ParseOwnership(shares)
	if err != nil {
		return nil, err
	}
	return ownership.Shares(), nil
}

func validateOwnershipMembers(members []domain.Member, ownership []domain.OwnershipShare) error {
	active := make(map[domain.MemberID]bool, len(members))
	for _, member := range members {
		active[member.ID] = member.ArchivedAt == nil
	}
	for _, share := range ownership {
		if !active[share.MemberID] {
			return &domain.Error{Code: domain.ErrValidation, Field: "ownership", Message: "all owners must be active members"}
		}
	}
	return nil
}

func validateOwnershipMembersForUpdate(members []domain.Member, ownership, existing []domain.OwnershipShare) error {
	active := make(map[domain.MemberID]bool, len(members))
	for _, member := range members {
		active[member.ID] = member.ArchivedAt == nil
	}
	retained := make(map[domain.MemberID]bool, len(existing))
	for _, share := range existing {
		retained[share.MemberID] = true
	}
	for _, share := range ownership {
		if !active[share.MemberID] && !retained[share.MemberID] {
			return &domain.Error{Code: domain.ErrValidation, Field: "ownership", Message: "new owners must be active members"}
		}
	}
	return nil
}

func containsInstitution(items []domain.Institution, id *domain.InstitutionID) bool {
	if id == nil {
		return true
	}
	for _, item := range items {
		if item.ID == *id {
			return true
		}
	}
	return false
}

func containsGroup(items []domain.Group, id *domain.GroupID) bool {
	if id == nil {
		return true
	}
	for _, item := range items {
		if item.ID == *id {
			return true
		}
	}
	return false
}

func normalizeIconKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if err := domain.ValidateIconKey(value); err != nil {
		return "", err
	}
	if value == "" {
		return "", &domain.Error{Code: domain.ErrValidation, Field: "iconKey", Message: "icon is required"}
	}
	return value, nil
}

func (s *Stager) Bootstrap(ctx context.Context) (Bootstrap, error) {
	household, err := s.repository.Household(ctx)
	if err != nil {
		return Bootstrap{}, err
	}
	if household == nil {
		return Bootstrap{}, nil
	}
	members, err := s.repository.ListMembers(ctx, false)
	if err != nil {
		return Bootstrap{}, err
	}
	institutions, err := s.repository.ListInstitutions(ctx, false)
	if err != nil {
		return Bootstrap{}, err
	}
	groups, err := s.repository.ListGroups(ctx, false)
	if err != nil {
		return Bootstrap{}, err
	}
	return Bootstrap{Household: household, Members: members, Institutions: institutions, Groups: groups}, nil
}

func (s *Stager) requireHousehold(ctx context.Context) (domain.Household, error) {
	household, err := s.repository.Household(ctx)
	if err != nil {
		return domain.Household{}, err
	}
	if household == nil {
		return domain.Household{}, onboardingRequired()
	}
	return *household, nil
}

func (s *Stager) accountRecord(ctx context.Context, id domain.AccountID) (domain.AccountRecord, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	return s.repository.AccountRecord(ctx, household.ID, id)
}

func (s *Stager) portfolioSnapshot(ctx context.Context) (domain.PortfolioSnapshot, *domain.Household, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.PortfolioSnapshot{}, nil, err
	}
	return snapshot, snapshot.Household, nil
}
