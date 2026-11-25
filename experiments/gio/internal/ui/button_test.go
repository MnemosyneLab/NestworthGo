package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

func testView() View {
	palette := Palette{}.WithSemantics(false)
	return View{Language: i18n.English, Colors: &palette, Theme: NewTheme(), Motion: &Motion{}}
}

func testContext(ops *op.Ops, router *input.Router, now time.Time, maxWidth int) layout.Context {
	gtx := layout.Context{
		Ops: ops, Now: now, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(maxWidth, 400)},
	}
	if router != nil {
		gtx.Source = router.Source()
	}
	return gtx
}

func TestButtonsShareControlHeightsAcrossVariants(t *testing.T) {
	view := testView()
	var ops op.Ops
	var click widget.Clickable
	for _, variant := range []ButtonVariant{ButtonPrimary, ButtonSoft, ButtonDestructive, ButtonOutline, ButtonGhost, ButtonLink} {
		for size, want := range map[ButtonSize]unit.Dp{ButtonDefault: ControlDefault, ButtonSmall: ControlSmall, ButtonLarge: ControlLarge} {
			ops.Reset()
			dims := view.ButtonOf(&click, ButtonSpec{Variant: variant, Size: size, Title: "Save"})(testContext(&ops, nil, time.Unix(1, 0), 600))
			if dims.Size.Y != int(want) {
				t.Errorf("variant %d size %d: height %d, want %v", variant, size, dims.Size.Y, want)
			}
		}
	}
}

func TestIconButtonIsSquareAndFillStretches(t *testing.T) {
	view := testView()
	var ops op.Ops
	var icon, wide widget.Clickable
	glyph := MustIcon(icons.ContentAdd)
	dims := view.ButtonOf(&icon, ButtonSpec{Name: "Add", Icon: glyph})(testContext(&ops, nil, time.Unix(1, 0), 600))
	if dims.Size.X != dims.Size.Y || dims.Size.Y != int(ControlDefault) {
		t.Fatalf("icon-only button is %v, want a %ddp square", dims.Size, int(ControlDefault))
	}
	ops.Reset()
	dims = view.ButtonOf(&wide, ButtonSpec{Title: "Continue", Fill: true})(testContext(&ops, nil, time.Unix(1, 0), 480))
	if dims.Size.X != 480 {
		t.Fatalf("filled button width = %d, want 480", dims.Size.X)
	}
	ops.Reset()
	narrow := view.ButtonOf(&wide, ButtonSpec{Title: "A much longer title than fits in this small column"})(testContext(&ops, nil, time.Unix(1, 0), 120))
	if narrow.Size.X > 120 {
		t.Fatalf("button overflowed its constraint: %d", narrow.Size.X)
	}
}

func TestDisabledButtonDoesNotClick(t *testing.T) {
	view := testView()
	var click widget.Clickable
	var ops op.Ops
	var router input.Router
	now := time.Unix(1, 0)
	frame := func(disabled bool) {
		ops.Reset()
		now = now.Add(16 * time.Millisecond)
		gtx := testContext(&ops, &router, now, 300)
		if disabled {
			gtx = gtx.Disabled()
		}
		view.ButtonOf(&click, ButtonSpec{Title: "Save"})(gtx)
		router.Frame(&ops)
	}
	for _, disabled := range []bool{true, false} {
		frame(disabled)
		frame(disabled)
		pos := f32.Pt(20, 10)
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
		frame(disabled)
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
		ops.Reset()
		now = now.Add(16 * time.Millisecond)
		gtx := testContext(&ops, &router, now, 300)
		if disabled {
			gtx = gtx.Disabled()
		}
		// Pages read Clicked before laying the button out; Layout consumes clicks.
		clicked := click.Clicked(gtx)
		view.ButtonOf(&click, ButtonSpec{Title: "Save"})(gtx)
		router.Frame(&ops)
		if clicked == disabled {
			t.Fatalf("disabled=%v: clicked=%v", disabled, clicked)
		}
	}
}

func TestMotionRegistryDropsControlsThatStoppedDrawing(t *testing.T) {
	var m Motion
	a, b := new(widget.Clickable), new(widget.Clickable)
	m.For(a)
	m.For(b)
	m.EndFrame()
	m.For(a)
	m.EndFrame()
	if m.Len() != 1 {
		t.Fatalf("registry kept %d entries, want 1", m.Len())
	}
	var nilMotion *Motion
	if nilMotion.For(a) == nil || nilMotion.Len() != 0 {
		t.Fatal("a nil Motion must hand out throwaway state")
	}
	nilMotion.EndFrame()
}

func TestHoverTweenFadesInAcrossFrames(t *testing.T) {
	view := testView()
	var click widget.Clickable
	var ops op.Ops
	var router input.Router
	now := time.Unix(1, 0)
	draw := func() {
		ops.Reset()
		now = now.Add(16 * time.Millisecond)
		view.ButtonOf(&click, ButtonSpec{Variant: ButtonOutline, Title: "Save"})(testContext(&ops, &router, now, 300))
		router.Frame(&ops)
		view.Motion.EndFrame()
	}
	draw()
	draw()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(20, 10)})
	draw()
	draw()
	state := view.Motion.For(&click)
	if state.Hover.Target() != 1 {
		t.Fatal("hover did not target 1 while the pointer is over the button")
	}
	probe := layout.Context{Now: now}
	if !state.Hover.Running(probe) {
		t.Fatal("the hover blend should still be animating a few frames in")
	}
	// Far in the future the blend has settled.
	probe.Now = now.Add(time.Second)
	if state.Hover.Running(probe) {
		t.Fatal("the hover blend never settled")
	}
}
