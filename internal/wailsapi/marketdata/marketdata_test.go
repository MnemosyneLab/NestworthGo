package marketdata_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

// fakeProvider is a deterministic test double for application.MarketDataProvider.
type fakeProvider struct {
	key string
}

func (p fakeProvider) Key() string { return p.key }
func (p fakeProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestFX: true, LatestInstrument: true}
}
func (p fakeProvider) LatestInstrument(context.Context, application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	price, _ := domain.ParseUnitPrice("100")
	return application.LatestInstrumentQuote{Price: price, Currency: "USD", SourceKey: p.key, QuotedAt: time.Now().UTC()}, nil
}
func (p fakeProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	rate, _ := domain.ParseFxRate("0.75")
	return application.LatestFXQuote{Rate: rate, BaseCurrency: "SGD", QuoteCurrency: "USD", SourceKey: p.key, QuotedAt: time.Now().UTC()}, nil
}

type fakeEmitter struct {
	mu     sync.Mutex
	events []struct {
		name string
		data any
	}
	notify chan struct{}
}

func newFakeEmitter() *fakeEmitter {
	return &fakeEmitter{notify: make(chan struct{}, 16)}
}

func (e *fakeEmitter) Emit(name string, data any) {
	e.mu.Lock()
	e.events = append(e.events, struct {
		name string
		data any
	}{name, data})
	e.mu.Unlock()
	e.notify <- struct{}{}
}

func (e *fakeEmitter) waitForEvent(t *testing.T) {
	t.Helper()
	select {
	case <-e.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for an emitted event")
	}
}

func (e *fakeEmitter) last() (string, any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.events) == 0 {
		return "", nil
	}
	last := e.events[len(e.events)-1]
	return last.name, last.data
}

func onboardedAppWithProvider(t *testing.T) *application.Service {
	t.Helper()
	registry := application.NewMarketDataRegistryWithDefault(application.FrankfurterProviderKey, fakeProvider{key: application.FrankfurterProviderKey})
	app := wailstest.NewService(t)
	app.SetMarketDataRegistry(registry)
	if err := household.NewService(app).CompleteOnboarding(context.Background(), household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	return app
}

func TestSetAndGetFXProvider(t *testing.T) {
	app := onboardedAppWithProvider(t)
	service := marketdata.NewService(app, nil)
	if err := service.SetFXProvider("frankfurter"); err != nil {
		t.Fatalf("SetFXProvider: %v", err)
	}
	if key := service.FXProviderKey(); key != "frankfurter" {
		t.Fatalf("FXProviderKey = %q, want frankfurter", key)
	}
}

func TestSetFXProviderUnknownKey(t *testing.T) {
	app := onboardedAppWithProvider(t)
	service := marketdata.NewService(app, nil)
	err := service.SetFXProvider("does-not-exist")
	if err == nil {
		t.Fatal("want an error for an unregistered provider key")
	}
}

func TestRefreshAllWithNoTargetsIsEmptyNotError(t *testing.T) {
	app := onboardedAppWithProvider(t)
	service := marketdata.NewService(app, nil)
	result, err := service.RefreshAll(context.Background())
	if err != nil {
		t.Fatalf("RefreshAll: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("Items = %+v, want empty (no accounts/instruments to refresh)", result.Items)
	}
}

func TestStartRefreshAllEmitsCompletionEvent(t *testing.T) {
	app := onboardedAppWithProvider(t)
	emitter := newFakeEmitter()
	service := marketdata.NewService(app, emitter)
	service.StartRefreshAll("req-1")
	emitter.waitForEvent(t)
	name, _ := emitter.last()
	if name != marketdata.RefreshCompletedEvent {
		t.Fatalf("event name = %q, want %q", name, marketdata.RefreshCompletedEvent)
	}
}

func TestCancelRefreshOnUnknownRequestIsNoop(t *testing.T) {
	app := onboardedAppWithProvider(t)
	service := marketdata.NewService(app, nil)
	service.CancelRefresh("never-started") // must not panic
}

func TestStartRefreshInstrumentWithInvalidIDEmitsError(t *testing.T) {
	app := onboardedAppWithProvider(t)
	emitter := newFakeEmitter()
	service := marketdata.NewService(app, emitter)
	service.StartRefreshInstrument("req-2", "not-a-uuid")
	emitter.waitForEvent(t)
	name, data := emitter.last()
	if name != marketdata.RefreshCompletedEvent {
		t.Fatalf("event name = %q, want %q", name, marketdata.RefreshCompletedEvent)
	}
	payload, ok := data.(marketdata.RefreshCompletedPayload)
	if !ok {
		t.Fatalf("event data = %#v, want marketdata.RefreshCompletedPayload", data)
	}
	if payload.RequestID != "req-2" || payload.Error == "" {
		t.Fatalf("payload = %+v, want RequestID=req-2 and a non-empty Error", payload)
	}
}

func TestStartRefreshAllPayloadCarriesResult(t *testing.T) {
	app := onboardedAppWithProvider(t)
	emitter := newFakeEmitter()
	service := marketdata.NewService(app, emitter)
	service.StartRefreshAll("req-3")
	emitter.waitForEvent(t)
	_, data := emitter.last()
	payload, ok := data.(marketdata.RefreshCompletedPayload)
	if !ok {
		t.Fatalf("event data = %#v, want marketdata.RefreshCompletedPayload", data)
	}
	if payload.RequestID != "req-3" || payload.Error != "" || payload.Result == nil {
		t.Fatalf("payload = %+v, want RequestID=req-3, no error, and a Result", payload)
	}
}
