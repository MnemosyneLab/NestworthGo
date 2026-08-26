// Package directory adapts internal/application.Service's Member,
// Institution, and Group CRUD/archive/icon/logo surface for the Wails IPC
// boundary. These three entities share the same shape in both the Fyne UI
// and the release contracts, so one service covers all three (technical
// design Sec6).
package directory

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

// --- Members ---

func (s *Service) ListMembers(ctx context.Context, includeArchived bool) ([]wire.MemberDTO, error) {
	members, err := s.app.ListMembers(ctx, includeArchived)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromMembers(members), nil
}

func (s *Service) CreateMember(ctx context.Context, name string) (wire.MemberDTO, error) {
	member, err := s.app.CreateMember(ctx, name)
	if err != nil {
		return wire.MemberDTO{}, apierror.Wrap(err)
	}
	return wire.FromMember(member), nil
}

func (s *Service) UpdateMember(ctx context.Context, id, name string) (wire.MemberDTO, error) {
	memberID, err := domain.ParseMemberID(id)
	if err != nil {
		return wire.MemberDTO{}, apierror.Wrap(err)
	}
	member, err := s.app.UpdateMember(ctx, memberID, name)
	if err != nil {
		return wire.MemberDTO{}, apierror.Wrap(err)
	}
	return wire.FromMember(member), nil
}

func (s *Service) ArchiveMember(ctx context.Context, id string, archived bool) error {
	memberID, err := domain.ParseMemberID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.ArchiveMember(ctx, memberID, archived))
}

func (s *Service) SetMemberAvatar(ctx context.Context, id, mediaAssetID string) error {
	memberID, err := domain.ParseMemberID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	assetID, err := domain.ParseMediaAssetID(mediaAssetID)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.SetMemberAvatar(ctx, memberID, assetID))
}

// --- Institutions ---

func (s *Service) ListInstitutions(ctx context.Context, includeArchived bool) ([]wire.InstitutionDTO, error) {
	institutions, err := s.app.ListInstitutions(ctx, includeArchived)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromInstitutions(institutions), nil
}

func (s *Service) CreateInstitution(ctx context.Context, name, iconKey string) (wire.InstitutionDTO, error) {
	var institution domain.Institution
	var err error
	if iconKey == "" {
		institution, err = s.app.CreateInstitution(ctx, name)
	} else {
		institution, err = s.app.CreateInstitution(ctx, name, iconKey)
	}
	if err != nil {
		return wire.InstitutionDTO{}, apierror.Wrap(err)
	}
	return wire.FromInstitution(institution), nil
}

func (s *Service) UpdateInstitution(ctx context.Context, id, name string) (wire.InstitutionDTO, error) {
	institutionID, err := domain.ParseInstitutionID(id)
	if err != nil {
		return wire.InstitutionDTO{}, apierror.Wrap(err)
	}
	institution, err := s.app.UpdateInstitution(ctx, institutionID, name)
	if err != nil {
		return wire.InstitutionDTO{}, apierror.Wrap(err)
	}
	return wire.FromInstitution(institution), nil
}

func (s *Service) ArchiveInstitution(ctx context.Context, id string, archived bool) error {
	institutionID, err := domain.ParseInstitutionID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.ArchiveInstitution(ctx, institutionID, archived))
}

func (s *Service) SetInstitutionIcon(ctx context.Context, id, iconKey string) error {
	institutionID, err := domain.ParseInstitutionID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.SetInstitutionIcon(ctx, institutionID, iconKey))
}

func (s *Service) SetInstitutionLogo(ctx context.Context, id, mediaAssetID string) error {
	institutionID, err := domain.ParseInstitutionID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	assetID, err := domain.ParseMediaAssetID(mediaAssetID)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.SetInstitutionLogo(ctx, institutionID, assetID))
}

// --- Groups ---

func (s *Service) ListGroups(ctx context.Context, includeArchived bool) ([]wire.GroupDTO, error) {
	groups, err := s.app.ListGroups(ctx, includeArchived)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromGroups(groups), nil
}

func (s *Service) CreateGroup(ctx context.Context, name, iconKey string) (wire.GroupDTO, error) {
	var group domain.Group
	var err error
	if iconKey == "" {
		group, err = s.app.CreateGroup(ctx, name)
	} else {
		group, err = s.app.CreateGroup(ctx, name, iconKey)
	}
	if err != nil {
		return wire.GroupDTO{}, apierror.Wrap(err)
	}
	return wire.FromGroup(group), nil
}

func (s *Service) UpdateGroup(ctx context.Context, id, name string) (wire.GroupDTO, error) {
	groupID, err := domain.ParseGroupID(id)
	if err != nil {
		return wire.GroupDTO{}, apierror.Wrap(err)
	}
	group, err := s.app.UpdateGroup(ctx, groupID, name)
	if err != nil {
		return wire.GroupDTO{}, apierror.Wrap(err)
	}
	return wire.FromGroup(group), nil
}

func (s *Service) ArchiveGroup(ctx context.Context, id string, archived bool) error {
	groupID, err := domain.ParseGroupID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.ArchiveGroup(ctx, groupID, archived))
}

func (s *Service) SetGroupIcon(ctx context.Context, id, iconKey string) error {
	groupID, err := domain.ParseGroupID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.SetGroupIcon(ctx, groupID, iconKey))
}

func (s *Service) SetGroupLogo(ctx context.Context, id, mediaAssetID string) error {
	groupID, err := domain.ParseGroupID(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	assetID, err := domain.ParseMediaAssetID(mediaAssetID)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.SetGroupLogo(ctx, groupID, assetID))
}
