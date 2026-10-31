package application

import (
	"time"
)

func finishMobileLatest(job *MobileMarketJob, result RefreshResult, err error, now time.Time) {
	job.Result = result
	job.Total = len(result.Items)
	job.Completed = job.Total
	job.Current = nil
	job.Phase = "complete"
	job.Outcome = SyncOutcomeSucceeded
	failed := 0
	for _, item := range result.Items {
		if item.Status == RefreshFailed || item.Status == RefreshRateLimited {
			failed++
		}
	}
	if failed > 0 {
		job.Outcome = SyncOutcomePartial
	}
	if failed > 0 && failed == len(result.Items) {
		job.Outcome = SyncOutcomeFailed
	}
	if err != nil {
		job.Outcome = SyncOutcomeCancelled
	}
	job.FinishedAt = &now
}

func cloneMobileJob(job MobileMarketJob) MobileMarketJob {
	job.Items = append([]SyncItemProgress(nil), job.Items...)
	job.Result.Items = append([]RefreshTargetResult(nil), job.Result.Items...)
	job.Blockers = append([]SyncBlocker(nil), job.Blockers...)
	if job.Current != nil {
		item := *job.Current
		job.Current = &item
	}
	return job
}

// Read the same task snapshots used by desktop and MCP. No provider call or
// job admission happens here. Repair-all remains desktop-only for retries.
func (s *Service) sharedMarketJob(id string) (MobileMarketJob, bool) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	var latest *MobileMarketJob
	if s.latestSnapshot != nil && s.latestSnapshot.Instance == s.MobileWorkspace() && (id == "current" || s.latestSnapshot.ID == id) {
		copy := cloneMobileJob(*s.latestSnapshot)
		latest = &copy
	}
	syncID := id
	if id == "current" {
		syncID = s.currentSyncID
	}
	state := s.syncJobs[syncID]
	if state == nil || state.workspace != s.workspaceIdentity() || state.epoch != s.refreshEpoch.Load() {
		if latest == nil {
			return MobileMarketJob{}, false
		}
		return *latest, true
	}
	snap := state.snapshot.Clone()
	if latest != nil && (latest.Outcome == SyncOutcomeRunning || snap.Outcome != SyncOutcomeRunning && latest.StartedAt.After(state.startedAt)) {
		return *latest, true
	}
	r := MobileMarketRequest{Mode: "history", Scope: string(snap.Scope.Scope), InstrumentID: snap.Scope.InstrumentID, CurrencyA: snap.Scope.CurrencyA, CurrencyB: snap.Scope.CurrencyB}
	return MobileMarketJob{ID: snap.JobID, Instance: s.MobileWorkspace(), Request: r, Source: "shared", Retryable: r.Scope == "instrument" || r.Scope == "fx", Outcome: snap.Outcome, Phase: snap.Phase, StartedAt: state.startedAt, Completed: snap.CompletedRequests, Total: snap.EstimatedRequests, Current: snap.Current, Items: snap.Items, Blockers: snap.Blockers, ErrorCode: snap.ErrorCode, NextEligibilityAt: snap.NextEligibilityAt}, true
}

// Cancellation is an asynchronous request. Waiting while holding the write gate
// would deadlock a provider worker that is waiting to persist an accepted batch.
// Already persisted observations are retained, never rolled back.
func (s *Service) RequestCancelMobileMarket(id string) bool {
	s.mobileJobsMu.Lock()
	job := s.mobileJobs[id]
	if job == nil {
		s.mobileJobsMu.Unlock()
		return false
	}
	cancel, syncID := job.cancel, job.SyncID
	s.mobileJobsMu.Unlock()
	if syncID != "" {
		s.syncMu.Lock()
		state := s.syncJobs[syncID]
		if state != nil {
			cancel = state.cancel
		}
		s.syncMu.Unlock()
	}
	if cancel != nil {
		cancel()
		return true
	}
	return false
}
