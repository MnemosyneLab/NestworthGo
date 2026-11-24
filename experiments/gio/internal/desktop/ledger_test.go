package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestHistorySidebarOpensTheIsolatedLedger(t *testing.T) {
	ctx := t.Context()
	runDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NESTWORTH_GIO_RUN_DIR", runDir)
	t.Setenv("NESTWORTH_DATABASE_PATH", filepath.Join(runDir, "data", "nestworth.db"))
	t.Setenv("NESTWORTH_SETTINGS_PATH", filepath.Join(runDir, "data", "settings.json"))
	paths, err := pilot.PathsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if err := pilot.Prepare(ctx, paths); err != nil {
		t.Fatal(err)
	}
	db, err := pilot.Open(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	model := history.New(ctx, application.NewService(sqlite.NewRepository(db)), func() {})
	defer model.Wait()
	h := newShellHarnessWithPages(t, ledgerPages{history: newHistoryPage(model).layout})
	h.frame()
	h.click(i18n.SimplifiedChinese.Text(i18n.NavHistory), false)
	model.Wait()
	h.frame()
	if model.Err() != nil || h.shell.activeRoute != historyRoute {
		t.Fatalf("sidebar loaded route %q with error %v", h.shell.activeRoute, model.Err())
	}
	assertPageLabel(t, &h.router, "Bank interest")
	// This activity was recorded just after midnight in the household timezone.
	assertPageLabel(t, &h.router, "2026-11-12")
}
