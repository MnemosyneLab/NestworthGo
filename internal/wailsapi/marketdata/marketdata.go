// Package marketdata adapts internal/application.Service's provider
// refresh surface (RefreshAll, RefreshRequiredFX, RefreshInstrument,
// RefreshFX, SetFXProvider, FXProviderKey) for the Wails IPC boundary.
//
// A synchronous method per operation is exposed for simple callers (the
// frontend's TanStack Query useMutation already treats any bound method as
// async, since every Wails call already returns a Promise). The
// StartXxx/CancelRefresh pair additionally implements the cancellable,
// event-streamed design from the technical design (Sec6, "Long-running /
// streaming operations"): an event for an abandoned request ID is ignored
// by the frontend so a stale completion cannot corrupt current UI state.
package marketdata

import (
	"context"
	"sync"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

// EventEmitter is the minimal surface this service needs from a Wails
// application object. It is a local interface (not github.com/wailsapp/
// wails/v3) so this package stays importable and unit-testable without the
// Wails runtime, per the technical design's Sec3 dependency rule; the real
// *application.App.Event satisfies it structurally in cmd/nestworth's
// main.go wiring (Phase 2).
type EventEmitter interface {
	Emit(name string, data any)
}

// noopEmitter is used when no emitter is supplied (e.g. Phase 1 unit
// tests), so a nil check is not required on every emit call.
type noopEmitter struct{}

func (noopEmitter) Emit(string, any) {}

const RefreshCompletedEvent = "marketdata.refresh.completed"

type Service struct {
	app    *application.Service
	events EventEmitter

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewService(app *application.Service, events EventEmitter) *Service {
	if events == nil {
		events = noopEmitter{}
	}
	return &Service{app: app, events: events, cancels: make(map[string]context.CancelFunc)}
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

func (s *Service) RefreshAll(ctx context.Context) (RefreshResultDTO, error) {
	result, err := s.app.RefreshAll(ctx)
	if err != nil {
		return RefreshResultDTO{}, apierror.Wrap(err)
	}
	return fromRefreshResult(result), nil
}

func (s *Service) RefreshRequiredFX(ctx context.Context) (RefreshResultDTO, error) {
	result, err := s.app.RefreshRequiredFX(ctx)
	if err != nil {
		return RefreshResultDTO{}, apierror.Wrap(err)
	}
	return fromRefreshResult(result), nil
}

func (s *Service) RefreshInstrument(ctx context.Context, instrumentID string) (RefreshResultDTO, error) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return RefreshResultDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.RefreshInstrument(ctx, id)
	if err != nil {
		return RefreshResultDTO{}, apierror.Wrap(err)
	}
	return fromRefreshResult(result), nil
}

func (s *Service) RefreshFX(ctx context.Context, currencyA, currencyB string) (RefreshResultDTO, error) {
	result, err := s.app.RefreshFX(ctx, currencyA, currencyB)
	if err != nil {
		return RefreshResultDTO{}, apierror.Wrap(err)
	}
	return fromRefreshResult(result), nil
}

func (s *Service) SetFXProvider(key string) error {
	return apierror.Wrap(s.app.SetFXProvider(key))
}

func (s *Service) FXProviderKey() string {
	return s.app.FXProviderKey()
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
	Result    *RefreshResultDTO `json:"result,omitempty"`
	Error     string            `json:"error,omitempty"`
}

func (s *Service) registerCancel(requestID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancels[requestID] = cancel
}

func (s *Service) clearCancel(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancels, requestID)
}

// CancelRefresh cancels the in-flight refresh started under requestID, if
// any. Calling it for an unknown or already-completed requestID is a no-op,
// not an error: the frontend does not need to race its own completion
// event to decide whether cancellation is still meaningful.
func (s *Service) CancelRefresh(requestID string) {
	s.mu.Lock()
	cancel, ok := s.cancels[requestID]
	delete(s.cancels, requestID)
	s.mu.Unlock()
	if ok {
		cancel()
	}
}

func (s *Service) runAsync(requestID string, run func(context.Context) (application.RefreshResult, error)) {
	ctx, cancel := context.WithCancel(context.Background())
	s.registerCancel(requestID, cancel)
	go func() {
		defer cancel()
		defer s.clearCancel(requestID)
		result, err := run(ctx)
		payload := RefreshCompletedPayload{RequestID: requestID}
		if err != nil {
			payload.Error = apierror.Wrap(err).Error()
		} else {
			dto := fromRefreshResult(result)
			payload.Result = &dto
		}
		s.events.Emit(RefreshCompletedEvent, payload)
	}()
}

// StartRefreshAll launches RefreshAll in a goroutine and returns
// immediately; completion is reported via RefreshCompletedEvent.
func (s *Service) StartRefreshAll(requestID string) {
	s.runAsync(requestID, s.app.RefreshAll)
}

func (s *Service) StartRefreshRequiredFX(requestID string) {
	s.runAsync(requestID, s.app.RefreshRequiredFX)
}

func (s *Service) StartRefreshInstrument(requestID, instrumentID string) {
	id, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		s.events.Emit(RefreshCompletedEvent, RefreshCompletedPayload{RequestID: requestID, Error: apierror.Wrap(err).Error()})
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
