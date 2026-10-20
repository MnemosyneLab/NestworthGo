package application

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func e1Rows(t *testing.T, f seriesZeroFixture, query string) []string {
	t.Helper()
	rows, err := f.db.SQL.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func e1AuthoritativeRows(t *testing.T, f seriesZeroFixture) []string {
	t.Helper()
	result := []string{}
	for _, q := range []string{
		`SELECT json_array(household_id,instrument_id,timezone_version,on_date,input_policy,quantity,native_currency,instrument_type,amount_exact,cost_exact,evidence_json,coverage_json,input_fingerprint,computed_at) FROM investment_review_daily_inputs ORDER BY input_policy,on_date`,
		`SELECT json_array(rule_id,condition_version,instrument_id,timezone_version,on_date,predicate_truth,reason,evidence_json,input_fingerprint,evaluated_at,revised_at) FROM investment_review_daily_results ORDER BY rule_id,on_date`,
		`SELECT json_array(id,rule_id,instrument_id,position_epoch_id,opened_condition_version,first_hit_on,triggered_on,first_detected_at,window_start,window_end,latest_closed_on,hit_count,false_count,unknown_count,consecutive_count,resolved_at,resolved_reason,acknowledged_at,evidence_json,evidence_fingerprint,revision) FROM investment_review_episodes ORDER BY id`,
		`SELECT json_array(episode_id,event_key,kind,payload_json,created_at) FROM investment_review_episode_events ORDER BY episode_id,event_key`,
	} {
		result = append(result, e1Rows(t, f, q)...)
	}
	return result
}

func TestReviewFinancialFactShadowCorrectionEquivalence(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	r := sqlite.NewRepository(f.db).WithFullReviewRecompute()
	f.s.repository = r.WithReviewFinancialFactShadow(128)
	importReviewDailyClose(t, f, "2026-08-01", "100")
	f.day(t, "2026-08-08")
	quoteID := importReviewDailyClose(t, f, "2026-08-08", "120")
	*f.now = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
		{Leaf: &domain.ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []domain.InstrumentType{domain.InstrumentStock}}},
		{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "10", LookbackDays: 7}},
	}}
	rule := saveSyntheticReviewRule(t, f, "Synthetic E1 first", spec)
	spec.Predicate.All[1].Leaf.Value = "15"
	saveSyntheticReviewRule(t, f, "Synthetic E1 second", spec)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var episode string
	var revision int
	if err := f.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(&episode, &revision); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episode, string(domain.NewMutationID()), revision); err != nil {
		t.Fatal(err)
	}
	originals := e1Rows(t, f, `SELECT json_array(id,semantic_key,fact_kind,source_version,payload_json,coverage_json,created_at) FROM review_financial_daily_facts ORDER BY id`)
	var facts, refs, oldInputs, bytes, oldBytes, factEncodedBytes, refEncodedBytes int
	if err := f.db.SQL.QueryRow(`SELECT count(*),sum(length(CAST(payload_json AS BLOB))+length(CAST(coverage_json AS BLOB))) FROM review_financial_daily_facts`).Scan(&facts, &bytes); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM review_result_fact_refs`).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*),sum(length(CAST(evidence_json AS BLOB))+length(CAST(coverage_json AS BLOB))) FROM investment_review_daily_inputs`).Scan(&oldInputs, &oldBytes); err != nil {
		t.Fatal(err)
	}
	if facts != oldInputs || refs != 2*oldInputs {
		t.Fatalf("threshold sharing failed facts=%d refs=%d oldInputs=%d", facts, refs, oldInputs)
	}
	if err := f.db.SQL.QueryRow(`SELECT sum(length(CAST(id||household_id||instrument_id||semantic_key||fact_kind||source_version||payload_json||coverage_json||created_at AS BLOB))) FROM review_financial_daily_facts`).Scan(&factEncodedBytes); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`SELECT sum(length(CAST(rule_id||condition_version||instrument_id||timezone_version||on_date||fact_id||input_fingerprint AS BLOB))) FROM review_result_fact_refs`).Scan(&refEncodedBytes); err != nil {
		t.Fatal(err)
	}
	t.Logf("encoded column bytes facts=%d refs=%d total=%d; excludes SQLite records/indexes/pages/WAL", factEncodedBytes, refEncodedBytes, factEncodedBytes+refEncodedBytes)
	t.Logf("initial: rule-inputs=%d shared-facts=%d (base+metrics) refs=%d payload+coverage bytes old=%d shared=%d; added inserts=%d", oldInputs, facts, refs, oldBytes, bytes, facts+refs)
	before := e1Rows(t, f, `SELECT payload_json FROM investment_review_episode_events WHERE kind='opened' ORDER BY episode_id`)
	f.s.repository = r // authoritative correction with shadow off
	correction := AgentMarketDataItem{Operation: "correct", TargetQuoteID: quoteID, Kind: "close", InstrumentID: f.instrument.ID.String(), Currency: "CNY", Date: "2026-08-08", Timezone: "UTC", Value: "130", SourceTitle: "Synthetic E1 correction"}
	if _, err := f.s.ImportAgentMarketData(t.Context(), domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{correction}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	baseline := e1AuthoritativeRows(t, f)
	after := e1Rows(t, f, `SELECT payload_json FROM investment_review_episode_events WHERE kind='opened' ORDER BY episode_id`)
	if reflect.DeepEqual(before, after) {
		t.Fatal("fixture failed to exercise deliberate opening evidence correction")
	}
	f.s.repository = r.WithReviewFinancialFactShadow(128)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := e1AuthoritativeRows(t, f); !reflect.DeepEqual(baseline, got) {
		t.Fatal("shadow changed authoritative episode/result/evidence/ack rows")
	}
	all := e1Rows(t, f, `SELECT json_array(id,semantic_key,fact_kind,source_version,payload_json,coverage_json,created_at) FROM review_financial_daily_facts ORDER BY id`)
	for _, old := range originals {
		found := false
		for _, row := range all {
			found = found || row == old
		}
		if !found {
			t.Fatal("old referenced version mutated")
		}
	}
	referenceQuery := `SELECT json_array(rule_id,condition_version,instrument_id,timezone_version,on_date,fact_id,input_fingerprint) FROM review_result_fact_refs ORDER BY rule_id,on_date,fact_id`
	references := e1Rows(t, f, referenceQuery)
	var corrected int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM review_financial_daily_facts WHERE fact_kind='metrics_v1' AND payload_json LIKE '%"periodPercent":"30"%'`).Scan(&corrected); err != nil || corrected != 1 {
		t.Fatalf("corrected metric versions=%d err=%v", corrected, err)
	}
	var oldMetric int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM review_financial_daily_facts WHERE fact_kind='metrics_v1' AND payload_json LIKE '%"periodPercent":"20"%'`).Scan(&oldMetric); err != nil || oldMetric != 1 {
		t.Fatalf("original metric versions=%d err=%v", oldMetric, err)
	}
	var ack string
	if err := f.db.SQL.QueryRow(`SELECT acknowledged_at FROM investment_review_episodes WHERE id=? AND resolved_at IS NULL`, episode).Scan(&ack); err != nil || ack == "" {
		t.Fatal("acknowledgement lost", err)
	}
	t.Logf("after correction: shared-facts=%d; old exact rows retained=%d; 20%% and 30%% metrics each have one version shared by both thresholds", len(all), len(originals))
	// A late authoritative write failure must roll back newly staged shadow rows too.
	f.day(t, "2026-08-10")
	if _, err := f.db.SQL.Exec(`CREATE TRIGGER synthetic_e1_failure BEFORE UPDATE ON review_temporal_checkpoints BEGIN SELECT RAISE(ABORT,'synthetic E1 late failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err == nil || !strings.Contains(err.Error(), "synthetic E1 late failure") {
		t.Fatalf("late fault=%v", err)
	}
	if got := e1Rows(t, f, `SELECT json_array(id,semantic_key,fact_kind,source_version,payload_json,coverage_json,created_at) FROM review_financial_daily_facts ORDER BY id`); !reflect.DeepEqual(all, got) {
		t.Fatal("late failure committed shadow versions")
	}
	if _, err := f.db.SQL.Exec(`DROP TRIGGER synthetic_e1_failure`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.s.ReconcileInvestmentReviewsV2(ctx); err == nil {
		t.Fatal("cancelled reconcile succeeded")
	}
	if got := e1Rows(t, f, `SELECT json_array(id,semantic_key,fact_kind,source_version,payload_json,coverage_json,created_at) FROM review_financial_daily_facts ORDER BY id`); !reflect.DeepEqual(all, got) {
		t.Fatal("cancelled reconcile committed facts")
	}
	// Cancel while the actual publisher holds its transaction, at its final clock fence.
	shadow := r.WithReviewFinancialFactShadow(128)
	capture, err := shadow.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := PrepareReviewPublication(t.Context(), capture, *f.now, f.s.fxProviderKey)
	if err != nil {
		t.Fatal(err)
	}
	publishCtx, cancelPublish := context.WithCancel(t.Context())
	defer cancelPublish()
	if err := shadow.PublishReviewProjectionWithClock(publishCtx, publication, func() time.Time { cancelPublish(); return *f.now }); err == nil {
		t.Fatal("publication ignored cancellation")
	}
	if got := e1AuthoritativeRows(t, f); !reflect.DeepEqual(baseline, got) {
		t.Fatal("failure/cancellation altered authoritative rows")
	}
	if got := e1Rows(t, f, `SELECT json_array(id,semantic_key,fact_kind,source_version,payload_json,coverage_json,created_at) FROM review_financial_daily_facts ORDER BY id`); !reflect.DeepEqual(all, got) {
		t.Fatal("publication cancellation committed facts")
	}
	if got := e1Rows(t, f, referenceQuery); !reflect.DeepEqual(references, got) {
		t.Fatal("failure/cancellation committed references")
	}
	snapshot := filepath.Join(t.TempDir(), "synthetic-e1-restore.db")
	if err := f.db.SnapshotTo(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	restored, err := sqlite.Open(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restoredFixture := f
	restoredFixture.db = restored
	if got := e1Rows(t, restoredFixture, `SELECT json_array(id,semantic_key,fact_kind,source_version,payload_json,coverage_json,created_at) FROM review_financial_daily_facts ORDER BY id`); !reflect.DeepEqual(all, got) {
		t.Fatal("snapshot/restore changed fact versions")
	}
	if got := e1AuthoritativeRows(t, restoredFixture); !reflect.DeepEqual(baseline, got) {
		t.Fatal("snapshot/restore changed evidence/ack")
	}

}

func TestReviewFinancialFactShadowPreservesExactFX(t *testing.T) {
	f := newSeriesZeroFixture(t, true, "UTC", "2026-08-01", "1")
	r := sqlite.NewRepository(f.db).WithFullReviewRecompute()
	f.s.repository = r
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "100", Currency: "CNY", FXMode: "convert_on_day"}}}}
	saveSyntheticReviewRule(t, f, "Synthetic exact FX", spec)
	if _, err := f.s.AppendManualFXQuote(t.Context(), "USD", "CNY", "2.123456789012", f.now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-08-03")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	baseline := e1AuthoritativeRows(t, f)
	var count int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM review_financial_daily_facts`).Scan(&count); err != nil || count != 0 {
		t.Fatal("default-off wrote facts", count, err)
	}
	f.s.repository = r.WithReviewFinancialFactShadow(1)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := e1AuthoritativeRows(t, f); !reflect.DeepEqual(baseline, got) {
		t.Fatal("FX shadow changed authoritative rows")
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM review_financial_daily_facts`).Scan(&count); err != nil || count != 2 {
		t.Fatal("one rule-day cap violated", count, err)
	}
	var payload string
	if err := f.db.SQL.QueryRow(`SELECT payload_json FROM review_financial_daily_facts WHERE fact_kind='metrics_v1'`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"ExactSelectedFXRates":{"CNY":"2.123456789012"}`) {
		t.Fatalf("lost exact private FX scalar: %s", payload)
	}
}

func TestReviewFinancialFactShadowSkipsNonexistentCivilDate(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "Pacific/Apia", "2011-12-29", "1")
	r := sqlite.NewRepository(f.db).WithFullReviewRecompute()
	f.s.repository = r
	spec := syntheticHistoricalReviewSpec()
	spec.Schedule.Timezone = "Pacific/Apia"
	saveSyntheticReviewRule(t, f, "Synthetic skipped civil day", spec)
	f.day(t, "2012-01-01")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal("baseline publication", err)
	}
	baseline := e1AuthoritativeRows(t, f)
	var truth, reason string
	var leaves int
	if err := f.db.SQL.QueryRow(`SELECT predicate_truth,reason,json_array_length(evidence_json,'$.leaves') FROM investment_review_daily_results WHERE on_date='2011-12-30'`).Scan(&truth, &reason, &leaves); err != nil {
		t.Fatal(err)
	}
	if truth != "unknown" || reason != "nonexistent_local_date" || leaves != 0 {
		t.Fatalf("invalid baseline skipped slot: truth=%s reason=%s leaves=%d", truth, reason, leaves)
	}
	var facts int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM review_financial_daily_facts`).Scan(&facts); err != nil || facts != 0 {
		t.Fatal("fixture must start with empty shadow store", facts, err)
	}
	f.s.repository = r.WithReviewFinancialFactShadow(128)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal("shadow publication", err)
	}
	if got := e1AuthoritativeRows(t, f); !reflect.DeepEqual(baseline, got) {
		t.Fatal("shadow changed authoritative skipped-date output")
	}
	var skippedRefs int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM review_result_fact_refs WHERE on_date='2011-12-30'`).Scan(&skippedRefs); err != nil || skippedRefs != 0 {
		t.Fatal("nonexistent day received shadow refs", skippedRefs, err)
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM review_financial_daily_facts`).Scan(&facts); err != nil || facts != 4 {
		t.Fatal("expected base/metric facts for only December 29 and 31", facts, err)
	}
}
