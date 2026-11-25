package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
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
	for _, chrome := range []fieldChrome{{Tag: &tag}, {Tag: &tag, Active: true, Ring: true}, {Tag: &tag, Invalid: true, Active: true, Ring: true}, {Tag: &tag, Disabled: true}} {
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
		view.fieldFrame(gtx, fieldChrome{Tag: &tag, Active: active, Ring: active}, content)
		view.Motion.EndFrame()
	}
	frame(start, false)
	frame(start.Add(time.Millisecond), true)
	mid := view.Motion.For(&tag).Select.Value(testContext(&ops, nil, start.Add(DurationBase/2), 300))
	frame(start.Add(DurationBase*2), true)
	end := view.Motion.For(&tag).Select.Value(testContext(&ops, nil, start.Add(DurationBase*2), 300))
	if mid <= 0 || mid >= 1 || end != 1 {
		t.Fatalf("focus tween mid=%v end=%v, want mid in (0,1) and end 1", mid, end)
	}
}

func TestSelectPanelMatchesItsTriggerAndGrowsOnlyWhenNeeded(t *testing.T) {
	for _, c := range []struct {
		name         string
		anchor, need int
		want         int
	}{
		{"wide trigger, short options", 400, 120, 400},
		{"narrow trigger takes the minimum", 100, 90, int(MinSelectPanelWidth)},
		{"narrow trigger grows to fit", 100, 250, 250},
		{"growth stops at 24rem", 100, 900, int(MaxSelectPanelWidth)},
		{"a wider trigger sets its own cap", 500, 900, 500},
	} {
		t.Run(c.name, func(t *testing.T) {
			view := testView()
			var overlay Overlay
			var buf op.Ops
			view.Overlay = &overlay
			overlay.Trigger = new(widget.Clickable)
			overlay.Anchor = image.Rect(20, 100, 20+c.anchor, 136)
			overlay.pane = Popover{
				fit:     true,
				measure: func(layout.Context, View) int { return c.need },
				Body:    func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(10, 40)} },
			}
			gtx := testContext(&buf, nil, time.Unix(1, 0), 1200)
			gtx.Constraints.Max.Y = 800
			overlay.Layout(gtx, view)
			if got := overlay.Panel.Dx(); got != c.want {
				t.Fatalf("panel width = %d, want %d", got, c.want)
			}
		})
	}
}

func TestOptionHighlightAppearsOnlyAfterKeyboardUse(t *testing.T) {
	items := []Choice{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	var list optionList
	list.setActive(items, "b", false)
	if list.keyboard || list.highlight != "b" {
		t.Fatalf("a mouse open must keep the current value without showing a highlight: %+v", list)
	}
	list.move(items, 1)
	if !list.keyboard || list.highlight != "c" {
		t.Fatalf("arrow navigation must show the highlight: %+v", list)
	}
	list.setActive(items, "a", true)
	if !list.keyboard {
		t.Fatal("a keyboard open must show the highlight at once")
	}
}
