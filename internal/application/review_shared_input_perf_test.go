//go:build perf

package application

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Two tiny observations, not a benchmark matrix or statistical latency claim.
func TestReviewSharedInputsObservation(t *testing.T) {
	for _, ruleCount := range []int{2, 8} {
		t.Run(fmt.Sprint(ruleCount), func(t *testing.T) {
			f, _, _ := e2Fixture(t)
			if ruleCount > 2 {
				spec := syntheticHistoricalReviewSpec()
				// Copy the already validated spec exactly except for its threshold.
				rules, err := f.db.SQL.Query(`SELECT canonical_spec_json FROM investment_review_rule_versions LIMIT 1`)
				if err != nil {
					t.Fatal(err)
				}
				var raw string
				for rules.Next() {
					if err := rules.Scan(&raw); err != nil {
						t.Fatal(err)
					}
				}
				rules.Close()
				decoded, err := domain.DecodeReviewRuleSpec([]byte(raw))
				if err != nil {
					t.Fatal(err)
				}
				spec = decoded
				for i := 2; i < ruleCount; i++ {
					spec.Predicate.All[0].Leaf.Value = fmt.Sprint(10 + i)
					saveSyntheticReviewRule(t, f, fmt.Sprintf("E2 observation %d", i), spec)
				}
			}
			for _, shared := range []bool{false, true} {
				path := filepath.Join(t.TempDir(), "arm.db")
				if err := f.db.SnapshotTo(t.Context(), path); err != nil {
					t.Fatal(err)
				}
				db, err := sqlite.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				arm := f
				arm.db = db
				repo := sqlite.NewRepository(db).WithReviewSharedInputs(shared)
				capture, err := repo.CaptureReviewProjection(t.Context(), arm.account.Account.HouseholdID, *arm.now)
				if err != nil {
					t.Fatal(err)
				}
				publication, err := PrepareReviewPublication(t.Context(), capture, *arm.now, arm.s.fxProviderKey)
				if err != nil {
					t.Fatal(err)
				}
				start := time.Now()
				if err := repo.PublishReviewProjection(t.Context(), publication, *arm.now); err != nil {
					t.Fatal(err)
				}
				elapsed := time.Since(start)
				var inputBytes, factBytes, refBytes, inputs, facts, refs, selectors int
				if err := db.SQL.QueryRow(`SELECT count(*),COALESCE(sum(length(CAST(household_id||instrument_id||timezone_version||on_date||input_policy||COALESCE(quantity,'')||COALESCE(native_currency,'')||COALESCE(instrument_type,'')||COALESCE(amount_exact,'')||COALESCE(cost_exact,'')||evidence_json||coverage_json||input_fingerprint||computed_at AS BLOB))),0),sum(json_type(evidence_json,'$.SharedFactV1') IS NOT NULL) FROM investment_review_daily_inputs`).Scan(&inputs, &inputBytes, &selectors); err != nil {
					t.Fatal(err)
				}
				if err := db.SQL.QueryRow(`SELECT count(*),COALESCE(sum(length(CAST(id||household_id||instrument_id||semantic_key||fact_kind||source_version||payload_json||coverage_json||created_at AS BLOB))),0) FROM review_financial_daily_facts`).Scan(&facts, &factBytes); err != nil {
					t.Fatal(err)
				}
				if err := db.SQL.QueryRow(`SELECT count(*),COALESCE(sum(length(CAST(rule_id||condition_version||instrument_id||timezone_version||on_date||fact_id||input_fingerprint AS BLOB))),0) FROM review_result_fact_refs`).Scan(&refs, &refBytes); err != nil {
					t.Fatal(err)
				}
				readStart := time.Now()
				reread, err := repo.CaptureReviewProjection(t.Context(), arm.account.Account.HouseholdID, arm.now.AddDate(0, 0, 1))
				if err != nil {
					t.Fatal(err)
				}
				readElapsed := time.Since(readStart)
				if len(reread.ReusableDailyEvidence) == 0 {
					t.Fatal("window read not hit")
				}
				t.Logf("Go=%s rules=%d days=8 shared=%v inputs=%d facts=%d refs=%d selectors=%d encoded columns input=%d fact=%d ref=%d total=%d publish=%s capture+window=%s window_days=%d; fresh reopened synthetic arm; row/index/page/WAL excluded", runtime.Version(), ruleCount, shared, inputs, facts, refs, selectors, inputBytes, factBytes, refBytes, inputBytes+factBytes+refBytes, elapsed, readElapsed, len(reread.ReusableDailyEvidence))
				db.Close()
			}
		})
	}
}
