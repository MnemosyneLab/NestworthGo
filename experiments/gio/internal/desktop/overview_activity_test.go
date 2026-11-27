package desktop

import (
	"image"
	"reflect"
	"testing"
	"time"

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
	assertPageLabel(t, &h.router, "Upcoming window · 2026-11-22 to 2026-12-22 (Asia/Singapore)")
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
