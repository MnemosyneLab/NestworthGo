package preferences

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestGeneralPreferencesPreserveOtherSettingsAcrossSaveResetAndReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	dbPath := filepath.Join(dir, "household.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := application.NewService(sqlite.NewRepository(db))
	ctx := t.Context()
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Preferences QA", BaseCurrency: "USD", MemberNames: []string{"Member"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.StartHistory(ctx, "Asia/Tokyo"); err != nil {
		t.Fatal(err)
	}
	other := settings.Default()
	other.Currency, other.WeekStart, other.DecimalPlaces = "SGD", settings.WeekStartSunday, 4
	other.CoinGeckoAPIKey, other.QuoteCacheTTL = "synthetic-fixture-token", settings.QuoteCacheTTL3h
	other.WindowWidth, other.WindowHeight = 1333, 800
	if err := settings.NewStore(path).Save(other); err != nil {
		t.Fatal(err)
	}
	service, err := Open(path, db, app)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.HouseholdCurrency != "USD" || first.HistoryTimezone != "Asia/Tokyo" || first.WeekStart != settings.WeekStartSunday {
		t.Fatalf("incorrect ledger context %+v", first)
	}
	sidebar := first
	sidebar.Language = settings.LanguageJa
	if _, err := service.Save(ctx, first, sidebar); err != nil {
		t.Fatal(err)
	}
	draft := first
	draft.Timezone = "America/New_York"
	draft.Appearance, draft.Accent = settings.AppearanceDark, settings.AccentMint
	draft.DateFormat, draft.TimeFormat = settings.DateFormatDayFirst, settings.TimeFormat12
	draft.DecimalSeparator, draft.GroupingSeparator = settings.DecimalComma, settings.GroupingSpace
	draft.HouseholdCurrency, draft.HistoryTimezone = "JPY", "Pacific/Auckland"
	wrote, err := service.Save(ctx, first, draft)
	if err != nil {
		t.Fatal(err)
	}
	if wrote.Language != settings.LanguageJa || wrote.HouseholdCurrency != "USD" || wrote.HistoryTimezone != "Asia/Tokyo" {
		t.Fatalf("stale page draft overwrote sidebar or ledger values: %+v", wrote)
	}
	invalid := wrote
	invalid.DecimalSeparator, invalid.GroupingSeparator = ".", "."
	if _, err := service.Save(ctx, wrote, invalid); err == nil {
		t.Fatal("invalid separators saved")
	}
	raw, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(raw), "synthetic-fixture-token") || strings.Contains(string(raw), "America/New_York") {
		t.Fatalf("durable preferences leaked into presentation file or cannot read it: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	service, err = Open(path, db, application.NewService(sqlite.NewRepository(db)))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := service.Load(ctx); err != nil || got != wrote {
		t.Fatalf("settings after reopen: %+v, %v", got, err)
	}
	reset, err := service.Reset(ctx)
	want := Default()
	want.HouseholdCurrency, want.HistoryTimezone, want.WeekStart = "USD", "Asia/Tokyo", settings.WeekStartSunday
	if err != nil || reset != want {
		t.Fatalf("reset: %+v, %v", reset, err)
	}
	if persisted, err := service.store.Load(); err != nil || persisted != other {
		t.Fatalf("reset changed settings outside General or failed to restore defaults: %v", err)
	}
}

func TestPreferencesBeforeOnboarding(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := Open(filepath.Join(dir, "settings.json"), db, application.NewService(sqlite.NewRepository(db)))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := service.Load(t.Context()); err != nil || got != Default() {
		t.Fatalf("before onboarding: %+v, %v", got, err)
	}
}

func TestFailedPublicationRestoresDurableTimezone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	db, err := sqlite.Open(filepath.Join(dir, "household.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := Open(path, db, application.NewService(sqlite.NewRepository(db)))
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	// A directory blocks the file rename even when tests run as root. The
	// SQLite configuration is still writable when the settings save starts.
	original := filepath.Join(dir, "original.json")
	if err := os.Rename(path, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	next := current
	next.Appearance, next.Timezone = settings.AppearanceDark, "Asia/Tokyo"
	if err := service.persist(current, next); err == nil {
		t.Fatal("save reported success after the presentation file failed")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(original, path); err != nil {
		t.Fatal(err)
	}
	if got, err := service.store.Load(); err != nil || got != current {
		t.Fatalf("failed publication left a partial settings change, error %v", err)
	}
}
