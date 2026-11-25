package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
)

func TestGridColumnsFollowWidthAndBalanceRows(t *testing.T) {
	var ops op.Ops
	gtx := testContext(&ops, nil, time.Unix(0, 0), 600)
	grid := Grid{MinColumn: 150, Gap: 10, MaxColumns: 4}
	for _, test := range []struct{ width, count, want int }{
		{100, 4, 1}, {310, 4, 2}, {470, 4, 3}, {630, 4, 4}, {2000, 4, 4}, {2000, 9, 4},
	} {
		if got := grid.Columns(gtx, test.width, test.count); got != test.want {
			t.Errorf("columns(width %d, count %d) = %d, want %d", test.width, test.count, got, test.want)
		}
	}
	grid.Balanced = true
	for _, test := range []struct{ width, count, want int }{
		{470, 4, 2}, {630, 4, 4}, {630, 5, 3}, {100, 4, 1}, {470, 0, 3},
	} {
		if got := grid.Columns(gtx, test.width, test.count); got != test.want {
			t.Errorf("balanced columns(width %d, count %d) = %d, want %d", test.width, test.count, got, test.want)
		}
	}
}

func TestGridPlacesCellsWithTheirOwnOrigin(t *testing.T) {
	view := testView()
	view.Origin = image.Pt(100, 50)
	var ops op.Ops
	gtx := testContext(&ops, nil, time.Unix(0, 0), 330)
	grid := Grid{MinColumn: 150, Gap: 10}
	var origins []image.Point
	var widths []int
	dims := grid.Layout(gtx, view, 3, func(gtx layout.Context, view View, index int) layout.Dimensions {
		origins = append(origins, view.Origin)
		widths = append(widths, gtx.Constraints.Max.X)
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 20+10*index)}
	})
	// Two columns of 160 with a 10 gap; the first row is as tall as its taller cell.
	want := []image.Point{{100, 50}, {270, 50}, {100, 90}}
	for i := range want {
		if origins[i] != want[i] || widths[i] != 160 {
			t.Errorf("cell %d origin %v width %d, want %v width 160", i, origins[i], widths[i], want[i])
		}
	}
	if dims.Size != image.Pt(330, 30+10+40) {
		t.Errorf("grid size = %v, want (330, 80)", dims.Size)
	}
}
