package desktop

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
	"github.com/waltwang/nestworth-go/internal/wailsapi/quote"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func seedStage6Market(t *testing.T, start time.Time, withInstrument bool) ([]byte, domain.InstrumentID) {
	t.Helper()
	ctx := t.Context()
	db, _, svc := openStage6Database(t, filepath.Join(t.TempDir(), "source.db"), func() time.Time { return start },
		application.NewMarketDataRegistry(&unavailableOverviewProvider{}))
	if err := svc.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic Stage 6 market", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := svc.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, currency := range []string{"JPY", "SGD"} {
		_, err := testfixture.CreateAccount(svc, ctx, application.AccountInput{
			Name: "Synthetic " + currency, AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
			DefaultCurrency: currency, InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testfixture.SetFXPreference(svc, ctx, currency, "USD", "provider"); err != nil {
			t.Fatal(err)
		}
	}
	var id domain.InstrumentID
	if withInstrument {
		instrument, err := testfixture.CreateInstrument(svc, ctx, application.InstrumentInput{
			Name: "Synthetic provider fund", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider",
			ProviderKey: application.YahooFinanceProviderKey, ProviderSymbol: "DEMO", MarketCode: "US",
		})
		if err != nil {
			t.Fatal(err)
		}
		id = instrument.ID
	}
	if _, err := testfixture.CaptureSyntheticOrigin(svc, ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	return stage6SeedBytes(t, db), id
}

type stage6LatestFXProvider struct {
	refreshTestProvider
	recoverSGD atomic.Bool
	sgdCalls   atomic.Int32
}

func (p *stage6LatestFXProvider) LatestFX(ctx context.Context, id application.FXMarketIdentity) (application.LatestFXQuote, error) {
	if err := ctx.Err(); err != nil {
		return application.LatestFXQuote{}, err
	}
	if id.BaseCurrency != "SGD" && id.QuoteCurrency != "SGD" {
		return p.refreshTestProvider.LatestFX(ctx, id)
	}
	p.sgdCalls.Add(1)
	if !p.recoverSGD.Load() {
		return application.LatestFXQuote{}, &domain.Error{Code: domain.ErrProviderUnavailable}
	}
	rate, err := domain.ParseFxRate("1.3456")
	return application.LatestFXQuote{Rate: rate, BaseCurrency: id.BaseCurrency, QuoteCurrency: id.QuoteCurrency,
		SourceKey: p.Key(), QuotedAt: p.now.Add(-time.Minute)}, err
}

type stage6LatestInstrumentProvider struct {
	refreshInstrumentTestProvider
	calls atomic.Int32
	now   time.Time
}

func (p *stage6LatestInstrumentProvider) LatestInstrument(ctx context.Context, id application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	if err := ctx.Err(); err != nil {
		return application.LatestInstrumentQuote{}, err
	}
	p.calls.Add(1)
	value, err := p.refreshInstrumentTestProvider.LatestInstrument(ctx, id)
	value.QuotedAt = p.now.Add(-time.Minute)
	return value, err
}

type stage6RefreshEvents chan marketdata.RefreshCompletedPayload

func (events stage6RefreshEvents) Emit(name string, data any) {
	if name == marketdata.RefreshCompletedEvent {
		events <- data.(marketdata.RefreshCompletedPayload)
	}
}

func (events stage6RefreshEvents) await(t *testing.T, id, status string) *marketdata.RefreshResultDTO {
	t.Helper()
	select {
	case event := <-events:
		if event.RequestID != id || event.Status != status {
			t.Fatalf("Wails completion = %+v, want %s %s", event, id, status)
		}
		return event.Result
	case <-time.After(5 * time.Second):
		t.Fatal("Wails did not emit the refresh outcome")
		return nil
	}
}

func TestStage6PairedMixedLatestRefresh(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	now := time.Date(2026, 11, 6, 18, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	seed, instrument := seedStage6Market(t, now.AddDate(0, 0, -4), true)
	gioPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "gio"), seed)
	wailsPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "wails"), seed)
	newProviders := func() (*stage6LatestFXProvider, *stage6LatestInstrumentProvider, *application.MarketDataRegistry) {
		fx := &stage6LatestFXProvider{refreshTestProvider: refreshTestProvider{cancelFirstLatest: true, now: now}}
		prices := &stage6LatestInstrumentProvider{now: now}
		return fx, prices, application.NewMarketDataRegistryWithDefault(fx.Key(), fx, prices)
	}
	gioFX, gioPrices, gioRegistry := newProviders()
	wailsFX, wailsPrices, wailsRegistry := newProviders()
	gioDB, _, gio := openStage6Database(t, gioPath, clock, gioRegistry)
	wailsDB, _, wails := openStage6Database(t, wailsPath, clock, wailsRegistry)
	events := make(stage6RefreshEvents, 8)
	adapter := marketdata.NewService(wails, events)
	t.Cleanup(adapter.CancelAllAndWait)
	page := &marketDataPage{ctx: ctx, reader: gio, quoteHistoryReader: gio, quoteHistoryClock: clock, invalidate: func() {},
		sync: &dataHealthRepair{service: gio}, latest: marketLatest{service: gio}}
	t.Cleanup(func() { stop(); page.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{marketData: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.trail = newNavigationTrail(marketDataRoute)
	page.enter(marketDataVisit{})
	waitMarketOperations(t, h, func() bool { return page.current[marketInstruments] != nil })
	h.frame()

	capture := func(s *application.Service) (*wire.InstrumentQuoteDTO, *wire.FXQuoteDTO, *wire.FXQuoteDTO) {
		t.Helper()
		api := quote.NewService(s)
		price, err := api.CurrentInstrumentQuote(ctx, instrument.String())
		if err != nil {
			t.Fatal(err)
		}
		jpy, err := api.CurrentFXQuote(ctx, "JPY", "USD")
		if err != nil {
			t.Fatal(err)
		}
		sgd, err := api.CurrentFXQuote(ctx, "SGD", "USD")
		if err != nil {
			t.Fatal(err)
		}
		return price, jpy, sgd
	}
	assertNoQuotes := func(s *application.Service) {
		t.Helper()
		if price, jpy, sgd := capture(s); price != nil || jpy != nil || sgd != nil {
			t.Fatal("cancelled refresh stored an observation")
		}
	}
	h.click("Update latest prices", false)
	adapter.StartRefreshMissingOrStale("cancel")
	waitMarketOperations(t, h, func() bool { return gioFX.latestCalls.Load() == 1 && wailsFX.latestCalls.Load() == 1 })
	h.click("Cancel refresh", false)
	adapter.CancelRefresh("cancel")
	events.await(t, "cancel", "cancelled")
	adapter.CancelAllAndWait()
	waitMarketOperations(t, h, func() bool { return page.latest.result != nil && page.latest.result.cancelled && page.request == nil })
	assertNoQuotes(gio)
	assertNoQuotes(wails)

	terminal := func() {
		waitMarketOperations(t, h, func() bool { return page.latest.result != nil && !page.latest.result.cancelled && page.request == nil })
	}
	assertResults := func(ref *marketdata.RefreshResultDTO, status ...application.RefreshStatus) {
		t.Helper()
		got := page.latest.result.result
		targets := []string{"fx:JPY/USD", "fx:SGD/USD", "instrument:" + instrument.String()}
		if ref == nil || got.RateLimited || ref.RateLimited || len(got.Items) != len(targets) || len(ref.Items) != len(targets) {
			t.Fatalf("unpaired mixed refresh = %+v, Wails = %+v", got, ref)
		}
		for i, target := range targets {
			g, w := got.Items[i], ref.Items[i]
			if g.TargetKey != target || w.TargetKey != target || g.Status != status[i] || w.Status != string(g.Status) ||
				w.Kind != string(g.Kind) || w.ErrorCode != string(g.ErrorCode) {
				t.Fatalf("target %s: Gio %+v, Wails %+v", target, g, w)
			}
			if g.Status == application.RefreshFailed && g.ErrorCode != domain.ErrProviderUnavailable {
				t.Fatalf("unexpected failure for %s: %s", target, g.ErrorCode)
			}
		}
	}
	h.click("Retry refresh", false)
	adapter.StartRefreshMissingOrStale("partial")
	assertResultsAfterPartial := events.await(t, "partial", "completed")
	terminal()
	assertResults(assertResultsAfterPartial, application.RefreshFetched, application.RefreshFailed, application.RefreshFetched)
	assertPageLabel(t, &h.router, "Updated 2 · Reused 0 · Skipped 0 · Failed 1")
	for _, s := range []*application.Service{gio, wails} {
		price, jpy, sgd := capture(s)
		if price == nil || price.UnitPrice != "17.12345678" || jpy == nil || jpy.Rate != "140.4321" || sgd != nil {
			t.Fatal("partial refresh failed to retain both successful targets and the missing FX")
		}
	}

	gioFX.recoverSGD.Store(true)
	wailsFX.recoverSGD.Store(true)
	h.click("Retry refresh", false)
	adapter.StartRefreshMissingOrStale("retry")
	ref := events.await(t, "retry", "completed")
	terminal()
	assertResults(ref, application.RefreshCached, application.RefreshFetched, application.RefreshCached)
	assertPageLabel(t, &h.router, "Updated 1 · Reused 2 · Skipped 0 · Failed 0")
	for _, fx := range []*stage6LatestFXProvider{gioFX, wailsFX} {
		if fx.latestCalls.Load() != 2 || fx.sgdCalls.Load() != 2 {
			t.Fatal("retry requested a successful FX again or omitted the failed target")
		}
	}
	if gioPrices.calls.Load() != 1 || wailsPrices.calls.Load() != 1 {
		t.Fatal("failed-target retry fetched the successful instrument again")
	}
	h.click("View quote history", false)
	waitMarketOperations(t, h, func() bool { return page.quoteHistory != nil && page.quoteHistory.result != nil })
	series, err := quote.NewService(wails).InstrumentQuoteSeries(ctx, instrument.String(), string(domain.Trend30Days), "all")
	actual := page.quoteHistory.result.series.Observations
	if err != nil || len(actual) != 1 || len(series.Observations) != 1 {
		t.Fatalf("paired quote history = %+v / %+v (%v)", actual, series.Observations, err)
	}
	point, reference := actual[0], series.Observations[0]
	if point.Value != reference.Value || point.Currency.String() != reference.Currency || point.SourceKey != reference.SourceKey ||
		string(point.SourceKind) != reference.SourceKind || wire.FormatTime(point.QuotedAt) != reference.QuotedAt ||
		point.EffectiveDate != reference.EffectiveDate || point.ObservationKind != reference.ObservationKind ||
		point.PriceBasis != reference.PriceBasis || point.TimestampBasis != reference.TimestampBasis || point.Delayed != reference.Delayed {
		t.Fatalf("Gio and Wails quote provenance differs: %+v / %+v", point, reference)
	}
	source := page.quoteHistory.visit.source
	h.press(key.NameEscape)
	h.frame()
	h.focused(source)

	for _, profile := range []struct {
		close func() error
		path  string
	}{{gioDB.Close, gioPath}, {wailsDB.Close, wailsPath}} {
		if err := profile.close(); err != nil {
			t.Fatal(err)
		}
		_, _, saved := openStage6Database(t, profile.path, clock)
		price, jpy, sgd := capture(saved)
		if price == nil || jpy == nil || sgd == nil || price.UnitPrice != "17.12345678" ||
			jpy.Rate != "140.4321" || sgd.Rate != "1.3456" || price.SourceKey != application.YahooFinanceProviderKey ||
			jpy.SourceKey != application.FrankfurterProviderKey || sgd.SourceKey != jpy.SourceKey ||
			price.QuotedAt != wire.FormatTime(now.Add(-time.Minute)) || jpy.QuotedAt != price.QuotedAt || sgd.QuotedAt != price.QuotedAt {
			t.Fatalf("stored observations changed after reopen: %+v %+v %+v", price, jpy, sgd)
		}
	}
}
