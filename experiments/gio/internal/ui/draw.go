package ui

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// Fade multiplies the alpha of c by opacity, for disabled content and
// cross-fades.
func Fade(c color.NRGBA, opacity float64) color.NRGBA {
	c.A = uint8(math.Round(float64(c.A) * math.Max(0, math.Min(1, opacity))))
	return c
}

// FillRoundRect paints a filled rounded rectangle.
func FillRoundRect(gtx layout.Context, bounds image.Rectangle, radius unit.Dp, fill color.NRGBA) {
	if fill.A == 0 || bounds.Empty() {
		return
	}
	paint.FillShape(gtx.Ops, fill, clip.UniformRRect(bounds, roundRadius(gtx, bounds, radius)).Op(gtx.Ops))
}

// StrokeRoundRect paints a border that lies fully inside bounds. A stroke is
// centered on its path, so the path is inset by half the width.
func StrokeRoundRect(gtx layout.Context, bounds image.Rectangle, radius, width unit.Dp, stroke color.NRGBA) {
	w := max(1, gtx.Dp(width))
	if stroke.A == 0 || bounds.Dx() <= w || bounds.Dy() <= w {
		return
	}
	inset := float32(w) / 2
	r := float32(roundRadius(gtx, bounds, radius)) - inset
	if r < 0 {
		r = 0
	}
	var p clip.Path
	p.Begin(gtx.Ops)
	roundedRectPath(&p,
		f32.Pt(float32(bounds.Min.X)+inset, float32(bounds.Min.Y)+inset),
		f32.Pt(float32(bounds.Max.X)-inset, float32(bounds.Max.Y)-inset), r)
	paint.FillShape(gtx.Ops, stroke, clip.Stroke{Path: p.End(), Width: float32(w)}.Op())
}

// roundedRectPath appends a closed rounded rectangle with the given radius.
func roundedRectPath(p *clip.Path, min, max f32.Point, r float32) {
	const k = 0.5522847498 // quarter-circle Bézier constant
	r = minf(r, (max.X-min.X)/2, (max.Y-min.Y)/2)
	c := r * k
	p.MoveTo(f32.Pt(min.X+r, min.Y))
	p.LineTo(f32.Pt(max.X-r, min.Y))
	p.CubeTo(f32.Pt(max.X-r+c, min.Y), f32.Pt(max.X, min.Y+r-c), f32.Pt(max.X, min.Y+r))
	p.LineTo(f32.Pt(max.X, max.Y-r))
	p.CubeTo(f32.Pt(max.X, max.Y-r+c), f32.Pt(max.X-r+c, max.Y), f32.Pt(max.X-r, max.Y))
	p.LineTo(f32.Pt(min.X+r, max.Y))
	p.CubeTo(f32.Pt(min.X+r-c, max.Y), f32.Pt(min.X, max.Y-r+c), f32.Pt(min.X, max.Y-r))
	p.LineTo(f32.Pt(min.X, min.Y+r))
	p.CubeTo(f32.Pt(min.X, min.Y+r-c), f32.Pt(min.X+r-c, min.Y), f32.Pt(min.X+r, min.Y))
	p.Close()
}

func minf(values ...float32) float32 {
	least := values[0]
	for _, v := range values[1:] {
		least = min(least, v)
	}
	return least
}

// roundRadius converts a radius in dp to pixels, never exceeding half the
// shorter side, so RadiusFull draws a pill.
func roundRadius(gtx layout.Context, bounds image.Rectangle, radius unit.Dp) int {
	return min(gtx.Dp(radius), bounds.Dx()/2, bounds.Dy()/2)
}

// FocusRing draws the keyboard focus indicator: a 2dp ring in the accent at
// half strength, separated from the control by a 2dp gap. It matches
// focus-visible:ring-2 ring-primary/50 ring-offset-2 in the Wails controls.
// The ring lies outside bounds, so a parent that clips to the control hides it.
func (p *Palette) FocusRing(gtx layout.Context, bounds image.Rectangle, radius unit.Dp) {
	p.FocusRingAt(gtx, bounds, radius, 1)
}

// FocusRingAt draws the focus ring at a fraction of its full strength, for a
// fade-in while a tween runs.
func (p *Palette) FocusRingAt(gtx layout.Context, bounds image.Rectangle, radius unit.Dp, amount float64) {
	const gap, width = 2, 2
	if amount <= 0 {
		return
	}
	grow := gtx.Dp(gap + width)
	outer := bounds.Inset(-grow)
	StrokeRoundRect(gtx, outer, radius+gap+width, width, WithAlpha(p.Accent, 0.5*amount))
}

// FocusBorder draws the inset accent border used by text-like inputs, where
// Wails pairs border-primary/50 with a ring-2 ring-primary/50. The ring sits
// flush against the border instead of floating away from it.
func (p *Palette) FocusBorder(gtx layout.Context, bounds image.Rectangle, radius unit.Dp) {
	width := gtx.Dp(2)
	StrokeRoundRect(gtx, bounds.Inset(-width), radius+2, 2, WithAlpha(p.Accent, 0.5))
}

// DrawShadow paints a CSS-style box shadow for bounds. Gio has no blur, so each
// layer is approximated by stacked rounded rectangles whose alpha follows a
// Gaussian edge profile (CSS blur radius B has σ = B/2). Draw it before the
// surface it belongs to; it is not clipped, so the surface must be opaque or
// the caller must accept the shadow showing through.
func (p *Palette) DrawShadow(gtx layout.Context, bounds image.Rectangle, radius unit.Dp, level Elevation) {
	for _, layer := range p.ShadowLayers(level) {
		drawShadowLayer(gtx, bounds, radius, layer, p.ShadowTint)
	}
}

func drawShadowLayer(gtx layout.Context, bounds image.Rectangle, radius unit.Dp, layer ShadowLayer, tint color.NRGBA) {
	spread := gtx.Dp(layer.Spread)
	if layer.Spread < 0 {
		spread = -gtx.Dp(-layer.Spread)
	}
	base := bounds.Add(image.Pt(0, gtx.Dp(layer.OffsetY))).Inset(-spread)
	baseRadius := gtx.Dp(radius) + spread
	sigma := float64(gtx.Dp(layer.Blur)) / 2
	if sigma < 0.5 {
		// A hairline shadow has no meaningful falloff.
		fillShadow(gtx, base, baseRadius, WithAlpha(tint, layer.Alpha))
		return
	}
	steps := min(24, max(3, int(math.Ceil(sigma)))) // wider blurs use up to 24 layers
	for _, band := range shadowBands(sigma, steps, layer.Alpha) {
		rect := base.Inset(-int(math.Round(band.grow)))
		fillShadow(gtx, rect, baseRadius+int(math.Round(band.grow)), WithAlpha(tint, band.alpha))
	}
}

// shadowBand is one stacked rectangle: grown by grow pixels (negative values
// shrink it) and painted with alpha over the bands before it.
type shadowBand struct{ grow, alpha float64 }

// shadowBands splits a Gaussian edge of standard deviation sigma and peak
// opacity into steps rectangles. The rectangles span two sigma each way from
// the shadow edge. Composited in order, the covered alpha at the middle of
// every step equals peak times the Gaussian tail at that distance.
func shadowBands(sigma float64, steps int, peak float64) []shadowBand {
	extent := 2 * sigma
	width := 2 * extent / float64(steps)
	bands := make([]shadowBand, 0, steps)
	previous := 0.0
	for k := range steps {
		grow := extent - float64(k)*width
		cumulative := peak * gaussianTail((grow-width/2)/sigma)
		alpha := 1 - (1-cumulative)/(1-previous)
		previous = cumulative
		if alpha > 0 {
			bands = append(bands, shadowBand{grow: grow, alpha: alpha})
		}
	}
	return bands
}

func fillShadow(gtx layout.Context, rect image.Rectangle, radius int, fill color.NRGBA) {
	if fill.A == 0 || rect.Empty() {
		return
	}
	radius = max(0, min(radius, rect.Dx()/2, rect.Dy()/2))
	paint.FillShape(gtx.Ops, fill, clip.UniformRRect(rect, radius).Op(gtx.Ops))
}

// gaussianTail is the probability that a standard normal exceeds z.
func gaussianTail(z float64) float64 { return 0.5 * math.Erfc(z/math.Sqrt2) }

// PressTransform scales the content of a pressed control around its center.
// Pop the returned stack when the content is drawn. A scale of 1 is a no-op.
func PressTransform(gtx layout.Context, size image.Point, scale float64) op.TransformStack {
	if scale == 1 {
		return op.Affine(f32.Affine2D{}).Push(gtx.Ops)
	}
	center := f32.Pt(float32(size.X)/2, float32(size.Y)/2)
	return op.Affine(f32.Affine2D{}.Scale(center, f32.Pt(float32(scale), float32(scale)))).Push(gtx.Ops)
}
