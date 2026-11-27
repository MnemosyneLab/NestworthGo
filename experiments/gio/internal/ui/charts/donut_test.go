package charts

import (
	"image"
	"math"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func donutGeometryFor(width int, stacked bool, rows int) donutGeometry {
	legend := make([]int, rows)
	for i := range legend {
		legend[i] = 32
	}
	return donutGeometry{width: width, stacked: stacked, diameter: 240, pullout: 6, holeRatio: 0.64, gap: 16, legendRows: legend}
}

func TestDonutPlanNeverMirrorsOrMergesSlices(t *testing.T) {
	spec := DonutSpec{Slices: []Slice{
		{ID: "cash", Value: Num(600)},
		{ID: "stock", Value: Num(390)},
		{ID: "tiny", Value: Num(10)},
		{ID: "debt", Value: Num(-300)},
		{ID: "unknown", Value: NoValue},
		{ID: "zero", Value: Num(0)},
	}}
	p := spec.plan(donutGeometryFor(600, false, 6))
	if len(p.slices) != 6 {
		t.Fatalf("got %d legend entries, want all 6: nothing is merged or dropped", len(p.slices))
	}
	if p.total != 1000 {
		t.Fatalf("total = %v, want 1000 (the negative is not abs-ed in)", p.total)
	}
	drawn := map[ID]bool{}
	for _, s := range p.slices {
		drawn[s.sel.Item] = s.drawn
	}
	want := map[ID]bool{"cash": true, "stock": true, "tiny": true, "debt": false, "unknown": false, "zero": false}
	for id, w := range want {
		if drawn[id] != w {
			t.Fatalf("%s drawn = %v, want %v", id, drawn[id], w)
		}
	}
	if got := p.slices[2].span.sweep(); math.Abs(got-0.01*2*math.Pi) > 1e-9 {
		t.Fatalf("a 1%% slice sweeps %v rad, want its exact share", got)
	}
}

func TestDonutPlanZeroTotalHasNoArcs(t *testing.T) {
	spec := DonutSpec{Slices: []Slice{{ID: "a", Value: Num(0)}, {ID: "b", Value: Num(-5)}, {ID: "c", Value: NoValue}}}
	p := spec.plan(donutGeometryFor(320, true, 3))
	if p.total != 0 {
		t.Fatalf("total = %v, want 0", p.total)
	}
	for _, s := range p.slices {
		if s.drawn {
			t.Fatalf("slice %v drawn with a zero total", s.sel)
		}
	}
	ring := image.Pt(int(p.center.X), int(p.center.Y)-int((p.outer+p.inner)/2))
	if sel, ok := p.pick(ring); ok {
		t.Fatalf("the ring of a zero total picked %v", sel)
	}
}

func TestDonutPickRingAndLegend(t *testing.T) {
	spec := DonutSpec{Slices: []Slice{{ID: "half", Value: Num(50)}, {ID: "rest", Value: Num(50)}}}
	p := spec.plan(donutGeometryFor(600, false, 2))
	mid := (p.outer + p.inner) / 2
	right := image.Pt(int(p.center.X+float32(mid)), int(p.center.Y))
	left := image.Pt(int(p.center.X-float32(mid)), int(p.center.Y))
	if sel, ok := p.pick(right); !ok || sel.Item != "half" {
		t.Fatalf("3 o'clock = %v %v, want half (first arc runs clockwise from the top)", sel, ok)
	}
	if sel, ok := p.pick(left); !ok || sel.Item != "rest" {
		t.Fatalf("9 o'clock = %v %v, want rest", sel, ok)
	}
	if _, ok := p.pick(image.Pt(int(p.center.X), int(p.center.Y))); ok {
		t.Fatal("the hole picks nothing")
	}
	row := p.slices[1].row
	if sel, ok := p.pick(row.Min.Add(image.Pt(5, 5))); !ok || sel.Item != "rest" {
		t.Fatalf("legend row hit = %v %v, want rest", sel, ok)
	}
}

func TestDonutPlanStackedFitsNarrowWidth(t *testing.T) {
	spec := DonutSpec{Slices: []Slice{{ID: "a", Value: Num(1)}, {ID: "b", Value: Num(2)}, {ID: "c", Value: Num(3)}}}
	g := donutGeometryFor(280, true, 3)
	p := spec.plan(g)
	if p.legend.Min.Y < g.diameter || p.legend.Dx() != 280 {
		t.Fatalf("stacked legend %v must sit below the ring at full width", p.legend)
	}
	if got := int(p.center.X); got != 140 {
		t.Fatalf("ring center x = %d, want the middle 140", got)
	}
	if p.center.X-float32(p.outer+p.pullout) < 0 {
		t.Fatalf("the ring %v overflows a %dpx column", p.center, g.width)
	}
	wide := spec.plan(donutGeometryFor(700, false, 3))
	if wide.legend.Min.X < 240 {
		t.Fatalf("side legend %v overlaps the ring", wide.legend)
	}
}

func TestDonutRowsCarryShareAndMissingText(t *testing.T) {
	spec := DonutSpec{
		Messages: Messages{Missing: "n/a"},
		Selected: Selection{Item: "a"},
		Slices: []Slice{
			{ID: "a", Label: "Cash", Value: Num(10), Text: "$10.00", Share: "100%", Detail: []string{"USD"}},
			{ID: "b", Label: "Unknown", Value: NoValue},
		},
	}
	rows := spec.Rows()
	if rows[0].Value != "$10.00" || rows[0].Detail[0] != "100%" || rows[0].Detail[1] != "USD" || !rows[0].Selected {
		t.Fatalf("row 0 = %+v", rows[0])
	}
	if rows[1].Value != "n/a" || !rows[1].Missing {
		t.Fatalf("row 1 = %+v", rows[1])
	}
}

func donutSubject() subject {
	var chart DonutChart
	build := func(sel Selection) DonutSpec {
		return DonutSpec{
			Label: "Composition", Selected: sel, NotDrawn: "Not in ring", ZeroTotal: "Nothing to show",
			CenterValue: "$1,000.00", CenterCaption: "Total",
			Messages: Messages{Selected: "Selected", Missing: "No value", Empty: "No data"},
			Slices: []Slice{
				{ID: "cash", Label: "Cash", Value: Num(600), Text: "$600.00", Share: "60%"},
				{ID: "stock", Label: "Stocks", Value: Num(400), Text: "$400.00", Share: "40%"},
				{ID: "debt", Label: "Credit card", Value: Num(-300), Text: "-$300.00"},
			},
		}
	}
	return subject{
		name: "donut", tag: &chart, end: Selection{Item: "debt"},
		items: []Selection{{Item: "cash"}, {Item: "stock"}, {Item: "debt"}},
		draw: func(gtx layout.Context, view ui.View, sel Selection) (layout.Dimensions, []Event) {
			return chart.Layout(gtx, view, build(sel))
		},
		scan: downTheLeftColumn,
	}
}

func init() { registry = append(registry, donutSubject()) }

func TestDonutChartNoticesForEmptyAndZeroTotal(t *testing.T) {
	msgs := Messages{Empty: "No data", Missing: "No values"}
	r := newRig(320, false)
	for name, spec := range map[string]DonutSpec{
		"empty":      {Messages: msgs},
		"zero total": {Messages: msgs, ZeroTotal: "Nothing", Slices: []Slice{{ID: "a", Value: Num(0)}, {ID: "b", Value: NoValue}}},
	} {
		var chart DonutChart
		size, events := r.frame(func(gtx layout.Context, v ui.View, _ Selection) (layout.Dimensions, []Event) {
			return chart.Layout(gtx, v, spec)
		}, Selection{})
		if size.X <= 0 || size.Y <= 0 || len(events) != 0 {
			t.Fatalf("%s: size %v events %v", name, size, events)
		}
	}
}

func TestDonutCanHighlightWithoutAFooter(t *testing.T) {
	r := newRig(320, false)
	var chart DonutChart
	spec := DonutSpec{Slices: []Slice{{ID: "cash", Label: "Cash", Value: Num(60), Text: "US$60.00", Share: "100%"}}}
	draw := func(gtx layout.Context, view ui.View, _ Selection) (layout.Dimensions, []Event) {
		return chart.Layout(gtx, view, spec)
	}
	regular, _ := r.frame(draw, Selection{})
	spec.HideReadout = true
	compact, _ := r.frame(draw, Selection{})
	r.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(175, 38)})
	hovered, _ := r.frame(draw, Selection{})
	if chart.hover.Item != "cash" || compact != hovered || compact.Y >= regular.Y {
		t.Fatalf("footerless donut hover = %+v, size = %v on hover and %v at rest, regular = %v", chart.hover, hovered, compact, regular)
	}
}
