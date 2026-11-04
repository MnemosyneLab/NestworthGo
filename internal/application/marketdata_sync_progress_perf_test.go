//go:build perf

package application

import (
	"fmt"
	"testing"
	"time"
)

var progressBenchmarkResult SyncJobProgress
var snapshotBenchmarkResult SyncJobSnapshot

// Retained synthetic job only: compares getter allocation, excluding provider,
// database, lock contention and MCP serialization. No latency-percentile claim.
func BenchmarkSyncJobPollingProjection(b *testing.B) {
	for _, rows := range []int{1, 4096} {
		at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
		service := &Service{syncJobs: map[string]*syncJobState{"job": {snapshot: SyncJobSnapshot{JobID: "job", NextEligibilityAt: &at, Items: make([]SyncItemProgress, rows), Blockers: make([]SyncBlocker, rows), Prerequisites: make([]SyncBlocker, rows)}}}}
		b.Run(fmt.Sprintf("full_clone/%d_per_array", rows), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				snapshotBenchmarkResult, _ = service.GetSyncJob("job")
			}
		})
		b.Run(fmt.Sprintf("progress/%d_per_array", rows), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				progressBenchmarkResult, _ = service.GetSyncJobProgress("job")
			}
		})
	}
}
