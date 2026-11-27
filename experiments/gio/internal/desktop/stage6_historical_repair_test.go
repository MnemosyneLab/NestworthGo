package desktop

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
)

type stage6HistoryFXProvider struct {
	stage6LatestFXProvider
	sgdHistoryCalls atomic.Int32
}

func (p *stage6HistoryFXProvider) FXDailyHistory(ctx context.Context, id application.FXMarketIdentity, rng application.DateRange) (application.MappingOutcome[application.FXDailyObservation], error) {
	if id.BaseCurrency == "SGD" || id.QuoteCurrency == "SGD" {
		p.sgdHistoryCalls.Add(1)
		if !p.recoverSGD.Load() {
			return application.MappingOutcome[application.FXDailyObservation]{}, &domain.Error{Code: domain.ErrValidation}
		}
	}
	return p.refreshTestProvider.FXDailyHistory(ctx, id, rng)
}

type stage6HistoryEvents chan marketdata.SyncJobDTO

func (events stage6HistoryEvents) Emit(name string, data any) {
	if name == marketdata.SyncCompletedEvent {
		events <- data.(marketdata.SyncJobDTO)
	}
}

func (events stage6HistoryEvents) await(t *testing.T, id string) marketdata.SyncJobDTO {
	t.Helper()
	select {
	case event := <-events:
		if event.JobID != id {
			t.Fatalf("Wails completion = %s, want %s", event.JobID, id)
		}
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("Wails did not emit the historical repair outcome")
		return marketdata.SyncJobDTO{}
	}
}

func TestStage6PairedHistoricalRepairStaleAndRetry(t *testing.T) {
	now := time.Date(2026, 11, 6, 18, 0, 0, 0, time.UTC)
	var advance atomic.Int64
	clock := func() time.Time { return now.Add(time.Duration(advance.Load())) }
	seed, _ := seedStage6Market(t, now.AddDate(0, 0, -4), false)
	gioPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "gio"), seed)
	wailsPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "wails"), seed)
	open := func(path string) (*application.Service, *stage6HistoryFXProvider) {
		t.Helper()
		fx := &stage6HistoryFXProvider{stage6LatestFXProvider: stage6LatestFXProvider{refreshTestProvider: refreshTestProvider{cancelFirstHistory: true, now: now}}}
		_, _, svc := openStage6Database(t, path, clock, application.NewMarketDataRegistryWithDefault(fx.Key(), fx))
		t.Cleanup(svc.CancelMarketDataSyncAndWait)
		return svc, fx
	}
	gio, gioFX := open(gioPath)
	wails, wailsFX := open(wailsPath)
	events := make(stage6HistoryEvents, 4)
	adapter := marketdata.NewService(wails, events)
	t.Cleanup(adapter.CancelAllAndWait)
	ctx, stop := context.WithCancel(t.Context())
	page := &marketDataPage{ctx: ctx, reader: gio, invalidate: func() {}, sync: &dataHealthRepair{service: gio}}
	gio.SetMarketDataSyncListener(func(string, application.SyncJobSnapshot) { page.sync.revision.Add(1) })
	t.Cleanup(func() { stop(); page.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{marketData: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.trail = newNavigationTrail(marketDataRoute)
	page.enter(marketDataVisit{tab: marketFX})
	waitMarketOperations(t, h, func() bool { return page.request == nil })
	h.frame()

	request := marketdata.SyncRequestDTO{Scope: string(application.SyncScopeRepairAll)}
	openReview := func(button string) marketdata.SyncPlanPreviewDTO {
		t.Helper()
		h.click(button, false)
		waitMarketOperations(t, h, func() bool { return page.sync.plan != nil })
		plan, err := adapter.ReviewMarketDataSync(request)
		evidence := plan
		evidence.ReviewID, evidence.ExpiresAt = "", ""
		if err != nil || plan.ReviewID == "" || page.sync.plan.ReviewID == "" || !reflect.DeepEqual(marketSyncEvidence(*page.sync.plan), evidence) {
			t.Fatalf("Gio preview = %+v, Wails = %+v, %v", page.sync.plan, plan, err)
		}
		return plan
	}
	start := func(plan marketdata.SyncPlanPreviewDTO) string {
		t.Helper()
		h.click("Start sync", false)
		result, err := adapter.ConfirmMarketDataSync(request, plan.ReviewID)
		if err != nil || result.Conflict || result.Job.JobID == "" {
			t.Fatalf("Wails confirmation = %+v, %v", result, err)
		}
		return result.Job.JobID
	}
	assertNoQuotes := func() {
		t.Helper()
		for _, svc := range []*application.Service{gio, wails} {
			if quotes, err := svc.FXQuoteHistory(ctx); len(quotes) != 0 || err != nil {
				t.Fatalf("unapproved or cancelled repair stored FX quotes %+v, %v", quotes, err)
			}
		}
	}
	assertStale := func(plan marketdata.SyncPlanPreviewDTO, calls int32) {
		t.Helper()
		h.click("Start sync", false)
		waitMarketOperations(t, h, func() bool { return page.sync.problem == i18n.HealthRepairStale })
		result, err := adapter.ConfirmMarketDataSync(request, plan.ReviewID)
		var problem *apierror.WireError
		if !errors.As(err, &problem) || problem.Code != string(domain.ErrStalePreview) || result.Job.JobID != "" {
			t.Fatalf("Wails stale review = %+v, %v", result, err)
		}
		if gioFX.historyCalls.Load() != calls || wailsFX.historyCalls.Load() != calls ||
			gioFX.latestCalls.Load() != 0 || wailsFX.latestCalls.Load() != 0 {
			t.Fatal("stale review reached a provider")
		}
		assertNoQuotes()
	}

	h.click("More actions", false)
	old := openReview("Sync and repair")
	advance.Store(int64(16 * time.Minute))
	assertStale(old, 0)
	if _, exists := gio.GetCurrentSyncJob(); exists {
		t.Fatal("expired review created a job")
	}

	old = openReview("Retry sync")
	for _, svc := range []*application.Service{gio, wails} {
		if _, err := svc.SetFXPreference(ctx, "SGD", "USD", "manual"); err != nil {
			t.Fatal(err)
		}
	}
	assertStale(old, 0)
	for _, svc := range []*application.Service{gio, wails} {
		if _, err := svc.SetFXPreference(ctx, "SGD", "USD", "provider"); err != nil {
			t.Fatal(err)
		}
	}
	fresh := openReview("Retry sync")
	if fresh.FXTargets != 2 || fresh.FetchRanges == 0 {
		t.Fatalf("restored review did not include both FX targets: %+v", fresh)
	}
	cancelled := start(fresh)
	waitMarketOperations(t, h, func() bool {
		return page.sync.job.Outcome == application.SyncOutcomeRunning && page.sync.startRead == nil &&
			gioFX.historyCalls.Load() == 1 && wailsFX.historyCalls.Load() == 1
	})
	h.frame()
	h.click("Cancel repair", false)
	_, _ = adapter.CancelSyncJob(cancelled)
	if event := events.await(t, cancelled); event.Outcome != string(application.SyncOutcomeCancelled) {
		t.Fatalf("Wails cancellation = %+v", event)
	}
	waitMarketOperations(t, h, func() bool { return page.sync.job.Outcome == application.SyncOutcomeCancelled && page.request == nil })
	assertNoQuotes()

	partialID := start(openReview("Retry sync"))
	partial := events.await(t, partialID)
	waitMarketOperations(t, h, func() bool { return page.sync.job.Outcome == application.SyncOutcomePartial && page.request == nil })
	if partial.Outcome != string(application.SyncOutcomePartial) || partial.CommittedBatches != 1 || page.sync.job.CommittedBatches != 1 {
		t.Fatalf("partial repair did not preserve successful range: Gio %+v, Wails %+v", page.sync.job, partial)
	}
	jpyQuotes := func(svc *application.Service) []domain.FXQuote {
		t.Helper()
		quotes, err := svc.FXQuoteHistory(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var out []domain.FXQuote
		for _, q := range quotes {
			if (q.BaseCurrency == "JPY" || q.QuoteCurrency == "JPY") && q.EffectiveDate != "" {
				if q.Rate.Canonical() != "139.123456" || q.SourceKind != domain.QuoteSourceProvider || q.SourceKey != gioFX.Key() || q.SourcePolicyVersion != domain.FrankfurterV2BlendedPolicy {
					t.Fatalf("JPY historical provider evidence = %+v", q)
				}
				out = append(out, q)
			}
		}
		if len(out) == 0 {
			t.Fatal("partial repair did not store JPY history")
		}
		return out
	}
	savedGio, savedWails := jpyQuotes(gio), jpyQuotes(wails)
	gioFX.recoverSGD.Store(true)
	wailsFX.recoverSGD.Store(true)
	retry := openReview("Retry sync")
	if retry.Scope != request || retry.FXTargets != 1 {
		t.Fatalf("retry changed scope or re-planned verified JPY range: %+v", retry)
	}
	firstGioJob := page.sync.job.JobID
	retriedID := start(retry)
	retried := events.await(t, retriedID)
	waitMarketOperations(t, h, func() bool {
		return page.sync.job.JobID != firstGioJob && page.sync.job.Outcome != application.SyncOutcomeRunning && page.request == nil
	})
	if page.sync.job.Outcome != application.SyncOutcomeSucceeded || retried.Outcome != application.SyncOutcomeSucceeded ||
		gioFX.sgdHistoryCalls.Load() != 2 || wailsFX.sgdHistoryCalls.Load() != 2 {
		t.Fatalf("retry did not recover failed SGD: Gio %+v, Wails %+v", page.sync.job, retried)
	}
	_, _, reopenedGio := openStage6Database(t, gioPath, clock)
	_, _, reopenedWails := openStage6Database(t, wailsPath, clock)
	if !reflect.DeepEqual(savedGio, jpyQuotes(reopenedGio)) || !reflect.DeepEqual(savedWails, jpyQuotes(reopenedWails)) {
		t.Fatal("retry changed successfully committed JPY history")
	}
	for _, svc := range []*application.Service{reopenedGio, reopenedWails} {
		q, err := svc.CurrentFXQuote(ctx, "SGD", "USD")
		if err != nil || q == nil || q.Rate.Canonical() != "1.3456" || q.SourceKey != gioFX.Key() {
			t.Fatalf("reopened SGD quote = %+v, %v", q, err)
		}
	}
}
