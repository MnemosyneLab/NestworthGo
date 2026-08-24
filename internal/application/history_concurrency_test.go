package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type historySnapshotBarrierRepository struct {
	Repository
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *historySnapshotBarrierRepository) ReadPortfolioSnapshot(ctx context.Context, filter domain.AccountFilter) (domain.PortfolioSnapshot, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return r.Repository.ReadPortfolioSnapshot(ctx, filter)
}

func TestStartHistoryAndCreateInstrumentSerializeHistoryBoundary(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/history-boundary.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	barrier := &historySnapshotBarrierRepository{Repository: sqlite.NewRepository(database), entered: make(chan struct{}), release: make(chan struct{})}
	service := NewService(barrier)
	clock := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "History boundary", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}

	historyDone := make(chan error, 1)
	go func() {
		_, startErr := service.StartHistory(ctx, "UTC")
		historyDone <- startErr
	}()
	select {
	case <-barrier.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("StartHistory did not reach the portfolio snapshot barrier")
	}

	instrumentDone := make(chan error, 1)
	go func() {
		_, createErr := service.CreateInstrument(ctx, InstrumentInput{Name: "Created during start", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "provider", ProviderKey: "fake", ProviderSymbol: "BOUNDARY"})
		instrumentDone <- createErr
	}()
	select {
	case err := <-instrumentDone:
		t.Fatalf("CreateInstrument completed before StartHistory snapshot committed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(barrier.release)
	if err := <-historyDone; err != nil {
		t.Fatalf("StartHistory() error = %v", err)
	}
	if err := <-instrumentDone; err != nil {
		t.Fatalf("CreateInstrument() error = %v", err)
	}

	origin, err := service.HistoryOrigin(ctx)
	if err != nil || origin == nil {
		t.Fatalf("HistoryOrigin() = %#v, %v", origin, err)
	}
	observations, err := barrier.ListInstrumentPreferenceObservations(ctx, origin.HouseholdID)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 {
		t.Fatalf("instrument observations = %d, want one creation observation", len(observations))
	}
}

func TestConcurrentObservationWritersSerializeUnderHistoryLock(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/observation-writers.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Observation writers", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Observed instrument", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	origin, err := service.HistoryOrigin(ctx)
	if err != nil || origin == nil {
		t.Fatalf("HistoryOrigin() = %#v, %v", origin, err)
	}

	var group sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			errs <- service.AppendInstrumentPreferenceObservation(ctx, domain.InstrumentPreferenceObservation{InstrumentID: instrument.ID, SourceKind: domain.QuoteSourceManual, EffectiveAt: clock})
		}()
	}
	group.Wait()
	close(errs)
	for writeErr := range errs {
		if writeErr != nil {
			t.Fatal(writeErr)
		}
	}

	observations, err := service.repository.ListInstrumentPreferenceObservations(ctx, origin.HouseholdID)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 2 {
		t.Fatalf("observation writers persisted %d rows, want 2", len(observations))
	}
}
