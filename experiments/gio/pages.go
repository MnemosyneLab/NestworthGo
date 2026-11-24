package main

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (w *workspace) overview(gtx layout.Context) layout.Dimensions {
	colors := w.colors()
	return w.panel(gtx, colors.divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(20).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return w.label(gtx, w.language.text(sampleData), 12, font.Medium, colors.secondaryInk)
				}),
				layout.Rigid(layout.Spacer{Height: 12}.Layout),
				layout.Rigid(w.amount(netWorth, "¥126,800.00", 32)),
				layout.Rigid(layout.Spacer{Height: 17}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Constraints.Max.X, 1)
					paint.FillShape(gtx.Ops, colors.divider, clip.Rect{Max: size}.Op())
					return layout.Dimensions{Size: size}
				}),
				layout.Rigid(layout.Spacer{Height: 15}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Flexed(1, w.amount(assets, "¥138,500.00", 20)),
						layout.Flexed(1, w.amount(liabilities, "−¥11,700.00", 20)),
					)
				}),
			)
		})
	})
}

func (w *workspace) amount(label messageID, value string, size unit.Sp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		colors := w.colors()
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return w.label(gtx, w.language.text(label), 13, font.Normal, colors.secondaryInk)
			}),
			layout.Rigid(layout.Spacer{Height: 4}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return w.label(gtx, value, size, font.Medium, colors.ink)
			}),
		)
	}
}

func (w *workspace) directory(gtx layout.Context) layout.Dimensions {
	colors := w.colors()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return w.label(gtx, w.language.text(inputMethodTest), 15, font.Medium, colors.ink)
		}),
		layout.Rigid(layout.Spacer{Height: 10}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			border := colors.divider
			if gtx.Focused(&w.directoryEditor) {
				border = colors.accent
			}
			editor := material.Editor(w.theme, &w.directoryEditor, w.language.text(inputMethodHint))
			editor.TextSize, editor.HintColor = 15, colors.secondaryInk
			return w.panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
				height := min(gtx.Constraints.Max.Y, gtx.Dp(120))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
				return layout.Inset{Top: 12, Bottom: 12, Left: 14, Right: 14}.Layout(gtx, editor.Layout)
			})
		}),
	)
}

func (w *workspace) panel(gtx layout.Context, border color.NRGBA, content layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: gtx.Constraints.Min}
		radius := gtx.Dp(10)
		paint.FillShape(gtx.Ops, border, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
		paint.FillShape(gtx.Ops, w.colors().surface, clip.UniformRRect(bounds.Inset(1), radius-1).Op(gtx.Ops))
		return layout.Dimensions{Size: bounds.Size()}
	}, content)
}
