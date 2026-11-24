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

type presentation struct {
	language appLanguage
	colors   *palette
	theme    *material.Theme
}

func (v presentation) label(gtx layout.Context, text string, size unit.Sp, weight font.Weight, fg color.NRGBA) layout.Dimensions {
	label := material.Label(v.theme, size, text)
	label.Color = fg
	label.Font.Weight = weight
	label.MaxLines = 1
	return label.Layout(gtx)
}

func (v presentation) panel(gtx layout.Context, border color.NRGBA, content layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: gtx.Constraints.Min}
		radius := gtx.Dp(10)
		paint.FillShape(gtx.Ops, border, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
		paint.FillShape(gtx.Ops, v.colors.surface, clip.UniformRRect(bounds.Inset(1), radius-1).Op(gtx.Ops))
		return layout.Dimensions{Size: bounds.Size()}
	}, content)
}
