package accessibility_test

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func TestCurrencySelectPublishesActiveOptionAndReturnsNativeFocus(t *testing.T) {
	var bridge accessibility.TestFrame
	var router input.Router
	var ops op.Ops
	var picker ui.Select
	var overlay ui.Overlay
	palette := ui.Palette{}.WithSemantics(false)
	view := ui.View{Language: i18n.English, Colors: &palette, Theme: ui.NewTheme(), Overlay: &overlay, Motion: &ui.Motion{}}
	field := ui.PickerField{Label: i18n.SettingsCurrency}
	items := []ui.Choice{
		{ID: "USD", Label: "US Dollar"},
		{ID: "XXX", Label: "Unavailable currency", Disabled: true},
		{ID: "EUR", Label: "Euro"},
		{ID: "SGD", Label: "Singapore Dollar"},
	}
	value := "USD"
	frameTime := time.Unix(1, 0)
	render := func(id uint64, action accessibility.TestAction) []accessibility.TestElement {
		ops.Reset()
		frameTime = frameTime.Add(time.Second)
		gtx := bridge.Begin(layout.Context{
			Source: router.Source(), Now: frameTime, Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(400, 280)},
		}, id, action)
		overlay.BeginFrame(gtx)
		picker.Layout(gtx, view, field, ui.SelectOptions{Items: items}, value, func(id string) { value = id })
		overlay.Layout(gtx, view)
		view.Motion.EndFrame()
		router.Frame(gtx.Ops)
		return bridge.Snapshot(gtx)
	}
	find := func(nodes []accessibility.TestElement, name string, pressable bool) accessibility.TestElement {
		t.Helper()
		for _, node := range nodes {
			if node.Name == name && node.Pressable == pressable {
				return node
			}
		}
		t.Fatalf("no matching element %q in %+v", name, nodes)
		return accessibility.TestElement{}
	}
	focused := func(nodes []accessibility.TestElement, name string) accessibility.TestElement {
		t.Helper()
		var actual []accessibility.TestElement
		for _, node := range nodes {
			if node.Focused {
				actual = append(actual, node)
			}
		}
		if len(actual) != 1 || actual[0].Name != name {
			t.Fatalf("focused elements = %+v, want exactly %q", actual, name)
		}
		return actual[0]
	}
	fieldName := view.Language.Text(field.Label)
	trigger := find(render(0, 0), fieldName, true)
	shown := render(trigger.ID, accessibility.TestPress)
	if current := focused(shown, "US Dollar"); !current.Selected || current.Parent == 0 || !router.Source().Focused(&picker) {
		t.Fatalf("AX press did not focus the enabled selected option in its list: %+v", current)
	}
	disabled := find(shown, "Unavailable currency", false)
	if disabled.Enabled || disabled.Focusable {
		t.Fatalf("disabled option exposes a native action: %+v", disabled)
	}
	focused(render(disabled.ID, accessibility.TestPress), "US Dollar")
	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	if active := focused(render(0, 0), "Euro"); active.Selected || value != "USD" {
		t.Fatalf("moving the cursor selected a new value: %+v, %s", active, value)
	}
	sgd := find(shown, "Singapore Dollar", true)
	focused(render(sgd.ID, accessibility.TestFocus), "Singapore Dollar")
	if value != "USD" || !router.Source().Focused(&picker) {
		t.Fatal("native focus committed the currency or bypassed the picker's keyboard handling")
	}

	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	closed := render(0, 0)
	if restored := focused(closed, fieldName); overlay.Trigger != nil || restored.ID != trigger.ID || value != "USD" {
		t.Fatalf("Escape did not restore native focus to the original trigger: %+v", restored)
	}
	shown = render(trigger.ID, accessibility.TestPress)
	sgd = find(shown, "Singapore Dollar", true)
	if restored := focused(render(sgd.ID, accessibility.TestPress), fieldName); restored.ID != trigger.ID || value != "SGD" {
		t.Fatalf("AX selection did not commit and restore focus: %+v, value %s", restored, value)
	}
	// The trigger was drawn before the floating option handled the selection.
	if saved := focused(render(0, 0), fieldName); saved.Value != "Singapore Dollar" {
		t.Fatalf("the settled trigger did not publish the selected value: %+v", saved)
	}
}
