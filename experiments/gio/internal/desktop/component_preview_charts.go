package desktop

import (
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

// previewExample is one chart with its own controlled selection. The preview
// owns the selection and applies every request, which is what a page does when
// it has nothing to load for a selected item.
type previewExample struct {
	caption  i18n.MessageID
	chart    any
	selected charts.Selection
	draw     func(gtx layout.Context, view ui.View, selected charts.Selection) (layout.Dimensions, []charts.Event)
	// table builds the data table of a chart type that has one, as the pages
	// show it under their line and donut charts.
	table func(view ui.View, selected charts.Selection) (int, func() charts.Table)
	data  chartDataTable
}

// previewChartGroup shows every example of one chart type.
type previewChartGroup struct {
	title    i18n.MessageID
	examples []*previewExample
}

// newPreviewExample binds a chart widget to a spec builder. build receives the
// current selection, so the spec always carries the selection the preview holds.
func newPreviewExample[S any](caption i18n.MessageID, chart any,
	layoutChart func(layout.Context, ui.View, S) (layout.Dimensions, []charts.Event),
	build func(ui.View, charts.Selection) S) *previewExample {
	return &previewExample{
		caption: caption, chart: chart,
		draw: func(gtx layout.Context, view ui.View, selected charts.Selection) (layout.Dimensions, []charts.Event) {
			return layoutChart(gtx, view, build(view, selected))
		},
		table: func(view ui.View, selected charts.Selection) (int, func() charts.Table) {
			labels := chartTableLabels(view.Language, i18n.ChartDetails)
			switch spec := any(build(view, selected)).(type) {
			case charts.LineSpec:
				return lineTableItems(spec), func() charts.Table { return spec.Table(labels) }
			case charts.DonutSpec:
				return len(spec.Slices), func() charts.Table { return spec.Table(labels) }
			case charts.BarSpec:
				return len(spec.Bars), func() charts.Table { return spec.Table(labels) }
			case charts.WaterfallSpec:
				return len(spec.Steps), func() charts.Table { return spec.Table(labels) }
			case charts.SankeySpec:
				return len(spec.Nodes) + len(spec.Links), func() charts.Table { return spec.Table(labels) }
			}
			return 0, nil
		},
	}
}

// newPreviewChartGroups lists one group per chart type, each with a typical and
// an edge-case example. Sankey also has a dense allocation example.
func newPreviewChartGroups() []previewChartGroup {
	lineStandard, lineEdge := new(charts.LineChart), new(charts.LineChart)
	barsStandard, barsEdge := new(charts.BarChart), new(charts.BarChart)
	waterfallStandard, waterfallEdge := new(charts.WaterfallChart), new(charts.WaterfallChart)
	donutStandard, donutEdge := new(charts.DonutChart), new(charts.DonutChart)
	sankeyStandard, sankeyEdge, sankeyDense := new(charts.SankeyChart), new(charts.SankeyChart), new(charts.SankeyChart)
	return []previewChartGroup{
		{i18n.ChartTitleLine, []*previewExample{
			newPreviewExample(i18n.ChartExampleStandard, lineStandard, lineStandard.Layout, previewWealthLine),
			newPreviewExample(i18n.ChartExampleEdge, lineEdge, lineEdge.Layout, previewReturnLine),
		}},
		{i18n.ChartTitleBars, []*previewExample{
			newPreviewExample(i18n.ChartExampleStandard, barsStandard, barsStandard.Layout, previewStandardBars),
			newPreviewExample(i18n.ChartExampleEdge, barsEdge, barsEdge.Layout, previewEdgeBars),
		}},
		{i18n.ChartTitleWaterfall, []*previewExample{
			newPreviewExample(i18n.ChartExampleStandard, waterfallStandard, waterfallStandard.Layout, previewStandardWaterfall),
			newPreviewExample(i18n.ChartExampleEdge, waterfallEdge, waterfallEdge.Layout, previewEdgeWaterfall),
		}},
		{i18n.ChartTitleDonut, []*previewExample{
			newPreviewExample(i18n.ChartExampleStandard, donutStandard, donutStandard.Layout, previewStandardDonut),
			newPreviewExample(i18n.ChartExampleEdge, donutEdge, donutEdge.Layout, previewEdgeDonut),
		}},
		{i18n.ChartTitleSankey, []*previewExample{
			newPreviewExample(i18n.ChartExampleStandard, sankeyStandard, sankeyStandard.Layout, previewStandardSankey),
			newPreviewExample(i18n.ChartExampleEdge, sankeyEdge, sankeyEdge.Layout, previewEdgeSankey),
			newPreviewExample(i18n.ChartExampleDense, sankeyDense, sankeyDense.Layout, previewDenseSankey),
		}},
	}
}

// chartsSection lays out the heading, the help line and one card per chart type.
func (p *componentPreview) chartsSection(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	children := []layout.FlexChild{
		layout.Rigid(view.Message(l.Text(i18n.ChartPreviewHelp), view.Colors.SecondaryInk)),
	}
	for i := range p.chartGroups {
		group := &p.chartGroups[i]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.chartCard(gtx, view, group)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space12)}.Layout(gtx, children...)
}

func (p *componentPreview) chartCard(gtx layout.Context, view ui.View, group *previewChartGroup) layout.Dimensions {
	l := view.Language
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(ui.Space12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(view.Body(ui.TextLabel, l.Text(group.title), view.Colors.Ink)),
			}
			for _, example := range group.examples {
				children = append(children,
					layout.Rigid(view.Body(ui.TextCaptionStrong, l.Text(example.caption), view.Colors.SecondaryInk)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						view.Focus.Watch(gtx, example.chart)
						dims, events := example.draw(gtx, view, example.selected)
						if len(events) > 0 {
							example.selected = charts.Apply(example.selected, events)
							gtx.Execute(op.InvalidateCmd{})
						}
						return dims
					}))
				if items, build := example.table(view, example.selected); items > 0 {
					children = append(children, layout.Rigid(example.data.layout(view, items, build)))
				}
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space12)}.Layout(gtx, children...)
		})
	})
}
