package sqlite

import (
	"context"
	"database/sql"
	"errors"
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
	result := domain.ReviewProjectionCapture{HouseholdID: householdID, SourceCutoff: cutoff, Rules: []domain.ReviewRule{}, Suppressions: []domain.ReviewSuppression{}, OpenEpisodes: []domain.ReviewStoredEpisode{}}
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
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return result, nil
}
