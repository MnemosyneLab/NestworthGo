package desktop

import (
	"context"
	"errors"
	"image"
	"slices"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type directoryFaultService struct {
	*application.Service
	memberEdits  int
	groupArchive int
	memberReload bool
	groupReload  bool
}

func (s *directoryFaultService) EditMember(ctx context.Context, id domain.MemberID, edit application.DirectoryEdit) (domain.Member, error) {
	s.memberEdits++
	row, err := s.Service.EditMember(ctx, id, edit)
	s.memberReload = err == nil
	return row, err
}

func (s *directoryFaultService) ListMembers(ctx context.Context, includeArchived bool) ([]domain.Member, error) {
	if s.memberReload {
		s.memberReload = false
		return nil, errors.New("injected member reload failure")
	}
	return s.Service.ListMembers(ctx, includeArchived)
}

func (s *directoryFaultService) ArchiveGroup(ctx context.Context, id domain.GroupID, archived bool) error {
	s.groupArchive++
	err := s.Service.ArchiveGroup(ctx, id, archived)
	s.groupReload = err == nil && archived
	return err
}

func (s *directoryFaultService) ListGroups(ctx context.Context, includeArchived bool) ([]domain.Group, error) {
	if s.groupReload {
		s.groupReload = false
		return nil, errors.New("injected group reload failure")
	}
	return s.Service.ListGroups(ctx, includeArchived)
}

func directoryHarness(t *testing.T, ctx context.Context, svc *directoryFaultService) (*previewHarness, *directoryPage, func()) {
	t.Helper()
	directory := newDirectoryPage(
		newMembersList(ctx, svc, func() {}),
		newInstitutionsPage(ctx, svc, func() {}),
		newGroupsList(ctx, svc, func() {}),
	)
	t.Cleanup(directory.wait)
	h := newShellHarnessWithPages(t, ledgerPages{directory: directory})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Directory", false)
	finish := func() {
		directory.wait()
		h.frame()
	}
	h.size = image.Pt(700, 460)
	finish()
	return h, directory, finish
}

func TestMemberDirectorySavesAnAtomicEditAndHonorsArchiveGuards(t *testing.T) {
	ctx, open := newDirectoryFixture(t)
	service, close := open()
	svc := &directoryFaultService{Service: service}
	h, page, finish := directoryHarness(t, ctx, svc)
	members := page.activeList()
	assertPageLabel(t, &h.router, "Sample member")
	h.click("Archive", false)
	assertPageLabel(t, &h.router, "Archive member?")
	h.press(key.NameTab)
	h.press(key.NameReturn)
	finish()
	assertPageLabel(t, &h.router, i18n.English.Text(i18n.DirectoryMemberOwnsAccount))
	assertPageLabel(t, &h.router, "Sample member")

	h.click("Add member", false)
	h.focused(&members.draft.Editor)
	h.router.Queue(key.EditEvent{Text: "  QA Member  "})
	h.frame()
	chooseDirectoryIcon(h, "home", "Home")
	if members.icon != "home" || h.shell.overlay.Trigger != nil || members.editMode != directoryCreate {
		t.Fatalf("icon selection changed edit state: icon=%q overlay=%v mode=%d", members.icon, h.shell.overlay.Trigger != nil, members.editMode)
	}
	h.click("Add", false)
	finish()
	check := func(name, icon string, archived bool) domain.Member {
		t.Helper()
		rows, err := service.ListMembers(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		index := slices.IndexFunc(rows, func(row domain.Member) bool { return row.Name == name })
		if index < 0 {
			t.Fatalf("member %q not found in %v", name, rows)
		}
		row := rows[index]
		if row.IconKey == nil || *row.IconKey != icon || (row.ArchivedAt != nil) != archived {
			t.Fatalf("incorrect member icon or archive state: %+v", row)
		}
		return row
	}
	added := check("QA Member", "home", false)
	searchDirectory(h, "Search members", "qa member")
	assertPageLabel(t, &h.router, added.Name)
	h.click("Edit", false)
	h.frame()
	replaceDirectoryName(h, members, "")
	chooseDirectoryIcon(h, "Camera", "Camera")
	h.click("Save", false)
	finish()
	assertPageLabel(t, &h.router, i18n.English.Text(i18n.PreviewNameRequired))
	check(added.Name, "home", false)
	replaceDirectoryName(h, members, "QA Member Renamed")
	h.click("Save", false)
	finish()
	assertPageLabel(t, &h.router, "The change was saved. Retry to reload the list.")
	assertPageLabel(t, &h.router, added.Name)
	edited := check("QA Member Renamed", "camera", false)
	if edited.ID != added.ID {
		t.Fatal("the member edit replaced its ID")
	}
	h.click("Retry reload", false)
	finish()
	assertPageLabel(t, &h.router, edited.Name)
	if svc.memberEdits != 2 {
		t.Fatalf("reload replayed the edit (%d writes)", svc.memberEdits)
	}

	h.click("Archive", false)
	h.press(key.NameEscape)
	check(edited.Name, "camera", false)
	h.press(key.NameReturn)
	h.press(key.NameTab)
	h.press(key.NameReturn)
	finish()
	assertPageLabel(t, &h.router, "No matching entries")
	check(edited.Name, "camera", true)
	h.click("Clear search", true)

	// In this disposable ledger, archiving the sample account removes the
	// ownership guard. The original member is now blocked by the last-member guard.
	accounts, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(accounts) != 1 {
		t.Fatalf("expected the synthetic account, got %v, %v", accounts, err)
	}
	if err := service.ArchiveAccount(ctx, accounts[0].Account.ID, true); err != nil {
		t.Fatal(err)
	}
	h.click("Archive", false)
	h.press(key.NameTab)
	h.press(key.NameReturn)
	finish()
	assertPageLabel(t, &h.router, i18n.English.Text(i18n.DirectoryLastActiveMember))
	assertPageLabel(t, &h.router, "Sample member")

	h.click("Show archived", false)
	finish()
	searchDirectory(h, "Search members", "QA MEMBER")
	assertPageLabel(t, &h.router, "Archived")
	h.click("Restore", false)
	finish()
	assertPageLabel(t, &h.router, "Entry restored.")
	if restored := check(edited.Name, "camera", false); restored.ID != added.ID {
		t.Fatal("restore replaced the member ID")
	}
	close()

	service, close = open()
	defer close()
	if saved := check(edited.Name, "camera", false); saved.ID != added.ID {
		t.Fatal("restored member did not persist")
	}
}

func TestGroupDirectoryKeepsReferencesAndDoesNotReplayArchive(t *testing.T) {
	ctx, open := newDirectoryFixture(t)
	service, close := open()
	svc := &directoryFaultService{Service: service}
	h, page, finish := directoryHarness(t, ctx, svc)
	searchDirectory(h, "Search members", "Sample member")
	h.click("Groups", false)
	finish()
	groups := page.activeList()
	h.click("Add group", false)
	h.router.Queue(key.EditEvent{Text: "QA Group"})
	h.frame()
	chooseDirectoryIcon(h, "Ethereum", "Ethereum (ETH logo)")
	h.click("Add", false)
	finish()
	assertPageLabel(t, &h.router, "QA Group")
	check := func(name, icon string, archived bool) domain.Group {
		t.Helper()
		rows, err := service.ListGroups(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		index := slices.IndexFunc(rows, func(row domain.Group) bool { return row.Name == name })
		if index < 0 {
			t.Fatalf("group %q not found in %v", name, rows)
		}
		row := rows[index]
		if row.IconKey == nil || *row.IconKey != icon || (row.ArchivedAt != nil) != archived {
			t.Fatalf("unexpected group icon or state: %+v", row)
		}
		return row
	}
	added := check("QA Group", "crypto-logo:eth", false)
	searchDirectory(h, "Search groups", "  qa group  ")
	h.click("Edit", false)
	h.frame()
	replaceDirectoryName(h, groups, "QA Group Renamed")
	chooseDirectoryIcon(h, "Folder", "Folder")
	h.click("Save", false)
	finish()
	assertPageLabel(t, &h.router, "QA Group Renamed")
	edited := check("QA Group Renamed", "folder", false)
	if edited.ID != added.ID {
		t.Fatal("the group edit replaced its ID")
	}

	accounts, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(accounts) != 1 {
		t.Fatalf("expected the synthetic account, got %v, %v", accounts, err)
	}
	accountID := accounts[0].Account.ID
	if _, err := service.UpdateAccount(ctx, accountID, application.AccountInput{GroupID: edited.ID.String(), GroupIDSet: true}); err != nil {
		t.Fatal(err)
	}
	checkAccount := func() {
		t.Helper()
		rows, err := service.ListAccounts(ctx, domain.AccountFilter{})
		if err != nil || len(rows) != 1 {
			t.Fatalf("expected the synthetic account, got %v, %v", rows, err)
		}
		if row := rows[0]; row.Account.ID != accountID || row.Account.GroupID == nil || *row.Account.GroupID != added.ID || row.GroupName != edited.Name {
			t.Fatalf("group reference changed: %+v", row)
		}
	}
	h.click("Archive", false)
	assertPageLabel(t, &h.router, "Archive group?")
	h.press(key.NameTab)
	h.press(key.NameReturn)
	finish()
	assertPageLabel(t, &h.router, "The change was saved. Retry to reload the list.")
	check(edited.Name, "folder", true)
	checkAccount()
	_, err = service.UpdateAccount(ctx, accountID, application.AccountInput{GroupID: added.ID.String(), GroupIDSet: true})
	var invalid *domain.Error
	if !errors.As(err, &invalid) || invalid.Code != domain.ErrValidation || invalid.Field != "groupId" {
		t.Fatalf("explicitly selecting the archived group should be rejected: %v", err)
	}
	checkAccount()

	h.click("Members", false)
	finish()
	assertPageLabel(t, &h.router, "Sample member")
	if page.activeList().query != "Sample member" {
		t.Fatal("the group filter replaced the member filter")
	}
	h.click("Groups", false)
	finish()
	assertPageLabel(t, &h.router, "The change was saved. Retry to reload the list.")
	h.click("Retry reload", false)
	finish()
	assertPageLabel(t, &h.router, "No matching entries")
	if svc.groupArchive != 1 {
		t.Fatalf("reload replayed the archive (%d writes)", svc.groupArchive)
	}
	h.click("Show archived", false)
	finish()
	assertPageLabel(t, &h.router, edited.Name)
	assertPageLabel(t, &h.router, "Archived")
	h.click("Restore", false)
	finish()
	assertPageLabel(t, &h.router, "Entry restored.")
	check(edited.Name, "folder", false)
	checkAccount()
	close()

	service, close = open()
	defer close()
	if saved := check(edited.Name, "folder", false); saved.ID != added.ID {
		t.Fatal("restored group did not persist")
	}
	checkAccount()
}

func searchDirectory(h *previewHarness, label, value string) {
	h.t.Helper()
	h.click(label, true)
	h.router.Queue(key.EditEvent{Text: value})
	h.frame()
}

func replaceDirectoryName(h *previewHarness, list *directoryList, value string) {
	h.t.Helper()
	h.focused(&list.draft.Editor)
	h.router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: list.draft.Editor.Len()}, Text: value})
	h.frame()
}

func chooseDirectoryIcon(h *previewHarness, query, label string) {
	h.t.Helper()
	h.click("Icon", true)
	h.click("Filter options", true)
	h.router.Queue(key.EditEvent{Text: query})
	h.frame()
	h.click(label, false)
	h.frame()
}

func TestDirectoryIconChoicesHaveVisibleGlyphs(t *testing.T) {
	for _, language := range []i18n.Language{i18n.English, i18n.SimplifiedChinese, i18n.TraditionalChinese, i18n.Japanese} {
		for _, choice := range directoryIconChoices[language] {
			if choice.Glyph == nil {
				t.Errorf("%s: %q has no directory icon", language, choice.ID)
			}
		}
	}
}
