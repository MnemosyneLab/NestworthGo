package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenEnsuresActivityMutationKeysOnExistingV9Database(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-v9.db")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.Exec(`DROP TABLE activity_mutation_keys`); err != nil {
		t.Fatalf("drop mutation keys: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open after drop: %v", err)
	}
	defer reopened.Close()
	var version int
	if err := reopened.SQL.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}
	var table string
	if err := reopened.SQL.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'activity_mutation_keys'`).Scan(&table); err != nil {
		t.Fatalf("mutation keys table missing after Open: %v", err)
	}
	if err := reopened.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestOpenReadOnlyForVerifyAcceptsV9DatabaseWithoutMutationKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup-v9.db")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.Exec(`DROP TABLE activity_mutation_keys`); err != nil {
		t.Fatalf("drop mutation keys: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatalf("OpenReadOnlyForVerify rejected v9 without mutation keys: %v", err)
	}
	defer verified.Close()
	var count int
	if err := verified.SQL.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'activity_mutation_keys'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("read-only verify created activity_mutation_keys")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("read-only verify changed the database bytes")
	}
}

func TestSchemaSQLIncludesActivityMutationKeys(t *testing.T) {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), "CREATE TABLE activity_mutation_keys") {
		t.Fatal("schema.sql is missing activity_mutation_keys")
	}
	if !strings.Contains(activityMutationKeysCreateSQL, "CREATE TABLE IF NOT EXISTS activity_mutation_keys") {
		t.Fatal("live Open repair SQL is missing IF NOT EXISTS activity_mutation_keys")
	}
}
