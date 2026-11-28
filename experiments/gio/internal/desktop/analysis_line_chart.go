package desktop

import (
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

const analysisWideChartMinWidth = 610

// analysisLineCard is a titled card holding a line chart and, under it, the
// chart's data table disclosure with the given column titles. A nil table
// leaves the disclosure out.
func analysisLineCard(view ui.View, chart *charts.LineChart, spec charts.LineSpec, selected *charts.Selection, table *chartDataTable, labels charts.TableLabels) layout.Widget {
	return analysisLineCardTable(view, chart, spec, selected, table, lineTableItems(spec), func(current charts.LineSpec) charts.Table {
		return current.Table(labels)
	})
}

// analysisLineCardTable is analysisLineCard with the owner's own table of the
// given number of rows, for a chart whose table lists more than its points.
// build receives the spec with the current selection.
func analysisLineCardTable(view ui.View, chart *charts.LineChart, spec charts.LineSpec, selected *charts.Selection, table *chartDataTable, items int, build func(charts.LineSpec) charts.Table) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			rows := []ui.ColumnChild{
				ui.Rigid(func(gtx layout.Context) layout.Dimensions {
					return (layout.Inset{Top: 10, Left: 10, Right: 10}).Layout(gtx, view.Body(ui.TextLabel, spec.Label, view.Colors.Ink))
				}),
				ui.Rigid(analysisLineBodyTable(view, chart, spec, selected, table, items, build)),
			}
			return (ui.Column{Gap: 4}).Layout(gtx, rows...)
		})
	}
}

// analysisLineBodyTable lays out the chart and its data-table disclosure inside
// a card owned by the caller. It keeps the chart's focus and selection wiring
// shared with the standalone card form.
func analysisLineBodyTable(view ui.View, chart *charts.LineChart, spec charts.LineSpec, selected *charts.Selection, table *chartDataTable, items int, build func(charts.LineSpec) charts.Table) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		current := spec
		current.Selected = *selected
		if gtx.Constraints.Max.X < gtx.Dp(analysisWideChartMinWidth) {
			current.Height = 180
		}
		rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			view.Focus.Watch(gtx, chart)
			dims, events := chart.Layout(gtx, view, current)
			if len(events) > 0 {
				*selected = charts.Apply(*selected, events)
				gtx.Execute(op.InvalidateCmd{})
			}
			return dims
		})}
		if table != nil {
			rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (layout.Inset{Left: 10, Right: 10, Bottom: 10}).Layout(gtx,
					table.layout(view, items, func() charts.Table { return build(current) }))
			}))
		}
		return (ui.Column{Gap: 4}).Layout(gtx, rows...)
	}
}

// analysisLineBody is the chart alone, for a section that already sits on a
// card and has its own title.
func analysisLineBody(view ui.View, chart *charts.LineChart, spec charts.LineSpec, selected *charts.Selection) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		current := spec
		current.Selected = *selected
		if gtx.Constraints.Max.X < gtx.Dp(analysisWideChartMinWidth) {
			current.Height = 180
		}
		view.Focus.Watch(gtx, chart)
		dims, events := chart.Layout(gtx, view, current)
		if len(events) > 0 {
			*selected = charts.Apply(*selected, events)
			gtx.Execute(op.InvalidateCmd{})
		}
		return dims
	}
}
