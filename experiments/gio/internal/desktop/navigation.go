package desktop

import (
	"fmt"
	"slices"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

const navigationLimit = 40

type navigationTarget struct {
	route  routeID
	object directoryRef
}

func (t navigationTarget) hasObject() bool { return t.object != (directoryRef{}) }

type navigationEntry struct {
	target    navigationTarget
	name      string
	focus     event.Tag
	scroll    layout.Position
	directory directoryVisit
	visited   bool
}

type navigationTrail struct {
	entries []navigationEntry
	cursor  int
}

func newNavigationTrail(route routeID) navigationTrail {
	return navigationTrail{entries: []navigationEntry{{target: navigationTarget{route: route}}}}
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
	n.entries = append(n.entries, entry)
	n.cursor = len(n.entries) - 1
}

func (n *navigationTrail) rename(target navigationTarget, name string) {
	for i := range n.entries {
		if n.entries[i].target == target {
			n.entries[i].name = name
		}
	}
}

type navigationRequest struct {
	target navigationTarget
	name   string
	focus  event.Tag
	step   int
}

func (s *shell) activeRoute() routeID { return s.trail.current().target.route }

func (s *shell) navigate(gtx layout.Context, request navigationRequest) {
	if !gtx.Enabled() || s.settingsPage.confirming || s.confirmingArchive() {
		return
	}
	entry := s.trail.current()
	if request.step != 0 {
		if s.trail.adjacent(request.step) == nil {
			return
		}
	} else {
		item := s.routes[request.target.route]
		if item == nil || !item.available() || entry.target == request.target {
			return
		}
		if request.target.hasObject() && (request.target.route != directoryRoute || s.directory == nil) {
			return
		}
	}

	s.saveVisit(entry)
	if request.focus != nil {
		entry.focus = request.focus
	}
	if entry.target.hasObject() {
		s.directory.object.leave()
	}
	if request.step != 0 {
		s.trail.cursor += request.step
		s.restoreVisit(s.trail.current())
	} else {
		s.trail.push(navigationEntry{target: request.target, name: request.name})
	}
	entry = s.trail.current()
	if entry.target.hasObject() {
		ref := entry.target.object
		list := s.directory.objectList(ref)
		if list == nil {
			s.trail.rename(entry.target, "")
		}
		s.directory.object.enter(ref, list)
	}
	s.narrowExpanded = false
	s.restoreFocus = request.step != 0 || entry.target.hasObject()
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
		return language.Text(i18n.DirectoryObjectUnavailable)
	}
	return language.Text(s.routes[entry.target.route].label)
}
