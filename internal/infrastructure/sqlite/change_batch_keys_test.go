package sqlite

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestChangeBatchKeysReadOnlyCompatibilityAndWritableRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prior-v13.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	householdID := domain.NewHouseholdID()
	if _, err := db.SQL.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES(?,?,?,?,?)`, householdID.String(), "Existing household", "USD", "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`DROP TABLE change_batch_mutation_keys`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	readOnly, err := OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatalf("read-only verification of prior v13 file: %v", err)
	}
	var count int
	if err := readOnly.SQL.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='change_batch_mutation_keys'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("read-only verification created batch receipt table")
	}
	if err := readOnly.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read-only verification changed database bytes")
	}
	repaired, err := Open(path)
	if err != nil {
		t.Fatalf("writable Open of prior v13 file: %v", err)
	}
	defer repaired.Close()
	if err := repaired.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := repaired.SQL.QueryRow(`SELECT name FROM households WHERE id=?`, householdID.String()).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Existing household" {
		t.Fatalf("prior household = %q", name)
	}
	if err := repaired.SQL.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='change_batch_mutation_keys'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("writable Open did not repair batch receipt table")
	}
}
