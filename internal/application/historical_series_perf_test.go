//go:build perf

package application

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestHistoricalSeriesMemoryEvidence(t *testing.T) {
	for _, dense := range []bool{false, true} {
		s, _ := seriesBenchmarkFixture(t, dense)
		for _, days := range []int{1, 7, 31} {
			runtime.GC()
			var baseline runtime.MemStats
			runtime.ReadMemStats(&baseline)
			stop, peak := make(chan struct{}), make(chan uint64)
			go func() {
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				max := baseline.HeapAlloc
				for {
					var current runtime.MemStats
					runtime.ReadMemStats(&current)
					if current.HeapAlloc > max {
						max = current.HeapAlloc
					}
					select {
					case <-ticker.C:
					case <-stop:
						peak <- max
						return
					}
				}
			}()
			start := time.Now()
			out, err := s.HistoricalOverviewSeries(context.Background(), "2026-08-01", fmt.Sprintf("2026-08-%02d", days))
			close(stop)
			maximum := <-peak
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("dense=%t days=%d elapsed=%s baselineHeap=%d sampledPeakHeap=%d addedHeap=%d facts=%d queries=%d", dense, days, time.Since(start), baseline.HeapAlloc, maximum, maximum-baseline.HeapAlloc, out.Work.CandidateFacts, out.Work.Queries)
		}
	}
}

func seriesBenchmarkFixture(t testing.TB, dense bool) (*Service, *sqlite.DB) {
	backfills, activities := 0, 0
	if dense {
		backfills, activities = 1000, 25000
	}
	return seriesHistoryScaleFixture(t, 100, backfills, activities)
}

func BenchmarkHistoricalSeriesBounded(b *testing.B) {
	for _, dense := range []bool{false, true} {
		b.Run(fmt.Sprintf("dense=%t", dense), func(b *testing.B) {
			s, _ := seriesBenchmarkFixture(b, dense)
			for _, days := range []int{1, 7, 31} {
				b.Run(fmt.Sprintf("days=%d", days), func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						out, err := s.HistoricalOverviewSeries(context.Background(), "2026-08-01", time.Date(2026, 8, days, 0, 0, 0, 0, time.UTC).Format("2006-01-02"))
						if err != nil {
							b.Fatal(err)
						}
						b.ReportMetric(float64(out.Work.Queries), "queries/op")
						b.ReportMetric(float64(out.Work.CandidateFacts), "facts/op")
						b.ReportMetric(float64(out.Work.CandidateBytes), "input-B/op")
						if out.Work.Leaves != 1000 {
							b.Fatalf("leaves=%d", out.Work.Leaves)
						}
					}
				})
			}
		})
	}
}

func TestHistoricalSeriesBenchmarkCancellationEvidence(t *testing.T) {
	s, _ := seriesBenchmarkFixture(t, true)
	runtime.GC()
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(10*time.Millisecond, cancel)
	defer timer.Stop()
	start := time.Now()
	_, err := s.HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-31")
	elapsed := time.Since(start)
	if err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("slow cancellation %s", elapsed)
	}
	t.Logf("dense cancellation latency=%s (cancel scheduled at 10ms)", elapsed)
}
