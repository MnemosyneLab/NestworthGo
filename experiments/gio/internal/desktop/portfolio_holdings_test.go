package desktop

import (
	"fmt"

	"image"
	"slices"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	portapi "github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
)

func TestPortfolioMyHoldingsGroupedAmountsAndCompactNavigation(t *testing.T) {
	ctx := t.Context()
	service, _, _, ids := openPortfolioFixture(t, "partial")
	quoteAt := accountTestClock.Add(-time.Hour).Format(time.RFC3339)
	stock := func(name, symbol, currency, quantity, cost, price string) (domain.Instrument, domain.HoldingID) {
		t.Helper()
		instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{
			Name: name, Symbol: symbol, Type: "stock", QuoteCurrency: currency, QuoteSource: "manual",
		})
		if err != nil {
			t.Fatal(err)
		}
		holding, err := service.CreateHolding(ctx, application.HoldingInput{
			AccountID: ids.account.String(), InstrumentID: instrument.ID.String(), Quantity: quantity, UnitCost: cost,
		})
		if err != nil {
			t.Fatal(err)
		}
		if price != "" {
			if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, price, quoteAt, false); err != nil {
				t.Fatal(err)
			}
		}
		return instrument, holding.ID
	}
	loss, lossHolding := stock("Singapore Stock", "QA-LOSS", "SGD", "2", "20", "12.75")
	closed, _ := stock("Closed Stock", "QA-ZERO", "USD", "0", "", "")
	if _, err := service.SetFXPreference(ctx, "SGD", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "SGD", "USD", "0.75", quoteAt); err != nil {
		t.Fatal(err)
	}

	wails, err := analytics.NewService(service).InstrumentHoldingGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wailsSummary, err := portapi.NewService(service).Portfolio(ctx, account.AccountFilterRequest{})
	if err != nil {
		t.Fatal(err)
	}
	portfolio := newPortfolioPage(ctx, service, func() {})
	clock := func() time.Time { return accountTestClock }
	accounts := newAccountsPage(ctx, service, clock, func() {})
	timeline := newLedgerHistory(ctx, service, clock, func() {})
	returns := &returnAnalysisPage{ctx: ctx, reader: service, clock: clock, invalidate: func() {}}
	assets := &assetChangesPage{ctx: ctx, reader: service, clock: clock, invalidate: func() {}}
	drain := func() {
		portfolio.workers.Wait()
		accounts.workers.Wait()
		timeline.reads.workers.Wait()
		timeline.model.Wait()
		returns.workers.Wait()
		assets.workers.Wait()
	}
	t.Cleanup(drain)
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: portfolio, accounts: accounts, history: timeline, returns: returns, assets: assets})
	h.size = image.Pt(665, 510)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.preferences.language = i18n.English
	h.shell.theme.Face = font.Typeface("DejaVu Sans")
	h.locale = system.Locale{Language: "en-US"}
	settle := func() {
		for range 3 {
			h.frame()
			drain()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	reveal := func(label string) {
		t.Helper()
		for range 45 {
			if node := findPageLabel(&h.router, label); node != nil && node.Desc.Bounds.Min.Y > 142 && node.Desc.Bounds.Max.Y < h.size.Y {
				return
			}
			h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(410, 400), Scroll: f32.Pt(0, 58)})
			h.frame()
		}
		t.Fatalf("could not reveal %q", label)
	}

	h.frame()
	click("Investment assets")
	if got := portfolio.current.summary.DisplayValuation; got.Total != nil || got.KnownSubtotal == nil ||
		got.KnownSubtotal.Amount != "127.5692" || wailsSummary.DisplayValuation.KnownSubtotal == nil ||
		wailsSummary.DisplayValuation.KnownSubtotal.Amount != got.KnownSubtotal.Amount ||
		wailsSummary.DisplayValuation.Quality.Availability != got.Quality.Availability {
		t.Fatalf("wrong base subtotal or coverage: Gio %+v, Wails %+v", got, wailsSummary.DisplayValuation)
	}
	groups := portfolio.current.groups
	if len(groups) != 2 || len(wails) != 2 || groups[0].Key != domain.HoldingGroupStock || groups[1].Key != domain.HoldingGroupFund ||
		wails[0].Key != string(groups[0].Key) || wails[1].Key != string(groups[1].Key) {
		t.Fatalf("wrong group order or membership: Gio %+v, Wails %+v", groups, wails)
	}
	if len(groups[0].Subtotals) != 1 || len(wails[0].Subtotals) != 1 {
		t.Fatal("closed USD stock affected active SGD subtotals")
	}
	gs, ws := groups[0].Subtotals[0], wails[0].Subtotals[0]
	if gs.Currency != "SGD" || ws.Currency != "SGD" || gs.InstrumentCount != 1 || ws.InstrumentCount != 1 ||
		gs.Amounts.TotalCost.Amount != "40" || ws.TotalCost.Amount != "40" ||
		gs.Amounts.CurrentValue.Amount != "25.5" || ws.CurrentValue.Amount != "25.5" ||
		gs.Amounts.UnrealizedGain.Amount != "-14.5" || ws.UnrealizedGain.Amount != "-14.5" {
		t.Fatalf("wrong native stock subtotal: Gio %+v, Wails %+v", gs, ws)
	}
	for i, row := range groups[1].Subtotals {
		ref := wails[1].Subtotals[i]
		if ref.Currency != row.Currency.String() ||
			ref.InstrumentCount != row.InstrumentCount ||
			(ref.CurrentValue == nil) != (row.Amounts.CurrentValue == nil) ||
			(ref.UnrealizedGain == nil) != (row.Amounts.UnrealizedGain == nil) {
			t.Fatalf("mismatched native fund subtotal for %s", row.Currency)
		}
		if row.Currency == "EUR" && (row.Amounts.CurrentValue != nil || row.Amounts.UnrealizedGain != nil ||
			row.Amounts.TotalCost.Amount != "68.36" || ref.TotalCost.Amount != "68.36") {
			t.Fatalf("missing EUR quote suppressed known cost or became zero: %+v", row)
		}
	}
	if len(groups[1].Instruments) != len(wails[1].Instruments) {
		t.Fatal("mismatched instruments in Funds & ETFs")
	}
	i := slices.IndexFunc(groups[1].Instruments, func(row domain.InstrumentHoldingsView) bool { return row.Symbol == "QA-FULL" })
	if i < 0 {
		t.Fatal("priced fund absent from Funds & ETFs")
	}
	row, ref := groups[1].Instruments[i], wails[1].Instruments[i]
	if row.InstrumentID.String() != ref.InstrumentID || len(row.Holdings) != 2 || len(ref.Holdings) != 2 ||
		row.Amounts.UnrealizedGain.Amount != "30.0102" || ref.Amounts.UnrealizedGain.Amount != "30.0102" ||
		row.Holdings[0].Amounts.UnrealizedGain.Amount != "25.0085" || ref.Holdings[0].Amounts.UnrealizedGain.Amount != "25.0085" {
		t.Fatalf("wrong instrument or account-level gain: Gio %+v, Wails %+v", row, ref)
	}

	narrowSize := h.size
	h.size = image.Pt(900, 600)
	settle()
	summaryAtNineHundred := assertPageLabel(t, &h.router, "Current investment value · Known subtotal")
	toolbarTab := semanticBounds(t, &h.router, "By instrument", false)
	groupAtNineHundred := semanticBounds(t, &h.router, "Stocks · 1 holding", false)
	searchAtNineHundred := semanticBounds(t, &h.router, "Search holdings", true)
	if summaryAtNineHundred.Min.Y > 130 {
		t.Fatalf("investment summary retained excess empty space above it at 900px: %v", summaryAtNineHundred)
	}
	if searchAtNineHundred.Empty() || findPageLabel(&h.router, "Search holdings") != nil {
		t.Fatalf("the compact holdings search should retain its accessible name without a visible label row: %v", searchAtNineHundred)
	}
	if searchAtNineHundred.Min.Y > toolbarTab.Max.Y || searchAtNineHundred.Max.Y < toolbarTab.Min.Y {
		t.Fatalf("the search field should share the tabs' toolbar row at 900px: tabs %v, search %v", toolbarTab, searchAtNineHundred)
	}
	if groupAtNineHundred.Empty() || groupAtNineHundred.Min.Y > 285 || toolbarTab.Min.Y >= groupAtNineHundred.Min.Y {
		t.Fatalf("the grouped holdings table should follow the single responsive toolbar near the first viewport: tab %v, group %v",
			toolbarTab, groupAtNineHundred)
	}
	for _, heading := range []string{"Cost", "Current value", "Gain / loss"} {
		assertPageLabel(t, &h.router, heading)
	}
	h.size = image.Pt(1280, 800)
	settle()
	stockHeader := portfolio.holdings.sortButtons[domain.HoldingGroupStock]
	setBankField(h, "Search holdings", &portfolio.search.Field, "qa-full")
	settle()
	summary := assertPageLabel(t, &h.router, "Current investment value · Known subtotal")
	coverage := fmt.Sprintf("Totals exclude missing values · %d missing inputs", len(wailsSummary.MissingInputs))
	assertPageLabel(t, &h.router, coverage)
	tab := semanticBounds(t, &h.router, "My holdings", false)
	if summary.Min.Y >= tab.Min.Y {
		t.Fatalf("portfolio summary should stay above the views and search: %v / %v", summary, tab)
	}
	fundHeader := portfolio.holdings.sortButtons[domain.HoldingGroupFund]
	if stockHeader == nil || fundHeader == nil || stockHeader == fundHeader {
		t.Fatal("expanded asset groups should each have keyboard-reachable sort headers")
	}
	filtered := portfolio.visibleHoldingGroups(i18n.English)
	if len(filtered) != 1 || len(filtered[0].Instruments) != 1 || filtered[0].Instruments[0].Symbol != "QA-FULL" {
		t.Fatalf("search did not isolate the intended holding: %+v", filtered)
	}
	visibleCost := filtered[0].Instruments[0].Amounts.TotalCost
	if visibleCost == nil || visibleCost.Amount != "60.36" || !slices.ContainsFunc(filtered[0].Subtotals, func(subtotal domain.HoldingCurrencySubtotal) bool {
		return subtotal.Currency == "EUR" && subtotal.Amounts.TotalCost != nil && subtotal.Amounts.TotalCost.Amount == "68.36"
	}) {
		t.Fatalf("expected visible EUR 60.36 and full-group EUR 68.36: %+v", filtered)
	}
	assertPageLabel(t, &h.router, "EUR · Full group subtotal")
	groupCostHeading := assertPageLabel(t, &h.router, "Cost")
	groupCostValue := assertPageLabel(t, &h.router, "Cost basis · €68.36")
	if absTitlebarPixel(groupCostHeading.Max.X-groupCostValue.Max.X) > 8 {
		t.Fatalf("currency subtotal label and value should share the right-aligned cost column: %v / %v", groupCostHeading, groupCostValue)
	}
	name := ids.instruments["QA-FULL"].DisplayLabel()
	reveal("Actions for " + name)
	beforeWideMenu := semanticBounds(t, &h.router, name, false)
	click("Actions for " + name)
	afterWideMenu := semanticBounds(t, &h.router, name, false)
	if beforeWideMenu != afterWideMenu {
		t.Fatalf("opening the wide row menu moved its row: before %v, after %v", beforeWideMenu, afterWideMenu)
	}
	h.press(key.NameEscape)
	settle()
	identity := assertPageLabel(t, &h.router, name)
	fullName := assertPageLabel(t, &h.router, "Complete Fund")
	accountCount := assertPageLabel(t, &h.router, "2 accounts")
	cost := assertPageLabel(t, &h.router, "Cost basis · €60.36")
	menu := assertPageLabel(t, &h.router, "Actions for "+name)
	if fullName.Min.Y <= identity.Min.Y || accountCount.Min.Y <= fullName.Min.Y ||
		cost.Min.X < identity.Max.X || menu.Min.X < cost.Max.X {
		t.Fatalf("holding identity should read symbol, full name and account count in order before the right-side values and actions: identity %v, full name %v, accounts %v, cost %v, actions %v",
			identity, fullName, accountCount, cost, menu)
	}
	costHeader := semanticBounds(t, &h.router, "Cost", false)
	if costHeader.Max.X-8 != cost.Max.X {
		t.Fatalf("sort caption and holding amount must share the cost column: %v / %v", costHeader, cost)
	}
	click("Cost")
	if portfolio.holdings.visit.sort != (holdingSort{key: holdingSortCost}) {
		t.Fatal("the first column activation did not select ascending cost")
	}
	h.router.Source().Execute(key.FocusCmd{Tag: &fundHeader[holdingSortCost]})
	h.press(key.NameReturn)
	settle()
	if !portfolio.holdings.visit.sort.descending {
		t.Fatal("a second column activation did not reverse its direction")
	}
	h.size = narrowSize

	setBankField(h, "Search holdings", &portfolio.search.Field, "qa-loss")
	settle()
	reveal("Stocks · 1 holding")
	click("Stocks · 1 holding")
	if !portfolio.holdings.visit.collapsed[domain.HoldingGroupStock] || findPageLabel(&h.router, loss.DisplayLabel()) != nil {
		t.Fatal("collapse did not hide the selected group rows")
	}
	click("Stocks · 1 holding")
	reveal("Unrealized gain or loss · -S$14.50")
	if findPageLabel(&h.router, "Unrealized gain or loss · -SGD S$14.50") != nil {
		t.Fatal("holding amount repeated the currency code")
	}
	reveal("Show account holdings for QA-LOSS")
	disclosure := portfolio.holdings.disclosures[loss.ID]
	h.router.Source().Execute(key.FocusCmd{Tag: disclosure})
	h.press(key.NameReturn)
	settle()
	if !portfolio.holdings.visit.expanded[loss.ID] {
		t.Fatal("keyboard did not expand the account holdings")
	}
	accountLink := "Open account · Harbor Brokerage"
	reveal(accountLink)
	click(accountLink)
	if accounts.target.account != ids.account {
		t.Fatal("expanded row opened the wrong account")
	}
	click("Back")
	control := portfolio.holdingActions(portfolioHoldingKey{instrument: loss.ID, holding: lossHolding})
	h.focused(&control.account)
	if portfolio.search.Field.Text() != "qa-loss" || !portfolio.holdings.visit.expanded[loss.ID] ||
		portfolio.holdings.visit.sort != (holdingSort{key: holdingSortCost, descending: true}) {
		t.Fatal("Back discarded contextual Portfolio state")
	}
	click("Account details")
	reveal(loss.DisplayLabel())
	click(loss.DisplayLabel())
	if portfolio.accountScope != ids.account || len(portfolio.current.positions) != 1 ||
		len(portfolio.current.positions[0].Holdings) != 1 || portfolio.current.positions[0].Holdings[0].AccountID != ids.account {
		t.Fatal("by-account row did not reuse the scoped instrument detail")
	}
	click("Back")
	if !portfolio.holdings.visit.byAccount {
		t.Fatal("Back discarded the selected holdings view")
	}
	reveal("Actions for " + loss.DisplayLabel())
	beforeMenuRow := semanticBounds(t, &h.router, loss.DisplayLabel(), false)
	click("Actions for " + loss.DisplayLabel())
	afterMenuRow := semanticBounds(t, &h.router, loss.DisplayLabel(), false)
	if beforeMenuRow != afterMenuRow {
		t.Fatalf("opening the anchored row menu moved its row: before %v, after %v", beforeMenuRow, afterMenuRow)
	}
	if h.shell.overlay.Trigger != &control.menu.Trigger || portfolio.holdings.rowWidth <= 0 {
		t.Fatal("row action menu did not stay attached to its visible trigger")
	}
	reveal("View instrument history")
	click("View instrument history")
	if filters := timeline.model.Filters(); filters.Account != ids.account || filters.Instrument != loss.ID {
		t.Fatalf("holding action used the wrong ledger scope %+v", filters)
	}
	click("Back")
	h.focused(&control.menu.Trigger)
	for _, choice := range []struct {
		label string
		route routeID
	}{
		{"View return", returnAnalysisRoute},
		{"View asset changes", assetChangesRoute},
	} {
		click("Actions for " + loss.DisplayLabel())
		reveal(choice.label)
		click(choice.label)
		if h.shell.activeRoute() != choice.route {
			t.Fatalf("holding action did not open %s", choice.label)
		}
		selection := returns.filters.selection
		if choice.route == assetChangesRoute {
			selection = assets.filters.selection
		}
		if selection.scope != (domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: loss.ID.String()}) ||
			selection.account != ids.account.String() || selection.valuation != domain.ValuationBase || !selection.excludeCash {
			t.Fatalf("holding analysis used the wrong scope %+v", selection)
		}
		if (choice.route == returnAnalysisRoute && returns.tab != returnTrendTab) ||
			(choice.route == assetChangesRoute && assets.state.tab != assetDriversTab) {
			t.Fatalf("holding action did not open the expected analysis view for %s", choice.label)
		}
		click("Back")
		h.focused(&control.menu.Trigger)
	}

	setBankField(h, "Search holdings", &portfolio.search.Field, "qa-zero")
	settle()
	if len(portfolio.visibleHoldingGroups(i18n.English)) != 0 {
		t.Fatal("closed holding appeared before enabling zero holdings")
	}
	box := &portfolio.holdings.showZero
	h.router.Source().Execute(key.FocusCmd{Tag: box})
	h.press(key.NameSpace)
	settle()
	reveal(closed.DisplayLabel())
	reveal("Unrealized gain or loss · US$0.00")
	if !portfolio.holdings.visit.showZero {
		t.Fatal("keyboard could not reveal a known zero holding")
	}
}

func TestPortfolioRefreshingKeepsPressedHoldingActionAnchored(t *testing.T) {
	ctx := t.Context()
	service, session, _, ids := openPortfolioFixture(t, "partial")
	t.Cleanup(func() { _ = session.Close() })
	probe := &portfolioReadProbe{Service: service, started: make(chan struct{}), release: make(chan struct{})}
	portfolio := newPortfolioPage(ctx, probe, func() {})
	portfolio.openHoldingHistory = func(domain.InstrumentID, domain.AccountID, string, event.Tag) {}
	var released bool
	releaseRead := func() {
		if !released {
			close(probe.release)
			released = true
		}
	}
	t.Cleanup(func() {
		releaseRead()
		portfolio.workers.Wait()
	})
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: portfolio})
	h.size = image.Pt(900, 600)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 3 {
			h.frame()
			portfolio.workers.Wait()
		}
		h.frame()
	}
	h.frame()
	h.click("Investment assets", false)
	settle()
	if portfolio.current == nil || len(portfolio.current.groups) == 0 {
		t.Fatal("the fixture did not load populated holdings")
	}
	portfolio.list.ScrollTo(2)
	settle()
	listBefore, pageBefore := portfolio.list.Position, h.shell.stage.Position
	if listBefore.First == 0 && listBefore.Offset == 0 {
		t.Fatalf("the populated holdings tab did not scroll: %+v", listBefore)
	}
	type visibleHolding struct {
		instrument  domain.Instrument
		row, action image.Rectangle
	}
	visible := make([]visibleHolding, 0, len(ids.instruments))
	for _, instrument := range ids.instruments {
		name := instrument.DisplayLabel()
		rowNode := findPageLabel(&h.router, name)
		actionNode := findPageLabel(&h.router, fmt.Sprintf(i18n.English.Text(i18n.PortfolioActionsFor), name))
		if rowNode == nil || actionNode == nil || rowNode.Desc.Bounds.Empty() || actionNode.Desc.Bounds.Empty() ||
			rowNode.Desc.Bounds.Min.Y < 0 || rowNode.Desc.Bounds.Max.Y > h.size.Y {
			continue
		}
		visible = append(visible, visibleHolding{instrument: instrument, row: rowNode.Desc.Bounds, action: actionNode.Desc.Bounds})
	}
	if len(visible) < 2 {
		t.Fatalf("the scrolled shell needs at least two visible holdings to detect a wrong-row release: %+v", visible)
	}
	slices.SortFunc(visible, func(a, b visibleHolding) int { return a.row.Min.Y - b.row.Min.Y })
	target := visible[0]
	name := target.instrument.DisplayLabel()
	pressAt := target.action.Min.Add(target.action.Max).Div(2)
	h.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary,
		Position: f32.Pt(float32(pressAt.X), float32(pressAt.Y))})
	h.frame()
	if got := findPageLabel(&h.router, name); got == nil || got.Desc.Bounds != target.row {
		t.Fatalf("the initial row-action press moved %s: got %+v, want %v", name, got, target.row)
	}

	probe.holdPrice.Store(true)
	h.shell.refreshLedgerReads()
	select {
	case <-probe.started:
	case <-time.After(3 * time.Second):
		t.Fatal("the delayed portfolio refresh did not start")
	}
	h.frame()
	if portfolio.request == nil {
		t.Fatal("the shell did not retain the pending portfolio read")
	}
	assertPageLabel(t, &h.router, "Updating valuation…")
	rowDuringRefresh := findPageLabel(&h.router, name)
	actionDuringRefresh := findPageLabel(&h.router, fmt.Sprintf(i18n.English.Text(i18n.PortfolioActionsFor), name))
	if rowDuringRefresh == nil || rowDuringRefresh.Desc.Bounds != target.row || actionDuringRefresh == nil || actionDuringRefresh.Desc.Bounds != target.action ||
		portfolio.list.Position != listBefore || h.shell.stage.Position != pageBefore {
		t.Fatalf("a pending background read moved the pressed holding or its context: row %+v vs %v, action %+v vs %v, list %+v vs %+v, page %+v vs %+v",
			rowDuringRefresh, target.row, actionDuringRefresh, target.action, portfolio.list.Position, listBefore, h.shell.stage.Position, pageBefore)
	}

	h.router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse,
		Position: f32.Pt(float32(pressAt.X), float32(pressAt.Y))})
	h.frame()
	control := portfolio.holdingActions(portfolioHoldingKey{instrument: target.instrument.ID})
	if portfolio.id != "" || h.shell.activeRoute() != portfolioRoute || h.shell.overlay.Trigger != &control.menu.Trigger {
		t.Fatalf("releasing the row action selected another holding or opened the wrong menu: route %s, instrument %s, trigger %p",
			h.shell.activeRoute(), portfolio.id, h.shell.overlay.Trigger)
	}
	rowAfterOpen := findPageLabel(&h.router, name)
	if rowAfterOpen == nil || rowAfterOpen.Desc.Bounds != target.row || !h.shell.overlay.Anchor.Overlaps(target.action) ||
		h.shell.overlay.Anchor.Min.Y >= target.row.Max.Y || h.shell.overlay.Anchor.Max.Y <= target.row.Min.Y {
		t.Fatalf("the opened menu no longer anchors to the same visible holding: row %+v, want %v; anchor %v",
			rowAfterOpen, target.row, h.shell.overlay.Anchor)
	}
	anchorAfterOpen := h.shell.overlay.Anchor

	releaseRead()
	settle()
	rowAfterRefresh := findPageLabel(&h.router, name)
	actionAfterRefresh := findPageLabel(&h.router, fmt.Sprintf(i18n.English.Text(i18n.PortfolioActionsFor), name))
	if portfolio.request != nil || rowAfterRefresh == nil || rowAfterRefresh.Desc.Bounds != target.row ||
		actionAfterRefresh == nil || actionAfterRefresh.Desc.Bounds != target.action || h.shell.overlay.Trigger != &control.menu.Trigger ||
		h.shell.overlay.Anchor != anchorAfterOpen || portfolio.id != "" {
		t.Fatalf("settled refresh changed the menu's holding target: row %+v, action %+v, anchor %v (was %v), route %s, instrument %s",
			rowAfterRefresh, actionAfterRefresh, h.shell.overlay.Anchor, anchorAfterOpen, h.shell.activeRoute(), portfolio.id)
	}
	if findPageLabel(&h.router, "Updating valuation…") != nil {
		t.Fatal("the settled refresh still displays the in-progress badge")
	}
	h.press(key.NameEscape)
	if h.shell.overlay.Trigger != nil {
		t.Fatal("Escape did not dismiss the row menu")
	}
}

func TestPortfolioHoldingSortKeepsCurrenciesAndUnknownValuesInPlace(t *testing.T) {
	row := func(name string, currency domain.CurrencyCode, value, gain string) *domain.InstrumentHoldingsView {
		instrument := &domain.InstrumentHoldingsView{Name: name, QuoteCurrency: currency}
		if value != "" {
			instrument.Amounts.CurrentValue = &domain.MoneyView{Amount: value, Currency: currency}
		}
		if gain != "" {
			instrument.Amounts.UnrealizedGain = &domain.SignedMoneyView{Amount: gain, Currency: currency}
		}
		return instrument
	}
	eur := row("EUR position", "EUR", "8", "-8")
	small := row("Smaller", "USD", "10.0001", "-0.0002")
	large := row("Larger", "USD", "10.0002", "-0.0001")
	missing := row("First by name", "USD", "", "")
	rows := []portfolioHoldingRow{{instrument: missing}, {instrument: large}, {instrument: small}, {instrument: eur}}
	check := func(want ...*domain.InstrumentHoldingsView) {
		t.Helper()
		for i, r := range rows {
			if r.instrument != want[i] {
				t.Fatalf("row %d = %s, want %s", i, r.instrument.Name, want[i].Name)
			}
		}
	}
	sortPortfolioHoldingRows(rows, holdingSort{key: holdingSortValue}, i18n.English)
	check(eur, small, large, missing)
	sortPortfolioHoldingRows(rows, holdingSort{key: holdingSortGain, descending: true}, i18n.English)
	check(eur, large, small, missing)

	// Account details sort by each holding amount, using the same currency order.
	rows[1].member = &domain.InstrumentHoldingMember{Amounts: domain.HoldingAmounts{CurrentValue: &domain.MoneyView{Amount: "0.0001"}}}
	rows[2].member = &domain.InstrumentHoldingMember{Amounts: domain.HoldingAmounts{CurrentValue: &domain.MoneyView{Amount: "0.0002"}}}
	sortPortfolioHoldingRows(rows, holdingSort{key: holdingSortValue}, i18n.English)
	check(eur, large, small, missing)
}
