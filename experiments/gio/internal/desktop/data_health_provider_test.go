package desktop

import (
	"errors"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"image"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/marketdata"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	marketapi "github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
	settingsapi "github.com/waltwang/nestworth-go/internal/wailsapi/settings"
)

type providerReadinessTransport struct{ calls atomic.Int32 }

func (p *providerReadinessTransport) RoundTrip(*http.Request) (*http.Response, error) {
	p.calls.Add(1)
	return nil, errors.New("a local provider-readiness check attempted HTTP")
}

func providerReadinessRegistry(keySource func() (string, error), network *providerReadinessTransport) *application.MarketDataRegistry {
	fx := marketdata.NewFrankfurterProvider(network)
	return application.NewMarketDataRegistryWithDefault(fx.Key(), fx, marketdata.NewCoinGeckoProvider(keySource, network))
}

func seedStage6ProviderSettings(t *testing.T, start time.Time) ([]byte, domain.InstrumentID) {
	t.Helper()
	ctx := t.Context()
	transport := new(providerReadinessTransport)
	now := start
	db, _, seed := openStage6Database(t, filepath.Join(t.TempDir(), "provider-settings.db"), func() time.Time { return now },
		providerReadinessRegistry(nil, transport))
	if err := seed.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic provider readiness", BaseCurrency: "USD", MemberNames: []string{"Fixture owner"},
	}); err != nil {
		t.Fatal(err)
	}
	owners, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := testfixture.CreateAccount(seed, ctx, application.AccountInput{
		Name: "Synthetic wallet", AccountType: "brokerage", TrackingMode: "holdings", BalanceSheetRole: "asset", DefaultCurrency: "USD",
		IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{owners.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := testfixture.CreateInstrument(seed, ctx, application.InstrumentInput{
		Name: "Synthetic CoinGecko holding", Type: "crypto", Symbol: "BTC", QuoteCurrency: "USD", QuoteSource: "provider",
		ProviderKey: domain.CoinGeckoProviderKey, ProviderSymbol: "bitcoin", MarketCode: "CRYPTO",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testfixture.CaptureSyntheticOrigin(seed, ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	if _, err := testfixture.CreateHolding(seed, ctx, application.HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1", UnitCost: "10"}); err != nil {
		t.Fatal(err)
	}
	// Overview can materialize derived snapshots. Seed those same closed days
	// before copying so both Data Health reads start with identical evidence.
	now = start.AddDate(0, 0, 4)
	from, to := start.Format(time.DateOnly), now.AddDate(0, 0, -1).Format(time.DateOnly)
	if count, err := seed.RebuildHistoricalSnapshots(ctx, from, to); err != nil || count != 4 {
		t.Fatalf("materialized %d of four synthetic days: %v", count, err)
	}
	if transport.calls.Load() != 0 {
		t.Fatal("the synthetic provider settings fixture attempted provider HTTP")
	}
	return stage6SeedBytes(t, db), instrument.ID
}

func TestDataHealthCredentialHandoffRechecksSameProviderOnBothDesktops(t *testing.T) {
	ctx := t.Context()
	seed, instrument := seedStage6ProviderSettings(t, accountTestClock)
	clock := func() time.Time { return accountTestClock.AddDate(0, 0, 4) }
	root := t.TempDir()
	gioPath := writeStage6Fixture(t, filepath.Join(root, "gio"), seed)
	wailsPath := writeStage6Fixture(t, filepath.Join(root, "wails"), seed)
	gioDB, _, gio := openStage6Database(t, gioPath, clock)
	wailsDB, _, wails := openStage6Database(t, wailsPath, clock)
	gioSettings, err := preferences.Open(filepath.Join(filepath.Dir(gioPath), "settings.json"), gioDB, gio)
	if err != nil {
		t.Fatal(err)
	}
	wailsStore := settings.NewStore(filepath.Join(filepath.Dir(wailsPath), "settings.json"))
	if err := wailsStore.Attach(sqlite.NewConfigurationRepository(wailsDB)); err != nil {
		t.Fatal(err)
	}
	gioTransport, wailsTransport := new(providerReadinessTransport), new(providerReadinessTransport)
	gio.SetMarketDataRegistry(providerReadinessRegistry(gioSettings.CoinGeckoKey, gioTransport))
	wails.SetMarketDataRegistry(providerReadinessRegistry(func() (string, error) {
		saved, err := wailsStore.Load()
		return saved.CoinGeckoAPIKey, err
	}, wailsTransport))
	wailsSettings, wailsHealth := settingsapi.NewService(wailsStore, wails), marketapi.NewService(wails, nil)
	t.Cleanup(wailsHealth.CancelAllAndWait)
	prefs := newPreferenceState(ctx, gioSettings, func() {})
	prefs.reload()
	prefs.wait()
	overview := newOverviewPage(ctx, gio, clock, func() {})
	health := &dataHealthPage{ctx: ctx, reader: gio, invalidate: func() {}}
	t.Cleanup(overview.workers.Wait)
	t.Cleanup(health.workers.Wait)
	t.Cleanup(prefs.wait)
	h := newShellHarnessWithPages(t, ledgerPages{prefs: prefs, overview: overview, dataHealth: health})
	h.size = image.Pt(1230, 1000)
	settle := func() {
		for range 5 {
			h.frame()
			overview.workers.Wait()
			health.workers.Wait()
			prefs.wait()
		}
		h.frame()
	}
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		settle()
	}
	providerFinding := func(issue application.HealthIssue) bool {
		return issue.Kind == application.HealthKindMissingProviderKey && issue.Provider == domain.CoinGeckoProviderKey
	}
	compare := func(report *application.MarketDataHealthReport, missing bool) {
		t.Helper()
		want, err := wailsHealth.ScanMarketDataHealth()
		if err != nil || report == nil || !reflect.DeepEqual(marketapi.HealthReportDTO(*report), want) {
			t.Fatalf("different local provider diagnoses: %+v / %+v / %v", report, want, err)
		}
		if got := slices.ContainsFunc(report.Issues, providerFinding); got != missing {
			t.Fatalf("missing-provider prerequisite = %v, want %v", got, missing)
		}
	}
	settle()
	compare(overview.healthScan.report, true)
	at := slices.IndexFunc(overview.healthScan.report.Issues, providerFinding)
	issue := overview.healthScan.report.Issues[at]
	overview.list.ScrollTo(overview.trendIndex() - 1)
	settle()
	// Open a scoped Issues visit. The ordinary Overview provider action now
	// opens Settings directly, and is covered by the Overview journey.
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: dataHealthRoute, health: healthFindingFocus{
		id: issue.ID, label: issue.Label, from: issue.RangeStart, to: issue.RangeEnd,
	}}, focus: overview.healthScan.links[issue.ID]}
	settle()
	if h.shell.activeRoute() != dataHealthRoute || health.focus.id != issue.ID {
		t.Fatalf("did not enter the exact provider finding: %+v", health.focus)
	}
	h.size = image.Pt(700, 520)
	settle()
	click("Open provider settings")
	page := h.shell.settingsPage
	if h.shell.activeRoute() != settingsRoute || page.active != settingsMarket {
		t.Fatal("the provider prerequisite did not open Market data settings")
	}
	h.focused(&page.credential.input.Editor)
	bounds := semanticBounds(t, &h.router, "CoinGecko API key", true)
	if bounds.Min.Y < headerHeight || bounds.Max.Y > h.size.Y {
		t.Fatalf("the focused credential was outside the compact viewport: %v", bounds)
	}
	click("Back")
	compare(health.report, true)
	h.focused(health.issueActions[issue.ID])
	h.press(key.NameReturn)
	settle()
	if h.shell.activeRoute() != settingsRoute {
		t.Fatal("the restored provider action did not reopen its settings")
	}
	h.focused(&page.credential.input.Editor)
	const dummyKey = "synthetic-readiness-key"
	h.router.Queue(key.EditEvent{Text: dummyKey})
	settle()
	if page.credential.input.Editor.Mask == 0 {
		t.Fatal("the prerequisite opened an unmasked credential editor")
	}
	click("Save key")
	if !prefs.saved.CoinGeckoConfigured || page.credential.input.Text() != "" {
		t.Fatal("credential save did not publish readiness and clear input")
	}
	if status, err := wailsSettings.SaveCoinGeckoAPIKey(dummyKey); err != nil || !status.Configured {
		t.Fatalf("Wails did not save its independent synthetic credential: %+v / %v", status, err)
	}
	click("Back")
	compare(health.report, false)
	if !slices.ContainsFunc(health.report.Issues, func(remaining application.HealthIssue) bool {
		return remaining.InstrumentID == instrument.String() && remaining.Executable && !remaining.Collapsed
	}) {
		t.Fatal("the shared scan did not expose the existing instrument repair after configuration")
	}
	assertPageLabel(t, &h.router, "No saved findings match this focus.")
	h.focused(&health.recheck)
	click("Back")
	compare(overview.healthScan.report, false)

	click("Forward")
	click("Forward")
	click("Remove key")
	if status, err := wailsSettings.DeleteCoinGeckoAPIKey(); err != nil || status.Configured {
		t.Fatalf("Wails did not remove its synthetic credential: %+v / %v", status, err)
	}
	click("Back")
	compare(health.report, true)
	click("Back")
	compare(overview.healthScan.report, true)
	if gioTransport.calls.Load()+wailsTransport.calls.Load() != 0 {
		t.Fatal("reading or changing local provider configuration attempted provider HTTP")
	}
}
