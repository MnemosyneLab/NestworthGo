package application

import (
	"reflect"
	"testing"
	"time"
)

func TestSyncJobProgressExactCountersAndDetachedTime(t *testing.T) {
	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	job := &syncJobState{snapshot: SyncJobSnapshot{
		JobID: "old-job", Sequence: 17, Phase: "verify", Outcome: "partial", EstimatedRequests: 33, CompletedRequests: 22, TargetCount: 11, CompletedTargets: 10,
		SnapshotDaysPlanned: 14, SnapshotDaysRebuilt: 13, CommittedBatches: 9, ErrorCode: "synthetic-error", NextEligibilityAt: &at,
		Items: make([]SyncItemProgress, 4096), Blockers: make([]SyncBlocker, 2000), Prerequisites: make([]SyncBlocker, 1000),
	}}
	service := &Service{currentSyncID: "new-job", syncJobs: map[string]*syncJobState{"old-job": job, "nil-job": nil, "new-job": {snapshot: SyncJobSnapshot{JobID: "new-job", Sequence: 1}}}}
	got, ok := service.GetSyncJobProgress(" old-job ")
	want := SyncJobProgress{JobID: "old-job", Sequence: 17, Phase: "verify", Outcome: "partial", EstimatedRequests: 33, CompletedRequests: 22, TargetCount: 11, CompletedTargets: 10, SnapshotDaysPlanned: 14, SnapshotDaysRebuilt: 13, CommittedBatches: 9, ItemCount: 4096, BlockerCount: 2000, PrerequisiteCount: 1000, ErrorCode: "synthetic-error", NextEligibilityAt: &at}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong exact-job projection: %+v", got)
	}
	if got.NextEligibilityAt == job.snapshot.NextEligibilityAt {
		t.Fatal("shared mutable time")
	}
	*got.NextEligibilityAt = got.NextEligibilityAt.Add(time.Hour)
	if !job.snapshot.NextEligibilityAt.Equal(at) {
		t.Fatal("returned time modified retained job")
	}
	for _, id := range []string{"missing", "nil-job", ""} {
		if p, ok := service.GetSyncJobProgress(id); ok || !reflect.DeepEqual(p, SyncJobProgress{}) {
			t.Fatalf("%q guessed completion: %+v %v", id, p, ok)
		}
	}
	full, ok := service.GetSyncJob("old-job")
	if !ok || len(full.Items) != got.ItemCount || len(full.Blockers) != got.BlockerCount || len(full.Prerequisites) != got.PrerequisiteCount {
		t.Fatal("full evidence no longer available")
	}
	full.Items[0].Detail = "external edit"
	if job.snapshot.Items[0].Detail != "" {
		t.Fatal("full snapshot lost clone isolation")
	}
}
