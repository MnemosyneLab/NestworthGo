package desktop

import (
	"image"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestOverviewUpcomingAndRecentActivityUseTheWailsReads(t *testing.T) {
	ctx := t.Context()
	service, _, _, fixture := openManagedProductFixture(t)
	if err := seedAdditionalTaskProducts(ctx, service, fixture, accountTestClock); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return accountTestClock }
	reader := &firstTasksReadFailure{Service: service}
	page := newOverviewPage(ctx, reader, clock, func() {})
	tasks := newTasksPage(ctx, service, func() {})
	tasks.checks = service
	ledger := newLedgerHistory(ctx, service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{overview: page, tasks: tasks, history: ledger})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1120, 1000)
	settle := func() {
		for range 4 {
			h.frame()
			page.workers.Wait()
			tasks.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
		h.frame()
	}
	settle()
	if page.current == nil || page.upcoming.err == nil {
		t.Fatal("a failed upcoming action read hid the overview data")
	}
	page.list.ScrollTo(page.trendIndex() - 1)
	settle()
	assertPageLabel(t, &h.router, "Could not load upcoming actions.")
	h.click("Retry", false)
	settle()
	data := page.upcoming.current
	want, err := liquidity.NewService(service).UpcomingActions(ctx)
	if err != nil || data == nil || !reflect.DeepEqual(liquidity.FromUpcomingActions(*data), want) {
		t.Fatalf("overview used different upcoming evidence: %+v / %v", data, err)
	}
	overview, err := portfolio.NewService(service).Overview(ctx, account.AccountFilterRequest{})
	if err != nil || len(overview.RecentActivities) == 0 ||
		!reflect.DeepEqual(wire.FromActivities(page.current.value.RecentActivities), overview.RecentActivities) {
		t.Fatalf("overview used different recent activity: %+v / %v", overview.RecentActivities, err)
	}
	assertPageLabel(t, &h.router, "Next 30 days")
	assertPageLabel(t, &h.router, "Upcoming window · 2026-11-22 to 2026-12-22")
	assertPageLabel(t, &h.router, "Read at 2026-11-22 01:00:00")
	if bounds := semanticBounds(t, &h.router, "View upcoming actions", false); bounds.Dx() > 200 {
		t.Fatalf("upcoming action spans %dpx; want a compact link", bounds.Dx())
	}
	h.click("View upcoming actions", false)
	settle()
	if h.shell.activeRoute() != tasksRoute || tasks.tab != tasksUpcoming {
		t.Fatal("upcoming actions did not open the Upcoming tab")
	}
	h.click("Back", false)
	settle()
	h.focused(&page.upcoming.all)
	page.list.ScrollTo(2)
	settle()
	h.click("Investment review · 0", false)
	settle()
	if h.shell.activeRoute() != tasksRoute || tasks.tab != tasksInvestment {
		t.Fatal("the review count did not open Investment review")
	}
	h.click("Back", false)
	settle()
	h.focused(&page.upcoming.summary.links[tasksInvestment])
	h.click("Open reconciliations · 0", false)
	settle()
	if h.shell.activeRoute() != tasksRoute || tasks.tab != tasksReconciliation {
		t.Fatal("the session count did not open Reconciliation")
	}
	h.click("Back", false)
	settle()
	h.focused(&page.upcoming.summary.links[tasksReconciliation])
	page.list.ScrollTo(page.recentActivityIndex())
	settle()
	assertPageLabel(t, &h.router, "Recent activity")
	if bounds := semanticBounds(t, &h.router, "View all activity", false); bounds.Dx() > 200 {
		t.Fatalf("recent activity action spans %dpx; want a compact header action", bounds.Dx())
	}
	h.click("View all activity", false)
	settle()
	if h.shell.activeRoute() != historyRoute {
		t.Fatal("recent activity did not open Ledger activity")
	}
	h.click("Back", false)
	settle()
	h.focused(&page.recent)
}

func TestOverviewRecentActivityUsesLocalizedSentences(t *testing.T) {
	brokerage, cash := domain.AccountID("brokerage"), domain.AccountID("cash")
	instrument := domain.InstrumentID("instrument")
	names := historyRowNames{
		accounts:    map[domain.AccountID]domain.Account{brokerage: {Name: "US Brokerage"}, cash: {Name: "Checking"}},
		instruments: map[domain.InstrumentID]string{instrument: "AAPL"},
	}
	money := func(amount string, currency domain.CurrencyCode) *domain.Money {
		t.Helper()
		result, err := domain.ParseMoney(amount, currency)
		if err != nil {
			t.Fatal(err)
		}
		return &result
	}
	quantity, err := domain.ParseQuantity("3")
	if err != nil {
		t.Fatal(err)
	}
	gross, dividend, fee, sent := money("615", "USD"), money("12.5", "USD"), money("1.25", "USD"), money("100", "USD")
	for _, tc := range []struct {
		activity domain.Activity
		en, zh   string
	}{
		{domain.Activity{Kind: domain.ActivitySell, TradeDetail: &domain.TradeDetail{InstrumentID: instrument, Quantity: quantity, Gross: *gross},
			Effects: []domain.ActivityEffect{{Role: domain.EffectRolePrincipal, AccountID: &brokerage, Money: gross}}},
			"Sold 3 AAPL in US Brokerage for US$615.00", "在 US Brokerage 以 US$615.00 卖出 3 AAPL"},
		{domain.Activity{Kind: domain.ActivityCashDividend, Reason: domain.ReasonIncome,
			DividendDetail: &domain.DividendDetail{InstrumentID: instrument, Amount: *dividend},
			Effects:        []domain.ActivityEffect{{AccountID: &brokerage, Money: dividend}}},
			"Dividend US$12.50 from AAPL into US Brokerage", "从 AAPL 分得 US$12.50 至 US Brokerage"},
		{domain.Activity{Kind: domain.ActivityCashTransfer,
			Effects: []domain.ActivityEffect{{Role: domain.EffectRoleTransferTo, AccountID: &cash, Money: sent},
				{Role: domain.EffectRoleTransferFrom, AccountID: &brokerage, Money: sent},
				{Role: domain.EffectRoleFee, AccountID: &brokerage, Money: fee}}},
			"Transferred US$100.00 from US Brokerage to Checking (fee US$1.25)", "将 US$100.00 从 US Brokerage 转到 Checking（费用 US$1.25）"},
	} {
		for lang, want := range map[i18n.Language]string{i18n.English: tc.en, i18n.SimplifiedChinese: tc.zh} {
			view := ui.View{Language: lang, GroupingSeparator: settings.GroupingComma, DecimalSeparator: settings.DecimalDot}
			if got := overviewActivitySentence(view, tc.activity, names); got != want {
				t.Errorf("%s/%s: got %q, want %q", lang, tc.activity.Kind, got, want)
			}
		}
	}
}
