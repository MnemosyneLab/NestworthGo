package settings_test

import (
	"path/filepath"
	"testing"

	appsettings "github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func newTestService(t *testing.T) *settings.Service {
	t.Helper()
	store := appsettings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	app := wailstest.NewService(t)
	return settings.NewService(store, app)
}

func TestLoadReturnsDefaultsWhenNoFileExists(t *testing.T) {
	service := newTestService(t)
	value, err := service.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if value != appsettings.Default() {
		t.Fatalf("Load() = %+v, want defaults", value)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	service := newTestService(t)
	updated := appsettings.Default()
	updated.Appearance = appsettings.AppearanceDark
	updated.Language = appsettings.LanguageZhCN
	if err := service.Save(updated); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := service.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Appearance != appsettings.AppearanceDark || loaded.Language != appsettings.LanguageZhCN {
		t.Fatalf("loaded = %+v, want dark/zh-CN", loaded)
	}
}

func TestSaveRejectsInvalidSettings(t *testing.T) {
	service := newTestService(t)
	invalid := appsettings.Default()
	invalid.Appearance = "not-a-real-appearance"
	err := service.Save(invalid)
	if err == nil {
		t.Fatal("want a validation error for an invalid appearance")
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok || wireErr.Code != "validation" {
		t.Fatalf("err = %v, want validation", err)
	}
}

func TestSaveRejectsUnsupportedFXProvider(t *testing.T) {
	service := newTestService(t)
	invalid := appsettings.Default()
	invalid.FXProvider = "does-not-exist"
	err := service.Save(invalid)
	if err == nil {
		t.Fatal("want a validation error for an unsupported FX provider before it ever reaches SetFXProvider")
	}
}

func TestResetRestoresDefaults(t *testing.T) {
	service := newTestService(t)
	updated := appsettings.Default()
	updated.Appearance = appsettings.AppearanceDark
	if err := service.Save(updated); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reset, err := service.Reset()
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if reset != appsettings.Default() {
		t.Fatalf("Reset() = %+v, want defaults", reset)
	}
	loaded, err := service.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != appsettings.Default() {
		t.Fatalf("Load() after Reset = %+v, want defaults", loaded)
	}
}

func TestSupportedCurrenciesMatchesSettingsPackage(t *testing.T) {
	service := newTestService(t)
	currencies := service.SupportedCurrencies()
	if len(currencies) != len(appsettings.SupportedCurrencies()) {
		t.Fatalf("SupportedCurrencies() = %v, want %v", currencies, appsettings.SupportedCurrencies())
	}
}
