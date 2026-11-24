package desktop

import (
	"fmt"
	"slices"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const navigationLimit, recentLimit = 40, 20

type navigationTarget struct {
	route       routeID
	object      directoryRef
	account     domain.AccountID
	institution domain.InstitutionID
}

func (t navigationTarget) hasObject() bool { return t.object != (directoryRef{}) || t.account != "" }

type navigationEntry struct {
	id               uint64
	target           navigationTarget
	name             string
	focus            event.Tag
	scroll           layout.Position
	directory        directoryVisit
	accountsArchived bool
	visited          bool
}

type navigationTrail struct {
	entries []navigationEntry
	cursor  int
	recent  []navigationEntry
	nextID  uint64
}

func newNavigationTrail(route routeID) navigationTrail {
	return navigationTrail{entries: []navigationEntry{{id: 1, target: navigationTarget{route: route}}}, nextID: 1}
}

func (n *navigationTrail) current() *navigationEntry { return &n.entries[n.cursor] }

func (n *navigationTrail) adjacent(step int) *navigationEntry {
	if i := n.cursor + step; i >= 0 && i < len(n.entries) {
		return &n.entries[i]
	}
	return nil
}

func (n *navigationTrail) push(entry navigationEntry) {
	n.entries = slices.Delete(n.entries, n.cursor+1, len(n.entries))
	if len(n.entries) == navigationLimit {
		n.entries = slices.Delete(n.entries, 0, 1)
	}
	n.nextID++
	entry.id = n.nextID
	n.entries = append(n.entries, entry)
	n.cursor = len(n.entries) - 1
}

func (n *navigationTrail) rename(target navigationTarget, name string) {
	for _, entries := range [][]navigationEntry{n.entries, n.recent} {
		for i := range entries {
			if entries[i].target == target {
				entries[i].name = name
			}
		}
	}
}

func sameRecentDestination(a, b navigationEntry) bool {
	if a.target != b.target {
		return false
	}
	if a.target.route == accountsRoute && !a.target.hasObject() {
		return a.accountsArchived == b.accountsArchived
	}
	if a.target.route != directoryRoute || a.target.hasObject() {
		return true
	}
	return a.directory.tab == b.directory.tab &&
		a.directory.list.includeArchived == b.directory.list.includeArchived &&
		normalizeDirectoryQuery(a.directory.list.query) == normalizeDirectoryQuery(b.directory.list.query)
}

func (n *navigationTrail) recordRecent(entry navigationEntry) {
	n.recent = slices.DeleteFunc(n.recent, func(previous navigationEntry) bool {
		return previous.id == entry.id || sameRecentDestination(previous, entry)
	})
	n.recent = slices.Insert(n.recent, 0, entry)
	if len(n.recent) > recentLimit {
		n.recent = slices.Delete(n.recent, recentLimit, len(n.recent))
	}
}

type navigationRequest struct {
	target           navigationTarget
	name             string
	focus            event.Tag
	step             int
	recent           *navigationEntry
	accountsArchived bool
}

func (s *shell) activeRoute() routeID { return s.trail.current().target.route }

func (s *shell) navigate(gtx layout.Context, request navigationRequest) {
	if !gtx.Enabled() || s.settingsPage.confirming || s.confirmingArchive() || s.confirmingAccountCreation() || s.savingAccountInstitution() || s.accountMaintenanceBlocking() {
		return
	}
	if s.historyPage != nil && s.historyPage.contributions.writing() {
		return
	}
	entry := s.trail.current()
	if request.step != 0 {
		if s.trail.adjacent(request.step) == nil {
			return
		}
	} else {
		item := s.routes[request.target.route]
		if item == nil || !item.available() || (request.recent == nil && entry.target == request.target) {
			return
		}
		if request.target.object != (directoryRef{}) && (request.target.route != directoryRoute || s.directory == nil) {
			return
		}
		if (request.target.account != "" || request.target.institution != "") && (request.target.route != accountsRoute || s.accounts == nil) {
			return
		}
	}

	source := *entry
	s.saveVisit(&source)
	if request.recent != nil && sameRecentDestination(source, *request.recent) {
		return
	}
	if request.focus != nil {
		source.focus = request.focus
	}
	*entry = source
	s.trail.recordRecent(source)
	if entry.target.route == directoryRoute && entry.target.hasObject() {
		s.directory.object.leave()
	}
	if entry.target.route == accountsRoute {
		s.accounts.leave()
		s.accounts.creation.discard()
		if m := s.accounts.maintenance; m != nil {
			m.draft, m.focus, m.issue, m.success = nil, nil, i18n.NoMessage, i18n.NoMessage
		}
		if entry.target.account == s.accounts.createdID {
			s.accounts.createdID = ""
		}
	}
	if request.step != 0 {
		s.trail.cursor += request.step
		s.restoreVisit(s.trail.current())
	} else if request.recent != nil {
		s.trail.push(*request.recent)
		s.restoreVisit(s.trail.current())
	} else {
		s.trail.push(navigationEntry{target: request.target, name: request.name, accountsArchived: request.accountsArchived})
	}
	entry = s.trail.current()
	if entry.target.route == directoryRoute && entry.target.hasObject() {
		ref := entry.target.object
		list := s.directory.objectList(ref)
		if list == nil {
			s.trail.rename(entry.target, "")
		}
		s.directory.object.enter(ref, list)
	}
	if entry.target.route == accountsRoute {
		s.accounts.enter(entry.target, entry.accountsArchived, entry.scroll)
	}
	s.narrowExpanded = false
	s.restoreFocus = request.step != 0 || request.recent != nil || entry.target.hasObject()
	gtx.Execute(op.InvalidateCmd{})
}

func (s *shell) updateNavigation(gtx layout.Context) {
	if s.pendingNavigation != nil {
		request := *s.pendingNavigation
		s.pendingNavigation = nil
		s.navigate(gtx, request)
		return
	}
	switch {
	case s.back.Clicked(gtx):
		s.navigate(gtx, navigationRequest{step: -1})
		return
	case s.forward.Clicked(gtx):
		s.navigate(gtx, navigationRequest{step: 1})
		return
	}
	for _, group := range s.groups {
		for i := range group.items {
			item := &group.items[i]
			if item.available() && item.button.Clicked(gtx) {
				s.navigate(gtx, navigationRequest{target: navigationTarget{route: item.id}})
				return
			}
		}
	}
}

func (s *shell) saveVisit(entry *navigationEntry) {
	entry.visited = true
	switch entry.target.route {
	case accountsRoute:
		entry.accountsArchived = s.accounts.archived
		entry.scroll = s.accounts.list.Position
	case overviewRoute:
		if s.overviewPage != nil {
			entry.scroll = s.overviewPage.list.Position
		}
	case directoryRoute:
		if s.directory != nil && !entry.target.hasObject() {
			entry.directory = s.directory.visit()
		}
	case settingsRoute:
		entry.scroll = s.settingsPage.scroll.Position
	case historyRoute:
		if s.historyPage != nil {
			entry.scroll = s.historyPage.list.Position
		}
	case componentPreviewRoute:
		entry.scroll = s.componentPreview.scroll.Position
	}
}

func (s *shell) restoreVisit(entry *navigationEntry) {
	if !entry.visited {
		return
	}
	switch entry.target.route {
	case overviewRoute:
		if s.overviewPage != nil {
			s.overviewPage.list.Position = restoredPosition(entry.scroll, s.overviewPage.rowCount())
		}
	case directoryRoute:
		if s.directory != nil && !entry.target.hasObject() {
			s.directory.restore(entry.directory)
		}
	case settingsRoute:
		s.settingsPage.scroll.Position = entry.scroll
	case historyRoute:
		if s.historyPage != nil {
			s.historyPage.list.Position = restoredPosition(entry.scroll, len(s.historyPage.model.Rows()))
		}
	case componentPreviewRoute:
		s.componentPreview.scroll.Position = entry.scroll
	}
}

func (s *shell) finishPageFocus(gtx layout.Context, focus ui.PageFocus) {
	entry := s.trail.current()
	if focus.Current != nil {
		entry.focus = focus.Current
	}
	if !s.restoreFocus || !gtx.Enabled() {
		return
	}
	// A user focus change while a scope is being reloaded takes precedence.
	if focus.Current != nil && focus.Current != focus.Target {
		s.restoreFocus = false
		return
	}
	if !focus.TargetDrawn && focus.Target != nil && s.waitingForPageFocus() {
		return
	}
	target := focus.Target
	if !focus.TargetDrawn {
		target = &s.headingFocus
	}
	gtx.Execute(key.FocusCmd{Tag: target})
	s.restoreFocus = false
}

func (s *shell) waitingForPageFocus() bool {
	if s.activeRoute() == accountsRoute && s.accounts != nil {
		return s.accounts.request != nil
	}
	if s.activeRoute() != directoryRoute || s.directory == nil {
		return false
	}
	if s.trail.current().target.hasObject() {
		return s.directory.object.result != nil
	}
	return s.directory.activeList().restoreVisit != nil
}

func (s *shell) navigationButton(gtx layout.Context, view ui.View, step int) layout.Dimensions {
	button, hint := &s.back, &s.backHint
	label, targetLabel := i18n.NavigationBack, i18n.NavigationBackTo
	if step > 0 {
		button, hint = &s.forward, &s.forwardHint
		label, targetLabel = i18n.NavigationForward, i18n.NavigationForwardTo
	}
	title := view.Language.Text(label)
	name := title
	if entry := s.trail.adjacent(step); entry != nil {
		name = fmt.Sprintf(view.Language.Text(targetLabel), s.visitLabel(view.Language, entry))
	} else {
		gtx = gtx.Disabled()
	}
	dims := view.NamedButton(button, title, name)(gtx)
	gtx.Constraints = layout.Exact(dims.Size)
	hint.Layout(gtx, view, button.Hovered(), name)
	return dims
}

func (s *shell) visitLabel(language i18n.Language, entry *navigationEntry) string {
	if entry.target.hasObject() {
		if entry.name != "" {
			return entry.name
		}
		if entry.target.route == accountsRoute {
			return language.Text(i18n.AccountUnavailable)
		}
		return language.Text(i18n.DirectoryObjectUnavailable)
	}
	title := language.Text(s.routes[entry.target.route].label)
	if entry.target.route == accountsRoute {
		if entry.target.institution != "" {
			if entry.name == "" {
				return title + " · " + language.Text(i18n.AccountReferenceUnavailable)
			}
			title += " · " + entry.name
		}
		if entry.accountsArchived {
			title += " · " + language.Text(i18n.InstitutionsShowArchived)
		}
	}
	if entry.target.route == directoryRoute && entry.visited && s.directory != nil {
		title += " · " + language.Text(s.directory.lists[entry.directory.tab].copy.title)
		if entry.directory.list.includeArchived || strings.TrimSpace(entry.directory.list.query) != "" {
			title += " · " + language.Text(i18n.NavigationFiltered)
		}
	}
	return title
}
