package main

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type presentation struct {
	language appLanguage
	colors   *palette
	theme    *material.Theme
	overlay  *popoverHost
	origin   image.Point
}

func (v presentation) message(text string, fg color.NRGBA) layout.Widget {
	style := material.Label(v.theme, 13, text)
	style.Color = fg
	return style.Layout
}

func (v presentation) button(button *widget.Clickable, title string) layout.Widget {
	style := material.Button(v.theme, button, title)
	style.Background = v.colors.surface
	style.Color = v.colors.ink
	style.TextSize = 12
	style.CornerRadius = 7
	style.Inset = layout.Inset{Top: 7, Bottom: 7, Left: 8, Right: 8}
	return style.Layout
}

func limitWidth(gtx layout.Context, width unit.Dp) layout.Context {
	size := min(gtx.Dp(width), gtx.Constraints.Max.X)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = size, size
	return gtx
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
