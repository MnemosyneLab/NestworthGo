// Package directory adapts internal/application.Service's Member,
// Institution, and Group CRUD/archive/icon surface for the Wails IPC
// boundary. These three entities share the same CRUD/archive/icon shape in
// the product UI and the release contracts, so one service covers all three.
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

func (s *Service) CreateMember(ctx context.Context, name, iconKey string) (wire.MemberDTO, error) {
	var member domain.Member
	var err error
	if iconKey == "" {
		member, err = s.app.CreateMember(ctx, name)
	} else {
		member, err = s.app.CreateMember(ctx, name, iconKey)
	}
	if err != nil {
		return wire.MemberDTO{}, apierror.Wrap(err)
	}
	return wire.FromMember(member), nil
}

func (s *Service) UpdateMember(ctx context.Context, id, name string) (wire.MemberDTO, error) {
	return updateEntity(id, domain.ParseMemberID, func(memberID domain.MemberID) (domain.Member, error) {
		return s.app.UpdateMember(ctx, memberID, name)
	}, wire.FromMember)
}

func (s *Service) ArchiveMember(ctx context.Context, id string, archived bool) error {
	return withParsedID(id, domain.ParseMemberID, func(memberID domain.MemberID) error {
		return s.app.ArchiveMember(ctx, memberID, archived)
	})
}

func (s *Service) SetMemberIcon(ctx context.Context, id, iconKey string) error {
	return withParsedID(id, domain.ParseMemberID, func(memberID domain.MemberID) error {
		return s.app.SetMemberIcon(ctx, memberID, iconKey)
	})
}

// --- Institutions ---

func (s *Service) ListInstitutions(ctx context.Context, includeArchived bool) ([]wire.InstitutionDTO, error) {
	institutions, err := s.app.ListInstitutions(ctx, includeArchived)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromInstitutions(institutions), nil
}

func (s *Service) CreateInstitution(ctx context.Context, name, institutionType, iconKey string) (wire.InstitutionDTO, error) {
	parsedType, parseErr := domain.ParseInstitutionType(institutionType)
	if parseErr != nil {
		return wire.InstitutionDTO{}, apierror.Wrap(parseErr)
	}
	var institution domain.Institution
	var err error
	if iconKey == "" {
		institution, err = s.app.CreateInstitution(ctx, name, parsedType)
	} else {
		institution, err = s.app.CreateInstitution(ctx, name, parsedType, iconKey)
	}
	if err != nil {
		return wire.InstitutionDTO{}, apierror.Wrap(err)
	}
	return wire.FromInstitution(institution), nil
}

func (s *Service) UpdateInstitution(ctx context.Context, id, name string) (wire.InstitutionDTO, error) {
	return updateEntity(id, domain.ParseInstitutionID, func(institutionID domain.InstitutionID) (domain.Institution, error) {
		return s.app.UpdateInstitution(ctx, institutionID, name)
	}, wire.FromInstitution)
}

func (s *Service) ArchiveInstitution(ctx context.Context, id string, archived bool) error {
	return withParsedID(id, domain.ParseInstitutionID, func(institutionID domain.InstitutionID) error {
		return s.app.ArchiveInstitution(ctx, institutionID, archived)
	})
}

func (s *Service) SetInstitutionIcon(ctx context.Context, id, iconKey string) error {
	return withParsedID(id, domain.ParseInstitutionID, func(institutionID domain.InstitutionID) error {
		return s.app.SetInstitutionIcon(ctx, institutionID, iconKey)
	})
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
	return updateEntity(id, domain.ParseGroupID, func(groupID domain.GroupID) (domain.Group, error) {
		return s.app.UpdateGroup(ctx, groupID, name)
	}, wire.FromGroup)
}

func (s *Service) ArchiveGroup(ctx context.Context, id string, archived bool) error {
	return withParsedID(id, domain.ParseGroupID, func(groupID domain.GroupID) error {
		return s.app.ArchiveGroup(ctx, groupID, archived)
	})
}

func (s *Service) SetGroupIcon(ctx context.Context, id, iconKey string) error {
	return withParsedID(id, domain.ParseGroupID, func(groupID domain.GroupID) error {
		return s.app.SetGroupIcon(ctx, groupID, iconKey)
	})
}

func updateEntity[ID ~string, Domain any, DTO any](
	id string,
	parse func(string) (ID, error),
	update func(ID) (Domain, error),
	from func(Domain) DTO,
) (DTO, error) {
	var zero DTO
	parsed, err := parse(id)
	if err != nil {
		return zero, apierror.Wrap(err)
	}
	value, err := update(parsed)
	if err != nil {
		return zero, apierror.Wrap(err)
	}
	return from(value), nil
}

func withParsedID[ID ~string](id string, parse func(string) (ID, error), fn func(ID) error) error {
	parsed, err := parse(id)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(fn(parsed))
}
