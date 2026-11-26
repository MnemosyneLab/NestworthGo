package desktop

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestPortfolioInstrumentActionsRetainDatesAccountAndFocus(t *testing.T) {
	ctx := t.Context()
	clock := func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }
	seed, account, instrument, receipt := seedStage6Contribution(t, time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC))
	path := writeStage6Fixture(t, filepath.Join(t.TempDir(), "instrument-actions"), seed)
	_, _, service := openStage6Database(t, path, clock)
	portfolio := newPortfolioPage(ctx, service, func() {})
	market := &marketDataPage{ctx: ctx, reader: service, invalidate: func() {}, quoteHistoryReader: service, quoteHistoryClock: clock}
	timeline := newLedgerHistory(ctx, service, clock, func() {})
	returns := &returnAnalysisPage{ctx: ctx, reader: service, clock: clock, invalidate: func() {}}
	assets := &assetChangesPage{ctx: ctx, reader: service, clock: clock, invalidate: func() {}}
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: portfolio, marketData: market, history: timeline, returns: returns, assets: assets})
	h.size = image.Pt(1280, 900)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() {
		portfolio.workers.Wait()
		market.workers.Wait()
		timeline.reads.workers.Wait()
		timeline.model.Wait()
		returns.workers.Wait()
		assets.workers.Wait()
	}
	t.Cleanup(wait)
	settle := func() {
		for range 4 {
			h.frame()
			wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	clickTab := func(tab instrumentDetailSection) {
		t.Helper()
		click(i18n.Language(settings.LanguageEnglish).Text(instrumentDetailLabels[tab]))
	}
	activate := func(tag event.Tag) {
		t.Helper()
		h.router.Source().Execute(key.FocusCmd{Tag: tag})
		h.frame()
		h.focused(tag)
		h.press(key.NameReturn)
		settle()
	}
	assertDetail := func(holdingAccount domain.AccountID, focus event.Tag) {
		t.Helper()
		if h.shell.activeRoute() != portfolioRoute || portfolio.id != instrument || portfolio.accountScope != holdingAccount ||
			portfolio.detail.visit.section != instrumentDetailHoldings {
			t.Fatal("Back lost the original instrument, account or Holdings tab")
		}
		h.focused(focus)
	}
	assertAnalysis := func(route routeID, window analysisDateWindow, holdingAccount domain.AccountID) {
		t.Helper()
		if h.shell.activeRoute() != route {
			t.Fatalf("instrument action opened %s, want %s", h.shell.activeRoute(), route)
		}
		selection := returns.filters.selection
		if route == returnAnalysisRoute {
			if returns.tab != returnTrendTab {
				t.Fatal("instrument return did not open the Trend tab")
			}
		} else {
			selection = assets.filters.selection
			if assets.state.tab != assetDriversTab {
				t.Fatal("instrument asset changes did not open the Drivers tab")
			}
		}
		want := analysisSelection{
			scope:     domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: instrument.String()},
			valuation: domain.ValuationBase, excludeCash: true, account: holdingAccount.String(),
			from: domain.LocalDate(window.from), to: domain.LocalDate(window.to),
		}
		if selection != want {
			t.Fatalf("instrument analysis selection = %+v, want %+v", selection, want)
		}
	}

	recent := analysisDateWindow{from: "2026-10-02", to: "2026-10-04"}
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: returnAnalysisRoute},
		returns: &returnAnalysisVisit{tab: returnTrendTab, selection: analysisSelection{
			scope:     domain.AnalysisScope{Kind: domain.ScopeAccount, ID: account.String()},
			valuation: domain.ValuationNative, from: domain.LocalDate(recent.from), to: domain.LocalDate(recent.to), currency: "EUR",
		}}}
	settle()
	portfolio.openHoldingInstrument(instrument, account, "Synthetic euro fund", nil)
	settle()

	clickTab(instrumentDetailData)
	if q := portfolio.detail.histories[instrumentPriceHistory]; q == nil || q.result == nil ||
		portfolio.detail.visit.actionDates != (analysisDateWindow{}) {
		t.Fatal("the default quote period was treated as an explicitly chosen range")
	}
	clickTab(instrumentDetailHoldings)
	click("Related transactions")
	if h.shell.activeRoute() != historyRoute || !timeline.model.Filters().Equal(history.Filters{Instrument: instrument, Account: account}) {
		t.Fatalf("instrument transactions inherited unrelated analysis dates: %+v", timeline.model.Filters())
	}
	var scopedDividendCount int
	for _, row := range timeline.model.Rows() {
		if row.Kind != domain.ActivityCashDividend {
			continue
		}
		scopedDividendCount++
		if row.ID.String() != receipt.ID {
			t.Fatalf("account-scoped transactions included an unrelated dividend: %s", row.ID)
		}
	}
	if scopedDividendCount != 1 {
		t.Fatalf("account-scoped transactions did not load the expected dividend: %+v", timeline.model.Rows())
	}
	click("Back")
	assertDetail(account, &portfolio.detail.transactions)
	activate(&portfolio.detail.returns)
	assertAnalysis(returnAnalysisRoute, recent, account)
	click("Back")
	assertDetail(account, &portfolio.detail.returns)

	clickTab(instrumentDetailData)
	click("Last week")
	quoteDates := portfolio.detail.histories[instrumentPriceHistory].visit.window
	if quoteDates.from == "" || quoteDates.to == "" || quoteDates == recent || portfolio.detail.visit.actionDates != quoteDates {
		t.Fatalf("changing quote dates did not update the instrument action range: %+v", quoteDates)
	}
	clickTab(instrumentDetailHoldings)
	activate(&portfolio.detail.changes)
	assertAnalysis(assetChangesRoute, quoteDates, account)
	click("Back")
	assertDetail(account, &portfolio.detail.changes)
	activate(&portfolio.detail.returns)
	assertAnalysis(returnAnalysisRoute, quoteDates, account)
	click("Back")
	assertDetail(account, &portfolio.detail.returns)
	click("Related transactions")
	filters := timeline.model.Filters()
	if h.shell.activeRoute() != historyRoute || !filters.Equal(history.Filters{Instrument: instrument, Account: account, From: quoteDates.from, To: quoteDates.to}) {
		t.Fatalf("instrument Ledger filters did not retain account, instrument and dates: %+v", filters)
	}
	click("Back")
	assertDetail(account, &portfolio.detail.transactions)

	sourceDates := analysisDateWindow{from: "2026-10-02", to: "2026-10-02"}
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: marketDataRoute, health: healthFindingFocus{
		instrument: instrument.String(), from: sourceDates.from, to: sourceDates.to,
	}}}
	settle()
	if portfolio.detail.visit.section != instrumentDetailData || portfolio.detail.visit.actionDates != sourceDates {
		t.Fatal("source entry did not seed its instrument action range")
	}
	clickTab(instrumentDetailHoldings)
	activate(&portfolio.detail.changes)
	assertAnalysis(assetChangesRoute, sourceDates, "")
	click("Back")
	assertDetail("", &portfolio.detail.changes)
	click("Related transactions")
	if filters := timeline.model.Filters(); !filters.Equal(history.Filters{Instrument: instrument, From: sourceDates.from, To: sourceDates.to}) {
		t.Fatalf("household instrument Ledger entry used the wrong dates or inherited an account: %+v", filters)
	}
	if rows := timeline.model.Rows(); len(rows) != 2 || rows[0].Kind != domain.ActivityCashDividend || rows[1].Kind != domain.ActivityCashDividend {
		t.Fatalf("household instrument transactions did not load both accounts' dividends: %+v", rows)
	}
	click("Back")
	assertDetail("", &portfolio.detail.transactions)
}
