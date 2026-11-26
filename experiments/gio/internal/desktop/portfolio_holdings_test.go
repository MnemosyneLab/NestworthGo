package desktop

import (
	"image"
	"slices"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

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
	h.size = image.Pt(1280, 800)
	settle()
	stockHeader := portfolio.holdings.sortButtons[domain.HoldingGroupStock]
	setBankField(h, "Search holdings", &portfolio.search.Field, "qa-full")
	settle()
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
	assertPageLabel(t, &h.router, "Cost basis · €68.36")
	name := ids.instruments["QA-FULL"].DisplayLabel()
	identity := assertPageLabel(t, &h.router, name)
	cost := assertPageLabel(t, &h.router, "Cost basis · €60.36")
	menu := assertPageLabel(t, &h.router, "Actions for "+name)
	if cost.Min.X < identity.Max.X || menu.Min.X < cost.Max.X ||
		cost.Min.Y >= identity.Max.Y || menu.Min.Y >= identity.Max.Y ||
		cost.Max.Y <= identity.Min.Y || menu.Max.Y <= identity.Min.Y {
		t.Fatalf("desktop holding cells should share a single row: instrument %v, cost %v, actions %v", identity, cost, menu)
	}
	click("Cost basis")
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
	reveal("1 account")
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
	click("Actions for " + loss.DisplayLabel())
	reveal("View instrument history")
	click("View instrument history")
	if filters := timeline.model.Filters(); filters.Account != ids.account || filters.Instrument != loss.ID {
		t.Fatalf("holding action used the wrong ledger scope %+v", filters)
	}
	click("Back")
	h.focused(&control.toggle)
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
		h.focused(&control.toggle)
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

func TestPortfolioMyHoldingsUnknownCostKeepsValue(t *testing.T) {
	ctx := t.Context()
	service, _ := reopenOnboardingProfile(t, prepareOnboardingProfile(t))
	service.SetClock(func() time.Time { return accountTestClock })
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic prehistory holdings", BaseCurrency: "USD", MemberNames: []string{"Ana Example"},
	}); err != nil {
		t.Fatal(err)
	}
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	broker, err := service.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic Brokerage", AccountType: "brokerage", TrackingMode: "holdings", BalanceSheetRole: "asset",
		DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInNetWorth: true, IncludeInPortfolio: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	fund, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Unknown Cost Fund", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, application.HoldingInput{
		AccountID: broker.Account.ID.String(), InstrumentID: fund.ID.String(), Quantity: "4",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, fund.ID, "10", accountTestClock.Format(time.RFC3339), false); err != nil {
		t.Fatal(err)
	}
	p := newPortfolioPage(ctx, service, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: p})
	h.size = image.Pt(1100, 950)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Investment assets", false)
	p.workers.Wait()
	h.frame()
	wails, err := analytics.NewService(service).InstrumentHoldingGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, ref := p.current.groups[0].Instruments[0].Amounts, wails[0].Instruments[0].Amounts
	if got.CurrentValue == nil || ref.CurrentValue == nil || got.CurrentValue.Amount != "40" || ref.CurrentValue.Amount != "40" ||
		got.TotalCost != nil || ref.TotalCost != nil || got.UnrealizedGain != nil || ref.UnrealizedGain != nil ||
		got.CostMissingReason == "" || got.CostMissingReason != ref.CostMissingReason || got.GainMissingReason != ref.GainMissingReason {
		t.Fatalf("unknown cost and gain replaced or hid market value: Gio %+v, Wails %+v", got, ref)
	}
	for _, label := range []string{"Acquisition cost is unavailable.", "Cost basis · Unavailable", "Native market value · US$40.00", "Unrealized gain or loss · Unavailable"} {
		assertPageLabel(t, &h.router, label)
	}
	if findPageLabel(&h.router, "Native market value · USD US$40.00") != nil {
		t.Fatal("holdings duplicated the USD prefix")
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
