package desktop

import (
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// valueRow is a name on the left with an amount and an optional caption on the
// right, the shape of a list row in the Wails cards. The amount aligns to the
// trailing edge so a column of rows reads as figures.
func valueRow(view ui.View, name, amount, caption string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return view.Line(gtx, ui.TextBody, name, view.Colors.Ink)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.Line(gtx, ui.TextLabel, amount, view.Colors.Ink)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if caption == "" {
					return layout.Dimensions{}
				}
				gtx.Constraints.Min.X = gtx.Dp(52)
				return view.LineEnd(gtx, ui.TextCaption, caption, view.Colors.SecondaryInk)
			}),
		)
	}
}
