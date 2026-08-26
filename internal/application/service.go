package application

import (
	"context"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// ImageNormalizer is the application port for bounded image decoding and
// PNG normalization. Infrastructure owns the concrete implementation so the
// application layer does not depend on a storage or codec package.
type ImageNormalizer interface {
	Normalize([]byte) ([]byte, error)
	ReadAndNormalize(io.Reader) ([]byte, error)
}

type Service struct {
	repository      Repository
	imageNormalizer ImageNormalizer
	valuation       *ValuationService
	gain            *GainService

	// stateMu guards the mutable service configuration below so a refresh
	// worker reading it never races a concurrent setter.
	stateMu       sync.RWMutex
	now           func() time.Time
	marketData    MarketDataRegistryPort
	fxProviderKey string

	changeMu sync.Mutex
}

func NewService(repository Repository, registries ...MarketDataRegistryPort) *Service {
	service := &Service{repository: repository, now: time.Now}
	service.valuation = NewValuationService(repository, service.clock)
	service.gain = NewGainService(repository, service.clock)
	if len(registries) > 0 {
		service.marketData = registries[0]
	}
	return service
}

// NewServiceWithImageNormalizer wires the concrete image boundary without
// making application depend on infrastructure/media. NewService remains
// available for callers that do not use image operations.
func NewServiceWithImageNormalizer(repository Repository, normalizer ImageNormalizer, registries ...MarketDataRegistryPort) *Service {
	service := NewService(repository, registries...)
	service.imageNormalizer = normalizer
	return service
}

func (s *Service) MarketDataRegistry() MarketDataRegistryPort {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.marketData
}

func (s *Service) SetMarketDataRegistry(registry MarketDataRegistryPort) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.marketData = registry
}

// SetFXProvider selects the registered provider used by explicit FX refresh.
// Instrument refresh continues to use each Instrument's own binding.
func (s *Service) SetFXProvider(key string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		key = FrankfurterProviderKey
	}
	registry := s.MarketDataRegistry()
	if registry == nil {
		return &domain.Error{Code: domain.ErrUnavailable, Field: "fxProvider", Message: "provider is not configured"}
	}
	provider, err := registry.Resolve(key)
	if err != nil {
		return err
	}
	if !provider.Capabilities().LatestFX {
		return &domain.Error{Code: domain.ErrUnavailable, Field: "fxProvider", Message: "provider does not support FX refresh"}
	}
	s.stateMu.Lock()
	s.fxProviderKey = key
	s.stateMu.Unlock()
	return nil
}

// FXProviderKey returns the explicit selection, or the registry default for
// deterministic test registries that have not configured one.
func (s *Service) FXProviderKey() string {
	s.stateMu.RLock()
	key := s.fxProviderKey
	registry := s.marketData
	s.stateMu.RUnlock()
	if key != "" {
		return key
	}
	if registry != nil {
		if provider, err := registry.Default(); err == nil {
			return strings.ToLower(strings.TrimSpace(provider.Key()))
		}
	}
	return ""
}

func (s *Service) setClock(now func() time.Time) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.now = now
}

// clock reads the configured clock through stateMu. It doubles as the
// injectable clock value for services that take a func() time.Time.
func (s *Service) clock() time.Time {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.now()
}

type Bootstrap struct {
	Household    *domain.Household
	Members      []domain.Member
	Institutions []domain.Institution
	Groups       []domain.Group
}

func (s *Service) Bootstrap(ctx context.Context) (Bootstrap, error) {
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

// requireHousehold is the identity-only gate for mutations and media reads
// that need the current Household ID but do not validate directory references.
// Keep Bootstrap for callers that genuinely need active Members, Institutions,
// or Groups.
func (s *Service) requireHousehold(ctx context.Context) (domain.Household, error) {
	household, err := s.repository.Household(ctx)
	if err != nil {
		return domain.Household{}, err
	}
	if household == nil {
		return domain.Household{}, onboardingRequired()
	}
	return *household, nil
}

// Household is the identity-only read used by Wails adapters that must
// inject the current Household ID instead of trusting a client-submitted
// value.
func (s *Service) Household(ctx context.Context) (domain.Household, error) {
	return s.requireHousehold(ctx)
}

type OnboardingInput struct {
	HouseholdName string
	BaseCurrency  string
	MemberNames   []string
	Timezone      string
}

func (s *Service) ListMembers(ctx context.Context, includeArchived bool) ([]domain.Member, error) {
	return s.repository.ListMembers(ctx, includeArchived)
}
func (s *Service) ListInstitutions(ctx context.Context, includeArchived bool) ([]domain.Institution, error) {
	return s.repository.ListInstitutions(ctx, includeArchived)
}
func (s *Service) ListGroups(ctx context.Context, includeArchived bool) ([]domain.Group, error) {
	return s.repository.ListGroups(ctx, includeArchived)
}

func (s *Service) CompleteOnboarding(ctx context.Context, input OnboardingInput) error {
	if household, err := s.repository.Household(ctx); err != nil {
		return err
	} else if household != nil {
		return &domain.Error{Code: domain.ErrConflict, Message: "a Household already exists"}
	}
	currency, err := domain.ParseCurrency(strings.ToUpper(strings.TrimSpace(input.BaseCurrency)))
	if err != nil {
		return err
	}
	household, err := domain.NewHousehold(input.HouseholdName, currency, s.clock())
	if err != nil {
		return err
	}
	if len(input.MemberNames) == 0 {
		return &domain.Error{Code: domain.ErrValidation, Field: "members", Message: "at least one member is required"}
	}
	members := make([]domain.Member, 0, len(input.MemberNames))
	for index, name := range input.MemberNames {
		member, err := domain.NewMember(household.ID, name, s.clock())
		if err != nil {
			return err
		}
		member.SortOrder = index
		members = append(members, member)
	}
	if strings.TrimSpace(input.Timezone) == "" {
		return s.repository.CreateOnboarding(ctx, household, members)
	}
	origin, err := domain.NewHistoryOrigin(household.ID, input.Timezone, s.clock(), s.clock())
	if err != nil {
		return err
	}
	return s.repository.CreateOnboardingWithHistory(ctx, household, members, domain.HistoryOriginData{Origin: origin})
}

func (s *Service) CreateMember(ctx context.Context, name string) (domain.Member, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.Member{}, err
	}
	member, err := domain.NewMember(household.ID, name, s.clock())
	if err != nil {
		return domain.Member{}, err
	}
	if err := s.repository.CreateMember(ctx, member); err != nil {
		return domain.Member{}, err
	}
	return member, nil
}
func (s *Service) UpdateMember(ctx context.Context, id domain.MemberID, name string) (domain.Member, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.Member{}, err
	}
	current, err := s.repository.Member(ctx, household.ID, id)
	if err != nil {
		return domain.Member{}, err
	}
	updated, err := domain.NewMember(current.HouseholdID, name, s.clock())
	if err != nil {
		return domain.Member{}, err
	}
	updated.ID, updated.AvatarAssetID, updated.Note, updated.SortOrder, updated.CreatedAt, updated.ArchivedAt = current.ID, current.AvatarAssetID, current.Note, current.SortOrder, current.CreatedAt, current.ArchivedAt
	if err := s.repository.UpdateMember(ctx, updated); err != nil {
		return domain.Member{}, err
	}
	return updated, nil
}

func (s *Service) ArchiveMember(ctx context.Context, id domain.MemberID, archived bool) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	return s.repository.SetMemberArchive(ctx, household.ID, id, archived, s.clock())
}

func (s *Service) CreateInstitution(ctx context.Context, name string, iconKeys ...string) (domain.Institution, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.Institution{}, err
	}
	institution, err := domain.NewInstitution(household.ID, name, s.clock())
	if err != nil {
		return domain.Institution{}, err
	}
	if err := applyOptionalIcon(&institution.IconKey, iconKeys); err != nil {
		return domain.Institution{}, err
	}
	if err := s.repository.CreateInstitution(ctx, institution); err != nil {
		return domain.Institution{}, err
	}
	return institution, nil
}
func (s *Service) UpdateInstitution(ctx context.Context, id domain.InstitutionID, name string) (domain.Institution, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.Institution{}, err
	}
	current, err := s.repository.Institution(ctx, household.ID, id)
	if err != nil {
		return domain.Institution{}, err
	}
	updated, err := domain.NewInstitution(current.HouseholdID, name, s.clock())
	if err != nil {
		return domain.Institution{}, err
	}
	updated.ID, updated.IconKey, updated.InstitutionType, updated.CountryCode, updated.Website, updated.Note, updated.LogoAssetID, updated.SortOrder, updated.CreatedAt, updated.ArchivedAt = current.ID, current.IconKey, current.InstitutionType, current.CountryCode, current.Website, current.Note, current.LogoAssetID, current.SortOrder, current.CreatedAt, current.ArchivedAt
	if err := s.repository.UpdateInstitution(ctx, updated); err != nil {
		return domain.Institution{}, err
	}
	return updated, nil
}

func (s *Service) ArchiveInstitution(ctx context.Context, id domain.InstitutionID, archived bool) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	return s.repository.SetInstitutionArchive(ctx, household.ID, id, archived, s.clock())
}

func (s *Service) CreateGroup(ctx context.Context, name string, iconKeys ...string) (domain.Group, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.Group{}, err
	}
	group, err := domain.NewGroup(household.ID, name, s.clock())
	if err != nil {
		return domain.Group{}, err
	}
	if err := applyOptionalIcon(&group.IconKey, iconKeys); err != nil {
		return domain.Group{}, err
	}
	if err := s.repository.CreateGroup(ctx, group); err != nil {
		return domain.Group{}, err
	}
	return group, nil
}
func (s *Service) UpdateGroup(ctx context.Context, id domain.GroupID, name string) (domain.Group, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.Group{}, err
	}
	current, err := s.repository.Group(ctx, household.ID, id)
	if err != nil {
		return domain.Group{}, err
	}
	updated, err := domain.NewGroup(current.HouseholdID, name, s.clock())
	if err != nil {
		return domain.Group{}, err
	}
	updated.ID, updated.IconKey, updated.Color, updated.LogoAssetID, updated.Description, updated.SortOrder, updated.CreatedAt, updated.ArchivedAt = current.ID, current.IconKey, current.Color, current.LogoAssetID, current.Description, current.SortOrder, current.CreatedAt, current.ArchivedAt
	if err := s.repository.UpdateGroup(ctx, updated); err != nil {
		return domain.Group{}, err
	}
	return updated, nil
}

func (s *Service) ArchiveGroup(ctx context.Context, id domain.GroupID, archived bool) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	return s.repository.SetGroupArchive(ctx, household.ID, id, archived, s.clock())
}

func (s *Service) CreateMediaAsset(ctx context.Context, mimeType string, data []byte) (domain.MediaAsset, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.MediaAsset{}, err
	}
	if mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/webp" {
		return domain.MediaAsset{}, &domain.Error{Code: domain.ErrValidation, Field: "mimeType", Message: "unsupported image type"}
	}
	if s.imageNormalizer == nil {
		return domain.MediaAsset{}, &domain.Error{Code: domain.ErrUnavailable, Field: "image", Message: "image normalization is not configured"}
	}
	normalized, normalizeErr := s.imageNormalizer.Normalize(data)
	if normalizeErr != nil {
		return domain.MediaAsset{}, normalizeErr
	}
	asset := domain.MediaAsset{ID: domain.NewMediaAssetID(), HouseholdID: household.ID, MimeType: "image/png", Data: normalized, CreatedAt: s.clock()}
	if err := s.repository.CreateMediaAsset(ctx, asset); err != nil {
		return domain.MediaAsset{}, err
	}
	return asset, nil
}

// NormalizeImage reads an uploaded image and normalizes it to PNG without
// persisting anything. The UI calls this at pick time; persistence happens
// via CreateMediaAsset when the user confirms the surrounding form.
func (s *Service) NormalizeImage(reader io.Reader) ([]byte, error) {
	if s.imageNormalizer == nil {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Field: "image", Message: "image normalization is not configured"}
	}
	return s.imageNormalizer.ReadAndNormalize(reader)
}

func (s *Service) MediaAsset(ctx context.Context, id domain.MediaAssetID) (domain.MediaAsset, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.MediaAsset{}, err
	}
	return s.repository.MediaAsset(ctx, household.ID, id)
}
func (s *Service) SetMemberAvatar(ctx context.Context, id domain.MemberID, asset domain.MediaAssetID) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	return s.repository.SetMemberAvatar(ctx, household.ID, id, asset, s.clock())
}
func (s *Service) SetInstitutionLogo(ctx context.Context, id domain.InstitutionID, asset domain.MediaAssetID) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	return s.repository.SetInstitutionLogo(ctx, household.ID, id, asset, s.clock())
}
func (s *Service) SetGroupLogo(ctx context.Context, id domain.GroupID, asset domain.MediaAssetID) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	return s.repository.SetGroupLogo(ctx, household.ID, id, asset, s.clock())
}
func (s *Service) SetAccountLogo(ctx context.Context, id domain.AccountID, asset domain.MediaAssetID) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	return s.repository.SetAccountLogo(ctx, household.ID, id, asset, s.clock())
}

func (s *Service) SetInstitutionIcon(ctx context.Context, id domain.InstitutionID, iconKey string) error {
	return s.setIcon(ctx, iconKey, func(householdID domain.HouseholdID, normalized string, now time.Time) error {
		return s.repository.SetInstitutionIcon(ctx, householdID, id, normalized, now)
	})
}

func (s *Service) SetGroupIcon(ctx context.Context, id domain.GroupID, iconKey string) error {
	return s.setIcon(ctx, iconKey, func(householdID domain.HouseholdID, normalized string, now time.Time) error {
		return s.repository.SetGroupIcon(ctx, householdID, id, normalized, now)
	})
}

func (s *Service) SetAccountIcon(ctx context.Context, id domain.AccountID, iconKey string) error {
	return s.setIcon(ctx, iconKey, func(householdID domain.HouseholdID, normalized string, now time.Time) error {
		return s.repository.SetAccountIcon(ctx, householdID, id, normalized, now)
	})
}

func (s *Service) setIcon(ctx context.Context, iconKey string, save func(domain.HouseholdID, string, time.Time) error) error {
	normalized, err := normalizeIconKey(iconKey)
	if err != nil {
		return err
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	return save(household.ID, normalized, s.clock())
}

type AccountInput struct {
	Name                     string
	PrimaryCategory          string
	SecondaryCategory        string
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
	IncludeInInvestment      bool
	IncludeInInvestmentSet   bool
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

func (s *Service) CreateAccount(ctx context.Context, input AccountInput) (domain.AccountRecord, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if bootstrap.Household == nil {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	// The Starting-point read and the history-chain commit below must be
	// atomic against other change writers.
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	origin, err := s.repository.HistoryOrigin(ctx, bootstrap.Household.ID)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	primary, err := domain.ParsePrimaryCategory(input.PrimaryCategory)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	secondary, err := domain.ParseSecondaryCategory(input.SecondaryCategory)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	mode, err := domain.ParseTrackingMode(input.TrackingMode)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	currency, err := domain.ParseCurrency(strings.ToUpper(strings.TrimSpace(input.DefaultCurrency)))
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
	accountInput := domain.AccountInput{HouseholdID: bootstrap.Household.ID, Name: input.Name, PrimaryCategory: primary, SecondaryCategory: secondary, TrackingMode: mode, DefaultCurrency: currency, Note: input.Note, IconKey: iconKey, IncludeInNetWorth: input.IncludeInNetWorth, IncludeInInvestment: input.IncludeInInvestment, IncludeInLiquidAssets: input.IncludeInLiquidAssets, OpenedOn: input.OpenedOn, ClosedOn: input.ClosedOn, Ownership: ownershipShares, InitialAmount: input.InitialAmount}
	var historyInitial *domain.Money
	if origin != nil && strings.TrimSpace(input.InitialAmount) != "" && mode != domain.TrackingHoldings {
		parsedInitial, parseErr := domain.ParseMoney(input.InitialAmount, currency)
		if parseErr != nil {
			return domain.AccountRecord{}, parseErr
		}
		if !parsedInitial.IsZero() {
			historyInitial = &parsedInitial
			accountInput.InitialAmount = "0"
		}
	}
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
		created, valueErr := domain.NewAccountValue(account, *initial, s.clock(), s.clock())
		if valueErr != nil {
			return domain.AccountRecord{}, valueErr
		}
		value = &created
	}
	if origin == nil {
		if err := s.repository.CreateAccount(ctx, account, ownership, value); err != nil {
			return domain.AccountRecord{}, err
		}
		return domain.AccountRecord{Account: account, Ownership: ownership, LatestValue: value}, nil
	}
	creationObservation := domain.AccountStateObservation{
		ID:                    domain.NewAccountStateObservationID(),
		AccountID:             account.ID,
		EffectiveAt:           account.CreatedAt,
		ArchivedAt:            account.ArchivedAt,
		IncludeInNetWorth:     account.IncludeInNetWorth,
		IncludeInInvestment:   account.IncludeInInvestment,
		IncludeInLiquidAssets: account.IncludeInLiquidAssets,
		CreatedAt:             account.CreatedAt,
		Ownership:             ownership.Shares(),
	}
	if historyInitial == nil {
		if err := s.repository.CreateAccountWithHistory(ctx, account, ownership, value, creationObservation, nil, s.clock()); err != nil {
			return domain.AccountRecord{}, err
		}
		return domain.AccountRecord{Account: account, Ownership: ownership, LatestValue: value}, nil
	}
	zero, zeroErr := domain.ParseMoney("0", account.DefaultCurrency)
	if zeroErr != nil {
		return domain.AccountRecord{}, zeroErr
	}
	state := domain.ChangeState{HouseholdID: account.HouseholdID, OriginAt: origin.StartedAt, Timezone: origin.Timezone, Now: s.clock(), Accounts: map[domain.AccountID]domain.ChangeAccountState{account.ID: {ID: account.ID, Name: account.Name, Currency: account.DefaultCurrency, Mode: account.TrackingMode, Liability: account.PrimaryCategory.IsLiability(), Current: zero}}, Cash: make(map[domain.AccountID]map[domain.CurrencyCode]domain.Money), Holdings: make(map[domain.HoldingID]domain.ChangeHoldingState)}
	preview, previewErr := domain.PreviewChange(state, domain.MoneyAddedInput{HouseholdID: account.HouseholdID, AccountID: account.ID, Amount: *historyInitial, Reason: domain.ReasonContribution, EffectiveAt: account.CreatedAt})
	if previewErr != nil {
		return domain.AccountRecord{}, previewErr
	}
	commit := domain.ActivityCommit{Activity: preview.Activity, Effects: preview.Effects, Resulting: preview.Resulting}
	if err := s.repository.CreateAccountWithHistory(ctx, account, ownership, value, creationObservation, &commit, s.clock()); err != nil {
		return domain.AccountRecord{}, err
	}
	resultMoney, parseErr := domain.ParseMoney(preview.Resulting[0].Amount, preview.Resulting[0].Currency)
	if parseErr != nil {
		return domain.AccountRecord{}, parseErr
	}
	resultValue, valueErr := domain.NewAccountValue(account, resultMoney, preview.Activity.EffectiveAt, preview.Activity.CreatedAt)
	if valueErr != nil {
		return domain.AccountRecord{}, valueErr
	}
	return domain.AccountRecord{Account: account, Ownership: ownership, LatestValue: &resultValue}, nil
}

// UpdateAccount changes metadata and ownership without rewriting AccountValue history.
func (s *Service) UpdateAccount(ctx context.Context, id domain.AccountID, input AccountInput) (domain.AccountRecord, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	currentRecord, err := s.accountRecord(ctx, id)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	current := &currentRecord
	if current.LatestValue == nil && current.Account.TrackingMode != domain.TrackingHoldings {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Message: "account has no current value"}
	}
	if input.PrimaryCategory == "" {
		input.PrimaryCategory = current.Account.PrimaryCategory.String()
	}
	if input.SecondaryCategory == "" {
		input.SecondaryCategory = string(current.Account.SecondaryCategory)
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
	if len(input.Ownership) == 0 && len(input.OwnerIDs) == 0 {
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
	if !input.IncludeInInvestmentSet {
		input.IncludeInInvestment = current.Account.IncludeInInvestment
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
	primary, err := domain.ParsePrimaryCategory(input.PrimaryCategory)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	secondary, err := domain.ParseSecondaryCategory(input.SecondaryCategory)
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
	account, ownership, _, err := domain.NewAccount(domain.AccountInput{HouseholdID: current.Account.HouseholdID, InstitutionID: institutionID, GroupID: groupID, Name: input.Name, PrimaryCategory: primary, SecondaryCategory: secondary, TrackingMode: mode, DefaultCurrency: currency, Note: input.Note, IconKey: iconKey, IncludeInNetWorth: input.IncludeInNetWorth, IncludeInInvestment: input.IncludeInInvestment, IncludeInLiquidAssets: input.IncludeInLiquidAssets, OpenedOn: input.OpenedOn, ClosedOn: input.ClosedOn, SortOrder: current.Account.SortOrder, Ownership: ownershipShares, InitialAmount: input.InitialAmount}, s.clock())
	if err != nil {
		return domain.AccountRecord{}, err
	}
	account.ID, account.CreatedAt, account.ArchivedAt, account.IconKey, account.LogoAssetID = current.Account.ID, current.Account.CreatedAt, current.Account.ArchivedAt, current.Account.IconKey, current.Account.LogoAssetID
	allMembers, err := s.repository.ListMembers(ctx, true)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if err := validateOwnershipMembersForUpdate(allMembers, ownership.Shares(), current.Ownership.Shares()); err != nil {
		return domain.AccountRecord{}, err
	}
	observation, observationErr := s.accountStateObservation(ctx, account, ownership)
	if observationErr != nil {
		return domain.AccountRecord{}, observationErr
	}
	if observation.ID != "" {
		if err := s.repository.UpdateAccountWithObservation(ctx, account, ownership, observation); err != nil {
			return domain.AccountRecord{}, err
		}
	} else if err := s.repository.UpdateAccount(ctx, account, ownership); err != nil {
		return domain.AccountRecord{}, err
	}
	current.Account, current.Ownership = account, ownership
	return *current, nil
}

func (s *Service) ListAccounts(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountRecord, error) {
	household, err := s.repository.Household(ctx)
	if err != nil {
		return nil, err
	}
	if household == nil {
		return []domain.AccountRecord{}, nil
	}
	return s.repository.ListAccountRecords(ctx, household.ID, filter)
}

func (s *Service) accountRecord(ctx context.Context, id domain.AccountID) (domain.AccountRecord, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	return s.repository.AccountRecord(ctx, household.ID, id)
}

func (s *Service) AppendAccountValue(ctx context.Context, accountID domain.AccountID, amount, effectiveAt string) (domain.AccountValue, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
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
	if strings.TrimSpace(effectiveAt) != "" {
		parsed, parseErr := time.Parse("2006-01-02", effectiveAt)
		if parseErr != nil {
			return domain.AccountValue{}, &domain.Error{Code: domain.ErrValidation, Field: "effectiveAt", Message: "must use YYYY-MM-DD"}
		}
		when = parsed.UTC()
	}
	origin, originErr := s.repository.HistoryOrigin(ctx, record.Account.HouseholdID)
	if originErr != nil {
		return domain.AccountValue{}, originErr
	}
	if origin != nil {
		preview, commitErr := s.recordChangeLocked(ctx, domain.ValueUpdateInput{HouseholdID: record.Account.HouseholdID, AccountID: accountID, NewValue: money, Reason: domain.ReasonReconciliation, EffectiveAt: when})
		if commitErr != nil {
			return domain.AccountValue{}, commitErr
		}
		resultMoney, parseErr := domain.ParseMoney(preview.Resulting[0].Amount, preview.Resulting[0].Currency)
		if parseErr != nil {
			return domain.AccountValue{}, parseErr
		}
		return domain.NewAccountValue(record.Account, resultMoney, preview.Activity.EffectiveAt, preview.Activity.CreatedAt)
	}
	value, err := domain.NewAccountValue(record.Account, money, when, s.clock())
	if err != nil {
		return domain.AccountValue{}, err
	}
	if err := s.repository.AppendAccountValue(ctx, value); err != nil {
		return domain.AccountValue{}, err
	}
	return value, nil
}

func (s *Service) ArchiveAccount(ctx context.Context, id domain.AccountID, archived bool) error {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
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
	observation, observationErr := s.accountStateObservation(ctx, current.Account, current.Ownership)
	if observationErr != nil {
		return observationErr
	}
	if observation.ID != "" {
		return s.repository.SetAccountArchiveWithObservation(ctx, household.ID, id, archived, now, observation)
	}
	return s.repository.SetAccountArchive(ctx, household.ID, id, archived, now)
}

func (s *Service) AccountValuation(ctx context.Context, id domain.AccountID) (domain.AccountValuation, error) {
	return s.valuation.Account(ctx, id)
}

// HoldingGain returns the derived cost and gain view for one Holding.
func (s *Service) HoldingGain(ctx context.Context, id domain.HoldingID) (domain.HoldingGainView, error) {
	return s.gain.HoldingGain(ctx, id)
}

// AccountGain returns the derived cost and gain views for all active Holdings
// in one account.
func (s *Service) AccountGain(ctx context.Context, id domain.AccountID) (domain.AccountGainView, error) {
	return s.gain.AccountGain(ctx, id)
}

// RealizedGainInRange returns realized gains grouped by Instrument and
// Account for an inclusive local-date range.
func (s *Service) RealizedGainInRange(ctx context.Context, scope domain.GainScope, from, to domain.LocalDate) (domain.RealizedGainView, error) {
	return s.gain.RealizedGainInRange(ctx, scope, from, to)
}

// RealizedGain resolves an Analytics trend range through GainService.
func (s *Service) RealizedGain(ctx context.Context, scope domain.GainScope, trendRange domain.TrendRange) (domain.RealizedGainView, error) {
	return s.gain.RealizedGain(ctx, scope, trendRange)
}

func (s *Service) AccountValuations(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountValuation, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, filter)
	if err != nil {
		return nil, err
	}
	valuations, _, err := s.valuation.ValueAccounts(snapshot)
	return valuations, err
}

func (s *Service) Portfolio(ctx context.Context, filter domain.AccountFilter) (domain.PortfolioValuation, error) {
	filter.IncludeArchived = false
	return s.valuation.Portfolio(ctx, filter)
}

func (s *Service) Overview(ctx context.Context, filter domain.AccountFilter) (domain.OverviewResult, error) {
	filter.IncludeArchived = false
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, filter)
	if err != nil {
		return domain.OverviewResult{}, err
	}
	if snapshot.Household == nil {
		return domain.OverviewResult{}, nil
	}
	valuations, _, err := s.valuation.ValueAccounts(snapshot)
	if err != nil {
		return domain.OverviewResult{}, err
	}
	result := domain.OverviewResult{Currency: snapshot.Household.BaseCurrency, AccountCount: len(snapshot.Accounts), Complete: true}
	category := map[string]decimal.Decimal{}
	member := map[string]decimal.Decimal{}
	institution := map[string]decimal.Decimal{}
	group := map[string]decimal.Decimal{}
	memberLabels := map[string]string{}
	institutionLabels := map[string]string{}
	groupLabels := map[string]string{}
	for _, current := range snapshot.Members {
		memberLabels[current.ID.String()] = current.Name
	}
	for _, current := range snapshot.Institutions {
		institutionLabels[current.ID.String()] = current.Name
	}
	for _, current := range snapshot.Groups {
		groupLabels[current.ID.String()] = current.Name
	}
	for _, valuation := range valuations {
		if !valuation.Account.IncludeInNetWorth {
			continue
		}
		value, err := exactBaseAmount(valuation)
		if err != nil {
			return domain.OverviewResult{}, err
		}
		if !valuation.Complete {
			result.Complete = false
			result.MissingInputs = append(result.MissingInputs, valuation.MissingInputs...)
		}
		if valuation.Account.PrimaryCategory.IsLiability() {
			result.Liabilities = result.Liabilities.Add(value)
			continue
		}
		result.Assets = result.Assets.Add(value)
		categoryKey := valuation.Account.PrimaryCategory.String()
		category[categoryKey] = category[categoryKey].Add(value)
		for _, share := range valuation.Ownership.Shares() {
			member[share.MemberID.String()] = member[share.MemberID.String()].Add(value.Mul(decimal.NewFromInt(int64(share.ShareBPS))).Div(decimal.NewFromInt(domain.TotalOwnershipBPS)))
		}
		institutionKey := "unassigned"
		if valuation.Account.InstitutionID != nil {
			institutionKey = valuation.Account.InstitutionID.String()
			if valuation.InstitutionName != "" {
				institutionLabels[institutionKey] = valuation.InstitutionName
			}
		}
		institution[institutionKey] = institution[institutionKey].Add(value)
		groupKey := "unassigned"
		if valuation.Account.GroupID != nil {
			groupKey = valuation.Account.GroupID.String()
			if valuation.GroupName != "" {
				groupLabels[groupKey] = valuation.GroupName
			}
		}
		group[groupKey] = group[groupKey].Add(value)
	}
	sortMissing(result.MissingInputs)
	result.MissingInputs = deduplicateMissing(result.MissingInputs)
	result.NetWorth = result.Assets.Sub(result.Liabilities)
	result.ByCategory = makeBreakdown(category, result.Assets)
	result.ByMember = makeBreakdownWithLabels(member, result.Assets, memberLabels)
	result.ByInstitution = makeBreakdownWithLabels(institution, result.Assets, institutionLabels)
	result.ByGroup = makeBreakdownWithLabels(group, result.Assets, groupLabels)
	return result, nil
}

func resolveOwnership(input AccountInput) ([]domain.OwnershipShare, error) {
	if len(input.Ownership) > 0 {
		return input.Ownership, nil
	}
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

func makeBreakdown(values map[string]decimal.Decimal, denominator decimal.Decimal) []domain.BreakdownItem {
	return makeBreakdownWithLabels(values, denominator, nil)
}
func makeBreakdownWithLabels(values map[string]decimal.Decimal, denominator decimal.Decimal, labels map[string]string) []domain.BreakdownItem {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	shares := make(map[string]int, len(keys))
	type remainder struct {
		key   string
		value decimal.Decimal
	}
	remainders := make([]remainder, 0, len(keys))
	allocated := 0
	if !denominator.IsZero() {
		for _, key := range keys {
			exact := values[key].Mul(decimal.NewFromInt(domain.TotalOwnershipBPS)).Div(denominator)
			floor := int(exact.IntPart())
			shares[key] = floor
			allocated += floor
			remainders = append(remainders, remainder{key: key, value: exact.Sub(decimal.NewFromInt(int64(floor)))})
		}
		remaining := domain.TotalOwnershipBPS - allocated
		sort.SliceStable(remainders, func(i, j int) bool {
			if remainders[i].value.Equal(remainders[j].value) {
				return remainders[i].key < remainders[j].key
			}
			return remainders[i].value.GreaterThan(remainders[j].value)
		})
		for index := 0; index < remaining && index < len(remainders); index++ {
			shares[remainders[index].key]++
		}
	}
	result := make([]domain.BreakdownItem, 0, len(keys))
	for _, key := range keys {
		label := key
		if labels != nil && labels[key] != "" {
			label = labels[key]
		}
		result = append(result, domain.BreakdownItem{Key: key, Label: label, Amount: values[key], ShareBPS: shares[key]})
	}
	return result
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

func applyOptionalIcon(target **string, keys []string) error {
	if len(keys) == 0 || strings.TrimSpace(keys[0]) == "" {
		return nil
	}
	key, err := normalizeIconKey(keys[0])
	if err != nil {
		return err
	}
	*target = &key
	return nil
}
