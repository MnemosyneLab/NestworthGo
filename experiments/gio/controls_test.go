package main

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

type previewHarness struct {
	t       *testing.T
	shell   *shell
	preview *componentPreview
	router  input.Router
	ops     op.Ops
	now     time.Time
	size    image.Point
}

func newShellHarness(t *testing.T) *previewHarness {
	t.Helper()
	brand := image.NewRGBA(image.Rect(0, 0, 1, 1))
	return &previewHarness{t: t, shell: newShell(brand, brand), size: image.Pt(1120, 840), now: time.Unix(1, 0)}
}

func newPreviewHarness(t *testing.T) *previewHarness {
	t.Helper()
	h := newShellHarness(t)
	preview := newComponentPreview()
	h.preview = preview
	h.shell.routes[componentPreviewRoute].body = preview.layout
	h.shell.activeRoute = componentPreviewRoute
	h.shell.preferences.language = english
	h.frame()
	return h
}

func (h *previewHarness) frame() {
	h.ops.Reset()
	h.now = h.now.Add(time.Second / 60)
	h.shell.layout(layout.Context{
		Ops: &h.ops, Source: h.router.Source(), Now: h.now,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(h.size),
	})
	h.router.Frame(&h.ops)
}

func (h *previewHarness) click(label string, field bool) {
	h.t.Helper()
	bounds := semanticBounds(h.t, &h.router, label, field)
	h.clickAt(bounds.Min.Add(bounds.Max).Div(2))
}

func (h *previewHarness) clickAt(point image.Point) {
	pos := f32.Pt(float32(point.X), float32(point.Y))
	h.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
	h.frame()
	h.router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
	h.frame()
}

func (h *previewHarness) press(name key.Name) {
	h.router.Queue(key.Event{Name: name, State: key.Press})
	h.frame()
	h.router.Queue(key.Event{Name: name, State: key.Release})
	h.frame()
}

func (h *previewHarness) focused(tag event.Tag) {
	h.t.Helper()
	if !h.router.Source().Focused(tag) {
		h.t.Fatalf("expected keyboard focus on %T", tag)
	}
}

func TestPreviewTextSearchAndExplicitSubmission(t *testing.T) {
	h := newPreviewHarness(t)
	p := h.preview
	h.click("Submit sample", false)
	if p.submissions != 0 || p.nameProblem(p.name.text()) != previewNameRequired {
		t.Fatal("an empty institution name was accepted")
	}
	h.focused(&p.name.editor)

	// Simulate toolkit events to cover editor routing, independently of native IME.
	h.router.Queue(key.EditEvent{Text: "ni"}, key.CompositionEvent{Start: 0, End: 2})
	h.frame()
	h.press(key.NameReturn)
	h.router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 2}, Text: "你"}, key.CompositionEvent{Start: -1, End: -1})
	h.frame()
	if p.name.text() != "你" || p.submissions != 0 {
		t.Fatalf("composition changed form submission or text: %q, %d", p.name.text(), p.submissions)
	}

	overBudget := strings.Repeat("字", institutionNameBudget/3+1)
	h.router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 1}, Text: overBudget})
	h.frame()
	h.click("Submit sample", false)
	if p.name.text() != overBudget || p.submissions != 0 {
		t.Fatal("byte validation truncated or accepted an over-budget draft")
	}
	h.router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: p.name.editor.Len()}, Text: "Harbor Bank"})
	h.frame()

	h.click("Search institutions", true)
	h.router.Queue(key.EditEvent{Text: "Harbor"})
	h.frame()
	h.press(key.NameReturn)
	if p.query != "Harbor" || p.submissions != 0 {
		t.Fatalf("unexpected search or submission: %q, %d", p.query, p.submissions)
	}
	h.router.MoveFocus(key.FocusForward)
	h.frame()
	h.focused(&p.search.clear)
	h.press(key.NameReturn)
	if p.query != "" || p.search.field.text() != "" {
		t.Fatal("the clear button did not report the empty query")
	}
	h.focused(&p.search.field.editor)

	h.click("Submit sample", false)
	if p.submissions != 1 || p.submittedName != "Harbor Bank" {
		t.Fatalf("explicit submit did not commit the sample draft: %d, %q", p.submissions, p.submittedName)
	}

	h.click("Read-only example", true)
	h.focused(&p.readOnly.editor)
	h.router.Queue(key.EditEvent{Text: "replacement"})
	h.frame()
	if p.readOnly.text() != "Harbor Bank" {
		t.Fatal("the read-only field accepted an edit")
	}
	h.click("Disabled example", true)
	if h.router.Source().Focused(&p.disabled.editor) {
		t.Fatal("the disabled field received focus")
	}
}

func TestPreviewPopoverFollowsAnchorAndRestoresFocus(t *testing.T) {
	h := newPreviewHarness(t)
	host := &h.shell.overlay
	anchor := &h.preview.info
	h.click("About this form", false)
	h.focused(&host.dismiss)
	initial := host.panel
	if initial.Empty() || initial.Max.Y >= host.anchor.Min.Y {
		t.Fatalf("popover should flip above the footer trigger: panel %v, trigger %v", initial, host.anchor)
	}

	h.size = image.Pt(700, 460)
	h.frame()
	window := image.Rectangle{Max: h.size}
	if host.panel.Empty() || !host.panel.In(window) || host.panel == initial || host.panel.Max.Y >= host.anchor.Min.Y {
		t.Fatalf("popover did not follow its anchor inside the resized window: %v", host.panel)
	}
	h.clickAt(host.panel.Min.Add(image.Pt(18, 14)))
	if host.trigger != anchor {
		t.Fatal("a press inside the panel dismissed it")
	}
	h.press(key.NameEscape)
	if host.trigger != nil {
		t.Fatal("Escape did not dismiss the popover")
	}
	h.focused(anchor)

	h.press(key.NameReturn)
	h.focused(&host.dismiss)
	h.click("Submit sample", false)
	if host.trigger != nil || h.preview.checkName {
		t.Fatal("the outside press failed to close the popover or activated the covered button")
	}
	h.focused(anchor)

	h.press(key.NameReturn)
	h.focused(&host.dismiss)
	h.router.Queue(input.SystemEvent{Event: key.Event{Name: key.NameTab, State: key.Press, Modifiers: key.ModShift}})
	h.frame()
	if host.trigger != nil {
		t.Fatal("Shift-Tab did not dismiss the popover")
	}
	h.focused(anchor)

	h.press(key.NameReturn)
	h.focused(&host.dismiss)
	h.click("Close", false)
	if host.trigger != nil {
		t.Fatal("the close button failed to dismiss the popover")
	}
	h.focused(anchor)

	h.press(key.NameReturn)
	h.shell.activeRoute = overviewRoute
	h.frame()
	if host.trigger != nil || h.router.Source().Focused(anchor) {
		t.Fatal("removing the trigger left the popover or its focus active")
	}
}

func TestPopoverPlacementPrefersBelowWhenItFits(t *testing.T) {
	safe := image.Rect(8, 8, 692, 452)
	anchor := image.Rect(636, 32, 670, 64)
	size := image.Pt(300, 96)
	got := placePopover(safe, anchor, size, 6, 382, 18)
	if want := image.Rect(392, 70, 692, 166); got != want {
		t.Fatalf("got panel %v, want %v", got, want)
	}
}
