package application

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewProjectionCaptureContainsRulesSuppressionAndFinancialFacts(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	spec := syntheticHistoricalReviewSpec()
	rule, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic cost rule", Enabled: true, Severity: "warning", Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	suppression, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: f.instrument.ID.String(), Mode: "permanent"})
	if err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	capture, err := sqlite.NewRepository(f.db).CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.Rules) != 1 || capture.Rules[0].ID != rule.ID || len(capture.Suppressions) != 1 || capture.Suppressions[0].ID != suppression.ID || len(capture.Historical.History.Portfolio.Holdings) == 0 || capture.HistoricalFingerprint == "" || capture.Control.ConfigRevision < 2 || capture.Control.ReviewStateRevision < 1 {
		t.Fatalf("incomplete review capture: %+v", capture)
	}
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("read-only capture wrote %d rows", after-before)
	}
	f.day(t, "2026-08-02")
	plan, err := PlanReviewRuleInstrument(t.Context(), capture, rule, f.instrument.ID, *f.now, "")
	if err != nil || len(plan.Days) != 1 || plan.Days[0].On != "2026-08-01" || plan.Days[0].Result.Truth != domain.ReviewTrue || plan.LatestWindow.Truth != domain.ReviewTrue || len(plan.EpochIDs) != 1 || plan.ActiveSuppression == nil {
		t.Fatalf("daily rule projection lost evidence or permanent ignore: %+v %v", plan, err)
	}
}

func TestMigratedLegacyReviewRuleIsHiddenFromV2ReadsAndProjection(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	const configPrefix = `{"revision":4,"policy":{"enabled":true,"windowDays":30,"lowAnnualizedPercent":"1","lossPercent":"-1","sustainDays":7,"maxAgeDays":7},"enrolled":{"`
	const configSuffix = `":true},"acknowledged":{"old-run":true},"snoozedUntil":{}}`
	config := []byte(configPrefix + f.instrument.ID.String() + configSuffix)
	if _, err := domain.DecodeReviewLegacyConfiguration(config); err != nil {
		t.Fatalf("synthetic migrated settings: %v", err)
	}
	spec := []byte(`{"specVersion":1,"evaluator":"legacy_v1","configuration":` + string(config) + `}`)
	digest := sha256.Sum256(spec)
	ruleID := domain.NewReviewRuleID()
	stamp := f.now.UTC().Format(time.RFC3339Nano)
	if _, err := f.db.SQL.Exec(`INSERT INTO investment_review_rules(id,household_id,name,enabled,severity,evaluator,row_revision,current_condition_version,created_at,updated_at) VALUES(?,?,?,1,'warning','legacy_v1',1,1,?,?)`, string(ruleID), f.account.Account.HouseholdID.String(), "Legacy investment review", stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`INSERT INTO investment_review_rule_versions(rule_id,condition_version,spec_version,canonical_spec_json,semantic_hash,created_at) VALUES(?,1,1,?,?,?)`, string(ruleID), string(spec), hex.EncodeToString(digest[:]), stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`INSERT INTO investment_review_rule_targets(rule_id,instrument_id) VALUES(?,?)`, string(ruleID), f.instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	listed, err := f.s.ListReviewRulesV2(t.Context(), "", 100)
	if err != nil || len(listed) != 0 {
		t.Fatalf("migrated legacy rule leaked into V2 rule list: %+v %v", listed, err)
	}
	capture, err := sqlite.NewRepository(f.db).CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
	if err != nil || len(capture.Rules) != 0 {
		t.Fatalf("migrated legacy rule leaked into V2 projection: %+v %v", capture.Rules, err)
	}
}
