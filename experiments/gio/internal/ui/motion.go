package ui

import (
	"math"
	"sync/atomic"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
)

// Easing maps linear progress in [0, 1] to eased progress.
type Easing func(float64) float64

var (
	// EaseOut is cubic-bezier(0, 0, 0.2, 1), Tailwind's ease-out, used by
	// layers that enter and by color transitions.
	EaseOut Easing = cubicBezier(0, 0, 0.2, 1)
	// EaseInOut is cubic-bezier(0.4, 0, 0.2, 1), Tailwind's default.
	EaseInOut Easing = cubicBezier(0.4, 0, 0.2, 1)
	// Linear is the identity within [0, 1].
	Linear Easing = func(t float64) float64 { return math.Max(0, math.Min(1, t)) }
)

// cubicBezier returns the CSS cubic-bezier easing for control points
// (x1, y1) and (x2, y2).
func cubicBezier(x1, y1, x2, y2 float64) Easing {
	sample := func(a1, a2, t float64) float64 {
		// Bernstein form with the end points fixed at 0 and 1.
		return 3*a1*t*(1-t)*(1-t) + 3*a2*t*t*(1-t) + t*t*t
	}
	slope := func(a1, a2, t float64) float64 {
		return 3*a1*(1-t)*(1-3*t) + 3*a2*t*(2-3*t) + 3*t*t
	}
	return func(x float64) float64 {
		switch {
		case x <= 0:
			return 0
		case x >= 1:
			return 1
		}
		// Solve sample(x1, x2, t) = x for t with Newton's method, falling back
		// to bisection when the slope is too flat to converge.
		t := x
		for range 8 {
			err := sample(x1, x2, t) - x
			if math.Abs(err) < 1e-6 {
				return sample(y1, y2, t)
			}
			d := slope(x1, x2, t)
			if math.Abs(d) < 1e-6 {
				break
			}
			t -= err / d
		}
		lo, hi := 0.0, 1.0
		t = x
		for range 32 {
			v := sample(x1, x2, t)
			if math.Abs(v-x) < 1e-6 {
				break
			}
			if v < x {
				lo = t
			} else {
				hi = t
			}
			t = (lo + hi) / 2
		}
		return sample(y1, y2, t)
	}
}

var reducedMotion atomic.Bool

// SetReducedMotion applies the operating system's reduce-motion preference.
// While set, every Tween jumps to its target and no frames are requested.
func SetReducedMotion(reduced bool) { reducedMotion.Store(reduced) }

// ReducedMotion reports whether animations are suppressed.
func ReducedMotion() bool { return reducedMotion.Load() }

// Tween animates one float toward a target. It is driven by gtx.Now and asks
// for another frame only while it is moving, so an idle window draws nothing.
//
// The first Set snaps to the target, so a control that appears already
// hovered or selected does not animate in. Retargeting mid-flight starts from
// the current value, which keeps rapid hover changes continuous.
type Tween struct {
	from, to float64
	start    time.Time
	duration time.Duration
	ease     Easing
	started  bool
}

// Set directs the tween toward target. Calling it every frame with an
// unchanged target is cheap and does not restart the animation.
func (t *Tween) Set(gtx layout.Context, target float64, duration time.Duration, ease Easing) {
	if !t.started {
		t.from, t.to, t.started = target, target, true
		return
	}
	if target == t.to {
		return
	}
	t.from = t.at(gtx.Now)
	t.to, t.start, t.duration, t.ease = target, gtx.Now, duration, ease
	if ease == nil {
		t.ease = EaseOut
	}
}

// Snap jumps to value without animating.
func (t *Tween) Snap(value float64) {
	t.from, t.to, t.started = value, value, true
}

// Value returns the current value and requests the next frame while running.
func (t *Tween) Value(gtx layout.Context) float64 {
	v := t.at(gtx.Now)
	if v != t.to {
		gtx.Execute(op.InvalidateCmd{})
	}
	return v
}

// Running reports whether the tween has not reached its target at gtx.Now.
func (t *Tween) Running(gtx layout.Context) bool { return t.at(gtx.Now) != t.to }

// Target is the value the tween is heading toward.
func (t *Tween) Target() float64 { return t.to }

func (t *Tween) at(now time.Time) float64 {
	if t.from == t.to || ReducedMotion() || t.duration <= 0 {
		return t.to
	}
	elapsed := now.Sub(t.start)
	if elapsed <= 0 {
		return t.from
	}
	if elapsed >= t.duration {
		return t.to
	}
	progress := t.ease(float64(elapsed) / float64(t.duration))
	return t.from + (t.to-t.from)*progress
}
