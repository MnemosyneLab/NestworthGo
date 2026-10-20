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

// Compare complete logical rows while preserving every original key/scalar and
// timestamp. This query is only the independent test oracle, not a runtime view.
func e2InputRows(t *testing.T, f seriesZeroFixture) []string {
	return e1Rows(t, f, `SELECT json_array(d.household_id,d.instrument_id,d.timezone_version,d.on_date,d.input_policy,d.quantity,d.native_currency,d.instrument_type,d.amount_exact,d.cost_exact,CASE WHEN json_type(d.evidence_json,'$.SharedFactV1') IS NOT NULL THEN (SELECT payload_json FROM review_financial_daily_facts WHERE id=json_extract(d.evidence_json,'$.SharedFactV1')) ELSE d.evidence_json END,d.coverage_json,d.input_fingerprint) FROM investment_review_daily_inputs d ORDER BY d.input_policy,d.on_date`)
}

func e2Fixture(t *testing.T) (seriesZeroFixture, *sqlite.Repository, string) {
	t.Helper()
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	r := sqlite.NewRepository(f.db).WithDefaultReviewOptimizations()
	f.s.repository = r.WithFullReviewRecompute()
	f.s.fxProviderKey = "synthetic-pinned"
	importReviewDailyClose(t, f, "2026-08-01", "100")
	f.day(t, "2026-08-08")
	quote := importReviewDailyClose(t, f, "2026-08-08", "120")
	*f.now = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "10", LookbackDays: 7}}}}
	saveSyntheticReviewRule(t, f, "E2 first threshold", spec)
	spec.Predicate.All[0].Leaf.Value = "15"
	saveSyntheticReviewRule(t, f, "E2 second threshold", spec)
	return f, r, quote
}

func e2Publish(t *testing.T, f seriesZeroFixture, r *sqlite.Repository) {
	t.Helper()
	capture, err := r.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	// Independent complete preparation; do not import a suffix or window oracle.
	capture.Suffix = nil
	capture.ReusableDailyEvidence = nil
	capture.TemporalFullRecompute = true
	p, err := PrepareReviewPublication(t.Context(), capture, *f.now, f.s.fxProviderKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.PublishReviewProjection(t.Context(), p, *f.now); err != nil {
		t.Fatal(err)
	}
}

func e2Count(t *testing.T, f seriesZeroFixture, q string) int {
	t.Helper()
	var n int
	if err := f.db.SQL.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestReviewSharedInputsCorrectionRollbackAndRestart(t *testing.T) {
	f, r, quote := e2Fixture(t)
	e2Publish(t, f, r.WithFullReviewRecompute())
	baseline := e2InputRows(t, f)
	enabled := r.WithReviewSharedInputs(true)
	e2Publish(t, f, enabled)
	if got := e2InputRows(t, f); !reflect.DeepEqual(got, baseline) {
		t.Fatal("shared inputs differ from complete inline oracle")
	}
	if got := e2Count(t, f, `SELECT count(*) FROM investment_review_daily_inputs WHERE json_type(evidence_json,'$.SharedFactV1') IS NOT NULL`); got != 16 {
		t.Fatalf("normalized days=%d", got)
	}
	if got := e2Count(t, f, `SELECT count(*) FROM review_financial_daily_facts`); got != 8 {
		t.Fatalf("threshold versions=%d", got)
	}
	old := e1Rows(t, f, `SELECT json_array(id,semantic_key,source_version,payload_json,coverage_json) FROM review_financial_daily_facts ORDER BY id`)
	var episode string
	var revision int
	if err := f.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes WHERE resolved_at IS NULL ORDER BY id LIMIT 1`).Scan(&episode, &revision); err != nil {
		t.Fatal(err)
	}
	f.s.repository = enabled
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episode, string(domain.NewMutationID()), revision); err != nil {
		t.Fatal(err)
	}
	before := e1Rows(t, f, `SELECT payload_json FROM investment_review_episode_events WHERE kind='opened' ORDER BY episode_id`)
	correction := AgentMarketDataItem{Operation: "correct", TargetQuoteID: quote, Kind: "close", InstrumentID: f.instrument.ID.String(), Currency: "CNY", Date: "2026-08-08", Timezone: "UTC", Value: "130", SourceTitle: "Synthetic E2 correction"}
	if _, err := f.s.ImportAgentMarketData(t.Context(), domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{correction}}); err != nil {
		t.Fatal(err)
	}
	e2Publish(t, f, enabled)
	after := e1Rows(t, f, `SELECT payload_json FROM investment_review_episode_events WHERE kind='opened' ORDER BY episode_id`)
	if reflect.DeepEqual(before, after) {
		t.Fatal("opening correction not exercised")
	}
	var ack string
	if err := f.db.SQL.QueryRow(`SELECT acknowledged_at FROM investment_review_episodes WHERE id=? AND resolved_at IS NULL`, episode).Scan(&ack); err != nil || ack == "" {
		t.Fatal("episode identity/ack lost", err)
	}
	all := e1Rows(t, f, `SELECT json_array(id,semantic_key,source_version,payload_json,coverage_json) FROM review_financial_daily_facts ORDER BY id`)
	for _, row := range old {
		found := false
		for _, v := range all {
			found = found || row == v
		}
		if !found {
			t.Fatal("shared version overwritten")
		}
	}
	corrected := e2InputRows(t, f)
	authoritative := e1AuthoritativeRows(t, f)
	snapshot := filepath.Join(t.TempDir(), "synthetic-e2.db")
	if err := f.db.SnapshotTo(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reopened := f
	reopened.db = db
	reopenedRepo := sqlite.NewRepository(db).WithDefaultReviewOptimizations()
	capture, err := reopenedRepo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if capture.Certified != nil && capture.Certified.MayCertify {
		t.Fatal("restart acquired reuse authority")
	}
	if !reflect.DeepEqual(e2InputRows(t, reopened), corrected) {
		t.Fatal("reopen changed input bytes")
	}
	// Rollback materializes inline bytes from full sources, retaining every fact.
	e2Publish(t, f, enabled.WithFullReviewRecompute())
	if !reflect.DeepEqual(e2InputRows(t, f), corrected) {
		t.Fatal("rollback changed complete input")
	}
	if got := e2Count(t, f, `SELECT count(*) FROM investment_review_daily_inputs WHERE json_type(evidence_json,'$.SharedFactV1') IS NOT NULL`); got != 0 {
		t.Fatal("rollback left selectors")
	}
	// Only input representation changes; compare lifecycle and result rows exactly.
	rolled := e1AuthoritativeRows(t, f)
	if !reflect.DeepEqual(authoritative[16:], rolled[16:]) {
		t.Fatal("rollback changed results/lifecycle/ack")
	}
	if !reflect.DeepEqual(e1Rows(t, f, `SELECT json_array(id,semantic_key,source_version,payload_json,coverage_json) FROM review_financial_daily_facts ORDER BY id`), all) {
		t.Fatal("rollback altered versions")
	}
}

func TestReviewSharedInputsAtomicFailureAndReadback(t *testing.T) {
	f, r, _ := e2Fixture(t)
	enabled := r.WithReviewSharedInputs(true)
	e2Publish(t, f, enabled)
	// C2 window reader must resolve references with B disabled.
	reader := sqlite.NewRepository(f.db)
	next := f.now.AddDate(0, 0, 1)
	capture, err := reader.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.ReusableDailyEvidence) == 0 {
		t.Fatal("mixed-format reader was not hit")
	}
	for _, v := range capture.ReusableDailyEvidence {
		if v.Input.InstrumentID != f.instrument.ID {
			t.Fatal("reference readback identity")
		}
	}
	old := e1AuthoritativeRows(t, f)
	versions := e1Rows(t, f, `SELECT json_array(id,payload_json) FROM review_financial_daily_facts ORDER BY id`)
	refs := e1Rows(t, f, `SELECT json_array(rule_id,on_date,fact_id,input_fingerprint) FROM review_result_fact_refs ORDER BY rule_id,on_date,fact_id`)
	*f.now = next
	if _, err := f.db.SQL.Exec(`CREATE TRIGGER synthetic_e2_failure BEFORE UPDATE ON review_temporal_checkpoints BEGIN SELECT RAISE(ABORT,'synthetic E2 late failure'); END`); err != nil {
		t.Fatal(err)
	}
	c, err := enabled.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	c.Suffix = nil
	c.ReusableDailyEvidence = nil
	c.TemporalFullRecompute = true
	p, err := PrepareReviewPublication(t.Context(), c, *f.now, f.s.fxProviderKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := enabled.PublishReviewProjection(t.Context(), p, *f.now); err == nil || !strings.Contains(err.Error(), "synthetic E2 late failure") {
		t.Fatalf("late fault=%v", err)
	}
	if _, err := f.db.SQL.Exec(`DROP TRIGGER synthetic_e2_failure`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := enabled.PublishReviewProjectionWithClock(ctx, p, func() time.Time { cancel(); return *f.now }); err == nil {
		t.Fatal("cancelled publication succeeded")
	}
	if !reflect.DeepEqual(old, e1AuthoritativeRows(t, f)) || !reflect.DeepEqual(versions, e1Rows(t, f, `SELECT json_array(id,payload_json) FROM review_financial_daily_facts ORDER BY id`)) || !reflect.DeepEqual(refs, e1Rows(t, f, `SELECT json_array(rule_id,on_date,fact_id,input_fingerprint) FROM review_result_fact_refs ORDER BY rule_id,on_date,fact_id`)) {
		t.Fatal("failed publication exposed partial state")
	}
}

func TestReviewSharedInputsCertifiedSuffixAndDamageFallback(t *testing.T) {
	f, r, _ := e2Fixture(t)
	enabled := r.WithReviewSharedInputs(true)
	f.s.repository = enabled
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n := e2Count(t, f, `SELECT count(*) FROM investment_review_daily_inputs WHERE json_type(evidence_json,'$.SharedFactV1') IS NOT NULL`); n != 16 {
		t.Fatal("production writer was not hit", n)
	}
	enabled.EvictReviewPrefixMemo()
	*f.now = f.now.AddDate(0, 0, 1)
	capture, err := enabled.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if capture.Suffix == nil || len(capture.Suffix.Prefix) == 0 {
		t.Fatal("certified mixed-format suffix was not hit")
	}
	for _, day := range capture.Suffix.Prefix {
		if !day.OutputAttested {
			t.Fatal("complete SQL image not attested")
		}
	}
	// Same-handle external payload damage must invalidate the complete row seal.
	if _, err := f.db.SQL.Exec(`UPDATE review_financial_daily_facts SET payload_json=payload_json||' '`); err != nil {
		t.Fatal(err)
	}
	damaged, err := enabled.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if damaged.Suffix != nil || damaged.Certified != nil && damaged.Certified.MayCertify {
		t.Fatal("damaged shared version retained authority")
	}
	f.s.repository = r.WithFullReviewRecompute()
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal("full fallback after damage", err)
	}
	if n := e2Count(t, f, `SELECT count(*) FROM investment_review_daily_inputs WHERE json_type(evidence_json,'$.SharedFactV1') IS NOT NULL`); n != 0 {
		t.Fatal("full fallback retained normalized selectors")
	}
}

func TestReviewSharedInputsSemanticIsolationFXAndSkippedDate(t *testing.T) {
	t.Run("requirements", func(t *testing.T) {
		f, r, _ := e2Fixture(t)
		spec := syntheticHistoricalReviewSpec()
		spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "10", LookbackDays: 30}}}}
		rule := saveSyntheticReviewRule(t, f, "E2 different lookback", spec)
		e2Publish(t, f, r.WithReviewSharedInputs(true))
		var count int
		if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_daily_inputs WHERE input_policy=? AND json_type(evidence_json,'$.SharedFactV1') IS NOT NULL`, "rule:"+string(rule.ID)+"/v:1").Scan(&count); err != nil || count != 0 {
			t.Fatal("different lookback shared a fact", count, err)
		}
		if n := e2Count(t, f, `SELECT count(*) FROM investment_review_daily_inputs WHERE json_type(evidence_json,'$.SharedFactV1') IS NOT NULL`); n != 16 {
			t.Fatal("compatible thresholds not shared", n)
		}
	})
	t.Run("exact_fx", func(t *testing.T) {
		f := newSeriesZeroFixture(t, true, "UTC", "2026-08-01", "1")
		r := sqlite.NewRepository(f.db).WithReviewSharedInputs(true)
		f.s.repository = r
		spec := syntheticHistoricalReviewSpec()
		spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "100", Currency: "CNY", FXMode: "convert_on_day"}}}}
		saveSyntheticReviewRule(t, f, "E2 FX one", spec)
		spec.Predicate.All[0].Leaf.Value = "200"
		saveSyntheticReviewRule(t, f, "E2 FX two", spec)
		if _, err := f.s.AppendManualFXQuote(t.Context(), "USD", "CNY", "2.123456789012", f.now.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
		f.day(t, "2026-08-03")
		e2Publish(t, f, r)
		var payload string
		if err := f.db.SQL.QueryRow(`SELECT payload_json FROM review_financial_daily_facts WHERE payload_json LIKE '%ExactSelectedFXRates%' LIMIT 1`).Scan(&payload); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(payload, `"ExactSelectedFXRates":{"CNY":"2.123456789012"}`) {
			t.Fatal("private exact FX scalar lost")
		}
		c, err := r.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, f.now.AddDate(0, 0, 1))
		if err != nil {
			t.Fatal(err)
		}
		hit := false
		for _, v := range c.ReusableDailyEvidence {
			if fx, ok := v.Input.SelectedFX["CNY"]; ok {
				hit = true
				if fx.Quote.Rate.String() != "2.123456789012" {
					t.Fatal("resolved exact FX changed")
				}
			}
		}
		if !hit {
			t.Fatal("FX resolver was not hit")
		}
	})
	t.Run("nonexistent_date", func(t *testing.T) {
		f := newSeriesZeroFixture(t, false, "Pacific/Apia", "2011-12-29", "1")
		r := sqlite.NewRepository(f.db).WithReviewSharedInputs(true)
		f.s.repository = r
		spec := syntheticHistoricalReviewSpec()
		spec.Schedule.Timezone = "Pacific/Apia"
		saveSyntheticReviewRule(t, f, "E2 Apia one", spec)
		saveSyntheticReviewRule(t, f, "E2 Apia two", spec)
		f.day(t, "2012-01-01")
		e2Publish(t, f, r)
		if n := e2Count(t, f, `SELECT count(*) FROM investment_review_daily_results WHERE on_date='2011-12-30' AND predicate_truth='unknown' AND reason='nonexistent_local_date' AND json_array_length(evidence_json,'$.leaves')=0`); n != 2 {
			t.Fatal("skipped date evidence lost", n)
		}
		if n := e2Count(t, f, `SELECT count(*) FROM review_result_fact_refs WHERE on_date='2011-12-30'`); n != 0 {
			t.Fatal("skipped date normalized")
		}
		if n := e2Count(t, f, `SELECT count(*) FROM investment_review_daily_inputs WHERE json_type(evidence_json,'$.SharedFactV1') IS NOT NULL`); n != 4 {
			t.Fatal("valid Apia dates not normalized", n)
		}
	})
}

func TestReviewSharedInputsAdmissionAndMissingReference(t *testing.T) {
	t.Run("budget", func(t *testing.T) {
		f, r, _ := e2Fixture(t)
		f.day(t, "2026-09-10")
		e2Publish(t, f, r.WithReviewSharedInputs(true))
		if n := e2Count(t, f, `SELECT count(*) FROM review_financial_daily_facts`); n != 0 {
			t.Fatal("oversized publication partially admitted")
		}
		if n := e2Count(t, f, `SELECT count(*) FROM investment_review_daily_inputs`); n != 80 {
			t.Fatal("fallback dropped evidence", n)
		}
	})
	t.Run("missing_ref", func(t *testing.T) {
		f, r, _ := e2Fixture(t)
		e2Publish(t, f, r.WithReviewSharedInputs(true))
		if _, err := f.db.SQL.Exec(`DELETE FROM review_result_fact_refs`); err != nil {
			t.Fatal(err)
		}
		reader := sqlite.NewRepository(f.db)
		if _, err := reader.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, f.now.AddDate(0, 0, 1)); err == nil {
			t.Fatal("missing authoritative ref silently accepted")
		}
		// A caller can always force source reconstruction without reading damaged refs.
		e2Publish(t, f, r.WithFullReviewRecompute())
		if n := e2Count(t, f, `SELECT count(*) FROM investment_review_daily_inputs WHERE json_type(evidence_json,'$.SharedFactV1') IS NOT NULL`); n != 0 {
			t.Fatal("fallback did not restore inline authority")
		}
	})
}

func TestReviewSharedInputsC2SameSourceConversion(t *testing.T) {
	f, _, _ := e2Fixture(t)
	inline := sqlite.NewRepository(f.db)
	e2Publish(t, f, inline)
	before := e2InputRows(t, f)
	enabled := inline.WithReviewSharedInputs(true)
	e2Publish(t, f, enabled)
	if n := e2Count(t, f, `SELECT count(*) FROM investment_review_daily_inputs WHERE json_type(evidence_json,'$.SharedFactV1') IS NOT NULL`); n != 16 {
		t.Fatal("same-source C2 cutover not published", n)
	}
	var timestamps []string
	timestamps = e1Rows(t, f, `SELECT computed_at FROM investment_review_daily_inputs ORDER BY input_policy,on_date`)
	*f.now = f.now.Add(time.Minute)
	e2Publish(t, f, enabled)
	if !reflect.DeepEqual(timestamps, e1Rows(t, f, `SELECT computed_at FROM investment_review_daily_inputs ORDER BY input_policy,on_date`)) {
		t.Fatal("unchanged selectors rewrote timestamps")
	}
	e2Publish(t, f, enabled.WithFullReviewRecompute())
	if !reflect.DeepEqual(before, e2InputRows(t, f)) {
		t.Fatal("same-source C2 rollback changed logical input")
	}
	if n := e2Count(t, f, `SELECT count(*) FROM investment_review_daily_inputs WHERE json_type(evidence_json,'$.SharedFactV1') IS NOT NULL`); n != 0 {
		t.Fatal("same-source C2 rollback retained selectors", n)
	}
}
