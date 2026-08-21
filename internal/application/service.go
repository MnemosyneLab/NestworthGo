package application

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/media"
)

// Repository is the application boundary implemented by infrastructure.
type Repository interface {
	Household(context.Context) (*domain.Household, error)
	CreateOnboarding(context.Context, domain.Household, []domain.Member) error
	ListMembers(context.Context, bool) ([]domain.Member, error)
	CreateMember(context.Context, domain.Member) error
	UpdateMember(context.Context, domain.Member) error
	SetMemberArchive(context.Context, domain.HouseholdID, domain.MemberID, bool, time.Time) error
	CreateInstitution(context.Context, domain.Institution) error
	ListInstitutions(context.Context, bool) ([]domain.Institution, error)
	UpdateInstitution(context.Context, domain.Institution) error
	SetInstitutionArchive(context.Context, domain.HouseholdID, domain.InstitutionID, bool, time.Time) error
	CreateGroup(context.Context, domain.Group) error
	ListGroups(context.Context, bool) ([]domain.Group, error)
	UpdateGroup(context.Context, domain.Group) error
	SetGroupArchive(context.Context, domain.HouseholdID, domain.GroupID, bool, time.Time) error
	CreateMediaAsset(context.Context, domain.MediaAsset) error
	MediaAsset(context.Context, domain.HouseholdID, domain.MediaAssetID) (domain.MediaAsset, error)
	SetMemberAvatar(context.Context, domain.HouseholdID, domain.MemberID, domain.MediaAssetID) error
	SetInstitutionLogo(context.Context, domain.HouseholdID, domain.InstitutionID, domain.MediaAssetID) error
	SetGroupLogo(context.Context, domain.HouseholdID, domain.GroupID, domain.MediaAssetID) error
	SetAccountLogo(context.Context, domain.HouseholdID, domain.AccountID, domain.MediaAssetID) error
	SetInstitutionIcon(context.Context, domain.HouseholdID, domain.InstitutionID, string) error
	SetGroupIcon(context.Context, domain.HouseholdID, domain.GroupID, string) error
	SetAccountIcon(context.Context, domain.HouseholdID, domain.AccountID, string) error
	CreateAccount(context.Context, domain.Account, domain.Ownership, domain.AccountValue) error
	UpdateAccount(context.Context, domain.Account, domain.Ownership) error
	AppendAccountValue(context.Context, domain.AccountValue) error
	SetAccountArchive(context.Context, domain.HouseholdID, domain.AccountID, bool, time.Time) error
	ListAccountRecords(context.Context, domain.HouseholdID, domain.AccountFilter) ([]domain.AccountRecord, error)
	ReadSnapshot(context.Context, domain.AccountFilter) (domain.ReadSnapshot, error)
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (s *Service) setClock(now func() time.Time) { s.now = now }

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

type OnboardingInput struct {
	HouseholdName string
	BaseCurrency  string
	MemberNames   []string
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
	household, err := domain.NewHousehold(input.HouseholdName, currency, s.now())
	if err != nil {
		return err
	}
	if len(input.MemberNames) == 0 {
		return &domain.Error{Code: domain.ErrValidation, Field: "members", Message: "at least one member is required"}
	}
	members := make([]domain.Member, 0, len(input.MemberNames))
	for index, name := range input.MemberNames {
		member, err := domain.NewMember(household.ID, name, s.now())
		if err != nil {
			return err
		}
		member.SortOrder = index
		members = append(members, member)
	}
	return s.repository.CreateOnboarding(ctx, household, members)
}

func (s *Service) CreateMember(ctx context.Context, name string) (domain.Member, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.Member{}, err
	}
	if bootstrap.Household == nil {
		return domain.Member{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	member, err := domain.NewMember(bootstrap.Household.ID, name, s.now())
	if err != nil {
		return domain.Member{}, err
	}
	if err := s.repository.CreateMember(ctx, member); err != nil {
		return domain.Member{}, err
	}
	return member, nil
}
func (s *Service) UpdateMember(ctx context.Context, id domain.MemberID, name string) (domain.Member, error) {
	items, err := s.repository.ListMembers(ctx, true)
	if err != nil {
		return domain.Member{}, err
	}
	for _, current := range items {
		if current.ID != id {
			continue
		}
		updated, err := domain.NewMember(current.HouseholdID, name, s.now())
		if err != nil {
			return domain.Member{}, err
		}
		updated.ID, updated.AvatarAssetID, updated.Note, updated.SortOrder, updated.CreatedAt, updated.ArchivedAt = current.ID, current.AvatarAssetID, current.Note, current.SortOrder, current.CreatedAt, current.ArchivedAt
		if err := s.repository.UpdateMember(ctx, updated); err != nil {
			return domain.Member{}, err
		}
		return updated, nil
	}
	return domain.Member{}, &domain.Error{Code: domain.ErrNotFound, Message: "member was not found"}
}

func (s *Service) ArchiveMember(ctx context.Context, id domain.MemberID, archived bool) error {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil || bootstrap.Household == nil {
		if err != nil {
			return err
		}
		return &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	return s.repository.SetMemberArchive(ctx, bootstrap.Household.ID, id, archived, s.now())
}

func (s *Service) CreateInstitution(ctx context.Context, name string, iconKeys ...string) (domain.Institution, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.Institution{}, err
	}
	if bootstrap.Household == nil {
		return domain.Institution{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	institution, err := domain.NewInstitution(bootstrap.Household.ID, name, s.now())
	if err != nil {
		return domain.Institution{}, err
	}
	if len(iconKeys) > 0 && strings.TrimSpace(iconKeys[0]) != "" {
		iconKey, iconErr := normalizeIconKey(iconKeys[0])
		if iconErr != nil {
			return domain.Institution{}, iconErr
		}
		institution.IconKey = &iconKey
	}
	if err := s.repository.CreateInstitution(ctx, institution); err != nil {
		return domain.Institution{}, err
	}
	return institution, nil
}
func (s *Service) UpdateInstitution(ctx context.Context, id domain.InstitutionID, name string) (domain.Institution, error) {
	items, err := s.repository.ListInstitutions(ctx, true)
	if err != nil {
		return domain.Institution{}, err
	}
	for _, current := range items {
		if current.ID != id {
			continue
		}
		updated, err := domain.NewInstitution(current.HouseholdID, name, s.now())
		if err != nil {
			return domain.Institution{}, err
		}
		updated.ID, updated.IconKey, updated.InstitutionType, updated.CountryCode, updated.Website, updated.Note, updated.LogoAssetID, updated.SortOrder, updated.CreatedAt, updated.ArchivedAt = current.ID, current.IconKey, current.InstitutionType, current.CountryCode, current.Website, current.Note, current.LogoAssetID, current.SortOrder, current.CreatedAt, current.ArchivedAt
		if err := s.repository.UpdateInstitution(ctx, updated); err != nil {
			return domain.Institution{}, err
		}
		return updated, nil
	}
	return domain.Institution{}, &domain.Error{Code: domain.ErrNotFound, Message: "institution was not found"}
}

func (s *Service) ArchiveInstitution(ctx context.Context, id domain.InstitutionID, archived bool) error {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil || bootstrap.Household == nil {
		if err != nil {
			return err
		}
		return &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	return s.repository.SetInstitutionArchive(ctx, bootstrap.Household.ID, id, archived, s.now())
}

func (s *Service) CreateGroup(ctx context.Context, name string, iconKeys ...string) (domain.Group, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.Group{}, err
	}
	if bootstrap.Household == nil {
		return domain.Group{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	group, err := domain.NewGroup(bootstrap.Household.ID, name, s.now())
	if err != nil {
		return domain.Group{}, err
	}
	if len(iconKeys) > 0 && strings.TrimSpace(iconKeys[0]) != "" {
		iconKey, iconErr := normalizeIconKey(iconKeys[0])
		if iconErr != nil {
			return domain.Group{}, iconErr
		}
		group.IconKey = &iconKey
	}
	if err := s.repository.CreateGroup(ctx, group); err != nil {
		return domain.Group{}, err
	}
	return group, nil
}
func (s *Service) UpdateGroup(ctx context.Context, id domain.GroupID, name string) (domain.Group, error) {
	items, err := s.repository.ListGroups(ctx, true)
	if err != nil {
		return domain.Group{}, err
	}
	for _, current := range items {
		if current.ID != id {
			continue
		}
		updated, err := domain.NewGroup(current.HouseholdID, name, s.now())
		if err != nil {
			return domain.Group{}, err
		}
		updated.ID, updated.IconKey, updated.Color, updated.LogoAssetID, updated.Description, updated.SortOrder, updated.CreatedAt, updated.ArchivedAt = current.ID, current.IconKey, current.Color, current.LogoAssetID, current.Description, current.SortOrder, current.CreatedAt, current.ArchivedAt
		if err := s.repository.UpdateGroup(ctx, updated); err != nil {
			return domain.Group{}, err
		}
		return updated, nil
	}
	return domain.Group{}, &domain.Error{Code: domain.ErrNotFound, Message: "group was not found"}
}

func (s *Service) ArchiveGroup(ctx context.Context, id domain.GroupID, archived bool) error {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if bootstrap.Household == nil {
		return &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	return s.repository.SetGroupArchive(ctx, bootstrap.Household.ID, id, archived, s.now())
}

func (s *Service) CreateMediaAsset(ctx context.Context, mimeType string, data []byte) (domain.MediaAsset, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.MediaAsset{}, err
	}
	if bootstrap.Household == nil {
		return domain.MediaAsset{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	if mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/webp" {
		return domain.MediaAsset{}, &domain.Error{Code: domain.ErrValidation, Field: "mimeType", Message: "unsupported image type"}
	}
	normalized, normalizeErr := media.Normalize(data)
	if normalizeErr != nil {
		return domain.MediaAsset{}, &domain.Error{Code: domain.ErrValidation, Field: "data", Message: "image is invalid or exceeds the local size limit"}
	}
	asset := domain.MediaAsset{ID: domain.NewMediaAssetID(), HouseholdID: bootstrap.Household.ID, MimeType: "image/png", Data: normalized, CreatedAt: s.now()}
	if err := s.repository.CreateMediaAsset(ctx, asset); err != nil {
		return domain.MediaAsset{}, err
	}
	return asset, nil
}

func (s *Service) MediaAsset(ctx context.Context, id domain.MediaAssetID) (domain.MediaAsset, error) {
	b, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.MediaAsset{}, err
	}
	if b.Household == nil {
		return domain.MediaAsset{}, &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	return s.repository.MediaAsset(ctx, b.Household.ID, id)
}
func (s *Service) SetMemberAvatar(ctx context.Context, id domain.MemberID, asset domain.MediaAssetID) error {
	b, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if b.Household == nil {
		return &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	return s.repository.SetMemberAvatar(ctx, b.Household.ID, id, asset)
}
func (s *Service) SetInstitutionLogo(ctx context.Context, id domain.InstitutionID, asset domain.MediaAssetID) error {
	b, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if b.Household == nil {
		return &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	return s.repository.SetInstitutionLogo(ctx, b.Household.ID, id, asset)
}
func (s *Service) SetGroupLogo(ctx context.Context, id domain.GroupID, asset domain.MediaAssetID) error {
	b, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if b.Household == nil {
		return &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	return s.repository.SetGroupLogo(ctx, b.Household.ID, id, asset)
}
func (s *Service) SetAccountLogo(ctx context.Context, id domain.AccountID, asset domain.MediaAssetID) error {
	b, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if b.Household == nil {
		return &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	return s.repository.SetAccountLogo(ctx, b.Household.ID, id, asset)
}

func (s *Service) SetInstitutionIcon(ctx context.Context, id domain.InstitutionID, iconKey string) error {
	return s.setIcon(ctx, iconKey, func(householdID domain.HouseholdID, normalized string) error {
		return s.repository.SetInstitutionIcon(ctx, householdID, id, normalized)
	})
}

func (s *Service) SetGroupIcon(ctx context.Context, id domain.GroupID, iconKey string) error {
	return s.setIcon(ctx, iconKey, func(householdID domain.HouseholdID, normalized string) error {
		return s.repository.SetGroupIcon(ctx, householdID, id, normalized)
	})
}

func (s *Service) SetAccountIcon(ctx context.Context, id domain.AccountID, iconKey string) error {
	return s.setIcon(ctx, iconKey, func(householdID domain.HouseholdID, normalized string) error {
		return s.repository.SetAccountIcon(ctx, householdID, id, normalized)
	})
}

func (s *Service) setIcon(ctx context.Context, iconKey string, save func(domain.HouseholdID, string) error) error {
	normalized, err := normalizeIconKey(iconKey)
	if err != nil {
		return err
	}
	b, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if b.Household == nil {
		return &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	return save(b.Household.ID, normalized)
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
	if currency != bootstrap.Household.BaseCurrency {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Field: "defaultCurrency", Message: "v0.1.1 accounts must use the Household base currency"}
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
	account, ownership, initial, err := domain.NewAccount(accountInput, s.now())
	if err != nil {
		return domain.AccountRecord{}, err
	}
	value, err := domain.NewAccountValue(account, *initial, s.now(), s.now())
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if err := s.repository.CreateAccount(ctx, account, ownership, value); err != nil {
		return domain.AccountRecord{}, err
	}
	return domain.AccountRecord{Account: account, Ownership: ownership, LatestValue: &value}, nil
}

// UpdateAccount changes metadata and ownership without rewriting AccountValue history.
func (s *Service) UpdateAccount(ctx context.Context, id domain.AccountID, input AccountInput) (domain.AccountRecord, error) {
	records, err := s.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.AccountRecord{}, err
	}
	var current *domain.AccountRecord
	for index := range records {
		if records[index].Account.ID == id {
			current = &records[index]
			break
		}
	}
	if current == nil {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
	}
	if current.LatestValue == nil {
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
	if input.InitialAmount == "" {
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
	if currency != current.Account.DefaultCurrency || currency != bootstrap.Household.BaseCurrency {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrValidation, Field: "defaultCurrency", Message: "account currency is immutable and must match the Household base currency"}
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
	account, ownership, _, err := domain.NewAccount(domain.AccountInput{HouseholdID: current.Account.HouseholdID, InstitutionID: institutionID, GroupID: groupID, Name: input.Name, PrimaryCategory: primary, SecondaryCategory: secondary, TrackingMode: mode, DefaultCurrency: currency, Note: input.Note, IconKey: iconKey, IncludeInNetWorth: input.IncludeInNetWorth, IncludeInInvestment: input.IncludeInInvestment, IncludeInLiquidAssets: input.IncludeInLiquidAssets, OpenedOn: input.OpenedOn, ClosedOn: input.ClosedOn, SortOrder: current.Account.SortOrder, Ownership: ownershipShares, InitialAmount: input.InitialAmount}, s.now())
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
	if err := s.repository.UpdateAccount(ctx, account, ownership); err != nil {
		return domain.AccountRecord{}, err
	}
	current.Account, current.Ownership = account, ownership
	return *current, nil
}

func (s *Service) ListAccounts(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountRecord, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if bootstrap.Household == nil {
		return []domain.AccountRecord{}, nil
	}
	return s.repository.ListAccountRecords(ctx, bootstrap.Household.ID, filter)
}

func (s *Service) AppendAccountValue(ctx context.Context, accountID domain.AccountID, amount, effectiveAt string) (domain.AccountValue, error) {
	records, err := s.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.AccountValue{}, err
	}
	var record *domain.AccountRecord
	for index := range records {
		if records[index].Account.ID == accountID {
			record = &records[index]
			break
		}
	}
	if record == nil {
		return domain.AccountValue{}, &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
	}
	money, err := domain.ParseMoney(amount, record.Account.DefaultCurrency)
	if err != nil {
		return domain.AccountValue{}, err
	}
	when := s.now()
	if strings.TrimSpace(effectiveAt) != "" {
		parsed, parseErr := time.Parse("2006-01-02", effectiveAt)
		if parseErr != nil {
			return domain.AccountValue{}, &domain.Error{Code: domain.ErrValidation, Field: "effectiveAt", Message: "must use YYYY-MM-DD"}
		}
		when = parsed.UTC()
	}
	value, err := domain.NewAccountValue(record.Account, money, when, s.now())
	if err != nil {
		return domain.AccountValue{}, err
	}
	if err := s.repository.AppendAccountValue(ctx, value); err != nil {
		return domain.AccountValue{}, err
	}
	return value, nil
}

func (s *Service) ArchiveAccount(ctx context.Context, id domain.AccountID, archived bool) error {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if bootstrap.Household == nil {
		return &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	return s.repository.SetAccountArchive(ctx, bootstrap.Household.ID, id, archived, s.now())
}

func (s *Service) Overview(ctx context.Context, filter domain.AccountFilter) (domain.OverviewResult, error) {
	filter.IncludeArchived = false
	snapshot, err := s.repository.ReadSnapshot(ctx, filter)
	if err != nil {
		return domain.OverviewResult{}, err
	}
	if snapshot.Household == nil {
		return domain.OverviewResult{}, nil
	}
	result := domain.OverviewResult{Currency: snapshot.Household.BaseCurrency, AccountCount: len(snapshot.Accounts)}
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
	for _, record := range snapshot.Accounts {
		if !record.Account.IncludeInNetWorth || record.LatestValue == nil || record.Account.DefaultCurrency != snapshot.Household.BaseCurrency {
			continue
		}
		value := record.LatestValue.Amount.Amount()
		if record.Account.PrimaryCategory.IsLiability() {
			result.Liabilities = result.Liabilities.Add(value)
			continue
		}
		result.Assets = result.Assets.Add(value)
		categoryKey := record.Account.PrimaryCategory.String()
		category[categoryKey] = category[categoryKey].Add(value)
		for _, share := range record.Ownership.Shares() {
			member[share.MemberID.String()] = member[share.MemberID.String()].Add(value.Mul(decimal.NewFromInt(int64(share.ShareBPS))).Div(decimal.NewFromInt(domain.TotalOwnershipBPS)))
		}
		institutionKey := "unassigned"
		if record.Account.InstitutionID != nil {
			institutionKey = record.Account.InstitutionID.String()
			if record.InstitutionName != "" {
				institutionLabels[institutionKey] = record.InstitutionName
			}
		}
		institution[institutionKey] = institution[institutionKey].Add(value)
		groupKey := "unassigned"
		if record.Account.GroupID != nil {
			groupKey = record.Account.GroupID.String()
			if record.GroupName != "" {
				groupLabels[groupKey] = record.GroupName
			}
		}
		group[groupKey] = group[groupKey].Add(value)
	}
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
