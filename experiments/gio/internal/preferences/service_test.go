package preferences

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/testfixture"
)

func TestPreferencesPreserveUneditedValuesOnSaveAndCredentialsOnReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	dbPath := filepath.Join(dir, "household.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := application.NewService(&testfixture.BaselineRepository{Repository: sqlite.NewRepository(db)})
	ctx := t.Context()
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Preferences QA", BaseCurrency: "USD", MemberNames: []string{"Member"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CaptureSyntheticOrigin(ctx, "Asia/Tokyo"); err != nil {
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
	if err := service.ConfigureValuation(); err != nil || app.QuoteCacheTTL() != 3*time.Hour {
		t.Fatalf("restore cache duration on startup: %s, %v", app.QuoteCacheTTL(), err)
	}
	first, err := service.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.HouseholdCurrency != "USD" || first.HistoryTimezone != "Asia/Tokyo" || first.WeekStart != settings.WeekStartSunday || !first.CoinGeckoConfigured {
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
	draft.QuoteCacheTTL = settings.QuoteCacheTTL24h
	draft.HouseholdCurrency, draft.HistoryTimezone = "JPY", "Pacific/Auckland"
	wrote, err := service.Save(ctx, first, draft)
	if err != nil {
		t.Fatal(err)
	}
	if wrote.Language != settings.LanguageJa || wrote.HouseholdCurrency != "USD" || wrote.HistoryTimezone != "Asia/Tokyo" {
		t.Fatalf("stale page draft overwrote sidebar or ledger values: %+v", wrote)
	}
	if app.QuoteCacheTTL() != 24*time.Hour {
		t.Fatalf("save did not apply the cache duration: %s", app.QuoteCacheTTL())
	}
	persisted, err := service.store.Load()
	if err != nil || persisted.Currency != other.Currency || persisted.WeekStart != other.WeekStart || persisted.DecimalPlaces != other.DecimalPlaces ||
		persisted.WindowWidth != other.WindowWidth || persisted.WindowHeight != other.WindowHeight || persisted.CoinGeckoAPIKey != other.CoinGeckoAPIKey {
		t.Fatalf("ordinary save changed unedited settings: %v", err)
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
	service, err = Open(path, db, application.NewService(&testfixture.BaselineRepository{Repository: sqlite.NewRepository(db)}))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := service.Load(ctx); err != nil || got != wrote {
		t.Fatalf("settings after reopen: %+v, %v", got, err)
	}
	reset, err := service.Reset(ctx)
	want := Default()
	want.HouseholdCurrency, want.HistoryTimezone, want.LogPath = "USD", "Asia/Tokyo", wrote.LogPath
	want.CoinGeckoConfigured = true
	if err != nil || reset != want {
		t.Fatalf("reset: %+v, %v", reset, err)
	}
	wantStored := settings.Default()
	wantStored.CoinGeckoAPIKey = other.CoinGeckoAPIKey
	if persisted, err := service.store.Load(); err != nil || persisted != wantStored {
		t.Fatalf("reset lost the credential or failed to restore preferences: %v", err)
	}
}

func TestPreferencesBeforeOnboarding(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := Open(filepath.Join(dir, "settings.json"), db, application.NewService(&testfixture.BaselineRepository{Repository: sqlite.NewRepository(db)}))
	if err != nil {
		t.Fatal(err)
	}
	want := Default()
	want.LogPath = filepath.Join(dir, "logs", "nestworth.log")
	if got, err := service.Load(t.Context()); err != nil || got != want {
		t.Fatalf("before onboarding: %+v, %v", got, err)
	}
}

func TestMarketCredentialAndDiagnosticLoggingUseProfileStorage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	db, err := sqlite.Open(filepath.Join(dir, "household.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := application.NewService(&testfixture.BaselineRepository{Repository: sqlite.NewRepository(db)})
	service, err := Open(path, db, app)
	if err != nil {
		t.Fatal(err)
	}
	log, err := service.OpenLog()
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	ctx := t.Context()
	initial, err := service.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(initial.LogPath); !os.IsNotExist(err) {
		t.Fatalf("logging off created a file: %v", err)
	}
	const fixtureKey = "synthetic-credential"
	updated, err := service.SetCoinGeckoKey(ctx, "  "+fixtureKey+"  ")
	if err != nil || !updated.CoinGeckoConfigured {
		t.Fatalf("key was not configured: %v", err)
	}
	if got, err := service.CoinGeckoKey(); err != nil || got != fixtureKey {
		t.Fatalf("provider could not load the trimmed credential: %v", err)
	}
	if _, err := service.SetCoinGeckoKey(ctx, "   "); err == nil {
		t.Fatal("whitespace replaced the credential")
	}
	draft := initial
	draft.LogLevel, draft.QuoteCacheTTL = "info", settings.QuoteCacheTTL1h
	saved, err := service.Save(ctx, initial, draft)
	if err != nil || !saved.CoinGeckoConfigured || app.QuoteCacheTTL() != time.Hour {
		t.Fatalf("shared draft did not preserve the separate credential or set the cache duration: %v", err)
	}
	log.Logger().Info("gio test log marker")
	logged, err := os.ReadFile(saved.LogPath)
	if err != nil || !strings.Contains(string(logged), "gio test log marker") {
		t.Fatalf("saved logging level did not enable file logging: %v", err)
	}
	reset, err := service.Reset(ctx)
	if err != nil || reset.LogLevel != "off" || !reset.CoinGeckoConfigured || app.QuoteCacheTTL() != 12*time.Hour {
		t.Fatalf("reset did not restore runtime defaults and preserve the credential: %v", err)
	}
	log.Logger().Info("disabled log marker")
	if got, err := os.ReadFile(saved.LogPath); err != nil || string(got) != string(logged) {
		t.Fatalf("file logging continued after reset: %v", err)
	}
	cleared, err := service.SetCoinGeckoKey(ctx, "")
	if err != nil || cleared.CoinGeckoConfigured {
		t.Fatalf("credential was not revoked: %v", err)
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
	service, err := Open(path, db, application.NewService(&testfixture.BaselineRepository{Repository: sqlite.NewRepository(db)}))
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
	next.QuoteCacheTTL, next.CoinGeckoAPIKey = settings.QuoteCacheTTL24h, "fixture-key"
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
