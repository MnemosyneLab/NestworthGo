package charts

import (
	"image"
	"math"
	"testing"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func columnGeometry(n int) waterfallGeometry {
	return waterfallGeometry{
		width: 700, plot: image.Rect(60, 10, 690, 210), labelTop: 214, cellBottom: 280,
		inset: 4, barFraction: 0.55, maxBarWidth: 56, minBarWidth: 12,
	}
}

func rowGeometry(n int) waterfallGeometry {
	heights := make([]int, n)
	heads := make([]int, n)
	for i := range heights {
		heights[i], heads[i] = 50, 16
	}
	return waterfallGeometry{
		rows: true, width: 320, plot: image.Rect(8, 0, 312, 0), rowHeights: heights, headHeights: heads,
		barHeight: 12, rowGap: 6, stubGap: 6, valueW: 90, inset: 8,
	}
}

func reconciledSteps() []WaterfallStep {
	return []WaterfallStep{
		{ID: "start", Kind: StepTotal, To: Num(1000)},
		{ID: "in", From: Num(1000), To: Num(1200)},
		{ID: "out", From: Num(1200), To: Num(1100)},
		{ID: "end", Kind: StepTotal, To: Num(1100)},
	}
}

func TestWaterfallPlanStartsAndEndsOnBaselineAndConnects(t *testing.T) {
	spec := WaterfallSpec{Steps: reconciledSteps()}
	ticks, ok := spec.domain(5)
	if !ok || ticks[0] != 0 {
		t.Fatalf("totals stand on zero, so the axis must include it: %v %v", ticks, ok)
	}
	p := spec.plan(columnGeometry(4), ticks)
	zeroY := int(math.Round(p.level.at(0)))
	if p.steps[0].bar.Max.Y != zeroY || p.steps[3].bar.Max.Y != zeroY {
		t.Fatalf("totals end at %d and %d, want the baseline %d", p.steps[0].bar.Max.Y, p.steps[3].bar.Max.Y, zeroY)
	}
	// The ending is drawn at exactly the level supplied: the same height as
	// the 1100 level where the last change stops (the bottom of a falling bar).
	if got, want := p.steps[3].bar.Min.Y, p.steps[2].bar.Max.Y; got != want {
		t.Fatalf("ending top %d, want the level the last step reached %d", got, want)
	}
	if len(spec.Breaks()) != 0 {
		t.Fatalf("a reconciled waterfall has no breaks: %v", spec.Breaks())
	}
	if len(p.connectors) != 3 {
		t.Fatalf("got %d connectors, want one per join", len(p.connectors))
	}
	for _, c := range p.connectors {
		if c.warn || c.from.Y != c.to.Y {
			t.Fatalf("connector %+v must be a level, unwarned line", c)
		}
	}
	if !p.steps[1].up || p.steps[2].up {
		t.Fatal("the 1000→1200 step rises and the 1200→1100 step falls")
	}
}

func TestWaterfallNeverRecomputesCallerTotals(t *testing.T) {
	steps := reconciledSteps()
	steps[3].To = Num(1111) // does not equal 1100: the caller owns the residual decision
	spec := WaterfallSpec{Steps: steps}
	if got := spec.Breaks(); len(got) != 1 || got[0] != "end" {
		t.Fatalf("Breaks = %v, want [end]", got)
	}
	ticks, _ := spec.domain(5)
	p := spec.plan(columnGeometry(4), ticks)
	if got := p.level.invert(float64(p.steps[3].bar.Min.Y)); math.Abs(got-1111) > 5 {
		t.Fatalf("ending drawn at %v, want the supplied 1111", got)
	}
	var warn int
	for _, c := range p.connectors {
		if c.warn {
			warn++
		}
	}
	if warn == 0 {
		t.Fatal("a non-reconciling join must be drawn as a warning")
	}
}

func TestWaterfallMissingLevelBreaksConnectorsWithPlaceholder(t *testing.T) {
	steps := reconciledSteps()
	steps[2].To = NoValue
	spec := WaterfallSpec{Steps: steps}
	ticks, _ := spec.domain(5)
	p := spec.plan(columnGeometry(4), ticks)
	if p.steps[2].present || !p.steps[2].bar.Empty() || p.steps[2].placeholder.Empty() {
		t.Fatalf("a missing level draws a placeholder, not a bar: %+v", p.steps[2])
	}
	if len(p.connectors) != 1 {
		t.Fatalf("got %d connectors, want only start→in", len(p.connectors))
	}
	if len(spec.Breaks()) != 0 {
		t.Fatalf("a join with a missing side is not a break: %v", spec.Breaks())
	}
}

func TestWaterfallNegativeLevelsAndRows(t *testing.T) {
	spec := WaterfallSpec{Steps: []WaterfallStep{
		{ID: "a", Kind: StepTotal, To: Num(-200)},
		{ID: "b", From: Num(-200), To: Num(100)},
		{ID: "c", Kind: StepTotal, To: Num(100)},
	}}
	ticks, _ := spec.domain(5)
	p := spec.plan(rowGeometry(3), ticks)
	if !p.rows {
		t.Fatal("expected the row layout")
	}
	zero := int(math.Round(p.level.at(0)))
	if a := p.steps[0].bar; a.Max.X != zero || a.Min.X >= zero {
		t.Fatalf("a negative total extends left of the baseline %d, got %v", zero, a)
	}
	if b := p.steps[1].bar; b.Min.X >= zero || b.Max.X <= zero {
		t.Fatalf("a step crossing zero spans it, got %v", b)
	}
	for _, s := range p.steps {
		if s.bar.Min.X < p.plot.Min.X || s.bar.Max.X > p.plot.Max.X {
			t.Fatalf("bar %v leaves the track %v", s.bar, p.plot)
		}
	}
	// Row joins are short stubs: they must stay clear of the next row's label.
	for _, c := range p.connectors {
		if c.from.X != c.to.X || c.to.Y <= c.from.Y {
			t.Fatalf("stub %+v must be a downward vertical guide", c)
		}
	}
}

func TestWaterfallDomainEmpty(t *testing.T) {
	if _, ok := (WaterfallSpec{}).domain(5); ok {
		t.Fatal("an empty waterfall has no domain")
	}
	spec := WaterfallSpec{Steps: []WaterfallStep{{ID: "a", Kind: StepTotal, To: NoValue}}}
	if _, ok := spec.domain(5); ok {
		t.Fatal("only missing levels: no domain")
	}
}

func waterfallSubject(minColumn unit.Dp) subject {
	var chart WaterfallChart
	build := func(sel Selection) WaterfallSpec {
		return WaterfallSpec{
			Label: "Change", Selected: sel, MinColumn: minColumn,
			Messages:    Messages{Selected: "Selected", Missing: "No value", Empty: "No data"},
			FormatValue: func(float64) string { return "v" },
			Steps: []WaterfallStep{
				{ID: "start", Label: "Beginning", Kind: StepTotal, To: Num(1000), Text: "$1,000.00"},
				{ID: "in", Label: "Contributions", From: Num(1000), To: Num(1200), Text: "+$200.00"},
				{ID: "out", Label: "Withdrawals", From: Num(1200), To: Num(1100), Text: "-$100.00"},
				{ID: "end", Label: "Ending", Kind: StepTotal, To: Num(1100), Text: "$1,100.00"},
			},
		}
	}
	return subject{
		name: "waterfall", tag: &chart, end: Selection{Item: "end"},
		items: []Selection{{Item: "start"}, {Item: "in"}, {Item: "out"}, {Item: "end"}},
		draw: func(gtx layout.Context, view ui.View, sel Selection) (layout.Dimensions, []Event) {
			return chart.Layout(gtx, view, build(sel))
		},
		// A sweep across the plot reaches every column; a sweep down the middle
		// reaches every row. Whichever layout is active, the other sweep adds
		// nothing out of order.
		scan: func(size image.Point) []image.Point {
			var pts []image.Point
			for x := 0; x < size.X; x += 3 {
				pts = append(pts, image.Pt(x, 40))
			}
			return append(pts, downTheLeftColumn(size)...)
		},
	}
}

func init() {
	registry = append(registry, waterfallSubject(0))
	fitOnly = append(fitOnly, waterfallSubject(24))
}

func TestWaterfallChartNoticeForEmpty(t *testing.T) {
	var chart WaterfallChart
	size, events := newRig(320, false).frame(func(gtx layout.Context, v ui.View, _ Selection) (layout.Dimensions, []Event) {
		return chart.Layout(gtx, v, WaterfallSpec{Messages: Messages{Empty: "No data"}})
	}, Selection{})
	if size.X <= 0 || size.Y <= 0 || len(events) != 0 {
		t.Fatalf("size %v events %v", size, events)
	}
}

func TestWaterfallSwitchesToRowsWhenColumnsDoNotFit(t *testing.T) {
	// A horizontal sweep through the first 40px reaches every step in the
	// column layout and only the first step in the row layout.
	reach := func(width int) int {
		r := newRig(width, false)
		s := waterfallSubject(0)
		size, _ := r.frame(s.draw, Selection{})
		r.frame(s.draw, Selection{})
		var seen []Selection
		for x := 0; x < size.X; x += 3 {
			r.press(image.Pt(x, 40))
			_, events := r.frame(s.draw, Selection{})
			for _, e := range events {
				if indexOf(seen, e.Selection) < 0 {
					seen = append(seen, e.Selection)
				}
			}
		}
		return len(seen)
	}
	if got := reach(1100); got != 4 {
		t.Fatalf("wide window reached %d steps in one sweep, want 4 columns", got)
	}
	if got := reach(320); got != 1 {
		t.Fatalf("narrow window reached %d steps in one sweep, want 1 row", got)
	}
}
