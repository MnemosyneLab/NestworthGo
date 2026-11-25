package charts

import (
	"math"
	"time"

	"gioui.org/f32"
)

// This file holds the pure coordinate math shared by the charts. Nothing here
// touches Gio operations, so every mapping can be tested with plain numbers.

// scale maps a value in a domain to a pixel coordinate by linear
// interpolation. The range may run backwards, as a vertical axis does.
type scale struct{ d0, d1, r0, r1 float64 }

func (s scale) at(v float64) float64 {
	if s.d1 == s.d0 {
		return (s.r0 + s.r1) / 2
	}
	return s.r0 + (v-s.d0)/(s.d1-s.d0)*(s.r1-s.r0)
}

func (s scale) invert(px float64) float64 {
	if s.r1 == s.r0 {
		return s.d0
	}
	return s.d0 + (px-s.r0)/(s.r1-s.r0)*(s.d1-s.d0)
}

// paddedDomain widens a degenerate domain so a constant series still has a
// visible axis. A non-degenerate domain is returned unchanged.
func paddedDomain(lo, hi float64) (float64, float64) {
	if lo < hi {
		return lo, hi
	}
	delta := math.Max(math.Abs(lo)*0.05, 1)
	return lo - delta, hi + delta
}

// niceTicks returns round tick values that cover [lo, hi] with at most
// maxTicks entries. The first and last tick bound the data, so the plotted
// domain is [ticks[0], ticks[len-1]]. The step is 1, 2, 2.5 or 5 times a power
// of ten.
func niceTicks(lo, hi float64, maxTicks int) []float64 {
	lo, hi = paddedDomain(lo, hi)
	maxTicks = max(2, maxTicks)
	raw := (hi - lo) / float64(maxTicks-1)
	step := niceStep(raw)
	for {
		first, last := math.Floor(lo/step), math.Ceil(hi/step)
		count := int(last-first) + 1
		if count <= maxTicks || count <= 2 {
			ticks := make([]float64, 0, count)
			for k := first; k <= last; k++ {
				ticks = append(ticks, cleanTick(k*step, step))
			}
			return ticks
		}
		step = niceStep(step * 1.01)
	}
}

func niceStep(raw float64) float64 {
	magnitude := math.Pow(10, math.Floor(math.Log10(raw)))
	fraction := raw / magnitude
	for _, candidate := range [...]float64{1, 2, 2.5, 5, 10} {
		if fraction <= candidate*(1+1e-9) {
			return candidate * magnitude
		}
	}
	return 10 * magnitude
}

// cleanTick removes the binary noise of k*step, so 0.1*3 prints as 0.3.
func cleanTick(v, step float64) float64 {
	digits := int(math.Max(0, math.Ceil(-math.Log10(step)))) + 2
	factor := math.Pow(10, float64(digits))
	cleaned := math.Round(v*factor) / factor
	if cleaned == 0 {
		return 0
	}
	return cleaned
}

// timeStep is one calendar interval a time axis may advance by.
type timeStep struct {
	days, months int
}

var timeSteps = [...]timeStep{
	{days: 1}, {days: 2}, {days: 7}, {days: 14},
	{months: 1}, {months: 2}, {months: 3}, {months: 6},
	{months: 12}, {months: 24}, {months: 60}, {months: 120}, {months: 240}, {months: 600},
}

// timeTicks returns calendar-aligned ticks inside [from, to], at most maxTicks
// of them. Dates are interpreted in UTC, as civil dates are carried as UTC
// midnight. A span shorter than a day yields its two end points.
func timeTicks(from, to time.Time, maxTicks int) []time.Time {
	from, to = from.UTC(), to.UTC()
	if !to.After(from) {
		return []time.Time{from}
	}
	if to.Sub(from) < 24*time.Hour {
		return []time.Time{from, to}
	}
	maxTicks = max(1, maxTicks)
	var last []time.Time
	for _, step := range timeSteps {
		ticks := ticksFor(from, to, step)
		if len(ticks) == 0 {
			continue
		}
		last = ticks
		if len(ticks) <= maxTicks {
			return ticks
		}
	}
	if len(last) > maxTicks {
		last = last[:maxTicks]
	}
	return last
}

func ticksFor(from, to time.Time, step timeStep) []time.Time {
	var ticks []time.Time
	if step.days > 0 {
		day := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
		if day.Before(from) {
			day = day.AddDate(0, 0, 1)
		}
		if step.days == 7 || step.days == 14 {
			// Align weekly ticks to Monday so the labels share a weekday.
			for day.Weekday() != time.Monday {
				day = day.AddDate(0, 0, 1)
			}
		}
		for ; !day.After(to); day = day.AddDate(0, 0, step.days) {
			ticks = append(ticks, day)
		}
		return ticks
	}
	// Month steps align to multiples of the step counted from year zero, so a
	// three-month axis lands on January, April, July and October.
	month := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
	for (month.Year()*12+int(month.Month())-1)%step.months != 0 || month.Before(from) {
		month = month.AddDate(0, 1, 0)
		if month.After(to) {
			return nil
		}
	}
	for ; !month.After(to); month = month.AddDate(0, step.months, 0) {
		ticks = append(ticks, month)
	}
	return ticks
}

// dashPolyline splits a polyline into drawn dashes of length on separated by
// gaps of length off, measured along the path. Gio strokes have no dash
// pattern, so a dashed edge is drawn as several short solid strokes.
func dashPolyline(points []f32.Point, on, off float32) [][]f32.Point {
	if len(points) < 2 || on <= 0 {
		return nil
	}
	if off <= 0 {
		return [][]f32.Point{points}
	}
	var dashes [][]f32.Point
	var current []f32.Point
	drawing := true
	remaining := on
	current = append(current, points[0])
	for i := 1; i < len(points); i++ {
		from, to := points[i-1], points[i]
		length := float32(math.Hypot(float64(to.X-from.X), float64(to.Y-from.Y)))
		if length == 0 {
			continue
		}
		travelled := float32(0)
		for length-travelled > remaining {
			travelled += remaining
			t := travelled / length
			at := f32.Pt(from.X+(to.X-from.X)*t, from.Y+(to.Y-from.Y)*t)
			if drawing {
				current = append(current, at)
				dashes = append(dashes, current)
				current = nil
				remaining = off
			} else {
				current = []f32.Point{at}
				remaining = on
			}
			drawing = !drawing
		}
		remaining -= length - travelled
		if drawing {
			current = append(current, to)
		}
	}
	if drawing && len(current) > 1 {
		dashes = append(dashes, current)
	}
	return dashes
}

// arcSpan is an angular interval of a ring in radians. Angles start at the top
// of the ring (12 o'clock) and grow clockwise.
type arcSpan struct{ start, end float64 }

func (a arcSpan) sweep() float64 { return a.end - a.start }

// ringShares converts values to arc spans around a full ring. Only finite,
// strictly positive values are drawn; the others receive an empty span at the
// current angle. The spans of drawn values cover exactly 2π together. The total
// is zero, and every span empty, when no value is drawable.
func ringShares(values []Value) (spans []arcSpan, total float64) {
	for _, v := range values {
		if v.Valid() && v.Number > 0 {
			total += v.Number
		}
	}
	spans = make([]arcSpan, len(values))
	if total <= 0 || math.IsInf(total, 0) {
		return spans, 0
	}
	angle := 0.0
	lastDrawn := -1
	for i, v := range values {
		spans[i] = arcSpan{start: angle, end: angle}
		if v.Valid() && v.Number > 0 {
			angle += v.Number / total * 2 * math.Pi
			spans[i].end = angle
			lastDrawn = i
		}
	}
	// Floating-point sums can leave a hairline gap at the seam.
	if lastDrawn >= 0 {
		spans[lastDrawn].end = 2 * math.Pi
	}
	return spans, total
}

// ringAngle returns the clockwise angle from the top of the ring to point p
// around center, in [0, 2π).
func ringAngle(center, p f32.Point) float64 {
	angle := math.Atan2(float64(p.Y-center.Y), float64(p.X-center.X)) + math.Pi/2
	if angle < 0 {
		angle += 2 * math.Pi
	}
	if angle >= 2*math.Pi {
		angle -= 2 * math.Pi
	}
	return angle
}

// ringPoint is the point at a clockwise angle from the top, at radius r.
func ringPoint(center f32.Point, r float64, angle float64) f32.Point {
	return f32.Pt(center.X+float32(r*math.Sin(angle)), center.Y-float32(r*math.Cos(angle)))
}

// finite reports whether v is neither NaN nor infinite.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
