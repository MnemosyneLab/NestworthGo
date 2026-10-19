package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestWindowSizeFromSettingsRestoresSavedSize(t *testing.T) {
	preference := settings.Default()
	preference.WindowWidth = 1440
	preference.WindowHeight = 900

	width, height := windowSizeFromSettings(preference)
	if width != 1440 || height != 900 {
		t.Fatalf("windowSizeFromSettings() = %dx%d, want 1440x900", width, height)
	}
}

func TestWindowSizeFromSettingsUsesDefaultsForInvalidSize(t *testing.T) {
	preference := settings.Default()
	preference.WindowWidth = settings.MinWindowWidth - 1
	preference.WindowHeight = settings.DefaultWindowHeight

	width, height := windowSizeFromSettings(preference)
	if width != settings.DefaultWindowWidth || height != settings.DefaultWindowHeight {
		t.Fatalf("windowSizeFromSettings() = %dx%d, want defaults %dx%d", width, height, settings.DefaultWindowWidth, settings.DefaultWindowHeight)
	}
}

func TestPersistWindowSizeValueLoadsLatestSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := settings.NewStore(path)
	value := settings.Default()
	value.Appearance = settings.AppearanceDark
	value.Language = settings.LanguageZhCN
	value.Currency = "USD"
	if err := store.Save(value); err != nil {
		t.Fatalf("initial Save() error = %v", err)
	}

	latest := value
	latest.Appearance = settings.AppearanceLight
	latest.Language = settings.LanguageZhTW
	latest.Currency = "TWD"
	if err := store.Save(latest); err != nil {
		t.Fatalf("latest Save() error = %v", err)
	}

	if err := persistWindowSizeValue(store, 1440, 900); err != nil {
		t.Fatalf("persistWindowSizeValue() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Appearance != latest.Appearance || got.Language != latest.Language || got.Currency != latest.Currency {
		t.Fatalf("persistWindowSizeValue() overwrote latest settings: %#v", got)
	}
	if got.WindowWidth != 1440 || got.WindowHeight != 900 {
		t.Fatalf("window size = %vx%v, want 1440x900", got.WindowWidth, got.WindowHeight)
	}
}

func TestPersistWindowSizeValuePreservesMalformedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"appearance":`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if err := persistWindowSizeValue(settings.NewStore(path), 1440, 900); err == nil {
		t.Fatal("persistWindowSizeValue() unexpectedly overwrote malformed settings")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after recovery = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("persistWindowSizeValue() changed malformed settings")
	}
	got, err := settings.NewStore(path).Load()
	if err != nil {
		t.Fatalf("Load() after preservation = %v", err)
	}
	if got != settings.Default() {
		t.Fatalf("Load() after preservation = %#v, want temporary defaults %#v", got, settings.Default())
	}
}

func TestPersistWindowSizeValuePreservesUnsupportedSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	value := settings.Default()
	value.SchemaVersion++
	before, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := persistWindowSizeValue(settings.NewStore(path), 1440, 900); err == nil {
		t.Fatal("persistWindowSizeValue() unexpectedly overwrote unsupported schema")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after preservation = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("persistWindowSizeValue() changed unsupported schema")
	}
}

func TestPersistWindowSizeLogsOmitErrorAndPath(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	path := filepath.Join(t.TempDir(), "settings.json")
	store := settings.NewStore(path)
	if err := store.Save(settings.Default()); err != nil {
		t.Fatal(err)
	}
	if err := persistWindowSizeValue(store, int(settings.MaxWindowWidth)+1, 900); err == nil {
		t.Fatal("expected invalid window size to fail")
	}
	logged := buf.String()
	if strings.Contains(logged, path) || strings.Contains(logged, "error=") || strings.Contains(logged, "width=") {
		t.Fatalf("log leaked forbidden fields: %s", logged)
	}
}

func TestDefaultDatabasePathKeepsExplicitTestAndUserPaths(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "ledger with spaces - 東京 ? #.db")
	path, err := defaultDatabasePathFrom(explicit, func() (string, error) {
		t.Fatal("explicit database path should not consult the user config directory")
		return "", nil
	})
	if err != nil || path != explicit {
		t.Fatalf("explicit path = %q, err=%v; want %q", path, err, explicit)
	}

	configDir := filepath.Join(t.TempDir(), "App Data")
	path, err = defaultDatabasePathFrom("", func() (string, error) { return configDir, nil })
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(configDir, "Nestworth", "nestworth.db")
	if path != want {
		t.Fatalf("default path = %q, want %q", path, want)
	}
}

func TestDefaultDatabasePathFailsActionablyWhenConfigLocationIsUnavailable(t *testing.T) {
	path, err := defaultDatabasePathFrom("", func() (string, error) {
		return "", errors.New("synthetic config directory failure")
	})
	if err == nil || path != "" {
		t.Fatalf("path = %q, err=%v; expected a failure", path, err)
	}
	if !strings.Contains(err.Error(), "NESTWORTH_DATABASE_PATH") || !strings.Contains(err.Error(), "persistent database location") || strings.Contains(strings.ToLower(err.Error()), "temporary") {
		t.Fatalf("error is not actionable without suggesting a temporary ledger: %v", err)
	}

	_, err = defaultDatabasePathFrom("", func() (string, error) { return "", nil })
	if err == nil || !strings.Contains(err.Error(), "NESTWORTH_DATABASE_PATH") {
		t.Fatalf("empty config directory error = %v", err)
	}
}
