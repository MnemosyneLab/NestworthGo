package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// BadgeTone selects the color of a Badge, as the Badge variants do in
// frontend/src/components/ui/badge.tsx.
type BadgeTone uint8

const (
	// BadgeAccent is the default accent tint.
	BadgeAccent BadgeTone = iota
	// BadgeNeutral is the quiet muted pill for counts and kinds.
	BadgeNeutral
	BadgeSuccess
	BadgeWarning
	BadgeDestructive
	// BadgeOutline has a hairline and no fill.
	BadgeOutline
)

// Badge draws a small status pill. The text is the message; the tone only
// reinforces it, so a badge never relies on color alone.
func (v View) Badge(gtx layout.Context, tone BadgeTone, text string) layout.Dimensions {
	p := v.Colors
	var fill, ink, border color.NRGBA
	switch tone {
	case BadgeNeutral:
		fill, ink = p.HoverSurface, p.SecondaryInk
	case BadgeSuccess:
		fill, ink = Mix(p.Surface, p.Success, 0.14), p.Success
	case BadgeWarning:
		fill, ink = Mix(p.Surface, p.Warning, 0.18), p.WarningInk
	case BadgeDestructive:
		fill, ink = Mix(p.Surface, p.Destructive, 0.12), p.Destructive
	case BadgeOutline:
		ink, border = p.Ink, p.Divider
	default:
		fill, ink = Mix(p.Surface, p.Accent, 0.10), p.Accent
	}
	gtx.Constraints.Min = image.Point{}
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: gtx.Constraints.Min}
		if fill != (color.NRGBA{}) {
			FillRoundRect(gtx, bounds, RadiusFull, fill)
		}
		if border != (color.NRGBA{}) {
			StrokeRoundRect(gtx, bounds, RadiusFull, 1, border)
		}
		return layout.Dimensions{Size: bounds.Size()}
	}, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: Space8, Right: Space8, Top: 2, Bottom: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return v.Line(gtx, TextCaptionStrong, text, ink)
		})
	})
}

// Hairline is a one-pixel rule across the available width.
func (v View) Hairline(gtx layout.Context) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, max(1, gtx.Dp(1)))
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, v.Colors.Divider)
	return layout.Dimensions{Size: size}
}

// CardPadded is a Card with the standard 20dp content padding.
func (v View) CardPadded(gtx layout.Context, content layout.Widget) layout.Dimensions {
	return v.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		// The card honors a minimum height, which a stretched Grid row sets, by
		// growing its padded content rather than adding padding to the minimum.
		padding := gtx.Dp(Space20)
		minimum := max(gtx.Constraints.Min.Y-2*padding, 0)
		return layout.UniformInset(Space20).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			dims := content(gtx)
			v.reportNaturalHeight(dims.Size.Y + 2*padding)
			dims.Size.Y = max(dims.Size.Y, minimum)
			return dims
		})
	})
}

// IconTile is a rounded square that holds a glyph, the leading mark of a
// list row or card in Wails. Fill is the tile color and Ink the glyph color.
func (v View) IconTile(gtx layout.Context, glyph Glyph, size unit.Dp, fill, ink color.NRGBA) layout.Dimensions {
	side := gtx.Dp(size)
	bounds := image.Rectangle{Max: image.Pt(side, side)}
	FillRoundRect(gtx, bounds, RadiusMD, fill)
	inset := (side - gtx.Dp(16)) / 2
	defer op.Offset(image.Pt(inset, inset)).Push(gtx.Ops).Pop()
	LayoutGlyph(gtx, glyph, 16, ink)
	return layout.Dimensions{Size: bounds.Size()}
}
