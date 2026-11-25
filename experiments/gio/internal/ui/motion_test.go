package ui

import (
	"math"
	"testing"
	"time"

	"gioui.org/layout"
)

func TestEasingEndpointsAndMonotonic(t *testing.T) {
	for name, ease := range map[string]Easing{"out": EaseOut, "inOut": EaseInOut, "linear": Linear} {
		if ease(0) != 0 || ease(1) != 1 || ease(-1) != 0 || ease(2) != 1 {
			t.Errorf("%s does not clamp to 0 and 1", name)
		}
		last := 0.0
		for i := 1; i <= 100; i++ {
			v := ease(float64(i) / 100)
			if v < last-1e-9 || v < 0 || v > 1+1e-9 {
				t.Fatalf("%s is not monotonic in [0,1] at %d: %v after %v", name, i, v, last)
			}
			last = v
		}
	}
	// ease-out starts fast: it is well past the linear value early on.
	if EaseOut(0.25) < 0.4 {
		t.Fatalf("ease-out(0.25) = %v, expected a fast start", EaseOut(0.25))
	}
	// Reference from an independent bisection of cubic-bezier(0.4, 0, 0.2, 1).
	if got := EaseInOut(0.5); math.Abs(got-0.77556) > 1e-4 {
		t.Fatalf("ease-in-out(0.5) = %v, want 0.77556", got)
	}
}

func TestTweenSnapsFirstThenAnimatesAndRetargets(t *testing.T) {
	now := time.Unix(100, 0)
	gtx := layout.Context{Now: now}
	var tween Tween
	tween.Set(gtx, 0, DurationBase, Linear)
	if tween.Value(gtx) != 0 || tween.Running(gtx) {
		t.Fatal("the first Set must snap")
	}
	tween.Set(gtx, 1, 100*time.Millisecond, Linear)
	if tween.Value(gtx) != 0 || !tween.Running(gtx) {
		t.Fatal("a new target must start from the current value")
	}
	gtx.Now = now.Add(50 * time.Millisecond)
	if got := tween.Value(gtx); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("halfway value = %v", got)
	}
	// Reverse mid-flight: continue from 0.5, not from the old start.
	tween.Set(gtx, 0, 100*time.Millisecond, Linear)
	if got := tween.Value(gtx); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("retarget jumped to %v", got)
	}
	gtx.Now = now.Add(200 * time.Millisecond)
	if tween.Value(gtx) != 0 || tween.Running(gtx) {
		t.Fatal("the tween did not settle on its target")
	}
	// Repeating the same target must not restart it.
	tween.Set(gtx, 0, DurationBase, Linear)
	if tween.Running(gtx) {
		t.Fatal("an unchanged target restarted the tween")
	}
}

func TestReducedMotionJumpsToTarget(t *testing.T) {
	SetReducedMotion(true)
	t.Cleanup(func() { SetReducedMotion(false) })
	gtx := layout.Context{Now: time.Unix(1, 0)}
	var tween Tween
	tween.Set(gtx, 0, DurationLayer, EaseOut)
	tween.Set(gtx, 1, DurationLayer, EaseOut)
	if tween.Value(gtx) != 1 || tween.Running(gtx) {
		t.Fatal("reduced motion must not animate")
	}
}

func TestShadowBandsReproduceGaussianEdge(t *testing.T) {
	const peak = 0.24
	sigma := 12.0
	bands := shadowBands(sigma, 16, peak)
	if len(bands) == 0 {
		t.Fatal("no bands")
	}
	cumulative := 0.0
	previousGrow := math.Inf(1)
	for _, band := range bands {
		if band.alpha <= 0 || band.alpha >= peak {
			t.Fatalf("band alpha %v outside (0, peak)", band.alpha)
		}
		if band.grow >= previousGrow {
			t.Fatal("bands must shrink from the outside in")
		}
		previousGrow = band.grow
		cumulative = 1 - (1-cumulative)*(1-band.alpha)
	}
	// Deep inside the edge the shadow reaches nearly its peak opacity.
	if cumulative > peak || cumulative < peak*0.95 {
		t.Fatalf("interior alpha %v, want just under %v", cumulative, peak)
	}
}
