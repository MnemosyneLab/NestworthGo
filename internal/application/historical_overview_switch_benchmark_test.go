package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type historicalSwitchMeasurements struct {
	elapsed, finalLatency, obsoleteExit, dbBusy, dbWait time.Duration
	maxInUse, cancelled, maxPending                     int
}

// Same application/SQLite workload in both arms. The baseline deliberately
// reproduces the old UI's missing cancellation; the fixed arm cancels each
// obsolete request without waiting for it before launching the next selection.
// Fixture creation is excluded. This is not native IPC or a GUI benchmark.
func runHistoricalSwitchBurst(t testing.TB, s *Service, db *sqlite.DB, count int, compare string, supersede bool) historicalSwitchMeasurements {
	t.Helper()
	type reply struct {
		index int
		out   HistoricalOverviewResult
		err   error
		at    time.Time
	}
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Minute)
	defer stop()
	before := db.SQL.Stats()
	var result historicalSwitchMeasurements
	var sampler sync.WaitGroup
	sampler.Add(1)
	sampleStop := make(chan struct{})
	go func() {
		defer sampler.Done()
		last := time.Now()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				stats := db.SQL.Stats()
				result.maxInUse = max(result.maxInUse, stats.InUse)
				if stats.InUse > 0 {
					result.dbBusy += now.Sub(last)
				}
				last = now
			case <-sampleStop:
				return
			}
		}
	}()
	var stopOnce sync.Once
	stopSampling := func() {
		stopOnce.Do(func() { close(sampleStop) })
		sampler.Wait()
	}
	defer stopSampling()
	starts, cancelledAt := make([]time.Time, count), make([]time.Time, count)
	cancels := make([]context.CancelFunc, count)
	replies := make(chan reply, count)
	start := time.Now()
	var pendingMu sync.Mutex
	pending, maxPending := 0, 0
	for n := 0; n < count; n++ {
		if n > 0 {
			timer := time.NewTimer(20 * time.Millisecond)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				t.Fatal(ctx.Err())
			}
			if supersede {
				cancelledAt[n-1] = time.Now()
				cancels[n-1]()
			}
		}
		requestCtx, cancel := context.WithCancel(ctx)
		cancels[n] = cancel
		defer cancel()
		starts[n] = time.Now()
		pendingMu.Lock()
		pending++
		maxPending = max(maxPending, pending)
		pendingMu.Unlock()
		go func(index int) {
			out, err := s.HistoricalOverview(requestCtx, fmt.Sprintf("2026-08-%02d", index+1), compare)
			at := time.Now()
			pendingMu.Lock()
			pending--
			pendingMu.Unlock()
			replies <- reply{index, out, err, at}
		}(n)
	}
	for n := 0; n < count; n++ {
		var received reply
		select {
		case received = <-replies:
		case <-ctx.Done():
			t.Fatal("burst did not drain: ", ctx.Err())
		}
		if received.index < count-1 && supersede {
			if !errors.Is(received.err, context.Canceled) || !reflect.DeepEqual(received.out, HistoricalOverviewResult{}) {
				t.Fatalf("obsolete request %d returned partial/successful data: err=%v", received.index, received.err)
			}
			result.cancelled++
			result.obsoleteExit = max(result.obsoleteExit, received.at.Sub(cancelledAt[received.index]))
			continue
		}
		if received.err != nil {
			t.Fatal(received.err)
		}
		out := received.out
		if out.Left.Assets == nil || *out.Left.Assets != "0.108" || len(out.Rows) != 1100 || !out.Left.Complete || out.Left.Date != fmt.Sprintf("2026-08-%02d", received.index+1) {
			t.Fatalf("wrong full dense result: date=%s rows=%d assets=%v", out.Left.Date, len(out.Rows), out.Left.Assets)
		}
		if compare != "" && (out.Right == nil || out.Right.Date != compare || out.Change == nil || out.Change.NetWorth == nil || *out.Change.NetWorth != "0") {
			t.Fatal("wrong comparison endpoint/change")
		}
		if received.index == count-1 {
			result.finalLatency = received.at.Sub(starts[count-1])
		}
	}
	result.elapsed = time.Since(start)
	result.maxPending = maxPending // All goroutines have completed before this read.
	result.dbWait = db.SQL.Stats().WaitDuration - before.WaitDuration
	// Stop sampling before copying the metrics written by the sampler.
	stopSampling()
	if db.SQL.Stats().InUse != 0 {
		t.Fatal("burst retained the database connection")
	}
	return result
}

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
func TestHistoricalOverviewCancellationReleasesConnection(t *testing.T) {
	s, db := seriesHistoryScaleFixture(t, 2, 0, 8300)
	for _, compare := range []string{"", "2026-08-31", "current"} {
		t.Run(compare, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				out, err := s.HistoricalOverview(ctx, "2026-08-01", compare)
				if !reflect.DeepEqual(out, HistoricalOverviewResult{}) {
					err = fmt.Errorf("cancelled read returned partial data: %v", err)
				}
				done <- err
			}()
			waitForHistoricalConnection(t, db)
			start := time.Now()
			cancel()
			probeCtx, probeCancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer probeCancel()
			var one int
			if err := db.SQL.QueryRowContext(probeCtx, "SELECT 1").Scan(&one); err != nil || one != 1 {
				t.Fatalf("cancelled read kept the connection: %v", err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-probeCtx.Done():
				t.Fatal("cancelled application work did not stop promptly")
			}
			t.Logf("cancel to SQL waiter completion and application exit: %s; inUse=%d", time.Since(start), db.SQL.Stats().InUse)
			final, err := s.HistoricalOverview(t.Context(), "2026-08-02", compare)
			if err != nil {
				t.Fatal(err)
			}
			wantOverviewAmount(t, final.Left.Assets, "0.00216")
		})
	}
}

func waitForHistoricalConnection(t *testing.T, db *sqlite.DB) {
	t.Helper()
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for db.SQL.Stats().InUse == 0 {
		select {
		case <-ticker.C:
		case <-timeout.C:
			t.Fatal("historical request never occupied the connection")
		}
	}
}

func TestHistoricalOverviewDenseRapidSwitchEvidence(t *testing.T) {
	if os.Getenv("NW_HISTORICAL_SWITCH_EVIDENCE") != "1" {
		t.Skip("opt-in dense rapid-switch measurement; also available as a benchmark")
	}
	s, db := seriesBenchmarkFixture(t, true)
	for _, compare := range []string{"", "2026-08-31", "current"} {
		metrics := runHistoricalSwitchBurst(t, s, db, 10, compare, true)
		t.Logf("comparison=%q requests=10 cancel=true elapsed=%s final=%s maxCancelExit=%s sampledDBBusy=%s DBWait=%s maxInUse=%d maxPending=%d cancelled=%d", compare, metrics.elapsed, metrics.finalLatency, metrics.obsoleteExit, metrics.dbBusy, metrics.dbWait, metrics.maxInUse, metrics.maxPending, metrics.cancelled)
	}
}
