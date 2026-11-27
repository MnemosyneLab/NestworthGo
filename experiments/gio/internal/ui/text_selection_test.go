package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
)

func TestPageTextDragAndCopy(t *testing.T) {
	var selection TextSelection
	var router input.Router
	var ops op.Ops
	now := time.Unix(1, 0)
	view := testView().WithTextSelection(&selection)
	const amount = "余额 CN¥1,022,706.58"
	visible := true
	frame := func() layout.Dimensions {
		ops.Reset()
		now = now.Add(16 * time.Millisecond)
		gtx := testContext(&ops, &router, now, 340)
		var dims layout.Dimensions
		if visible {
			dims = view.Line(gtx, TextMoneyMD, amount, view.Colors.Ink)
		}
		selection.EndFrame()
		router.Frame(&ops)
		return dims
	}
	queue := func(kind pointer.Kind, x, y int, buttons pointer.Buttons) {
		router.Queue(pointer.Event{Kind: kind, Source: pointer.Mouse, Buttons: buttons, Position: f32.Pt(float32(x), float32(y)), Time: now.Sub(time.Unix(1, 0))})
	}
	copyText := func(want string) {
		t.Helper()
		router.Queue(key.Event{Name: "C", Modifiers: key.ModShortcut, State: key.Press})
		frame()
		if mime, data, ok := router.WriteClipboard(); !ok || mime != "application/text" || string(data) != want {
			t.Fatalf("clipboard = %q (%q, %v), want %q", data, mime, ok, want)
		}
	}

	dims := frame()
	y := dims.Size.Y / 2
	// The first gesture has no preceding hover frame.
	queue(pointer.Press, dims.Size.X-1, y, pointer.ButtonPrimary)
	if pressed := frame(); pressed != dims {
		t.Fatalf("press changed the text layout from %+v to %+v", dims, pressed)
	}
	queue(pointer.Move, 0, y, pointer.ButtonPrimary)
	frame()
	queue(pointer.Release, 0, y, 0)
	frame()
	copyText(amount)

	// A second drag on the same label must reach the same selection target.
	now = now.Add(time.Second)
	queue(pointer.Press, 0, y, pointer.ButtonPrimary)
	frame()
	queue(pointer.Move, dims.Size.X/2, y, pointer.ButtonPrimary)
	frame()
	queue(pointer.Release, dims.Size.X/2, y, 0)
	frame()
	part := selection.editor.SelectedText()
	if part == "" || part == amount {
		t.Fatalf("drag selected %q, want a portion of %q", part, amount)
	}
	copyText(part)
	tree := router.AppendSemantics(nil)
	if len(tree) == 0 || len(tree[0].Children) != 1 || tree[0].Children[0].Desc.Label != amount {
		t.Fatalf("selection lost the text's semantic label: %+v", tree)
	}

	// Removed page text cannot retain focus or send text to the clipboard.
	visible = false
	frame()
	router.Queue(key.Event{Name: "C", Modifiers: key.ModShortcut, State: key.Press})
	frame()
	if _, _, ok := router.WriteClipboard(); ok || selection.active != nil {
		t.Fatal("selection survived removal of its label")
	}
}

func TestSelectableTextKeepsListScrolling(t *testing.T) {
	var selection TextSelection
	var router input.Router
	var ops op.Ops
	state := widget.List{List: layout.List{Axis: layout.Vertical}}
	view := testView().WithTextSelection(&selection)
	frame := func() {
		ops.Reset()
		gtx := testContext(&ops, &router, time.Unix(1, 0), 300)
		gtx.Constraints = layout.Exact(image.Pt(300, 60))
		view.List(&state).Layout(gtx, 30, func(gtx layout.Context, _ int) layout.Dimensions {
			return view.Line(gtx, TextBody, "Ledger balance", view.Colors.Ink)
		})
		selection.EndFrame()
		router.Frame(&ops)
	}
	frame()
	pos := f32.Pt(24, 8)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Position: pos, Buttons: pointer.ButtonPrimary})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
	frame()
	if selection.active == nil {
		t.Fatal("list text could not receive selection focus")
	}
	// Wheel events beginning on the selected label must still reach the list.
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: pos, Scroll: f32.Pt(0, 46)})
	frame()
	if state.Position.First == 0 && state.Position.Offset == 0 {
		t.Fatal("label intercepted the list's scroll event")
	}
}

func TestSelectablePageTextDoesNotCaptureButtonOrTab(t *testing.T) {
	var selection TextSelection
	var router input.Router
	var ops op.Ops
	var first, last widget.Clickable
	var clicked bool
	view := testView().WithTextSelection(&selection)
	frame := func() {
		ops.Reset()
		gtx := testContext(&ops, &router, time.Unix(1, 0), 300)
		clicked = first.Clicked(gtx)
		layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(view.ButtonOf(&first, ButtonSpec{Name: "Open", Content: view.Body(TextBody, "Open details", view.Colors.Ink)})),
			layout.Rigid(view.Body(TextBody, "Ledger balance", view.Colors.Ink)),
			layout.Rigid(view.Button(&last, "Close")),
		)
		selection.EndFrame()
		router.Frame(&ops)
	}
	frame()
	router.MoveFocus(key.FocusForward)
	if !router.Source().Focused(&first) {
		t.Fatal("first button was skipped")
	}
	router.MoveFocus(key.FocusForward)
	if !router.Source().Focused(&last) {
		t.Fatal("static text interrupted the tab order between buttons")
	}
	pos := f32.Pt(24, 16)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Position: pos, Buttons: pointer.ButtonPrimary})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
	frame()
	if !clicked || selection.active != nil {
		t.Fatal("selectable text intercepted the button label")
	}
	if len(selection.labels) != 1 {
		t.Fatal("only the static body text should register a selection target")
	}
}
