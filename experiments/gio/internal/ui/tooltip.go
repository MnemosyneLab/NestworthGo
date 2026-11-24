package ui

import (
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
)

// Tooltip draws a short label beside an icon after the pointer rests on it.
// Call Layout inside the trigger's layout with its bounds in gtx.Constraints.
type Tooltip struct {
	since time.Time
}

func (t *Tooltip) Layout(gtx layout.Context, view View, hovered bool, label string) {
	if !hovered || !gtx.Enabled() || label == "" || (view.Overlay != nil && view.Overlay.Trigger != nil) {
		t.since = time.Time{}
		return
	}
	if t.since.IsZero() {
		t.since = gtx.Now
	}
	showAt := t.since.Add(500 * time.Millisecond)
	if gtx.Now.Before(showAt) {
		gtx.Execute(op.InvalidateCmd{At: showAt})
		return
	}

	// Defer retains the trigger's transform while escaping its clipping area.
	// The tooltip has no input target of its own.
	macro := op.Record(gtx.Ops)
	offset := op.Offset(image.Pt(gtx.Constraints.Min.X+gtx.Dp(6), 0)).Push(gtx.Ops)
	gtx.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(250), gtx.Dp(40))}
	view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 5, Bottom: 5, Left: 9, Right: 9}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, label, 12, font.Normal, view.Colors.Ink)
		})
	})
	offset.Pop()
	op.Defer(gtx.Ops, macro.Stop())
}
