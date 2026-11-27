package desktop

import (
	"image"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"

	historyui "github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
)

func TestAccountPositionsFilterAndScopedInstrumentHistory(t *testing.T) {
	ctx := t.Context()
	service, _, _, fixture := openPortfolioFixture(t, "complete")
	current := fixture.instruments["QA-FULL"]
	closed, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Closed example fund", Symbol: "QA-CLOSED", Type: "etf", QuoteCurrency: "EUR", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	second := slices.IndexFunc(records, func(r domain.AccountRecord) bool { return r.Account.Name == "Second Brokerage" })
	if second < 0 {
		t.Fatal("second brokerage not seeded")
	}
	for account, quantity := range map[domain.AccountID]string{fixture.account: "0", records[second].Account.ID: "1"} {
		if _, err := service.CreateHolding(ctx, application.HoldingInput{
			AccountID: account.String(), InstrumentID: closed.ID.String(), Quantity: quantity, UnitCost: "2",
		}); err != nil {
			t.Fatal(err)
		}
	}
	want, err := analytics.NewService(service).AccountHoldingGroups(ctx, fixture.account.String())
	if err != nil || len(want) != 1 || len(want[0].Instruments) != 2 || len(want[0].Subtotals) != 1 {
		t.Fatalf("Wails holding groups: %+v / %v", want, err)
	}
	clock := func() time.Time { return accountTestClock }
	accounts := newAccountsPage(ctx, service, clock, func() {})
	investments := newPortfolioPage(ctx, service, func() {})
	ledger := newLedgerHistory(ctx, service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, portfolio: investments, history: ledger})
	h.size = image.Pt(1100, 1400)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 3 {
			h.frame()
			accounts.workers.Wait()
			investments.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		settle()
	}
	missing := func(label string) {
		t.Helper()
		if findPageLabel(&h.router, label) != nil {
			t.Fatalf("unexpected visible label %q", label)
		}
	}
	h.frame()
	click("Accounts")
	click("Harbor Brokerage")
	const accountActions = "More actions for Harbor Brokerage"
	h.assertTitlebarAction(accountActions)
	click(accountActions)
	menu := &h.shell.accountTitleMenu
	if h.shell.overlay.Trigger != &menu.Trigger || h.shell.overlay.Panel.Empty() {
		t.Fatal("account actions did not open from the titlebar")
	}
	h.press(key.NameDownArrow)
	h.press(key.NameReturn)
	settle()
	if h.shell.activeRoute() != historyRoute || !ledger.model.Filters().Equal(historyui.Filters{Account: fixture.account}) {
		t.Fatal("account titlebar menu did not open its scoped transactions")
	}
	click("Back")
	h.focused(&menu.Trigger)
	if groups := accounts.current.holdingGroups; len(groups) == 0 || len(groups) != len(want) ||
		len(groups[0].Subtotals) == 0 || len(want[0].Subtotals) == 0 ||
		groups[0].Subtotals[0].Amounts.TotalCost == nil || want[0].Subtotals[0].TotalCost == nil ||
		groups[0].Subtotals[0].Amounts.TotalCost.Amount != want[0].Subtotals[0].TotalCost.Amount {
		t.Fatalf("account card diverged from Wails exact cost: %+v", groups)
	}
	currentLink, closedLink := "Open instrument · "+current.DisplayLabel(), "Open instrument · "+closed.DisplayLabel()
	assertPageLabel(t, &h.router, currentLink)
	assertPageLabel(t, &h.router, "Cost basis · €50.30")
	missing(closedLink)
	click("Closed")
	assertPageLabel(t, &h.router, closedLink)
	missing(currentLink)
	missing("Cost basis · €50.30")
	assertPageLabel(t, &h.router, "Zero quantity. Figures show remaining recorded values.")
	click(closedLink)
	if investments.accountScope != fixture.account || investments.id != closed.ID || len(investments.current.positions) != 1 || investments.current.positions[0].Amounts.Quantity != "0" {
		t.Fatalf("closed holding opened outside its account: %q / %+v", investments.accountScope, investments.current.positions)
	}
	click("Related transactions")
	if filters := ledger.model.Filters(); !filters.Equal(historyui.Filters{Account: fixture.account, Instrument: closed.ID}) {
		t.Fatalf("scoped instrument ledger lost account: %+v", filters)
	}
	click("Back")
	h.focused(&investments.detail.transactions)
	click("Back")
	h.focused(portfolioButton(accounts.instruments, closed.ID))
	if accounts.positions.visit.filter != accountPositionsClosed {
		t.Fatalf("back did not restore the closed filter: %v", accounts.positions.visit.filter)
	}
	click("All")
	assertPageLabel(t, &h.router, "Totals exclude zero-quantity positions.")
	assertPageLabel(t, &h.router, currentLink)
	assertPageLabel(t, &h.router, closedLink)
	click("Actions for " + current.DisplayLabel())
	click("View instrument history")
	if filters := ledger.model.Filters(); !filters.Equal(historyui.Filters{Account: fixture.account, Instrument: current.ID}) {
		t.Fatalf("account holding action lost scope: %+v", filters)
	}
	click("Back")
	position := slices.IndexFunc(accounts.current.holdingGroups[0].Instruments, func(row domain.InstrumentHoldingsView) bool { return row.InstrumentID == current.ID })
	if position < 0 {
		t.Fatal("current position disappeared after returning from its ledger")
	}
	h.focused(&accounts.positions.actions[accounts.current.holdingGroups[0].Instruments[position].Holdings[0].HoldingID].toggle)
}

func TestAccountPositionDividendStartsWithExactHolding(t *testing.T) {
	ctx := t.Context()
	service, _, _, fixture := openExistingBuyFixture(t)
	other, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Other Fund", Symbol: "QA-OTHER", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, application.HoldingInput{
		AccountID: fixture.account.String(), InstrumentID: other.ID.String(), Quantity: "1.5", UnitCost: "2",
	}); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return accountTestClock }
	accounts := newAccountsPage(ctx, service, clock, func() {})
	accounts.investment = &accountInvestment{service: service}
	ledger := newLedgerHistory(ctx, service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, history: ledger})
	h.size = image.Pt(1100, 1400)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 3 {
			h.frame()
			accounts.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		settle()
	}
	h.frame()
	click("Accounts")
	click("Harbor Brokerage")
	click("Actions for QA-BUY")
	button := accounts.investment.dividendHoldings[fixture.holding]
	if button == nil {
		t.Fatal("no dividend action on the scoped holding")
	}
	h.router.Source().Execute(key.FocusCmd{Tag: button})
	h.press(key.NameReturn)
	settle()
	d := accounts.investment.draft
	if d == nil || d.sources == nil || d.lockedHolding.ID != fixture.holding || d.instrument != fixture.instrument || len(d.sources.Instruments) != 1 {
		t.Fatalf("dividend isn't pinned to account and holding: %+v", d)
	}
	click("Cancel")
	h.focused(button)
}
