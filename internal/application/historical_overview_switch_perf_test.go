//go:build perf

package application

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkHistoricalOverviewRapidSwitch(b *testing.B) {
	s, db := seriesBenchmarkFixture(b, true)
	for _, compare := range []string{"", "2026-08-31"} {
		for _, count := range []int{6, 10} {
			for _, supersede := range []bool{false, true} {
				b.Run(fmt.Sprintf("comparison=%t/requests=%d/cancel=%t", compare != "", count, supersede), func(b *testing.B) {
					var total, final, obsolete, busy, wait time.Duration
					var metrics historicalSwitchMeasurements
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						metrics = runHistoricalSwitchBurst(b, s, db, count, compare, supersede)
						total += metrics.elapsed
						final += metrics.finalLatency
						obsolete = max(obsolete, metrics.obsoleteExit)
						busy += metrics.dbBusy
						wait += metrics.dbWait
					}
					b.StopTimer()
					b.ReportMetric(float64(total.Microseconds())/1000/float64(b.N), "burst_ms/op")
					b.ReportMetric(float64(final.Microseconds())/1000/float64(b.N), "final_ms/op")
					b.ReportMetric(float64(obsolete.Microseconds())/1000, "max_cancel_exit_ms")
					b.ReportMetric(float64(busy.Microseconds())/1000/float64(b.N), "sampled_db_busy_ms/op")
					b.ReportMetric(float64(wait.Microseconds())/1000/float64(b.N), "db_wait_ms/op")
					b.ReportMetric(float64(metrics.maxInUse), "max_db_in_use")
					b.ReportMetric(float64(metrics.maxPending), "max_pending")
					b.ReportMetric(float64(metrics.cancelled), "cancelled/op")
				})
			}
		}
	}
}

// Mandatory smaller race-friendly fixture still exercises a live read with
// SQLite's actual one-connection pool, cancellation, and a queued SQL waiter.

func TestHistoricalOverviewDenseRapidSwitchEvidence(t *testing.T) {
	s, db := seriesBenchmarkFixture(t, true)
	for _, compare := range []string{"", "2026-08-31", "current"} {
		metrics := runHistoricalSwitchBurst(t, s, db, 10, compare, true)
		t.Logf("comparison=%q requests=10 cancel=true elapsed=%s final=%s maxCancelExit=%s sampledDBBusy=%s DBWait=%s maxInUse=%d maxPending=%d cancelled=%d", compare, metrics.elapsed, metrics.finalLatency, metrics.obsoleteExit, metrics.dbBusy, metrics.dbWait, metrics.maxInUse, metrics.maxPending, metrics.cancelled)
	}
}
