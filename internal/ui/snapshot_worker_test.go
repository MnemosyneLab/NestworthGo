package ui

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type snapshotWorkerFake struct {
	mu             sync.Mutex
	origin         *domain.HistoryOrigin
	originErr      error
	state          domain.DailySnapshotState
	originEntered  chan struct{}
	originReleased chan struct{}
	originReturned chan struct{}
	rebuildCalled  chan struct{}
	once           sync.Once
}

func (f *snapshotWorkerFake) HistoryOrigin(context.Context) (*domain.HistoryOrigin, error) {
	if f.originEntered != nil {
		f.once.Do(func() { close(f.originEntered) })
	}
	if f.originReleased != nil {
		<-f.originReleased
	}
	if f.originReturned != nil {
		close(f.originReturned)
	}
	return f.origin, f.originErr
}

func (f *snapshotWorkerFake) DailySnapshotState(context.Context, domain.HouseholdID) (domain.DailySnapshotState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, nil
}

func (f *snapshotWorkerFake) RebuildHistoricalSnapshots(context.Context, string, string) (int, error) {
	if f.rebuildCalled != nil {
		select {
		case f.rebuildCalled <- struct{}{}:
		default:
		}
	}
	return 1, nil
}

func newSnapshotController(fake snapshotService) *Controller {
	return &Controller{snapshotRunner: fake, translator: i18n.New(settings.LanguageEnglish)}
}

func TestStartSnapshotWorkerReturnsBeforePreparationQueriesFinish(t *testing.T) {
	fake := &snapshotWorkerFake{originEntered: make(chan struct{}), originReleased: make(chan struct{}), originReturned: make(chan struct{})}
	controller := newSnapshotController(fake)
	finished := make(chan struct{})
	controller.snapshotObserver = func() { close(finished) }

	returned := make(chan struct{})
	go func() {
		controller.startSnapshotWorker()
		close(returned)
	}()
	waitForChannel(t, fake.originEntered)
	waitForChannel(t, returned)
	select {
	case <-fake.originReturned:
		t.Fatal("preparation query returned before its release barrier")
	default:
	}

	close(fake.originReleased)
	waitForChannel(t, finished)
	if controller.snapshotTask.pending {
		t.Fatal("snapshot task remained pending after preparation completed")
	}
}

func TestCancelledSnapshotPreparationCannotStartRebuild(t *testing.T) {
	fake := &snapshotWorkerFake{origin: &domain.HistoryOrigin{HouseholdID: domain.NewHouseholdID(), Timezone: "UTC", StartedAt: time.Now().Add(-48 * time.Hour)}, originEntered: make(chan struct{}), originReleased: make(chan struct{}), originReturned: make(chan struct{}), rebuildCalled: make(chan struct{}, 1)}
	controller := newSnapshotController(fake)
	go controller.startSnapshotWorker()
	waitForChannel(t, fake.originEntered)
	controller.cancelSnapshotWorker()
	close(fake.originReleased)
	waitForChannel(t, fake.originReturned)
	select {
	case <-fake.rebuildCalled:
		t.Fatal("cancelled preparation started historical rebuild")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestPrepareSnapshotReportsNoHistoryNoWorkAndErrors(t *testing.T) {
	noHistory, err := prepareSnapshot(context.Background(), &snapshotWorkerFake{})
	if err != nil || noHistory.result.status != snapshotWorkerNoHistory {
		t.Fatalf("no-history preparation = %#v, %v", noHistory, err)
	}

	future := &snapshotWorkerFake{origin: &domain.HistoryOrigin{HouseholdID: domain.NewHouseholdID(), Timezone: "UTC", StartedAt: time.Now().Add(48 * time.Hour)}}
	noWork, err := prepareSnapshot(context.Background(), future)
	if err != nil || noWork.result.status != snapshotWorkerNoWork {
		t.Fatalf("no-work preparation = %#v, %v", noWork, err)
	}

	failed, err := prepareSnapshot(context.Background(), &snapshotWorkerFake{originErr: errors.New("sql: hidden")})
	if err == nil || failed.result.status != "" {
		t.Fatalf("failed preparation = %#v, %v", failed, err)
	}
	if got := snapshotError(err).Error(); got != "history snapshots could not be prepared" {
		t.Fatalf("safe snapshot error = %q", got)
	}
}

func waitForChannel(t *testing.T, channel <-chan struct{}) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(2 * time.Second):
		t.Fatal("channel signal did not arrive before timeout")
	}
}
