package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
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
	expected.TiingoAPIKey = "backup-key"
	expected.Appearance = settings.AppearanceDark
	config, err := json.Marshal(expected)
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
	local.TiingoAPIKey = "local-key"
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
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Currency != expected.Currency || got.Timezone != expected.Timezone || got.TiingoAPIKey != expected.TiingoAPIKey || got.Appearance != expected.Appearance {
		t.Fatal("backup settings not restored")
	}
	if err := os.Remove(store.Path); err != nil {
		t.Fatal(err)
	}
	got, err = store.Load()
	if err != nil || got.TiingoAPIKey != expected.TiingoAPIKey {
		t.Fatal("restored key depended on JSON", err)
	}
}
