package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// unbounded is the height at or above which a constraint means "no limit".
// Gio's layout.List passes 1e6 along its scroll axis and an inset only takes a
// few pixels off it, so the threshold sits far below that and far above any
// real window.
const unbounded = 100_000

// ColumnChild is one row of a Column: either natural height or the row that
// takes the remaining height.
type ColumnChild struct {
	widget layout.Widget
	fill   bool
}

// Rigid is a row at its natural height.
func Rigid(widget layout.Widget) ColumnChild { return ColumnChild{widget: widget} }

// Fill is the row that takes the height left over by the others, such as a
// scrolling list. A Column has at most one.
func Fill(widget layout.Widget) ColumnChild { return ColumnChild{widget: widget, fill: true} }

// Column stacks rows vertically like a vertical layout.Flex, with two
// differences that make a page adapt to its window. It reports its true height
// instead of clamping to the window, so a stage can scroll a short window; and
// the Fill row never shrinks below MinFill, so a list always keeps a few rows.
//
// The height a Fill row can use is the window's height, taken from the
// constraints: Max.Y when it is bounded, otherwise Min.Y, which is how the
// shell stage passes the viewport to a page.
type Column struct {
	Gap unit.Dp
	// MinFill is the least height the Fill row gets; zero means DefaultMinFill.
	MinFill unit.Dp
}

// DefaultMinFill keeps about three list rows visible.
const DefaultMinFill unit.Dp = 220

// Layout lays the rows out. Rigid rows see no height limit.
func (c Column) Layout(gtx layout.Context, children ...ColumnChild) layout.Dimensions {
	height := gtx.Constraints.Max.Y
	if height >= unbounded {
		height = gtx.Constraints.Min.Y
	}
	gap := gtx.Dp(c.Gap)
	minFill := gtx.Dp(c.MinFill)
	if c.MinFill == 0 {
		minFill = gtx.Dp(DefaultMinFill)
	}

	type placed struct {
		call op.CallOp
		size image.Point
	}
	rows := make([]placed, len(children))
	used, fill := 0, -1
	widest := 0
	child := gtx
	child.Constraints.Min.Y, child.Constraints.Max.Y = 0, unbounded
	for i, row := range children {
		if row.fill {
			fill = i
			continue
		}
		macro := op.Record(gtx.Ops)
		dims := row.widget(child)
		rows[i] = placed{macro.Stop(), dims.Size}
		used += dims.Size.Y
		widest = max(widest, dims.Size.X)
	}
	total := used
	if len(children) > 1 {
		total += gap * (len(children) - 1)
	}
	if fill >= 0 {
		filled := child
		filled.Constraints.Max.Y = max(height-total, minFill)
		filled.Constraints.Min.Y = filled.Constraints.Max.Y
		macro := op.Record(gtx.Ops)
		dims := children[fill].widget(filled)
		rows[fill] = placed{macro.Stop(), dims.Size}
		total += dims.Size.Y
		widest = max(widest, dims.Size.X)
	}

	y := 0
	for _, row := range rows {
		stack := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		row.call.Add(gtx.Ops)
		stack.Pop()
		y += row.size.Y + gap
	}
	// Report the real height even when it exceeds a bounded parent.
	size := gtx.Constraints.Constrain(image.Pt(widest, 0))
	size.Y = max(total, gtx.Constraints.Min.Y)
	return layout.Dimensions{Size: size}
}

// FitWindow restores the window as the height limit for a layout that still
// splits its height with layout.Flex and layout.Flexed. Such a layout must not
// receive the unbounded height of the shell stage, or its Flexed row would take
// all of it. New code uses Column and Fill instead; this exists so a screen can
// be migrated on its own.
func FitWindow(gtx layout.Context) layout.Context {
	if gtx.Constraints.Max.Y >= unbounded {
		gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
	}
	return gtx
}
