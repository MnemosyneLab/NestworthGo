package desktop

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

const analysisWideChartMinWidth = 610

func analysisLineCard(view ui.View, chart *charts.LineChart, spec charts.LineSpec, selected *charts.Selection) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		current := spec
		current.Selected = *selected
		if gtx.Constraints.Max.X < gtx.Dp(analysisWideChartMinWidth) {
			current.Height = 180
		}
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: 4}).Layout(gtx,
				ui.Rigid(func(gtx layout.Context) layout.Dimensions {
					return (layout.Inset{Top: 10, Left: 10, Right: 10}).Layout(gtx, view.Body(ui.TextLabel, current.Label, view.Colors.Ink))
				}),
				ui.Rigid(func(gtx layout.Context) layout.Dimensions {
					view.Focus.Watch(gtx, chart)
					dims, events := chart.Layout(gtx, view, current)
					if len(events) > 0 {
						*selected = charts.Apply(*selected, events)
						gtx.Execute(op.InvalidateCmd{})
					}
					return dims
				}))
		})
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

// analysisTableRow shows a table row as its first detail (the date) on the
// left and the value on the right, with any further details as a caption. The
// joined sentence stays the accessible label.
func analysisTableRow(view ui.View, row charts.TextRow) layout.Widget {
	head := labeledRow(view, analysisLineTableText(row), row.Detail[0], row.Value, view.Colors.Ink)
	if len(row.Detail) < 2 {
		return head
	}
	extra := strings.Join(row.Detail[1:], " · ")
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx,
			layout.Rigid(head),
			layout.Rigid(view.Body(ui.TextCaption, extra, view.Colors.SecondaryInk)),
		)
	}
}

func analysisLineTableText(row charts.TextRow) string {
	parts := []string{row.Detail[0], row.Value}
	return strings.Join(append(parts, row.Detail[1:]...), " · ")
}
