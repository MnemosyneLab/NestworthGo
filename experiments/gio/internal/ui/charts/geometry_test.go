package charts

import (
	"math"
	"testing"
	"time"

	"gioui.org/f32"
)

func TestScaleMapsBothDirections(t *testing.T) {
	up := scale{d0: 0, d1: 100, r0: 10, r1: 210}
	if got := up.at(50); got != 110 {
		t.Fatalf("at(50) = %v, want 110", got)
	}
	// Value axes run bottom-up: the largest value has the smallest pixel row.
	down := scale{d0: 100, d1: -100, r0: 0, r1: 400}
	if got := down.at(0); got != 200 {
		t.Fatalf("zero of a symmetric axis = %v, want the middle 200", got)
	}
	for _, v := range []float64{-100, -3.5, 0, 42, 100} {
		if got := down.invert(down.at(v)); math.Abs(got-v) > 1e-9 {
			t.Fatalf("invert(at(%v)) = %v", v, got)
		}
	}
	if got := (scale{d0: 5, d1: 5, r0: 0, r1: 100}).at(5); got != 50 {
		t.Fatalf("degenerate domain maps to %v, want the range middle", got)
	}
}

func TestNiceTicksCoverDataAndIncludeZero(t *testing.T) {
	cases := []struct {
		lo, hi float64
		max    int
	}{
		{0, 1_093_373, 6}, {-32_400, 120_000, 5}, {-0.04, 0.09, 4}, {7, 7, 4}, {-5, -1, 3},
	}
	for _, c := range cases {
		ticks := niceTicks(c.lo, c.hi, c.max)
		if len(ticks) < 2 || len(ticks) > c.max {
			t.Fatalf("niceTicks(%v, %v, %d) = %v, want 2..%d ticks", c.lo, c.hi, c.max, ticks, c.max)
		}
		lo, hi := paddedDomain(c.lo, c.hi)
		if ticks[0] > lo || ticks[len(ticks)-1] < hi {
			t.Fatalf("niceTicks(%v, %v) = %v does not cover the data", c.lo, c.hi, ticks)
		}
		for i := 1; i < len(ticks); i++ {
			if ticks[i] <= ticks[i-1] {
				t.Fatalf("ticks %v are not increasing", ticks)
			}
		}
		if lo < 0 && hi > 0 {
			hasZero := false
			for _, tick := range ticks {
				hasZero = hasZero || tick == 0
			}
			if !hasZero {
				t.Fatalf("ticks %v of a range that crosses zero lack an exact zero", ticks)
			}
		}
	}
}

func TestTimeTicksAreCalendarAlignedUTC(t *testing.T) {
	from := time.Date(2025, time.July, 3, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	ticks := timeTicks(from, to, 4)
	if len(ticks) == 0 || len(ticks) > 4 {
		t.Fatalf("got %d ticks, want 1..4: %v", len(ticks), ticks)
	}
	for _, tick := range ticks {
		if tick.Before(from) || tick.After(to) {
			t.Fatalf("tick %v is outside the viewport", tick)
		}
		if tick.Hour() != 0 || tick.Location() != time.UTC {
			t.Fatalf("tick %v is not UTC midnight", tick)
		}
	}
	// Dense capacity may pick months; each tick must then be the first of one.
	for _, tick := range timeTicks(from, to, 13) {
		if tick.Day() != 1 && tick.Weekday() != time.Monday {
			t.Fatalf("tick %v is aligned to neither a month nor a week", tick)
		}
	}
	if got := timeTicks(from, from, 5); len(got) != 1 {
		t.Fatalf("an empty span yields %v, want its single instant", got)
	}
	if got := timeTicks(from, from.Add(6*time.Hour), 5); len(got) != 2 {
		t.Fatalf("a sub-day span yields %v, want its two ends", got)
	}
}

func TestDashPolylineSplitsOnLength(t *testing.T) {
	dashes := dashPolyline([]f32.Point{f32.Pt(0, 0), f32.Pt(100, 0)}, 10, 10)
	if len(dashes) != 5 {
		t.Fatalf("got %d dashes, want 5: %v", len(dashes), dashes)
	}
	for i, d := range dashes {
		if got, want := d[0].X, float32(i*20); math.Abs(float64(got-want)) > 1e-3 {
			t.Fatalf("dash %d starts at %v, want %v", i, got, want)
		}
		if got := d[len(d)-1].X - d[0].X; math.Abs(float64(got)-10) > 1e-3 {
			t.Fatalf("dash %d is %v long, want 10", i, got)
		}
	}
	if got := dashPolyline([]f32.Point{f32.Pt(3, 3)}, 4, 4); len(got) != 0 {
		t.Fatalf("a single point produced %v", got)
	}
}

func TestRingSharesDrawOnlyPositiveValues(t *testing.T) {
	spans, total := ringShares([]Value{Num(30), Num(-500), NoValue, Num(0), Num(10), Num(math.NaN())})
	if total != 40 {
		t.Fatalf("total = %v, want 40: negatives, zero and missing never count", total)
	}
	if got := spans[0].sweep(); math.Abs(got-1.5*math.Pi) > 1e-9 {
		t.Fatalf("first sweep = %v, want 3π/2", got)
	}
	for _, i := range []int{1, 2, 3, 5} {
		if spans[i].sweep() != 0 {
			t.Fatalf("entry %d has an arc %v", i, spans[i])
		}
	}
	if spans[4].end != 2*math.Pi || spans[4].start != spans[0].end {
		t.Fatalf("last arc %v does not close the ring after %v", spans[4], spans[0])
	}
}

func TestRingSharesZeroTotalDrawsNothing(t *testing.T) {
	for name, values := range map[string][]Value{
		"empty": nil, "zeros": {Num(0), Num(0)}, "negatives": {Num(-1), Num(-2)}, "missing": {NoValue, NoValue},
		"infinite": {Num(math.Inf(1))},
	} {
		spans, total := ringShares(values)
		if total != 0 {
			t.Fatalf("%s: total %v, want 0", name, total)
		}
		for _, s := range spans {
			if s.sweep() != 0 {
				t.Fatalf("%s: drew %v", name, s)
			}
		}
	}
}

func TestRingAngleAndPointAreInverse(t *testing.T) {
	center := f32.Pt(100, 100)
	if got := ringAngle(center, f32.Pt(100, 40)); got != 0 {
		t.Fatalf("12 o'clock is %v, want 0", got)
	}
	if got := ringAngle(center, f32.Pt(160, 100)); math.Abs(got-math.Pi/2) > 1e-6 {
		t.Fatalf("3 o'clock is %v, want π/2", got)
	}
	for angle := 0.1; angle < 2*math.Pi; angle += 0.37 {
		p := ringPoint(center, 50, angle)
		if got := ringAngle(center, p); math.Abs(got-angle) > 1e-4 {
			t.Fatalf("angle %v came back as %v", angle, got)
		}
	}
}
