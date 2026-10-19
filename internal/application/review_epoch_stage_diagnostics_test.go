package application

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type reviewPerfStageRepository struct {
	*sqlite.Repository
	metrics                            *reviewSQLStats
	capture, prepareAdmission, publish time.Duration
	captureEnd                         time.Time
	captureSQL, publishSQL             reviewSQLSnapshot
	captureCalls, publishCalls, reused int
}

func (r *reviewPerfStageRepository) CaptureReviewProjection(ctx context.Context, id domain.HouseholdID, at time.Time) (domain.ReviewProjectionCapture, error) {
	before := r.metrics.totals()
	start := time.Now()
	capture, err := r.Repository.CaptureReviewProjection(ctx, id, at)
	r.captureEnd = time.Now()
	r.capture += r.captureEnd.Sub(start)
	r.captureSQL = reviewPerfSQLDifference(r.metrics.totals(), before)
	r.captureCalls++
	r.reused = len(capture.ReusableDailyEvidence)
	return capture, err
}

func (r *reviewPerfStageRepository) PublishReviewProjection(ctx context.Context, publication domain.ReviewPublication, at time.Time) error {
	start := time.Now()
	r.prepareAdmission += start.Sub(r.captureEnd)
	before := r.metrics.totals()
	err := r.Repository.PublishReviewProjection(ctx, publication, at)
	r.publish += time.Since(start)
	r.publishSQL = reviewPerfSQLDifference(r.metrics.totals(), before)
	r.publishCalls++
	return err
}

// O(1), no statement-map copy/sort or heap allocation inside measured stages.
func (s *reviewSQLStats) totals() reviewSQLSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return reviewSQLSnapshot{queries: s.queries, rows: s.rows, queryTime: s.queryTime, execs: s.execs, writes: s.writes, affected: s.affected, execTime: s.execTime, prepares: s.prepares, prepareTime: s.prepareTime}
}

func reviewPerfSQLDifference(a, b reviewSQLSnapshot) reviewSQLSnapshot {
	return reviewSQLSnapshot{queries: a.queries - b.queries, rows: a.rows - b.rows, queryTime: a.queryTime - b.queryTime, execs: a.execs - b.execs, writes: a.writes - b.writes, affected: a.affected - b.affected, execTime: a.execTime - b.execTime, prepares: a.prepares - b.prepares, prepareTime: a.prepareTime - b.prepareTime}
}

func (r *reviewPerfStageRepository) log(t *testing.T, path, sample int, elapsed time.Duration, before, after runtime.MemStats) {
	t.Helper()
	if r.captureCalls != 1 || r.publishCalls != 1 {
		t.Fatalf("immutable fixture retried: capture=%d publish=%d", r.captureCalls, r.publishCalls)
	}
	var window reviewSQLSnapshot
	for _, statement := range r.metrics.snapshot(0).statements {
		if strings.HasPrefix(statement.query, "WITH ranked_keys AS") || strings.HasPrefix(statement.query, "WITH requested(instrument_id)") {
			window.queries += statement.queries
			window.rows += statement.rows
			window.queryTime += statement.queryTime
		}
	}
	if window.rows != int64(r.reused) {
		t.Fatalf("window metric lost rows: measured=%d reused=%d", window.rows, r.reused)
	}
	t.Logf("stage path=%d sample=%d total_ns=%d capture_ns=%d prepare_admission_ns=%d publish_ns=%d residual_ns=%d window_ns=%d window_queries=%d window_rows=%d reused=%d gc_cycles=%d gc_pause_ns=%d heap_before=%d heap_after=%d allocated_bytes=%d allocations=%d", path, sample, elapsed.Nanoseconds(), r.capture.Nanoseconds(), r.prepareAdmission.Nanoseconds(), r.publish.Nanoseconds(), (elapsed - r.capture - r.prepareAdmission - r.publish).Nanoseconds(), window.queryTime.Nanoseconds(), window.queries, window.rows, r.reused, after.NumGC-before.NumGC, after.PauseTotalNs-before.PauseTotalNs, before.HeapAlloc, after.HeapAlloc, after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs)
	t.Logf("stage SQL path=%d sample=%d capture={%s} publish={%s}", path, sample, r.captureSQL.summary(), r.publishSQL.summary())
}

var _ Repository = (*reviewPerfStageRepository)(nil)
var _ ReviewProjectionRepository = (*reviewPerfStageRepository)(nil)
