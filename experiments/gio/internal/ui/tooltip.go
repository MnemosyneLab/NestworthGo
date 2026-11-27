package ui

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
)

// Tooltip draws a short label beside an icon after the pointer rests on it.
// Call Layout inside the trigger's layout with its bounds in gtx.Constraints.
type Tooltip struct {
	since   time.Time
	fade    Tween
	showing bool
}

func (t *Tooltip) Layout(gtx layout.Context, view View, hovered bool, label string) {
	if !hovered || !gtx.Enabled() || label == "" || (view.Overlay != nil && view.Overlay.Trigger != nil) {
		t.since, t.showing = time.Time{}, false
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
	if !t.showing {
		t.fade.Snap(0)
		t.showing = true
	}
	t.fade.Set(gtx, 1, DurationFast, EaseOut)
	progress := t.fade.Value(gtx)
	macro := op.Record(gtx.Ops)
	fade := paint.PushOpacity(gtx.Ops, float32(progress))
	offset := op.Offset(image.Pt(gtx.Constraints.Min.X+gtx.Dp(6), 0)).Push(gtx.Ops)
	gtx.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(250), gtx.Dp(40))}
	view.Surface(gtx, SurfaceSpec{Radius: RadiusMD, Elevation: ElevationMD}, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return view.WithoutTextSelection().Line(gtx, TextCaption, label, view.Colors.Ink)
		})
	})
	offset.Pop()
	fade.Pop()
	op.Defer(gtx.Ops, macro.Stop())
}
