package charts

import (
	"image"
	"math"
	"testing"

	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func barGeometryFor(width, rows int, wide bool) barGeometry {
	heights := make([]int, rows)
	heads := make([]int, rows)
	for i := range heights {
		heights[i], heads[i] = 40, 16
	}
	return barGeometry{width: width, wide: wide, labelW: 160, valueW: 80, gap: 8, inset: 4, rowHeights: heights, headHeights: heads, barHeight: 12, rowGap: 6}
}

func TestBarPlanSharesOneZeroAxisWithoutSorting(t *testing.T) {
	spec := BarSpec{Bars: []Bar{
		{ID: "gain", Label: "A very long instrument name that must wrap rather than be cut", Value: Num(100)},
		{ID: "loss", Label: "Loss", Value: Num(-50)},
		{ID: "flat", Label: "Flat", Value: Num(0)},
		{ID: "gone", Label: "Missing", Value: NoValue},
	}}
	p := spec.plan(barGeometryFor(600, len(spec.Bars), true))
	if len(p.rows) != 4 {
		t.Fatalf("got %d rows, want 4: nothing is dropped", len(p.rows))
	}
	for i, id := range []ID{"gain", "loss", "flat", "gone"} {
		if p.rows[i].sel.Item != id {
			t.Fatalf("row %d is %q, want %q: order is the caller's", i, p.rows[i].sel.Item, id)
		}
	}
	gain, loss, flat, gone := p.rows[0], p.rows[1], p.rows[2], p.rows[3]
	if gain.bar.Min.X != p.zeroX || gain.bar.Max.X != p.trackMax {
		t.Fatalf("positive bar %v must run from zero %d to the track end %d", gain.bar, p.zeroX, p.trackMax)
	}
	if loss.bar.Max.X != p.zeroX || loss.bar.Min.X >= p.zeroX {
		t.Fatalf("negative bar %v must end at zero %d and extend left", loss.bar, p.zeroX)
	}
	// Half the magnitude is half the length.
	if got, want := float64(loss.bar.Dx()), float64(gain.bar.Dx())/2; math.Abs(got-want) > 1.5 {
		t.Fatalf("lengths %v and %v do not share one scale", got, want)
	}
	if !flat.present || flat.bar.Dx() < minBarTickPx || flat.bar.Min.X > p.zeroX || flat.bar.Max.X < p.zeroX {
		t.Fatalf("a real zero is a tick on the axis, got %v", flat.bar)
	}
	if gone.present || !gone.bar.Empty() {
		t.Fatalf("a missing value draws no bar, got %+v", gone)
	}
}

func TestBarPlanSymmetricCentersZero(t *testing.T) {
	spec := BarSpec{Symmetric: true, Bars: []Bar{{ID: "a", Value: Num(10)}, {ID: "b", Value: Num(-40)}}}
	p := spec.plan(barGeometryFor(600, 2, true))
	if mid := (p.trackMin + p.trackMax) / 2; absInt(p.zeroX-mid) > 1 {
		t.Fatalf("zero at %d, want the track center %d", p.zeroX, mid)
	}
	if a, b := p.rows[0].bar.Dx(), p.rows[1].bar.Dx(); a*4 < b-4 || a*4 > b+4 {
		t.Fatalf("lengths %d and %d, want 1:4", a, b)
	}
}

func TestBarPlanKeepsSubpixelBarsOnTheirSignedSide(t *testing.T) {
	spec := BarSpec{Symmetric: true, Bars: []Bar{
		{ID: "price", Value: Num(177.25)},
		{ID: "interest", Value: Num(0.01)},
		{ID: "fee", Value: Num(-0.25)},
	}}
	p := spec.plan(barGeometryFor(600, len(spec.Bars), true))
	interest, fee := p.rows[1].bar, p.rows[2].bar
	if interest.Min.X != p.zeroX || interest.Dx() != minBarTickPx {
		t.Fatalf("small positive bar %v should remain visible to the right of zero %d", interest, p.zeroX)
	}
	if fee.Max.X != p.zeroX || fee.Dx() != minBarTickPx {
		t.Fatalf("small negative bar %v should remain visible to the left of zero %d", fee, p.zeroX)
	}
}

func TestBarPlanNarrowRowsStackLabelAndBar(t *testing.T) {
	spec := BarSpec{Bars: []Bar{{ID: "a", Value: Num(5)}, {ID: "b", Value: Num(-5)}}}
	g := barGeometryFor(320, 2, false)
	p := spec.plan(g)
	if !p.compact {
		t.Fatal("a narrow plan must use the two-line layout")
	}
	for _, r := range p.rows {
		if r.band.Min.Y <= r.label.Y {
			t.Fatalf("the bar band %v must sit below the label at %v", r.band, r.label)
		}
		if r.band.Min.X < 0 || r.band.Max.X > g.width {
			t.Fatalf("band %v overflows the %dpx width", r.band, g.width)
		}
		if r.labelWidth <= 0 {
			t.Fatalf("label width %d", r.labelWidth)
		}
	}
	if _, ok := p.pick(image.Pt(10, p.rows[1].row.Min.Y+3)); !ok {
		t.Fatal("pointer in the second row picks nothing")
	}
}

func TestBarDomainAllZeroOrMissing(t *testing.T) {
	if _, _, ok := (BarSpec{Bars: []Bar{{ID: "a", Value: NoValue}}}).domain(); ok {
		t.Fatal("no value means not ok")
	}
	lo, hi, ok := (BarSpec{Bars: []Bar{{ID: "a", Value: Num(0)}}}).domain()
	if !ok || lo >= hi {
		t.Fatalf("a real zero must be ok on a non-degenerate scale, got %v %v %v", lo, hi, ok)
	}
}

func barsSubject() subject {
	var chart BarChart
	build := func(sel Selection) BarSpec {
		return BarSpec{
			Label: "Contribution", Selected: sel, Messages: Messages{Selected: "Selected", Missing: "No value", Empty: "No data"},
			FormatValue: func(v float64) string { return "v" },
			Bars: []Bar{
				{ID: "long", Label: "A very long instrument name that has to wrap inside a narrow label column", Value: Num(120), Text: "+$120.00"},
				{ID: "loss", Label: "Loss", Value: Num(-80), Text: "-$80.00"},
				{ID: "zero", Label: "Flat", Value: Num(0), Text: "$0.00"},
				{ID: "gone", Label: "No quote", Value: NoValue},
			},
		}
	}
	return subject{
		name: "bars", tag: &chart, end: Selection{Item: "gone"},
		items: []Selection{{Item: "long"}, {Item: "loss"}, {Item: "zero"}, {Item: "gone"}},
		draw: func(gtx layout.Context, view ui.View, sel Selection) (layout.Dimensions, []Event) {
			return chart.Layout(gtx, view, build(sel))
		},
		scan: downTheLeftColumn,
	}
}

func init() { registry = append(registry, barsSubject()) }

func TestBarChartNoticeForEmpty(t *testing.T) {
	var chart BarChart
	size, events := newRig(320, false).frame(func(gtx layout.Context, v ui.View, _ Selection) (layout.Dimensions, []Event) {
		return chart.Layout(gtx, v, BarSpec{Messages: Messages{Empty: "No data"}})
	}, Selection{})
	if size.X <= 0 || size.Y <= 0 || len(events) != 0 {
		t.Fatalf("size %v events %v", size, events)
	}
}

func TestToneColorsAreDistinctInBothThemes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		c := testPalette(dark)
		gain, loss, flat, warn := toneColor(c, ToneAuto, 1), toneColor(c, ToneAuto, -1), toneColor(c, ToneAuto, 0), toneColor(c, ToneWarning, 0)
		seen := map[any]string{}
		for name, col := range map[string]any{"gain": gain, "loss": loss, "flat": flat, "warn": warn} {
			if prior, dup := seen[col]; dup {
				t.Fatalf("dark=%v: %s and %s share a color", dark, name, prior)
			}
			seen[col] = name
		}
		if toneColor(c, ToneLoss, 5) != loss || toneColor(c, ToneGain, -5) != gain {
			t.Fatalf("dark=%v: explicit tone must override the sign", dark)
		}
	}
	if testPalette(true).Success == testPalette(false).Success {
		t.Fatal("light and dark success colors should differ")
	}
}
