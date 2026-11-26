package desktop

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
)

// Review identity is one-use interaction state, separate from plan evidence.
func marketSyncEvidence(plan application.SyncPlanPreview) marketdata.SyncPlanPreviewDTO {
	plan.ReviewID = ""
	return marketdata.SyncPreviewDTO(plan)
}

type refreshTestProvider struct {
	unavailableOverviewProvider
	latestCalls, historyCalls atomic.Int32
	useNewRate                atomic.Bool
	cancelFirstLatest         bool
	cancelFirstHistory        bool
	now                       time.Time
}

func (*refreshTestProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestFX: true, FXDailyHistory: true}
}

type refreshInstrumentTestProvider struct{ unavailableOverviewProvider }

func (*refreshInstrumentTestProvider) Key() string { return application.YahooFinanceProviderKey }

func (*refreshInstrumentTestProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestInstrument: true, InstrumentDailyHistory: true}
}

func (p *refreshInstrumentTestProvider) LatestInstrument(_ context.Context, id application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	if id.ProviderSymbol != "DEMO" {
		return application.LatestInstrumentQuote{}, &domain.Error{Code: domain.ErrProviderUnavailable}
	}
	price, err := domain.ParseUnitPrice("17.12345678")
	return application.LatestInstrumentQuote{Price: price, Currency: id.QuoteCurrency, SourceKey: p.Key(),
		QuotedAt: time.Date(2026, time.November, 6, 18, 0, 0, 0, time.UTC)}, err
}

func (p *refreshTestProvider) LatestFX(ctx context.Context, id application.FXMarketIdentity) (application.LatestFXQuote, error) {
	if p.latestCalls.Add(1) == 1 && p.cancelFirstLatest {
		<-ctx.Done()
		return application.LatestFXQuote{}, ctx.Err()
	}
	if id.BaseCurrency == "SGD" || id.QuoteCurrency == "SGD" {
		return application.LatestFXQuote{}, &domain.Error{Code: domain.ErrProviderUnavailable}
	}
	value, quotedAt := "140.4321", p.now.Add(-time.Minute)
	if p.useNewRate.Load() {
		value, quotedAt = "140.4322", p.now
	}
	rate, err := domain.ParseFxRate(value)
	return application.LatestFXQuote{Rate: rate, BaseCurrency: id.BaseCurrency, QuoteCurrency: id.QuoteCurrency,
		SourceKey: p.Key(), QuotedAt: quotedAt}, err
}

func (p *refreshTestProvider) FXDailyHistory(ctx context.Context, id application.FXMarketIdentity, rng application.DateRange) (application.MappingOutcome[application.FXDailyObservation], error) {
	if p.historyCalls.Add(1) == 1 && p.cancelFirstHistory {
		<-ctx.Done()
		return application.MappingOutcome[application.FXDailyObservation]{}, ctx.Err()
	}
	batch := application.HistoryBatch[application.FXDailyObservation]{
		Evidence:       application.ResponseEvidence{Adapter: "frankfurter", SourcePolicy: domain.FrankfurterV2BlendedPolicy},
		VerifiedRanges: []application.DateRange{rng},
	}
	from, err := time.Parse(time.DateOnly, string(rng.Start))
	if err != nil {
		return application.MappingOutcome[application.FXDailyObservation]{}, err
	}
	for day := from; day.Format(time.DateOnly) <= string(rng.End); day = day.AddDate(0, 0, 1) {
		batch.Observations = append(batch.Observations, application.FXDailyObservation{
			MarketDate: application.MarketDate(day.Format(time.DateOnly)), Rate: "139.123456",
			BaseCurrency: id.BaseCurrency.String(), QuoteCurrency: id.QuoteCurrency.String(), ValueEffectiveAt: day.Add(16 * time.Hour),
			Kind: application.FXObservationDailyReference, TimestampBasis: application.TimestampBasisPolicyDerived,
		})
	}
	return application.MappingOutcome[application.FXDailyObservation]{Status: application.MappingMapped, Batch: batch}, nil
}

func newMarketOperationsHarness(t *testing.T, provider *refreshTestProvider, historical bool) (*previewHarness, *marketDataPage, *application.Service, *marketdata.Service) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "provider.db"))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db)
	service := application.NewService(repo, application.NewMarketDataRegistryWithDefault(provider.Key(), provider, &refreshInstrumentTestProvider{}))
	appports.AttachSQLiteHistory(service, repo)
	service.SetLiveDatabasePath(db.Path)
	start := time.Date(2026, time.November, 2, 18, 0, 0, 0, time.UTC)
	service.SetClock(func() time.Time { return start })
	repair := &dataHealthRepair{service: service}
	page := &marketDataPage{ctx: ctx, reader: service, invalidate: func() {}, sync: repair, latest: marketLatest{service: service}}
	repair.latestBusy = func() bool { return page.latest.request != nil }
	t.Cleanup(func() {
		cancel()
		page.workers.Wait()
		service.CancelMarketDataSyncAndWait()
		_ = db.Close()
	})
	wails := marketdata.NewService(service, nil)
	service.SetMarketDataSyncListener(func(string, application.SyncJobSnapshot) { repair.revision.Add(1) })
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic provider", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, currency := range []string{"JPY", "SGD"} {
		if _, err := service.CreateAccount(ctx, application.AccountInput{
			Name: "Synthetic " + currency, AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
			DefaultCurrency: currency, InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.SetFXPreference(ctx, currency, "USD", "provider"); err != nil {
			t.Fatal(err)
		}
	}
	if historical {
		if _, err := service.StartHistory(ctx, "UTC"); err != nil {
			t.Fatal(err)
		}
	}
	provider.now = start.AddDate(0, 0, 4)
	service.SetClock(func() time.Time { return provider.now })
	h := newShellHarnessWithPages(t, ledgerPages{marketData: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.trail = newNavigationTrail(marketDataRoute)
	page.enter(marketDataVisit{tab: marketFX})
	page.workers.Wait()
	h.frame()
	return h, page, service, wails
}

func waitMarketOperations(t *testing.T, h *previewHarness, condition func() bool) {
	t.Helper()
	end := time.Now().Add(5 * time.Second)
	for !condition() && time.Now().Before(end) {
		h.frame()
		time.Sleep(time.Millisecond)
	}
	if !condition() {
		p := h.shell.marketData
		t.Fatalf("market operation did not reach expected state: latest=%+v sync=%+v", p.latest.result, p.sync.job)
	}
}

func TestMarketDataLatestCancelRetryAndPartialReadback(t *testing.T) {
	provider := &refreshTestProvider{cancelFirstLatest: true}
	h, p, service, _ := newMarketOperationsHarness(t, provider, false)
	ctx := t.Context()
	h.click("Update latest prices", false)
	waitMarketOperations(t, h, func() bool { return provider.latestCalls.Load() == 1 })
	h.click("Latest refresh details", false)
	h.frame()
	assertPageLabel(t, &h.router, "Running")
	h.press(key.NameEscape)
	h.frame()
	h.focused(&p.latest.details)
	// A second refresh or a historical preview cannot start while the
	// provider owns the first request.
	p.startLatest(marketLatestTarget{force: true})
	p.previewMarketSync(application.SyncRequest{Scope: application.SyncScopeRepairAll}, &p.previewSync)
	if p.operation != nil || p.sync.previewRead != nil || provider.latestCalls.Load() != 1 {
		t.Fatal("another provider operation was admitted")
	}
	h.click("Cancel refresh", false)
	waitMarketOperations(t, h, func() bool { return p.latest.result != nil && p.latest.result.cancelled && p.request == nil })
	assertPageLabel(t, &h.router, "Latest refresh cancelled.")
	if quote, err := service.CurrentFXQuote(ctx, "JPY", "USD"); quote != nil || err != nil {
		t.Fatalf("cancelled quote = %+v, %v", quote, err)
	}
	h.click("Retry refresh", false)
	waitMarketOperations(t, h, func() bool {
		return p.latest.result != nil && !p.latest.result.cancelled && p.request == nil
	})
	assertPageLabel(t, &h.router, "Updated 1 · Reused 0 · Skipped 0 · Failed 1")
	row := p.current[marketFX].fx[0]
	if row.pair != (marketFXPair{"JPY", "USD"}) || row.quote == nil || row.quote.Rate.Canonical() != "140.4321" ||
		row.quote.SourceKind != domain.QuoteSourceProvider || row.quote.SourceKey != provider.Key() {
		t.Fatalf("FX row was not reloaded with the exact provider evidence: %+v", row)
	}
	if quote, err := service.CurrentFXQuote(ctx, "SGD", "USD"); quote != nil || err != nil {
		t.Fatalf("failed target quote = %+v, %v", quote, err)
	}
	h.click("Latest refresh details", false)
	h.frame()
	assertPageLabel(t, &h.router, "JPY/USD")
	assertPageLabel(t, &h.router, "SGD/USD")
	assertPageLabel(t, &h.router, "Failed")
	h.press(key.NameEscape)
	h.click("More actions", false)
	beforeForce := provider.latestCalls.Load()
	provider.useNewRate.Store(true)
	h.click("Force refresh all", false)
	waitMarketOperations(t, h, func() bool { return p.latest.result != nil && p.request == nil })
	if !p.latest.last.force || provider.latestCalls.Load() <= beforeForce ||
		!slices.ContainsFunc(p.latest.result.result.Items, func(item application.RefreshTargetResult) bool {
			return item.TargetKey == "fx:JPY/USD" && item.Status == application.RefreshFetched
		}) {
		t.Fatalf("force refresh = %+v, result = %+v", p.latest.last, p.latest.result)
	}
	if quote := p.current[marketFX].fx[0].quote; quote == nil || quote.Rate.Canonical() != "140.4322" {
		t.Fatalf("force refresh did not replace displayed evidence: %+v", quote)
	}
	for _, row := range p.current[marketFX].fx {
		if row.source != domain.QuoteSourceProvider {
			t.Fatalf("refresh changed the saved source for %+v", row.pair)
		}
	}
	h.click("Open provider settings", false)
	h.frame()
	if h.shell.activeRoute() != settingsRoute || h.shell.settingsPage.active != settingsMarket {
		t.Fatal("provider shortcut did not open the shared Market data settings")
	}
	h.click("Back", false)
	waitMarketOperations(t, h, func() bool { return h.shell.activeRoute() == marketDataRoute && p.request == nil })
	h.focused(&p.providerSettings)
	if p.tab != marketFX {
		t.Fatalf("Back changed the FX tab to %d", p.tab)
	}
}

func TestMarketDataFocusedSyncPreviewCancelAndVerifiedRetry(t *testing.T) {
	provider := &refreshTestProvider{cancelFirstHistory: true}
	h, p, service, wails := newMarketOperationsHarness(t, provider, true)
	h.click("Preview historical repair", false)
	waitMarketOperations(t, h, func() bool { return p.sync.plan != nil })
	source := p.operation.source
	request := p.operation.request
	want, err := wails.PreviewMarketDataSync(marketdata.SyncRequestDTO{Scope: string(application.SyncScopeFX), CurrencyA: "JPY", CurrencyB: "USD"})
	if err != nil || want.FetchRanges == 0 || request.Scope != application.SyncScopeFX ||
		!reflect.DeepEqual(marketSyncEvidence(*p.sync.plan), want) {
		t.Fatalf("scoped request = %+v, Gio plan = %+v, Wails = %+v, %v", request, p.sync.plan, want, err)
	}
	h.press(key.NameEscape)
	h.frame()
	h.focused(source)
	if _, exists := service.GetCurrentSyncJob(); exists || provider.latestCalls.Load() != 0 || provider.historyCalls.Load() != 0 {
		t.Fatal("preview or dismissal performed provider work")
	}
	h.press(key.NameReturn)
	waitMarketOperations(t, h, func() bool { return p.sync.plan != nil })
	h.click("Start sync", false)
	waitMarketOperations(t, h, func() bool {
		return p.sync.job.Outcome == application.SyncOutcomeRunning && p.sync.job.Current != nil && provider.historyCalls.Load() == 1
	})
	h.click("Historical sync details", false)
	h.frame()
	if p.operation == nil || p.operation.kind != marketSyncDetails || p.sync.job.Current.StartDate == "" {
		t.Fatalf("missing active historical range: %+v", p.sync.job.Current)
	}
	h.press(key.NameEscape)
	h.frame()
	h.focused(&p.syncDetails)
	h.click("Cancel repair", false)
	waitMarketOperations(t, h, func() bool { return p.sync.job.Outcome == application.SyncOutcomeCancelled && p.request == nil })
	first := p.sync.job.JobID
	h.click("Retry sync", false)
	waitMarketOperations(t, h, func() bool { return p.sync.plan != nil })
	if p.sync.plan.Scope != request.Normalized() {
		t.Fatalf("retry changed scope: %+v", p.sync.plan.Scope)
	}
	h.click("Start sync", false)
	waitMarketOperations(t, h, func() bool {
		return p.sync.job.JobID != first && p.sync.job.JobID != "" && p.sync.job.Outcome != application.SyncOutcomeRunning && p.request == nil
	})
	if p.sync.job.Outcome != application.SyncOutcomePartial || p.sync.job.CommittedBatches != 1 || provider.historyCalls.Load() != 2 {
		t.Fatalf("retried sync = %+v, history calls = %d", p.sync.job, provider.historyCalls.Load())
	}
	if row := p.current[marketFX].fx[0]; row.quote == nil || row.quote.SourceKind != domain.QuoteSourceProvider {
		t.Fatalf("completed sync did not refresh the persisted FX row: %+v", row)
	}
	if quote, err := service.CurrentFXQuote(t.Context(), "SGD", "USD"); quote != nil || err != nil {
		t.Fatalf("unrelated FX target = %+v, %v", quote, err)
	}
	quotes, err := service.FXQuoteHistory(t.Context())
	if err != nil || len(quotes) == 0 {
		t.Fatalf("persisted FX history = %+v, %v", quotes, err)
	}
	for _, quote := range quotes {
		if quote.EffectiveDate != "" && quote.Rate.Canonical() == "139.123456" && quote.SourceKey == provider.Key() {
			return
		}
	}
	t.Fatalf("no dated historical observation has the exact provider rate: %+v", quotes)
}

func TestMarketDataInstrumentRefreshAndForcedPlan(t *testing.T) {
	provider := &refreshTestProvider{}
	h, p, service, wails := newMarketOperationsHarness(t, provider, true)
	instrument, err := service.CreateInstrument(t.Context(), application.InstrumentInput{
		Name: "Synthetic provider fund", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider",
		ProviderKey: application.YahooFinanceProviderKey, ProviderSymbol: "DEMO", MarketCode: "US",
	})
	if err != nil {
		t.Fatal(err)
	}
	h.click("Instruments", false)
	waitMarketOperations(t, h, func() bool { return p.tab == marketInstruments && p.request == nil })
	h.click("Update this quote", false)
	waitMarketOperations(t, h, func() bool { return p.latest.result != nil && p.request == nil })
	row := p.current[marketInstruments].instruments[0]
	if row.instrument.ID != instrument.ID || row.quote == nil || row.quote.UnitPrice.Canonical() != "17.12345678" ||
		row.quote.SourceKey != application.YahooFinanceProviderKey || provider.latestCalls.Load() != 0 {
		t.Fatalf("instrument-only refresh read back %+v, FX calls = %d", row, provider.latestCalls.Load())
	}
	h.click("Preview historical repair", false)
	waitMarketOperations(t, h, func() bool { return p.sync.plan != nil })
	want, err := wails.PreviewMarketDataSync(marketdata.SyncRequestDTO{Scope: string(application.SyncScopeInstrument), InstrumentID: instrument.ID.String()})
	if err != nil || !reflect.DeepEqual(marketSyncEvidence(*p.sync.plan), want) {
		t.Fatalf("Gio plan = %+v, Wails = %+v, %v", p.sync.plan, want, err)
	}
	if p.operation.request.InstrumentID != instrument.ID.String() {
		t.Fatalf("instrument target changed: %+v", p.operation.request)
	}
	h.press(key.NameEscape)
	h.click("More actions", false)
	h.click("Sync and repair", false)
	waitMarketOperations(t, h, func() bool { return p.sync.plan != nil })
	h.click("Force historical recheck", false)
	waitMarketOperations(t, h, func() bool { return p.sync.plan != nil && p.sync.plan.Scope.ForceRecheck })
	if p.sync.plan.Scope.Scope != application.SyncScopeRepairAll {
		t.Fatalf("global forced plan = %+v", p.sync.plan)
	}
	h.press(key.NameEscape)
	h.frame()
	h.focused(&p.previewSync)
	if _, exists := service.GetCurrentSyncJob(); exists || provider.historyCalls.Load() != 0 {
		t.Fatal("calculating the previews started a job")
	}
}
