package application

import (
	"context"
	"errors"
	"fmt"
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
