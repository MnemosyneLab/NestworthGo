package desktop

import (
	"context"
	"errors"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// The directory list owns UI keys and state. Each adapter converts them to
// the typed IDs and commands of the corresponding application service.
type directorySource struct {
	list    func(context.Context, bool) ([]directoryRow, error)
	create  func(context.Context, directoryChange) (string, error)
	edit    func(context.Context, directoryChange) error
	archive func(context.Context, string, bool) error
}

type directoryRow struct {
	ID, Name        string
	icon            *string
	institutionType domain.InstitutionType
	archived        bool
}

func (r directoryRow) iconKey() string {
	if r.icon == nil {
		return ""
	}
	return *r.icon
}

func directoryRows[T any](ctx context.Context, archive bool, list func(context.Context, bool) ([]T, error), row func(T) directoryRow) ([]directoryRow, error) {
	entities, err := list(ctx, archive)
	if err != nil {
		return nil, err
	}
	rows := make([]directoryRow, 0, len(entities))
	for _, entity := range entities {
		rows = append(rows, row(entity))
	}
	return rows, nil
}

type institutionService interface {
	ListInstitutions(context.Context, bool) ([]domain.Institution, error)
	CreateInstitution(context.Context, string, domain.InstitutionType, ...string) (domain.Institution, error)
	UpdateInstitution(context.Context, domain.InstitutionID, string) (domain.Institution, error)
	ArchiveInstitution(context.Context, domain.InstitutionID, bool) error
}

func newInstitutionsPage(ctx context.Context, service institutionService, invalidate func()) *directoryList {
	source := directorySource{
		list: func(ctx context.Context, archived bool) ([]directoryRow, error) {
			return directoryRows(ctx, archived, service.ListInstitutions, func(row domain.Institution) directoryRow {
				return directoryRow{ID: row.ID.String(), Name: row.Name, institutionType: row.InstitutionType, archived: row.ArchivedAt != nil}
			})
		},
		create: func(ctx context.Context, change directoryChange) (string, error) {
			row, err := service.CreateInstitution(ctx, change.name, change.kind)
			return row.ID.String(), err
		},
		edit: func(ctx context.Context, change directoryChange) error {
			_, err := service.UpdateInstitution(ctx, domain.InstitutionID(change.id), change.name)
			return err
		},
		archive: func(ctx context.Context, id string, archived bool) error {
			return service.ArchiveInstitution(ctx, domain.InstitutionID(id), archived)
		},
	}
	copy := directoryCopy{
		title: i18n.NavInstitutions, name: i18n.PreviewInstitutionName,
		add: i18n.InstitutionsAdd, edit: i18n.InstitutionsEditTitle,
		search: i18n.PreviewSearch, searchHint: i18n.InstitutionsSearchHint,
		loading: i18n.InstitutionsLoading, loadError: i18n.InstitutionsLoadError,
		empty: i18n.InstitutionsEmpty, emptyAll: i18n.InstitutionsEmptyAll, noMatches: i18n.PreviewNoMatches,
		created: i18n.InstitutionsCreated, saved: i18n.InstitutionsSaved,
		archived: i18n.InstitutionsArchived, restored: i18n.InstitutionsRestored,
		archiveTitle: i18n.InstitutionsArchiveTitle, archiveError: i18n.InstitutionsArchiveError,
		restoreError: i18n.InstitutionsRestoreError, saveError: i18n.InstitutionsSaveError,
	}
	return newDirectoryList(ctx, source, copy, "", invalidate)
}

type memberService interface {
	ListMembers(context.Context, bool) ([]domain.Member, error)
	CreateMember(context.Context, string, ...string) (domain.Member, error)
	EditMember(context.Context, domain.MemberID, application.DirectoryEdit) (domain.Member, error)
	ArchiveMember(context.Context, domain.MemberID, bool) error
}

func newMembersList(ctx context.Context, service memberService, invalidate func()) *directoryList {
	source := directorySource{
		list: func(ctx context.Context, archived bool) ([]directoryRow, error) {
			return directoryRows(ctx, archived, service.ListMembers, func(row domain.Member) directoryRow {
				return directoryRow{ID: row.ID.String(), Name: row.Name, icon: row.IconKey, archived: row.ArchivedAt != nil}
			})
		},
		create: func(ctx context.Context, change directoryChange) (string, error) {
			row, err := service.CreateMember(ctx, change.name, *change.icon)
			return row.ID.String(), err
		},
		edit: func(ctx context.Context, change directoryChange) error {
			_, err := service.EditMember(ctx, domain.MemberID(change.id), application.DirectoryEdit{Name: &change.name, IconKey: change.icon})
			return err
		},
		archive: func(ctx context.Context, id string, archived bool) error {
			return service.ArchiveMember(ctx, domain.MemberID(id), archived)
		},
	}
	copy := memberGroupCopy()
	copy.title, copy.name, copy.add, copy.edit = i18n.NavMembers, i18n.DirectoryMemberName, i18n.DirectoryAddMember, i18n.DirectoryEditMember
	copy.search, copy.archiveTitle = i18n.DirectorySearchMembers, i18n.DirectoryArchiveMember
	return newDirectoryList(ctx, source, copy, domain.DefaultMemberIcon, invalidate)
}

type groupService interface {
	ListGroups(context.Context, bool) ([]domain.Group, error)
	CreateGroup(context.Context, string, ...string) (domain.Group, error)
	EditGroup(context.Context, domain.GroupID, application.DirectoryEdit) (domain.Group, error)
	ArchiveGroup(context.Context, domain.GroupID, bool) error
}

func newGroupsList(ctx context.Context, service groupService, invalidate func()) *directoryList {
	source := directorySource{
		list: func(ctx context.Context, archived bool) ([]directoryRow, error) {
			return directoryRows(ctx, archived, service.ListGroups, func(row domain.Group) directoryRow {
				return directoryRow{ID: row.ID.String(), Name: row.Name, icon: row.IconKey, archived: row.ArchivedAt != nil}
			})
		},
		create: func(ctx context.Context, change directoryChange) (string, error) {
			row, err := service.CreateGroup(ctx, change.name, *change.icon)
			return row.ID.String(), err
		},
		edit: func(ctx context.Context, change directoryChange) error {
			_, err := service.EditGroup(ctx, domain.GroupID(change.id), application.DirectoryEdit{Name: &change.name, IconKey: change.icon})
			return err
		},
		archive: func(ctx context.Context, id string, archived bool) error {
			return service.ArchiveGroup(ctx, domain.GroupID(id), archived)
		},
	}
	copy := memberGroupCopy()
	copy.title, copy.name, copy.add, copy.edit = i18n.NavDirectoryGroups, i18n.DirectoryGroupName, i18n.DirectoryAddGroup, i18n.DirectoryEditGroup
	copy.search, copy.archiveTitle = i18n.DirectorySearchGroups, i18n.DirectoryArchiveGroup
	return newDirectoryList(ctx, source, copy, domain.DefaultGroupIcon, invalidate)
}

type directoryCopy struct {
	title, name, add, edit, search, searchHint          i18n.MessageID
	loading, loadError, empty, emptyAll, noMatches      i18n.MessageID
	created, saved, archived, restored                  i18n.MessageID
	archiveTitle, archiveError, restoreError, saveError i18n.MessageID
}

func memberGroupCopy() directoryCopy {
	return directoryCopy{
		searchHint: i18n.DirectorySearchHint,
		loading:    i18n.DirectoryLoading, loadError: i18n.DirectoryLoadError,
		empty: i18n.DirectoryEmpty, emptyAll: i18n.DirectoryEmptyAll, noMatches: i18n.DirectoryNoMatches,
		created: i18n.DirectoryCreated, saved: i18n.DirectorySaved,
		archived: i18n.DirectoryArchiveDone, restored: i18n.DirectoryRestored,
		archiveError: i18n.DirectoryArchiveError, restoreError: i18n.DirectoryRestoreError, saveError: i18n.DirectorySaveError,
	}
}

func (p *directoryList) archiveProblem(err error) i18n.MessageID {
	var conflict *domain.Error
	if p.copy.title == i18n.NavMembers && errors.As(err, &conflict) && conflict.Code == domain.ErrConflict {
		// The application owns both guards. Their distinct domain error
		// fields distinguish the actionable owner conflict from the last member.
		if conflict.Field == "memberId" {
			return i18n.DirectoryMemberOwnsAccount
		}
		if conflict.Message == "household must retain at least one active member" {
			return i18n.DirectoryLastActiveMember
		}
	}
	return p.copy.archiveError
}
