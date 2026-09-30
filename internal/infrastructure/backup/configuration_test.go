package backup

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	_ "modernc.org/sqlite"
)

func TestV12BackupMigratesDurableSettingsAndRestoresDatabaseAuthority(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")
	schema, err := os.ReadFile(filepath.Join("..", "sqlite", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	legacySchema := string(schema)
	auditStart := strings.Index(legacySchema, "CREATE TABLE agent_quote_batches (")
	auditEnd := strings.Index(legacySchema, "PRAGMA user_version = 15;")
	if auditStart < 0 || auditEnd <= auditStart {
		t.Fatal("current schema is missing the Agent quote audit block")
	}
	legacySchema = legacySchema[:auditStart] + legacySchema[auditEnd:]
	legacySchema = strings.ReplaceAll(legacySchema, "CHECK(source_kind IN ('manual','provider','agent'))", "CHECK(source_kind IN ('manual','provider'))")
	legacySchema = strings.ReplaceAll(legacySchema, "CHECK(quote_source IN ('manual','provider','agent'))", "CHECK(quote_source IN ('manual','provider'))")
	legacySchema = strings.ReplaceAll(legacySchema, "CHECK(quote_source <> 'provider' OR (provider_key IS NOT NULL AND provider_symbol IS NOT NULL))", "CHECK(quote_source = 'manual' OR (provider_key IS NOT NULL AND provider_symbol IS NOT NULL))")
	legacySchema = strings.Replace(legacySchema, "PRAGMA user_version = 15;", "PRAGMA user_version = 12;", 1)
	legacySchema = strings.Replace(legacySchema, "CREATE TABLE app_configuration (key TEXT PRIMARY KEY NOT NULL, value TEXT NOT NULL);", "", 1)
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(legacySchema); err != nil {
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
	legacy.TiingoAPIKey = "backup-key"
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
	if got.Currency != legacy.Currency || got.Timezone != legacy.Timezone || got.TiingoAPIKey != legacy.TiingoAPIKey || got.Appearance != legacy.Appearance {
		t.Fatal("backup settings not restored")
	}
	if err := os.Remove(store.Path); err != nil {
		t.Fatal(err)
	}
	got, err = store.Load()
	if err != nil || got.TiingoAPIKey != legacy.TiingoAPIKey {
		t.Fatal("restored key depended on JSON", err)
	}
}
