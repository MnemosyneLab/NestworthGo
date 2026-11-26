package desktop

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// TestVisualSnapshotsSampleLedgerPages renders Overview and Accounts from a
// backup of the default Gio profile. The live database is not opened.
func TestVisualSnapshotsSampleLedgerPages(t *testing.T) {
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
	overview := newOverviewPage(ctx, service, clock, func() {})
	accounts := newAccountsPage(ctx, service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{overview: overview, accounts: accounts})
	h.size = image.Pt(1375, 943)
	h.shell.prefs.saved = values
	h.shell.prefs.systemLocation = time.Local
	h.shell.prefs.systemDark = false
	t.Cleanup(func() {
		overview.workers.Wait()
		accounts.workers.Wait()
	})

	h.frame()
	overview.workers.Wait()
	h.frame()
	overview.workers.Wait()
	h.frame()
	if overview.current == nil {
		t.Fatalf("overview did not load: %v", overview.err)
	}
	h.snapshot(t, dir, "overview-default")

	// Snapshot frames at 2x. Lay out once more at the window size before clicking.
	h.frame()
	h.click("账户", false)
	accounts.workers.Wait()
	h.frame()
	if h.shell.activeRoute() != accountsRoute {
		t.Fatalf("click did not open accounts, route %s", h.shell.activeRoute())
	}
	if accounts.current == nil {
		t.Fatalf("accounts did not load: %v", accounts.err)
	}
	h.snapshot(t, dir, "accounts-default")
}

func openDefaultProfileCopy(t *testing.T) (*application.Service, *preferences.Service) {
	t.Helper()
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(config, pilot.AppID, "data")
	root := createFixtureRoot(t)
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(data, "nestworth.db")
	settingsPath := filepath.Join(data, "settings.json")
	backup := exec.Command("/usr/bin/sqlite3", "-readonly", filepath.Join(source, "nestworth.db"),
		fmt.Sprintf(".backup %s", database))
	if out, err := backup.CombinedOutput(); err != nil {
		t.Fatalf("backup default Gio database: %v\n%s", err, out)
	}
	settings, err := os.ReadFile(filepath.Join(source, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, settings, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NESTWORTH_DATABASE_PATH", database)
	t.Setenv("NESTWORTH_SETTINGS_PATH", settingsPath)
	paths, err := pilot.PathsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	session, err := pilot.OpenSession(t.Context(), paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	repo := sqlite.NewRepository(session.DB).WithDefaultReviewOptimizations()
	service := application.NewService(repo)
	appports.Wire(service)
	appports.AttachSQLiteHistory(service, repo)
	prefs, err := preferences.Open(paths.SettingsPath(), session.DB, service)
	if err != nil {
		t.Fatal(err)
	}
	return service, prefs
}
