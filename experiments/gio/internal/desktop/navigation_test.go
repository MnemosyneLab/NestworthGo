package desktop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestNavigationRestoresDraftSelectionAndBranches(t *testing.T) {
	h := newShellHarness(t)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	s := h.shell
	h.click("Accounts", false)
	if s.activeRoute() != overviewRoute || len(s.trail.entries) != 1 {
		t.Fatal("an unimplemented destination entered history")
	}
	h.click("Component Preview", false)
	p := s.componentPreview
	h.click("Notes", true)
	const draft = "A draft to retain"
	h.router.Queue(key.EditEvent{Text: draft})
	h.frame()
	selection := key.Range{Start: 7, End: 2}
	h.router.Queue(key.SelectionEvent(selection), key.SnippetEvent{End: p.notes.Editor.Len()})
	h.frame()
	h.focused(&p.notes.Editor)

	h.click("Settings", false)
	h.click("Date format", true)
	h.click("DD/MM/YYYY", false)
	h.frame()
	want := s.settingsPage.draft().DateFormat
	if want != settings.DateFormatDayFirst || !s.prefs.ready {
		t.Fatal("the settings form did not capture the draft")
	}
	h.click("Back", false)
	if s.activeRoute() != componentPreviewRoute || p.notes.Text() != draft {
		t.Fatal("Back failed to restore the preview draft")
	}
	h.focused(&p.notes.Editor)
	if start, end := p.notes.Editor.Selection(); start != selection.Start || end != selection.End {
		t.Fatalf("selection after Back was %d..%d", start, end)
	}
	h.click("Forward", false)
	if s.activeRoute() != settingsRoute || s.settingsPage.draft().DateFormat != want ||
		s.prefs.saved.DateFormat == want || p.submissions != 0 {
		t.Fatal("Forward lost the draft or replayed a submission")
	}
	h.focused(&s.settingsPage.fields[4].Trigger)
	h.click("Back", false)
	h.click("Overview", false)
	if s.activeRoute() != overviewRoute || s.trail.adjacent(1) != nil || len(s.trail.entries) != 3 {
		t.Fatal("an ordinary destination did not replace the forward branch")
	}
	h.click("Overview", false)
	if len(s.trail.entries) != 3 {
		t.Fatal("selecting the current destination duplicated the visit")
	}
}

func TestDirectoryObjectNavigationRevalidatesAndDoesNotReplayWrites(t *testing.T) {
	ctx, open := newDirectoryFixture(t)
	service, close := open()
	defer close()
	svc := &navigationTestService{Service: service}
	first, err := service.CreateGroup(ctx, "Target Group")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateGroup(ctx, "Another Group"); err != nil {
		t.Fatal(err)
	}

	directory := newDirectoryPage(newMembersList(ctx, service, func() {}), newGroupsList(ctx, svc, func() {}))
	t.Cleanup(directory.wait)
	h := newShellHarnessWithPages(t, ledgerPages{directory: directory})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	s := h.shell
	finish := func() {
		h.frame()
		directory.wait()
		h.frame()
	}
	h.click("Directory", false)
	finish()
	h.click("Groups", false)
	finish()
	list := directory.activeList()
	h.click("Show archived", false)
	finish()
	searchDirectory(h, "Search groups", "Target")
	h.click("View", false)
	finish()
	assertPageLabel(t, &h.router, "Target Group")
	assertPageLabel(t, &h.router, "Active")
	if s.trail.current().target.object.id != first.ID.String() || svc.edits != 0 {
		t.Fatal("opening a group selected the wrong ID or performed a write")
	}
	h.click("Back", false)
	finish()
	h.focused(&list.buttons[first.ID.String()].view)
	if list.query != "Target" || !list.includeArchived {
		t.Fatal("the object link changed the list scope")
	}
	h.click("Edit", false)
	h.frame()
	replaceDirectoryName(h, list, "Target Renamed")
	svc.failAfterEdit = true
	h.click("Save", false)
	finish()
	assertPageLabel(t, &h.router, "The change was saved. Retry to reload the list.")
	h.click("Forward", false)
	finish()
	assertPageLabel(t, &h.router, "Target Renamed")
	if found := findPageLabel(&h.router, "Target Group"); found != nil || svc.edits != 1 {
		t.Fatal("Forward displayed the pre-edit object or replayed the edit")
	}
	h.click("Back", false)
	finish()
	assertPageLabel(t, &h.router, "The change was saved. Retry to reload the list.")
	h.click("Retry reload", false)
	finish()
	assertPageLabel(t, &h.router, "Target Renamed")
	if svc.edits != 1 {
		t.Fatal("Retry in a restored visit replayed the write")
	}

	backRect := semanticBounds(t, &h.router, "Back", false)
	h.click("Archive", false)
	assertPageLabel(t, &h.router, "Archive group?")
	h.clickAt(backRect.Min.Add(backRect.Max).Div(2))
	if !s.confirmingArchive() || s.trail.cursor != 1 {
		t.Fatal("the confirmation allowed navigation to change the trail")
	}
	h.press(key.NameTab)
	h.press(key.NameReturn)
	finish()
	h.click("Forward", false)
	finish()
	assertPageLabel(t, &h.router, "Archived")

	svc.failRead = true
	h.click("Reload", false)
	finish()
	assertPageLabel(t, &h.router, "Could not load the directory item.")
	assertPageLabel(t, &h.router, "Target Renamed")
	svc.failRead = false
	svc.omitID = first.ID
	h.click("Retry reload", false)
	finish()
	assertPageLabel(t, &h.router, "Directory item unavailable.")
	if findPageLabel(&h.router, "Target Renamed") != nil || findPageLabel(&h.router, "Another Group") != nil {
		t.Fatal("an unavailable object kept a stale row or substituted another record")
	}
	h.click("Back", false)
	finish()
	h.focused(&s.headingFocus)
	assertPageLabel(t, &h.router, "No matching entries")
	if list.query != "Target" || !list.includeArchived || svc.edits != 1 || svc.archives != 1 {
		t.Fatal("scope, filter or write count changed after traversal")
	}
	if label := s.visitLabel(i18n.English, s.trail.adjacent(1)); label != "Directory item unavailable." {
		t.Fatalf("unavailable target label = %q", label)
	}
}

type navigationTestService struct {
	*application.Service
	edits, archives int
	failAfterEdit   bool
	failRead        bool
	omitID          domain.GroupID
}

func (s *navigationTestService) EditGroup(ctx context.Context, id domain.GroupID, edit application.DirectoryEdit) (domain.Group, error) {
	s.edits++
	return s.Service.EditGroup(ctx, id, edit)
}

func (s *navigationTestService) ArchiveGroup(ctx context.Context, id domain.GroupID, archived bool) error {
	s.archives++
	return s.Service.ArchiveGroup(ctx, id, archived)
}

func (s *navigationTestService) ListGroups(ctx context.Context, includeArchived bool) ([]domain.Group, error) {
	if s.failAfterEdit || s.failRead {
		s.failAfterEdit = false
		return nil, errors.New("injected directory read failure")
	}
	rows, err := s.Service.ListGroups(ctx, includeArchived)
	for i, row := range rows {
		if row.ID == s.omitID {
			return append(rows[:i], rows[i+1:]...), err
		}
	}
	return rows, err
}

func TestNavigationRestoresDirectoryScopeAndScrollAcrossVisits(t *testing.T) {
	ctx, open := newDirectoryFixture(t)
	service, close := open()
	defer close()
	for i := range 24 {
		if _, err := service.CreateGroup(ctx, "Trip "+string(rune('A'+i))); err != nil {
			t.Fatal(err)
		}
	}
	directory := newDirectoryPage(newMembersList(ctx, service, func() {}), newGroupsList(ctx, service, func() {}))
	t.Cleanup(directory.wait)
	h := newShellHarnessWithPages(t, ledgerPages{directory: directory})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	finish := func() {
		directory.wait()
		h.frame()
	}
	h.click("Directory", false)
	finish()
	h.click("Groups", false)
	finish()
	list := directory.activeList()
	h.click("Show archived", false)
	finish()
	searchDirectory(h, "Search groups", "Trip")
	bounds := semanticBounds(t, &h.router, "View", false)
	pos := f32.Pt(float32(bounds.Min.X), float32(bounds.Min.Y))
	h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: pos, Scroll: f32.Pt(0, 510)})
	h.frame()
	first := list.list.Position.First
	if first == 0 {
		t.Fatal("fixture did not scroll")
	}
	h.click("Settings", false)
	h.click("Directory", false)
	h.click("Hide archived", false)
	finish()
	h.click("Clear search", true)
	h.click("Members", false)
	finish()

	h.click("Back", false)
	h.click("Back", false)
	finish()
	if directory.activeList() != list || !list.includeArchived || list.query != "Trip" || list.list.Position.First != first {
		t.Fatalf("older visit lost its tab, filter or scroll: %+v", directory.visit())
	}
	h.focused(&list.search.Field.Editor)
	h.click("Forward", false)
	h.click("Forward", false)
	finish()
	if directory.activeList() == list {
		t.Fatal("newer visit lost its Members tab")
	}
}

func TestNavigationLimitsAndMalformedObject(t *testing.T) {
	trail := newNavigationTrail(overviewRoute)
	for i := range navigationLimit + 12 {
		trail.push(navigationEntry{name: string(rune('a' + i))})
	}
	if len(trail.entries) != navigationLimit || trail.cursor != navigationLimit-1 {
		t.Fatalf("unbounded navigation: %+v", trail)
	}
	ctx, open := newDirectoryFixture(t)
	service, close := open()
	defer close()
	directory := newDirectoryPage(newGroupsList(ctx, service, func() {}))
	h := newShellHarnessWithPages(t, ledgerPages{directory: directory})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{
		route: directoryRoute, object: directoryRef{kind: i18n.NavDirectoryGroups, id: "malformed-object-id"},
	}}
	h.frame()
	assertPageLabel(t, &h.router, "Directory item unavailable.")
	if directory.object.result != nil || findPageLabel(&h.router, "malformed-object-id") != nil {
		t.Fatal("invalid ID was queried or displayed")
	}
	h.click("Back", false)
	if h.shell.activeRoute() != overviewRoute {
		t.Fatal("unavailable destination blocked Back")
	}
}

func TestNavigationIgnoresAReadFromAnAbandonedObject(t *testing.T) {
	ctx, open := newDirectoryFixture(t)
	service, closeFixture := open()
	defer closeFixture()
	if _, err := service.CreateGroup(ctx, "First Object"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateGroup(ctx, "Second Object"); err != nil {
		t.Fatal(err)
	}
	groups := newGroupsList(ctx, service, func() {})
	directory := newDirectoryPage(groups)
	firstStarted, secondRead := make(chan struct{}), make(chan struct{})
	releaseFirst := make(chan struct{}, 1)
	defer func() {
		close(releaseFirst)
		directory.wait()
	}()
	var objectReads atomic.Int32
	read := groups.source.list
	groups.source.list = func(ctx context.Context, archived bool) ([]directoryRow, error) {
		rows, err := read(ctx, archived)
		if archived {
			switch objectReads.Add(1) {
			case 1:
				close(firstStarted)
				<-releaseFirst // Simulate a read completed below the cancellation boundary.
			case 2:
				close(secondRead)
			}
		}
		return rows, err
	}
	h := newShellHarnessWithPages(t, ledgerPages{directory: directory})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Directory", false)
	groups.workers.Wait()
	h.frame()
	searchDirectory(h, "Search groups", "First Object")
	h.click("View", false)
	h.frame()
	select {
	case <-firstStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("the first object read did not start")
	}
	assertPageLabel(t, &h.router, "Loading directory item…")
	h.click("Back", false)
	h.click("Clear search", true)
	searchDirectory(h, "Search groups", "Second Object")
	h.click("View", false)
	h.frame()
	select {
	case <-secondRead:
	case <-time.After(3 * time.Second):
		t.Fatal("the second object blocked on the abandoned read")
	}
	releaseFirst <- struct{}{}
	directory.wait()
	h.frame()
	assertPageLabel(t, &h.router, "Second Object")
	if findPageLabel(&h.router, "First Object") != nil || h.shell.trail.adjacent(1) != nil {
		t.Fatal("the new object retained the abandoned forward branch")
	}
}
