// Package marketdata exposes asynchronous, cancellable provider refresh jobs
// and their progress events through the Wails IPC boundary.
package marketdata

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

// EventEmitter is the minimal surface this service needs from a Wails
// application object. It is a local interface (not github.com/wailsapp/
// wails/v3) so this package stays importable and unit-testable without the
// Wails runtime; the real *application.App.Event satisfies it structurally
// in cmd/nestworth's main.go wiring.
type EventEmitter interface {
	Emit(name string, data any)
}

// noopEmitter is used when no emitter is supplied in unit tests, so a nil
// check is not required on every emit call.
type noopEmitter struct{}

func (noopEmitter) Emit(string, any) {}

const RefreshCompletedEvent = "marketdata.refresh.completed"

type Service struct {
	app    *application.Service
	events EventEmitter

	mu        sync.Mutex
	nextToken uint64
	cancels   map[string]refreshRegistration
	wg        sync.WaitGroup
}

type refreshRegistration struct {
	cancel context.CancelFunc
	token  uint64
}

func NewService(app *application.Service, events EventEmitter) *Service {
	if events == nil {
		events = noopEmitter{}
	}
	service := &Service{app: app, events: events, cancels: make(map[string]refreshRegistration)}
	if app != nil {
		app.SetMarketDataSyncListener(service.emitSyncSnapshot)
	}
	return service
}

// RefreshTargetResultDTO mirrors application.RefreshTargetResult.
type RefreshTargetResultDTO struct {
	TargetKey string `json:"targetKey"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	ErrorCode string `json:"errorCode,omitempty"`
}

// RefreshResultDTO mirrors application.RefreshResult.
type RefreshResultDTO struct {
	Items       []RefreshTargetResultDTO `json:"items"`
	RateLimited bool                     `json:"rateLimited"`
}

func fromRefreshResult(value application.RefreshResult) RefreshResultDTO {
	items := make([]RefreshTargetResultDTO, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, RefreshTargetResultDTO{TargetKey: item.TargetKey, Kind: string(item.Kind), Status: string(item.Status), ErrorCode: string(item.ErrorCode)})
	}
	return RefreshResultDTO{Items: items, RateLimited: value.RateLimited}
}

// InstrumentSearchHitDTO is a Yahoo search candidate used to prefill the
// Instrument form. Empty optional identity fields stay omitted so the UI can
// keep whatever the user already typed.
type InstrumentSearchHitDTO struct {
	ProviderKey    string `json:"providerKey"`
	ProviderSymbol string `json:"providerSymbol"`
	Name           string `json:"name"`
	Symbol         string `json:"symbol"`
	Type           string `json:"type"`
	MarketCode     string `json:"marketCode,omitempty"`
	CountryCode    string `json:"countryCode,omitempty"`
	QuoteCurrency  string `json:"quoteCurrency,omitempty"`
	Exchange       string `json:"exchange,omitempty"`
}

func fromInstrumentSearchHit(hit application.InstrumentSearchHit) InstrumentSearchHitDTO {
	return InstrumentSearchHitDTO{
		ProviderKey: hit.ProviderKey, ProviderSymbol: hit.ProviderSymbol, Name: hit.Name,
		Symbol: hit.Symbol, Type: hit.Type, MarketCode: hit.MarketCode, CountryCode: hit.CountryCode,
		QuoteCurrency: hit.QuoteCurrency, Exchange: hit.Exchange,
	}
}

// SearchInstruments routes stock/ETF and crypto searches to their providers.
// It is a form-assist read and does not persist anything.
func (s *Service) SearchInstruments(ctx context.Context, query, instrumentType string) ([]InstrumentSearchHitDTO, error) {
	if s.app == nil {
		return nil, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	hits, err := s.app.SearchInstruments(ctx, query, instrumentType)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	result := make([]InstrumentSearchHitDTO, 0, len(hits))
	for _, hit := range hits {
		result = append(result, fromInstrumentSearchHit(hit))
	}
	return result, nil
}

// RefreshCompletedPayload is the event payload for RefreshCompletedEvent.
// RequestID lets the frontend ignore a completion for an abandoned request
// (the "stale completion" rule: ignore a completion for an abandoned
// request); Error is the same apierror.WireError JSON shape a
// synchronous method would have returned, or empty on success. Exported so
// main.go can register it with application.RegisterEvent for a typed
// TypeScript event payload.
type RefreshCompletedPayload struct {
	RequestID string            `json:"requestId"`
	Status    string            `json:"status"`
	Result    *RefreshResultDTO `json:"result,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// registerCancel atomically replaces the active request for an ID. The old
// context is cancelled after the new registration is visible, so an old
// worker cannot later remove or complete the replacement request.
func (s *Service) registerCancel(requestID string, cancel context.CancelFunc) uint64 {
	s.mu.Lock()
	previous := s.cancels[requestID]
	s.nextToken++
	token := s.nextToken
	s.cancels[requestID] = refreshRegistration{cancel: cancel, token: token}
	s.mu.Unlock()
	if previous.cancel != nil {
		previous.cancel()
	}
	return token
}

// emitIfCurrent drops stale completions and clears only the registration that
// produced this completion. Emitting while holding the small lifecycle lock
// also keeps completion ordering deterministic against a replacement.
func (s *Service) emitIfCurrent(requestID string, token uint64, payload RefreshCompletedPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.cancels[requestID]
	if !ok || current.token != token {
		return
	}
	delete(s.cancels, requestID)
	s.events.Emit(RefreshCompletedEvent, payload)
}

// CancelRefresh cancels the in-flight refresh started under requestID, if
// any, and emits a cancelled completion event. Calling it for an unknown or
// already-completed requestID is a no-op, not an error: the frontend does not
// need to race its own completion event to decide whether cancellation is
// still meaningful.
func (s *Service) CancelRefresh(requestID string) {
	s.mu.Lock()
	registration, ok := s.cancels[requestID]
	delete(s.cancels, requestID)
	s.mu.Unlock()
	if ok {
		registration.cancel()
		s.events.Emit(RefreshCompletedEvent, RefreshCompletedPayload{RequestID: requestID, Status: "cancelled"})
	}
}

func (s *Service) CancelAllAndWait() {
	if s.app != nil {
		s.app.CancelMarketDataSyncAndWait()
	}
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.cancels))
	for id, registration := range s.cancels {
		cancels = append(cancels, registration.cancel)
		delete(s.cancels, id)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	s.wg.Wait()
}

func (s *Service) runAsync(requestID string, run func(context.Context) (application.RefreshResult, error)) {
	ctx, cancel := context.WithCancel(context.Background())
	token := s.registerCancel(requestID, cancel)
	ctx = application.WithRefreshProgress(ctx, func(item application.SyncItemProgress) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if registration, ok := s.cancels[requestID]; ok && registration.token == token {
			s.events.Emit("marketdata.refresh.progress", RefreshProgressPayload{RequestID: requestID, Item: fromSyncItem(item)})
		}
	})
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		slog.Info("market data refresh started")
		result, err := run(ctx)
		slog.Info("market data refresh finished", "failed", err != nil, "items", len(result.Items), "rate_limited", result.RateLimited)
		payload := RefreshCompletedPayload{RequestID: requestID, Status: "completed"}
		if err != nil {
			payload.Status = "failed"
			payload.Error = apierror.Wrap(err).Error()
		} else {
			dto := fromRefreshResult(result)
			payload.Result = &dto
		}
		s.emitIfCurrent(requestID, token, payload)
	}()
}

func (s *Service) emitImmediate(requestID string, err error) {
	token := s.registerCancel(requestID, func() {})
	s.emitIfCurrent(requestID, token, RefreshCompletedPayload{RequestID: requestID, Status: "failed", Error: apierror.Wrap(err).Error()})
}

// StartRefreshAll launches RefreshAll in a goroutine and returns
// immediately; completion is reported via RefreshCompletedEvent.
func (s *Service) StartRefreshAll(requestID string) {
	s.runAsync(requestID, s.app.RefreshAll)
}

func (s *Service) StartRefreshMissingOrStale(requestID string) {
	s.runAsync(requestID, s.app.RefreshMissingOrStale)
}

func (s *Service) StartRefreshInstrument(requestID, instrumentID string) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		s.emitImmediate(requestID, err)
		return
	}
	s.runAsync(requestID, func(ctx context.Context) (application.RefreshResult, error) {
		return s.app.RefreshInstrument(ctx, id)
	})
}

func (s *Service) StartRefreshFX(requestID, currencyA, currencyB string) {
	s.runAsync(requestID, func(ctx context.Context) (application.RefreshResult, error) {
		return s.app.RefreshFX(ctx, currencyA, currencyB)
	})
}

const (
	SyncStartedEvent   = "marketdata.sync.started"
	SyncProgressEvent  = "marketdata.sync.progress"
	SyncItemEvent      = "marketdata.sync.item"
	SyncCompletedEvent = "marketdata.sync.completed"
)

type SyncRequestDTO struct {
	Scope        string `json:"scope"`
	InstrumentID string `json:"instrumentId,omitempty"`
	CurrencyA    string `json:"currencyA,omitempty"`
	CurrencyB    string `json:"currencyB,omitempty"`
	ForceRecheck bool   `json:"forceRecheck,omitempty"`
}

type SyncBlockerDTO struct {
	TargetKey string `json:"targetKey"`
	Code      string `json:"code"`
	Reason    string `json:"reason"`
}

type SyncItemDTO struct {
	Label     string `json:"label,omitempty"`
	Symbol    string `json:"symbol,omitempty"`
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`
	TargetKey string `json:"targetKey"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Provider  string `json:"provider,omitempty"`
}

type SyncJobDTO struct {
	Current             *SyncItemDTO     `json:"current,omitempty"`
	JobID               string           `json:"jobId"`
	WorkspaceID         string           `json:"workspaceId"`
	HouseholdID         string           `json:"householdId"`
	PlanRevision        string           `json:"planRevision"`
	Sequence            int              `json:"sequence"`
	Phase               string           `json:"phase"`
	Outcome             string           `json:"outcome"`
	Scope               SyncRequestDTO   `json:"scope"`
	EstimatedRequests   int              `json:"estimatedRequests"`
	CompletedRequests   int              `json:"completedRequests"`
	TargetCount         int              `json:"targetCount"`
	CompletedTargets    int              `json:"completedTargets"`
	SnapshotDaysPlanned int              `json:"snapshotDaysPlanned"`
	SnapshotDaysRebuilt int              `json:"snapshotDaysRebuilt"`
	CommittedBatches    int              `json:"committedBatches"`
	Items               []SyncItemDTO    `json:"items,omitempty"`
	Blockers            []SyncBlockerDTO `json:"blockers,omitempty"`
	Prerequisites       []SyncBlockerDTO `json:"prerequisites,omitempty"`
	NextEligibilityAt   string           `json:"nextEligibilityAt,omitempty"`
	ErrorCode           string           `json:"errorCode,omitempty"`
}

type SyncPlanPreviewDTO struct {
	Items                 []SyncItemDTO    `json:"items,omitempty"`
	AsOf                  string           `json:"asOf"`
	ConfigRevision        string           `json:"configRevision"`
	Scope                 SyncRequestDTO   `json:"scope"`
	EstimatedRequestCount int              `json:"estimatedRequestCount"`
	SnapshotWorkEstimate  int              `json:"snapshotWorkEstimate"`
	Unresolved            []SyncBlockerDTO `json:"unresolved,omitempty"`
	InstrumentTargets     int              `json:"instrumentTargets"`
	FXTargets             int              `json:"fxTargets"`
	LatestInstrumentCount int              `json:"latestInstrumentCount"`
	LatestFXCount         int              `json:"latestFxCount"`
	FetchRanges           int              `json:"fetchRanges"`
}

type SyncStartResultDTO struct {
	Job      SyncJobDTO `json:"job"`
	Attached bool       `json:"attached"`
	Conflict bool       `json:"conflict"`
	Reason   string     `json:"reason,omitempty"`
}

type HealthIssueDTO struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	GroupKey     string `json:"groupKey"`
	TargetKey    string `json:"targetKey"`
	Label        string `json:"label,omitempty"`
	Provider     string `json:"provider,omitempty"`
	InstrumentID string `json:"instrumentId,omitempty"`
	AccountID    string `json:"accountId,omitempty"`
	CurrencyA    string `json:"currencyA,omitempty"`
	CurrencyB    string `json:"currencyB,omitempty"`
	RangeStart   string `json:"rangeStart,omitempty"`
	RangeEnd     string `json:"rangeEnd,omitempty"`
	RangeCount   int    `json:"rangeCount,omitempty"`
	Code         string `json:"code,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Action       string `json:"action"`
	Executable   bool   `json:"executable"`
	Collapsed    bool   `json:"collapsed,omitempty"`
}

type MarketDataHealthReportDTO struct {
	Healthy                 bool             `json:"healthy"`
	IncompleteSince         string           `json:"incompleteSince,omitempty"`
	CoverageThrough         string           `json:"coverageThrough,omitempty"`
	LastFinalizedMarketDate string           `json:"lastFinalizedMarketDate,omitempty"`
	IssueCount              int              `json:"issueCount"`
	ExecutableCount         int              `json:"executableCount"`
	PrerequisiteCount       int              `json:"prerequisiteCount"`
	SnapshotDays            int              `json:"snapshotDays"`
	Issues                  []HealthIssueDTO `json:"issues,omitempty"`
}

func fromHealthIssue(issue application.HealthIssue) HealthIssueDTO {
	return HealthIssueDTO{
		ID: issue.ID, Kind: issue.Kind, Severity: issue.Severity, GroupKey: issue.GroupKey,
		TargetKey: issue.TargetKey, Label: issue.Label, Provider: issue.Provider,
		InstrumentID: issue.InstrumentID, AccountID: issue.AccountID, CurrencyA: issue.CurrencyA, CurrencyB: issue.CurrencyB,
		RangeStart: issue.RangeStart, RangeEnd: issue.RangeEnd, RangeCount: issue.RangeCount,
		Code: issue.Code, Reason: issue.Reason, Action: issue.Action, Executable: issue.Executable,
		Collapsed: issue.Collapsed,
	}
}

func fromHealthReport(report application.MarketDataHealthReport) MarketDataHealthReportDTO {
	dto := MarketDataHealthReportDTO{
		Healthy: report.Healthy, IncompleteSince: report.IncompleteSince, CoverageThrough: report.CoverageThrough,
		LastFinalizedMarketDate: report.LastFinalizedMarketDate, IssueCount: report.IssueCount,
		ExecutableCount: report.ExecutableCount, PrerequisiteCount: report.PrerequisiteCount,
		SnapshotDays: report.SnapshotDays,
	}
	for _, issue := range report.Issues {
		dto.Issues = append(dto.Issues, fromHealthIssue(issue))
	}
	return dto
}

// ScanMarketDataHealth runs the local Data Health scan. Opening Data Health
// must not cause provider HTTP; this method only reads the database, local
// Tiingo key status, and in-process job snapshot.
func (s *Service) ScanMarketDataHealth() (MarketDataHealthReportDTO, error) {
	if s.app == nil {
		return MarketDataHealthReportDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	report, err := s.app.ScanMarketDataHealth(context.Background())
	if err != nil {
		return MarketDataHealthReportDTO{}, apierror.Wrap(err)
	}
	return fromHealthReport(report), nil
}

func fromSyncRequest(dto SyncRequestDTO) application.SyncRequest {
	return application.SyncRequest{
		Scope:        application.SyncScopeKind(dto.Scope),
		InstrumentID: dto.InstrumentID,
		CurrencyA:    dto.CurrencyA,
		CurrencyB:    dto.CurrencyB,
		ForceRecheck: dto.ForceRecheck,
	}
}

func fromSyncJob(snapshot application.SyncJobSnapshot) SyncJobDTO {
	dto := SyncJobDTO{
		JobID:        snapshot.JobID,
		WorkspaceID:  snapshot.WorkspaceID,
		HouseholdID:  snapshot.HouseholdID.String(),
		PlanRevision: snapshot.PlanRevision,
		Sequence:     snapshot.Sequence,
		Phase:        snapshot.Phase,
		Outcome:      snapshot.Outcome,
		Scope: SyncRequestDTO{
			Scope:        string(snapshot.Scope.Scope),
			InstrumentID: snapshot.Scope.InstrumentID,
			CurrencyA:    snapshot.Scope.CurrencyA,
			CurrencyB:    snapshot.Scope.CurrencyB,
			ForceRecheck: snapshot.Scope.ForceRecheck,
		},
		EstimatedRequests:   snapshot.EstimatedRequests,
		CompletedRequests:   snapshot.CompletedRequests,
		TargetCount:         snapshot.TargetCount,
		CompletedTargets:    snapshot.CompletedTargets,
		SnapshotDaysPlanned: snapshot.SnapshotDaysPlanned,
		SnapshotDaysRebuilt: snapshot.SnapshotDaysRebuilt,
		CommittedBatches:    snapshot.CommittedBatches,
		ErrorCode:           snapshot.ErrorCode,
	}
	if snapshot.Current != nil {
		item := fromSyncItem(*snapshot.Current)
		dto.Current = &item
	}
	for _, item := range snapshot.Items {
		dto.Items = append(dto.Items, fromSyncItem(item))
	}
	for _, blocker := range snapshot.Blockers {
		dto.Blockers = append(dto.Blockers, SyncBlockerDTO{TargetKey: blocker.TargetKey, Code: blocker.Code, Reason: blocker.Reason})
	}
	for _, blocker := range snapshot.Prerequisites {
		dto.Prerequisites = append(dto.Prerequisites, SyncBlockerDTO{TargetKey: blocker.TargetKey, Code: blocker.Code, Reason: blocker.Reason})
	}
	if snapshot.NextEligibilityAt != nil {
		dto.NextEligibilityAt = snapshot.NextEligibilityAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func (s *Service) PreviewMarketDataSync(request SyncRequestDTO) (SyncPlanPreviewDTO, error) {
	if s.app == nil {
		return SyncPlanPreviewDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	preview, err := s.app.PreviewMarketDataSync(context.Background(), fromSyncRequest(request))
	if err != nil {
		return SyncPlanPreviewDTO{}, apierror.Wrap(err)
	}
	dto := SyncPlanPreviewDTO{
		AsOf:                  preview.AsOf.UTC().Format(time.RFC3339),
		ConfigRevision:        preview.ConfigRevision,
		Scope:                 SyncRequestDTO{Scope: string(preview.Scope.Scope), InstrumentID: preview.Scope.InstrumentID, CurrencyA: preview.Scope.CurrencyA, CurrencyB: preview.Scope.CurrencyB, ForceRecheck: preview.Scope.ForceRecheck},
		EstimatedRequestCount: preview.EstimatedRequestCount,
		SnapshotWorkEstimate:  preview.SnapshotWorkEstimate,
		InstrumentTargets:     preview.InstrumentTargets,
		FXTargets:             preview.FXTargets,
		LatestInstrumentCount: preview.LatestInstrumentCount,
		LatestFXCount:         preview.LatestFXCount,
		FetchRanges:           preview.FetchRanges,
	}
	for _, item := range preview.Items {
		dto.Items = append(dto.Items, fromSyncItem(item))
	}
	for _, blocker := range preview.Unresolved {
		dto.Unresolved = append(dto.Unresolved, SyncBlockerDTO{TargetKey: blocker.TargetKey, Code: blocker.Code, Reason: blocker.Reason})
	}
	return dto, nil
}

func (s *Service) StartMarketDataSync(request SyncRequestDTO) (SyncStartResultDTO, error) {
	if s.app == nil {
		return SyncStartResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	result, err := s.app.StartMarketDataSync(context.Background(), fromSyncRequest(request))
	if err != nil {
		return SyncStartResultDTO{}, apierror.Wrap(err)
	}
	return SyncStartResultDTO{Job: fromSyncJob(result.Job), Attached: result.Attached, Conflict: result.Conflict, Reason: result.Reason}, nil
}

func (s *Service) GetCurrentSyncJob() (SyncJobDTO, error) {
	if s.app == nil {
		return SyncJobDTO{}, nil
	}
	snapshot, ok := s.app.GetCurrentSyncJob()
	if !ok {
		return SyncJobDTO{}, nil
	}
	return fromSyncJob(snapshot), nil
}

func (s *Service) CancelSyncJob(jobID string) (SyncJobDTO, error) {
	if s.app == nil {
		return SyncJobDTO{}, nil
	}
	snapshot, ok := s.app.CancelSyncJob(jobID)
	if !ok {
		return SyncJobDTO{}, nil
	}
	return fromSyncJob(snapshot), nil
}

func (s *Service) emitSyncSnapshot(event string, snapshot application.SyncJobSnapshot) {
	dto := fromSyncJob(snapshot)
	switch event {
	case application.SyncEventStarted:
		s.events.Emit(SyncStartedEvent, dto)
	case application.SyncEventItem:
		s.events.Emit(SyncItemEvent, dto)
	case application.SyncEventCompleted:
		s.events.Emit(SyncCompletedEvent, dto)
	default:
		s.events.Emit(SyncProgressEvent, dto)
	}
}

func (s *Service) CoinGeckoQuoteCurrencies(ctx context.Context) ([]string, error) {
	if s.app == nil {
		return nil, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	values, err := s.app.CoinGeckoQuoteCurrencies(ctx)
	return values, apierror.Wrap(err)
}

func fromSyncItem(item application.SyncItemProgress) SyncItemDTO {
	return SyncItemDTO{TargetKey: item.TargetKey, Label: item.Label, Symbol: item.Symbol, StartDate: item.StartDate, EndDate: item.EndDate, Kind: item.Kind, Status: item.Status, Detail: item.Detail, Provider: item.Provider}
}

type RefreshProgressPayload struct {
	RequestID string      `json:"requestId"`
	Item      SyncItemDTO `json:"item"`
}
