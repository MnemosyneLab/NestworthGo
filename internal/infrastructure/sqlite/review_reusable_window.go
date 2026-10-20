package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Retained verbatim as the selectable key-first oracle/rollback path.
const reviewWindowBaselineSQL = `WITH ranked_keys AS (
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
		LEFT JOIN latest_date ld ON ld.instrument_id=d.instrument_id
		LEFT JOIN latest_streak s ON s.instrument_id=k.instrument_id AND s.latest_closed_on=ld.latest_on
		WHERE k.slot_rank<=? ORDER BY k.instrument_id,k.on_date`

// Dates, not true votes, define the window. LIMIT applies before reading JSON;
// the existing PK seeks household/instrument/timezone then walks dates backward.
// Old input policies can add key visits, but never JSON reads for rejected keys.
const reviewWindowDatesSQL = `SELECT on_date FROM investment_review_daily_inputs
 WHERE household_id=? AND instrument_id=? AND timezone_version=? AND input_policy=?
 ORDER BY on_date DESC LIMIT ?`

func reviewWindowWidth(rule domain.ReviewRule) int {
	switch rule.Spec.TriggerPolicy.Kind {
	case "consecutive":
		return rule.Spec.TriggerPolicy.Count
	case "rolling":
		return rule.Spec.TriggerPolicy.WindowDays
	default:
		return 1
	}
}

// Match the publication candidate universe, including previously evaluated and
// closed positions. A current-positive-holdings-only query loses lifecycle data.
func reviewWindowInstruments(capture domain.ReviewProjectionCapture, rule domain.ReviewRule) []domain.InstrumentID {
	ids := map[domain.InstrumentID]bool{}
	if rule.ArchivedAt == nil {
		if rule.Spec.Target.Kind == "instruments" {
			for _, id := range rule.Spec.Target.InstrumentIDs {
				ids[id] = true
			}
		} else {
			for _, holding := range capture.Historical.History.Portfolio.Holdings {
				ids[holding.InstrumentID] = true
			}
		}
	}
	for _, id := range capture.EvaluatedInstruments[rule.ID] {
		ids[id] = true
	}
	for _, episode := range capture.OpenEpisodes {
		if episode.RuleID == rule.ID {
			ids[episode.InstrumentID] = true
		}
	}
	result := make([]domain.InstrumentID, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	slices.Sort(result)
	return result
}

func (r *Repository) loadReusableReviewWindow(ctx context.Context, tx queryer, capture *domain.ReviewProjectionCapture) error {
	evidence := make(map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence)
	capture.ReusableDailyEvidence = evidence
	if r.reviewFullRecompute || capture.TemporalFullRecompute || capture.Control.ProjectionStatus != "complete" || capture.Control.PublishedFingerprint != capture.HistoricalFingerprint {
		return nil
	}
	for _, rule := range capture.Rules {
		if rule.Evaluator != domain.ReviewEvaluator {
			continue
		}
		policy := "rule:" + string(rule.ID) + "/v:" + strconv.Itoa(rule.CurrentConditionVersion)
		tz := rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion
		if r.reviewWindowBaseline {
			rows, err := tx.QueryContext(ctx, reviewWindowBaselineSQL, capture.HouseholdID.String(), policy, tz, string(rule.ID), capture.HouseholdID.String(), policy, tz, string(rule.ID), rule.CurrentConditionVersion, tz, reviewWindowWidth(rule))
			if err != nil {
				return err
			}
			if err := scanReusableReviewWindow(rows, rule, tz, evidence, &reviewSharedInputResolver{ctx: ctx, tx: tx, household: capture.HouseholdID}); err != nil {
				return err
			}
			continue
		}
		ids := reviewWindowInstruments(*capture, rule)
		for start := 0; start < len(ids); start += 100 {
			end := start + 100
			if end > len(ids) {
				end = len(ids)
			}
			statement, args := reviewWindowChunkQuery(*capture, rule, ids[start:end])
			rows, err := tx.QueryContext(ctx, statement, args...)
			if err != nil {
				return err
			}
			if err := scanReusableReviewWindow(rows, rule, tz, evidence, &reviewSharedInputResolver{ctx: ctx, tx: tx, household: capture.HouseholdID}); err != nil {
				return err
			}
		}
	}
	return nil
}

// At most 100 IDs + 14 scalar parameters per query, far below SQLite's
// supported parameter limit. Each correlated subquery seeks the same existing
// PK prefix; the outer join reads JSON for only the selected full date keys.
func reviewWindowChunkQuery(capture domain.ReviewProjectionCapture, rule domain.ReviewRule, ids []domain.InstrumentID) (string, []any) {
	values := make([]string, len(ids))
	args := make([]any, 0, len(ids)+14)
	for i, id := range ids {
		values[i] = "(?)"
		args = append(args, id.String())
	}
	tz := rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion
	policy := fmt.Sprintf("rule:%s/v:%d", rule.ID, rule.CurrentConditionVersion)
	household := capture.HouseholdID.String()
	args = append(args, household, tz, policy, household, tz, policy, reviewWindowWidth(rule), string(rule.ID), household, tz, policy, string(rule.ID), rule.CurrentConditionVersion, tz)
	// Materialize the per-instrument aggregate once: retained closed epochs can
	// be numerous, and a correlated scalar here would rescan them for every day.
	statement := `WITH requested(instrument_id) AS (VALUES ` + strings.Join(values, ",") + `),
 window_keys AS MATERIALIZED (
 SELECT i.instrument_id,d.on_date FROM requested i CROSS JOIN investment_review_daily_inputs d
 ON d.household_id=? AND d.instrument_id=i.instrument_id AND d.timezone_version=? AND d.input_policy=?
 AND d.on_date IN (SELECT on_date FROM investment_review_daily_inputs k
 WHERE k.household_id=? AND k.instrument_id=i.instrument_id AND k.timezone_version=? AND k.input_policy=? ORDER BY on_date DESC LIMIT ?)
 ), latest_date AS (SELECT instrument_id,MAX(on_date) latest_on FROM window_keys GROUP BY instrument_id),
 latest_streak AS MATERIALIZED (
 SELECT ld.instrument_id,COALESCE(MAX(s.consecutive_count),0) consecutive_count
 FROM latest_date ld LEFT JOIN investment_review_evaluation_state s
 ON s.rule_id=? AND s.instrument_id=ld.instrument_id AND s.latest_closed_on=ld.latest_on
 GROUP BY ld.instrument_id)
 SELECT d.instrument_id,d.on_date,d.input_fingerprint,d.evidence_json,d.coverage_json,
 r.evidence_json,r.input_fingerprint,s.consecutive_count
 FROM window_keys k
 CROSS JOIN investment_review_daily_inputs d ON d.household_id=? AND d.timezone_version=? AND d.input_policy=? AND d.instrument_id=k.instrument_id AND d.on_date=k.on_date
 LEFT JOIN investment_review_daily_results r ON r.rule_id=? AND r.condition_version=? AND r.timezone_version=? AND r.instrument_id=k.instrument_id AND r.on_date=k.on_date
 JOIN latest_streak s ON s.instrument_id=k.instrument_id ORDER BY k.instrument_id,k.on_date`
	return statement, args
}

func scanReusableReviewWindow(rows *sql.Rows, rule domain.ReviewRule, timezoneVersion string, evidence map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence, resolvers ...*reviewSharedInputResolver) error {
	for rows.Next() {
		var instrumentID, on, inputFingerprint, inputJSON, coverageJSON string
		var resultJSON, resultFingerprint sql.NullString
		var priorConsecutiveCount int
		if err := rows.Scan(&instrumentID, &on, &inputFingerprint, &inputJSON, &coverageJSON, &resultJSON, &resultFingerprint, &priorConsecutiveCount); err != nil {
			rows.Close()
			return err
		}
		id, err := domain.ParseInstrumentID(instrumentID)
		if err != nil {
			rows.Close()
			return err
		}
		var resolver *reviewSharedInputResolver
		if len(resolvers) > 0 {
			resolver = resolvers[0]
		}
		key := domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: id, TimezoneVersion: timezoneVersion, On: on}
		inputJSON, err = resolver.resolve(inputJSON, key, coverageJSON, inputFingerprint)
		if err != nil {
			rows.Close()
			return err
		}
		var item domain.ReviewReusableDailyEvidence
		if err := decodeReviewInput([]byte(inputJSON), &item.Input); err != nil {
			rows.Close()
			return err
		}
		if err := jsoncodec.Unmarshal([]byte(coverageJSON), &item.CoverageMissing); err != nil {
			rows.Close()
			return err
		}
		if item.Input.On != on || item.Input.InstrumentID != id {
			rows.Close()
			return fmt.Errorf("invalid cached investment review input for %s on %s", instrumentID, on)
		}
		item.InputFingerprint = inputFingerprint
		item.PriorConsecutiveCount = priorConsecutiveCount
		if resultJSON.Valid && resultFingerprint.Valid && resultFingerprint.String == inputFingerprint {
			if err := jsoncodec.Unmarshal([]byte(resultJSON.String), &item.Result); err != nil {
				rows.Close()
				return err
			}
			item.HasResult = item.Result.On == on
		}
		evidence[key] = item
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	return rows.Close()
}
