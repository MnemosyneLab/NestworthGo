package mcpserver

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

// editDirectory applies a partial name and icon edit to a member, institution
// or group through one application statement, so the two fields never apply
// separately. It uses the application service directly: the Wails-bound
// directory service keeps its two independent GUI writes unchanged.
func editDirectory[ID ~string, Domain any, DTO any](id string, parse func(string) (ID, error), edit func(ID) (Domain, error), from func(Domain) DTO) (any, error) {
	parsed, err := parse(id)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	value, err := edit(parsed)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return from(value), nil
}

func (s *Service) editMember(ctx context.Context, in UpdateDirectoryInput) (any, error) {
	return editDirectory(in.ID, domain.ParseMemberID, func(id domain.MemberID) (domain.Member, error) {
		return s.app.EditMember(ctx, id, application.DirectoryEdit{Name: in.Name, IconKey: in.IconKey})
	}, wire.FromMember)
}

func (s *Service) editInstitution(ctx context.Context, in UpdateDirectoryInput) (any, error) {
	return editDirectory(in.ID, domain.ParseInstitutionID, func(id domain.InstitutionID) (domain.Institution, error) {
		return s.app.EditInstitution(ctx, id, application.DirectoryEdit{Name: in.Name, IconKey: in.IconKey})
	}, wire.FromInstitution)
}

func (s *Service) editGroup(ctx context.Context, in UpdateDirectoryInput) (any, error) {
	return editDirectory(in.ID, domain.ParseGroupID, func(id domain.GroupID) (domain.Group, error) {
		return s.app.EditGroup(ctx, id, application.DirectoryEdit{Name: in.Name, IconKey: in.IconKey})
	}, wire.FromGroup)
}
