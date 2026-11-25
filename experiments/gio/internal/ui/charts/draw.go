package charts

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// markerShape distinguishes series without color: the legend and the plot use
// the same shape for the same series.
type markerShape uint8

const (
	markerCircle markerShape = iota
	markerDiamond
	markerSquare
	markerTriangle
	markerShapes
)

// strokePolyline strokes a polyline with a round-ended path of the given width.
func strokePolyline(gtx layout.Context, points []f32.Point, width float32, ink color.NRGBA) {
	if len(points) < 2 || ink.A == 0 {
		return
	}
	var path clip.Path
	path.Begin(gtx.Ops)
	path.MoveTo(points[0])
	for _, p := range points[1:] {
		path.LineTo(p)
	}
	paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: path.End(), Width: width}.Op())
}

// fillPolygon fills the closed polygon through points.
func fillPolygon(gtx layout.Context, points []f32.Point, fill color.NRGBA) {
	if len(points) < 3 || fill.A == 0 {
		return
	}
	var path clip.Path
	path.Begin(gtx.Ops)
	path.MoveTo(points[0])
	for _, p := range points[1:] {
		path.LineTo(p)
	}
	path.Close()
	paint.FillShape(gtx.Ops, fill, clip.Outline{Path: path.End()}.Op())
}

func fillRect(gtx layout.Context, r image.Rectangle, fill color.NRGBA) {
	if r.Empty() || fill.A == 0 {
		return
	}
	paint.FillShape(gtx.Ops, fill, clip.Rect(r).Op())
}

// hairline draws a one-pixel-wide line between two axis-aligned points.
func hairline(gtx layout.Context, from, to image.Point, width int, ink color.NRGBA) {
	r := image.Rectangle{Min: from, Max: to}.Canon()
	if r.Dx() == 0 {
		r.Max.X = r.Min.X + width
	}
	if r.Dy() == 0 {
		r.Max.Y = r.Min.Y + width
	}
	fillRect(gtx, r, ink)
}

// drawMarker fills a marker of the given shape. radius is the half-width.
func drawMarker(gtx layout.Context, shape markerShape, center f32.Point, radius float32, fill color.NRGBA) {
	switch shape {
	case markerDiamond:
		fillPolygon(gtx, []f32.Point{
			{X: center.X, Y: center.Y - radius*1.2}, {X: center.X + radius*1.2, Y: center.Y},
			{X: center.X, Y: center.Y + radius*1.2}, {X: center.X - radius*1.2, Y: center.Y},
		}, fill)
	case markerSquare:
		half := radius * 0.9
		fillPolygon(gtx, []f32.Point{
			{X: center.X - half, Y: center.Y - half}, {X: center.X + half, Y: center.Y - half},
			{X: center.X + half, Y: center.Y + half}, {X: center.X - half, Y: center.Y + half},
		}, fill)
	case markerTriangle:
		fillPolygon(gtx, []f32.Point{
			{X: center.X, Y: center.Y - radius*1.2}, {X: center.X + radius*1.15, Y: center.Y + radius*0.9},
			{X: center.X - radius*1.15, Y: center.Y + radius*0.9},
		}, fill)
	default:
		bounds := image.Rect(int(math.Round(float64(center.X-radius))), int(math.Round(float64(center.Y-radius))),
			int(math.Round(float64(center.X+radius))), int(math.Round(float64(center.Y+radius))))
		paint.FillShape(gtx.Ops, fill, clip.Ellipse(bounds).Op(gtx.Ops))
	}
}

// drawRingedMarker draws a marker with an outline in ink, which marks the
// selected point independently of its color.
func drawRingedMarker(gtx layout.Context, shape markerShape, center f32.Point, radius, ring float32, fill, ink color.NRGBA) {
	drawMarker(gtx, shape, center, radius+ring, ink)
	drawMarker(gtx, shape, center, radius, fill)
}

// arcPolyline samples an arc of a circle every few degrees.
func arcPolyline(center f32.Point, r float64, from, to float64) []f32.Point {
	steps := max(2, int(math.Ceil(math.Abs(to-from)/(math.Pi/72))))
	points := make([]f32.Point, 0, steps+1)
	for i := 0; i <= steps; i++ {
		points = append(points, ringPoint(center, r, from+(to-from)*float64(i)/float64(steps)))
	}
	return points
}

// semanticItem labels one chart item for assistive technology. The node is a
// clip area, so call it with the item's bounds in chart coordinates.
func semanticItem(gtx layout.Context, bounds image.Rectangle, label string) {
	if bounds.Empty() || label == "" {
		return
	}
	defer clip.Rect(bounds).Push(gtx.Ops).Pop()
	semantic.LabelOp(label).Add(gtx.Ops)
}

// readout is the persistent text form of one item. It is what a person who
// cannot hover reads, and it repeats the tooltip.
type readout struct {
	label, value string
	detail       []string
	selected     bool
	missing      bool
}

// layoutReadout lays out the readout under a chart. Its height never drops
// below two lines, so moving the cursor does not make the chart jump.
func layoutReadout(gtx layout.Context, view ui.View, m Messages, hint string, r *readout) layout.Dimensions {
	width := gtx.Constraints.Max.X
	colors := view.Colors
	body, caption := ui.TextBody, ui.TextCaption
	minHeight := int(float32(gtx.Dp(1)) * (float32(body.Size)*body.LineHeight + float32(caption.Size)*caption.LineHeight))
	height := 0
	y := 0
	draw := func(block textBlock) {
		size := block.draw(gtx, view, image.Pt(0, y))
		y += size.Y
		height = y
	}
	if r == nil {
		draw(textBlock{style: caption, content: hint, ink: colors.SecondaryInk, maxWidth: width})
		return layout.Dimensions{Size: image.Pt(width, max(height, minHeight))}
	}
	head := r.label
	if r.value != "" {
		head += " · " + r.value
	}
	if r.selected && m.Selected != "" {
		head = m.Selected + " · " + head
	}
	ink := colors.Ink
	if r.missing {
		ink = colors.SecondaryInk
	}
	style := ui.TextLabel
	draw(textBlock{style: style, content: head, ink: ink, maxWidth: width})
	for _, line := range r.detail {
		draw(textBlock{style: caption, content: line, ink: colors.SecondaryInk, maxWidth: width})
	}
	return layout.Dimensions{Size: image.Pt(width, max(height, minHeight))}
}

// tooltipPosition places a tooltip of the given size beside anchor, above and
// to the right by default, and keeps it inside bounds.
func tooltipPosition(bounds image.Rectangle, anchor image.Point, size image.Point, gap int) image.Point {
	pos := image.Pt(anchor.X+gap, anchor.Y-size.Y-gap)
	if pos.X+size.X > bounds.Max.X {
		pos.X = anchor.X - gap - size.X
	}
	if pos.Y < bounds.Min.Y {
		pos.Y = anchor.Y + gap
	}
	pos.X = max(bounds.Min.X, min(pos.X, bounds.Max.X-size.X))
	pos.Y = max(bounds.Min.Y, min(pos.Y, bounds.Max.Y-size.Y))
	return pos
}

// drawTooltip floats the item's text beside its anchor. The tooltip repeats
// the readout and receives no input of its own.
func drawTooltip(gtx layout.Context, view ui.View, bounds image.Rectangle, anchor image.Point, r readout) {
	maxWidth := min(gtx.Dp(260), bounds.Dx())
	if maxWidth <= 0 {
		return
	}
	colors := view.Colors
	macro := op.Record(gtx.Ops)
	tgtx := gtx
	tgtx.Constraints = layout.Constraints{Max: image.Pt(maxWidth, 1<<20)}
	dims := view.Floating(tgtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(ui.Space8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			inner := gtx.Constraints.Max.X
			y := 0
			widest := 0
			line := func(block textBlock) {
				block.maxWidth = inner
				size := block.draw(gtx, view, image.Pt(0, y))
				y += size.Y
				widest = max(widest, size.X)
			}
			line(textBlock{style: ui.TextCaptionStrong, content: r.label, ink: colors.Ink})
			if r.value != "" {
				ink := colors.Ink
				if r.missing {
					ink = colors.SecondaryInk
				}
				line(textBlock{style: ui.TextLabel, content: r.value, ink: ink})
			}
			for _, d := range r.detail {
				line(textBlock{style: ui.TextCaption, content: d, ink: colors.SecondaryInk})
			}
			return layout.Dimensions{Size: image.Pt(widest, y)}
		})
	})
	call := macro.Stop()
	pos := tooltipPosition(bounds, anchor, dims.Size, gtx.Dp(ui.Space8))
	overlay := op.Record(gtx.Ops)
	stack := op.Offset(pos).Push(gtx.Ops)
	call.Add(gtx.Ops)
	stack.Pop()
	op.Defer(gtx.Ops, overlay.Stop())
}

// drawNotice paints a quiet centered message in a frame of the given size. It
// stands in for a chart that has nothing to draw, so an empty chart still
// says why.
func drawNotice(gtx layout.Context, view ui.View, size image.Point, message string) layout.Dimensions {
	colors := view.Colors
	bounds := image.Rectangle{Max: size}
	ui.FillRoundRect(gtx, bounds, ui.RadiusLG, colors.Tint(colors.Ink, 0.03))
	ui.StrokeRoundRect(gtx, bounds, ui.RadiusLG, 1, colors.Divider)
	pad := gtx.Dp(ui.Space16)
	block := textBlock{style: ui.TextBody, content: message, ink: colors.SecondaryInk, maxWidth: max(1, size.X-2*pad), align: text.Middle}
	measured := block.measure(gtx, view)
	block.draw(gtx, view, image.Pt(pad, max(0, (size.Y-measured.Y)/2)))
	return layout.Dimensions{Size: size}
}
