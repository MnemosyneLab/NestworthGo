package desktop

import (
	"context"
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type overviewHealthFixture struct {
	overviewReader
	fixedDataHealthRead
}

func (f overviewHealthFixture) UpcomingActions(context.Context) (domain.UpcomingActions, error) {
	return domain.UpcomingActions{WindowStartOn: "2026-11-22", WindowEndOn: "2026-12-22", Timezone: "Asia/Singapore"}, nil
}

func TestOverviewPairsActionCardsAndOpensAuthoritativeHealthFinding(t *testing.T) {
	ctx := t.Context()
	clock := func() time.Time { return accountTestClock }
	issues := []application.HealthIssue{
		{ID: "snapshot", Kind: application.HealthKindSnapshotOutdated, Action: application.HealthActionRepair, Label: "Synthetic snapshots"},
		{ID: "pending", Kind: application.HealthKindHistoryPending, Severity: application.HealthSeverityWarning, Action: application.HealthActionNone, Label: "Synthetic pending"},
		{ID: "provisional", Kind: application.HealthKindValuationProvisional, Severity: application.HealthSeverityInfo, Action: application.HealthActionNone, Label: "Synthetic provisional"},
		{ID: "provider", Kind: application.HealthKindMissingProviderKey, Severity: application.HealthSeverityBlocking, Action: application.HealthActionProviderSettings, Label: "Synthetic provider"},
		{ID: "collapsed", Kind: application.HealthKindMissingManualPrice, Action: application.HealthActionManualEntry, Label: "Synthetic collapsed", Collapsed: true},
		{ID: "history", Kind: application.HealthKindHistoryNotStarted, Action: application.HealthActionNone, Label: "Synthetic start"},
	}
	fixture := overviewHealthFixture{
		overviewReader: overviewReaderFunc(func(context.Context, domain.AccountFilter) (domain.OverviewResult, error) {
			return domain.OverviewResult{Currency: "USD", AccountCount: 1, HistoryStarted: true}, nil
		}),
		fixedDataHealthRead: fixedDataHealthRead{IssueCount: 3, Issues: issues},
	}
	page := newOverviewPage(ctx, fixture, clock, func() {})
	health := &dataHealthPage{ctx: ctx, invalidate: func() {}, reader: fixture}
	ledger := newHistoryPage(history.New(ctx, activityReaderFunc(func(context.Context, domain.ActivityQuery) (domain.ActivityPage, error) {
		return domain.ActivityPage{}, nil
	})), func() {})
	t.Cleanup(page.workers.Wait)
	t.Cleanup(health.workers.Wait)
	t.Cleanup(ledger.model.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{overview: page, dataHealth: health, history: ledger})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1260, 1020)
	settle := func() {
		for range 5 {
			h.frame()
			page.workers.Wait()
			health.workers.Wait()
			ledger.model.Wait()
		}
		h.frame()
	}
	settle()
	assertPageLabel(t, &h.router, "Data health · 3 issues")
	page.list.ScrollTo(page.trendIndex() - 1)
	settle()
	attention := assertPageLabel(t, &h.router, "Needs attention")
	upcoming := assertPageLabel(t, &h.router, "Next 30 days")
	if attention.Max.X >= upcoming.Min.X || attention.Min.Y > upcoming.Max.Y || upcoming.Min.Y > attention.Max.Y {
		t.Fatalf("wide Overview did not place its action cards side by side: %v %v", attention, upcoming)
	}
	assertPageLabel(t, &h.router, "Blocking")
	assertPageLabel(t, &h.router, "Warning")
	assertPageLabel(t, &h.router, "Info")
	assertPageLabel(t, &h.router, "Record the first balance, transfer, or investment change.")
	for _, suppressed := range []string{"Synthetic snapshots · Outdated snapshots", "Synthetic collapsed · Manual price needed", "Synthetic start · History not started"} {
		if findPageLabel(&h.router, suppressed) != nil {
			t.Fatalf("overview displayed a suppressed finding: %s", suppressed)
		}
	}
	if got := page.healthScan.findings; len(got) != 3 || got[0].ID != "provider" || got[1].ID != "pending" || got[2].ID != "provisional" {
		t.Fatalf("unexpected prioritized findings: %+v", got)
	}
	h.click("View gap · Synthetic provider · Provider configuration needed", false)
	settle()
	if h.shell.activeRoute() != dataHealthRoute || health.focus.id != "provider" {
		t.Fatalf("Overview opened the wrong Health finding: %+v", health.focus)
	}
	h.click("Back", false)
	settle()
	h.focused(page.healthScan.links["provider"])
	h.press(key.NameReturn)
	settle()
	if h.shell.activeRoute() != dataHealthRoute || health.focus.id != "provider" {
		t.Fatal("restored finding link did not reopen on Enter")
	}
	h.click("Back", false)
	settle()
	h.click("Open Ledger", false)
	settle()
	if h.shell.activeRoute() != historyRoute {
		t.Fatal("first-activity action did not open the Ledger")
	}
	h.click("Back", false)
	settle()
	h.focused(&page.firstActivity)

	h.size = image.Pt(660, 1040)
	page.list.ScrollTo(page.trendIndex() - 1)
	settle()
	attention = assertPageLabel(t, &h.router, "Needs attention")
	upcoming = assertPageLabel(t, &h.router, "Next 30 days")
	if attention.Max.Y >= upcoming.Min.Y || attention.Max.X <= upcoming.Min.X || upcoming.Max.X <= attention.Min.X {
		t.Fatalf("narrow Overview did not stack its action cards: %v %v", attention, upcoming)
	}
}
