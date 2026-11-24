package desktop

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

type popoverPane struct {
	body           layout.Widget
	width          unit.Dp
	focus          event.Tag
	otherFocus     []event.Tag
	next, previous event.Tag
	onClose        func()
}

// popoverHost renders after the shell and uses coordinates relative to the window content.
type popoverHost struct {
	trigger       *widget.Clickable
	anchor, panel image.Rectangle
	pane          popoverPane
	dismiss       widget.Clickable
	focusOnOpen   bool
	closePending  bool
	returnFocus   event.Tag
}

func (h *popoverHost) beginFrame(gtx layout.Context) {
	// The trigger must register again to keep an overlay open.
	h.pane.body = nil
	h.dismissEvents(gtx)
}

func (h *popoverHost) dismissEvents(gtx layout.Context) {
	filters := []event.Filter{pointer.Filter{Target: h, Kinds: pointer.Press}}
	tags := append([]event.Tag{&h.dismiss, h.pane.focus}, h.pane.otherFocus...)
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
		if h.trigger == nil || h.closePending {
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
				h.close()
			}
		case pointer.Event:
			if !image.Pt(int(e.Position.X), int(e.Position.Y)).In(h.panel) {
				h.close()
			}
		}
	}
}

// register receives current visible trigger bounds. Disabled or hidden triggers
// omit registration. It reports the start of a new popup session.
func (h *popoverHost) register(gtx layout.Context, trigger *widget.Clickable, bounds image.Rectangle, pane popoverPane) bool {
	opened := trigger.Clicked(gtx)
	if opened {
		if h.trigger != trigger {
			h.reset()
		}
		h.trigger = trigger
		h.focusOnOpen = true
	}
	if h.trigger == trigger {
		h.anchor, h.pane = bounds, pane
	}
	if opened {
		// Register keys for the new focus targets before the first visible frame.
		h.dismissEvents(gtx)
	}
	return opened
}

func (h *popoverHost) close() { h.closePending = true }

func (h *popoverHost) layout(gtx layout.Context, view presentation) {
	if h.trigger == nil {
		return
	}
	if h.pane.body == nil || h.anchor.Empty() {
		h.reset()
		return
	}
	safe := (image.Rectangle{Max: gtx.Constraints.Max}).Inset(gtx.Dp(8))
	gap := gtx.Dp(6)
	below := max(0, safe.Max.Y-h.anchor.Max.Y-gap)
	above := max(0, h.anchor.Min.Y-safe.Min.Y-gap)
	if h.closePending || h.dismiss.Clicked(gtx) || safe.Empty() || max(above, below) == 0 {
		h.finish(gtx)
		return
	}

	// Measure once, then replay the complete popup at its window-relative position.
	width := min(safe.Dx(), gtx.Dp(max(300, h.pane.width)))
	popup := gtx
	popup.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, max(above, below))}
	content := op.Record(gtx.Ops)
	dims := view.panel(popup, view.colors.divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.Y = max(0, gtx.Constraints.Max.Y-gtx.Dp(42))
					return h.pane.body(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.E.Layout(gtx, view.button(&h.dismiss, view.language.Text(i18n.FieldDismiss)))
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
	h.panel = placePopover(safe, h.anchor, dims.Size, gap, below, above)
	offset := op.Offset(h.panel.Min).Push(gtx.Ops)
	draw.Add(gtx.Ops)
	offset.Pop()

	if h.focusOnOpen {
		focus := h.pane.focus
		if focus == nil {
			focus = &h.dismiss
		}
		gtx.Execute(key.FocusCmd{Tag: focus})
		h.focusOnOpen = false
	}
}

func (h *popoverHost) finish(gtx layout.Context) {
	target := h.returnFocus
	if target == nil {
		target = h.trigger
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

func (h *popoverHost) reset() {
	if h.pane.onClose != nil {
		h.pane.onClose()
	}
	h.trigger, h.returnFocus = nil, nil
	h.pane = popoverPane{}
	h.anchor, h.panel = image.Rectangle{}, image.Rectangle{}
	h.focusOnOpen, h.closePending = false, false
}
