package desktop

import (
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func overview(gtx layout.Context, view ui.View) layout.Dimensions {
	gtx = ui.LimitWidth(gtx, 560)
	colors := view.Colors
	return view.Panel(gtx, colors.Divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(20).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Label(gtx, view.Language.Text(i18n.SampleData), 12, font.Medium, colors.SecondaryInk)
				}),
				layout.Rigid(layout.Spacer{Height: 12}.Layout),
				layout.Rigid(amount(view, i18n.NetWorth, "126800", 32)),
				layout.Rigid(layout.Spacer{Height: 17}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Constraints.Max.X, 1)
					paint.FillShape(gtx.Ops, colors.Divider, clip.Rect{Max: size}.Op())
					return layout.Dimensions{Size: size}
				}),
				layout.Rigid(layout.Spacer{Height: 15}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Flexed(1, amount(view, i18n.Assets, "138500", 20)),
						layout.Flexed(1, amount(view, i18n.Liabilities, "-11700", 20)),
					)
				}),
			)
		})
	})
}

func amount(view ui.View, label i18n.MessageID, value string, size unit.Sp) layout.Widget {
	display := format.Money(value, "CNY", settings.Settings{DecimalSeparator: view.DecimalSeparator, GroupingSeparator: view.GroupingSeparator})
	return func(gtx layout.Context) layout.Dimensions {
		colors := view.Colors
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, view.Language.Text(label), 13, font.Normal, colors.SecondaryInk)
			}),
			layout.Rigid(layout.Spacer{Height: 4}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, display, size, font.Medium, colors.Ink)
			}),
		)
	}
}
