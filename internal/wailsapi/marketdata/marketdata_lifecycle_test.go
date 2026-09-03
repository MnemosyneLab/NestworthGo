package marketdata

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
)

type lifecycleEmitter struct {
	mu     sync.Mutex
	events []RefreshCompletedPayload
	notify chan struct{}
}

func (e *lifecycleEmitter) Emit(name string, data any) {
	if name != RefreshCompletedEvent {
		return
	}
	e.mu.Lock()
	e.events = append(e.events, data.(RefreshCompletedPayload))
	e.mu.Unlock()
	e.notify <- struct{}{}
}

func (e *lifecycleEmitter) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.events)
}

func TestDuplicateRequestIDCancelsOldWorkerWithoutClearingNewRegistration(t *testing.T) {
	emitter := &lifecycleEmitter{notify: make(chan struct{}, 4)}
	service := &Service{events: emitter, cancels: make(map[string]refreshRegistration)}
	firstStarted := make(chan struct{})
	firstFinished := make(chan struct{})
	secondStarted := make(chan struct{})
	secondRelease := make(chan struct{})

	service.runAsync("duplicate", func(ctx context.Context) (application.RefreshResult, error) {
		close(firstStarted)
		<-ctx.Done()
		close(firstFinished)
		return application.RefreshResult{}, ctx.Err()
	})
	<-firstStarted

	service.runAsync("duplicate", func(ctx context.Context) (application.RefreshResult, error) {
		close(secondStarted)
		select {
		case <-secondRelease:
			return application.RefreshResult{}, nil
		case <-ctx.Done():
			t.Fatalf("replacement worker was cancelled by the old worker")
			return application.RefreshResult{}, ctx.Err()
		}
	})
	<-secondStarted
	<-firstFinished

	close(secondRelease)
	select {
	case <-emitter.notify:
	case <-time.After(time.Second):
		t.Fatal("replacement worker did not emit completion")
	}
	if got := emitter.count(); got != 1 {
		t.Fatalf("completion count = %d, want exactly one replacement completion", got)
	}

	// The old worker's deferred cancellation and completion have already run;
	// this must not cancel or delete a newer registration. The successful
	// replacement has already cleared its own registration, so this is a no-op.
	service.CancelRefresh("duplicate")
}

func TestCancelRefreshEmitsCancelledEventAndSuppressesWorkerCompletion(t *testing.T) {
	emitter := &lifecycleEmitter{notify: make(chan struct{}, 1)}
	service := &Service{events: emitter, cancels: make(map[string]refreshRegistration)}
	started := make(chan struct{})
	finished := make(chan struct{})

	service.runAsync("cancel", func(ctx context.Context) (application.RefreshResult, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return application.RefreshResult{}, ctx.Err()
	})
	<-started
	service.CancelRefresh("cancel")
	service.wg.Wait()
	<-finished

	if got := emitter.count(); got != 1 {
		t.Fatalf("event count = %d, want one cancellation event", got)
	}
	emitter.mu.Lock()
	payload := emitter.events[0]
	emitter.mu.Unlock()
	if payload.RequestID != "cancel" || payload.Status != "cancelled" || payload.Result != nil || payload.Error != "" {
		t.Fatalf("cancellation payload = %+v", payload)
	}
}
