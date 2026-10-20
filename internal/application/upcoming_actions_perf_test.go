//go:build perf

package application

import (
	"context"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func BenchmarkUpcomingActionsScale(b *testing.B) {
	spy := &upcomingSnapshotSpy{snapshot: upcomingScaleSnapshot()}
	s := NewService(spy)
	s.SetClock(func() time.Time { return time.Date(2026, 10, 11, 4, 0, 0, 0, time.UTC) })
	ctx := context.Background()
	o, snapshot, sources, e := s.prepareLiquidity(ctx, LiquidityOverviewQuery{}, s.clock())
	if e != nil {
		b.Fatal(e)
	}
	b.Run("projection", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, e := domain.ProjectUpcomingActions(o, sources, snapshot); e != nil {
				b.Fatal(e)
			}
		}
	})
	b.Run("endpoint", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, e := s.UpcomingActions(ctx); e != nil {
				b.Fatal(e)
			}
		}
	})
	b.Run("overview", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, e := s.LiquidityOverview(ctx, LiquidityOverviewQuery{}); e != nil {
				b.Fatal(e)
			}
		}
	})
}

// Timing ratios are a report, not a noisy CI correctness assertion.
func TestUpcomingActionsPerformanceReport(t *testing.T) {
	spy := &upcomingSnapshotSpy{snapshot: upcomingScaleSnapshot()}
	s := NewService(spy)
	s.SetClock(func() time.Time { return time.Date(2026, 10, 11, 4, 0, 0, 0, time.UTC) })
	ctx := context.Background()
	o, snapshot, sources, e := s.prepareLiquidity(ctx, LiquidityOverviewQuery{}, s.clock())
	if e != nil {
		t.Fatal(e)
	}
	measure := func(fn func() error) time.Duration {
		for i := 0; i < 3; i++ {
			if e := fn(); e != nil {
				t.Fatal(e)
			}
		}
		samples := []time.Duration{}
		for i := 0; i < 20; i++ {
			start := time.Now()
			if e := fn(); e != nil {
				t.Fatal(e)
			}
			samples = append(samples, time.Since(start))
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		return samples[18]
	}
	projection := measure(func() error { _, e := domain.ProjectUpcomingActions(o, sources, snapshot); return e })
	endpoint := measure(func() error { _, e := s.UpcomingActions(ctx); return e })
	overview := measure(func() error { _, e := s.LiquidityOverview(ctx, LiquidityOverviewQuery{}); return e })
	t.Logf("%s/%s %s; 1000 sources / 2000 reservations / 500 products; warmed 20-run P95 projection=%s endpoint=%s overview=%s endpoint review threshold=%s", runtime.GOOS, runtime.GOARCH, runtime.Version(), projection, endpoint, overview, time.Duration(float64(overview)*1.25)+50*time.Millisecond)
}
