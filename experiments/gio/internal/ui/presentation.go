package ui

import (
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
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
	// Motion holds hover, press and focus animation state. It may be nil.
	Motion *Motion
	Origin image.Point
	Focus  *PageFocus
}

// Message is a wrapping paragraph in the body style.
func (v View) Message(text string, fg color.NRGBA) layout.Widget {
	return v.Body(TextBody, text, fg)
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
