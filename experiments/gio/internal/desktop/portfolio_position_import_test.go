package desktop

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/testfixture"
)

func TestInvestmentAssetsImportsIntoSelectedAccountAndRestoresContext(t *testing.T) {
	ctx := t.Context()
	service, session, paths, ids := openExistingBuyFixture(t)
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, accountType, mode string) domain.AccountID {
		t.Helper()
		input := application.AccountInput{
			Name: name, AccountType: accountType, TrackingMode: mode, BalanceSheetRole: "asset",
			DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInPortfolio: true,
		}
		if mode == "balance" {
			input.InitialAmount = "0"
		}
		r, err := service.CreateAccount(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		return r.Account.ID
	}
	second := create("Second desk", "brokerage", "holdings")
	create("Synthetic wallet", "cash_on_hand", "balance")
	clock := func() time.Time { return accountTestClock }
	testfixture.SetClock(service, clock)

	portfolio := newPortfolioPage(ctx, service, func() {})
	f := &accountInvestment{service: service}
	portfolio.investment, portfolio.clock = f, clock
	ledger := newLedgerHistory(ctx, service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: portfolio, history: ledger, version: service.MobileDataVersion})
	h.size = image.Pt(640, 810)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		for range 4 {
			h.frame()
			portfolio.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	click := func(text string) { t.Helper(); h.click(text, false); finish() }
	finish()
	click("Investment assets")
	portfolio.search.Field.Editor.SetText("QA-BUY")
	finish()
	h.assertTitlebarAction("Record existing holding")
	click("Record existing holding")
	d := f.draft
	h.focused(&d.selectAccount.Trigger)
	if len(d.accounts) != 2 || d.account.ID != "" {
		t.Fatal("expected explicit selection between two eligible accounts")
	}
	pick := func(label string) { t.Helper(); h.click("Holdings account", true); finish(); click(label) }
	pick("Harbor Brokerage · USD")
	if d.instrument != ids.foreign || len(d.sources.Instruments) != 1 {
		t.Fatal("the picker included a security already held by the selected account")
	}
	assertPageLabel(t, &h.router, i18n.English.Text(i18n.PositionImportSingleCandidate))
	setBankField(h, "Existing quantity", &d.quantity, "4")
	setBankField(h, "Original cost per unit · EUR", &d.unitCost, "7")
	pick("Second desk · USD")
	if d.account.ID != second || d.instrument != "" || d.quantity.Text() != "" || d.unitCost.Text() != "" {
		t.Fatal("switching the account reused the previous instrument draft")
	}
	version := service.MobileDataVersion()
	click("Cancel")
	h.focused(&f.importOpen)
	if portfolio.search.Field.Text() != "QA-BUY" || service.MobileDataVersion() != version {
		t.Fatal("cancel changed search or the ledger")
	}
	h.press(key.NameReturn)
	finish()
	d = f.draft
	pick("Second desk · USD")
	chooseTradeInstrument(t, h, d, ids.foreign, finish)
	setBankField(h, "Existing quantity", &d.quantity, "1.125")
	setBankField(h, "Original cost per unit · EUR", &d.unitCost, "2.12345678")
	click("Review existing position")
	h.focused(&d.confirm)
	for _, label := range []string{
		"Quantity before · 0", "Quantity change · 1.125", "Quantity after · 1.125",
		"Original cost per unit · 2.12345678 EUR",
	} {
		assertPageLabel(t, &h.router, label)
	}
	assertPageLabelContains(t, &h.router, "Second desk", "QA-EUR")
	h.press(key.NameReturn)
	finish()
	h.assertTitlebarAction("Record existing holding")
	assertPageLabel(t, &h.router, "Existing position recorded.")
	assertPageLabel(t, &h.router, "View ledger activity")
	if portfolio.positionReceipt == nil {
		t.Fatal("saving the import did not leave a receipt on the portfolio")
	}
	id := portfolio.positionReceipt.activity
	click("View ledger activity")
	if ledger.reads.detail == nil || ledger.reads.detail.id != id {
		t.Fatal("the receipt did not open its saved ledger activity")
	}
	click("Back to Investment Assets")
	h.focused(&f.importOpen)
	if portfolio.search.Field.Text() != "QA-BUY" {
		t.Fatal("Ledger Back lost the Investment Assets search")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	rows, err := reopened.HoldingsByAccounts(ctx, []domain.AccountID{ids.account, second})
	if err != nil || len(rows[ids.account]) != 1 || len(rows[second]) != 1 ||
		rows[second][0].InstrumentID != ids.foreign || rows[second][0].Quantity.Canonical() != "1.125" {
		t.Fatalf("import did not persist in the selected account: %+v / %v", rows, err)
	}
	record, err := reopened.Activity(ctx, id)
	if err != nil || len(record.Effects) != 1 || record.Effects[0].HoldingID == nil || *record.Effects[0].HoldingID != rows[second][0].ID {
		t.Fatalf("receipt did not retain its selected Holding: %+v / %v", record, err)
	}
}

func assertPageLabelContains(t *testing.T, router *input.Router, parts ...string) {
	t.Helper()
	var visit func([]input.SemanticNode) bool
	visit = func(nodes []input.SemanticNode) bool {
		for _, node := range nodes {
			if node.Desc.Label != "" {
				containsAll := true
				for _, part := range parts {
					if !strings.Contains(node.Desc.Label, part) {
						containsAll = false
						break
					}
				}
				if containsAll {
					return true
				}
			}
			if visit(node.Children) {
				return true
			}
		}
		return false
	}
	if !visit(router.AppendSemantics(nil)) {
		t.Fatalf("expected one rendered semantic label containing %q", strings.Join(parts, " + "))
	}
}

func TestPortfolioPositionImportSheetPreservesScrolledHoldingsAndFocus(t *testing.T) {
	ctx := t.Context()
	service, session, _, ids := openExistingBuyFixture(t)
	t.Cleanup(func() { _ = session.Close() })
	var symbols []string
	for index := range 14 {
		symbol := fmt.Sprintf("QA-SCROLL-%02d", index)
		instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{
			Name: "Synthetic scroll holding " + symbol, Symbol: symbol, Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateHolding(ctx, application.HoldingInput{
			AccountID: ids.account.String(), InstrumentID: instrument.ID.String(), Quantity: "1", UnitCost: "1",
		}); err != nil {
			t.Fatal(err)
		}
		symbols = append(symbols, symbol)
	}

	portfolio := newPortfolioPage(ctx, service, func() {})
	f := &accountInvestment{service: service}
	portfolio.investment = f
	ledger := newLedgerHistory(ctx, service, func() time.Time { return accountTestClock }, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: portfolio, history: ledger, version: service.MobileDataVersion})
	h.size = image.Pt(900, 600)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	t.Cleanup(func() {
		portfolio.workers.Wait()
		ledger.model.Wait()
		ledger.reads.workers.Wait()
	})

	// Keep a small tail below the real portfolio body so this compact test
	// viewport exercises the shell's outer page scroll as well as its holdings list.
	route := h.shell.routes[portfolioRoute]
	body := route.body
	route.body = func(gtx layout.Context, view ui.View) layout.Dimensions {
		dims := body(gtx, view)
		dims.Size.Y += gtx.Dp(320)
		return dims
	}
	h.shell.routes[portfolioRoute] = route
	finish := func() {
		for range 4 {
			h.frame()
			portfolio.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	finish()
	h.click("Investment assets", false)
	finish()
	portfolio.list.ScrollTo(8)
	finish()
	listBefore := portfolio.list.Position
	if listBefore.First == 0 && listBefore.Offset == 0 {
		t.Fatalf("fixture did not scroll the holdings list: %+v", listBefore)
	}

	h.router.Queue(pointer.Event{
		Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(float32(h.size.X-20), 150), Scroll: f32.Pt(0, 23),
	})
	h.frame()
	finish()
	pageBefore := h.shell.stage.Position
	if pageBefore.Offset == 0 {
		t.Fatalf("fixture did not scroll the outer page: %+v", pageBefore)
	}
	var visibleSymbol string
	for _, symbol := range symbols {
		if node := findPageLabel(&h.router, symbol); node != nil && !node.Desc.Bounds.Empty() {
			visibleSymbol = symbol
			break
		}
	}
	if visibleSymbol == "" {
		t.Fatal("the scrolled holdings list has no visible synthetic security row")
	}
	rowBefore := findPageLabel(&h.router, visibleSymbol).Desc.Bounds
	version := service.MobileDataVersion()
	h.assertTitlebarAction("Record existing holding")
	h.click("Record existing holding", false)
	finish()
	if f.draft == nil || !f.draft.contextualSheet {
		t.Fatal("the titlebar action did not open the contextual sheet")
	}
	pressTab := func(modifiers key.Modifiers) {
		h.router.Queue(input.SystemEvent{Event: key.Event{Name: key.NameTab, State: key.Press, Modifiers: modifiers}})
		h.frame()
	}
	h.focused(&f.draft.selectAccount.Trigger)
	pressTab(0)
	h.focused(&f.draft.cancel)
	pressTab(0)
	h.focused(&portfolio.positionSheet.panel.close)
	pressTab(key.ModShift)
	h.focused(&f.draft.cancel)
	pressTab(0)
	h.focused(&portfolio.positionSheet.panel.close)
	if h.router.Source().Focused(&h.shell.railButton) {
		t.Fatal("Tab navigation moved keyboard focus behind the import sheet")
	}
	h.press(key.NameReturn)
	finish()
	if h.shell.narrowExpanded {
		t.Fatal("Return from the import sheet activated the background sidebar control")
	}
	if f.draft != nil {
		h.press(key.NameEscape)
		finish()
	}
	if f.draft != nil {
		t.Fatal("the keyboard focus-cycle check left the import sheet open")
	}
	h.click("Record existing holding", false)
	finish()
	if f.draft == nil || !f.draft.contextualSheet {
		t.Fatal("the titlebar action did not reopen the contextual sheet")
	}
	if portfolio.list.Position != listBefore || h.shell.stage.Position != pageBefore {
		t.Fatalf("opening the import sheet changed background scroll: holdings %+v vs %+v, page %+v vs %+v",
			portfolio.list.Position, listBefore, h.shell.stage.Position, pageBefore)
	}

	h.press(key.NameEscape)
	finish()
	if f.draft != nil || portfolio.list.Position != listBefore || h.shell.stage.Position != pageBefore {
		t.Fatalf("Escape changed the background visit: draft %v, holdings %+v vs %+v, page %+v vs %+v",
			f.draft != nil, portfolio.list.Position, listBefore, h.shell.stage.Position, pageBefore)
	}
	if after := findPageLabel(&h.router, visibleSymbol); after == nil || after.Desc.Bounds != rowBefore {
		t.Fatalf("Escape moved the previously visible %s row: got %+v, want %+v", visibleSymbol, after, rowBefore)
	}
	h.focused(&f.importOpen)
	if service.MobileDataVersion() != version {
		t.Fatal("opening and dismissing the import sheet wrote to the ledger")
	}
}
