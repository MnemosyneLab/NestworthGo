package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// Window width breakpoints in dp. They mirror the Tailwind sm, lg and xl steps
// that the Wails frontend uses, so a window of a given width picks the same
// density in both applications. Decisions about the window (sidebar rail,
// page chrome) use these. Decisions about a container (how many columns a card
// holds) use Grid instead, because a card's width depends on the sidebar.
const (
	BreakpointCompact unit.Dp = 640
	BreakpointMedium  unit.Dp = 960
	BreakpointWide    unit.Dp = 1280
)

// WidthAtLeast reports whether a width in pixels reaches the breakpoint.
func WidthAtLeast(gtx layout.Context, width int, breakpoint unit.Dp) bool {
	return width >= gtx.Dp(breakpoint)
}

// PickerHeight is the height of a labelled picker: the label line plus the
// default control.
const PickerHeight = 24 + ControlDefault

// Place lays a child out at an offset with an exact width and keeps the view
// origin in the same coordinate system, so popups anchored inside the child
// open next to their trigger. The child's height is its own measured height.
func Place(gtx layout.Context, view View, offset image.Point, width int, child func(layout.Context, View) layout.Dimensions) layout.Dimensions {
	defer op.Offset(offset).Push(gtx.Ops).Pop()
	gtx.Constraints.Min, gtx.Constraints.Max.X = image.Pt(width, 0), width
	view.Origin = view.Origin.Add(offset)
	return child(gtx, view)
}

// Grid flows equal-width cells into as many columns as the available width can
// hold, the way a CSS grid with a minimum column width does. A row is as tall
// as its tallest cell.
type Grid struct {
	// MinColumn is the narrowest a cell may become before the grid drops a
	// column.
	MinColumn unit.Dp
	// Gap separates cells horizontally and vertically.
	Gap unit.Dp
	// MaxColumns caps the column count; zero means no cap.
	MaxColumns int
	// Balanced evens out the rows so a short last row never leaves an orphan:
	// four cells in three columns become two rows of two.
	Balanced bool
	// Rows, when set, gives every cell the height of the tallest cell in its
	// row, as stretched CSS grid items are. It carries the heights between
	// frames, so a cell must pass its Constraints.Min.Y on to its card.
	Rows *RowHeights
}

// RowHeights remembers the tallest cell of each grid row from the previous
// frame. The first frame after the grid's shape changes lays out unstretched
// and asks for another frame.
type RowHeights struct {
	shape   [3]int
	heights []int
}

// Columns returns how many columns hold count cells in a width in pixels.
func (g Grid) Columns(gtx layout.Context, width, count int) int {
	gap, minimum := gtx.Dp(g.Gap), max(1, gtx.Dp(g.MinColumn))
	columns := max(1, (width+gap)/(minimum+gap))
	if g.MaxColumns > 0 {
		columns = min(columns, g.MaxColumns)
	}
	if g.Balanced && count > 0 {
		rows := (count + columns - 1) / columns
		columns = (count + rows - 1) / rows
	}
	return columns
}

// Layout places count cells and returns the grid's size. Every cell receives a
// view whose origin is its own position, so popups stay anchored.
func (g Grid) Layout(gtx layout.Context, view View, count int, cell func(gtx layout.Context, view View, index int) layout.Dimensions) layout.Dimensions {
	width := gtx.Constraints.Max.X
	gap := gtx.Dp(g.Gap)
	columns := g.Columns(gtx, width, count)
	cellWidth := max(0, (width-(columns-1)*gap)/columns)
	rows := (count + columns - 1) / columns
	var known []int
	if g.Rows != nil {
		shape := [3]int{cellWidth, columns, count}
		if g.Rows.shape != shape || len(g.Rows.heights) != rows {
			g.Rows.shape, g.Rows.heights = shape, make([]int, rows)
		}
		known = g.Rows.heights
	}
	y := 0
	changed := false
	for start := 0; start < count; start += columns {
		row := 0
		for column := 0; column < columns && start+column < count; column++ {
			index := start + column
			position := image.Pt(column*(cellWidth+gap), y)
			dims := Place(gtx, view, position, cellWidth, func(gtx layout.Context, view View) layout.Dimensions {
				if known != nil {
					gtx.Constraints.Min.Y = known[start/columns]
				}
				return cell(gtx, view, index)
			})
			row = max(row, dims.Size.Y)
		}
		if known != nil && known[start/columns] != row {
			known[start/columns], changed = row, true
		}
		y += row + gap
	}
	if changed {
		gtx.Execute(op.InvalidateCmd{})
	}
	if count > 0 {
		y -= gap
	}
	return layout.Dimensions{Size: image.Pt(width, y)}
}

// Wrap lays children out left to right at their natural widths and starts a
// new line when the next child would overflow, like a flex row with wrap. Use
// it for action rows whose labels differ in length or translate to longer text.
type Wrap struct {
	// Gap separates children horizontally and lines vertically.
	Gap unit.Dp
	// Middle centers the children of a line on its tallest child; otherwise
	// they share the top edge.
	Middle bool
}

// Layout places count children. A child that is wider than the row still gets
// a line of its own and is clipped by its own constraints.
func (w Wrap) Layout(gtx layout.Context, count int, child func(gtx layout.Context, index int) layout.Dimensions) layout.Dimensions {
	width, gap := gtx.Constraints.Max.X, gtx.Dp(w.Gap)
	gtx.Constraints.Min = image.Point{}
	type placed struct {
		call op.CallOp
		at   image.Point
		size image.Point
	}
	var line []placed
	var x, y, height, widest int
	flush := func() {
		for _, item := range line {
			dy := 0
			if w.Middle {
				dy = (height - item.size.Y) / 2
			}
			stack := op.Offset(image.Pt(item.at.X, y+dy)).Push(gtx.Ops)
			item.call.Add(gtx.Ops)
			stack.Pop()
		}
		line = line[:0]
	}
	for i := 0; i < count; i++ {
		macro := op.Record(gtx.Ops)
		dims := child(gtx, i)
		call := macro.Stop()
		if x > 0 && x+dims.Size.X > width {
			flush()
			x, y, height = 0, y+height+gap, 0
		}
		line = append(line, placed{call: call, at: image.Pt(x, 0), size: dims.Size})
		x += dims.Size.X + gap
		height = max(height, dims.Size.Y)
		widest = max(widest, x-gap)
	}
	flush()
	return layout.Dimensions{Size: image.Pt(widest, y+height)}
}

// Measure narrows a page to a readable column and centers it in the width it
// was given. Forms and detail screens declare a measure; list and card pages
// do not, so they fill the window. It returns the constraints and a view whose
// origin sits inside the column, so popups anchor correctly.
//
// Call it once at the top of a page; the offset applies to everything the page
// draws after it.
func (v View) Measure(gtx layout.Context, width unit.Dp) (layout.Context, View) {
	size := min(gtx.Dp(width), gtx.Constraints.Max.X)
	shift := (gtx.Constraints.Max.X - size) / 2
	op.Offset(image.Pt(shift, 0)).Add(gtx.Ops)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = size, size
	v.Origin.X += shift
	return gtx, v
}

// Bleed lays a widget out wider than its slot by amount on each side, so a row
// with a hover surface can reach past a card's padding while its text stays
// aligned with the card's other content. The reported width is the slot's.
func Bleed(gtx layout.Context, amount unit.Dp, content layout.Widget) layout.Dimensions {
	px := gtx.Dp(amount)
	gtx.Constraints.Max.X += 2 * px
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	defer op.Offset(image.Pt(-px, 0)).Push(gtx.Ops).Pop()
	dims := content(gtx)
	dims.Size.X -= 2 * px
	return dims
}
