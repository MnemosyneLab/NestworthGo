package ui

import (
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

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

	anchorScroll *widget.List
}

// WithScrollAnchors associates popovers registered by this view with the list
// that lays out their triggers in unscrolled content coordinates.
func (v View) WithScrollAnchors(list *widget.List) View {
	v.anchorScroll = list
	return v
}

// Message is a wrapping paragraph in the body style.
func (v View) Message(text string, fg color.NRGBA) layout.Widget {
	return v.Body(TextBody, text, fg)
}

// TabButton is a selectable option such as a tab or a filter chip. The
// selected option is filled; the others are quiet outline buttons.
func (v View) TabButton(button *widget.Clickable, title string, selected bool) layout.Widget {
	variant := ButtonOutline
	if selected {
		variant = ButtonPrimary
	}
	return v.ButtonOf(button, ButtonSpec{Variant: variant, Size: ButtonSmall, Title: title, Name: title, Toggle: true, Selected: selected})
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
