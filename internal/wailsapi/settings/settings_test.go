package settings_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
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

func defaultDTO() settings.SettingsDTO {
	value := appsettings.Default()
	return settings.SettingsDTO{
		SchemaVersion: value.SchemaVersion, Appearance: value.Appearance, Accent: value.Accent,
		Language: value.Language, Timezone: value.Timezone, WeekStart: value.WeekStart,
		DateFormat: value.DateFormat, TimeFormat: value.TimeFormat, Currency: value.Currency,
		DecimalSeparator: value.DecimalSeparator, GroupingSeparator: value.GroupingSeparator,
		DecimalPlaces: value.DecimalPlaces, WindowWidth: value.WindowWidth, WindowHeight: value.WindowHeight,
		FXProvider: value.FXProvider, QuoteCacheTTL: value.QuoteCacheTTL,
	}
}

func TestLoadReturnsDefaultsWhenNoFileExists(t *testing.T) {
	service := newTestService(t)
	value, err := service.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if value != defaultDTO() {
		t.Fatalf("Load() = %+v, want defaults", value)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	service := newTestService(t)
	updated := defaultDTO()
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
	invalid := defaultDTO()
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
	invalid := defaultDTO()
	invalid.FXProvider = "does-not-exist"
	err := service.Save(invalid)
	if err == nil {
		t.Fatal("want a validation error for an unsupported FX provider before it ever reaches SetFXProvider")
	}
}

func TestResetRestoresDefaults(t *testing.T) {
	service := newTestService(t)
	updated := defaultDTO()
	updated.Appearance = appsettings.AppearanceDark
	if err := service.Save(updated); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reset, err := service.Reset()
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if reset != defaultDTO() {
		t.Fatalf("Reset() = %+v, want defaults", reset)
	}
	loaded, err := service.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != defaultDTO() {
		t.Fatalf("Load() after Reset = %+v, want defaults", loaded)
	}
}

func TestSaveReturnsBusyDuringExclusive(t *testing.T) {
	store := appsettings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	app := wailstest.NewService(t)
	service := settings.NewService(store, app)
	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = app.WithExclusive(context.Background(), application.ExclusiveBackup, func(context.Context) error {
			close(started)
			<-release
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("exclusive operation did not start")
	}
	err := service.Save(defaultDTO())
	close(release)
	if err == nil {
		t.Fatal("expected settings save to be busy during backup")
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok || wireErr.Code != "backup_restore_busy" {
		t.Fatalf("err = %v, want backup_restore_busy", err)
	}
}

func TestSettingsDTOUsesCamelCaseWireKeys(t *testing.T) {
	payload, err := json.Marshal(defaultDTO())
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, key := range []string{"schemaVersion", "weekStart", "dateFormat", "fxProvider", "quoteCacheTTL"} {
		if !strings.Contains(encoded, `"`+key+`"`) {
			t.Fatalf("payload = %s, missing camelCase key %q", encoded, key)
		}
	}
	for _, key := range []string{"schema_version", "week_start", "date_format", "fx_provider", "quote_cache_ttl"} {
		if strings.Contains(encoded, `"`+key+`"`) {
			t.Fatalf("payload = %s, contains persisted snake_case key %q", encoded, key)
		}
	}
}
