package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	modernsqlite "modernc.org/sqlite"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

var reviewWindowKeyVisits atomic.Int64
var reviewWindowEpochVisits atomic.Int64

func init() {
	modernsqlite.MustRegisterScalarFunction("nw_review_test_epoch_visit", 1, func(_ *modernsqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
		reviewWindowEpochVisits.Add(1)
		return int64(1), nil
	})
	modernsqlite.MustRegisterScalarFunction("nw_review_test_key_visit", 1, func(_ *modernsqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
		reviewWindowKeyVisits.Add(1)
		return int64(1), nil
	})
}

func TestReviewReusableWindowEquivalent(t *testing.T) {
	epochs := 128
	if value := os.Getenv("NW_REVIEW_PERF_EPOCHS"); value != "" {
		var err error
		epochs, err = strconv.Atoi(value)
		if err != nil || epochs < 3 {
			t.Fatal("invalid NW_REVIEW_PERF_EPOCHS")
		}
	}
	t.Logf("epoch fixture: %d stored states per instrument (production primary key, no extra index)", epochs)
	for _, days := range []int{30, 365, 1095} {
		t.Run(fmt.Sprintf("days-%d", days), func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			for _, statement := range []string{
				`CREATE TABLE investment_review_daily_inputs(household_id TEXT,instrument_id TEXT,timezone_version TEXT,on_date TEXT,input_policy TEXT,input_fingerprint TEXT,evidence_json TEXT,coverage_json TEXT,PRIMARY KEY(household_id,instrument_id,timezone_version,on_date,input_policy))`,
				`CREATE TABLE investment_review_daily_results(rule_id TEXT,condition_version INTEGER,instrument_id TEXT,timezone_version TEXT,on_date TEXT,evidence_json TEXT,input_fingerprint TEXT,PRIMARY KEY(rule_id,condition_version,instrument_id,timezone_version,on_date))`,
				`CREATE TABLE investment_review_evaluation_state(rule_id TEXT,instrument_id TEXT,position_epoch_id TEXT,latest_closed_on TEXT,consecutive_count INTEGER,PRIMARY KEY(rule_id,instrument_id,position_epoch_id))`,
			} {
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			household := domain.NewHouseholdID()
			first, closed := domain.NewInstrumentID(), domain.NewInstrumentID()
			rule := domain.ReviewRule{ID: domain.NewReviewRuleID(), CurrentConditionVersion: 4, Evaluator: domain.ReviewEvaluator}
			rule.Spec.Schedule.Timezone = "Pacific/Apia"
			rule.Spec.Schedule.CutoffVersion = "v1"
			rule.Spec.Target.Kind = "instruments"
			rule.Spec.Target.InstrumentIDs = []domain.InstrumentID{first, first}
			tz := "Pacific/Apia/v1"
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []domain.InstrumentID{first, closed} {
				for day := 0; day < days; day++ {
					on := time.Date(2011, 12, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, day).Format(time.DateOnly)
					// Missing slots and Apia's nonexistent day are deliberately retained as
					// holes. LIMIT must fetch stored slots, never latest minus W days.
					if day%13 == 5 || on == "2011-12-30" {
						continue
					}
					currency := domain.CurrencyCode("USD")
					input := domain.ReviewDailyInput{InstrumentID: id, On: on, NativeCurrency: &currency}
					quantity := "2.000000000000000001"
					input.Quantity = &quantity
					inputJSON, _ := json.Marshal(input)
					for version := 1; version <= 4; version++ {
						policy := fmt.Sprintf("rule:%s/v:%d", rule.ID, version)
						if _, err := tx.Exec(`INSERT INTO investment_review_daily_inputs VALUES(?,?,?,?,?,?,?,?)`, household.String(), id.String(), tz, on, policy, "exact-fingerprint", string(inputJSON), `["quote_missing"]`); err != nil {
							t.Fatal(err)
						}
					}
					truth := []domain.ReviewTruth{domain.ReviewFalse, domain.ReviewUnknown, domain.ReviewTrue}[day%3]
					resultJSON, _ := json.Marshal(domain.ReviewDailyResult{On: on, Truth: truth})
					fingerprint := "exact-fingerprint"
					if day%7 == 0 {
						fingerprint = "stale-result"
					}
					if day%11 != 0 {
						if _, err := tx.Exec(`INSERT INTO investment_review_daily_results VALUES(?,?,?,?,?,?,?)`, string(rule.ID), 4, id.String(), tz, on, string(resultJSON), fingerprint); err != nil {
							t.Fatal(err)
						}
					}
				}
				var latest string
				if err := tx.QueryRow(`SELECT MAX(on_date) FROM investment_review_daily_inputs WHERE instrument_id=?`, id.String()).Scan(&latest); err != nil {
					t.Fatal(err)
				}
				for epoch := 3; epoch < epochs; epoch++ {
					if _, err := tx.Exec(`INSERT INTO investment_review_evaluation_state VALUES(?,?,?,?,?)`, string(rule.ID), id.String(), fmt.Sprintf("closed-epoch-%d", epoch), latest, 0); err != nil {
						t.Fatal(err)
					}
				}
				for _, state := range []struct {
					on    string
					count int
				}{{latest, days}, {latest, days + 7}, {"1900-01-01", 99999}} {
					if _, err := tx.Exec(`INSERT INTO investment_review_evaluation_state VALUES(?,?,?,?,?)`, string(rule.ID), id.String(), fmt.Sprintf("epoch-%s-%d", state.on, state.count), state.on, state.count); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			capture := domain.ReviewProjectionCapture{HouseholdID: household, HistoricalFingerprint: "source", Rules: []domain.ReviewRule{rule}, Control: domain.ReviewProjectionControl{ProjectionStatus: "complete", PublishedFingerprint: "source"}, EvaluatedInstruments: map[domain.ReviewRuleID][]domain.InstrumentID{rule.ID: {closed}}, OpenEpisodes: []domain.ReviewStoredEpisode{{RuleID: rule.ID, InstrumentID: closed}}}
			for _, policy := range []domain.ReviewTriggerPolicy{{Kind: "immediate"}, {Kind: "consecutive", Count: 3}, {Kind: "rolling", WindowDays: 7, MinHits: 3}} {
				capture.Rules[0].Spec.TriggerPolicy = policy
				baseline, candidate := capture, capture
				if err := (&Repository{reviewWindowBaseline: true}).loadReusableReviewWindow(t.Context(), db, &baseline); err != nil {
					t.Fatal(err)
				}
				if err := (&Repository{}).loadReusableReviewWindow(t.Context(), db, &candidate); err != nil {
					t.Fatal(err)
				}
				trial := capture
				if err := loadReviewWindowChunkTrial(t.Context(), db, &trial); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(baseline.ReusableDailyEvidence, trial.ReusableDailyEvidence) {
					t.Fatal("per-instrument trial differs")
				}
				if !reflect.DeepEqual(baseline.ReusableDailyEvidence, candidate.ReusableDailyEvidence) {
					t.Fatal("typed evidence differs")
				}
				if len(candidate.ReusableDailyEvidence) != 2*reviewWindowWidth(capture.Rules[0]) {
					t.Fatalf("unexpected slots: %d", len(candidate.ReusableDailyEvidence))
				}
				for _, item := range candidate.ReusableDailyEvidence {
					if item.PriorConsecutiveCount != days+7 {
						t.Fatalf("streak anchor lost: %d", item.PriorConsecutiveCount)
					}
				}
			}
			args := []any{household.String(), first.String(), tz, "rule:" + string(rule.ID) + "/v:4", 7}
			rows, err := db.Query(`EXPLAIN QUERY PLAN `+reviewWindowDatesSQL, args...)
			if err != nil {
				t.Fatal(err)
			}
			var plan []string
			for rows.Next() {
				var a, b, c int
				var detail string
				if err := rows.Scan(&a, &b, &c, &detail); err != nil {
					t.Fatal(err)
				}
				plan = append(plan, detail)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			t.Logf("bounded dates EXPLAIN: %v", plan)
			if !strings.Contains(strings.Join(plan, " "), "COVERING INDEX sqlite_autoindex_investment_review_daily_inputs_1 (household_id=? AND instrument_id=? AND timezone_version=?)") {
				t.Fatalf("lost primary-key prefix seek: %v", plan)
			}
			if strings.Contains(strings.Join(plan, " "), "TEMP B-TREE") {
				t.Fatal("date selection sorted history")
			}
			measureReviewWindowKeyVisits(t, db, capture)
			boundedSQL, boundedArgs := reviewWindowChunkQuery(capture, capture.Rules[0], reviewWindowInstruments(capture, capture.Rules[0]))
			logReviewWindowExplain(t, db, "bounded-chunk", boundedSQL, boundedArgs)
			probe := strings.ReplaceAll(boundedSQL, "s.rule_id=?", "nw_review_test_epoch_visit(s.position_epoch_id) AND s.rule_id=?")
			reviewWindowEpochVisits.Store(0)
			rows, err = db.Query(probe, boundedArgs...)
			if err != nil {
				t.Fatal(err)
			}
			if err := scanReusableReviewWindow(rows, capture.Rules[0], tz, map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence{}); err != nil {
				t.Fatal(err)
			}
			t.Logf("bounded epoch predicate visits=%d storedStates=%d returnedDays=%d", reviewWindowEpochVisits.Load(), 2*epochs, 14)
			// This predicate probe counts evaluation-state candidates only, not all
			// SQLite opcodes or source-row visits. Width must not multiply epoch work.
			if got := reviewWindowEpochVisits.Load(); got != int64(2*epochs) {
				t.Fatalf("epoch aggregate repeated across returned days: got=%d want=%d", got, 2*epochs)
			}
			logReviewWindowExplain(t, db, "baseline-ranking", reviewWindowBaselineSQL, []any{household.String(), "rule:" + string(rule.ID) + "/v:4", tz, string(rule.ID), household.String(), "rule:" + string(rule.ID) + "/v:4", tz, string(rule.ID), 4, tz, 7})

			if os.Getenv("NW_REVIEW_PERF_EVIDENCE") == "1" {
				measureReviewWindowQueries(t, db, capture)
			}
			if days == 30 {
				tx, err := db.Begin()
				if err != nil {
					t.Fatal(err)
				}
				for i := 0; i < 200; i++ {
					id := domain.NewInstrumentID()
					inputJSON, _ := json.Marshal(domain.ReviewDailyInput{InstrumentID: id, On: "2011-12-31"})
					if _, err := tx.Exec(`INSERT INTO investment_review_daily_inputs VALUES(?,?,?,?,?,?,?,?)`, household.String(), id.String(), tz, "2011-12-31", "rule:"+string(rule.ID)+"/v:4", "boundary-fingerprint", string(inputJSON), `[]`); err != nil {
						t.Fatal(err)
					}
					capture.EvaluatedInstruments[rule.ID] = append(capture.EvaluatedInstruments[rule.ID], id)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				baseline, candidate := capture, capture
				if err := (&Repository{reviewWindowBaseline: true}).loadReusableReviewWindow(t.Context(), db, &baseline); err != nil {
					t.Fatal(err)
				}
				if err := (&Repository{}).loadReusableReviewWindow(t.Context(), db, &candidate); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(baseline.ReusableDailyEvidence, candidate.ReusableDailyEvidence) {
					t.Fatal("100-ID chunk boundary lost rows")
				}
				canceled, cancel := context.WithCancel(t.Context())
				cancel()
				if err := (&Repository{}).loadReusableReviewWindow(canceled, db, &candidate); err == nil {
					t.Fatal("canceled query succeeded")
				}
			}

		})
	}
}

func measureReviewWindowQueries(t *testing.T, db *sql.DB, capture domain.ReviewProjectionCapture) {
	t.Helper()
	// Alternate old/new order. Measurements include typed JSON consumption and
	// Rows.Close, with immutable inputs and no writes or other benchmark tasks.
	for sample := 0; sample < 30; sample++ {
		for offset := 0; offset < 3; offset++ {
			strategy := (sample + offset) % 3
			candidate := capture
			started := time.Now()
			var err error
			if strategy == 2 {
				err = loadReviewWindowChunkTrial(context.Background(), db, &candidate)
			} else {
				err = (&Repository{reviewWindowBaseline: strategy == 0}).loadReusableReviewWindow(context.Background(), db, &candidate)
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("window sample=%d strategy=%d elapsed_ns=%d returnedRows=%d", sample, strategy, time.Since(started).Nanoseconds(), len(candidate.ReusableDailyEvidence))
		}
	}
}

const reviewWindowBoundedSQL = `WITH window_dates AS MATERIALIZED (` + reviewWindowDatesSQL + `),
 latest_streak AS (
 SELECT MAX(s.consecutive_count) AS consecutive_count
 FROM investment_review_evaluation_state s
 WHERE s.rule_id=? AND s.instrument_id=?
 AND s.latest_closed_on=(SELECT MAX(on_date) FROM window_dates)
 )
 SELECT d.instrument_id,d.on_date,d.input_fingerprint,d.evidence_json,d.coverage_json,
 r.evidence_json,r.input_fingerprint,COALESCE(s.consecutive_count,0)
 FROM window_dates k
 CROSS JOIN investment_review_daily_inputs d ON d.household_id=? AND d.instrument_id=? AND d.timezone_version=? AND d.on_date=k.on_date AND d.input_policy=?
 LEFT JOIN investment_review_daily_results r ON r.rule_id=? AND r.condition_version=? AND r.instrument_id=d.instrument_id AND r.timezone_version=? AND r.on_date=k.on_date
 CROSS JOIN latest_streak s ORDER BY d.instrument_id,d.on_date`

// Per-instrument prepared seek retained as a measured alternative to chunks.
func loadReviewWindowChunkTrial(ctx context.Context, db *sql.DB, capture *domain.ReviewProjectionCapture) error {
	capture.ReusableDailyEvidence = map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence{}
	statement, err := db.PrepareContext(ctx, reviewWindowBoundedSQL)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, rule := range capture.Rules {
		tz := rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion
		policy := fmt.Sprintf("rule:%s/v:%d", rule.ID, rule.CurrentConditionVersion)
		for _, id := range reviewWindowInstruments(*capture, rule) {
			rows, err := statement.QueryContext(ctx, capture.HouseholdID.String(), id.String(), tz, policy, reviewWindowWidth(rule), string(rule.ID), id.String(), capture.HouseholdID.String(), id.String(), tz, policy, string(rule.ID), rule.CurrentConditionVersion, tz)
			if err != nil {
				return err
			}
			if err := scanReusableReviewWindow(rows, rule, tz, capture.ReusableDailyEvidence); err != nil {
				return err
			}
		}
	}
	return nil
}

// A non-deterministic SQL probe counts predicate evaluations in otherwise
// identical date-selection queries. These are instrumented key evaluations,
// not SQLite VM steps, disk reads or returned JSON rows. Timing uses unprobed SQL.
func measureReviewWindowKeyVisits(t *testing.T, db *sql.DB, capture domain.ReviewProjectionCapture) {
	t.Helper()
	rule := capture.Rules[0]
	tz := rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion
	policy := fmt.Sprintf("rule:%s/v:%d", rule.ID, rule.CurrentConditionVersion)
	baseline := strings.Replace(reviewWindowBaselineSQL, "WHERE d.household_id=?", "WHERE nw_review_test_key_visit(d.on_date) AND d.household_id=?", 1)
	reviewWindowKeyVisits.Store(0)
	rows, err := db.Query(baseline, capture.HouseholdID.String(), policy, tz, string(rule.ID), capture.HouseholdID.String(), policy, tz, string(rule.ID), rule.CurrentConditionVersion, tz, reviewWindowWidth(rule))
	if err != nil {
		t.Fatal(err)
	}
	evidence := map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence{}
	if err := scanReusableReviewWindow(rows, rule, tz, evidence); err != nil {
		t.Fatal(err)
	}
	before := reviewWindowKeyVisits.Load()
	reviewWindowKeyVisits.Store(0)
	bounded := strings.Replace(reviewWindowDatesSQL, "WHERE household_id=?", "WHERE nw_review_test_key_visit(on_date) AND household_id=?", 1)
	for _, id := range reviewWindowInstruments(capture, rule) {
		rows, err := db.Query(bounded, capture.HouseholdID.String(), id.String(), tz, policy, reviewWindowWidth(rule))
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var on string
			if err := rows.Scan(&on); err != nil {
				t.Fatal(err)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	after := reviewWindowKeyVisits.Load()
	t.Logf("instrumented date-key predicate evaluations: baseline=%d bounded=%d returnedRows=%d", before, after, len(evidence))
	if after >= before {
		t.Fatal("window key work failed to decrease")
	}
}

func TestReviewWindowCandidateUniverse(t *testing.T) {
	target, holding, evaluated, episode := domain.NewInstrumentID(), domain.NewInstrumentID(), domain.NewInstrumentID(), domain.NewInstrumentID()
	rule := domain.ReviewRule{ID: domain.NewReviewRuleID()}
	rule.Spec.Target.Kind = "instruments"
	rule.Spec.Target.InstrumentIDs = []domain.InstrumentID{target, target}
	capture := domain.ReviewProjectionCapture{EvaluatedInstruments: map[domain.ReviewRuleID][]domain.InstrumentID{rule.ID: {evaluated}}, OpenEpisodes: []domain.ReviewStoredEpisode{{RuleID: rule.ID, InstrumentID: episode}}}
	capture.Historical.History.Portfolio.Holdings = []domain.Holding{{InstrumentID: holding}}
	assert := func(want ...domain.InstrumentID) {
		t.Helper()
		got := reviewWindowInstruments(capture, rule)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("universe=%v want=%v", got, want)
		}
	}
	assert(target, evaluated, episode)
	rule.Spec.Target.Kind = "all_current"
	assert(holding, evaluated, episode)
	archived := time.Now()
	rule.ArchivedAt = &archived
	assert(evaluated, episode)
	capture.EvaluatedInstruments = nil
	capture.OpenEpisodes = nil
	assert()
}

func logReviewWindowExplain(t *testing.T, db *sql.DB, label, statement string, args []any) {
	t.Helper()
	rows, err := db.Query(`EXPLAIN QUERY PLAN `+statement, args...)
	if err != nil {
		t.Fatal(err)
	}
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
	rows.Close()
	t.Logf("EXPLAIN %s: %s", label, strings.Join(details, " | "))
	if label == "bounded-chunk" && !strings.Contains(strings.Join(details, " "), "SEARCH d USING INDEX sqlite_autoindex_investment_review_daily_inputs_1 (household_id=? AND instrument_id=? AND timezone_version=? AND on_date=? AND input_policy=?)") {
		t.Fatal("bounded JSON join lost complete date key")
	}
}
