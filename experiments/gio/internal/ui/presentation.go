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
	// naturalHeight receives the unstretched height of a stretched Grid cell.
	naturalHeight *int
}

// reportNaturalHeight tells a stretched Grid the height a cell needs, apart
// from the height it was stretched to. An outer card reports after its
// content, so its height wins over a nested card's.
func (v View) reportNaturalHeight(height int) {
	if v.naturalHeight != nil {
		*v.naturalHeight = height
	}
}

// WithScrollAnchors associates popovers registered by this view with the list
// that lays out their triggers in unscrolled content coordinates.
func (v View) WithScrollAnchors(list *widget.List) View {
	v.anchorScroll = list
	return v
}

func (v View) PageScrollOffset() int {
	if v.anchorScroll == nil {
		return 0
	}
	return v.anchorScroll.Position.Offset
}

func (v View) RestorePageScroll(offset int) {
	if v.anchorScroll != nil {
		v.anchorScroll.Position.Offset = offset
	}
}

// Message is a wrapping paragraph in the body style.
func (v View) Message(text string, fg color.NRGBA) layout.Widget {
	return v.Body(TextBody, text, fg)
}

// TabButton is a selectable option such as a filter chip or a toggle. The
// selected option is tinted with the accent; the others are quiet outline
// buttons. Page tabs use TabTrack instead.
func (v View) TabButton(button *widget.Clickable, title string, selected bool) layout.Widget {
	variant := ButtonOutline
	if selected {
		variant = ButtonSoft
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

// TabTrack lays count segment buttons on the muted track of a pill tab
// switcher. Each child should be a ButtonSegment button. The track wraps its
// options onto further lines when the window is too narrow for one.
func (v View) TabTrack(gtx layout.Context, count int, child func(gtx layout.Context, index int) layout.Dimensions) layout.Dimensions {
	const pad = Space2
	gtx.Constraints.Min = image.Point{}
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: gtx.Constraints.Min}
		FillRoundRect(gtx, bounds, RadiusLG, v.Colors.HoverSurface)
		return layout.Dimensions{Size: bounds.Size()}
	}, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		return layout.UniformInset(pad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return (Wrap{Gap: Space2}).Layout(gtx, count, child)
		})
	})
}

// TabSegment is one option of a TabTrack.
func (v View) TabSegment(button *widget.Clickable, title string, selected bool) layout.Widget {
	return v.TabSegmentNamed(button, title, title, selected)
}

// TabSegmentNamed is TabSegment with an accessible name that differs from its
// text, for a tab whose short label needs page context.
func (v View) TabSegmentNamed(button *widget.Clickable, title, name string, selected bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		v.Focus.Watch(gtx, button)
		return v.ButtonOf(button, ButtonSpec{Variant: ButtonSegment, Title: title, Name: name, Toggle: true, Selected: selected})(gtx)
	}
}
