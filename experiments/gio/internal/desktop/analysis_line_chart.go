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

func analysisLineTableText(row charts.TextRow) string {
	parts := []string{row.Detail[0], row.Value}
	return strings.Join(append(parts, row.Detail[1:]...), " · ")
}
