// Package account adapts internal/application.Service's Account
// create/update/list/archive/value surface for the Wails IPC boundary.
package account

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type Service struct {
	app *application.Service
}

func NewService(app *application.Service) *Service {
	return &Service{app: app}
}

// CreateAccountRequest mirrors application.AccountInput's create shape. All
// fields except Name/PrimaryCategory/SecondaryCategory/TrackingMode/
// DefaultCurrency are optional.
type CreateAccountRequest struct {
	Name                  string                   `json:"name"`
	PrimaryCategory       string                   `json:"primaryCategory"`
	SecondaryCategory     string                   `json:"secondaryCategory"`
	TrackingMode          string                   `json:"trackingMode"`
	DefaultCurrency       string                   `json:"defaultCurrency"`
	InstitutionID         *string                  `json:"institutionId,omitempty"`
	GroupID               *string                  `json:"groupId,omitempty"`
	Note                  *string                  `json:"note,omitempty"`
	IconKey               *string                  `json:"iconKey,omitempty"`
	IncludeInNetWorth     bool                     `json:"includeInNetWorth"`
	IncludeInInvestment   bool                     `json:"includeInInvestment"`
	IncludeInLiquidAssets bool                     `json:"includeInLiquidAssets"`
	OpenedOn              *string                  `json:"openedOn,omitempty"`
	ClosedOn              *string                  `json:"closedOn,omitempty"`
	Ownership             []wire.OwnershipShareDTO `json:"ownership,omitempty"`
	OwnerIDs              []string                 `json:"ownerIds,omitempty"`
	OwnershipPercentages  []string                 `json:"ownershipPercentages,omitempty"`
	InitialAmount         string                   `json:"initialAmount,omitempty"`
}

func (r CreateAccountRequest) toApplicationInput() (application.AccountInput, error) {
	shares, err := wire.ToOwnershipShares(r.Ownership)
	if err != nil {
		return application.AccountInput{}, err
	}
	ownerIDs, err := parseMemberIDs(r.OwnerIDs)
	if err != nil {
		return application.AccountInput{}, err
	}
	return application.AccountInput{
		Name: r.Name, PrimaryCategory: r.PrimaryCategory, SecondaryCategory: r.SecondaryCategory,
		TrackingMode: r.TrackingMode, DefaultCurrency: r.DefaultCurrency,
		InstitutionID: wire.StringFromPtr(r.InstitutionID), InstitutionIDSet: r.InstitutionID != nil,
		GroupID: wire.StringFromPtr(r.GroupID), GroupIDSet: r.GroupID != nil,
		Note: r.Note, NoteSet: r.Note != nil,
		IconKey: wire.StringFromPtr(r.IconKey), IconKeySet: r.IconKey != nil,
		IncludeInNetWorth: r.IncludeInNetWorth, IncludeInNetWorthSet: true,
		IncludeInInvestment: r.IncludeInInvestment, IncludeInInvestmentSet: true,
		IncludeInLiquidAssets: r.IncludeInLiquidAssets, IncludeInLiquidAssetsSet: true,
		OpenedOn: r.OpenedOn, OpenedOnSet: r.OpenedOn != nil,
		ClosedOn: r.ClosedOn, ClosedOnSet: r.ClosedOn != nil,
		Ownership: shares, OwnerIDs: ownerIDs, OwnershipPercentages: r.OwnershipPercentages,
		InitialAmount: r.InitialAmount,
	}, nil
}

// UpdateAccountRequest follows the "Set flags" pattern required by
// application.AccountInput: a nil pointer means
// "leave unchanged," a non-nil pointer to an empty string means "clear."
// Boolean fields use *bool for the same reason.
type UpdateAccountRequest struct {
	Name                  *string                  `json:"name,omitempty"`
	PrimaryCategory       *string                  `json:"primaryCategory,omitempty"`
	SecondaryCategory     *string                  `json:"secondaryCategory,omitempty"`
	TrackingMode          *string                  `json:"trackingMode,omitempty"`
	DefaultCurrency       *string                  `json:"defaultCurrency,omitempty"`
	InstitutionID         *string                  `json:"institutionId,omitempty"`
	InstitutionIDSet      bool                     `json:"institutionIdSet,omitempty"`
	GroupID               *string                  `json:"groupId,omitempty"`
	GroupIDSet            bool                     `json:"groupIdSet,omitempty"`
	Note                  *string                  `json:"note,omitempty"`
	NoteSet               bool                     `json:"noteSet,omitempty"`
	IconKey               *string                  `json:"iconKey,omitempty"`
	IconKeySet            bool                     `json:"iconKeySet,omitempty"`
	IncludeInNetWorth     *bool                    `json:"includeInNetWorth,omitempty"`
	IncludeInInvestment   *bool                    `json:"includeInInvestment,omitempty"`
	IncludeInLiquidAssets *bool                    `json:"includeInLiquidAssets,omitempty"`
	OpenedOn              *string                  `json:"openedOn,omitempty"`
	OpenedOnSet           bool                     `json:"openedOnSet,omitempty"`
	ClosedOn              *string                  `json:"closedOn,omitempty"`
	ClosedOnSet           bool                     `json:"closedOnSet,omitempty"`
	Ownership             []wire.OwnershipShareDTO `json:"ownership,omitempty"`
	OwnerIDs              []string                 `json:"ownerIds,omitempty"`
	OwnershipPercentages  []string                 `json:"ownershipPercentages,omitempty"`
	InitialAmount         *string                  `json:"initialAmount,omitempty"`
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func (r UpdateAccountRequest) toApplicationInput() (application.AccountInput, error) {
	shares, err := wire.ToOwnershipShares(r.Ownership)
	if err != nil {
		return application.AccountInput{}, err
	}
	ownerIDs, err := parseMemberIDs(r.OwnerIDs)
	if err != nil {
		return application.AccountInput{}, err
	}
	input := application.AccountInput{
		Name: wire.StringFromPtr(r.Name), PrimaryCategory: wire.StringFromPtr(r.PrimaryCategory),
		SecondaryCategory: wire.StringFromPtr(r.SecondaryCategory), TrackingMode: wire.StringFromPtr(r.TrackingMode),
		DefaultCurrency: wire.StringFromPtr(r.DefaultCurrency),
		InstitutionID:   wire.StringFromPtr(r.InstitutionID), InstitutionIDSet: r.InstitutionIDSet,
		GroupID: wire.StringFromPtr(r.GroupID), GroupIDSet: r.GroupIDSet,
		Note: r.Note, NoteSet: r.NoteSet,
		IconKey: wire.StringFromPtr(r.IconKey), IconKeySet: r.IconKeySet,
		IncludeInNetWorth: boolValue(r.IncludeInNetWorth), IncludeInNetWorthSet: r.IncludeInNetWorth != nil,
		IncludeInInvestment: boolValue(r.IncludeInInvestment), IncludeInInvestmentSet: r.IncludeInInvestment != nil,
		IncludeInLiquidAssets: boolValue(r.IncludeInLiquidAssets), IncludeInLiquidAssetsSet: r.IncludeInLiquidAssets != nil,
		OpenedOn: r.OpenedOn, OpenedOnSet: r.OpenedOnSet,
		ClosedOn: r.ClosedOn, ClosedOnSet: r.ClosedOnSet,
		Ownership: shares, OwnerIDs: ownerIDs, OwnershipPercentages: r.OwnershipPercentages,
		InitialAmount: wire.StringFromPtr(r.InitialAmount),
	}
	return input, nil
}

func parseMemberIDs(values []string) ([]domain.MemberID, error) {
	result := make([]domain.MemberID, 0, len(values))
	for _, value := range values {
		id, err := domain.ParseMemberID(value)
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

// AccountFilterRequest mirrors domain.AccountFilter.
type AccountFilterRequest struct {
	IncludeArchived bool    `json:"includeArchived,omitempty"`
	MemberID        *string `json:"memberId,omitempty"`
	InstitutionID   *string `json:"institutionId,omitempty"`
	GroupID         *string `json:"groupId,omitempty"`
	Category        *string `json:"category,omitempty"`
	OwnershipScope  string  `json:"ownershipScope,omitempty"`
}

// ToDomain converts the request DTO to domain.AccountFilter. It is exported
// so other services (e.g. portfolio) that accept the same filter shape can
// reuse this parsing instead of duplicating it.
func (r AccountFilterRequest) ToDomain() (domain.AccountFilter, error) {
	filter := domain.AccountFilter{IncludeArchived: r.IncludeArchived}
	if r.MemberID != nil {
		id, err := domain.ParseMemberID(*r.MemberID)
		if err != nil {
			return domain.AccountFilter{}, err
		}
		filter.MemberID = &id
	}
	if r.InstitutionID != nil {
		id, err := domain.ParseInstitutionID(*r.InstitutionID)
		if err != nil {
			return domain.AccountFilter{}, err
		}
		filter.InstitutionID = &id
	}
	if r.GroupID != nil {
		id, err := domain.ParseGroupID(*r.GroupID)
		if err != nil {
			return domain.AccountFilter{}, err
		}
		filter.GroupID = &id
	}
	if r.Category != nil {
		category, err := domain.ParsePrimaryCategory(*r.Category)
		if err != nil {
			return domain.AccountFilter{}, err
		}
		filter.Category = &category
	}
	if r.OwnershipScope != "" {
		scope, err := domain.ParseOwnershipScope(r.OwnershipScope)
		if err != nil {
			return domain.AccountFilter{}, err
		}
		filter.OwnershipScope = scope
	}
	return filter, nil
}

func (s *Service) CreateAccount(ctx context.Context, request CreateAccountRequest) (wire.AccountRecordDTO, error) {
	input, err := request.toApplicationInput()
	if err != nil {
		return wire.AccountRecordDTO{}, apierror.Wrap(err)
	}
	record, err := s.app.CreateAccount(ctx, input)
	if err != nil {
		return wire.AccountRecordDTO{}, apierror.Wrap(err)
	}
	return wire.FromAccountRecord(record), nil
}

func (s *Service) UpdateAccount(ctx context.Context, id string, request UpdateAccountRequest) (wire.AccountRecordDTO, error) {
	accountID, err := domain.ParseAccountID(id)
	if err != nil {
		return wire.AccountRecordDTO{}, apierror.Wrap(err)
	}
	input, err := request.toApplicationInput()
	if err != nil {
		return wire.AccountRecordDTO{}, apierror.Wrap(err)
	}
	record, err := s.app.UpdateAccount(ctx, accountID, input)
	if err != nil {
		return wire.AccountRecordDTO{}, apierror.Wrap(err)
	}
	return wire.FromAccountRecord(record), nil
}

func (s *Service) ListAccounts(ctx context.Context, request AccountFilterRequest) ([]wire.AccountRecordDTO, error) {
	filter, err := request.ToDomain()
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	records, err := s.app.ListAccounts(ctx, filter)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromAccountRecords(records), nil
}

func (s *Service) ArchiveAccount(ctx context.Context, id string, archived bool) error {
	accountID, err := domain.ParseAccountID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.ArchiveAccount(ctx, accountID, archived))
}

func (s *Service) AppendAccountValue(ctx context.Context, id, amount, effectiveAt string) (wire.AccountValueDTO, error) {
	accountID, err := domain.ParseAccountID(id)
	if err != nil {
		return wire.AccountValueDTO{}, apierror.Wrap(err)
	}
	value, err := s.app.AppendAccountValue(ctx, accountID, amount, effectiveAt)
	if err != nil {
		return wire.AccountValueDTO{}, apierror.Wrap(err)
	}
	return wire.FromAccountValue(value), nil
}

func (s *Service) AccountValuation(ctx context.Context, id string) (wire.AccountValuationDTO, error) {
	accountID, err := domain.ParseAccountID(id)
	if err != nil {
		return wire.AccountValuationDTO{}, apierror.Wrap(err)
	}
	valuation, err := s.app.AccountValuation(ctx, accountID)
	if err != nil {
		return wire.AccountValuationDTO{}, apierror.Wrap(err)
	}
	return wire.FromAccountValuation(valuation), nil
}

func (s *Service) AccountValuations(ctx context.Context, request AccountFilterRequest) ([]wire.AccountValuationDTO, error) {
	filter, err := request.ToDomain()
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	valuations, err := s.app.AccountValuations(ctx, filter)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromAccountValuations(valuations), nil
}

func (s *Service) SetAccountIcon(ctx context.Context, id, iconKey string) error {
	accountID, err := domain.ParseAccountID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.SetAccountIcon(ctx, accountID, iconKey))
}

func (s *Service) SetAccountLogo(ctx context.Context, id, mediaAssetID string) error {
	accountID, err := domain.ParseAccountID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	assetID, err := domain.ParseMediaAssetID(mediaAssetID)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.SetAccountLogo(ctx, accountID, assetID))
}
