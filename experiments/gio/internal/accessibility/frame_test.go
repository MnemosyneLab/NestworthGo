package accessibility

import (
	"image"
	"slices"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"
)

func TestFramePublishesControlsAndDispatchesNativeActions(t *testing.T) {
	var bridge frame
	var inputRouter input.Router
	var ops op.Ops
	var click widget.Clickable
	var editor widget.Editor
	editor.SingleLine = true
	editor.SetText("A😀汉")
	editor.SetCaret(2, 1)

	label := "History"
	var disabled, hidden, clicked bool
	render := func(req request) []element {
		ops.Reset()
		gtx := bridge.start(layout.Context{
			Ops: &ops, Source: inputRouter.Source(),
			Constraints: layout.Exact(image.Pt(320, 240)),
		}, req)
		clicked = click.Clicked(gtx)
		if !hidden {
			local := gtx
			if disabled {
				local = local.Disabled()
			}
			SelectableButton(local, &click, label, true, func(gtx layout.Context) layout.Dimensions {
				return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					semantic.SelectedOp(true).Add(gtx.Ops)
					return testLabel(gtx, label)
				})
			})
		}
		{
			editorOffset := op.Offset(image.Pt(0, 35)).Push(gtx.Ops)
			ApplyEditor(gtx, &editor)
			Editor(gtx, &editor, "Search", "Search institutions", func(gtx layout.Context) layout.Dimensions {
				return testLabel(gtx, "Placeholder")
			})
			editorOffset.Pop()
			listOffset := op.Offset(image.Pt(0, 65)).Push(gtx.Ops)
			List(gtx, "history list", "History", func(gtx layout.Context) layout.Dimensions {
				return Row(gtx, "fixture activity", func(gtx layout.Context) layout.Dimensions {
					return testLabel(gtx, "Bank interest")
				})
			})
			listOffset.Pop()
		}
		bridge.snapshot(gtx)
		inputRouter.Frame(gtx.Ops)
		return bridge.last
	}
	nodes := render(request{})
	if len(nodes) != 5 {
		t.Fatalf("want one button, one editor and a list with a row and label, got %+v", nodes)
	}
	history, search, historyList, activity, note := nodes[0], nodes[1], nodes[2], nodes[3], nodes[4]
	if history.Role != button || history.Name != "History" || !history.Selected || !history.Pressable {
		t.Fatalf("unexpected history button %+v", history)
	}
	if search.Role != textField || search.Name != "Search" || search.Value != "A😀汉" ||
		search.Help != "Search institutions" || !search.Editable ||
		search.SelectionStart != 1 || search.SelectionLength != 2 {
		t.Fatalf("unexpected editor %+v", search)
	}
	if historyList.Role != list || activity.Parent != historyList.ID ||
		activity.Role != group || note.Parent != activity.ID || note.Value != "Bank interest" {
		t.Fatalf("invalid list tree %+v", nodes[2:])
	}

	render(request{id: history.ID, action: press})
	if !clicked {
		t.Fatal("native press did not reach the existing Gio button")
	}
	nodes = render(request{id: search.ID, action: setValue, value: "中😀"})
	if nodes[1].ID != search.ID || nodes[1].Value != "中😀" || nodes[1].SelectionStart != 3 {
		t.Fatalf("edit did not update the same accessibility element: %+v", nodes[1])
	}
	nodes = render(request{id: search.ID, action: setSelection, start: 1, length: 2})
	if nodes[1].SelectionStart != 1 || nodes[1].SelectionLength != 2 {
		t.Fatalf("UTF-16 selection did not round trip: %+v", nodes[1])
	}

	editor.ReadOnly = true
	nodes = render(request{})
	if nodes[1].Editable || !nodes[1].Focusable {
		t.Fatalf("read-only editor should still expose focus: %+v", nodes[1])
	}
	nodes = render(request{id: search.ID, action: setValue, value: "blocked edit"})
	if nodes[1].Value != "中😀" {
		t.Fatalf("read-only editor accepted a write: %+v", nodes[1])
	}
	nodes = render(request{id: search.ID, action: setSelection, start: 0, length: 1})
	if nodes[1].SelectionStart != 0 || nodes[1].SelectionLength != 1 {
		t.Fatalf("read-only editor lost text selection: %+v", nodes[1])
	}
	editor.Mask = '•'
	nodes = render(request{})
	if nodes[1].Value != "••" || editor.Text() != "中😀" {
		t.Fatal("the masked editor exposed or changed its value")
	}

	label = "Another day"
	nodes = render(request{})
	if nodes[0].ID == history.ID {
		t.Fatal("a reused button kept the ID of its previous action")
	}
	reusedID := nodes[0].ID
	render(request{id: history.ID, action: press})
	if clicked {
		t.Fatal("an old accessibility element activated a reused widget")
	}

	disabled = true
	nodes = render(request{})
	if nodes[0].Enabled || nodes[0].Pressable {
		t.Fatalf("disabled button still exposes actions: %+v", nodes[0])
	}
	render(request{id: reusedID, action: press})
	if clicked {
		t.Fatal("a disabled control received an action")
	}
	hidden = true
	nodes = render(request{})
	if slices.ContainsFunc(nodes, func(e element) bool { return e.ID == reusedID }) {
		t.Fatal("hidden button was still published")
	}
	render(request{id: reusedID, action: press})
	if clicked {
		t.Fatal("a stale accessibility element reached the hidden button")
	}
}

func TestFrameModalExcludesBackgroundAndRetiresItsActions(t *testing.T) {
	var bridge frame
	var router input.Router
	var ops op.Ops
	var sidebar, closeButton widget.Clickable
	var modal, backgroundPressed, closePressed bool
	render := func(req request) []element {
		ops.Reset()
		gtx := bridge.start(layout.Context{Ops: &ops, Source: router.Source(), Constraints: layout.Exact(image.Pt(320, 240))}, req)
		background := func(gtx layout.Context) layout.Dimensions {
			backgroundPressed = sidebar.Clicked(gtx)
			Button(gtx, &sidebar, "Overview", "", false, func(gtx layout.Context) layout.Dimensions {
				return testLabel(gtx, "Overview")
			})
			defer op.Offset(image.Pt(0, 30)).Push(gtx.Ops).Pop()
			testLabel(gtx, "Ledger balance")
			return layout.Dimensions{Size: image.Pt(320, 240)}
		}
		if modal {
			Hidden(gtx.Disabled(), background)
			Modal(gtx, &modal, "Ledger history", func(gtx layout.Context) layout.Dimensions {
				closePressed = closeButton.Clicked(gtx)
				return Button(gtx, &closeButton, "Close history", "", false, func(gtx layout.Context) layout.Dimensions {
					return testLabel(gtx, "Close history")
				})
			})
		} else {
			background(gtx)
		}
		router.Frame(gtx.Ops)
		bridge.snapshot(gtx)
		return bridge.last
	}
	before := render(request{})
	if len(before) != 2 || before[0].Name != "Overview" || before[1].Name != "Ledger balance" {
		t.Fatalf("background was missing before opening the sheet: %+v", before)
	}
	modal = true
	shown := render(request{})
	if len(shown) != 2 || shown[0].Role != sheet || shown[0].Name != "Ledger history" ||
		shown[1].Parent != shown[0].ID || !shown[1].Pressable || shown[1].Name != "Close history" {
		t.Fatalf("expected only the named sheet and its close control, got %+v", shown)
	}
	render(request{id: before[0].ID, action: press})
	if backgroundPressed {
		t.Fatal("a background element retained its AX action under the sheet")
	}
	render(request{id: shown[1].ID, action: press})
	if !closePressed {
		t.Fatal("the active sheet's close action did not reach its Gio control")
	}
	modal = false
	restored := render(request{})
	if len(restored) != 2 || restored[0].Name != "Overview" || !restored[0].Pressable || restored[1].Name != "Ledger balance" {
		t.Fatalf("the underlying tree did not return when the sheet closed: %+v", restored)
	}
}

func testLabel(gtx layout.Context, label string) layout.Dimensions {
	size := image.Pt(100, 25)
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	semantic.LabelOp(label).Add(gtx.Ops)
	return layout.Dimensions{Size: size}
}
