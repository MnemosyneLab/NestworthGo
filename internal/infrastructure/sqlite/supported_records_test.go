package sqlite

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestSupportedLegacyReviewRecordSurvivesUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-review21.db")
	seed := syntheticV21(t, path)
	household := domain.HouseholdID("00000000-0000-4000-8000-000000000001")
	raw := `{"revision":1,"policy":{"enabled":false,"windowDays":7,"lowAnnualizedPercent":"1.25","lossPercent":"-5","sustainDays":2,"maxAgeDays":7},"enrolled":{},"acknowledged":{}}`
	spec := `{"specVersion":1,"evaluator":"legacy_v1","configuration":` + raw + `}`
	if _, err := seed.Exec(`INSERT INTO households(id,name,base_currency,created_at,updated_at) VALUES(?,'Synthetic','USD','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`, household.String()); err != nil {
		t.Fatal(err)
	}
	rule := string(legacyReviewRuleID(household))
	if _, err := seed.Exec(`INSERT INTO investment_review_rules(id,household_id,name,enabled,severity,evaluator,row_revision,current_condition_version,created_at,updated_at) VALUES(?,?,'Legacy investment review',0,'warning','legacy_v1',1,1,'2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`, rule, household.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(`INSERT INTO investment_review_rule_versions(rule_id,condition_version,spec_version,canonical_spec_json,semantic_hash,created_at) VALUES(?,1,1,?,?,'2026-10-01T00:00:00Z')`, rule, spec, fmt.Sprintf("%x", sha256.Sum256([]byte(spec)))); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(`INSERT INTO investment_review_legacy_state VALUES(?,?,?,?,1,'compatible',NULL,'2026-10-01T00:00:00Z')`, household.String(), "investment_review/"+household.String(), raw, fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))); err != nil {
		t.Fatal(err)
	}
	seed.Close()
	for _, open := range []func(string) (*DB, error){OpenReadOnlyForVerify, Open} {
		db, err := open(path)
		if err != nil {
			t.Fatal(err)
		}
		got, found, err := NewConfigurationRepository(db).LoadConfiguration("investment_review/" + household.String())
		if err != nil || !found || string(got) != raw {
			t.Fatalf("legacy record changed: found=%t err=%v", found, err)
		}
		closures, err := NewRepository(db).ListInstrumentClosures(t.Context(), household)
		if err != nil || len(closures) != 0 {
			t.Fatalf("closure adapter: %v %v", closures, err)
		}
		db.Close()
	}
}
