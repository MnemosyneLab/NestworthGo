package desktop

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestLedgerHistoryUsesSavedGeneralFormatAndRecordedDay(t *testing.T) {
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
	service := application.NewService(sqlite.NewRepository(db))
	store, err := preferences.Open(paths.SettingsPath(), db, service)
	if err != nil {
		t.Fatal(err)
	}
	prefs := newPreferenceState(ctx, store, func() {})
	prefs.reload()
	prefs.wait()
	model := history.New(ctx, service, func() {})
	defer model.Wait()
	h := newShellHarnessWithPages(t, ledgerPages{prefs: prefs, history: newHistoryPage(model).layout})
	h.size = image.Pt(1120, 960)
	h.frame()
	lang := h.shell.preferences.language
	h.click(lang.Text(i18n.NavHistory), false)
	model.Wait()
	h.frame()
	if model.Err() != nil || h.shell.activeRoute != historyRoute {
		t.Fatalf("sidebar loaded route %q with error %v", h.shell.activeRoute, model.Err())
	}
	assertPageLabel(t, &h.router, "Bank interest")
	assertPageLabel(t, &h.router, "2026-11-12")

	h.click(lang.Text(i18n.NavSettings), false)
	h.click(lang.Text(i18n.SettingsDate), true)
	h.click("DD/MM/YYYY", false)
	h.click(lang.Text(i18n.SettingsTimezone), true)
	h.router.Queue(key.EditEvent{Text: "UTC"})
	h.frame()
	h.click("UTC", false)
	h.click(lang.Text(i18n.SettingsSave), false)
	prefs.wait()
	h.frame()
	assertPageLabel(t, &h.router, lang.Text(i18n.SettingsSaved))
	saved, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if saved.DateFormat != settings.DateFormatDayFirst || saved.Timezone != "UTC" || saved.HistoryTimezone != "Asia/Singapore" {
		t.Fatalf("unexpected saved presentation or history timezone: %+v", saved)
	}

	h.click(lang.Text(i18n.NavHistory), false)
	assertPageLabel(t, &h.router, "Bank interest")
	assertPageLabel(t, &h.router, "12/11/2026")
	page, err := service.ListActivityPage(ctx, domain.ActivityQuery{Limit: 1})
	if err != nil || len(page.Activities) != 1 {
		t.Fatalf("reading the latest recorded activity: %v, got %d rows", err, len(page.Activities))
	}
	// The display zone sees November 11 at 16:30. The ledger recorded November 12 at 00:30.
	interest := page.Activities[0]
	if interest.Note == nil || *interest.Note != "Bank interest" || interest.EffectiveLocalDate != "2026-11-12" ||
		interest.EffectiveAt.UTC().Format(time.RFC3339) != "2026-11-11T16:30:00Z" {
		t.Fatalf("unexpected recorded interest activity: %+v", interest)
	}
}
