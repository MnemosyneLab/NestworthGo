package mcpserver

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

// editDirectory applies a partial name, icon and archive edit to a member,
// institution or group. Name and icon still share one application statement.
// Archive state is part of the same call and the same MCP receipt. It uses the
// application service directly: the Wails-bound directory service keeps its
// independent GUI writes unchanged.
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

func directoryEdit(in UpdateDirectoryInput) application.DirectoryEdit {
	return application.DirectoryEdit{Name: in.Name, IconKey: in.IconKey, Archived: in.Archived}
}

func (s *Service) editMember(ctx context.Context, in UpdateDirectoryInput) (any, error) {
	return editDirectory(in.ID, domain.ParseMemberID, func(id domain.MemberID) (domain.Member, error) {
		return s.app.EditMember(ctx, id, directoryEdit(in))
	}, wire.FromMember)
}

func (s *Service) editInstitution(ctx context.Context, in UpdateDirectoryInput) (any, error) {
	return editDirectory(in.ID, domain.ParseInstitutionID, func(id domain.InstitutionID) (domain.Institution, error) {
		return s.app.EditInstitution(ctx, id, directoryEdit(in))
	}, wire.FromInstitution)
}

func (s *Service) editGroup(ctx context.Context, in UpdateDirectoryInput) (any, error) {
	return editDirectory(in.ID, domain.ParseGroupID, func(id domain.GroupID) (domain.Group, error) {
		return s.app.EditGroup(ctx, id, directoryEdit(in))
	}, wire.FromGroup)
}
