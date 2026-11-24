package main

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"
)

// popoverHost renders after the shell and uses coordinates relative to the window content.
type popoverHost struct {
	trigger       *widget.Clickable
	anchor, panel image.Rectangle
	body          layout.Widget
	dismiss       widget.Clickable
	focusOnOpen   bool
	closePending  bool
}

func (h *popoverHost) beginFrame(gtx layout.Context) {
	// The trigger must register again to keep an overlay open.
	h.body = nil
	for {
		e, ok := gtx.Event(
			pointer.Filter{Target: h, Kinds: pointer.Press},
			key.Filter{Focus: &h.dismiss, Name: key.NameEscape},
			key.Filter{Focus: &h.dismiss, Name: key.NameTab, Optional: key.ModShift},
		)
		if !ok {
			return
		}
		if h.trigger == nil {
			continue
		}
		switch e := e.(type) {
		case key.Event:
			h.closePending = h.closePending || e.State == key.Press
		case pointer.Event:
			h.closePending = h.closePending || !image.Pt(int(e.Position.X), int(e.Position.Y)).In(h.panel)
		}
	}
}

// register receives the current visible trigger bounds. Disabled or hidden
// triggers omit registration. The body is laid out only while the popover is open.
func (h *popoverHost) register(gtx layout.Context, trigger *widget.Clickable, bounds image.Rectangle, body layout.Widget) {
	if trigger.Clicked(gtx) {
		h.trigger = trigger
		h.focusOnOpen = true
	}
	if h.trigger == trigger {
		h.anchor, h.body = bounds, body
	}
}

func (h *popoverHost) layout(gtx layout.Context, view presentation) {
	if h.trigger == nil {
		return
	}
	if h.body == nil || h.anchor.Empty() {
		h.reset()
		return
	}
	safe := (image.Rectangle{Max: gtx.Constraints.Max}).Inset(gtx.Dp(8))
	gap := gtx.Dp(6)
	below := max(0, safe.Max.Y-h.anchor.Max.Y-gap)
	above := max(0, h.anchor.Min.Y-safe.Min.Y-gap)
	if h.closePending || h.dismiss.Clicked(gtx) || safe.Empty() || max(above, below) == 0 {
		gtx.Execute(key.FocusCmd{Tag: h.trigger})
		h.reset()
		return
	}

	mask := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, h)
	mask.Pop()

	// Measure once, then replay the complete popup at its window-relative position.
	width := min(safe.Dx(), gtx.Dp(300))
	popup := gtx
	popup.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, max(above, below))}
	content := op.Record(gtx.Ops)
	dims := view.panel(popup, view.colors.divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(h.body),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.E.Layout(gtx, view.button(&h.dismiss, view.language.text(fieldDismiss)))
				}),
			)
		})
	})
	draw := content.Stop()
	h.panel = placePopover(safe, h.anchor, dims.Size, gap, below, above)
	offset := op.Offset(h.panel.Min).Push(gtx.Ops)
	draw.Add(gtx.Ops)
	offset.Pop()

	if h.focusOnOpen {
		// The popup starts on a button. Editors keep their own composition keys.
		gtx.Execute(key.FocusCmd{Tag: &h.dismiss})
		h.focusOnOpen = false
	}
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
	h.trigger, h.body = nil, nil
	h.anchor, h.panel = image.Rectangle{}, image.Rectangle{}
	h.focusOnOpen, h.closePending = false, false
}
