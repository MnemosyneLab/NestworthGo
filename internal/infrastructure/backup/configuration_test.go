package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestCurrentBackupRestoresDurableSettingsAndDatabaseAuthority(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "expected.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := settings.Default()
	expected.Currency = "SGD"
	expected.Timezone = "Asia/Singapore"
	expected.CoinGeckoAPIKey = "backup-key"
	expected.Appearance = settings.AppearanceDark
	config, err := jsoncodec.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	sourceStore := settings.NewStore(filepath.Join(dir, "source-settings.json"))
	if err := sourceStore.Attach(sqlite.NewConfigurationRepository(db)); err != nil {
		t.Fatal(err)
	}
	if err := sourceStore.Save(expected); err != nil {
		t.Fatal(err)
	}

	private := sqlite.NewConfigurationRepository(db)
	legacy, _, err := private.LoadConfiguration("settings")
	if err != nil {
		t.Fatal(err)
	}
	var opaque map[string]any
	if err := jsoncodec.Unmarshal(legacy, &opaque); err != nil {
		t.Fatal(err)
	}
	opaque["tiingo_api_key"] = "synthetic-archived-backup-key"
	legacy, err = jsoncodec.Marshal(opaque)
	if err != nil {
		t.Fatal(err)
	}
	if err := private.SaveConfiguration("settings", legacy); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pkg := Package{Manifest: NewManifest(time.Now(), data, config, sqlite.EntityCounts{}), Database: data, Settings: config}
	archive := filepath.Join(dir, "current.nestworth-backup")
	if err := WritePackage(archive, pkg); err != nil {
		t.Fatal(err)
	}
	verified, err := ReadPackage(archive)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "restored.db")
	if err := ExtractDatabase(verified, dest); err != nil {
		t.Fatal(err)
	}
	// The local JSON must not override the restored DB, even when restoring format.
	store := settings.NewStore(filepath.Join(dir, "settings.json"))
	local := settings.Default()
	local.CoinGeckoAPIKey = "local-key"
	if err := store.Save(local); err != nil {
		t.Fatal(err)
	}
	journal := Journal{SettingsJSON: string(config), RestoreChrome: true, RestoreFormat: true, RestoreRouting: true}
	if err := finalizeVerified(dest, journal, store); err != nil {
		t.Fatal(err)
	}
	restored, err := sqlite.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := store.Attach(sqlite.NewConfigurationRepository(restored)); err != nil {
		t.Fatal(err)
	}

	persisted := sqlite.NewConfigurationRepository(restored)
	loaded, _, err := persisted.LoadConfiguration("settings")
	if err != nil {
		t.Fatal(err)
	}
	var restoredOpaque map[string]any
	if err := jsoncodec.Unmarshal(loaded, &restoredOpaque); err != nil {
		t.Fatal(err)
	}
	if restoredOpaque["tiingo_api_key"] != "synthetic-archived-backup-key" {
		t.Fatal("restore lost archived credential")
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Currency != expected.Currency || got.Timezone != expected.Timezone || got.CoinGeckoAPIKey != expected.CoinGeckoAPIKey || got.Appearance != expected.Appearance {
		t.Fatal("backup settings not restored")
	}
	if err := os.Remove(store.Path); err != nil {
		t.Fatal(err)
	}
	got, err = store.Load()
	if err != nil || got.CoinGeckoAPIKey != expected.CoinGeckoAPIKey {
		t.Fatal("restored key depended on JSON", err)
	}
}
