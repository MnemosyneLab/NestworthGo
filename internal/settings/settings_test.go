package settings

import (
	"os"
	"path/filepath"
	"reflect"
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
