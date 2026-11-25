package main

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

// The gallery shows each chart with synthetic data in four states: a standard
// dataset, edge cases, a dataset with no usable values and an empty one.
var (
	chartNames   = [...]string{"line", "bars", "waterfall", "donut"}
	chartTitles  = [...]string{"Line chart", "Signed bars", "Waterfall", "Donut"}
	variantNames = [...]string{"Standard", "Edge cases", "No values", "Empty"}
)

type gallery struct {
	chart, variant int
	dark           bool
	showRows       bool

	tabs       [len(chartNames)]widget.Clickable
	variants   [len(variantNames)]widget.Clickable
	appearance widget.Clickable
	rows       widget.Clickable
	scroll     widget.List

	line      charts.LineChart
	bars      charts.BarChart
	waterfall charts.WaterfallChart
	donut     charts.DonutChart
	// selected is the controlled selection of each chart.
	selected [len(chartNames)]charts.Selection
}

func newGallery() *gallery {
	return &gallery{scroll: widget.List{List: layout.List{Axis: layout.Vertical}}}
}

// lineSpec, barSpec and the others return the spec for the current variant
// with the controlled selection applied.
func (g *gallery) lineSpec() charts.LineSpec {
	spec := [...]func() charts.LineSpec{wealthHistory, returnTrend, missingLine, emptyLine}[g.variant]()
	spec.Selected = g.selected[0]
	return spec
}

func (g *gallery) barSpec() charts.BarSpec {
	spec := [...]func() charts.BarSpec{standardBars, edgeBars, missingBars, func() charts.BarSpec {
		return charts.BarSpec{Label: "Contribution to return", Summary: "No holdings.", Messages: messages}
	}}[g.variant]()
	spec.Selected = g.selected[1]
	return spec
}

func (g *gallery) waterfallSpec() charts.WaterfallSpec {
	spec := [...]func() charts.WaterfallSpec{standardWaterfall, edgeWaterfall, missingWaterfall, func() charts.WaterfallSpec {
		return charts.WaterfallSpec{Label: "Asset changes", Summary: "No changes.", Messages: messages}
	}}[g.variant]()
	spec.Selected = g.selected[2]
	return spec
}

func (g *gallery) donutSpec() charts.DonutSpec {
	spec := [...]func() charts.DonutSpec{standardDonut, edgeDonut, zeroDonut, emptyDonut}[g.variant]()
	spec.Selected = g.selected[3]
	return spec
}

// sampleSelection selects one visible item of the current chart, for launching
// the gallery with a selection already made.
func (g *gallery) sampleSelection() charts.Selection {
	switch g.chart {
	case 0:
		series := g.lineSpec().Series
		if len(series) > 0 && len(series[0].Points) > 0 {
			return charts.Selection{Series: series[0].ID, Item: series[0].Points[len(series[0].Points)/2].ID}
		}
	case 1:
		if bars := g.barSpec().Bars; len(bars) > 1 {
			return charts.Selection{Item: bars[1].ID}
		}
	case 2:
		if steps := g.waterfallSpec().Steps; len(steps) > 2 {
			return charts.Selection{Item: steps[2].ID}
		}
	case 3:
		if slices := g.donutSpec().Slices; len(slices) > 1 {
			return charts.Selection{Item: slices[1].ID}
		}
	}
	return charts.Selection{}
}

func (g *gallery) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	for i := range g.tabs {
		if g.tabs[i].Clicked(gtx) {
			g.chart = i
		}
	}
	for i := range g.variants {
		if g.variants[i].Clicked(gtx) {
			g.variant = i
			g.selected[g.chart] = charts.Selection{}
		}
	}
	if g.appearance.Clicked(gtx) {
		g.dark = !g.dark
	}
	if g.rows.Clicked(gtx) {
		g.showRows = !g.showRows
	}
	return layout.Inset{Top: ui.Space16, Bottom: ui.Space16, Left: ui.Space16, Right: ui.Space16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
			layout.Rigid(view.Body(ui.TextPageTitle, "Chart gallery", view.Colors.Ink)),
			layout.Rigid(view.Body(ui.TextCaption, "Synthetic data. Tab focuses a chart; arrow keys move, Return or Space selects, Escape clears.", view.Colors.SecondaryInk)),
			layout.Rigid(g.controls(view)),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return view.List(&g.scroll).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
					return g.card(gtx, view)
				})
			}),
		)
	})
}

func (g *gallery) controls(view ui.View) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return ui.Wrap{Gap: ui.Space8}.Layout(gtx, len(g.tabs)+len(g.variants)+2, func(gtx layout.Context, i int) layout.Dimensions {
			switch {
			case i < len(g.tabs):
				return view.TabButton(&g.tabs[i], chartTitles[i], g.chart == i)(gtx)
			case i < len(g.tabs)+len(g.variants):
				v := i - len(g.tabs)
				return view.TabButton(&g.variants[v], variantNames[v], g.variant == v)(gtx)
			case i == len(g.tabs)+len(g.variants):
				title := "Dark"
				if g.dark {
					title = "Light"
				}
				return view.Button(&g.appearance, title)(gtx)
			}
			return view.Button(&g.rows, "Data rows")(gtx)
		})
	}
}

func (g *gallery) card(gtx layout.Context, view ui.View) layout.Dimensions {
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(ui.Space12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			var rows []charts.TextRow
			dims := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
				layout.Rigid(view.Body(ui.TextHeading, chartTitles[g.chart]+" · "+variantNames[g.variant], view.Colors.Ink)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					var events []charts.Event
					var dims layout.Dimensions
					switch g.chart {
					case 0:
						spec := g.lineSpec()
						rows = spec.Rows()
						dims, events = g.line.Layout(gtx, view, spec)
					case 1:
						spec := g.barSpec()
						rows = spec.Rows()
						dims, events = g.bars.Layout(gtx, view, spec)
					case 2:
						spec := g.waterfallSpec()
						rows = spec.Rows()
						dims, events = g.waterfall.Layout(gtx, view, spec)
					default:
						spec := g.donutSpec()
						rows = spec.Rows()
						dims, events = g.donut.Layout(gtx, view, spec)
					}
					g.selected[g.chart] = charts.Apply(g.selected[g.chart], events)
					return dims
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !g.showRows {
						return layout.Dimensions{}
					}
					return g.rowsText(gtx, view, rows)
				}),
			)
			return dims
		})
	})
}

// rowsText renders the text rows the way a page would build a data table.
func (g *gallery) rowsText(gtx layout.Context, view ui.View, rows []charts.TextRow) layout.Dimensions {
	const limit = 12
	lines := make([]string, 0, limit+1)
	for i, r := range rows {
		if i == limit {
			lines = append(lines, "…")
			break
		}
		line := r.Label + " — " + r.Value
		if len(r.Detail) > 0 {
			line += " (" + strings.Join(r.Detail, "; ") + ")"
		}
		if r.Selected {
			line = "▸ " + line
		}
		lines = append(lines, line)
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space2)}.Layout(gtx, func() []layout.FlexChild {
		children := make([]layout.FlexChild, 0, len(lines))
		for _, line := range lines {
			children = append(children, layout.Rigid(view.Body(ui.TextCaption, line, view.Colors.SecondaryInk)))
		}
		return children
	}()...)
}
