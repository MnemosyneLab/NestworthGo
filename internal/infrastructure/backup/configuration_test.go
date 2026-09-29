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

func TestV12BackupMigratesDurableSettingsAndRestoresDatabaseAuthority(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec("DROP TABLE app_configuration; PRAGMA user_version=12;"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	legacy := settings.Default()
	legacy.Currency = "SGD"
	legacy.Timezone = "Asia/Singapore"
	legacy.WorkerAPIToken = "backup-token"
	legacy.Appearance = settings.AppearanceDark
	config, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	pkg := Package{Manifest: NewManifest(time.Now(), data, config, sqlite.EntityCounts{}), Database: data, Settings: config}
	pkg.Manifest.SchemaVersion = 12
	archive := filepath.Join(dir, "old.nestworth-backup")
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
	local.WorkerAPIToken = "local-token"
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
	if got.Currency != legacy.Currency || got.Timezone != legacy.Timezone || got.WorkerAPIToken != legacy.WorkerAPIToken || got.Appearance != legacy.Appearance {
		t.Fatal("backup settings not restored")
	}
	if err := os.Remove(store.Path); err != nil {
		t.Fatal(err)
	}
	got, err = store.Load()
	if err != nil || got.WorkerAPIToken != legacy.WorkerAPIToken {
		t.Fatal("restored token depended on JSON", err)
	}
}
