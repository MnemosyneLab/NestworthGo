package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// TestReviewFullPipelinePerformanceEvidence is an opt-in end-to-end budget
// measurement. It uses a temporary SQLite database populated only with
// synthetic accounts, positions, rules, quote history, FX references, and
// activity rows. Set NW_REVIEW_PERF_EVIDENCE=1 to run it.
func TestReviewFullPipelinePerformanceEvidence(t *testing.T) {
	if os.Getenv("NW_REVIEW_PERF_EVIDENCE") != "1" {
		t.Skip("set NW_REVIEW_PERF_EVIDENCE=1 to run full-scale SQLite evidence")
	}
	mode := os.Getenv("NW_REVIEW_PERF_MODE")
	if err := validateReviewPerfMode(mode); err != nil {
		t.Fatal(err)
	}
	historyDays := 365
	if mode == "warm" || mode == "stages" || mode == "warm-marketdata" || mode == "warm-correction" || mode == "stages-marketdata" || mode == "stages-correction" || mode == "profile" {
		historyDays = 30
	}
	fixture := newReviewFullPipelineFixture(t, historyDays)
	ctx := context.Background()

	started := time.Now()
	result, err := fixture.service.ReconcileInvestmentReviewsV2(ctx)
	cold := time.Since(started)
	if err != nil {
		t.Fatalf("cold reconcile: %v", err)
	}
	if result.RuleInstrumentCount != 2_000 {
		t.Fatalf("cold publication covered %d rule/instrument pairs, want 2000", result.RuleInstrumentCount)
	}
	t.Logf("fixture cold publish complete: historyDays=%d elapsed=%s", historyDays, cold)
	if mode == "cold" {
		logReviewFullPipelineSourceCounts(t, fixture, historyDays)
		logReviewFullPipelineStorage(t, fixture, historyDays)
		return
	}
	metrics := enableReviewSQLMetrics(t, fixture)
	if mode == "profile" || mode == "profile365" {
		profileReviewReusableEvidence(t, fixture, metrics)
		return
	}
	if mode == "stages" || mode == "stages365" {
		*fixture.now = fixture.now.AddDate(0, 0, 1)
		repository := sqlite.NewRepository(fixture.db)
		metrics.reset()
		started = time.Now()
		capture, err := repository.CaptureReviewProjection(ctx, fixture.householdID, *fixture.now)
		captureElapsed := time.Since(started)
		captureSQL := metrics.snapshot(8)
		if err != nil {
			t.Fatal(err)
		}
		started = time.Now()
		capturedFingerprint, err := domain.ReviewHistoricalFingerprint(capture.Historical)
		fingerprintElapsed := time.Since(started)
		if err != nil || capturedFingerprint != capture.HistoricalFingerprint {
			t.Fatalf("fingerprint recheck in memory differs: %v", err)
		}
		started = time.Now()
		publication, err := PrepareReviewPublication(ctx, capture, *fixture.now, fixture.service.FXProviderKey())
		prepareElapsed := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		plannedDays := 0
		for _, item := range publication.Rules {
			plannedDays += len(item.Days)
		}
		started = time.Now()
		var inputBytes, resultBytes int64
		for _, item := range publication.Rules {
			for _, day := range item.Days {
				inputJSON, err := json.Marshal(day.Input)
				if err != nil {
					t.Fatal(err)
				}
				coverageJSON, err := json.Marshal(day.CoverageMissing)
				if err != nil {
					t.Fatal(err)
				}
				resultJSON, err := json.Marshal(day.Result)
				if err != nil {
					t.Fatal(err)
				}
				inputBytes += int64(len(inputJSON) + len(coverageJSON))
				resultBytes += int64(len(resultJSON))
			}
		}
		serializationElapsed := time.Since(started)
		metrics.reset()
		started = time.Now()
		recheck, err := repository.CaptureReviewProjection(ctx, fixture.householdID, *fixture.now)
		recheckElapsed := time.Since(started)
		if err != nil || recheck.HistoricalFingerprint != capture.HistoricalFingerprint {
			t.Fatalf("read-only publication recheck differs: %v", err)
		}
		recheckSQL := metrics.snapshot(8)
		metrics.reset()
		started = time.Now()
		if err := repository.PublishReviewProjection(ctx, publication, *fixture.now); err != nil {
			t.Fatalf("publish staged warm increment: %v", err)
		}
		publishElapsed := time.Since(started)
		publishSQL := metrics.snapshot(8)
		stagedBytes := inputBytes + resultBytes
		t.Logf("one warm increment phases: capture+source fingerprint=%s (%s; top SQL: %s; positionActivitiesCaptured=%d); in-memory source fingerprint=%s; planning+daily fingerprints=%s; staged JSON serialization=%s (input+coverage=%d bytes result=%d bytes total=%d); separate read-only recheck=%s (%s; top SQL: %s); atomic publish recheck+serialization+writes=%s (%s; top SQL: %s); staged ruleInstrumentPairs=%d newDays=%d", captureElapsed, captureSQL.summary(), captureSQL.slowest(), len(capture.Historical.History.Activities), fingerprintElapsed, prepareElapsed, serializationElapsed, inputBytes, resultBytes, stagedBytes, recheckElapsed, recheckSQL.summary(), recheckSQL.slowest(), publishElapsed, publishSQL.summary(), publishSQL.slowest(), len(publication.Rules), plannedDays)
		logReviewFullPipelineStorage(t, fixture, historyDays)
		return
	}
	if mode == "stages-marketdata" || mode == "stages-correction" {
		*fixture.now = fixture.now.AddDate(0, 0, 1)
		if mode == "stages-marketdata" {
			if err := appendReviewPerfDailyMarketData(ctx, fixture, fixture.now.AddDate(0, 0, -1)); err != nil {
				t.Fatalf("append synthetic daily quote and FX refresh: %v", err)
			}
		} else if err := correctReviewPerfHistoricalQuote(ctx, fixture, 0); err != nil {
			t.Fatalf("correct synthetic historical quote: %v", err)
		}
		repository := sqlite.NewRepository(fixture.db)
		metrics.reset()
		started = time.Now()
		capture, err := repository.CaptureReviewProjection(ctx, fixture.householdID, *fixture.now)
		captureElapsed := time.Since(started)
		captureSQL := metrics.snapshot(8)
		if err != nil {
			t.Fatal(err)
		}
		if capture.HistoricalFingerprint == capture.Control.PublishedFingerprint {
			t.Fatalf("source change did not invalidate the published fingerprint: %s", mode)
		}
		if len(capture.ReusableDailyEvidence) != 0 {
			t.Fatalf("source change unexpectedly reused %d daily evidence rows: %s", len(capture.ReusableDailyEvidence), mode)
		}
		started = time.Now()
		publication, err := PrepareReviewPublication(ctx, capture, *fixture.now, fixture.service.FXProviderKey())
		prepareElapsed := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		metrics.reset()
		started = time.Now()
		if err := repository.PublishReviewProjection(ctx, publication, *fixture.now); err != nil {
			t.Fatal(err)
		}
		publishElapsed := time.Since(started)
		publishSQL := metrics.snapshot(8)
		var quoteBytes, resultBytes int64
		for _, item := range publication.Rules {
			for _, day := range item.Days {
				inputJSON, _ := json.Marshal(day.Input)
				coverageJSON, _ := json.Marshal(day.CoverageMissing)
				resultJSON, _ := json.Marshal(day.Result)
				quoteBytes += int64(len(inputJSON) + len(coverageJSON))
				resultBytes += int64(len(resultJSON))
			}
		}
		scenario := "historical quote correction (one existing instrument/day fact updated)"
		if mode == "stages-marketdata" {
			scenario = "daily market refresh (100 new instrument quotes + 4 FX references)"
		}
		t.Logf("one warm %s: capture+source fingerprint=%s (%s; top SQL: %s); planning+daily fingerprints=%s; staged JSON bytes input+coverage=%d result=%d total=%d; atomic publish recheck+serialization+writes=%s (%s; top SQL: %s); ruleInstrumentPairs=%d newDays=%d", scenario, captureElapsed, captureSQL.summary(), captureSQL.slowest(), prepareElapsed, quoteBytes, resultBytes, quoteBytes+resultBytes, publishElapsed, publishSQL.summary(), publishSQL.slowest(), len(publication.Rules), countReviewPublicationDays(publication))
		logReviewFullPipelineStorage(t, fixture, historyDays)
		return
	}

	incremental := make([]time.Duration, 30)
	for index := range incremental {
		*fixture.now = fixture.now.AddDate(0, 0, 1)
		if mode == "warm-marketdata" {
			if err := appendReviewPerfDailyMarketData(ctx, fixture, fixture.now.AddDate(0, 0, -1)); err != nil {
				t.Fatalf("append synthetic daily quote and FX refresh %d: %v", index+1, err)
			}
		} else if mode == "warm-correction" {
			if err := correctReviewPerfHistoricalQuote(ctx, fixture, index); err != nil {
				t.Fatalf("correct synthetic historical quote %d: %v", index+1, err)
			}
		}
		if index == 0 {
			metrics.reset()
		}
		started = time.Now()
		result, err = fixture.service.ReconcileInvestmentReviewsV2(ctx)
		incremental[index] = time.Since(started)
		if err != nil {
			t.Fatalf("incremental reconcile %d: %v", index+1, err)
		}
		if result.RuleInstrumentCount != 2_000 {
			t.Fatalf("incremental publication %d covered %d rule/instrument pairs, want 2000", index+1, result.RuleInstrumentCount)
		}
		if index == 0 {
			sqlSnapshot := metrics.snapshot(8)
			t.Logf("first warm full reconcile SQL: %s; top SQL: %s", sqlSnapshot.summary(), sqlSnapshot.slowest())
		}
	}

	pageReads := make([]time.Duration, 30)
	for index := range pageReads {
		if index == 0 {
			metrics.reset()
		}
		started = time.Now()
		page, err := fixture.service.ListReviewAlertGroupsV2(ctx, domain.ReviewAlertFilter{Status: "pending"}, "", 50)
		pageReads[index] = time.Since(started)
		if err != nil {
			t.Fatalf("first page read %d: %v", index+1, err)
		}
		if page.ProjectionStatus != "complete" || len(page.Items) == 0 {
			t.Fatalf("first page read returned status=%q items=%d", page.ProjectionStatus, len(page.Items))
		}
		if index == 0 {
			sqlSnapshot := metrics.snapshot(8)
			t.Logf("first alert page SQL: %s; top SQL: %s", sqlSnapshot.summary(), sqlSnapshot.slowest())
		}
	}

	quoteCount, activityCount, fxCount := 0, 0, 0
	for _, query := range []struct {
		sql  string
		into *int
	}{{`SELECT count(*) FROM instrument_quotes`, &quoteCount}, {`SELECT count(*) FROM activities`, &activityCount}, {`SELECT count(*) FROM fx_quotes`, &fxCount}} {
		if err := fixture.db.SQL.QueryRowContext(ctx, query.sql).Scan(query.into); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("fixture: holdings=100 rules=20 ruleInstrumentPairs=2000 instrumentQuotes=%d activities=%d fxReferences=%d historyDays=%d", quoteCount, activityCount, fxCount, historyDays)
	t.Logf("cold full reconcile n=1: %s (budget <=3s)", cold)
	if mode == "warm-marketdata" {
		t.Logf("warm incremental full reconcile with daily quote+FX additions n=%d: median=%s p95=%s max=%s (market data adds 100 quote + 4 FX rows per sample)", len(incremental), durationPercentile(incremental, 0.50), durationPercentile(incremental, 0.95), durationPercentile(incremental, 1))
	} else if mode == "warm-correction" {
		t.Logf("warm incremental full reconcile with one historical quote correction per sample n=%d: median=%s p95=%s max=%s", len(incremental), durationPercentile(incremental, 0.50), durationPercentile(incremental, 0.95), durationPercentile(incremental, 1))
	} else {
		t.Logf("warm incremental full reconcile n=%d: median=%s p95=%s max=%s (budget p95 <=500ms)", len(incremental), durationPercentile(incremental, 0.50), durationPercentile(incremental, 0.95), durationPercentile(incremental, 1))
	}
	t.Logf("published alert first page n=%d: median=%s p95=%s max=%s (budget p95 <=150ms)", len(pageReads), durationPercentile(pageReads, 0.50), durationPercentile(pageReads, 0.95), durationPercentile(pageReads, 1))
	logReviewFullPipelineStorage(t, fixture, historyDays)
}

type reusableEvidenceProfileRow struct {
	instrumentID, on, inputFingerprint, inputJSON, coverageJSON string
	resultJSON, resultFingerprint                               sql.NullString
	consecutiveCount                                            int
}

type reusableEvidenceProfile struct {
	query, next, scan, jsonDecode, queryToClose time.Duration
	rows                                        []reusableEvidenceProfileRow
}

const currentReusableEvidenceSQL = `WITH ranked AS (
	SELECT d.instrument_id,d.on_date,d.input_fingerprint,d.evidence_json,d.coverage_json,
	ROW_NUMBER() OVER (PARTITION BY d.instrument_id ORDER BY d.on_date DESC) AS slot_rank
	FROM investment_review_daily_inputs d
	WHERE d.household_id=? AND d.input_policy=? AND d.timezone_version=?
), latest_date AS (
	SELECT d.instrument_id,MAX(d.on_date) AS latest_on
	FROM investment_review_daily_inputs d
	WHERE d.household_id=? AND d.input_policy=? AND d.timezone_version=?
	GROUP BY d.instrument_id
), latest_streak AS (
	SELECT s.instrument_id,s.latest_closed_on,MAX(s.consecutive_count) AS consecutive_count
	FROM investment_review_evaluation_state s
	WHERE s.rule_id=?
	GROUP BY s.instrument_id,s.latest_closed_on
)
SELECT d.instrument_id,d.on_date,d.input_fingerprint,d.evidence_json,d.coverage_json,r.evidence_json,r.input_fingerprint,COALESCE(s.consecutive_count,0)
FROM ranked d LEFT JOIN investment_review_daily_results r ON r.rule_id=? AND r.condition_version=? AND r.instrument_id=d.instrument_id AND r.timezone_version=? AND r.on_date=d.on_date
LEFT JOIN latest_date ld ON ld.instrument_id=d.instrument_id
LEFT JOIN latest_streak s ON s.instrument_id=d.instrument_id AND s.latest_closed_on=ld.latest_on
WHERE d.slot_rank<=? ORDER BY d.instrument_id,d.on_date`

const narrowKeyReusableEvidenceSQL = `WITH ranked_keys AS (
	SELECT d.instrument_id,d.on_date,
	ROW_NUMBER() OVER (PARTITION BY d.instrument_id ORDER BY d.on_date DESC) AS slot_rank
	FROM investment_review_daily_inputs d
	WHERE d.household_id=? AND d.input_policy=? AND d.timezone_version=?
), latest_date AS (
	SELECT k.instrument_id,MAX(k.on_date) AS latest_on
	FROM ranked_keys k
	GROUP BY k.instrument_id
), latest_streak AS (
	SELECT s.instrument_id,s.latest_closed_on,MAX(s.consecutive_count) AS consecutive_count
	FROM investment_review_evaluation_state s
	WHERE s.rule_id=?
	GROUP BY s.instrument_id,s.latest_closed_on
)
SELECT d.instrument_id,d.on_date,d.input_fingerprint,d.evidence_json,d.coverage_json,r.evidence_json,r.input_fingerprint,COALESCE(s.consecutive_count,0)
FROM ranked_keys k
JOIN investment_review_daily_inputs d ON d.household_id=? AND d.input_policy=? AND d.timezone_version=? AND d.instrument_id=k.instrument_id AND d.on_date=k.on_date
LEFT JOIN investment_review_daily_results r ON r.rule_id=? AND r.condition_version=? AND r.instrument_id=k.instrument_id AND r.timezone_version=? AND r.on_date=k.on_date
LEFT JOIN latest_date ld ON ld.instrument_id=k.instrument_id
LEFT JOIN latest_streak s ON s.instrument_id=k.instrument_id AND s.latest_closed_on=ld.latest_on
WHERE k.slot_rank<=? ORDER BY k.instrument_id,k.on_date`

func profileReviewReusableEvidence(t *testing.T, fixture reviewFullPipelineFixture, metrics *reviewSQLStats) {
	t.Helper()
	ctx := context.Background()
	repository := sqlite.NewRepository(fixture.db)
	metrics.reset()
	started := time.Now()
	beforeCapture, err := repository.CaptureReviewProjection(ctx, fixture.householdID, *fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	beforeCaptureSQL := metrics.snapshot(8)
	t.Logf("production capture without trial index: %s; driver Query-to-Rows.Close aggregate=%s (includes row consumption, not pure SQL)", time.Since(started), beforeCaptureSQL.queryTime)

	beforeBytes := reviewPerfDatabaseBytes(t, fixture)
	beforePages, pageSize := reviewPerfPageStats(t, fixture)
	beforePlan, beforeCandidate, beforeCount := profileReusableEvidenceVariants(t, fixture, beforeCapture, false)
	t.Logf("without trial index, exact row outputs: rows=%d equivalent=true current={%s} narrow-keys={%s}", beforeCount, beforePlan.summary(), beforeCandidate.summary())

	indexStarted := time.Now()
	tx, err := fixture.db.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX idx_review_perf_inputs_scope_date ON investment_review_daily_inputs(household_id,input_policy,timezone_version,instrument_id,on_date DESC)`); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	indexBuild := time.Since(indexStarted)
	afterBytes := reviewPerfDatabaseBytes(t, fixture)
	afterPages, _ := reviewPerfPageStats(t, fixture)
	t.Logf("temporary additive-index migration on synthetic DB: elapsed=%s fileBytesDelta=%d pageDelta=%d pageSize=%d", indexBuild, afterBytes-beforeBytes, afterPages-beforePages, pageSize)

	metrics.reset()
	started = time.Now()
	afterCapture, err := repository.CaptureReviewProjection(ctx, fixture.householdID, *fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeCapture.ReusableDailyEvidence, afterCapture.ReusableDailyEvidence) {
		t.Fatal("trial index changed production reusable evidence")
	}
	afterCaptureSQL := metrics.snapshot(8)
	t.Logf("production capture with trial index: %s; driver Query-to-Rows.Close aggregate=%s (includes row consumption, not pure SQL); typed evidence equivalence=true", time.Since(started), afterCaptureSQL.queryTime)
	indexedPlan, indexedCandidate, indexedCount := profileReusableEvidenceVariants(t, fixture, afterCapture, true)
	if beforeCount != indexedCount {
		t.Fatalf("trial index changed row count: before=%d after=%d", beforeCount, indexedCount)
	}
	t.Logf("with trial index, exact row outputs: rows=%d equivalent=true current={%s} narrow-keys={%s}", indexedCount, indexedPlan.summary(), indexedCandidate.summary())
	t.Logf("trial-index storage after profile: database+wal+shm=%d bytes", afterBytes)
	logReviewFullPipelineStorage(t, fixture, int(fixture.now.Sub(fixture.start).Hours()/24))
}

func profileReusableEvidenceVariants(t *testing.T, fixture reviewFullPipelineFixture, capture domain.ReviewProjectionCapture, indexed bool) (reusableEvidenceProfile, reusableEvidenceProfile, int) {
	t.Helper()
	var currentTotal, narrowTotal reusableEvidenceProfile
	first := true
	for _, rule := range capture.Rules {
		if rule.Evaluator != domain.ReviewEvaluator {
			continue
		}
		policy := "rule:" + string(rule.ID) + "/v:" + strconv.Itoa(rule.CurrentConditionVersion)
		timezoneVersion := rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion
		width := 1
		switch rule.Spec.TriggerPolicy.Kind {
		case "consecutive":
			width = rule.Spec.TriggerPolicy.Count
		case "rolling":
			width = rule.Spec.TriggerPolicy.WindowDays
		}
		currentArgs := []any{fixture.householdID.String(), policy, timezoneVersion, fixture.householdID.String(), policy, timezoneVersion, string(rule.ID), string(rule.ID), rule.CurrentConditionVersion, timezoneVersion, width}
		narrowArgs := []any{fixture.householdID.String(), policy, timezoneVersion, string(rule.ID), fixture.householdID.String(), policy, timezoneVersion, string(rule.ID), rule.CurrentConditionVersion, timezoneVersion, width}
		if first {
			label := "without trial index"
			if indexed {
				label = "with trial index"
			}
			logReusableEvidencePlan(t, fixture.db.SQL, label+" current", currentReusableEvidenceSQL, currentArgs)
			logReusableEvidencePlan(t, fixture.db.SQL, label+" narrow-key", narrowKeyReusableEvidenceSQL, narrowArgs)
			first = false
		}
		current := timeReusableEvidenceQuery(t, fixture.db.SQL, currentReusableEvidenceSQL, currentArgs)
		narrow := timeReusableEvidenceQuery(t, fixture.db.SQL, narrowKeyReusableEvidenceSQL, narrowArgs)
		if !reflect.DeepEqual(current.rows, narrow.rows) {
			t.Fatalf("narrow-key query changed results for rule %s (indexed=%t): current=%d narrow=%d", rule.ID, indexed, len(current.rows), len(narrow.rows))
		}
		assertReusableEvidenceRowsMatchCapture(t, current.rows, capture, rule)
		assertReusableEvidenceRowsMatchCapture(t, narrow.rows, capture, rule)
		appendReusableEvidenceProfile(&currentTotal, current)
		appendReusableEvidenceProfile(&narrowTotal, narrow)
	}
	if len(currentTotal.rows) == 0 {
		t.Fatal("profiled no reusable daily evidence rows")
	}
	return currentTotal, narrowTotal, len(currentTotal.rows)
}

func assertReusableEvidenceRowsMatchCapture(t *testing.T, rows []reusableEvidenceProfileRow, capture domain.ReviewProjectionCapture, rule domain.ReviewRule) {
	t.Helper()
	want := make(map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence)
	for key, value := range capture.ReusableDailyEvidence {
		if key.RuleID == rule.ID {
			want[key] = value
		}
	}
	got := make(map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence, len(rows))
	for _, row := range rows {
		id, err := domain.ParseInstrumentID(row.instrumentID)
		if err != nil {
			t.Fatal(err)
		}
		item := domain.ReviewReusableDailyEvidence{InputFingerprint: row.inputFingerprint, PriorConsecutiveCount: row.consecutiveCount}
		if err := json.Unmarshal([]byte(row.inputJSON), &item.Input); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(row.coverageJSON), &item.CoverageMissing); err != nil {
			t.Fatal(err)
		}
		if row.resultJSON.Valid && row.resultFingerprint.Valid && row.resultFingerprint.String == row.inputFingerprint {
			if err := json.Unmarshal([]byte(row.resultJSON.String), &item.Result); err != nil {
				t.Fatal(err)
			}
			item.HasResult = item.Result.On == row.on
		}
		key := domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: id, TimezoneVersion: rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion, On: row.on}
		got[key] = item
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query rows differ from production capture for rule %s: got=%d want=%d", rule.ID, len(got), len(want))
	}
}

func timeReusableEvidenceQuery(t *testing.T, database *sql.DB, statement string, args []any) reusableEvidenceProfile {
	t.Helper()
	started := time.Now()
	queryStarted := time.Now()
	rows, err := database.QueryContext(context.Background(), statement, args...)
	if err != nil {
		t.Fatal(err)
	}
	profile := reusableEvidenceProfile{query: time.Since(queryStarted)}
	for {
		nextStarted := time.Now()
		next := rows.Next()
		profile.next += time.Since(nextStarted)
		if !next {
			break
		}
		var item reusableEvidenceProfileRow
		scanStarted := time.Now()
		if err := rows.Scan(&item.instrumentID, &item.on, &item.inputFingerprint, &item.inputJSON, &item.coverageJSON, &item.resultJSON, &item.resultFingerprint, &item.consecutiveCount); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		profile.scan += time.Since(scanStarted)
		jsonStarted := time.Now()
		var input domain.ReviewDailyInput
		var coverage []string
		if err := json.Unmarshal([]byte(item.inputJSON), &input); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(item.coverageJSON), &coverage); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		if item.resultJSON.Valid && item.resultFingerprint.Valid && item.resultFingerprint.String == item.inputFingerprint {
			var result domain.ReviewDailyResult
			if err := json.Unmarshal([]byte(item.resultJSON.String), &result); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
		}
		profile.jsonDecode += time.Since(jsonStarted)
		profile.rows = append(profile.rows, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	profile.queryToClose = time.Since(started)
	return profile
}

func appendReusableEvidenceProfile(total *reusableEvidenceProfile, item reusableEvidenceProfile) {
	total.query += item.query
	total.next += item.next
	total.scan += item.scan
	total.jsonDecode += item.jsonDecode
	total.queryToClose += item.queryToClose
	total.rows = append(total.rows, item.rows...)
}

func (p reusableEvidenceProfile) summary() string {
	return fmt.Sprintf("QueryContext=%s Rows.Next=%s Rows.Scan=%s JSONDecode=%s Query-to-Rows.Close=%s", p.query, p.next, p.scan, p.jsonDecode, p.queryToClose)
}

func logReusableEvidencePlan(t *testing.T, database *sql.DB, label, statement string, args []any) {
	t.Helper()
	rows, err := database.QueryContext(context.Background(), `EXPLAIN QUERY PLAN `+statement, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("EXPLAIN %s: %s", label, strings.Join(details, " | "))
}

func reviewPerfDatabaseBytes(t *testing.T, fixture reviewFullPipelineFixture) int64 {
	t.Helper()
	var total int64
	for _, path := range []string{fixture.dbPath, fixture.dbPath + "-wal", fixture.dbPath + "-shm"} {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		total += info.Size()
	}
	return total
}

func reviewPerfPageStats(t *testing.T, fixture reviewFullPipelineFixture) (int64, int64) {
	t.Helper()
	var pages, size int64
	if err := fixture.db.SQL.QueryRow(`PRAGMA page_count`).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.SQL.QueryRow(`PRAGMA page_size`).Scan(&size); err != nil {
		t.Fatal(err)
	}
	return pages, size
}

type reviewFullPipelineFixture struct {
	service     *Service
	db          *sqlite.DB
	now         *time.Time
	householdID domain.HouseholdID
	dbPath      string
	start       time.Time
	instruments []domain.Instrument
}

type reviewPerfConfig struct {
	Accounts, Instruments, Rules, HistoryDays, Quotes, CashActivities int
}

func referenceReviewPerfConfig(days int) reviewPerfConfig {
	return reviewPerfConfig{Accounts: 1, Instruments: 100, Rules: 20, HistoryDays: days, Quotes: 100000, CashActivities: 50000}
}

func (c reviewPerfConfig) validate() error {
	if c.Accounts < 1 || c.Instruments < 1 || c.Rules < 1 || c.HistoryDays < 1 || c.Quotes < 0 || c.CashActivities < 0 {
		return fmt.Errorf("invalid synthetic review fixture: %+v", c)
	}
	return nil
}

func validateReviewPerfMode(mode string) error {
	switch mode {
	case "", "cold", "warm", "warm365", "warm-marketdata", "warm-correction", "stages", "stages365", "stages-marketdata", "stages-correction", "profile", "profile365":
		return nil
	default:
		return fmt.Errorf("unknown NW_REVIEW_PERF_MODE %q", mode)
	}
}

func newReviewFullPipelineFixture(t *testing.T, historyDays int) reviewFullPipelineFixture {
	return newReviewPerfFixture(t, referenceReviewPerfConfig(historyDays))
}

func newReviewPerfFixture(t *testing.T, config reviewPerfConfig) reviewFullPipelineFixture {
	t.Helper()
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}
	historyDays := config.HistoryDays
	ctx := context.Background()
	start := time.Date(2025, 10, 17, 12, 0, 0, 0, time.UTC)
	now := start
	dbPath := t.TempDir() + "/review-full-pipeline.db"
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := NewService(sqlite.NewRepository(db))
	service.setClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic review pipeline", BaseCurrency: "CNY", MemberNames: []string{"Synthetic owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	accounts := make([]domain.AccountID, config.Accounts)
	for index := range accounts {
		account, err := service.CreateAccount(ctx, AccountInput{Name: fmt.Sprintf("Synthetic broker %03d", index), AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		accounts[index] = account.Account.ID
	}
	currencies := []string{"CNY", "USD", "EUR", "JPY", "SGD"}
	types := []domain.InstrumentType{domain.InstrumentStock, domain.InstrumentMutualFund, domain.InstrumentCrypto, domain.InstrumentBond}
	instruments := make([]domain.Instrument, config.Instruments)
	costs := make(map[domain.HoldingID]string, config.Instruments)
	for index := range instruments {
		currency := currencies[index%len(currencies)]
		kind := types[index%len(types)]
		instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: fmt.Sprintf("Synthetic holding %03d", index), Type: string(kind), QuoteCurrency: currency, QuoteSource: "manual"})
		if err != nil {
			t.Fatalf("create instrument %d: %v", index, err)
		}
		instruments[index] = instrument
		if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "100", start.Format(time.RFC3339), false); err != nil {
			t.Fatalf("add synthetic unit-cost seed quote %d: %v", index, err)
		}
		holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: accounts[index%len(accounts)].String(), InstrumentID: instrument.ID.String(), Quantity: "2"})
		if err != nil {
			t.Fatalf("create holding %d: %v", index, err)
		}
		costs[holding.ID] = "10"
	}
	for _, currency := range currencies[1:] {
		if _, err := service.SetFXPreference(ctx, currency, "CNY", "manual"); err != nil {
			t.Fatalf("set synthetic FX preference for %s: %v", currency, err)
		}
	}
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", costs); err != nil {
		t.Fatal(err)
	}

	if err := seedReviewFullPipelineFacts(ctx, db, bootstrap.Household.ID, instruments, accounts[0], start, historyDays, config.Quotes, config.CashActivities); err != nil {
		t.Fatalf("seed synthetic source facts: %v", err)
	}
	now = start.AddDate(0, 0, historyDays)
	for index := 0; index < config.Rules; index++ {
		spec := syntheticHistoricalReviewSpec()
		spec.Schedule.Timezone = []string{"UTC", "America/Los_Angeles", "Asia/Shanghai", "Europe/London"}[index%4]
		spec.Target = domain.ReviewRuleTarget{Kind: "all_current"}
		metric := index % 3
		if metric == 0 {
			spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "cost", Operator: "<", Value: "100000", Currency: "CNY", FXMode: "convert_on_day"}}
		} else if metric == 1 {
			spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "-100", LookbackDays: 7}}
		} else {
			spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "0", Currency: "CNY", FXMode: "convert_on_day"}}
		}
		if index%4 == 1 {
			spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "consecutive", Count: 3}
		} else if index%4 == 2 {
			spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 7, MinHits: 3}
		}
		if _, err := service.CreateReviewRuleV2(ctx, SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: fmt.Sprintf("Synthetic rule %02d", index), Enabled: true, Severity: "warning", Spec: spec}); err != nil {
			t.Fatalf("create rule %d: %v", index, err)
		}
	}
	return reviewFullPipelineFixture{service: service, db: db, now: &now, householdID: bootstrap.Household.ID, dbPath: dbPath, start: start, instruments: instruments}
}

func appendReviewPerfDailyMarketData(ctx context.Context, fixture reviewFullPipelineFixture, on time.Time) error {
	tx, err := fixture.db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	quoteStatement, err := tx.PrepareContext(ctx, `INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,value_effective_at,binding_revision,price_basis,timestamp_basis,revision) VALUES(?,?,?,?,'agent','agent',?,?, 'close',?,?,0,?,?,1)`)
	if err != nil {
		return err
	}
	defer quoteStatement.Close()
	date := on.UTC().Format(time.DateOnly)
	dayIndex := int(on.UTC().Sub(fixture.start).Hours() / 24)
	when := time.Date(on.Year(), on.Month(), on.Day(), 17, 0, 0, 0, time.UTC)
	createdAt := when.Add(time.Hour)
	for index, instrument := range fixture.instruments {
		priceBasis, timestampBasis := "agent_raw_close_v1", "date_label"
		if instrument.Type == domain.InstrumentMutualFund || instrument.Type == domain.InstrumentBankInvestmentProduct {
			priceBasis = "agent_unit_nav_v1"
		} else if instrument.Type == domain.InstrumentCrypto {
			priceBasis = domain.CoinGeckoDailyPriceBasis
			timestampBasis = "observed_publication"
		}
		id := fmt.Sprintf("00000000-0000-4000-8005-%012x", dayIndex*len(fixture.instruments)+index+1)
		price := fmt.Sprintf("%d", 100+dayIndex/30)
		if _, err := quoteStatement.ExecContext(ctx, id, instrument.ID.String(), price, instrument.QuoteCurrency.String(), formatReviewPerfTime(when), formatReviewPerfTime(createdAt), date, formatReviewPerfTime(when), priceBasis, timestampBasis); err != nil {
			return fmt.Errorf("insert daily synthetic quote for %s: %w", instrument.ID, err)
		}
	}
	fxStatement, err := tx.PrepareContext(ctx, `INSERT INTO fx_quotes(id,household_id,base_currency,quote_currency,rate,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,fetched_at,value_effective_at,timestamp_basis,revision) VALUES(?,?,?,'CNY','2','manual','manual',?,?, 'daily_reference',?,?,?,'date_label',1)`)
	if err != nil {
		return err
	}
	defer fxStatement.Close()
	for index, currency := range []string{"USD", "EUR", "JPY", "SGD"} {
		id := fmt.Sprintf("00000000-0000-4000-8006-%012x", dayIndex*4+index+1)
		if _, err := fxStatement.ExecContext(ctx, id, fixture.householdID.String(), currency, formatReviewPerfTime(when), formatReviewPerfTime(createdAt), date, formatReviewPerfTime(when), formatReviewPerfTime(when)); err != nil {
			return fmt.Errorf("insert daily synthetic FX reference for %s: %w", currency, err)
		}
	}
	return tx.Commit()
}

func correctReviewPerfHistoricalQuote(ctx context.Context, fixture reviewFullPipelineFixture, sample int) error {
	day := fixture.start.AddDate(0, 0, 5+sample%20).Format(time.DateOnly)
	tx, err := fixture.db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM instrument_quotes WHERE instrument_id=? AND effective_date=? AND source_kind='agent' ORDER BY created_at DESC,id DESC LIMIT 1`, fixture.instruments[0].ID.String(), day).Scan(&id); err != nil {
		return err
	}
	correctedPrice := fmt.Sprintf("%d", 250+sample)
	if _, err := tx.ExecContext(ctx, `UPDATE instrument_quotes SET unit_price=? WHERE id=?`, correctedPrice, id); err != nil {
		return err
	}
	return tx.Commit()
}

func countReviewPublicationDays(publication domain.ReviewPublication) int {
	count := 0
	for _, item := range publication.Rules {
		count += len(item.Days)
	}
	return count
}

func logReviewFullPipelineStorage(t *testing.T, fixture reviewFullPipelineFixture, historyDays int) {
	t.Helper()
	ctx := context.Background()
	counts := map[string]int{}
	for _, table := range []string{
		"investment_review_daily_inputs", "investment_review_daily_results", "investment_review_position_epochs",
		"investment_review_episodes", "investment_review_episode_events", "investment_review_evaluation_state",
	} {
		var count int
		if err := fixture.db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = count
	}
	bytes := int64(0)
	for _, path := range []string{fixture.dbPath, fixture.dbPath + "-wal", fixture.dbPath + "-shm"} {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("stat %s: %v", path, err)
		}
		bytes += info.Size()
	}
	t.Logf("published storage: historyDays=%d database+wal+shm=%d bytes inputRows=%d resultRows=%d positionEpochRows=%d episodeRows=%d episodeEventRows=%d evaluationStateRows=%d", historyDays, bytes, counts["investment_review_daily_inputs"], counts["investment_review_daily_results"], counts["investment_review_position_epochs"], counts["investment_review_episodes"], counts["investment_review_episode_events"], counts["investment_review_evaluation_state"])
}

func logReviewFullPipelineSourceCounts(t *testing.T, fixture reviewFullPipelineFixture, historyDays int) {
	t.Helper()
	ctx := context.Background()
	var quotes, activities, fx int
	for _, query := range []struct {
		sql  string
		into *int
	}{{`SELECT count(*) FROM instrument_quotes`, &quotes}, {`SELECT count(*) FROM activities`, &activities}, {`SELECT count(*) FROM fx_quotes`, &fx}} {
		if err := fixture.db.SQL.QueryRowContext(ctx, query.sql).Scan(query.into); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("synthetic source rows: holdings=100 rules=20 quoteRows=%d activities=%d fxRows=%d historyDays=%d", quotes, activities, fx, historyDays)
}

func seedReviewFullPipelineFacts(ctx context.Context, db *sqlite.DB, householdID domain.HouseholdID, instruments []domain.Instrument, accountID domain.AccountID, start time.Time, historyDays, quoteCount, activityCount int) error {
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	quoteStatement, err := tx.PrepareContext(ctx, `INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,value_effective_at,binding_revision,price_basis,timestamp_basis,revision) VALUES(?,?,?,?,'agent','agent',?,?, 'close',?,?,0,?,?,?)`)
	if err != nil {
		return err
	}
	defer quoteStatement.Close()
	quoteIndex := 0
	quoteSlots := len(instruments) * historyDays
	quotesPerSlot, quoteRemainder := quoteCount/quoteSlots, quoteCount%quoteSlots
	for day := 0; day < historyDays; day++ {
		date := start.AddDate(0, 0, day)
		for instrumentIndex, instrument := range instruments {
			copies := quotesPerSlot
			if instrumentIndex*historyDays+day < quoteRemainder {
				copies++
			}
			for duplicate := 0; duplicate < copies; duplicate++ {
				priceBasis, timestampBasis := "agent_raw_close_v1", "date_label"
				if instrument.Type == domain.InstrumentMutualFund || instrument.Type == domain.InstrumentBankInvestmentProduct {
					priceBasis = "agent_unit_nav_v1"
				} else if instrument.Type == domain.InstrumentCrypto {
					priceBasis = domain.CoinGeckoDailyPriceBasis
					timestampBasis = "observed_publication"
				}
				when := date.Add(17 * time.Hour)
				price := fmt.Sprintf("%d", 100+day/30)
				id := fmt.Sprintf("00000000-0000-4000-8000-%012x", quoteIndex+1)
				createdAt := when.Add(time.Duration(duplicate) * time.Second)
				if _, err := quoteStatement.ExecContext(ctx, id, instrument.ID.String(), price, instrument.QuoteCurrency.String(), formatReviewPerfTime(when), formatReviewPerfTime(createdAt), date.Format(time.DateOnly), formatReviewPerfTime(when), priceBasis, timestampBasis, duplicate+1); err != nil {
					return fmt.Errorf("insert quote %d: %w", quoteIndex, err)
				}
				quoteIndex++
			}
		}
	}
	if quoteIndex != quoteCount {
		return fmt.Errorf("seeded %d quote facts, want %d", quoteIndex, quoteCount)
	}
	fxStatement, err := tx.PrepareContext(ctx, `INSERT INTO fx_quotes(id,household_id,base_currency,quote_currency,rate,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,fetched_at,value_effective_at,timestamp_basis,revision) VALUES(?,?,?,'CNY',?,'manual','manual',?,?, 'daily_reference',?,?,?,'date_label',1)`)
	if err != nil {
		return err
	}
	defer fxStatement.Close()
	fxIndex := 0
	for day := 0; day < historyDays; day++ {
		date := start.AddDate(0, 0, day)
		for _, currency := range []string{"USD", "EUR", "JPY", "SGD"} {
			when := date.Add(16 * time.Hour)
			id := fmt.Sprintf("00000000-0000-4000-8001-%012x", fxIndex+1)
			if _, err := fxStatement.ExecContext(ctx, id, householdID.String(), currency, "2", formatReviewPerfTime(when), formatReviewPerfTime(when), date.Format(time.DateOnly), formatReviewPerfTime(when), formatReviewPerfTime(when)); err != nil {
				return fmt.Errorf("insert synthetic FX reference %d: %w", fxIndex, err)
			}
			fxIndex++
		}
	}
	activityStatement, err := tx.PrepareContext(ctx, `INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'value_update','other',?,?,?)`)
	if err != nil {
		return err
	}
	defer activityStatement.Close()
	effectStatement, err := tx.PrepareContext(ctx, `INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'observation','added','account_cash','adjustment',?,'0','CNY')`)
	if err != nil {
		return err
	}
	defer effectStatement.Close()
	valueStatement, err := tx.PrepareContext(ctx, `INSERT INTO account_cash_values(id,account_id,amount,currency,effective_at,created_at,activity_effect_id,projection_kind) VALUES(?,?,'0','CNY',?,?,?,'event')`)
	if err != nil {
		return err
	}
	defer valueStatement.Close()
	for index := 0; index < activityCount; index++ {
		when := start.AddDate(0, 0, index%historyDays).Add(time.Duration(index) * time.Microsecond)
		at := formatReviewPerfTime(when)
		activityID := fmt.Sprintf("00000000-0000-4000-8002-%012x", index+1)
		effectID := fmt.Sprintf("00000000-0000-4000-8003-%012x", index+1)
		valueID := fmt.Sprintf("00000000-0000-4000-8004-%012x", index+1)
		if _, err := activityStatement.ExecContext(ctx, activityID, householdID.String(), at, when.Format(time.DateOnly), at); err != nil {
			return fmt.Errorf("insert activity %d: %w", index, err)
		}
		if _, err := effectStatement.ExecContext(ctx, effectID, activityID, accountID.String()); err != nil {
			return fmt.Errorf("insert activity effect %d: %w", index, err)
		}
		if _, err := valueStatement.ExecContext(ctx, valueID, accountID.String(), at, at, effectID); err != nil {
			return fmt.Errorf("insert account cash value %d: %w", index, err)
		}
	}
	return tx.Commit()
}

func formatReviewPerfTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func durationPercentile(values []time.Duration, percentile float64) time.Duration {
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	index := int(math.Ceil(percentile*float64(len(ordered)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(ordered) {
		index = len(ordered) - 1
	}
	return ordered[index]
}
