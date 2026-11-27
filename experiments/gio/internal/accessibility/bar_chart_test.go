package accessibility_test

import (
	"image"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

func TestBarChartNativeRowsKeepOneKeyboardStop(t *testing.T) {
	var bridge accessibility.TestFrame
	var router input.Router
	var ops op.Ops
	var chart charts.BarChart
	var after widget.Clickable
	palette := ui.Palette{}.WithSemantics(false)
	view := ui.View{Language: i18n.English, Colors: &palette, Theme: ui.NewTheme(), Motion: &ui.Motion{}}
	spec := charts.BarSpec{
		Label: "Change contributions", Summary: "Decreases to the left, increases to the right.",
		Symmetric: true, ActionableRows: true,
		Bars: []charts.Bar{
			{ID: "price", Label: "Price Change", Text: "+US$12.50", Value: charts.Num(12.50)},
			{ID: "fee", Label: "Fees", Text: "-US$4.00", Value: charts.Num(-4)},
		},
	}
	var events []charts.Event
	render := func(id uint64, action accessibility.TestAction) []accessibility.TestElement {
		ops.Reset()
		gtx := bridge.Begin(layout.Context{
			Source: router.Source(), Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(620, 520)},
		}, id, action)
		dims, got := chart.Layout(gtx, view, spec)
		events = got
		offset := op.Offset(image.Pt(0, dims.Size.Y+12)).Push(gtx.Ops)
		view.Button(&after, "After chart")(gtx)
		offset.Pop()
		view.Motion.EndFrame()
		router.Frame(gtx.Ops)
		return bridge.Snapshot(gtx)
	}
	row := func(nodes []accessibility.TestElement, title, amount string) accessibility.TestElement {
		t.Helper()
		var chartID uint64
		var matching []accessibility.TestElement
		for _, node := range nodes {
			if node.Name == spec.Label {
				chartID = node.ID
			}
			if strings.Contains(node.Name, title) || strings.Contains(node.Name, amount) {
				matching = append(matching, node)
			}
		}
		if chartID == 0 || len(matching) != 1 || matching[0].Parent != chartID ||
			matching[0].Name != title+", "+amount || matching[0].Pressable != spec.ActionableRows {
			t.Fatalf("expected one complete row in the native chart: %+v", matching)
		}
		return matching[0]
	}
	focused := func(nodes []accessibility.TestElement, name string) {
		t.Helper()
		var matching []string
		for _, node := range nodes {
			if node.Focused {
				matching = append(matching, node.Name)
			}
		}
		if len(matching) != 1 || matching[0] != name {
			t.Fatalf("native focus = %v, want only %q", matching, name)
		}
	}
	selected := func(id charts.ID) {
		t.Helper()
		if len(events) != 1 || events[0] != (charts.Event{Kind: charts.EventSelect, Selection: charts.Selection{Item: id}}) {
			t.Fatalf("selection events = %+v, want one activation of %q", events, id)
		}
	}

	initial := render(0, 0)
	price := row(initial, "Price Change", "+US$12.50")
	fee := row(initial, "Fees", "-US$4.00")
	inspecting := render(fee.ID, accessibility.TestFocus)
	focused(inspecting, fee.Name)
	row(inspecting, "Fees", "-US$4.00")
	if len(events) != 0 || !router.Source().Focused(&chart) {
		t.Fatal("native row focus should move the chart cursor without activating the row")
	}
	router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	focused(render(0, 0), price.Name)
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	render(0, 0)
	selected("price")

	focused(render(fee.ID, accessibility.TestPress), fee.Name)
	selected("fee")
	router.MoveFocus(key.FocusForward)
	focused(render(0, 0), "After chart")
	router.Source().Execute(key.FocusCmd{Tag: &chart})
	focused(render(0, 0), fee.Name)

	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary,
		Position: f32.Pt(float32(price.X+price.Width/2), float32(price.Y+price.Height/2))})
	render(0, 0)
	selected("price")

	spec.ActionableRows = false
	passive := render(0, 0)
	row(passive, "Fees", "-US$4.00")
}
