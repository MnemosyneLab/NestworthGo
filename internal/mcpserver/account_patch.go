package mcpserver

import (
	"context"
	"strings"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

// AccountChanges is the update_account patch. Omitted fields keep the stored
// value. PatchString null clears institutionId, groupId, note, iconKey,
// openedOn and closedOn. An empty string is rejected on every string: it is
// not a clear and it is not a keep. Null is rejected for identity, immutable
// classification, inclusion flags, ownership and initialAmount. A note has no
// distinct empty stored value, so the only clear is null.
type AccountChanges struct {
	Name                  *string                     `json:"name,omitempty"`
	AccountType           *string                     `json:"accountType,omitempty"`
	BalanceSheetRole      *string                     `json:"balanceSheetRole,omitempty"`
	TrackingMode          *string                     `json:"trackingMode,omitempty"`
	DefaultCurrency       *string                     `json:"defaultCurrency,omitempty"`
	InstitutionID         agentcapability.PatchString `json:"institutionId,omitempty,omitzero"`
	GroupID               agentcapability.PatchString `json:"groupId,omitempty,omitzero"`
	Note                  agentcapability.PatchString `json:"note,omitempty,omitzero"`
	IconKey               agentcapability.PatchString `json:"iconKey,omitempty,omitzero"`
	IncludeInNetWorth     *bool                       `json:"includeInNetWorth,omitempty"`
	IncludeInPortfolio    *bool                       `json:"includeInPortfolio,omitempty"`
	IncludeInLiquidAssets *bool                       `json:"includeInLiquidAssets,omitempty"`
	OpenedOn              agentcapability.PatchString `json:"openedOn,omitempty,omitzero"`
	ClosedOn              agentcapability.PatchString `json:"closedOn,omitempty,omitzero"`
	Ownership             []wire.OwnershipShareDTO    `json:"ownership,omitempty"`
	OwnerIDs              []string                    `json:"ownerIds,omitempty"`
	OwnershipPercentages  []string                    `json:"ownershipPercentages,omitempty"`
	InitialAmount         *string                     `json:"initialAmount,omitempty"`
}

func (s *Service) updateAccount(ctx context.Context, in UpdateAccountInput) (any, error) {
	id, err := domain.ParseAccountID(in.ID)
	if err != nil {
		return nil, err
	}
	input, err := in.Changes.toApplication()
	if err != nil {
		return nil, err
	}
	record, err := s.app.UpdateAccount(ctx, id, input)
	if err != nil {
		return nil, err
	}
	return wire.FromAccountRecord(record), nil
}

func (in AccountChanges) toApplication() (application.AccountInput, error) {
	var out application.AccountInput
	var err error
	if out.Name, err = keptText("name", in.Name); err != nil {
		return application.AccountInput{}, err
	}
	if out.AccountType, err = keptText("accountType", in.AccountType); err != nil {
		return application.AccountInput{}, err
	}
	if out.BalanceSheetRole, err = keptText("balanceSheetRole", in.BalanceSheetRole); err != nil {
		return application.AccountInput{}, err
	}
	if out.TrackingMode, err = keptText("trackingMode", in.TrackingMode); err != nil {
		return application.AccountInput{}, err
	}
	if out.DefaultCurrency, err = keptText("defaultCurrency", in.DefaultCurrency); err != nil {
		return application.AccountInput{}, err
	}
	if err = applyClearable("institutionId", in.InstitutionID, func(value string) {
		out.InstitutionIDSet = true
		out.InstitutionID = value
	}); err != nil {
		return application.AccountInput{}, err
	}
	if err = applyClearable("groupId", in.GroupID, func(value string) {
		out.GroupIDSet = true
		out.GroupID = value
	}); err != nil {
		return application.AccountInput{}, err
	}
	if err = applyClearable("note", in.Note, func(value string) {
		out.NoteSet = true
		if value == "" {
			out.Note = nil
			return
		}
		note := value
		out.Note = &note
	}); err != nil {
		return application.AccountInput{}, err
	}
	if err = applyClearable("iconKey", in.IconKey, func(value string) {
		out.IconKeySet = true
		out.IconKey = value
	}); err != nil {
		return application.AccountInput{}, err
	}
	if err = applyClearable("openedOn", in.OpenedOn, func(value string) {
		out.OpenedOnSet = true
		if value == "" {
			out.OpenedOn = nil
			return
		}
		opened := value
		out.OpenedOn = &opened
	}); err != nil {
		return application.AccountInput{}, err
	}
	if err = applyClearable("closedOn", in.ClosedOn, func(value string) {
		out.ClosedOnSet = true
		if value == "" {
			out.ClosedOn = nil
			return
		}
		closed := value
		out.ClosedOn = &closed
	}); err != nil {
		return application.AccountInput{}, err
	}
	applyBool(in.IncludeInNetWorth, &out.IncludeInNetWorth, &out.IncludeInNetWorthSet)
	applyBool(in.IncludeInPortfolio, &out.IncludeInPortfolio, &out.IncludeInPortfolioSet)
	applyBool(in.IncludeInLiquidAssets, &out.IncludeInLiquidAssets, &out.IncludeInLiquidAssetsSet)
	if in.Ownership != nil {
		shares, shareErr := wire.ToOwnershipShares(in.Ownership)
		if shareErr != nil {
			return application.AccountInput{}, apierror.Wrap(shareErr)
		}
		out.Ownership = shares
	}
	owners, err := parseMemberIDs(in.OwnerIDs)
	if err != nil {
		return application.AccountInput{}, err
	}
	out.OwnerIDs = owners
	out.OwnershipPercentages = in.OwnershipPercentages
	if in.InitialAmount != nil {
		if strings.TrimSpace(*in.InitialAmount) == "" {
			return application.AccountInput{}, fail("validation", "initialAmount cannot be empty; omit it to keep the stored value. It does not rewrite balance history")
		}
		out.InitialAmount = *in.InitialAmount
	}
	return out, nil
}

func keptText(field string, value *string) (string, error) {
	if value == nil {
		return "", nil
	}
	if strings.TrimSpace(*value) == "" {
		return "", fail("validation", field+" cannot be empty; omit it to keep the stored value")
	}
	return *value, nil
}

func applyClearable(field string, patch agentcapability.PatchString, assign func(value string)) error {
	if !patch.Present() {
		return nil
	}
	if patch.Null() {
		assign("")
		return nil
	}
	value, _ := patch.Value()
	if strings.TrimSpace(value) == "" {
		return fail("validation", field+" empty string is invalid; omit the field to keep it or send null to clear it")
	}
	assign(value)
	return nil
}

func applyBool(value *bool, dest *bool, set *bool) {
	if value == nil {
		return
	}
	*dest = *value
	*set = true
}

func parseMemberIDs(values []string) ([]domain.MemberID, error) {
	if len(values) == 0 {
		return nil, nil
	}
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
