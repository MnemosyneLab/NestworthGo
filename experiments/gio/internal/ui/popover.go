package ui

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

// Select panel bounds, 11rem and 24rem as in the Wails select panel.
const (
	MinSelectPanelWidth unit.Dp = 176
	MaxSelectPanelWidth unit.Dp = 384
)

type Popover struct {
	Body  layout.Widget
	width unit.Dp
	menu  bool
	// fit sizes the popup like a Wails select panel: as wide as its anchor,
	// growing to the widest option only when that is wider, up to 24rem or
	// the anchor width, whichever is larger. measure reports the widest option
	// in pixels, including the popup padding.
	fit            bool
	measure        func(layout.Context, View) int
	focus          event.Tag
	otherFocus     []event.Tag
	next, previous event.Tag
	onClose        func()
}

// Overlay renders after the shell and uses coordinates relative to the window content.
type Overlay struct {
	Trigger       *widget.Clickable
	Anchor, Panel image.Rectangle
	pane          Popover
	Dismiss       widget.Clickable
	focusOnOpen   bool
	closePending  bool
	returnFocus   event.Tag
	// ByKeyboard reports that the open popup was started from the keyboard, so
	// its option highlight is shown at once. A mouse open shows none.
	ByKeyboard bool
	enter      Tween
	entered    bool
}

func (h *Overlay) BeginFrame(gtx layout.Context) {
	// The trigger must register again to keep an overlay open.
	h.pane.Body = nil
	h.dismissEvents(gtx)
}

func (h *Overlay) dismissEvents(gtx layout.Context) {
	filters := []event.Filter{pointer.Filter{Target: h, Kinds: pointer.Press}}
	tags := append([]event.Tag{&h.Dismiss, h.pane.focus}, h.pane.otherFocus...)
	for _, tag := range tags {
		if tag != nil {
			filters = append(filters, key.Filter{Focus: tag, Name: key.NameEscape},
				key.Filter{Focus: tag, Name: key.NameTab, Optional: key.ModShift})
		}
	}
	for {
		e, ok := gtx.Event(filters...)
		if !ok {
			return
		}
		if h.Trigger == nil || h.closePending {
			continue
		}
		switch e := e.(type) {
		case key.Event:
			if e.State == key.Press {
				if e.Name == key.NameTab {
					h.returnFocus = h.pane.next
					if e.Modifiers.Contain(key.ModShift) {
						h.returnFocus = h.pane.previous
					}
				}
				h.Close()
			}
		case pointer.Event:
			if !image.Pt(int(e.Position.X), int(e.Position.Y)).In(h.Panel) {
				h.Close()
			}
		}
	}
}

// Register receives current visible trigger bounds. Disabled or hidden triggers
// omit registration. It reports the start of a new popup session.
func (h *Overlay) Register(gtx layout.Context, trigger *widget.Clickable, bounds image.Rectangle, pane Popover) bool {
	opened := trigger.Clicked(gtx)
	if opened {
		if h.Trigger != trigger {
			h.reset()
		}
		h.Trigger = trigger
		h.focusOnOpen = true
		h.ByKeyboard = gtx.Focused(trigger)
	}
	if h.Trigger == trigger {
		h.Anchor, h.pane = bounds, pane
	}
	if opened {
		// Register keys for the new focus targets before the first visible frame.
		h.dismissEvents(gtx)
	}
	return opened
}

// OffsetAnchor translates a registered trigger into the scroll viewport after
// its list has finished layout. Scrolling the trigger out of view dismisses it.
func (h *Overlay) OffsetAnchor(trigger *widget.Clickable, delta image.Point, viewport image.Rectangle) {
	if h.Trigger != trigger {
		return
	}
	h.Anchor = h.Anchor.Add(delta)
	if !h.Anchor.Overlaps(viewport) {
		h.Close()
	}
}

// Shift moves the open popup's anchor by a container's scroll offset. Call it
// after laying out a scrolling container, once per container, so anchors
// registered inside it follow the content. A popup whose trigger scrolled out of
// the viewport closes. It does nothing unless the trigger registered this frame.
func (h *Overlay) Shift(delta image.Point, viewport image.Rectangle) {
	if h.Trigger == nil || h.pane.Body == nil || delta == (image.Point{}) {
		return
	}
	h.Anchor = h.Anchor.Add(delta)
	if !h.Anchor.Overlaps(viewport) {
		h.Close()
	}
}

func (h *Overlay) Close() { h.closePending = true }

func (h *Overlay) Layout(gtx layout.Context, view View) {
	if h.Trigger == nil {
		return
	}
	if h.pane.Body == nil || h.Anchor.Empty() {
		h.reset()
		return
	}
	safe := (image.Rectangle{Max: gtx.Constraints.Max}).Inset(gtx.Dp(8))
	gap := gtx.Dp(6)
	below := max(0, safe.Max.Y-h.Anchor.Max.Y-gap)
	above := max(0, h.Anchor.Min.Y-safe.Min.Y-gap)
	if h.closePending || h.Dismiss.Clicked(gtx) || safe.Empty() || max(above, below) == 0 {
		h.finish(gtx)
		return
	}

	// Measure once, then replay the complete popup at its window-relative position.
	width := h.pane.width
	if !h.pane.menu {
		width = max(300, width)
	}
	pixels := min(safe.Dx(), gtx.Dp(width))
	if h.pane.fit {
		anchor := h.Anchor.Dx()
		need := 0
		if h.pane.measure != nil {
			need = h.pane.measure(gtx, view)
		}
		low, high := max(anchor, gtx.Dp(MinSelectPanelWidth)), max(anchor, gtx.Dp(MaxSelectPanelWidth))
		pixels = min(safe.Dx(), min(max(need, low), high))
	}
	popup := gtx
	popup.Constraints = layout.Constraints{Min: image.Pt(pixels, 0), Max: image.Pt(pixels, max(above, below))}
	content := op.Record(gtx.Ops)
	dims := view.Floating(popup, func(gtx layout.Context) layout.Dimensions {
		if h.pane.menu {
			return layout.UniformInset(4).Layout(gtx, h.pane.Body)
		}
		return layout.UniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.Y = max(0, gtx.Constraints.Max.Y-gtx.Dp(ControlDefault+6))
					return h.pane.Body(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.E.Layout(gtx, view.Button(&h.Dismiss, view.Language.Text(i18n.FieldDismiss)))
				}),
			)
		})
	})
	draw := content.Stop()
	if h.closePending {
		h.finish(gtx)
		return
	}

	mask := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, h)
	mask.Pop()
	h.Panel = placePopover(safe, h.Anchor, dims.Size, gap, below, above)
	// Fade in and settle a few dp toward the anchor, like Wails popovers.
	if !h.entered {
		h.enter.Snap(0)
		h.entered = true
	}
	h.enter.Set(gtx, 1, DurationBase, EaseOut)
	progress := h.enter.Value(gtx)
	slide := int(float64(gtx.Dp(4)) * (1 - progress))
	if h.Panel.Min.Y < h.Anchor.Min.Y {
		slide = -slide
	}
	offset := op.Offset(h.Panel.Min.Add(image.Pt(0, -slide))).Push(gtx.Ops)
	fade := paint.PushOpacity(gtx.Ops, float32(progress))
	draw.Add(gtx.Ops)
	fade.Pop()
	offset.Pop()

	if h.focusOnOpen {
		focus := h.pane.focus
		if focus == nil {
			focus = &h.Dismiss
		}
		gtx.Execute(key.FocusCmd{Tag: focus})
		h.focusOnOpen = false
	}
}

func (h *Overlay) finish(gtx layout.Context) {
	target := h.returnFocus
	if target == nil {
		target = h.Trigger
	}
	gtx.Execute(key.FocusCmd{Tag: target})
	gtx.Execute(op.InvalidateCmd{})
	h.reset()
}

func placePopover(safe, anchor image.Rectangle, size image.Point, gap, below, above int) image.Rectangle {
	x := min(max(anchor.Min.X, safe.Min.X), safe.Max.X-size.X)
	y := anchor.Max.Y + gap
	if size.Y > below && above > below {
		y = anchor.Min.Y - gap - size.Y
	}
	y = min(max(y, safe.Min.Y), safe.Max.Y-size.Y)
	return image.Rectangle{Min: image.Pt(x, y), Max: image.Pt(x+size.X, y+size.Y)}
}

func (h *Overlay) reset() {
	if h.pane.onClose != nil {
		h.pane.onClose()
	}
	h.Trigger, h.returnFocus = nil, nil
	h.pane = Popover{}
	h.Anchor, h.Panel = image.Rectangle{}, image.Rectangle{}
	h.focusOnOpen, h.closePending = false, false
	h.entered = false
}
