package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
)

// A list's scrollbar floats over its edge. Rows must get the full width, or a
// fixed toolbar above the list stops lining up with the rows below it.
func TestListScrollbarDoesNotTakeRowWidth(t *testing.T) {
	view := testView()
	var ops op.Ops
	state := widget.List{List: layout.List{Axis: layout.Vertical}}
	gtx := testContext(&ops, nil, time.Unix(1, 0), 600)
	var rowWidth int
	view.List(&state).Layout(gtx, 40, func(gtx layout.Context, _ int) layout.Dimensions {
		rowWidth = gtx.Constraints.Max.X
		return layout.Dimensions{Size: image.Pt(rowWidth, 50)}
	})
	if rowWidth != 600 {
		t.Fatalf("row width %d, want the full 600", rowWidth)
	}
}
