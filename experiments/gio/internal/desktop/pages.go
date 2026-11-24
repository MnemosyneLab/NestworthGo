package desktop

import (
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

func overview(gtx layout.Context, view presentation) layout.Dimensions {
	gtx = limitWidth(gtx, 560)
	colors := view.colors
	return view.panel(gtx, colors.divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(20).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.label(gtx, view.language.Text(i18n.SampleData), 12, font.Medium, colors.secondaryInk)
				}),
				layout.Rigid(layout.Spacer{Height: 12}.Layout),
				layout.Rigid(amount(view, i18n.NetWorth, "¥126,800.00", 32)),
				layout.Rigid(layout.Spacer{Height: 17}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Constraints.Max.X, 1)
					paint.FillShape(gtx.Ops, colors.divider, clip.Rect{Max: size}.Op())
					return layout.Dimensions{Size: size}
				}),
				layout.Rigid(layout.Spacer{Height: 15}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Flexed(1, amount(view, i18n.Assets, "¥138,500.00", 20)),
						layout.Flexed(1, amount(view, i18n.Liabilities, "−¥11,700.00", 20)),
					)
				}),
			)
		})
	})
}

func amount(view presentation, label i18n.MessageID, value string, size unit.Sp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		colors := view.colors
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.label(gtx, view.language.Text(label), 13, font.Normal, colors.secondaryInk)
			}),
			layout.Rigid(layout.Spacer{Height: 4}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.label(gtx, value, size, font.Medium, colors.ink)
			}),
		)
	}
}

type inputMethodPage struct {
	editor textField
}

func (p *inputMethodPage) layout(gtx layout.Context, view presentation) layout.Dimensions {
	return p.editor.layout(limitWidth(gtx, 560), view, fieldOptions{
		label: i18n.InputMethodTest, hint: i18n.InputMethodHint,
	})
}
