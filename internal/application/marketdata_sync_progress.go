package application

import (
	"strings"
	"time"
)

// SyncJobProgress is a coherent scalar projection of one process-local job.
// It excludes target evidence and internal scope/identity metadata. Counts do
// not imply delivered evidence or complete current financial data.
type SyncJobProgress struct {
	JobID               string
	Sequence            int
	Phase               string
	Outcome             string
	EstimatedRequests   int
	CompletedRequests   int
	TargetCount         int
	CompletedTargets    int
	SnapshotDaysPlanned int
	SnapshotDaysRebuilt int
	CommittedBatches    int
	ItemCount           int
	BlockerCount        int
	PrerequisiteCount   int
	ErrorCode           string
	NextEligibilityAt   *time.Time
}

// GetSyncJobProgress reads the exact requested job under the same lock as full
// snapshots. Work and allocation do not grow with retained evidence; the only
// pointer is detached before releasing the lock. Full evidence still uses
// GetSyncJob and its defensive Clone.
func (s *Service) GetSyncJobProgress(jobID string) (SyncJobProgress, bool) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	job, ok := s.syncJobs[strings.TrimSpace(jobID)]
	if !ok || job == nil {
		return SyncJobProgress{}, false
	}
	snap := &job.snapshot
	progress := SyncJobProgress{
		JobID: snap.JobID, Sequence: snap.Sequence, Phase: snap.Phase, Outcome: snap.Outcome,
		EstimatedRequests: snap.EstimatedRequests, CompletedRequests: snap.CompletedRequests,
		TargetCount: snap.TargetCount, CompletedTargets: snap.CompletedTargets,
		SnapshotDaysPlanned: snap.SnapshotDaysPlanned, SnapshotDaysRebuilt: snap.SnapshotDaysRebuilt,
		CommittedBatches: snap.CommittedBatches, ItemCount: len(snap.Items), BlockerCount: len(snap.Blockers), PrerequisiteCount: len(snap.Prerequisites),
		ErrorCode: snap.ErrorCode,
	}
	if snap.NextEligibilityAt != nil {
		at := *snap.NextEligibilityAt
		progress.NextEligibilityAt = &at
	}
	return progress, true
}
