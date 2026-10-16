package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestV18UpgradePreservesLegacyReviewAndRecovery(t *testing.T) {
	for _, tc := range []struct{ name, raw, status string }{
		{"valid", `{"revision":4,"policy":{"enabled":true,"windowDays":30,"lowAnnualizedPercent":"1","lossPercent":"-1","sustainDays":7,"maxAgeDays":7},"enrolled":{"00000000-0000-4000-8000-000000000050":false},"acknowledged":{"old-run":true},"snoozedUntil":{"00000000-0000-4000-8000-000000000050":"2026-10-30"}}`, "compatible"},
		{"broken", `{broken`, "needs_repair"},
		{"null", `null`, "needs_repair"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "synthetic-v18.db")
			base, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
			if err != nil {
				t.Fatal(err)
			}
			schema, err := os.ReadFile("testdata/schema_v18.sql")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := base.Exec(string(schema)); err != nil {
				t.Fatal(err)
			}
			const household = "00000000-0000-4000-8000-000000000001"
			if _, err := base.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES(?,'Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`, household); err != nil {
				t.Fatal(err)
			}
			key := "investment_review/" + household
			if _, err := base.Exec(`INSERT INTO app_configuration(key,value) VALUES(?,?)`, key, tc.raw); err != nil {
				t.Fatal(err)
			}
			if err := base.Close(); err != nil {
				t.Fatal(err)
			}
			upgraded, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer upgraded.Close()
			if err := upgraded.Verify(context.Background()); err != nil {
				t.Fatal(err)
			}
			var raw, hash, status string
			if err := upgraded.SQL.QueryRow(`SELECT raw_json,raw_sha256,status FROM investment_review_legacy_state WHERE household_id=?`, household).Scan(&raw, &hash, &status); err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256([]byte(tc.raw))
			if raw != tc.raw || hash != hex.EncodeToString(digest[:]) || status != tc.status {
				t.Fatalf("raw/hash/status lost: %q %q %q", raw, hash, status)
			}
			var count int
			if err := upgraded.SQL.QueryRow(`SELECT count(*) FROM investment_review_rules`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("invented new rules: %d %v", count, err)
			}
			if err := upgraded.SQL.QueryRow(`SELECT count(*) FROM investment_review_suppressions`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("inferred a permanent ignore from legacy enrollment: %d %v", count, err)
			}
			if tc.status == "needs_repair" {
				var projection string
				if err := upgraded.SQL.QueryRow(`SELECT projection_status FROM investment_review_control WHERE household_id=?`, household).Scan(&projection); err != nil || projection != "needs_repair" {
					t.Fatalf("repair state: %q %v", projection, err)
				}
			}
			copies, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".nestworth-v18-recovery-*", "original-v18.sqlite"))
			if err != nil || len(copies) != 1 {
				t.Fatalf("recovery copy: %v %v", copies, err)
			}
			if _, err := Open(copies[0]); err == nil {
				t.Fatal("opened recovery copy writable")
			}
			readonly, err := OpenReadOnlyForVerify(copies[0])
			if err != nil {
				t.Fatal(err)
			}
			defer readonly.Close()
			var recovered string
			if err := readonly.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key=?`, key).Scan(&recovered); err != nil || recovered != tc.raw {
				t.Fatalf("recovery changed legacy state: %q %v", recovered, err)
			}
		})
	}
}

func TestV19VerifierDetectsReviewSchemaTampering(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "synthetic-v19.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.SQL.Exec(`DROP INDEX ux_investment_review_open_episode`); err != nil {
		t.Fatal(err)
	}
	if err := db.Verify(context.Background()); err == nil {
		t.Fatal("accepted missing review uniqueness index")
	}
}

func TestV18UpgradeQuarantinesMalformedAndOrphanReviewKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-v18-keys.db")
	base, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("testdata/schema_v18.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	// v18 constrains neither configuration key syntax nor its household link.
	const household = "00000000-0000-4000-8000-0000000000ab"
	if _, err := base.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES(?,'Synthetic','CNY','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`, household); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Exec(`INSERT INTO app_configuration(key,value) VALUES(?,?)`, "investment_review/"+household, `{}`); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"investment_review/not-a-uuid":                           `{"source":"broken-key"}`,
		"investment_review/00000000-0000-4000-8000-000000000099": `{"source":"orphan"}`,
		"investment_review/00000000-0000-4000-8000-0000000000AB": `{"source":"alias"}`,
		"investmentXreview/00000000-0000-4000-8000-000000000088": `{"source":"near-prefix"}`,
	}
	for key, value := range values {
		if _, err := base.Exec(`INSERT INTO app_configuration(key,value) VALUES(?,?)`, key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := base.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if err := upgraded.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	for key, expected := range values {
		var original string
		if err := upgraded.SQL.QueryRow(`SELECT value FROM app_configuration WHERE key=?`, key).Scan(&original); err != nil || original != expected {
			t.Fatalf("original %q changed: %q %v", key, original, err)
		}
		var raw, hash, reason string
		err := upgraded.SQL.QueryRow(`SELECT raw_json,raw_sha256,reason FROM investment_review_legacy_quarantine WHERE source_key=?`, key).Scan(&raw, &hash, &reason)
		if key[:len("investment_review/")] != "investment_review/" {
			if err != sql.ErrNoRows {
				t.Fatalf("near-prefix key was captured: %v", err)
			}
			continue
		}
		digest := sha256.Sum256([]byte(expected))
		if err != nil || raw != expected || hash != hex.EncodeToString(digest[:]) || reason == "" {
			t.Fatalf("quarantine %q: %q %q %q %v", key, raw, hash, reason, err)
		}
	}
	var count int
	if err := upgraded.SQL.QueryRow(`SELECT count(*) FROM investment_review_legacy_state WHERE source_key=?`, "investment_review/"+household).Scan(&count); err != nil || count != 1 {
		t.Fatalf("canonical source missing: %d %v", count, err)
	}
	if err := upgraded.SQL.QueryRow(`SELECT count(*) FROM investment_review_rules`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy key activated a rule: %d %v", count, err)
	}
}
