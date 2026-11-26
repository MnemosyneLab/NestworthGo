package desktop

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/settings"
)

// Renders the Overview and Accounts pages from the synthetic bank fixture.
// Opt in with NESTWORTH_GIO_SNAPSHOT_DIR. This file is a one-off visual check.
func TestVisualSnapshotsSampleLedgerPages(t *testing.T) {
	dir := snapshotDir(t)
	service, _ := openBankFixture(t, createFixtureRoot(t))
	clock := func() time.Time { return accountTestClock }
	overview := newOverviewPage(t.Context(), service, clock, func() {})
	accounts := newAccountsPage(t.Context(), service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{overview: overview, accounts: accounts})
	h.shell.prefs.saved.Language = settings.LanguageZhCN
	h.shell.prefs.saved.Appearance = settings.AppearanceLight
	t.Cleanup(func() {
		overview.workers.Wait()
		accounts.workers.Wait()
	})

	h.frame()
	overview.workers.Wait()
	h.frame()
	if overview.current == nil {
		t.Fatalf("overview did not load: %v", overview.err)
	}
	h.snapshot(t, dir, "overview-sample")

	h.click("账户", false)
	accounts.workers.Wait()
	h.frame()
	if accounts.current == nil {
		t.Fatalf("accounts did not load: %v", accounts.err)
	}
	h.snapshot(t, dir, "accounts-sample")
}
