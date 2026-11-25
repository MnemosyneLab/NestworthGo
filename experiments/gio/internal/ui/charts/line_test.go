package charts

import (
	"image"
	"math"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func lineSpecForPlan(points ...LinePoint) LineSpec {
	return LineSpec{Series: []LineSeries{{ID: "net", Name: "Net worth", Points: points}}}
}

func point(id string, d int, v Value) LinePoint {
	return LinePoint{ID: ID(id), Time: day(2026, time.January, d), Value: v}
}

func planLine(spec LineSpec, plot image.Rectangle) linePlan {
	return spec.plan(spec.domain(5), plot, 4, 8, testPalette(false))
}

func TestLinePlanMapsTimeAndValuesToPixels(t *testing.T) {
	spec := lineSpecForPlan(point("a", 1, Num(-100)), point("b", 11, Num(0)), point("c", 21, Num(300)))
	plot := image.Rect(50, 20, 450, 220)
	p := planLine(spec, plot)
	if !p.plottable || len(p.series) != 1 {
		t.Fatalf("plan is not plottable: %+v", p)
	}
	pts := p.series[0].points
	// The viewport is the data extent, so the first and last point touch the
	// plot edges and the middle one is exactly half way along the time axis.
	if pts[0].at.X != 50 || pts[2].at.X != 450 || pts[1].at.X != 250 {
		t.Fatalf("x positions %v %v %v, want 50 250 450", pts[0].at.X, pts[1].at.X, pts[2].at.X)
	}
	// The value axis is inverted and its domain bounds the ticks.
	if got := p.y.at(p.yTicks[len(p.yTicks)-1]); got != 20 {
		t.Fatalf("top tick maps to %v, want the plot top 20", got)
	}
	if got := p.y.at(p.yTicks[0]); got != 220 {
		t.Fatalf("bottom tick maps to %v, want the plot bottom 220", got)
	}
	if !p.zeroInside {
		t.Fatal("zero lies inside a range that crosses it")
	}
	if float64(pts[1].at.Y) != p.zero {
		t.Fatalf("a real zero is plotted at %v, want the zero line %v", pts[1].at.Y, p.zero)
	}
	if !(pts[0].at.Y > float32(p.zero) && pts[2].at.Y < float32(p.zero)) {
		t.Fatalf("negative must sit below and positive above the zero line: %v", pts)
	}
}

func TestLinePlanBreaksAtMissingValuesButNotAtZero(t *testing.T) {
	spec := lineSpecForPlan(
		point("a", 1, Num(10)), point("b", 2, Num(0)), point("c", 3, NoValue),
		point("d", 4, Num(12)), point("e", 5, Num(14)), point("f", 6, Num(math.NaN())), point("g", 7, Num(9)),
	)
	p := planLine(spec, image.Rect(0, 0, 700, 200))
	sp := p.series[0]
	if len(sp.runs) != 2 {
		t.Fatalf("got %d runs, want 2 (a-b and d-e): %+v", len(sp.runs), sp.runs)
	}
	if len(sp.runs[0].points) != 2 || len(sp.runs[1].points) != 2 {
		t.Fatalf("run lengths %d and %d, want 2 and 2", len(sp.runs[0].points), len(sp.runs[1].points))
	}
	// The real zero joined its neighbor; the missing points did not.
	if !sp.points[1].present || sp.points[2].present || sp.points[5].present {
		t.Fatalf("presence %v %v %v, want true false false", sp.points[1].present, sp.points[2].present, sp.points[5].present)
	}
	if len(sp.isolated) != 1 || sp.isolated[0] != 6 {
		t.Fatalf("isolated = %v, want only the lone final point 6", sp.isolated)
	}
	if sp.last != 6 {
		t.Fatalf("last = %d, want 6", sp.last)
	}
	// A missing point is still reachable from the keyboard and the pointer.
	if len(p.order) != 7 {
		t.Fatalf("keyboard order has %d items, want all 7", len(p.order))
	}
}

func TestLinePlanDeclaredBreakAndProvisionalEdges(t *testing.T) {
	pts := []LinePoint{point("a", 1, Num(1)), point("b", 2, Num(2)), point("c", 3, Num(3)), point("d", 4, Num(4)), point("e", 5, Num(5))}
	pts[2].BreakBefore = true
	pts[4].Provisional = true
	p := planLine(lineSpecForPlan(pts...), image.Rect(0, 0, 400, 200))
	sp := p.series[0]
	// a-b | c-d solid, d-e dashed because e is provisional.
	if len(sp.chains) != 2 {
		t.Fatalf("got %d chains, want 2 split at the declared break", len(sp.chains))
	}
	var dashed, solid int
	for _, r := range sp.runs {
		if r.dashed {
			dashed++
		} else {
			solid++
		}
	}
	if dashed != 1 || solid != 2 {
		t.Fatalf("runs: %d solid, %d dashed, want 2 and 1", solid, dashed)
	}
	if !sp.points[4].provisional || sp.points[3].provisional {
		t.Fatal("only the estimate is provisional")
	}
}

func TestLineDomainDistinguishesEmptyMissingAndZero(t *testing.T) {
	if d := (LineSpec{}).domain(4); d.plottable {
		t.Fatal("an empty chart is not plottable")
	}
	if d := lineSpecForPlan(point("a", 1, NoValue), point("b", 2, Num(math.Inf(1)))).domain(4); d.plottable {
		t.Fatal("a chart whose points are all missing is not plottable")
	}
	d := lineSpecForPlan(point("a", 1, Num(0)), point("b", 2, Num(0))).domain(4)
	if !d.plottable || len(d.yTicks) < 2 {
		t.Fatalf("real zeros must plot on a padded axis, got %+v", d)
	}
	if d := lineSpecForPlan(point("a", 1, Num(5)), point("b", 2, Num(9))).domain(4); d.yTicks[0] <= 0 {
		t.Fatalf("zero is not forced into the axis by default: %v", d.yTicks)
	}
	spec := lineSpecForPlan(point("a", 1, Num(5)), point("b", 2, Num(9)))
	spec.IncludeZero = true
	if d := spec.domain(4); d.yTicks[0] > 0 {
		t.Fatalf("IncludeZero must reach zero: %v", d.yTicks)
	}
}

func TestLinePickAndKeyboardMove(t *testing.T) {
	spec := LineSpec{Series: []LineSeries{
		{ID: "assets", Points: []LinePoint{point("a1", 1, Num(100)), point("a2", 11, Num(200)), point("a3", 21, Num(300))}},
		{ID: "debts", Points: []LinePoint{point("d1", 1, Num(-10)), point("d2", 11, Num(-20)), point("d3", 21, Num(-30))}},
	}}
	p := planLine(spec, image.Rect(0, 0, 400, 200))
	mid := p.series[0].points[1].at
	got, ok := p.pick(image.Pt(int(mid.X)+3, 10), 28)
	if !ok || got != (Selection{Series: "assets", Item: "a2"}) {
		t.Fatalf("pick near the top of the middle column = %v %v, want assets/a2", got, ok)
	}
	got, _ = p.pick(image.Pt(int(mid.X)+3, 190), 28)
	if got != (Selection{Series: "debts", Item: "d2"}) {
		t.Fatalf("pick near the bottom of the middle column = %v, want debts/d2", got)
	}
	if _, ok := p.pick(image.Pt(int(mid.X)+120, 100), 28); ok {
		t.Fatal("a pointer far from every column must pick nothing")
	}

	start := Selection{Series: "assets", Item: "a2"}
	for _, c := range []struct {
		key  key.Name
		want Selection
	}{
		{key.NameLeftArrow, Selection{Series: "assets", Item: "a1"}},
		{key.NameRightArrow, Selection{Series: "assets", Item: "a3"}},
		{key.NameHome, Selection{Series: "assets", Item: "a1"}},
		{key.NameEnd, Selection{Series: "assets", Item: "a3"}},
		{key.NameDownArrow, Selection{Series: "debts", Item: "d2"}},
		{key.NameUpArrow, start},
	} {
		next, ok := p.move(start, c.key)
		if !ok || next != c.want {
			t.Fatalf("%s from a2 = %v %v, want %v", c.key, next, ok, c.want)
		}
	}
	// The cursor stops at the ends instead of wrapping.
	if next, _ := p.move(Selection{Series: "assets", Item: "a3"}, key.NameRightArrow); next.Item != "a3" {
		t.Fatalf("right at the end moved to %v", next)
	}
}

func TestLineRowsAndValidate(t *testing.T) {
	spec := LineSpec{
		Messages: Messages{Missing: "No quote"},
		Selected: Selection{Series: "net", Item: "b"},
		Series: []LineSeries{{ID: "net", Name: "Net worth", Points: []LinePoint{
			{ID: "a", Time: day(2026, 1, 1), Value: Num(0), Text: "$0.00"},
			{ID: "b", Time: day(2026, 1, 2), Text: ""},
			{ID: "b", Time: day(2026, 1, 3)},
			{Time: day(2026, 1, 4)},
		}}},
	}
	rows := spec.Rows()
	if rows[0].Value != "$0.00" || rows[0].Missing {
		t.Fatalf("a real zero must keep its exact text and not be missing: %+v", rows[0])
	}
	if rows[1].Value != "No quote" || !rows[1].Missing || !rows[1].Selected {
		t.Fatalf("a missing point reads as missing and keeps selection: %+v", rows[1])
	}
	problems := spec.Validate()
	if len(problems) != 2 || problems[0].Kind != ProblemRepeatedID && problems[1].Kind != ProblemRepeatedID {
		t.Fatalf("problems = %+v, want one repeated and one empty ID", problems)
	}
}

func lineSubject() subject {
	var chart LineChart
	build := func(sel Selection) LineSpec {
		return LineSpec{
			Label: "Wealth", Summary: "Net worth over time", Selected: sel,
			Messages: Messages{Selected: "Selected", Missing: "No value", Empty: "No data"},
			Series: []LineSeries{
				{ID: "net", Name: "Net", Points: []LinePoint{point("a", 1, Num(10)), point("b", 11, Num(20)), point("c", 21, NoValue), point("d", 31, Num(15))}},
				{ID: "debt", Name: "Debt", Points: []LinePoint{point("x", 1, Num(-5)), point("y", 11, Num(0)), point("z", 21, Num(-8))}},
			},
		}
	}
	return subject{
		name: "line", tag: &chart, end: Selection{Series: "net", Item: "d"},
		items: []Selection{{Series: "net", Item: "a"}, {Series: "net", Item: "b"}, {Series: "net", Item: "c"}, {Series: "net", Item: "d"}, {Series: "debt", Item: "x"}, {Series: "debt", Item: "y"}, {Series: "debt", Item: "z"}},
		draw: func(gtx layout.Context, view ui.View, sel Selection) (layout.Dimensions, []Event) {
			return chart.Layout(gtx, view, build(sel))
		},
		scan: func(size image.Point) []image.Point {
			var pts []image.Point
			// Points of different series share a time column, so the sweep
			// repeats at several heights to reach each of them.
			for _, y := range []int{60, 120, 180, 215, 232} {
				for x := 0; x < size.X; x += 3 {
					pts = append(pts, image.Pt(x, y))
				}
			}
			return pts
		},
	}
}

func init() { registry = append(registry, lineSubject()) }

func TestLineChartNoticesForEmptyAndAllMissing(t *testing.T) {
	msgs := Messages{Empty: "No data", Missing: "No values"}
	r := newRig(320, false)
	for name, spec := range map[string]LineSpec{
		"empty":   {Messages: msgs},
		"missing": lineSpecForPlan(point("a", 1, NoValue), point("b", 2, NoValue)),
	} {
		spec.Messages = msgs
		var chart LineChart
		size, events := r.frame(func(gtx layout.Context, v ui.View, _ Selection) (layout.Dimensions, []Event) {
			return chart.Layout(gtx, v, spec)
		}, Selection{})
		if size.X <= 0 || size.Y <= 0 || len(events) != 0 {
			t.Fatalf("%s: size %v events %v", name, size, events)
		}
	}
}

func TestItemLabelDoesNotRelyOnColorForSelection(t *testing.T) {
	r := readout{label: "Jan 1", value: "$1.00", detail: []string{"manual"}, selected: true}
	got := itemLabel(r, Messages{Selected: "Selected"})
	if got != "Selected, Jan 1, $1.00, manual" {
		t.Fatalf("label = %q", got)
	}
	if itemLabel(readout{label: "x"}, Messages{Selected: "Selected"}) != "x" {
		t.Fatal("unselected items carry no marker")
	}
}
