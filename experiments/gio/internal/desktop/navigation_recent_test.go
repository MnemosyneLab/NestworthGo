package desktop

import (
	"fmt"
	"slices"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestRecentMenuOpensNewVisitAndSurvivesForwardTruncation(t *testing.T) {
	h := newShellHarness(t)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	s := h.shell
	h.click("Component Preview", false)
	preview := s.componentPreview
	h.click("Notes", true)
	h.router.Queue(key.EditEvent{Text: "Draft kept by Recent"})
	h.frame()
	h.click("Settings", false)
	h.click("Back", false)
	if got := recentTitles(s); !slices.Equal(got, []string{"Settings", "Component Preview", "Overview"}) {
		t.Fatalf("leaving through Back produced %v", got)
	}
	h.click("Recent", false)
	if s.overlay.Trigger != &s.recentPicker.Trigger || s.overlay.Panel.Empty() {
		t.Fatal("Recent did not open its menu")
	}
	h.press(key.NameEscape)
	h.focused(&s.recentPicker.Trigger)
	selectRecent(h, "Overview")
	if s.activeRoute() != overviewRoute || s.trail.adjacent(1) != nil || s.trail.adjacent(-1).target.route != componentPreviewRoute {
		t.Fatal("Recent moved the cursor instead of opening a new branch")
	}
	if got := recentTitles(s); !slices.Equal(got, []string{"Component Preview", "Settings", "Overview"}) {
		t.Fatalf("branch truncation changed independent recency: %v", got)
	}
	selectRecent(h, "Component Preview")
	if s.activeRoute() != componentPreviewRoute || preview.notes.Text() != "Draft kept by Recent" || preview.submissions != 0 {
		t.Fatal("reopening through Recent lost or submitted the draft")
	}
	h.focused(&preview.notes.Editor)
	h.click("Back", false)
	if s.activeRoute() != overviewRoute {
		t.Fatal("Back did not go to the source of the Recent selection")
	}
	h.click("Forward", false)
	if s.activeRoute() != componentPreviewRoute {
		t.Fatal("Forward did not restore the visit created by Recent")
	}
	count := len(s.trail.entries)
	selectRecent(h, "Component Preview")
	if len(s.trail.entries) != count {
		t.Fatal("choosing the current effective destination duplicated the visit")
	}

	fresh := newShellHarness(t)
	if len(fresh.shell.trail.recent) != 0 {
		t.Fatal("a new window inherited another session's Recent menu")
	}
}

func TestRecentRestoresDistinctDirectoryScopesWithoutExposingSearch(t *testing.T) {
	ctx, open := newDirectoryFixture(t)
	service, close := open()
	defer close()
	if _, err := service.CreateGroup(ctx, "Public Sample Group"); err != nil {
		t.Fatal(err)
	}
	groups := newGroupsList(ctx, service, func() {})
	directory := newDirectoryPage(groups)
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
	h.click("Show archived", false)
	finish()
	searchDirectory(h, "Search groups", "Public Sample")
	h.click("Settings", false)
	h.click("Directory", false)
	h.click("Hide archived", false)
	finish()
	h.click("Clear search", true)
	h.click("Overview", false)
	selectRecent(h, "Directory · Groups · Filtered")
	finish()
	if !groups.includeArchived || groups.query != "Public Sample" {
		t.Fatalf("Recent failed to restore the selected filter: %+v", groups.visit())
	}
	h.focused(&groups.search.Field.Editor)
	assertPageLabel(t, &h.router, "Public Sample Group")
	h.click("Back", false)
	if h.shell.activeRoute() != overviewRoute {
		t.Fatal("opening a filtered snapshot lost its source")
	}
	selectRecent(h, "Directory · Groups")
	finish()
	if groups.includeArchived || groups.query != "" {
		t.Fatal("recent visits with different directory scopes were conflated")
	}
}

func TestRecentBoundsDeduplicationAndUpdatedVisit(t *testing.T) {
	trail := newNavigationTrail(overviewRoute)
	for i := range recentLimit + 5 {
		trail.recordRecent(navigationEntry{
			id: uint64(i + 1), name: fmt.Sprint(i),
			target: navigationTarget{route: directoryRoute, object: directoryRef{kind: i18n.NavMembers, id: fmt.Sprint(i)}},
		})
	}
	if len(trail.recent) != recentLimit || trail.recent[0].name != "24" || trail.recent[recentLimit-1].name != "5" {
		t.Fatal("recent destinations were not ordered and bounded by departure")
	}
	target := trail.recent[3].target
	trail.recordRecent(navigationEntry{id: 99, target: target, name: "Current name"})
	if trail.recent[0].target != target || slices.ContainsFunc(trail.recent[1:], func(entry navigationEntry) bool { return entry.target == target }) {
		t.Fatal("an equivalent object target was not moved to the front")
	}
	trail.rename(target, "")
	if trail.recent[0].name != "" {
		t.Fatal("an unavailable object retained its stale label")
	}

	visit := navigationEntry{id: 100, target: navigationTarget{route: directoryRoute}}
	visit.directory.list.query = "Last visit"
	trail.recordRecent(visit)
	visit.id++
	visit.directory.list.query = "  last VISIT  "
	trail.recordRecent(visit)
	visit.directory.list.query = "Updated visit"
	trail.recordRecent(visit)
	for _, entry := range trail.recent[1:] {
		if entry.target == visit.target {
			t.Fatal("an edited visit or equivalent spelling left an obsolete snapshot")
		}
	}
}

func recentTitles(s *shell) []string {
	var titles []string
	for i := range s.trail.recent {
		titles = append(titles, s.visitLabel(i18n.English, &s.trail.recent[i]))
	}
	return titles
}

func selectRecent(h *previewHarness, title string) {
	h.t.Helper()
	index := slices.Index(recentTitles(h.shell), title)
	if index < 0 {
		h.t.Fatalf("Recent %q missing from %v", title, recentTitles(h.shell))
	}
	h.click("Recent", false)
	if h.shell.overlay.Trigger != &h.shell.recentPicker.Trigger {
		h.t.Fatal("Recent menu did not open")
	}
	for range index {
		h.press(key.NameDownArrow)
	}
	h.press(key.NameReturn)
	h.frame()
}
