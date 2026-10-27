package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestV22PriorityUpgradePreservesLegacyRowsAndRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-v22.db")
	seed, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = seed.SQL.Exec(`DROP TABLE investment_review_rule_priorities; PRAGMA user_version=22;
 INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Synthetic','USD','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z');
 INSERT INTO investment_review_rules(id,household_id,name,enabled,severity,evaluator,row_revision,current_condition_version,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','Synthetic priority',0,'warning','legacy_v1',1,1,'2026-10-01T00:00:00Z','2026-10-01T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	seed.Close()
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var severity, priority string
	if err = upgraded.SQL.QueryRow(`SELECT r.severity,`+reviewPrioritySQL+` FROM investment_review_rules r`).Scan(&severity, &priority); err != nil || severity != "warning" || priority != "p1" {
		t.Fatalf("legacy row: %s %s %v", severity, priority, err)
	}
	copies, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".nestworth-v22-recovery-*", "original-v22.sqlite"))
	if err != nil || len(copies) != 1 {
		t.Fatalf("recovery %v %v", copies, err)
	}
	info, err := os.Stat(copies[0])
	if err != nil || info.Mode().Perm() != 0400 {
		t.Fatalf("recovery mode: %v %v", info, err)
	}
	recovery, err := OpenReadOnlyForVerify(copies[0])
	if err != nil {
		t.Fatal(err)
	}
	var version int
	err = recovery.SQL.QueryRow(`PRAGMA user_version`).Scan(&version)
	recovery.Close()
	if err != nil || version != 22 {
		t.Fatalf("recovery changed: %d %v", version, err)
	}
	if writable, err := Open(copies[0]); err == nil {
		writable.Close()
		t.Fatal("recovery opened writable")
	}
}
