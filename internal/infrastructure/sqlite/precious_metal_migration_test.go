package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestV10MetalMigrationPreservesLegacyConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v10.db")
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacy := string(schema)
	legacy = strings.ReplaceAll(legacy, "    metal_template TEXT NOT NULL DEFAULT '',\n", "")
	legacy = strings.ReplaceAll(legacy, "    quantity_unit TEXT NOT NULL DEFAULT '',\n", "")
	legacy = strings.ReplaceAll(legacy, "    conversion_json TEXT,\n", "")
	if idx := strings.Index(legacy, "CREATE TABLE product_operations"); idx >= 0 {
		end := strings.Index(legacy, "PRAGMA user_version")
		if end > idx {
			legacy = legacy[:idx] + "PRAGMA user_version = 10;\n"
		}
	}
	lines := strings.Split(legacy, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.Contains(line, "CREATE UNIQUE INDEX ux_instruments_active_metal_binding") {
			continue
		}
		line = strings.ReplaceAll(line, " AND metal_template = ''", "")
		line = strings.ReplaceAll(line, "PRAGMA user_version = 12;", "PRAGMA user_version = 10;")
		kept = append(kept, line)
	}
	if _, err := db.Exec(strings.Join(kept, "\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Household','CNY','2026-09-01T00:00:00Z','2026-09-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO instruments(id,household_id,name,instrument_type,quote_currency,icon_key,quote_source,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','Legacy gold','precious_metal','CNY','investment','manual','2026-09-01T00:00:00Z','2026-09-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := migrateV10ToV11(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := migrateV11ToV12(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := verifySchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var name, source, template, unit string
	if err := db.QueryRow(`SELECT name,quote_source,metal_template,quantity_unit FROM instruments`).Scan(&name, &source, &template, &unit); err != nil {
		t.Fatal(err)
	}
	if name != "Legacy gold" || source != "manual" || template != "" || unit != "" {
		t.Fatalf("legacy instrument changed: %s %s %s %s", name, source, template, unit)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
}
