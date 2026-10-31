package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type mobileBlockingProvider struct {
	*refreshFakeProvider
	entered, release chan struct{}
	once             sync.Once
}

func (p *mobileBlockingProvider) LatestInstrument(ctx context.Context, id InstrumentMarketIdentity) (LatestInstrumentQuote, error) {
	p.once.Do(func() { close(p.entered) })
	select {
	case <-p.release:
	case <-ctx.Done():
		return LatestInstrumentQuote{}, ctx.Err()
	}
	return p.refreshFakeProvider.LatestInstrument(ctx, id)
}

func TestMobileMarketJobsDedupeAcrossEntrypointsAndKeepPartialResults(t *testing.T) {
	s, ctx, _, setClock := newOnboardedService(t, "mobile-market", []string{"Owner"})
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	setClock(now)
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	fake := newRefreshFakeProvider()
	price, err := domain.ParseUnitPrice("12.3456789")
	if err != nil {
		t.Fatal(err)
	}
	fake.instruments["AAA"] = struct {
		quote LatestInstrumentQuote
		err   error
	}{quote: LatestInstrumentQuote{Price: price, Currency: "CNY", SourceKey: "fake", QuotedAt: now.Add(-24 * time.Hour)}}
	fake.instruments["BBB"] = struct {
		quote LatestInstrumentQuote
		err   error
	}{err: &domain.Error{Code: domain.ErrProviderRateLimit}}
	provider := &mobileBlockingProvider{refreshFakeProvider: fake, entered: make(chan struct{}), release: make(chan struct{})}
	s.SetMarketDataRegistry(NewMarketDataRegistryWithDefault("fake", provider))
	var first domain.Instrument
	for _, symbol := range []string{"AAA", "BBB"} {
		key := "fake"
		i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic " + symbol, Type: "stock", QuoteCurrency: "CNY", QuoteSource: "provider", ProviderKey: key, ProviderSymbol: symbol})
		if err != nil {
			t.Fatal(err)
		}
		if symbol == "AAA" {
			first = i
		}
	}
	request := MobileMarketRequest{Mode: "latest", Scope: "all"}
	job, attached, conflict, err := s.StartMobileMarket(ctx, request)
	if err != nil || attached || conflict {
		t.Fatalf("start %v %v %v", attached, conflict, err)
	}
	select {
	case <-provider.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("provider not entered")
	}
	duplicate, attached, conflict, err := s.StartMobileMarket(ctx, request)
	if err != nil || !attached || conflict || duplicate.ID != job.ID {
		t.Fatalf("attach %v %v %v", attached, conflict, err)
	}
	_, _, conflict, err = s.StartMobileMarket(ctx, MobileMarketRequest{Mode: "latest", Scope: "stale"})
	if err != nil || !conflict {
		t.Fatal("different scope was queued")
	}
	direct, err := s.RefreshInstrument(ctx, first.ID)
	if err != nil || len(direct.Items) != 1 || direct.Items[0].ErrorCode != domain.ErrConflict {
		t.Fatalf("desktop bypass %+v %v", direct, err)
	}
	started, err := s.StartMarketDataSync(ctx, SyncRequest{Scope: SyncScopeInstrument, InstrumentID: first.ID.String()})
	if err != nil || !started.Conflict {
		t.Fatalf("sync bypass %+v %v", started, err)
	}
	close(provider.release)
	s.mobileJobsWG.Wait()
	finished, ok := s.MobileMarketJob(job.ID)
	if !ok || finished.Outcome != SyncOutcomePartial {
		t.Fatalf("outcome %+v", finished)
	}
	quotes, err := s.repository.ListInstrumentQuotes(ctx, first.ID)
	if err != nil || len(quotes) != 1 || !quotes[0].QuotedAt.Equal(now.Add(-24*time.Hour)) {
		t.Fatalf("actual effective time lost: %v %+v", err, quotes)
	}
	fresh := NewService(s.repository)
	if _, ok := fresh.MobileMarketJob(job.ID); ok {
		t.Fatal("restart fabricated job status")
	}
}

func TestMobileReadsSharedDesktopLatestAndRequestsCancellationWithoutDuplicateWork(t *testing.T) {
	s, ctx, _, setClock := newOnboardedService(t, "mobile-shared-market", []string{"Synthetic owner"})
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	setClock(now)
	fake := newRefreshFakeProvider()
	provider := &mobileBlockingProvider{refreshFakeProvider: fake, entered: make(chan struct{}), release: make(chan struct{})}
	s.SetMarketDataRegistry(NewMarketDataRegistryWithDefault("fake", provider))
	for _, symbol := range []string{"AAA", "BBB"} {
		_, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic " + symbol, Type: "stock", QuoteCurrency: "CNY", QuoteSource: "provider", ProviderKey: "fake", ProviderSymbol: symbol})
		if err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan struct{})
	go func() { defer close(done); _, _ = s.RefreshAll(ctx) }()
	select {
	case <-provider.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("provider not entered")
	}
	shared, ok := s.MobileMarketJob("current")
	if !ok || shared.Source != "shared" || shared.Outcome != SyncOutcomeRunning || shared.Current == nil || shared.Current.Label != "Synthetic AAA" || shared.Cancellable {
		t.Fatalf("missing shared progress: %+v", shared)
	}
	_, _, conflict, err := s.StartMobileMarket(ctx, MobileMarketRequest{Mode: "latest", Scope: "stale"})
	if err != nil || !conflict {
		t.Fatalf("queued duplicate: %v %v", conflict, err)
	}
	if s.RequestCancelMobileMarket(shared.ID) {
		t.Fatal("cancelled a task outside mobile ownership")
	}
	close(provider.release)
	<-done
	finished, ok := s.MobileMarketJob(shared.ID)
	if !ok || finished.Outcome != SyncOutcomeFailed || len(finished.Items) != 2 || finished.Items[0].Detail == "" {
		t.Fatalf("missing shared failure: %+v", finished)
	}
	// A new mobile task is cancellable, and repeated cancellation is harmless.
	provider.entered, provider.release, provider.once = make(chan struct{}), make(chan struct{}), sync.Once{}
	job, _, conflict, err := s.StartMobileMarket(ctx, MobileMarketRequest{Mode: "latest", Scope: "all"})
	if err != nil || conflict {
		t.Fatal(err)
	}
	select {
	case <-provider.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("mobile provider not entered")
	}
	if !s.RequestCancelMobileMarket(job.ID) || !s.RequestCancelMobileMarket(job.ID) {
		t.Fatal("cancel request missing")
	}
	s.mobileJobsWG.Wait()
	final, ok := s.MobileMarketJob(job.ID)
	if !ok || final.Outcome != SyncOutcomeCancelled {
		t.Fatalf("cancel: %+v", final)
	}
}

func TestMobileCurrentSelectsSharedHistoryAndPreservesDesktopOnlyRetryScope(t *testing.T) {
	s, _, _, setClock := newOnboardedService(t, "mobile-shared-history", []string{"Synthetic owner"})
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	setClock(now)
	workspace := s.MobileWorkspace()
	s.mobileJobs = map[string]*MobileMarketJob{"old-mobile": {ID: "old-mobile", Instance: workspace, StartedAt: now.Add(-time.Hour), Outcome: SyncOutcomeSucceeded}}
	s.mobileJobCurrent = "old-mobile"
	s.currentSyncID = "desktop-sync"
	s.syncJobs = map[string]*syncJobState{"desktop-sync": {startedAt: now, workspace: s.workspaceIdentity(), snapshot: SyncJobSnapshot{JobID: "desktop-sync", WorkspaceID: s.workspaceIdentity(), Scope: SyncRequest{Scope: SyncScopeRepairAll}, Outcome: SyncOutcomeRunning, Phase: SyncPhaseHistoricalInstruments, Blockers: []SyncBlocker{{Code: "provider_authentication"}}}}}
	job, ok := s.MobileMarketJob("current")
	if !ok || job.ID != "desktop-sync" || job.Instance != workspace || job.Retryable || job.Request.Scope != "repair_all" || len(job.Blockers) != 1 {
		t.Fatalf("shared history: %+v", job)
	}
	s.syncJobs["desktop-sync"].snapshot.Outcome = SyncOutcomePartial
	job, _ = s.MobileMarketJob("current")
	if job.ID != "desktop-sync" || job.Outcome != SyncOutcomePartial {
		t.Fatalf("finished shared history lost: %+v", job)
	}
	s.syncJobs["desktop-sync"].workspace = "replaced-workspace"
	job, _ = s.MobileMarketJob("current")
	if job.ID == "desktop-sync" {
		t.Fatal("restore exposed an old job")
	}
}
