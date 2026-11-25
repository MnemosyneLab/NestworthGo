package ui

import (
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

type View struct {
	DateFormat, DecimalSeparator, GroupingSeparator string
	TimeFormat, WeekStart                           string
	TimeLocation                                    *time.Location
	Language                                        i18n.Language
	Colors                                          *Palette
	Theme                                           *material.Theme
	Overlay                                         *Overlay
	Origin                                          image.Point
	Focus                                           *PageFocus
}

// Message is a wrapping paragraph in the body style.
func (v View) Message(text string, fg color.NRGBA) layout.Widget {
	return v.Body(TextBody, text, fg)
}

func (v View) Button(button *widget.Clickable, title string) layout.Widget {
	return v.NamedButton(button, title, title)
}

func (v View) NamedButton(button *widget.Clickable, title, name string) layout.Widget {
	style := material.Button(v.Theme, button, title)
	style.Background = v.Colors.Surface
	style.Color = v.Colors.Ink
	style.TextSize = 12
	style.CornerRadius = 7
	style.Inset = layout.Inset{Top: 7, Bottom: 7, Left: 8, Right: 8}
	return func(gtx layout.Context) layout.Dimensions {
		v.Focus.Watch(gtx, button)
		return accessibility.Button(gtx, button, name, "", false, style.Layout)
	}
}

func (v View) TabButton(button *widget.Clickable, title string, selected bool) layout.Widget {
	style := material.ButtonLayout(v.Theme, button)
	style.Background = v.Colors.Surface
	style.CornerRadius = 7
	ink := v.Colors.Ink
	if selected {
		style.Background, ink = v.Colors.Accent, v.Colors.OnAccent
	}
	return func(gtx layout.Context) layout.Dimensions {
		v.Focus.Watch(gtx, button)
		return accessibility.Button(gtx, button, title, "", false, func(gtx layout.Context) layout.Dimensions {
			return style.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.SelectedOp(selected).Add(gtx.Ops)
				semantic.DescriptionOp(title).Add(gtx.Ops)
				return layout.Inset{Top: 7, Bottom: 7, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return v.Label(gtx, title, 12, font.Normal, ink)
				})
			})
		})
	}
}

func (v View) PrimaryButton(button *widget.Clickable, title string) layout.Widget {
	style := material.Button(v.Theme, button, title)
	style.TextSize = 13
	style.CornerRadius = 8
	return func(gtx layout.Context) layout.Dimensions {
		v.Focus.Watch(gtx, button)
		return accessibility.Button(gtx, button, title, "", false, style.Layout)
	}
}

func LimitWidth(gtx layout.Context, width unit.Dp) layout.Context {
	size := min(gtx.Dp(width), gtx.Constraints.Max.X)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = size, size
	return gtx
}

func (v View) Label(gtx layout.Context, text string, size unit.Sp, weight font.Weight, fg color.NRGBA) layout.Dimensions {
	label := material.Label(v.Theme, size, text)
	label.Color = fg
	label.Font.Weight = weight
	label.MaxLines = 1
	return label.Layout(gtx)
}

func (v View) Panel(gtx layout.Context, border color.NRGBA, content layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: gtx.Constraints.Min}
		radius := gtx.Dp(10)
		paint.FillShape(gtx.Ops, border, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
		paint.FillShape(gtx.Ops, v.Colors.Surface, clip.UniformRRect(bounds.Inset(1), radius-1).Op(gtx.Ops))
		return layout.Dimensions{Size: bounds.Size()}
	}, content)
}
