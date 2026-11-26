package desktop

import (
	"image"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

// TestVisualSnapshotsEveryPage renders each sidebar page from a backup of the
// default Gio profile, for opt-in visual review. Page names pick the pages;
// NESTWORTH_GIO_SNAPSHOT_PAGES may hold a comma separated subset of labels.
func TestVisualSnapshotsEveryPage(t *testing.T) {
	dir := snapshotDir(t)
	ctx := t.Context()
	service, prefs := openDefaultProfileCopy(t)
	if err := prefs.ConfigureValuation(); err != nil {
		t.Fatal(err)
	}
	values, err := prefs.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now
	noop := func() {}
	directory := newDirectoryPage(newMembersList(ctx, service, noop), newInstitutionsPage(ctx, service, noop), newGroupsList(ctx, service, noop))
	timeline := newLedgerHistory(ctx, service, clock, noop)
	overview := newOverviewPage(ctx, service, clock, noop)
	wealth := newWealthHistoryPage(ctx, service, clock, noop)
	returns := &returnAnalysisPage{ctx: ctx, reader: service, clock: clock, invalidate: noop}
	assets := &assetChangesPage{ctx: ctx, reader: service, clock: clock, invalidate: noop}
	market := &marketDataPage{ctx: ctx, reader: service, invalidate: noop, manual: marketManualQuotes{service: service, clock: clock},
		latest: marketLatest{service: service}, quoteHistoryReader: service, quoteHistoryClock: clock}
	health := &dataHealthPage{ctx: ctx, reader: service, invalidate: noop}
	portfolio := newPortfolioPage(ctx, service, noop)
	portfolio.clock = clock
	tasks := newTasksPage(ctx, service, noop)
	tasks.holdingRecon = &holdingReconciliation{service: reconciliation.NewService(service), ledger: service, clock: clock}
	tasks.checks = service
	funds := newAvailableFundsPage(ctx, service, noop)
	funds.productPage = newProductDetailPage(ctx, service, noop)
	accounts := newAccountsPage(ctx, service, clock, noop)
	h := newShellHarnessWithPages(t, ledgerPages{directory: directory, history: timeline, overview: overview, funds: funds, tasks: tasks,
		accounts: accounts, portfolio: portfolio, wealthHistory: wealth, returns: returns, assets: assets, marketData: market, dataHealth: health})
	h.size = image.Pt(1375, 943)
	h.shell.prefs.saved = values
	h.shell.prefs.systemLocation = time.Local
	h.shell.prefs.systemDark = os.Getenv("NESTWORTH_GIO_SNAPSHOT_DARK") != ""
	settle := func() {
		for range 4 {
			h.frame()
			overview.workers.Wait()
			accounts.workers.Wait()
			portfolio.workers.Wait()
			tasks.workers.Wait()
			funds.workers.Wait()
			wealth.workers.Wait()
			returns.workers.Wait()
			assets.workers.Wait()
			market.workers.Wait()
			health.workers.Wait()
			timeline.model.Wait()
			directory.wait()
		}
	}
	t.Cleanup(settle)
	only := strings.Split(snapshotPages(), ",")
	pages := []struct{ label, file string }{
		{"概览", "overview"}, {"账户", "accounts"}, {"投资资产", "portfolio"}, {"待办核查", "tasks"}, {"账务记录", "history"},
		{"资产回溯", "wealth"}, {"收益分析", "returns"}, {"资产变动", "asset-changes"}, {"名录", "directory"},
		{"行情数据", "market"}, {"设置", "settings"},
	}
	settle()
	for _, page := range pages {
		if snapshotPages() != "" && !contains(only, page.file) {
			continue
		}
		h.frame()
		h.click(page.label, false)
		settle()
		name := "page-" + page.file
		if h.shell.prefs.systemDark {
			name += "-dark"
		}
		h.snapshot(t, dir, name)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func snapshotPages() string { return os.Getenv("NESTWORTH_GIO_SNAPSHOT_PAGES") }
