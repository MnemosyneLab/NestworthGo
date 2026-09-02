package settings

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStoreRoundTripUsesPrivateAtomicFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Nestworth", "settings.json")
	store := NewStore(path)
	want := Default()
	want.Appearance = AppearanceDark
	want.Accent = AccentOcean
	want.Language = LanguageZhTW
	want.Timezone = "Asia/Taipei"
	want.DateFormat = DateFormatLocalized
	want.TimeFormat = TimeFormat12
	want.Currency = "TWD"
	want.DecimalSeparator = DecimalComma
	want.GroupingSeparator = GroupingDot
	want.DecimalPlaces = 4
	want.FXProvider = FXProviderFrankfurter

	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("settings file mode = %o, want 600", mode)
	}
}

func TestDefaultUsesFrankfurterForFX(t *testing.T) {
	if DefaultFXProvider != FXProviderFrankfurter {
		t.Fatalf("DefaultFXProvider = %q, want %q", DefaultFXProvider, FXProviderFrankfurter)
	}
}

func TestDefaultStoreHonorsIsolatedSettingsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "isolated", "settings.json")
	t.Setenv("NESTWORTH_SETTINGS_PATH", path)
	if got := DefaultStore().Path; got != path {
		t.Fatalf("DefaultStore().Path = %q, want %q", got, path)
	}
}

func TestStoreCorruptFileFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"appearance":`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := NewStore(path).Load()
	if err == nil {
		t.Fatal("Load() error = nil, want corrupt-file error")
	}
	if !reflect.DeepEqual(got, Default()) {
		t.Fatalf("Load() = %#v, want defaults %#v", got, Default())
	}
}

func TestStoreLegacyFileWithoutFXProviderUsesFrankfurterDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	fields := make(map[string]json.RawMessage)
	data, err := json.Marshal(Default())
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	delete(fields, "fx_provider")
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatalf("Marshal(legacy) error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := NewStore(path).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.FXProvider != DefaultFXProvider {
		t.Fatalf("Load() FXProvider = %q, want %q", got.FXProvider, DefaultFXProvider)
	}
}

func TestValidateRejectsUnsupportedCurrencyAndSeparatorCollision(t *testing.T) {
	value := Default()
	value.Currency = "CAD"
	if err := value.Validate(); err == nil {
		t.Fatal("Validate() error = nil for unsupported currency")
	}

	value = Default()
	value.DecimalSeparator = DecimalComma
	if err := value.Validate(); err == nil {
		t.Fatal("Validate() error = nil for colliding separators")
	}

	value = Default()
	value.DecimalPlaces = 1
	if err := value.Validate(); err == nil {
		t.Fatal("Validate() error = nil for unsupported decimal places")
	}

	value = Default()
	value.FXProvider = " yahoo_finance"
	if err := value.Validate(); err == nil {
		t.Fatal("Validate() error = nil for whitespace-padded FX provider")
	}

	value = Default()
	value.FXProvider = "yahoo_finance"
	if err := value.Validate(); err == nil {
		t.Fatal("Validate() error = nil for removed Yahoo FX provider")
	}
}

func TestLoadSalvagesYahooFXProviderToFrankfurter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	stored := Default()
	stored.FXProvider = "yahoo_finance"
	data, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := NewStore(path).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.FXProvider != FXProviderFrankfurter {
		t.Fatalf("Load() FXProvider = %q, want %q", got.FXProvider, FXProviderFrankfurter)
	}
}

func TestValidateRejectsOutOfRangeWindowSize(t *testing.T) {
	value := Default()
	value.WindowWidth = MinWindowWidth - 1
	if err := value.Validate(); err == nil {
		t.Fatal("Validate() error = nil for undersized window width")
	}

	value = Default()
	value.WindowHeight = MaxWindowHeight + 1
	if err := value.Validate(); err == nil {
		t.Fatal("Validate() error = nil for oversized window height")
	}
}

// One hand-edited invalid value must not discard the rest of the stored
// preferences: only the offending field resets to its default.
func TestLoadSalvagesValidFieldsWhenOneFieldIsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Nestworth", "settings.json")
	store := NewStore(path)
	stored := Default()
	stored.Appearance = AppearanceDark
	stored.Language = LanguageZhTW
	stored.WindowWidth = 1440
	stored.DecimalPlaces = 4
	stored.Currency = "XXX" // the single invalid field

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Appearance != AppearanceDark || got.Language != LanguageZhTW || got.WindowWidth != 1440 || got.DecimalPlaces != 4 {
		t.Fatalf("Load() discarded valid fields: %#v", got)
	}
	if got.Currency != "CNY" {
		t.Fatalf("Load() kept invalid currency %q, want default CNY", got.Currency)
	}
}

func TestStoreRoundTripPersistsWindowSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := NewStore(path)
	want := Default()
	want.WindowWidth = 1440
	want.WindowHeight = 900

	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.WindowWidth != 1440 || got.WindowHeight != 900 {
		t.Fatalf("Load() window size = %vx%v, want 1440x900", got.WindowWidth, got.WindowHeight)
	}
}

func TestLoadSalvageLogsOmitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret-dir", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	value := Default()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	payload["appearance"] = "not-a-real-appearance"
	rewritten, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, rewritten, 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	if _, err := NewStore(path).Load(); err != nil {
		t.Fatal(err)
	}
	logged := buf.String()
	if strings.Contains(logged, path) || strings.Contains(logged, "secret-dir") {
		t.Fatalf("settings log leaked path: %s", logged)
	}
}
