package desktop

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
)

type retryReturnReads struct {
	*application.Service
	failCalendar, failDay bool
}

func (r *retryReturnReads) ReturnCalendar(ctx context.Context, query domain.AnalysisQuery, cursor, granularity string) (application.ReturnCalendarResult, error) {
	if r.failCalendar {
		r.failCalendar = false
		return application.ReturnCalendarResult{}, errors.New("synthetic calendar error")
	}
	return r.Service.ReturnCalendar(ctx, query, cursor, granularity)
}

func (r *retryReturnReads) ReturnDay(ctx context.Context, query domain.AnalysisQuery, date domain.LocalDate) (application.ReturnDayResult, error) {
	if r.failDay {
		r.failDay = false
		return application.ReturnDayResult{}, errors.New("synthetic day error")
	}
	return r.Service.ReturnDay(ctx, query, date)
}

func TestReturnAnalysisCalendarAndDayMatchWails(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "seed.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seed := application.NewService(sqlite.NewRepository(db))
	seed.SetClock(clock)
	if err := seed.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic Return Calendar", BaseCurrency: "USD", MemberNames: []string{"Sample owner"},
	}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	broker, err := seed.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic broker", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := seed.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic fund", Type: "stock", MarketCode: "US", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := seed.CreateHolding(ctx, application.HoldingInput{
		AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-02-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.StartHistoryWithCosts(ctx, "Asia/Singapore", map[domain.HoldingID]string{holding.ID: "100"}); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	if _, err := seed.AppendManualInstrumentQuote(ctx, instrument.ID, "12.5", "2026-02-02", false); err != nil {
		t.Fatal(err)
	}
	now = now.Add(32 * time.Hour) // February 4 in Singapore.
	if err := errors.Join(db.CheckpointWAL(ctx), db.Close()); err != nil {
		t.Fatal(err)
	}
	seedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	openCopy := func() (*application.Service, *unavailableOverviewProvider) {
		t.Helper()
		copyPath := filepath.Join(t.TempDir(), "copy.db")
		if err := os.WriteFile(copyPath, seedBytes, 0o600); err != nil {
			t.Fatal(err)
		}
		copyDB, err := sqlite.Open(copyPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = copyDB.Close() })
		provider := &unavailableOverviewProvider{}
		s := application.NewService(sqlite.NewRepository(copyDB), application.NewMarketDataRegistry(provider))
		s.SetClock(clock)
		return s, provider
	}
	wails, wailsProvider := openCopy()
	adapter := analysis.NewService(wails)
	query := analysis.AnalysisQueryRequest{ScopeKind: "household", From: "2026-02-01", To: "2026-02-03", Valuation: "base", Basis: "investment", IncludeCash: true}
	calendar, err := adapter.ReturnCalendar(ctx, query, "2026-02", "day")
	if err != nil {
		t.Fatal(err)
	}
	wailsDay, err := adapter.ReturnDay(ctx, query, "2026-02-02")
	if err != nil {
		t.Fatal(err)
	}
	if calendar.Summary.ReturnAmount == nil || calendar.Summary.ReturnAmount.Amount != "25" ||
		calendar.Summary.ReturnRate == nil || *calendar.Summary.ReturnRate != "0.25" {
		t.Fatalf("fixture did not produce a USD 25 return at 25 percent: %+v", calendar.Summary)
	}

	gio, gioProvider := openCopy()
	reader := &retryReturnReads{Service: gio, failCalendar: true, failDay: true}
	page := &returnAnalysisPage{ctx: ctx, reader: reader, clock: clock, invalidate: func() {}}
	assets := &assetChangesPage{ctx: ctx, reader: gio, clock: clock, invalidate: func() {}}
	ledger := newLedgerHistory(ctx, gio, clock, func() {})
	t.Cleanup(page.workers.Wait)
	t.Cleanup(assets.workers.Wait)
	t.Cleanup(ledger.model.Wait)
	t.Cleanup(ledger.reads.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{returns: page, assets: assets, history: ledger})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Return analysis", false)
	for range 3 {
		page.workers.Wait()
		h.frame()
	}
	assertPageLabel(t, &h.router, "Could not load investment returns.")
	h.click("Retry", false)
	page.workers.Wait()
	h.frame()
	if page.current == nil || page.err != nil {
		t.Fatalf("calendar retry did not load: %v", page.err)
	}
	current := page.current
	got := current.summary.Summary
	if current.summaryFrom != query.From || current.summaryTo != query.To ||
		got.ReturnAmount == nil || got.ReturnAmount.CanonicalAmount() != calendar.Summary.ReturnAmount.Amount ||
		got.ReturnRate == nil || got.ReturnRate.String() != *calendar.Summary.ReturnRate ||
		got.Coverage.RatedDays != calendar.Summary.RatedDays || got.Coverage.TotalDays != calendar.Summary.TotalDays {
		t.Fatalf("Gio summary differs from Wails: %+v and %+v", got, calendar.Summary)
	}
	for _, label := range []string{"Summary · 2026-02-01 to 2026-02-03 · USD", "Return amount", "Return rate", "+25%", "$ 25.00", "Day still open"} {
		assertPageLabel(t, &h.router, label)
	}
	wailsYear, err := adapter.ReturnCalendar(ctx, query, "", "day")
	if err != nil || len(wailsYear.Months) != 1 || wailsYear.Months[0].ReturnAmount == nil {
		t.Fatalf("Wails yearly grouping = %+v, %v", wailsYear.Months, err)
	}
	reader.failCalendar = true
	h.click("Year", false)
	page.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "Could not load investment returns.")
	h.click("Retry", false)
	page.workers.Wait()
	h.frame()
	if !page.year || page.current == nil || len(page.current.yearMonths) != 1 {
		t.Fatalf("yearly read = %+v, %v", page.current, page.err)
	}
	month, refMonth := page.current.yearMonths[0], wailsYear.Months[0]
	if month.Month != refMonth.Month || month.ReturnAmount == nil || month.ReturnAmount.CanonicalAmount() != refMonth.ReturnAmount.Amount ||
		month.ReturnRate == nil || refMonth.ReturnRate == nil || month.ReturnRate.String() != *refMonth.ReturnRate ||
		month.Coverage.RatedDays != refMonth.RatedDays || month.Coverage.TotalDays != refMonth.TotalDays {
		t.Fatalf("yearly month differs: Gio %+v, Wails %+v", month, refMonth)
	}
	h.click("February", false)
	page.workers.Wait()
	h.frame()
	if page.year || page.month.Format("2006-01") != "2026-02" {
		t.Fatal("the year card did not open the selected month")
	}
	h.focused(&page.monthMode)
	h.click("2026-02-02 · +25% · $ 25.00", true)
	page.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "Could not load investment returns.")
	h.focused(&page.day.close)
	h.click("Retry", false)
	page.workers.Wait()
	h.frame()
	day := page.day.value
	if day.ReturnAmount == nil || day.ReturnAmount.CanonicalAmount() != wailsDay.ReturnAmount.Amount ||
		day.ReturnRate == nil || day.ReturnRate.String() != *wailsDay.ReturnRate || len(day.Composition) != len(wailsDay.Composition) ||
		gioProvider.requests != 0 || wailsProvider.requests != 0 {
		t.Fatalf("Gio and Wails day or provider reads differ: %+v and %+v", day, wailsDay)
	}
	assertPageLabel(t, &h.router, "Return amount · $ 25.00")
	assertPageLabel(t, &h.router, "Price change · $ 25.00")
	assertPageLabel(t, &h.router, "Synthetic fund · $ 25.00")
	trigger := page.day.button
	h.press(key.NameEscape)
	h.frame()
	h.focused(trigger)
	h.press(key.NameReturn)
	if page.day == nil || page.day.date != "2026-02-02" {
		t.Fatal("keyboard focus did not reopen the same return day")
	}
	h.clickAt(image.Pt(340, 420))
	h.frame()
	if page.day != nil {
		t.Fatal("clicking outside the detail sheet did not dismiss it")
	}
	h.focused(trigger)
	dayQuery := query
	dayQuery.From, dayQuery.To = "2026-02-02", "2026-02-02"
	wailsAssets, err := adapter.AssetChange(ctx, dayQuery)
	if err != nil {
		t.Fatal(err)
	}
	h.press(key.NameReturn)
	page.workers.Wait()
	h.frame()
	h.click("View asset changes for this day", false)
	for range 3 {
		assets.workers.Wait()
		h.frame()
	}
	if h.shell.activeRoute() != assetChangesRoute || assets.state.tab != assetDriversTab ||
		assets.current == nil || assets.current.drivers == nil {
		t.Fatalf("asset driver destination unavailable: %v", assets.err)
	}
	report := assets.current.drivers
	if report.Summary.Change == nil || wailsAssets.Summary.Change == nil ||
		report.Summary.Change.CanonicalAmount() != wailsAssets.Summary.Change.Amount ||
		len(report.Waterfall) != len(wailsAssets.Waterfall) || assets.current.query.From != dayQuery.From ||
		assets.current.query.To != dayQuery.To {
		t.Fatalf("Asset Changes differs from Wails for the linked day: %+v and %+v", report, wailsAssets)
	}
	wailsAssetTrend, err := adapter.AssetTrend(ctx, dayQuery, string(application.TrendDay), string(application.TrendNetWorth))
	if err != nil {
		t.Fatal(err)
	}
	h.click("Asset trend", false)
	assets.workers.Wait()
	h.frame()
	if assets.current == nil || assets.current.trend == nil || len(assets.current.trend.Points) != 1 || len(wailsAssetTrend.Points) != 1 {
		t.Fatalf("the linked day's trend did not load: %v", assets.err)
	}
	point, wailsPoint := assets.current.trend.Points[0], wailsAssetTrend.Points[0]
	if point.Period != wailsPoint.Period || point.Value == nil || wailsPoint.Value == nil || point.Value.CanonicalAmount() != wailsPoint.Value.Amount {
		t.Fatalf("trend point %+v differs from Wails %+v", point, wailsPoint)
	}
	wailsCategories, err := adapter.Categories(ctx, dayQuery, string(application.CategoryInvestmentReturn))
	if err != nil {
		t.Fatal(err)
	}
	h.click("Categories", false)
	assets.workers.Wait()
	h.frame()
	h.click("Category type", true)
	h.click("Total return", false)
	assets.workers.Wait()
	h.frame()
	if assets.current == nil || assets.current.categories == nil || wailsCategories.Total == nil ||
		assets.current.categories.Total == nil || assets.current.categories.Total.CanonicalAmount() != wailsCategories.Total.Amount {
		t.Fatalf("linked day's categories differ from Wails: %v", assets.err)
	}
	h.click("Back", false)
	page.workers.Wait()
	h.frame()
	h.focused(trigger)

	wantTrend, err := adapter.ReturnTrend(ctx, query, string(application.ReturnTrendCumulativeAmount))
	if err != nil {
		t.Fatal(err)
	}
	h.click("Return trend", false)
	page.workers.Wait()
	h.frame()
	if page.err != nil || page.current == nil || page.current.trend == nil {
		t.Fatalf("Gio trend did not load: %v", page.err)
	}
	trend := page.current.trend
	if trend.Amount == nil || wantTrend.Amount == nil || trend.Amount.CanonicalAmount() != wantTrend.Amount.Amount ||
		trend.Rate == nil || wantTrend.Rate == nil || trend.Rate.String() != *wantTrend.Rate ||
		len(trend.Points) != len(wantTrend.Points) {
		t.Fatalf("Gio trend differs from Wails: %+v and %+v", trend, wantTrend)
	}
	wantRanks, err := adapter.Contribution(ctx, query, string(application.ContributionTotalReturn),
		string(application.ContributionGroupInstrument), string(application.ContributionSortAmountDesc))
	if err != nil {
		t.Fatal(err)
	}
	h.click("Contribution", false)
	page.workers.Wait()
	h.frame()
	if page.err != nil || page.current == nil || page.current.contribution == nil {
		t.Fatalf("Gio contribution did not load: %v", page.err)
	}
	ranking := page.current.contribution
	if len(ranking.Rows) != len(wantRanks.Rows) || len(ranking.Rows) == 0 ||
		ranking.Rows[0].Key != wantRanks.Rows[0].Key || ranking.Rows[0].Amount == nil || wantRanks.Rows[0].Amount == nil ||
		ranking.Rows[0].Amount.CanonicalAmount() != wantRanks.Rows[0].Amount.Amount ||
		gioProvider.requests != 0 || wailsProvider.requests != 0 {
		t.Fatalf("Gio and Wails contributions or provider reads differ: %+v and %+v", ranking, wantRanks)
	}
	if ranking.Rows[0].Key != instrument.ID.String() {
		t.Fatalf("expected the fund to appear first, got %+v", ranking.Rows)
	}
	h.size.Y = 1000
	h.frame()
	h.click("Negative contributions", false)
	assertPageLabel(t, &h.router, "Showing 0 of 1 contributions")
	h.click("Search contributions", true)
	h.router.Queue(key.EditEvent{Text: "No match"})
	h.frame()
	h.frame()
	if findPageLabel(&h.router, "Synthetic fund · $ 25.00 · +25%") != nil {
		t.Fatal("the details list did not respond to search")
	}
	h.router.Queue(key.EditEvent{Range: key.Range{End: len("No match")}, Text: "synthetic"})
	h.frame()
	h.frame()
	assertPageLabel(t, &h.router, "Synthetic fund · $ 25.00 · +25%")
	h.click("Largest impact", false)
	h.click("Show 20", false)
	assertPageLabel(t, &h.router, "Showing 1 of 1 contributions")
	h.click("More filters", false)
	h.click("Currency", true)
	h.router.Queue(key.EditEvent{Text: "USD"})
	h.frame()
	h.click("USD", false)
	page.workers.Wait()
	h.frame()
	h.click("Member", true)
	h.click("Sample owner", false)
	page.workers.Wait()
	h.frame()
	h.click("Asset class", true)
	h.click("Stocks", false)
	page.workers.Wait()
	h.frame()
	h.click("Instrument filter", true)
	h.click(instrument.SelectionLabel(), false)
	page.workers.Wait()
	h.frame()
	itemQuery := query
	currency := "USD"
	itemQuery.Currency = &currency
	member := bootstrap.Members[0].ID.String()
	itemQuery.MemberID = &member
	itemQuery.AssetClass = string(domain.InstrumentStock)
	instrumentID := instrument.ID.String()
	itemQuery.InstrumentID = &instrumentID
	wailsItem, err := adapter.ContributionItem(ctx, itemQuery, string(application.ContributionTotalReturn),
		string(application.ContributionGroupInstrument), instrument.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	h.click("Synthetic fund · $ 25.00 · +25%", false)
	page.workers.Wait()
	h.frame()
	if page.item == nil || page.item.value == nil || page.item.value.Amount == nil || wailsItem.Amount == nil ||
		page.item.value.Amount.CanonicalAmount() != wailsItem.Amount.Amount {
		t.Fatalf("the linked contribution item did not match Wails: %+v", page.item)
	}
	assertPageLabel(t, &h.router, "Ledger activity keeps dates, account, instrument and activity type filters. Currency and asset-class filters do not carry over.")
	assertPageLabel(t, &h.router, "The member filter does not carry over to Ledger activity.")
	rank := page.item.button
	h.click("View in Ledger activity", false)
	for range 3 {
		ledger.model.Wait()
		ledger.reads.workers.Wait()
		h.frame()
	}
	wantHistory := history.Filters{From: wailsItem.HistoryHint.From, To: wailsItem.HistoryHint.To,
		Account: domain.AccountID(wailsItem.HistoryHint.AccountID), Instrument: domain.InstrumentID(wailsItem.HistoryHint.InstrumentID)}
	if h.shell.activeRoute() != historyRoute || !ledger.model.Filters().Equal(wantHistory) {
		t.Fatalf("History did not use the Wails hint: got %+v, want %+v", ledger.model.Filters(), wantHistory)
	}
	h.click("Back", false)
	page.workers.Wait()
	h.frame()
	h.focused(rank)
	if page.tab != returnContributionTab || page.output.selectedKey != instrument.ID.String() {
		t.Fatal("Back did not restore Contribution and the selected fund")
	}
	if page.output.ranking.search.Field.Text() != "synthetic" || page.output.ranking.limit != 20 {
		t.Fatal("Back lost the Contribution result controls")
	}
}

func TestReturnCalendarGridUsesWholeWeeks(t *testing.T) {
	month := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		sunday bool
		first  string
		count  int
	}{
		{true, "2026-02-01", 28},
		{false, "2026-01-26", 35},
	} {
		first, count := returnGrid(month, tc.sunday)
		if first.Format(time.DateOnly) != tc.first || count != tc.count {
			t.Errorf("grid with Sunday start %v: got %v with %d days", tc.sunday, first, count)
		}
	}
}

func TestContributionHistoryHintKeepsMultipleActivityKinds(t *testing.T) {
	owner := domain.NewAccountID()
	security := domain.NewInstrumentID()
	hint := application.ContributionHistoryHint{From: "2026-08-01", To: "2026-08-29",
		AccountID: owner.String(), InstrumentID: security.String(),
		Kinds: []string{"sell", "cash_dividend"}}
	got, valid := contributionHistoryFilters(hint)
	want := history.Filters{From: hint.From, To: hint.To, Account: owner, Instrument: security,
		Kinds: []domain.ActivityKind{domain.ActivitySell, domain.ActivityCashDividend}}
	if !valid || !got.Equal(want) {
		t.Fatalf("hint produced %+v, want %+v", got, want)
	}
	hint.Kinds = []string{"price_change"}
	if _, valid := contributionHistoryFilters(hint); valid {
		t.Fatal("a return component was accepted as a ledger activity type")
	}
}

type calendarFilterReader struct {
	account    domain.AccountID
	instrument domain.InstrumentID
	secondary  domain.InstrumentID
	member     domain.MemberID
	lastTrend  domain.AnalysisQuery
	lastRanks  domain.AnalysisQuery
	lastAssets domain.AnalysisQuery
}

func (r *calendarFilterReader) HistoryOrigin(context.Context) (*domain.HistoryOrigin, error) {
	return &domain.HistoryOrigin{StartedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Timezone: "UTC"}, nil
}

func (r *calendarFilterReader) ListInstruments(context.Context, bool) ([]domain.Instrument, error) {
	return []domain.Instrument{
		{ID: r.instrument, Name: "Synthetic fund"},
		{ID: r.secondary, Name: "Synthetic treasuries"},
	}, nil
}

func (r *calendarFilterReader) ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error) {
	return []domain.AccountRecord{{Account: domain.Account{ID: r.account, Name: "Synthetic brokerage"}}}, nil
}

func (r *calendarFilterReader) ListMembers(context.Context, bool) ([]domain.Member, error) {
	return []domain.Member{{ID: r.member, Name: "Synthetic owner"}}, nil
}

func (r *calendarFilterReader) ReturnCalendar(_ context.Context, q domain.AnalysisQuery, _, _ string) (application.ReturnCalendarResult, error) {
	return application.ReturnCalendarResult{Days: []application.ReturnDayResult{{Date: q.From}}}, nil
}

func (r *calendarFilterReader) AssetChange(_ context.Context, q domain.AnalysisQuery) (application.AssetChangeResult, error) {
	r.lastAssets = q
	return application.AssetChangeResult{}, nil
}

func (r *calendarFilterReader) AssetTrend(context.Context, domain.AnalysisQuery, application.AssetTrendGranularity, application.AssetTrendMetric) (application.AssetTrendResult, error) {
	return application.AssetTrendResult{}, nil
}

func (r *calendarFilterReader) Categories(context.Context, domain.AnalysisQuery, application.AnalysisCategoryType) (application.CategoriesResult, error) {
	return application.CategoriesResult{}, nil
}

func (r *calendarFilterReader) ReturnDay(context.Context, domain.AnalysisQuery, domain.LocalDate) (application.ReturnDayResult, error) {
	return application.ReturnDayResult{}, nil
}

func (r *calendarFilterReader) ReturnTrend(_ context.Context, q domain.AnalysisQuery, display application.ReturnTrendDisplay) (application.ReturnTrendResult, error) {
	r.lastTrend = q
	if display != application.ReturnTrendCumulativeAmount {
		return application.ReturnTrendResult{}, errors.New("unexpected default trend mode")
	}
	return application.ReturnTrendResult{}, nil
}

func (r *calendarFilterReader) Contribution(_ context.Context, q domain.AnalysisQuery, metric application.ContributionReturnType, group application.ContributionGroupBy, sort application.ContributionSort) (application.ContributionResult, error) {
	r.lastRanks = q
	if metric != application.ContributionTotalReturn || group != application.ContributionGroupInstrument || sort != application.ContributionSortAmountDesc {
		return application.ContributionResult{}, errors.New("unexpected default contribution options")
	}
	return application.ContributionResult{}, nil
}

func (r *calendarFilterReader) ContributionItem(context.Context, domain.AnalysisQuery, application.ContributionReturnType, application.ContributionGroupBy, string) (application.ContributionItemResult, error) {
	return application.ContributionItemResult{}, nil
}

func TestReturnAnalysisTabsKeepWailsMonthAndFilters(t *testing.T) {
	reader := &calendarFilterReader{account: domain.NewAccountID(), instrument: domain.NewInstrumentID(),
		secondary: domain.NewInstrumentID(), member: domain.NewMemberID()}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	p := &returnAnalysisPage{ctx: t.Context(), reader: reader, clock: func() time.Time { return now }, invalidate: func() {}}
	assets := &assetChangesPage{ctx: t.Context(), reader: reader, clock: func() time.Time { return now }, invalidate: func() {}}
	t.Cleanup(p.workers.Wait)
	t.Cleanup(assets.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{returns: p, assets: assets})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Return analysis", false)
	settle := func() {
		for range 3 {
			p.workers.Wait()
			assets.workers.Wait()
			h.frame()
		}
	}
	settle()
	selectOption := func(field, option string) {
		h.click(field, true)
		h.click(option, false)
		settle()
	}
	selectOption("Scope", "Instrument")
	assertPageLabel(t, &h.router, "Choose an account or instrument for this scope.")
	selectOption("Instrument", "Synthetic fund")
	h.click("More filters", false)
	selectOption("Valuation", "Native currency")
	h.click("Include cash", false)
	settle()
	selectOption("Account filter", "Synthetic brokerage")
	h.click("Currency", true)
	h.router.Queue(key.EditEvent{Text: "USD"})
	h.frame()
	h.click("USD", false)
	settle()
	selectOption("Asset class", "Stocks")
	selectOption("Member", "Synthetic owner")
	selectOption("Instrument filter", "Synthetic treasuries")
	h.click("More filters", false)
	settle()
	h.click("Previous month", false)
	settle()
	assertPageLabel(t, &h.router, "August 2026")

	want := func(got domain.AnalysisQuery, to domain.LocalDate) {
		t.Helper()
		if got.From != "2026-08-01" || got.To != to ||
			got.Scope != (domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: reader.instrument.String()}) ||
			got.Valuation != domain.ValuationNative || got.IncludeCash ||
			got.Filters.AccountID == nil || *got.Filters.AccountID != reader.account ||
			got.Filters.Currency == nil || *got.Filters.Currency != "USD" ||
			got.Filters.AssetClass != "stock" || got.Filters.MemberID == nil || *got.Filters.MemberID != reader.member ||
			got.Filters.InstrumentID == nil || *got.Filters.InstrumentID != reader.secondary {
			t.Fatalf("historical month and typed filters differ from Wails contract: %+v", got)
		}
	}
	h.click("Return trend", false)
	settle()
	want(reader.lastTrend, "2026-08-31")
	h.click("Calendar", false)
	settle()
	assertPageLabel(t, &h.router, "August 2026")
	h.click("Contribution", false)
	settle()
	want(reader.lastRanks, "2026-08-31")
	h.click("Calendar", false)
	settle()
	assertPageLabel(t, &h.router, "August 2026")
	if p.filters.selection.from != "2026-08-01" || p.filters.selection.to != "2026-08-31" {
		t.Fatalf("calendar range not retained on return: %+v", p.filters.selection)
	}
	selection := p.filters.selection
	h.click("2026-08-01 · No return data", true)
	settle()
	trigger := p.day.button
	h.click("View asset changes for this day", false)
	settle()
	if h.shell.activeRoute() != assetChangesRoute || assets.state.tab != assetDriversTab {
		t.Fatal("calendar day did not open Change drivers")
	}
	want(reader.lastAssets, "2026-08-01")
	h.click("Back", false)
	settle()
	h.focused(trigger)
	assertPageLabel(t, &h.router, "August 2026")
	if p.filters.selection != selection {
		t.Fatal("returning from Asset Changes did not restore the originating filters")
	}
	h.click("More filters", false)
	h.click("Clear additional filters", false)
	settle()
	if p.filters.selection != selection.clearAdditional() {
		t.Fatalf("clearing additional filters changed the dates, scope or valuation: %+v", p.filters.selection)
	}
	h.focused(&p.filters.more)
}
