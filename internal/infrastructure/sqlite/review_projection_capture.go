package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// CaptureReviewProjection uses one SQLite snapshot for financial facts, rule
// revisions, handling state and the existing open episode identities. It does
// not create control rows or daily snapshots as a side effect of reading.
func (r *Repository) CaptureReviewProjection(ctx context.Context, householdID domain.HouseholdID, cutoff time.Time) (domain.ReviewProjectionCapture, error) {
	var empty domain.ReviewProjectionCapture
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	result := domain.ReviewProjectionCapture{HouseholdID: householdID, SourceCutoff: cutoff, Rules: []domain.ReviewRule{}, Suppressions: []domain.ReviewSuppression{}, OpenEpisodes: []domain.ReviewStoredEpisode{}, EvaluatedInstruments: map[domain.ReviewRuleID][]domain.InstrumentID{}}
	result.Historical, err = loadReviewHistoricalBatchQuery(ctx, tx, householdID, cutoff)
	if err != nil {
		return empty, err
	}
	result.HistoricalFingerprint, err = domain.ReviewHistoricalFingerprint(result.Historical)
	if err != nil {
		return empty, err
	}
	result.Control = domain.ReviewProjectionControl{Enabled: true, ConfigRevision: 1, ProjectionStatus: "pending"}
	var enabled int
	var published, fingerprint sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT enabled,config_revision,input_generation,review_state_revision,projection_status,published_on,published_input_fingerprint FROM investment_review_control WHERE household_id=?`, householdID.String()).Scan(&enabled, &result.Control.ConfigRevision, &result.Control.InputGeneration, &result.Control.ReviewStateRevision, &result.Control.ProjectionStatus, &published, &fingerprint)
	if err == nil {
		result.Control.Enabled = enabled != 0
		result.Control.PublishedOn = published.String
		result.Control.PublishedFingerprint = fingerprint.String
	} else if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	ruleRows, err := tx.QueryContext(ctx, reviewRuleSelect+` WHERE r.household_id=? ORDER BY r.id`, householdID.String())
	if err != nil {
		return empty, err
	}
	for ruleRows.Next() {
		item, err := scanReviewRule(ruleRows)
		if err != nil {
			ruleRows.Close()
			return empty, err
		}
		result.Rules = append(result.Rules, item)
	}
	err = ruleRows.Err()
	ruleRows.Close()
	if err != nil {
		return empty, err
	}
	result.ReusableDailyEvidence = make(map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence)
	if result.Control.ProjectionStatus == "complete" && result.Control.PublishedFingerprint == result.HistoricalFingerprint {
		for _, rule := range result.Rules {
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
			rows, err := tx.QueryContext(ctx, `WITH ranked AS (
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
		WHERE d.slot_rank<=? ORDER BY d.instrument_id,d.on_date`, householdID.String(), policy, timezoneVersion, householdID.String(), policy, timezoneVersion, string(rule.ID), string(rule.ID), rule.CurrentConditionVersion, timezoneVersion, width)
			if err != nil {
				return empty, err
			}
			for rows.Next() {
				var instrumentID, on, inputFingerprint, inputJSON, coverageJSON string
				var resultJSON, resultFingerprint sql.NullString
				var priorConsecutiveCount int
				if err := rows.Scan(&instrumentID, &on, &inputFingerprint, &inputJSON, &coverageJSON, &resultJSON, &resultFingerprint, &priorConsecutiveCount); err != nil {
					rows.Close()
					return empty, err
				}
				id, err := domain.ParseInstrumentID(instrumentID)
				if err != nil {
					rows.Close()
					return empty, err
				}
				var item domain.ReviewReusableDailyEvidence
				if err := json.Unmarshal([]byte(inputJSON), &item.Input); err != nil {
					rows.Close()
					return empty, err
				}
				if err := json.Unmarshal([]byte(coverageJSON), &item.CoverageMissing); err != nil {
					rows.Close()
					return empty, err
				}
				if item.Input.On != on || item.Input.InstrumentID != id {
					rows.Close()
					return empty, fmt.Errorf("invalid cached investment review input for %s on %s", instrumentID, on)
				}
				item.InputFingerprint = inputFingerprint
				item.PriorConsecutiveCount = priorConsecutiveCount
				if resultJSON.Valid && resultFingerprint.Valid && resultFingerprint.String == inputFingerprint {
					if err := json.Unmarshal([]byte(resultJSON.String), &item.Result); err != nil {
						rows.Close()
						return empty, err
					}
					item.HasResult = item.Result.On == on
				}
				key := domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: id, TimezoneVersion: timezoneVersion, On: on}
				result.ReusableDailyEvidence[key] = item
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return empty, err
			}
			rows.Close()
		}
	}
	suppressionRows, err := tx.QueryContext(ctx, reviewSuppressionSelect+` WHERE household_id=? ORDER BY id`, householdID.String())
	if err != nil {
		return empty, err
	}
	for suppressionRows.Next() {
		item, err := scanReviewSuppression(suppressionRows)
		if err != nil {
			suppressionRows.Close()
			return empty, err
		}
		result.Suppressions = append(result.Suppressions, item)
	}
	err = suppressionRows.Err()
	suppressionRows.Close()
	if err != nil {
		return empty, err
	}
	episodeRows, err := tx.QueryContext(ctx, `SELECT e.id,e.rule_id,e.instrument_id,e.position_epoch_id,e.opened_condition_version,e.first_hit_on,e.triggered_on,e.first_detected_at,e.acknowledged_at,e.revision FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id WHERE r.household_id=? AND e.resolved_at IS NULL ORDER BY e.rule_id,e.instrument_id,e.position_epoch_id`, householdID.String())
	if err != nil {
		return empty, err
	}
	for episodeRows.Next() {
		var item domain.ReviewStoredEpisode
		var id, ruleID, instrumentID, detected string
		var acknowledged sql.NullString
		if err := episodeRows.Scan(&id, &ruleID, &instrumentID, &item.PositionEpochID, &item.OpenedConditionVersion, &item.FirstHitOn, &item.TriggeredOn, &detected, &acknowledged, &item.Revision); err != nil {
			episodeRows.Close()
			return empty, err
		}
		if item.ID, err = domain.ParseReviewEpisodeID(id); err != nil {
			episodeRows.Close()
			return empty, err
		}
		if item.RuleID, err = domain.ParseReviewRuleID(ruleID); err != nil {
			episodeRows.Close()
			return empty, err
		}
		if item.InstrumentID, err = domain.ParseInstrumentID(instrumentID); err != nil {
			episodeRows.Close()
			return empty, err
		}
		if item.FirstDetectedAt, err = time.Parse(time.RFC3339Nano, detected); err != nil {
			episodeRows.Close()
			return empty, err
		}
		if acknowledged.Valid {
			at, err := time.Parse(time.RFC3339Nano, acknowledged.String)
			if err != nil {
				episodeRows.Close()
				return empty, err
			}
			item.AcknowledgedAt = &at
		}
		result.OpenEpisodes = append(result.OpenEpisodes, item)
	}
	err = episodeRows.Err()
	episodeRows.Close()
	if err != nil {
		return empty, err
	}
	stateRows, err := tx.QueryContext(ctx, `SELECT DISTINCT s.rule_id,s.instrument_id FROM investment_review_evaluation_state s JOIN investment_review_rules r ON r.id=s.rule_id WHERE r.household_id=? ORDER BY s.rule_id,s.instrument_id`, householdID.String())
	if err != nil {
		return empty, err
	}
	for stateRows.Next() {
		var ruleID, instrumentID string
		if err := stateRows.Scan(&ruleID, &instrumentID); err != nil {
			stateRows.Close()
			return empty, err
		}
		result.EvaluatedInstruments[domain.ReviewRuleID(ruleID)] = append(result.EvaluatedInstruments[domain.ReviewRuleID(ruleID)], domain.InstrumentID(instrumentID))
	}
	err = stateRows.Err()
	stateRows.Close()
	if err != nil {
		return empty, err
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return result, nil
}
