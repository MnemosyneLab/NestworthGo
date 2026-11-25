package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
)

func TestSurfaceAndFieldFrameKeepTheContentSize(t *testing.T) {
	view := testView()
	var ops op.Ops
	content := func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(120, 36)}
	}
	gtx := testContext(&ops, nil, time.Unix(0, 0), 300)
	if got := view.Card(gtx, content).Size; got != image.Pt(120, 36) {
		t.Fatalf("card size = %v, want the content size", got)
	}
	if got := view.Floating(gtx, content).Size; got != image.Pt(120, 36) {
		t.Fatalf("floating size = %v, want the content size", got)
	}
	var tag struct{ _ byte }
	for _, chrome := range []fieldChrome{{Tag: &tag}, {Tag: &tag, Active: true}, {Tag: &tag, Invalid: true, Active: true}, {Tag: &tag, Disabled: true}} {
		if got := view.fieldFrame(gtx, chrome, content).Size; got != image.Pt(120, 36) {
			t.Fatalf("field frame %+v size = %v, want the content size", chrome, got)
		}
	}
}

func TestFieldFrameFocusRingFadesIn(t *testing.T) {
	view := testView()
	var ops op.Ops
	var tag struct{ _ byte }
	start := time.Unix(0, 0)
	content := func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(100, 36)} }
	frame := func(at time.Time, active bool) {
		ops.Reset()
		gtx := testContext(&ops, nil, at, 300)
		view.fieldFrame(gtx, fieldChrome{Tag: &tag, Active: active}, content)
		view.Motion.EndFrame()
	}
	frame(start, false)
	frame(start.Add(time.Millisecond), true)
	mid := view.Motion.For(&tag).Focus.Value(testContext(&ops, nil, start.Add(DurationBase/2), 300))
	frame(start.Add(DurationBase*2), true)
	end := view.Motion.For(&tag).Focus.Value(testContext(&ops, nil, start.Add(DurationBase*2), 300))
	if mid <= 0 || mid >= 1 || end != 1 {
		t.Fatalf("focus tween mid=%v end=%v, want mid in (0,1) and end 1", mid, end)
	}
}
