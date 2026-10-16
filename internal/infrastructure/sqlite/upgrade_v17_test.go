package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestV17UpgradePreservesNotesAndVerifiedRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-v17.db")
	db, e := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("testdata/schema_v17.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(raw)); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','USD','2026-09-01T00:00:00Z','2026-09-01T00:00:00Z')"); e != nil {
		t.Fatal(e)
	}
	for _, status := range []string{"in_progress", "completed"} {
		var done any
		if status == "completed" {
			done = "2026-09-01T01:00:00Z"
		}
		if _, e = db.Exec("INSERT INTO reconciliation_sessions(id,household_id,status,revision,started_at,completed_at,note,partial) VALUES(?,'00000000-0000-4000-8000-000000000001',?,1,'2026-09-01T00:00:00Z',?,? ,0)", status, status, done, status+" note"); e != nil {
			t.Fatal(e)
		}
	}
	db.Close()
	upgraded, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer upgraded.Close()
	for _, status := range []string{"in_progress", "completed"} {
		var opening, closing string
		if e = upgraded.SQL.QueryRow("SELECT note,closing_note FROM reconciliation_sessions WHERE id=?", status).Scan(&opening, &closing); e != nil {
			t.Fatal(e)
		}
		if status == "completed" && (opening != "" || closing != "completed note") || status != "completed" && (opening != "in_progress note" || closing != "") {
			t.Fatalf("notes: %s %s %s", status, opening, closing)
		}
	}
	if e = upgraded.Verify(context.Background()); e != nil {
		t.Fatal(e)
	}
	copies, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".nestworth-v17-recovery-*", "original-v17.sqlite"))
	if len(copies) != 1 {
		t.Fatal(copies)
	}
	if _, e = Open(copies[0]); e == nil {
		t.Fatal("recovery opened writable")
	}
	recovery, e := OpenReadOnlyForVerify(copies[0])
	if e != nil {
		t.Fatal(e)
	}
	defer recovery.Close()
	var closing string
	if e = recovery.SQL.QueryRow("SELECT note FROM reconciliation_sessions WHERE status='completed'").Scan(&closing); e != nil || closing != "completed note" {
		t.Fatalf("recovery changed %q %v", closing, e)
	}
}
