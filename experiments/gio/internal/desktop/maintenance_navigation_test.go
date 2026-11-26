package desktop

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestMaintenanceTargetsDistinguishRecordsFindingsAndProviders(t *testing.T) {
	instrument, account := domain.NewInstrumentID(), domain.NewAccountID()
	plain := healthFindingFocus{instrument: instrument.String(), account: account.String(), label: "Synthetic instrument",
		from: "2026-11-18", to: "2026-11-21"}
	request := navigationRequest{target: navigationTarget{route: marketDataRoute, health: plain}}
	got := resolveMaintenanceTarget(request)
	if got.target != (navigationTarget{route: portfolioRoute, instrument: instrument, portfolioAccount: account, instrumentData: true, health: plain}) ||
		got.name != "Synthetic instrument" {
		t.Fatalf("legacy record scope was changed: %+v", got)
	}
	for _, focus := range []healthFindingFocus{
		{instrument: plain.instrument, id: "synthetic-gap"}, {instrument: plain.instrument, kind: application.HealthKindMissingBinding},
		{instrument: plain.instrument, action: application.HealthActionRepair}, {instrument: plain.instrument, reason: "agent_data_required"},
		{instrument: plain.instrument, executableSpecified: true}, {currencyA: "USD", currencyB: "EUR", from: plain.from, to: plain.to},
	} {
		request.target.health = focus
		if resolved := resolveMaintenanceTarget(request); resolved.target != request.target {
			t.Fatalf("repair or FX was routed away from maintenance: %+v", resolved)
		}
	}
	for _, route := range []routeID{marketDataRoute, dataHealthRoute} {
		focus := healthFindingFocus{instrument: plain.instrument, action: application.HealthActionProviderSettings, provider: domain.CoinGeckoProviderKey}
		got := resolveMaintenanceTarget(navigationRequest{target: navigationTarget{route: route, health: focus}})
		if got.target.route != settingsRoute || !got.providerSettings || got.settingsProvider != domain.CoinGeckoProviderKey {
			t.Fatalf("provider left the Settings routing path: %+v", got)
		}
	}
}

func TestMaintenanceTabsAndInstrumentRecordsRetainNavigationContext(t *testing.T) {
	ctx := t.Context()
	service, _, _, ids := openPortfolioFixture(t, "partial")
	instrument := ids.instruments["QA-FULL"]
	clock := func() time.Time { return accountTestClock }
	portfolio := newPortfolioPage(ctx, service, func() {})
	market := &marketDataPage{ctx: ctx, reader: service, invalidate: func() {}, quoteHistoryReader: service, quoteHistoryClock: clock}
	health := &dataHealthPage{ctx: ctx, reader: service, invalidate: func() {}}
	h := newShellHarnessWithPages(t, ledgerPages{marketData: market, dataHealth: health, portfolio: portfolio})
	h.size = image.Pt(1250, 1080)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 4 {
			h.frame()
			health.workers.Wait()
			market.workers.Wait()
			portfolio.workers.Wait()
		}
		h.frame()
	}
	t.Cleanup(func() { health.workers.Wait(); market.workers.Wait(); portfolio.workers.Wait() })
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	route := func(want routeID) {
		t.Helper()
		if got := h.shell.activeRoute(); got != want {
			t.Fatalf("active route = %s, want %s", got, want)
		}
	}
	tab := func(name string, selected bool) {
		t.Helper()
		if node := findPageLabel(&h.router, name); node == nil || node.Desc.Selected != selected {
			t.Fatalf("unexpected %q tab: %+v", name, node)
		}
	}
	settle()
	click("Data maintenance")
	route(marketDataRoute)
	tab("Issues & repair", false)
	tab("Quotes & FX", true)
	click("Issues & repair")
	route(dataHealthRoute)
	tab("Issues & repair", true)
	tab("Quotes & FX", false)
	click("Quotes & FX")
	route(marketDataRoute)
	tab("Issues & repair", false)
	tab("Quotes & FX", true)
	h.focused(&h.shell.maintenanceTabs[1])
	h.click("Search instruments", true)
	h.router.Queue(key.EditEvent{Text: "QA-FULL"})
	settle()
	source := &market.search.Field.Editor
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: marketDataRoute, health: healthFindingFocus{
		instrument: instrument.ID.String(), account: ids.account.String(), label: instrument.DisplayLabel(), from: "2026-11-18", to: "2026-11-21",
	}}, focus: source}
	settle()
	route(portfolioRoute)
	data := portfolio.detail.histories[instrumentPriceHistory]
	window := analysisDateWindow{from: "2026-11-18", to: "2026-11-21"}
	if portfolio.detail.visit.section != instrumentDetailData || portfolio.accountScope != ids.account || data == nil ||
		data.result == nil || data.visit.window != window || data.visit.target.instrument != instrument.ID {
		t.Fatalf("instrument records did not retain account or dates: %+v", data)
	}
	click("Maintain data")
	route(marketDataRoute)
	tab("Quotes & FX", true)
	focus := h.shell.trail.current().target.health
	if focus.instrument != instrument.ID.String() || focus.account != ids.account.String() ||
		focus.action != application.HealthActionInstrumentEditor || (analysisDateWindow{from: focus.from, to: focus.to}) != window {
		t.Fatalf("Maintain data changed instrument, dates or account: %+v", focus)
	}
	click("Back")
	route(portfolioRoute)
	h.focused(&portfolio.detail.maintain)
	if data := portfolio.detail.histories[instrumentPriceHistory]; data == nil || data.visit.window != window || portfolio.accountScope != ids.account {
		t.Fatal("Back lost the read-only instrument records visit")
	}
	click("Back")
	route(marketDataRoute)
	h.focused(source)
	if text := market.search.Field.Text(); text != "QA-FULL" {
		t.Fatalf("Back lost the quote search: %q", text)
	}
	click("Back")
	route(dataHealthRoute)
	h.focused(&h.shell.maintenanceTabs[1])
	tab("Issues & repair", true)
	click("Forward")
	route(marketDataRoute)
	tab("Quotes & FX", true)
	h.focused(source)

	fx := healthFindingFocus{currencyA: "EUR", currencyB: "USD", id: "synthetic-fx-gap", from: window.from, to: window.to,
		action: application.HealthActionManualEntry}
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: marketDataRoute, health: fx}, focus: source}
	settle()
	route(marketDataRoute)
	tab("Quotes & FX", true)
	if market.tab != marketFX || h.shell.trail.current().target.health != fx {
		t.Fatal("the scoped FX finding did not open the FX list under Quotes & FX")
	}
	click("Issues & repair")
	click("Back")
	route(marketDataRoute)
	if market.tab != marketFX || h.shell.trail.current().target.health != fx {
		t.Fatal("Back lost the focused FX visit")
	}
}
