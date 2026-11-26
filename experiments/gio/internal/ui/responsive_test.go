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

func TestWrapStartsANewLineWhenAChildOverflows(t *testing.T) {
	var ops op.Ops
	gtx := testContext(&ops, nil, time.Unix(0, 0), 250)
	widths := []int{100, 100, 100}
	dims := Wrap{Gap: 10}.Layout(gtx, len(widths), func(gtx layout.Context, index int) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(widths[index], 30)}
	})
	// Two children fit on the first line (210); the third wraps below.
	if want := image.Pt(210, 30+10+30); dims.Size != want {
		t.Fatalf("wrapped size = %v, want %v", dims.Size, want)
	}
	gtx.Constraints.Max.X = 400
	if got := (Wrap{Gap: 10}).Layout(gtx, len(widths), func(gtx layout.Context, index int) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(widths[index], 30)}
	}).Size; got != image.Pt(320, 30) {
		t.Fatalf("single-line size = %v, want (320, 30)", got)
	}
}

// rowOfHeight is a row of a fixed height. With height zero it takes the
// minimum height it is given, like a list that fills its row.
func rowOfHeight(height int, got *image.Point) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if got != nil {
			*got = image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
		}
		if height == 0 {
			height = gtx.Constraints.Min.Y
		}
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
	}
}

func TestColumnFillTakesTheRestOfTheWindowOrItsMinimum(t *testing.T) {
	for _, test := range []struct{ window, fill, total int }{
		{1000, 1000 - 100 - 8, 1000}, // the Fill row takes what is left
		{200, 220, 100 + 8 + 220},    // a short window keeps the minimum and scrolls
	} {
		var ops op.Ops
		gtx := testContext(&ops, nil, time.Unix(0, 0), 400)
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = test.window, 1_000_000 // the stage
		var fill image.Point
		dims := Column{Gap: 8}.Layout(gtx, Rigid(rowOfHeight(100, nil)), Fill(rowOfHeight(0, &fill)))
		if fill.Y != test.fill {
			t.Errorf("window %d: Fill row height = %d, want %d", test.window, fill.Y, test.fill)
		}
		if dims.Size.Y != test.total {
			t.Errorf("window %d: column height = %d, want %d", test.window, dims.Size.Y, test.total)
		}
	}
}

func TestColumnInsideABoundedParentFillsThatParent(t *testing.T) {
	var ops op.Ops
	gtx := testContext(&ops, nil, time.Unix(0, 0), 400)
	gtx.Constraints.Max.Y = 500
	var fill image.Point
	Column{}.Layout(gtx, Rigid(rowOfHeight(100, nil)), Fill(rowOfHeight(0, &fill)))
	if fill.Y != 400 {
		t.Errorf("Fill row height = %d, want 400", fill.Y)
	}
}

func TestMeasureCentersAReadableColumnAndMovesTheOrigin(t *testing.T) {
	view := testView()
	view.Origin = image.Pt(10, 20)
	var ops op.Ops
	gtx := testContext(&ops, nil, time.Unix(0, 0), 1000)
	gtx, view = view.Measure(gtx, 600)
	if gtx.Constraints.Max.X != 600 || gtx.Constraints.Min.X != 600 {
		t.Errorf("measured width = %d..%d, want 600", gtx.Constraints.Min.X, gtx.Constraints.Max.X)
	}
	if view.Origin != image.Pt(10+200, 20) {
		t.Errorf("origin = %v, want the centering shift added", view.Origin)
	}
	gtx = testContext(&ops, nil, time.Unix(0, 0), 400)
	_, view = testView().Measure(gtx, 600)
	if view.Origin.X != 0 {
		t.Errorf("a window narrower than the measure must not shift, got %v", view.Origin)
	}
}

func TestGridRowsStretchCellsToTheTallestOnTheNextFrame(t *testing.T) {
	view := testView()
	var rows RowHeights
	grid := Grid{MinColumn: 150, Gap: 10, Rows: &rows}
	var minimums []int
	frame := func() {
		var ops op.Ops
		gtx := testContext(&ops, nil, time.Unix(0, 0), 330)
		minimums = minimums[:0]
		grid.Layout(gtx, view, 2, func(gtx layout.Context, view View, index int) layout.Dimensions {
			minimums = append(minimums, gtx.Constraints.Min.Y)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, max(20+30*index, gtx.Constraints.Min.Y))}
		})
	}
	frame()
	if minimums[0] != 0 || minimums[1] != 0 {
		t.Fatalf("first frame minimums = %v, want none", minimums)
	}
	frame()
	if minimums[0] != 50 || minimums[1] != 50 {
		t.Fatalf("second frame minimums = %v, want the tallest cell (50)", minimums)
	}
}
