package accessibility_test

import (
	"image"
	"strings"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

func TestDonutLegendPublishesOneSummaryPerRow(t *testing.T) {
	for _, tc := range []struct {
		name    string
		width   int
		compact bool
	}{
		{"regular", 620, false},
		{"compact", 390, true},
		{"compact stacked", 300, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bridge accessibility.TestFrame
			var router input.Router
			var ops op.Ops
			var chart charts.DonutChart
			palette := ui.Palette{}.WithSemantics(false)
			view := ui.View{Language: i18n.English, Colors: &palette, Theme: ui.NewTheme()}
			spec := charts.DonutSpec{
				Label: "Value by currency", Compact: tc.compact, NotDrawn: "No chart mark",
				Messages: charts.Messages{Missing: "Unavailable"},
				Slices: []charts.Slice{
					{ID: "usd", Label: "USD", Text: "US$125.55", Share: "67.66%", Value: charts.Num(125.55)},
					{ID: "eur", Label: "EUR"},
				},
			}
			render := func() ([]accessibility.TestElement, []charts.Event) {
				ops.Reset()
				gtx := bridge.Begin(layout.Context{
					Source: router.Source(), Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
					Constraints: layout.Constraints{Max: image.Pt(tc.width, 520)},
				}, 0, 0)
				_, events := chart.Layout(gtx, view, spec)
				router.Frame(gtx.Ops)
				return bridge.Snapshot(gtx), events
			}
			nodes, _ := render()
			var listID uint64
			var amounts, missing []accessibility.TestElement
			for _, node := range nodes {
				if node.Name == spec.Label {
					listID = node.ID
				}
				if strings.Contains(node.Name, "USD") || strings.Contains(node.Name, "US$125.55") || strings.Contains(node.Name, "67.66%") {
					amounts = append(amounts, node)
				}
				if strings.Contains(node.Name, "EUR") || strings.Contains(node.Name, "Unavailable") || strings.Contains(node.Name, "No chart mark") {
					missing = append(missing, node)
				}
			}
			if listID == 0 || len(amounts) != 1 || amounts[0].Name != "USD, US$125.55 · 67.66%" ||
				amounts[0].Parent != listID || amounts[0].Focusable || amounts[0].Pressable {
				t.Fatalf("expected one passive native USD legend summary, got %+v", amounts)
			}
			if len(missing) != 1 || missing[0].Name != "EUR, Unavailable, No chart mark" || missing[0].Parent != listID {
				t.Fatalf("expected one native legend summary for unavailable EUR, got %+v", missing)
			}
			router.MoveFocus(key.FocusForward)
			_, _ = render()
			if !router.Source().Focused(&chart) {
				t.Fatal("Tab did not focus the chart")
			}
			router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
			_, events := render()
			if len(events) != 1 || events[0] != (charts.Event{Kind: charts.EventSelect, Selection: charts.Selection{Item: "usd"}}) {
				t.Fatalf("Return did not activate the focused slice: %+v", events)
			}
		})
	}
}
