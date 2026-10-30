package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type syncLatestKey struct{}
type latestRefreshRun struct {
	key    string
	done   chan struct{}
	result RefreshResult
}

func refreshConflict(targets []refreshTarget) RefreshResult {
	r := RefreshResult{Items: make([]RefreshTargetResult, 0, len(targets))}
	for _, t := range targets {
		r.Items = append(r.Items, RefreshTargetResult{TargetKey: t.key, Kind: t.kind, Status: RefreshFailed, ErrorCode: domain.ErrConflict})
	}
	return r
}
func (s *Service) admitLatestRefresh(targets []refreshTarget) (*latestRefreshRun, bool, bool) {
	var keys []string
	for _, t := range targets {
		keys = append(keys, fmt.Sprintf("%s:%t:%t:%s:%s:%s", t.key, t.skip, t.cached, t.providerKey, t.baseCurrency, t.quoteCurrency))
	}
	key := strings.Join(keys, "\n")
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if job := s.syncJobs[s.currentSyncID]; job != nil && job.snapshot.Outcome == SyncOutcomeRunning {
		return nil, false, true
	}
	if s.latestRun != nil {
		return s.latestRun, false, s.latestRun.key != key
	}
	run := &latestRefreshRun{key: key, done: make(chan struct{})}
	s.latestRun = run
	return run, true, false
}

type MobileMarketRequest struct {
	Mode         string `json:"mode"` // latest or history; repair_all is never mobile history.
	Scope        string `json:"scope"`
	InstrumentID string `json:"instrumentId,omitempty"`
	CurrencyA    string `json:"currencyA,omitempty"`
	CurrencyB    string `json:"currencyB,omitempty"`
}

func (r MobileMarketRequest) sync() SyncRequest {
	return SyncRequest{Scope: SyncScopeKind(r.Scope), InstrumentID: r.InstrumentID, CurrencyA: r.CurrencyA, CurrencyB: r.CurrencyB}
}
func (r MobileMarketRequest) Validate() error {
	if r.Mode == "history" {
		if r.Scope != "instrument" && r.Scope != "fx" {
			return mobileInvalid("scope")
		}
		return validateSyncRequest(r.sync())
	}
	if r.Mode != "latest" {
		return mobileInvalid("mode")
	}
	switch r.Scope {
	case "all", "stale":
		return nil
	case "instrument":
		_, err := domain.ParseInstrumentID(r.InstrumentID)
		return err
	case "fx":
		a, err := domain.ParseSupportedCurrency(r.CurrencyA)
		if err != nil {
			return err
		}
		b, err := domain.ParseSupportedCurrency(r.CurrencyB)
		if err != nil {
			return err
		}
		_, _, err = domain.NormalizeFXPair(a, b)
		return err
	}
	return mobileInvalid("scope")
}

type MobileMarketJob struct {
	ID                string              `json:"jobId"`
	Instance          string              `json:"instance"`
	Request           MobileMarketRequest `json:"request"`
	Outcome           string              `json:"outcome"`
	Phase             string              `json:"phase"`
	StartedAt         time.Time           `json:"startedAt"`
	FinishedAt        *time.Time          `json:"finishedAt,omitempty"`
	Result            RefreshResult       `json:"result"`
	ErrorCode         string              `json:"errorCode,omitempty"`
	Completed         int                 `json:"completed"`
	Total             int                 `json:"total"`
	Current           *SyncItemProgress   `json:"current,omitempty"`
	Items             []SyncItemProgress  `json:"items,omitempty"`
	NextEligibilityAt *time.Time          `json:"nextEligibilityAt,omitempty"`
	SyncID            string              `json:"-"`
	cancel            context.CancelFunc
}

func (s *Service) PreviewMobileMarket(ctx context.Context, r MobileMarketRequest) (*SyncPlanPreview, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if r.Mode == "latest" {
		return nil, nil
	}
	p, err := s.PreviewMarketDataSync(ctx, r.sync())
	return &p, err
}

func (s *Service) StartMobileMarket(ctx context.Context, r MobileMarketRequest) (MobileMarketJob, bool, bool, error) {
	if err := r.Validate(); err != nil {
		return MobileMarketJob{}, false, false, err
	}
	s.mobileJobsMu.Lock()
	defer s.mobileJobsMu.Unlock()
	if s.mobileJobs == nil {
		s.mobileJobs = map[string]*MobileMarketJob{}
	}
	if current := s.mobileJobs[s.mobileJobCurrent]; current != nil && current.Outcome == SyncOutcomeRunning {
		if current.SyncID != "" {
			if sync, ok := s.GetSyncJob(current.SyncID); ok && sync.Outcome != SyncOutcomeRunning {
				current.Outcome = sync.Outcome
			}
		}
		if current.Outcome == SyncOutcomeRunning {
			return *current, current.Request == r, current.Request != r, nil
		}
	}
	job := &MobileMarketJob{ID: uuid.NewString(), Instance: s.MobileWorkspace(), Request: r, Outcome: SyncOutcomeRunning, Phase: "latest", StartedAt: s.clock()}
	if r.Mode == "history" {
		started, err := s.StartMarketDataSync(ctx, r.sync())
		if err != nil {
			return MobileMarketJob{}, false, false, err
		}
		if started.Conflict {
			return MobileMarketJob{}, false, true, nil
		}
		job.ID = started.Job.JobID
		job.SyncID = started.Job.JobID
		job.Phase = started.Job.Phase
		s.mobileJobs[job.ID] = job
		s.mobileJobCurrent = job.ID
		return *job, started.Attached, false, nil
	}
	// Admission here catches direct desktop/MCP sync or latest work before a
	// task is accepted. refreshTargets remains the shared provider race fence.
	s.syncMu.Lock()
	busy := s.latestRun != nil
	current := s.syncJobs[s.currentSyncID]
	busy = busy || (current != nil && current.snapshot.Outcome == SyncOutcomeRunning)
	s.syncMu.Unlock()
	if busy {
		return MobileMarketJob{}, false, true, nil
	}
	job.Total = 1
	if r.Scope == "all" || r.Scope == "stale" {
		snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
		if err != nil {
			return MobileMarketJob{}, false, false, err
		}
		targets := refreshTargetsForSnapshot(snapshot)
		if r.Scope == "all" {
			targets = append(targets, extraFXRefreshTargetsForSnapshot(snapshot)...)
		}
		job.Total = len(targets)
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	jobCtx = WithRefreshProgress(jobCtx, func(item SyncItemProgress) {
		s.mobileJobsMu.Lock()
		defer s.mobileJobsMu.Unlock()
		if item.Status == "running" {
			copy := item
			job.Current = &copy
		} else {
			job.Completed++
			job.Items = append(job.Items, item)
			job.Current = nil
		}
	})
	job.cancel = cancel
	s.mobileJobs[job.ID] = job
	s.mobileJobCurrent = job.ID
	// Retain a bounded process-local history. Pending jobs are never evicted.
	if len(s.mobileJobs) > 30 {
		for id, old := range s.mobileJobs {
			if old.Outcome != SyncOutcomeRunning && id != job.ID {
				delete(s.mobileJobs, id)
				break
			}
		}
	}
	s.mobileJobsWG.Add(1)
	go func() {
		defer s.mobileJobsWG.Done()
		defer cancel()
		var result RefreshResult
		var err error
		switch r.Scope {
		case "all":
			result, err = s.RefreshAll(jobCtx)
		case "stale":
			result, err = s.RefreshMissingOrStale(jobCtx)
		case "instrument":
			result, err = s.RefreshInstrument(jobCtx, domain.InstrumentID(r.InstrumentID))
		case "fx":
			result, err = s.RefreshFX(jobCtx, r.CurrencyA, r.CurrencyB)
		}
		s.mobileJobsMu.Lock()
		defer s.mobileJobsMu.Unlock()
		job.Result = result
		job.Total = len(result.Items)
		job.Completed = job.Total
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
		if failed == len(result.Items) && failed > 0 {
			job.Outcome = SyncOutcomeFailed
		}
		if err != nil {
			job.Outcome = SyncOutcomeFailed
			job.ErrorCode = "unavailable"
		}
		if jobCtx.Err() != nil {
			job.Outcome = SyncOutcomeCancelled
		}
		now := s.clock()
		job.FinishedAt = &now
	}()
	return *job, false, false, nil
}

func (s *Service) MobileMarketJob(id string) (MobileMarketJob, bool) {
	s.mobileJobsMu.Lock()
	defer s.mobileJobsMu.Unlock()
	if id == "current" {
		id = s.mobileJobCurrent
	}
	p := s.mobileJobs[id]
	if p == nil {
		return MobileMarketJob{}, false
	}
	out := *p
	out.Result.Items = append([]RefreshTargetResult(nil), p.Result.Items...)
	out.Items = append([]SyncItemProgress(nil), p.Items...)
	if p.Current != nil {
		copy := *p.Current
		out.Current = &copy
	}
	if p.SyncID != "" {
		if sync, ok := s.GetSyncJob(p.SyncID); ok {
			out.Outcome = sync.Outcome
			out.Phase = sync.Phase
			out.Total = sync.EstimatedRequests
			out.Completed = sync.CompletedRequests
			out.ErrorCode = sync.ErrorCode
			out.Items = sync.Items
			out.Current = sync.Current
			out.NextEligibilityAt = sync.NextEligibilityAt
		}
	}
	return out, true
}

func (s *Service) CancelMobileMarketAndWait() {
	s.mobileJobsMu.Lock()
	for _, job := range s.mobileJobs {
		if job.cancel != nil {
			job.cancel()
		}
	}
	s.mobileJobsMu.Unlock()
	s.mobileJobsWG.Wait()
}
