package charts

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

const truncator = "…"

// textBlock describes a run of text placed at a chosen position. Charts draw
// labels at computed coordinates, so they place text explicitly instead of
// composing layouts.
type textBlock struct {
	style    ui.TextStyle
	content  string
	ink      color.NRGBA
	maxWidth int
	// maxLines of zero wraps without limit. One line truncates with an
	// ellipsis, which the callers use only for decorations such as axis
	// ticks; item labels always wrap in full.
	maxLines int
	align    text.Alignment
}

func (b textBlock) label(view ui.View) material.LabelStyle {
	label := material.Label(view.Theme, b.style.Size, b.content)
	label.Color = b.ink
	label.Font.Weight = b.style.Weight
	label.LineHeightScale = b.style.LineHeight
	label.MaxLines = b.maxLines
	label.Alignment = b.align
	label.Truncator = truncator
	return label
}

// measure returns the size the block would take without drawing it.
func (b textBlock) measure(gtx layout.Context, view ui.View) image.Point {
	if b.content == "" {
		return image.Point{}
	}
	macro := op.Record(gtx.Ops)
	size := b.layout(gtx, view).Size
	macro.Stop()
	return size
}

func (b textBlock) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	gtx.Constraints.Max.X = max(1, b.maxWidth)
	if b.maxLines != 1 {
		gtx.Constraints.Max.Y = 1 << 20
	}
	return b.label(view).Layout(gtx)
}

// draw paints the block with its top-left corner at at and returns its size.
// Alignment is relative to the block's own width, so a right-aligned block must
// pass the width it should align within as maxWidth and fill that width.
func (b textBlock) draw(gtx layout.Context, view ui.View, at image.Point) image.Point {
	if b.content == "" {
		return image.Point{}
	}
	defer op.Offset(at).Push(gtx.Ops).Pop()
	gtx.Constraints.Min = image.Point{}
	if b.align != text.Start {
		gtx.Constraints.Min.X = b.maxWidth
	}
	gtx.Constraints.Max.X = max(1, b.maxWidth)
	if b.maxLines != 1 {
		gtx.Constraints.Max.Y = 1 << 20
	}
	return b.label(view).Layout(gtx).Size
}

// drawCentered paints a single line centered on a point.
func drawCentered(gtx layout.Context, view ui.View, block textBlock, center image.Point) {
	size := block.measure(gtx, view)
	block.draw(gtx, view, image.Pt(center.X-size.X/2, center.Y-size.Y/2))
}

// natural is the width the block needs on one line, without truncation.
func (b textBlock) natural(gtx layout.Context, view ui.View) int {
	b.maxWidth, b.maxLines = 1<<16, 1
	return b.measure(gtx, view).X
}
