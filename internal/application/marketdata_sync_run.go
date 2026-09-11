package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *Service) runMarketDataSync(ctx context.Context, job *syncJobState, request SyncRequest) {
	defer s.syncWG.Done()
	defer job.cancel()
	outcome := SyncOutcomeFailed
	phase := SyncPhaseFailed
	defer func() {
		if ctx.Err() != nil || phase == SyncPhaseCancelled {
			outcome = SyncOutcomeCancelled
			phase = SyncPhaseCancelled
		}
		s.finishSync(job, phase, outcome)
	}()

	s.publishSync(job, SyncEventProgress, func() {
		job.snapshot.Phase = SyncPhasePlan
	})
	if aborted(ctx) {
		phase = SyncPhaseCancelled
		return
	}

	plan, err := s.PlanHistorySync(ctx, HistorySyncOptions{ForceRecheck: request.ForceRecheck})
	if err != nil {
		s.failSync(job, err)
		return
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		s.failSync(job, err)
		return
	}

	needs := filterInstrumentNeeds(plan.Instruments, request)
	instrumentTasks := s.instrumentHistoryTasksFromNeeds(needs)
	fxTasks, fxBlockers := s.planFXHistoryRanges(ctx, household.ID, plan, request)
	if request.Scope == SyncScopeInstrument {
		fxTasks = nil
		fxBlockers = nil
	}

	s.publishSync(job, SyncEventProgress, func() {
		job.snapshot.Phase = SyncPhaseHistoricalInstruments
		job.snapshot.PlanRevision = plan.ResolverPolicyVersion
		job.snapshot.TargetCount = len(uniqueInstrumentIDs(instrumentTasks)) + len(fxTasks)
		job.snapshot.Blockers = append(job.snapshot.Blockers, fxBlockers...)
	})

	providerKeys := orderedProviderKeys(instrumentTasks, fxTasks)
	stopped := map[string]string{}

	if !s.runInstrumentHistoryPhase(ctx, job, instrumentTasks, providerKeys, stopped) {
		if aborted(ctx) {
			phase = SyncPhaseCancelled
		}
		return
	}
	if aborted(ctx) {
		phase = SyncPhaseCancelled
		return
	}

	s.publishSync(job, SyncEventProgress, func() {
		job.snapshot.Phase = SyncPhaseHistoricalFX
	})
	if !s.runFXHistoryPhase(ctx, job, household.ID, fxTasks, stopped) {
		if aborted(ctx) {
			phase = SyncPhaseCancelled
		}
		return
	}
	if aborted(ctx) {
		phase = SyncPhaseCancelled
		return
	}

	if request.Scope != SyncScopeFX {
		s.publishSync(job, SyncEventProgress, func() {
			job.snapshot.Phase = SyncPhaseLatestInstruments
		})
		s.runLatestInstrumentPhase(ctx, job, needs, stopped)
	}
	if aborted(ctx) {
		phase = SyncPhaseCancelled
		return
	}
	if request.Scope != SyncScopeInstrument {
		s.publishSync(job, SyncEventProgress, func() {
			job.snapshot.Phase = SyncPhaseLatestFX
		})
		s.runLatestFXPhase(ctx, job, request, stopped)
	}
	if aborted(ctx) {
		phase = SyncPhaseCancelled
		return
	}

	if !s.jobMayWrite(job) {
		phase = SyncPhaseCancelled
		return
	}

	s.publishSync(job, SyncEventProgress, func() {
		job.snapshot.Phase = SyncPhaseInvalidateSnapshots
	})
	s.publishSync(job, SyncEventProgress, func() {
		job.snapshot.Phase = SyncPhaseRebuildSnapshots
	})
	rebuilt, rebuildErr := s.RebuildDirtySnapshots(ctx)
	if rebuildErr != nil && !aborted(ctx) {
		job.rebuildErr = true
		s.recordBlocker(job, "snapshots", string(domain.ErrHistoryUpdateFailed), "rebuild_failed")
	}
	s.publishSync(job, SyncEventProgress, func() {
		job.snapshot.SnapshotDaysRebuilt = rebuilt
	})
	if aborted(ctx) {
		phase = SyncPhaseCancelled
		return
	}

	s.publishSync(job, SyncEventProgress, func() {
		job.snapshot.Phase = SyncPhaseVerify
	})
	state, stateErr := s.repository.DailySnapshotState(ctx, household.ID)
	dirtyLeft := false
	if stateErr == nil {
		dirtyLeft = state.DirtyFrom != nil && strings.TrimSpace(*state.DirtyFrom) != ""
	}
	s.syncMu.Lock()
	committed := job.committed
	blockers := len(job.snapshot.Blockers)
	rateLimited := job.rateLimited
	rebuildFailed := job.rebuildErr
	s.syncMu.Unlock()

	switch {
	case rebuildFailed && !committed:
		outcome, phase = SyncOutcomeFailed, SyncPhaseFailed
	case blockers > 0 || dirtyLeft || rateLimited || rebuildFailed:
		outcome, phase = SyncOutcomePartial, SyncPhaseComplete
	default:
		outcome, phase = SyncOutcomeSucceeded, SyncPhaseComplete
	}
}

func (s *Service) finishSync(job *syncJobState, phase, outcome string) {
	s.publishSync(job, SyncEventCompleted, func() {
		job.snapshot.Phase = phase
		job.snapshot.Outcome = outcome
	})
}

func (s *Service) failSync(job *syncJobState, err error) {
	code := string(domain.ErrUnavailable)
	var domainErr *domain.Error
	if errors.As(err, &domainErr) && domainErr != nil {
		code = string(domainErr.Code)
	}
	s.publishSync(job, SyncEventProgress, func() {
		job.snapshot.ErrorCode = code
		job.snapshot.Blockers = append(job.snapshot.Blockers, SyncBlocker{Code: code, Reason: "sync_failed"})
	})
}

func (s *Service) instrumentHistoryTasksFromNeeds(needs []InstrumentRepairNeed) []instrumentHistoryTask {
	maxDays := s.historyMaxDays()
	tasks := make([]instrumentHistoryTask, 0)
	for _, need := range needs {
		if need.RouteStatus != domain.InstrumentRouteOK {
			continue
		}
		identity := InstrumentMarketIdentity{
			ProviderKey:    need.ProviderKey,
			ProviderSymbol: need.ProviderSymbol,
			QuoteCurrency:  need.QuoteCurrency,
			Market:         need.Market,
		}
		for _, rng := range CapHistoryRanges(need.FetchRanges, maxDays) {
			tasks = append(tasks, instrumentHistoryTask{need: need, rng: rng, identity: identity})
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].identity.ProviderKey != tasks[j].identity.ProviderKey {
			return tasks[i].identity.ProviderKey < tasks[j].identity.ProviderKey
		}
		if tasks[i].need.InstrumentID.String() != tasks[j].need.InstrumentID.String() {
			return tasks[i].need.InstrumentID.String() < tasks[j].need.InstrumentID.String()
		}
		return string(tasks[i].rng.Start) < string(tasks[j].rng.Start)
	})
	return tasks
}

func uniqueInstrumentIDs(tasks []instrumentHistoryTask) map[domain.InstrumentID]struct{} {
	out := make(map[domain.InstrumentID]struct{})
	for _, task := range tasks {
		out[task.need.InstrumentID] = struct{}{}
	}
	return out
}

func orderedProviderKeys(instrumentTasks []instrumentHistoryTask, fxTasks []fxHistoryTask) []string {
	seen := map[string]struct{}{}
	for _, task := range instrumentTasks {
		seen[strings.ToLower(task.identity.ProviderKey)] = struct{}{}
	}
	for _, task := range fxTasks {
		seen[strings.ToLower(task.identity.ProviderKey)] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (s *Service) runInstrumentHistoryPhase(ctx context.Context, job *syncJobState, tasks []instrumentHistoryTask, providerOrder []string, stopped map[string]string) bool {
	for _, providerKey := range providerOrder {
		if stopped[providerKey] != "" {
			continue
		}
		for _, task := range tasks {
			if strings.ToLower(task.identity.ProviderKey) != providerKey {
				continue
			}
			if aborted(ctx) {
				return false
			}
			if reason := stopped[providerKey]; reason != "" {
				s.recordItem(job, instrumentTargetKey(task.need.InstrumentID), "instrument", "skipped", reason, providerKey)
				continue
			}
			ok := s.fetchAndCommitInstrumentRange(ctx, job, task, stopped)
			if !ok && aborted(ctx) {
				return false
			}
		}
	}
	return true
}

func (s *Service) runFXHistoryPhase(ctx context.Context, job *syncJobState, householdID domain.HouseholdID, tasks []fxHistoryTask, stopped map[string]string) bool {
	for _, task := range tasks {
		if aborted(ctx) {
			return false
		}
		providerKey := strings.ToLower(task.identity.ProviderKey)
		if reason := stopped[providerKey]; reason != "" {
			s.recordItem(job, fxIdentityKey(task.identity), "fx", "skipped", reason, providerKey)
			continue
		}
		if !s.fetchAndCommitFXRange(ctx, job, householdID, task, stopped) && aborted(ctx) {
			return false
		}
	}
	return true
}

func (s *Service) fetchAndCommitInstrumentRange(ctx context.Context, job *syncJobState, task instrumentHistoryTask, stopped map[string]string) bool {
	providerKey := strings.ToLower(task.identity.ProviderKey)
	target := instrumentTargetKey(task.need.InstrumentID)
	registry := s.MarketDataRegistry()
	if registry == nil {
		s.recordBlocker(job, target, string(domain.ErrUnavailable), "provider_not_configured")
		return true
	}
	provider, err := registry.Resolve(task.identity.ProviderKey)
	if err != nil {
		s.recordBlocker(job, target, string(domain.ErrUnavailable), "provider_not_configured")
		return true
	}
	history, ok := provider.(InstrumentHistoryProvider)
	if !ok {
		s.recordBlocker(job, target, string(domain.ErrUnavailable), "instrument_history_unsupported")
		return true
	}
	identity := task.identity
	if inst, lookupErr := s.repository.Instrument(ctx, job.snapshot.HouseholdID, task.need.InstrumentID); lookupErr == nil {
		identity.QuoteCurrency = inst.QuoteCurrency
	}
	outcome, fetchErr := s.fetchInstrumentHistoryWithRetry(ctx, job, history, identity, task.rng, providerKey, stopped)
	if aborted(ctx) {
		return false
	}
	if fetchErr != nil {
		return true
	}
	if outcome.Status == MappingUnsupported || outcome.Status == MappingInvalid {
		s.recordBlocker(job, target, string(outcome.Status), outcome.Reason)
		s.recordItem(job, target, "instrument", "blocked", outcome.Reason, providerKey)
		return true
	}
	if err := RefuseFailClosedInstrumentHistory(outcome); err != nil {
		s.recordBlocker(job, target, "unsupported_price_basis", "refusing_fail_closed_history")
		s.recordItem(job, target, "instrument", "blocked", "unsupported_price_basis", providerKey)
		return true
	}
	if !s.jobMayWrite(job) {
		s.recordBlocker(job, target, string(domain.ErrBackupRestoreBusy), "workspace_fenced")
		return false
	}
	result, persistErr := s.persistInstrumentHistory(ctx, job, CommitInstrumentHistoryRequest{
		HouseholdID:  job.snapshot.HouseholdID,
		InstrumentID: task.need.InstrumentID,
		Identity:     identity,
		Market:       task.need.Market,
		Outcome:      outcome,
		FetchedAt:    s.clock(),
	})
	if persistErr != nil {
		if isWorkspaceFence(persistErr) || aborted(ctx) {
			s.recordBlocker(job, target, string(domain.ErrBackupRestoreBusy), "workspace_fenced")
			return false
		}
		s.recordBlocker(job, target, string(domain.ErrUnavailable), "persist_failed")
		return true
	}
	s.publishSync(job, SyncEventItem, func() {
		job.snapshot.CompletedRequests++
		if !result.Unchanged {
			job.committed = true
			job.snapshot.CommittedBatches++
		}
		job.snapshot.CompletedTargets++
		job.snapshot.Items = append(job.snapshot.Items, SyncItemProgress{
			TargetKey: target,
			Kind:      "instrument",
			Status:    "fetched",
			Detail:    string(task.rng.Start) + ".." + string(task.rng.End),
			Provider:  providerKey,
		})
	})
	return true
}

func (s *Service) fetchAndCommitFXRange(ctx context.Context, job *syncJobState, householdID domain.HouseholdID, task fxHistoryTask, stopped map[string]string) bool {
	providerKey := strings.ToLower(task.identity.ProviderKey)
	target := fxIdentityKey(task.identity)
	registry := s.MarketDataRegistry()
	if registry == nil {
		s.recordBlocker(job, target, string(domain.ErrUnavailable), "provider_not_configured")
		return true
	}
	provider, err := registry.Resolve(task.identity.ProviderKey)
	if err != nil {
		s.recordBlocker(job, target, string(domain.ErrUnavailable), "provider_not_configured")
		return true
	}
	history, ok := provider.(FXHistoryProvider)
	if !ok {
		s.recordBlocker(job, target, string(domain.ErrUnavailable), "fx_history_unsupported")
		return true
	}
	outcome, fetchErr := s.fetchFXHistoryWithRetry(ctx, job, history, task.identity, task.rng, providerKey, stopped)
	if aborted(ctx) {
		return false
	}
	if fetchErr != nil {
		return true
	}
	if outcome.Status == MappingUnsupported || outcome.Status == MappingInvalid {
		s.recordBlocker(job, target, string(outcome.Status), outcome.Reason)
		return true
	}
	if err := RefuseFailClosedFXHistory(outcome); err != nil {
		s.recordBlocker(job, target, "unsupported_mapping", "refusing_fail_closed_history")
		return true
	}
	if !s.jobMayWrite(job) {
		s.recordBlocker(job, target, string(domain.ErrBackupRestoreBusy), "workspace_fenced")
		return false
	}
	result, persistErr := s.persistFXHistory(ctx, job, CommitFXHistoryRequest{
		HouseholdID: householdID,
		Identity:    task.identity,
		Outcome:     outcome,
		FetchedAt:   s.clock(),
	})
	if persistErr != nil {
		if isWorkspaceFence(persistErr) || aborted(ctx) {
			s.recordBlocker(job, target, string(domain.ErrBackupRestoreBusy), "workspace_fenced")
			return false
		}
		s.recordBlocker(job, target, string(domain.ErrUnavailable), "persist_failed")
		return true
	}
	s.publishSync(job, SyncEventItem, func() {
		job.snapshot.CompletedRequests++
		if !result.Unchanged {
			job.committed = true
			job.snapshot.CommittedBatches++
		}
		job.snapshot.CompletedTargets++
		job.snapshot.Items = append(job.snapshot.Items, SyncItemProgress{
			TargetKey: target,
			Kind:      "fx",
			Status:    "fetched",
			Detail:    string(task.rng.Start) + ".." + string(task.rng.End),
			Provider:  providerKey,
		})
	})
	return true
}

func (s *Service) fetchInstrumentHistoryWithRetry(ctx context.Context, job *syncJobState, history InstrumentHistoryProvider, identity InstrumentMarketIdentity, rng DateRange, providerKey string, stopped map[string]string) (MappingOutcome[InstrumentDailyObservation], error) {
	var last MappingOutcome[InstrumentDailyObservation]
	for attempt := 1; attempt <= SyncMaxTransientAttempts; attempt++ {
		if aborted(ctx) {
			return MappingOutcome[InstrumentDailyObservation]{}, ctx.Err()
		}
		outcome, err := history.InstrumentDailyHistory(ctx, identity, rng)
		if err == nil {
			return outcome, nil
		}
		handle := s.classifyProviderFetchError(ctx, job, err, providerKey, instrumentTargetKeyFromIdentity(identity), stopped)
		if handle == fetchStopProvider || handle == fetchFail {
			return MappingOutcome[InstrumentDailyObservation]{}, err
		}
		if handle == fetchAbort {
			return MappingOutcome[InstrumentDailyObservation]{}, ctx.Err()
		}
		if attempt == SyncMaxTransientAttempts {
			s.recordBlocker(job, instrumentTargetKeyFromIdentity(identity), string(domain.ErrProviderUnavailable), "transient_retries_exhausted")
			return last, err
		}
		if sleepErr := s.sleepSync(ctx, syncBackoff(attempt)); sleepErr != nil {
			return MappingOutcome[InstrumentDailyObservation]{}, sleepErr
		}
		last = outcome
	}
	return last, &domain.Error{Code: domain.ErrProviderUnavailable, Message: "provider is unavailable"}
}

func (s *Service) fetchFXHistoryWithRetry(ctx context.Context, job *syncJobState, history FXHistoryProvider, identity FXMarketIdentity, rng DateRange, providerKey string, stopped map[string]string) (MappingOutcome[FXDailyObservation], error) {
	var last MappingOutcome[FXDailyObservation]
	for attempt := 1; attempt <= SyncMaxTransientAttempts; attempt++ {
		if aborted(ctx) {
			return MappingOutcome[FXDailyObservation]{}, ctx.Err()
		}
		outcome, err := history.FXDailyHistory(ctx, identity, rng)
		if err == nil {
			return outcome, nil
		}
		handle := s.classifyProviderFetchError(ctx, job, err, providerKey, fxIdentityKey(identity), stopped)
		if handle == fetchStopProvider || handle == fetchFail {
			return MappingOutcome[FXDailyObservation]{}, err
		}
		if handle == fetchAbort {
			return MappingOutcome[FXDailyObservation]{}, ctx.Err()
		}
		if attempt == SyncMaxTransientAttempts {
			s.recordBlocker(job, fxIdentityKey(identity), string(domain.ErrProviderUnavailable), "transient_retries_exhausted")
			return last, err
		}
		if sleepErr := s.sleepSync(ctx, syncBackoff(attempt)); sleepErr != nil {
			return MappingOutcome[FXDailyObservation]{}, sleepErr
		}
		last = outcome
	}
	return last, &domain.Error{Code: domain.ErrProviderUnavailable, Message: "provider is unavailable"}
}

type fetchHandle int

const (
	fetchRetry fetchHandle = iota
	fetchStopProvider
	fetchFail
	fetchAbort
)

func (s *Service) classifyProviderFetchError(ctx context.Context, job *syncJobState, err error, providerKey, target string, stopped map[string]string) fetchHandle {
	if aborted(ctx) {
		return fetchAbort
	}
	code := providerErrorCode(err)
	switch code {
	case domain.ErrProviderRateLimit:
		stopped[providerKey] = "rate_limited"
		next := s.clock().Add(time.Hour)
		s.publishSync(job, SyncEventItem, func() {
			job.rateLimited = true
			job.snapshot.NextEligibilityAt = &next
			job.snapshot.Blockers = append(job.snapshot.Blockers, SyncBlocker{TargetKey: target, Code: string(code), Reason: "rate_limited"})
			job.snapshot.Items = append(job.snapshot.Items, SyncItemProgress{TargetKey: target, Status: "rate_limited", Provider: providerKey})
		})
		return fetchStopProvider
	case domain.ErrProviderAuthentication:
		stopped[providerKey] = "authentication"
		s.recordBlocker(job, target, string(code), "provider_authentication")
		return fetchStopProvider
	case domain.ErrProviderUnavailable:
		return fetchRetry
	default:
		s.recordBlocker(job, target, string(code), "provider_error")
		return fetchFail
	}
}

func (s *Service) runLatestInstrumentPhase(ctx context.Context, job *syncJobState, needs []InstrumentRepairNeed, stopped map[string]string) {
	for _, need := range needs {
		if aborted(ctx) {
			return
		}
		if need.RouteStatus != domain.InstrumentRouteOK {
			continue
		}
		providerKey := strings.ToLower(need.ProviderKey)
		if reason := stopped[providerKey]; reason != "" {
			s.recordItem(job, instrumentTargetKey(need.InstrumentID), "instrument", "skipped", reason, providerKey)
			continue
		}
		result, err := s.RefreshInstrument(ctx, need.InstrumentID)
		if aborted(ctx) {
			return
		}
		if err != nil {
			if isWorkspaceFence(err) {
				return
			}
			s.recordBlocker(job, instrumentTargetKey(need.InstrumentID), string(domain.ErrUnavailable), "latest_failed")
			continue
		}
		s.applyLatestRefreshResult(job, result, stopped)
	}
}

func (s *Service) runLatestFXPhase(ctx context.Context, job *syncJobState, request SyncRequest, stopped map[string]string) {
	prefs, err := s.repository.ListFXPreferences(ctx, job.snapshot.HouseholdID)
	if err != nil {
		s.recordBlocker(job, "fx", string(domain.ErrUnavailable), "fx_preferences")
		return
	}
	providerKey := strings.ToLower(s.FXProviderKey())
	for _, preference := range prefs {
		if preference.SourceKind != domain.QuoteSourceProvider {
			continue
		}
		if request.Scope == SyncScopeFX && !fxPreferenceMatches(preference, request) {
			continue
		}
		if aborted(ctx) {
			return
		}
		if reason := stopped[providerKey]; reason != "" {
			s.recordItem(job, "fx:"+fxPairKey(preference.CurrencyA, preference.CurrencyB), "fx", "skipped", reason, providerKey)
			continue
		}
		result, refreshErr := s.RefreshFX(ctx, preference.CurrencyA.String(), preference.CurrencyB.String())
		if aborted(ctx) {
			return
		}
		if refreshErr != nil {
			if isWorkspaceFence(refreshErr) {
				return
			}
			s.recordBlocker(job, "fx:"+fxPairKey(preference.CurrencyA, preference.CurrencyB), string(domain.ErrUnavailable), "latest_failed")
			continue
		}
		s.applyLatestRefreshResult(job, result, stopped)
	}
}

func (s *Service) applyLatestRefreshResult(job *syncJobState, result RefreshResult, stopped map[string]string) {
	for _, item := range result.Items {
		status := string(item.Status)
		if item.Status == RefreshRateLimited {
			stopped[strings.ToLower(item.TargetKey)] = "rate_limited"
			s.publishSync(job, SyncEventItem, func() {
				job.rateLimited = true
				next := s.clock().Add(time.Hour)
				job.snapshot.NextEligibilityAt = &next
				job.snapshot.CompletedRequests++
				job.snapshot.Items = append(job.snapshot.Items, SyncItemProgress{TargetKey: item.TargetKey, Kind: string(item.Kind), Status: status, Provider: string(item.ErrorCode)})
			})
			continue
		}
		s.publishSync(job, SyncEventItem, func() {
			job.snapshot.CompletedRequests++
			job.snapshot.Items = append(job.snapshot.Items, SyncItemProgress{TargetKey: item.TargetKey, Kind: string(item.Kind), Status: status})
		})
	}
}

func (s *Service) persistInstrumentHistory(ctx context.Context, job *syncJobState, request CommitInstrumentHistoryRequest) (CommitHistoryResult, error) {
	writer, err := s.historyWriter()
	if err != nil {
		return CommitHistoryResult{}, err
	}
	var result CommitHistoryResult
	err = s.persistRefreshWrite(ctx, job.epoch, func(ctx context.Context) error {
		if s.workspaceIdentity() != job.workspace {
			return backupRestoreBusy()
		}
		var persistErr error
		result, persistErr = writer.PersistInstrumentHistory(ctx, request)
		return persistErr
	})
	return result, err
}

func (s *Service) persistFXHistory(ctx context.Context, job *syncJobState, request CommitFXHistoryRequest) (CommitHistoryResult, error) {
	writer, err := s.historyWriter()
	if err != nil {
		return CommitHistoryResult{}, err
	}
	var result CommitHistoryResult
	err = s.persistRefreshWrite(ctx, job.epoch, func(ctx context.Context) error {
		if s.workspaceIdentity() != job.workspace {
			return backupRestoreBusy()
		}
		var persistErr error
		result, persistErr = writer.PersistFXHistory(ctx, request)
		return persistErr
	})
	return result, err
}

func (s *Service) jobMayWrite(job *syncJobState) bool {
	if s.refreshEpoch.Load() != job.epoch {
		return false
	}
	return s.workspaceIdentity() == job.workspace
}

func (s *Service) recordBlocker(job *syncJobState, target, code, reason string) {
	s.publishSync(job, SyncEventItem, func() {
		job.snapshot.Blockers = append(job.snapshot.Blockers, SyncBlocker{TargetKey: target, Code: code, Reason: reason})
	})
}

func (s *Service) recordItem(job *syncJobState, target, kind, status, detail, provider string) {
	s.publishSync(job, SyncEventItem, func() {
		job.snapshot.Items = append(job.snapshot.Items, SyncItemProgress{TargetKey: target, Kind: kind, Status: status, Detail: detail, Provider: provider})
	})
}

func aborted(ctx context.Context) bool {
	return ctx.Err() != nil
}

func providerErrorCode(err error) domain.ErrorCode {
	var domainErr *domain.Error
	if errors.As(err, &domainErr) && domainErr != nil {
		return domainErr.Code
	}
	return domain.ErrProviderUnavailable
}

func isWorkspaceFence(err error) bool {
	var domainErr *domain.Error
	return errors.As(err, &domainErr) && domainErr != nil && domainErr.Code == domain.ErrBackupRestoreBusy
}

func syncBackoff(attempt int) time.Duration {
	wait := 50 * time.Millisecond
	for i := 1; i < attempt; i++ {
		wait *= 2
	}
	return wait
}

func fxIdentityKey(identity FXMarketIdentity) string {
	return "fx:" + fxPairKey(identity.BaseCurrency, identity.QuoteCurrency)
}

func instrumentTargetKeyFromIdentity(identity InstrumentMarketIdentity) string {
	return "instrument:" + strings.TrimSpace(identity.ProviderSymbol)
}
