package settings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestSQLiteSettingsSurviveMissingCorruptAndStaleJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	db, err := sqlite.Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewConfigurationRepository(db)
	store := settings.NewStore(path)
	want := settings.Default()
	want.Currency = "SGD"
	want.Timezone = "Asia/Singapore"
	want.Appearance = settings.AppearanceDark
	want.TiingoAPIKey = "tiingo-private"
	want.CoinGeckoAPIKey = "gecko-private"
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	legacy, _ := os.ReadFile(path)
	if err := store.Attach(repo); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil || got != want {
		t.Fatal("migration did not preserve settings", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"currency", "timezone", "api_key", "fx_provider"} {
		if strings.Contains(string(raw), key) {
			t.Fatalf("JSON contains durable field %s", key)
		}
	}
	for _, corrupt := range []bool{false, true} {
		if corrupt {
			err = os.WriteFile(path, []byte("broken"), 0600)
		} else {
			err = os.Remove(path)
		}
		if err != nil {
			t.Fatal(err)
		}
		reopened := settings.NewStore(path)
		if err := reopened.Attach(repo); err != nil {
			t.Fatal(err)
		}
		got, err = reopened.Load()
		if err != nil {
			t.Fatal(err)
		}
		expected := want
		expected.Appearance = settings.Default().Appearance
		if got != expected {
			t.Fatal("durable settings changed after JSON loss")
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal("JSON not rebuilt", err)
		}
	}
	got.TiingoAPIKey = ""
	if err := store.Save(got); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	reopened := settings.NewStore(path)
	if err := reopened.Attach(repo); err != nil {
		t.Fatal(err)
	}
	got, err = reopened.Load()
	if err != nil || got.TiingoAPIKey != "" {
		t.Fatal("stale JSON resurrected revoked key", err)
	}
}

func TestMigrationFailureRetainsLegacySettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	store := settings.NewStore(path)
	value := settings.Default()
	value.TiingoAPIKey = "private-token"
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	db, err := sqlite.Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewConfigurationRepository(db)
	db.Close()
	if err := store.Attach(repo); err == nil {
		t.Fatal("expected closed database error")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("failed migration changed legacy JSON")
	}
}
