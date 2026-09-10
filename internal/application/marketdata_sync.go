package application

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	SyncPhaseScan                  = "scan"
	SyncPhasePlan                  = "plan"
	SyncPhaseHistoricalInstruments = "historical_instruments"
	SyncPhaseHistoricalFX          = "historical_fx"
	SyncPhaseLatestInstruments     = "latest_instruments"
	SyncPhaseLatestFX              = "latest_fx"
	SyncPhaseInvalidateSnapshots   = "invalidate_snapshots"
	SyncPhaseRebuildSnapshots      = "rebuild_snapshots"
	SyncPhaseVerify                = "verify"
	SyncPhaseComplete              = "complete"
	SyncPhaseFailed                = "failed"
	SyncPhaseCancelled             = "cancelled"

	SyncOutcomeRunning   = "running"
	SyncOutcomeSucceeded = "succeeded"
	SyncOutcomePartial   = "partial"
	SyncOutcomeFailed    = "failed"
	SyncOutcomeCancelled = "cancelled"

	SyncScopeRepairAll  SyncScopeKind = "repair_all"
	SyncScopeInstrument SyncScopeKind = "instrument"
	SyncScopeFX         SyncScopeKind = "fx"

	SyncEventStarted   = "started"
	SyncEventProgress  = "progress"
	SyncEventItem      = "item"
	SyncEventCompleted = "completed"

	DefaultHistoryRequestMaxDays = 366
	SyncMaxTransientAttempts     = 3
)

type SyncScopeKind string

// MarketDataSyncListener receives job snapshots. Events are hints to reread
// backend state; GetCurrentSyncJob / GetSyncJob are the record of truth.
type MarketDataSyncListener func(event string, snapshot SyncJobSnapshot)

// HistoryPersister commits qualified historical batches. sqlite implements it
// through appports so application never imports sqlite.
type HistoryPersister interface {
	PersistInstrumentHistory(context.Context, CommitInstrumentHistoryRequest) (CommitHistoryResult, error)
	PersistFXHistory(context.Context, CommitFXHistoryRequest) (CommitHistoryResult, error)
}

type SyncRequest struct {
	Scope        SyncScopeKind
	InstrumentID string
	CurrencyA    string
	CurrencyB    string
	ForceRecheck bool
}

func (r SyncRequest) Normalized() SyncRequest {
	out := r
	out.Scope = SyncScopeKind(strings.ToLower(strings.TrimSpace(string(r.Scope))))
	if out.Scope == "" {
		out.Scope = SyncScopeRepairAll
	}
	out.InstrumentID = strings.TrimSpace(r.InstrumentID)
	out.CurrencyA = strings.ToUpper(strings.TrimSpace(r.CurrencyA))
	out.CurrencyB = strings.ToUpper(strings.TrimSpace(r.CurrencyB))
	return out
}

func (r SyncRequest) Equivalent(other SyncRequest) bool {
	a, b := r.Normalized(), other.Normalized()
	return a.Scope == b.Scope && a.InstrumentID == b.InstrumentID && a.CurrencyA == b.CurrencyA && a.CurrencyB == b.CurrencyB && a.ForceRecheck == b.ForceRecheck
}

type SyncBlocker struct {
	TargetKey string
	Code      string
	Reason    string
}

type SyncItemProgress struct {
	TargetKey string
	Kind      string
	Status    string
	Detail    string
	Provider  string
}

type SyncJobSnapshot struct {
	JobID               string
	WorkspaceID         string
	HouseholdID         domain.HouseholdID
	PlanRevision        string
	Sequence            int
	Phase               string
	Outcome             string
	Scope               SyncRequest
	EstimatedRequests   int
	CompletedRequests   int
	TargetCount         int
	CompletedTargets    int
	SnapshotDaysPlanned int
	SnapshotDaysRebuilt int
	CommittedBatches    int
	Items               []SyncItemProgress
	Blockers            []SyncBlocker
	Prerequisites       []SyncBlocker
	NextEligibilityAt   *time.Time
	ErrorCode           string
}

func (s SyncJobSnapshot) Clone() SyncJobSnapshot {
	out := s
	out.Items = append([]SyncItemProgress(nil), s.Items...)
	out.Blockers = append([]SyncBlocker(nil), s.Blockers...)
	out.Prerequisites = append([]SyncBlocker(nil), s.Prerequisites...)
	if s.NextEligibilityAt != nil {
		copied := *s.NextEligibilityAt
		out.NextEligibilityAt = &copied
	}
	return out
}

type SyncPlanPreview struct {
	AsOf                  time.Time
	ConfigRevision        string
	Scope                 SyncRequest
	EstimatedRequestCount int
	SnapshotWorkEstimate  int
	Unresolved            []SyncBlocker
	InstrumentTargets     int
	FXTargets             int
	LatestInstrumentCount int
	LatestFXCount         int
	FetchRanges           int
}

type SyncStartResult struct {
	Job      SyncJobSnapshot
	Attached bool
	Conflict bool
	Reason   string
}

type syncJobState struct {
	cancel      context.CancelFunc
	epoch       uint64
	workspace   string
	snapshot    SyncJobSnapshot
	committed   bool
	blockers    int
	rateLimited bool
	rebuildErr  bool
}

func (s *Service) SetMarketDataSyncListener(fn MarketDataSyncListener) {
	s.syncMu.Lock()
	s.syncListener = fn
	s.syncMu.Unlock()
}

func (s *Service) SetHistoryPersister(persister HistoryPersister) {
	s.stateMu.Lock()
	s.historyPersist = persister
	s.stateMu.Unlock()
}

func (s *Service) historyWriter() (HistoryPersister, error) {
	s.stateMu.RLock()
	persister := s.historyPersist
	s.stateMu.RUnlock()
	if persister == nil {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "history persistence is not configured"}
	}
	return persister, nil
}

func (s *Service) historyMaxDays() int {
	if s.historyRequestMaxDays > 0 {
		return s.historyRequestMaxDays
	}
	return DefaultHistoryRequestMaxDays
}

func (s *Service) sleepSync(ctx context.Context, wait time.Duration) error {
	fn := s.syncSleep
	if fn == nil {
		fn = defaultSyncSleep
	}
	return fn(ctx, wait)
}

func defaultSyncSleep(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Service) workspaceIdentity() string {
	if path := strings.TrimSpace(s.repository.Path()); path != "" {
		return path
	}
	return s.livePath()
}

// PreviewMarketDataSync estimates work for a requested scope without HTTP.
func (s *Service) PreviewMarketDataSync(ctx context.Context, request SyncRequest) (SyncPlanPreview, error) {
	request = request.Normalized()
	if err := validateSyncRequest(request); err != nil {
		return SyncPlanPreview{}, err
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return SyncPlanPreview{}, err
	}
	plan, err := s.PlanHistorySync(ctx, HistorySyncOptions{ForceRecheck: request.ForceRecheck})
	if err != nil {
		return SyncPlanPreview{}, err
	}
	preview := SyncPlanPreview{
		AsOf:           s.clock(),
		ConfigRevision: plan.ResolverPolicyVersion,
		Scope:          request,
	}
	instruments := filterInstrumentNeeds(plan.Instruments, request)
	maxDays := s.historyMaxDays()
	for _, need := range instruments {
		if need.RouteStatus != domain.InstrumentRouteOK {
			preview.Unresolved = append(preview.Unresolved, SyncBlocker{
				TargetKey: instrumentTargetKey(need.InstrumentID),
				Code:      need.RouteStatus,
				Reason:    need.SkipReason,
			})
			continue
		}
		capped := CapHistoryRanges(need.FetchRanges, maxDays)
		if len(capped) == 0 {
			continue
		}
		preview.InstrumentTargets++
		preview.FetchRanges += len(capped)
		preview.EstimatedRequestCount += len(capped)
	}
	if request.Scope != SyncScopeInstrument {
		if plan.ManualFX {
			preview.Unresolved = append(preview.Unresolved, SyncBlocker{TargetKey: "fx", Code: "manual", Reason: "manual_fx"})
		} else {
			fxRanges, fxBlockers := s.planFXHistoryRanges(ctx, household.ID, plan, request)
			preview.Unresolved = append(preview.Unresolved, fxBlockers...)
			preview.FXTargets = len(fxRanges)
			preview.FetchRanges += len(fxRanges)
			preview.EstimatedRequestCount += len(fxRanges)
		}
	}
	if request.Scope != SyncScopeFX {
		preview.LatestInstrumentCount = len(instruments)
		preview.EstimatedRequestCount += len(instruments)
	}
	if request.Scope != SyncScopeInstrument && !plan.ManualFX {
		prefs, prefErr := s.repository.ListFXPreferences(ctx, household.ID)
		if prefErr != nil {
			return SyncPlanPreview{}, prefErr
		}
		latestFX := 0
		for _, preference := range prefs {
			if preference.SourceKind != domain.QuoteSourceProvider {
				continue
			}
			if request.Scope == SyncScopeFX && !fxPreferenceMatches(preference, request) {
				continue
			}
			latestFX++
		}
		preview.LatestFXCount = latestFX
		preview.EstimatedRequestCount += latestFX
	}
	state, err := s.repository.DailySnapshotState(ctx, household.ID)
	if err != nil {
		return SyncPlanPreview{}, err
	}
	preview.SnapshotWorkEstimate = estimateDirtyDays(state)
	return preview, nil
}

func validateSyncRequest(request SyncRequest) error {
	switch request.Scope {
	case SyncScopeRepairAll:
		return nil
	case SyncScopeInstrument:
		if request.InstrumentID == "" {
			return &domain.Error{Code: domain.ErrValidation, Field: "instrumentId", Message: "instrument scope requires an instrument id"}
		}
		if _, err := domain.ParseInstrumentID(request.InstrumentID); err != nil {
			return err
		}
		return nil
	case SyncScopeFX:
		if request.CurrencyA == "" || request.CurrencyB == "" {
			return &domain.Error{Code: domain.ErrValidation, Field: "fx", Message: "fx scope requires a currency pair"}
		}
		return nil
	default:
		return &domain.Error{Code: domain.ErrValidation, Field: "scope", Message: "unsupported market-data sync scope"}
	}
}

func filterInstrumentNeeds(needs []InstrumentRepairNeed, request SyncRequest) []InstrumentRepairNeed {
	if request.Scope == SyncScopeFX {
		return nil
	}
	if request.Scope != SyncScopeInstrument {
		return needs
	}
	filtered := make([]InstrumentRepairNeed, 0, 1)
	for _, need := range needs {
		if need.InstrumentID.String() == request.InstrumentID {
			filtered = append(filtered, need)
		}
	}
	return filtered
}

func fxPreferenceMatches(preference domain.FXPreference, request SyncRequest) bool {
	requested, errA := domain.ParseSupportedCurrency(request.CurrencyA)
	other, errB := domain.ParseSupportedCurrency(request.CurrencyB)
	if errA != nil || errB != nil {
		return false
	}
	return fxPairKey(preference.CurrencyA, preference.CurrencyB) == fxPairKey(requested, other)
}

func estimateDirtyDays(state domain.DailySnapshotState) int {
	if state.DirtyFrom == nil || strings.TrimSpace(*state.DirtyFrom) == "" {
		return 0
	}
	from := strings.TrimSpace(*state.DirtyFrom)
	to := from
	if state.DirtyTo != nil && strings.TrimSpace(*state.DirtyTo) != "" {
		to = strings.TrimSpace(*state.DirtyTo)
	}
	dates, err := domain.InclusiveMarketDates(from, to)
	if err != nil {
		return 0
	}
	return len(dates)
}

// CapHistoryRanges splits planned ranges to adapter-sized requests without
// expanding the approved date set.
func CapHistoryRanges(ranges []DateRange, maxDays int) []DateRange {
	if maxDays <= 0 {
		maxDays = DefaultHistoryRequestMaxDays
	}
	out := make([]DateRange, 0, len(ranges))
	for _, rng := range ranges {
		dates, err := InclusiveMarketDates(rng)
		if err != nil || len(dates) == 0 {
			continue
		}
		for start := 0; start < len(dates); start += maxDays {
			end := start + maxDays - 1
			if end >= len(dates) {
				end = len(dates) - 1
			}
			out = append(out, DateRange{Start: dates[start], End: dates[end]})
		}
	}
	return out
}

func (s *Service) planFXHistoryRanges(ctx context.Context, householdID domain.HouseholdID, plan HistoryRepairPlan, request SyncRequest) ([]fxHistoryTask, []SyncBlocker) {
	prefs, err := s.repository.ListFXPreferences(ctx, householdID)
	if err != nil {
		return nil, []SyncBlocker{{TargetKey: "fx", Code: string(domain.ErrUnavailable), Reason: "fx_preferences"}}
	}
	providerKey := s.FXProviderKey()
	if providerKey == "" {
		return nil, []SyncBlocker{{TargetKey: "fx", Code: string(domain.ErrUnavailable), Reason: "provider_not_configured"}}
	}
	maxDays := s.historyMaxDays()
	tasks := make([]fxHistoryTask, 0)
	var blockers []SyncBlocker
	origin := plan.OriginLocalDate
	end := plan.LastFinalizedMarketDate
	if origin == "" || end == "" || origin > end {
		return nil, nil
	}
	for _, preference := range prefs {
		if preference.SourceKind != domain.QuoteSourceProvider {
			continue
		}
		if request.Scope == SyncScopeFX && !fxPreferenceMatches(preference, request) {
			continue
		}
		rng := DateRange{Start: MarketDate(origin), End: MarketDate(end)}
		capped := CapHistoryRanges([]DateRange{rng}, maxDays)
		for _, item := range capped {
			tasks = append(tasks, fxHistoryTask{
				identity: FXMarketIdentity{ProviderKey: providerKey, BaseCurrency: preference.CurrencyA, QuoteCurrency: preference.CurrencyB},
				rng:      item,
			})
		}
	}
	return tasks, blockers
}

type fxHistoryTask struct {
	identity FXMarketIdentity
	rng      DateRange
}

type instrumentHistoryTask struct {
	need     InstrumentRepairNeed
	rng      DateRange
	identity InstrumentMarketIdentity
}

// StartMarketDataSync starts a job or attaches to an equivalent in-flight job.
// A different scope is not scheduled and is not silently merged.
func (s *Service) StartMarketDataSync(ctx context.Context, request SyncRequest) (SyncStartResult, error) {
	request = request.Normalized()
	if err := validateSyncRequest(request); err != nil {
		return SyncStartResult{}, err
	}
	preview, err := s.PreviewMarketDataSync(ctx, request)
	if err != nil {
		return SyncStartResult{}, err
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return SyncStartResult{}, err
	}
	s.syncMu.Lock()
	if current, ok := s.syncJobs[s.currentSyncID]; ok && current != nil && current.snapshot.Outcome == SyncOutcomeRunning {
		if current.snapshot.Scope.Equivalent(request) {
			snap := current.snapshot.Clone()
			s.syncMu.Unlock()
			return SyncStartResult{Job: snap, Attached: true}, nil
		}
		snap := current.snapshot.Clone()
		s.syncMu.Unlock()
		return SyncStartResult{Job: snap, Conflict: true, Reason: "different_scope_not_scheduled"}, nil
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	jobID := uuid.NewString()
	workspace := s.workspaceIdentity()
	job := &syncJobState{
		cancel:    cancel,
		epoch:     s.refreshEpoch.Load(),
		workspace: workspace,
		snapshot: SyncJobSnapshot{
			JobID:               jobID,
			WorkspaceID:         workspace,
			HouseholdID:         household.ID,
			PlanRevision:        preview.ConfigRevision,
			Sequence:            1,
			Phase:               SyncPhaseScan,
			Outcome:             SyncOutcomeRunning,
			Scope:               request,
			EstimatedRequests:   preview.EstimatedRequestCount,
			TargetCount:         preview.InstrumentTargets + preview.FXTargets,
			SnapshotDaysPlanned: preview.SnapshotWorkEstimate,
			Prerequisites:       append([]SyncBlocker(nil), preview.Unresolved...),
			Blockers:            append([]SyncBlocker(nil), preview.Unresolved...),
		},
	}
	s.syncJobs[jobID] = job
	s.currentSyncID = jobID
	listener := s.syncListener
	started := job.snapshot.Clone()
	s.syncWG.Add(1)
	s.syncMu.Unlock()
	if listener != nil {
		listener(SyncEventStarted, started)
	}
	go s.runMarketDataSync(jobCtx, job, request)
	return SyncStartResult{Job: started}, nil
}

func (s *Service) GetCurrentSyncJob() (SyncJobSnapshot, bool) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	job, ok := s.syncJobs[s.currentSyncID]
	if !ok || job == nil {
		return SyncJobSnapshot{}, false
	}
	return job.snapshot.Clone(), true
}

func (s *Service) GetSyncJob(jobID string) (SyncJobSnapshot, bool) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	job, ok := s.syncJobs[strings.TrimSpace(jobID)]
	if !ok || job == nil {
		return SyncJobSnapshot{}, false
	}
	return job.snapshot.Clone(), true
}

func (s *Service) CancelSyncJob(jobID string) (SyncJobSnapshot, bool) {
	s.syncMu.Lock()
	job, ok := s.syncJobs[strings.TrimSpace(jobID)]
	if !ok || job == nil {
		s.syncMu.Unlock()
		return SyncJobSnapshot{}, false
	}
	cancel := job.cancel
	s.syncMu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.syncWG.Wait()
	return s.GetSyncJob(jobID)
}

func (s *Service) CancelMarketDataSyncAndWait() {
	s.refreshEpoch.Add(1)
	s.syncMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.syncJobs))
	for _, job := range s.syncJobs {
		if job != nil && job.cancel != nil && job.snapshot.Outcome == SyncOutcomeRunning {
			cancels = append(cancels, job.cancel)
		}
	}
	s.syncMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	s.syncWG.Wait()
}

func (s *Service) publishSync(job *syncJobState, event string, mutate func()) {
	s.syncMu.Lock()
	if mutate != nil {
		mutate()
	}
	job.snapshot.Sequence++
	snap := job.snapshot.Clone()
	listener := s.syncListener
	s.syncMu.Unlock()
	if listener != nil {
		listener(event, snap)
	}
}
