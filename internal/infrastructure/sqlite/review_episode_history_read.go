package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type reviewHistoryCursor struct {
	Revision string `json:"revision"`
	BeforeOn string `json:"beforeOn"`
	BeforeID string `json:"beforeId"`
}

func reviewHistoryRevision(parts ...any) (string, error) {
	raw, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func decodeReviewHistoryCursor(raw, revision string) (reviewHistoryCursor, error) {
	var value reviewHistoryCursor
	if len(raw) > 512 {
		return value, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid review history cursor"}
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || json.Unmarshal(decoded, &value) != nil {
		return value, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid review history cursor"}
	}
	if value.Revision != revision {
		return value, &domain.Error{Code: domain.ErrConflict, Field: "cursor", Message: "cursor_expired"}
	}
	return value, nil
}

func encodeReviewHistoryCursor(cursor reviewHistoryCursor) string {
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// History is scoped to one stable rule ID so its existing recent-episode
// index supplies a bounded keyset page, including archived rules and closed
// episodes. Reads never reconcile or reinterpret the stored condition version.
func (r *Repository) ListReviewEpisodeHistoryV2(ctx context.Context, householdID domain.HouseholdID, ruleID domain.ReviewRuleID, cursor string, pageSize int) (domain.ReviewEpisodeHistoryPage, error) {
	empty := domain.ReviewEpisodeHistoryPage{Items: []domain.ReviewEpisodeHistoryItem{}, RuleID: ruleID}
	if pageSize < 1 || pageSize > 100 {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "pageSize", Message: "use 1–100 episodes"}
	}
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var ruleRevision int
	err = tx.QueryRowContext(ctx, `SELECT row_revision FROM investment_review_rules WHERE id=? AND household_id=? AND evaluator=?`, string(ruleID), householdID.String(), domain.ReviewEvaluator).Scan(&ruleRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, &domain.Error{Code: domain.ErrNotFound, Field: "ruleId", Message: "daily review rule not found"}
	}
	if err != nil {
		return empty, err
	}
	control, _, err := reviewControlInTx(ctx, tx, householdID)
	if err != nil {
		return empty, err
	}
	revision, err := reviewHistoryRevision(control, ruleID, ruleRevision)
	if err != nil {
		return empty, err
	}
	beforeOn, beforeID := "9999-12-31", "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if cursor != "" {
		value, err := decodeReviewHistoryCursor(cursor, revision)
		if err != nil {
			return empty, err
		}
		if _, err := time.Parse(time.DateOnly, value.BeforeOn); err != nil || value.BeforeOn >= beforeOn {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid history date"}
		}
		if _, err := domain.ParseReviewEpisodeID(value.BeforeID); err != nil {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid history episode"}
		}
		beforeOn, beforeID = value.BeforeOn, value.BeforeID
	}
	rows, err := tx.QueryContext(ctx, `SELECT e.id,r.name,e.instrument_id,i.name,e.position_epoch_id,e.opened_condition_version,e.first_hit_on,e.triggered_on,e.first_detected_at,e.latest_closed_on,e.window_start,e.window_end,e.hit_count,e.false_count,e.unknown_count,e.acknowledged_at,e.resolved_at,e.resolved_reason,e.revision
	 FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id JOIN instruments i ON i.id=e.instrument_id
	 WHERE e.rule_id=? AND (e.triggered_on<? OR (e.triggered_on=? AND e.id<?))
	 ORDER BY e.triggered_on DESC,e.id DESC LIMIT ?`, string(ruleID), beforeOn, beforeOn, beforeID, pageSize+1)
	if err != nil {
		return empty, err
	}
	items := make([]domain.ReviewEpisodeHistoryItem, 0, pageSize+1)
	for rows.Next() {
		item := domain.ReviewEpisodeHistoryItem{RuleID: ruleID}
		var ack, resolved, reason sql.NullString
		if err := rows.Scan(&item.EpisodeID, &item.RuleName, &item.InstrumentID, &item.InstrumentName, &item.PositionEpochID, &item.OpenedConditionVersion, &item.FirstHitOn, &item.TriggeredOn, &item.FirstDetectedAt, &item.LatestClosedOn, &item.WindowStart, &item.WindowEnd, &item.HitCount, &item.FalseCount, &item.UnknownCount, &ack, &resolved, &reason, &item.Revision); err != nil {
			rows.Close()
			return empty, err
		}
		item.AcknowledgedAt, item.ResolvedAt, item.ResolvedReason = ack.String, resolved.String, reason.String
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return empty, err
	}
	if err := rows.Close(); err != nil {
		return empty, err
	}
	result := domain.ReviewEpisodeHistoryPage{Items: items, ReportRevision: revision, RuleID: ruleID}
	if len(items) > pageSize {
		result.Items = items[:pageSize]
		last := result.Items[pageSize-1]
		result.NextCursor = encodeReviewHistoryCursor(reviewHistoryCursor{revision, last.TriggeredOn, string(last.EpisodeID)})
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return result, nil
}

// Event pages use the same immutable episode ID even after recovery, target
// removal or rule archive. The episode revision expires a cursor when an ack
// or corrected evidence changes the historical explanation.
func (r *Repository) ListReviewEpisodeEventsV2(ctx context.Context, householdID domain.HouseholdID, episodeID domain.ReviewEpisodeID, cursor string, pageSize int) (domain.ReviewEpisodeEventPage, error) {
	empty := domain.ReviewEpisodeEventPage{Items: []domain.ReviewEpisodeEvent{}, EpisodeID: episodeID}
	if pageSize < 1 || pageSize > 100 {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "pageSize", Message: "use 1–100 events"}
	}
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var episodeRevision int
	err = tx.QueryRowContext(ctx, `SELECT e.revision FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id WHERE e.id=? AND r.household_id=? AND r.evaluator=?`, string(episodeID), householdID.String(), domain.ReviewEvaluator).Scan(&episodeRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, &domain.Error{Code: domain.ErrNotFound, Field: "episodeId", Message: "daily review episode not found"}
	}
	if err != nil {
		return empty, err
	}
	control, _, err := reviewControlInTx(ctx, tx, householdID)
	if err != nil {
		return empty, err
	}
	revision, err := reviewHistoryRevision(control, episodeID, episodeRevision)
	if err != nil {
		return empty, err
	}
	beforeAt, beforeID := "9999-12-31T23:59:59Z", "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if cursor != "" {
		value, err := decodeReviewHistoryCursor(cursor, revision)
		if err != nil {
			return empty, err
		}
		if _, err := time.Parse(time.RFC3339Nano, value.BeforeOn); err != nil || value.BeforeOn >= beforeAt {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid event time"}
		}
		if _, err := domain.ParseReviewEpisodeID(value.BeforeID); err != nil {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid event ID"}
		}
		beforeAt, beforeID = value.BeforeOn, value.BeforeID
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,kind,payload_json,created_at FROM investment_review_episode_events WHERE episode_id=? AND (created_at<? OR (created_at=? AND id<?)) ORDER BY created_at DESC,id DESC LIMIT ?`, string(episodeID), beforeAt, beforeAt, beforeID, pageSize+1)
	if err != nil {
		return empty, err
	}
	items := make([]domain.ReviewEpisodeEvent, 0, pageSize+1)
	for rows.Next() {
		item := domain.ReviewEpisodeEvent{EpisodeID: episodeID}
		if err := rows.Scan(&item.ID, &item.Kind, &item.PayloadJSON, &item.CreatedAt); err != nil {
			rows.Close()
			return empty, err
		}
		if !json.Valid([]byte(item.PayloadJSON)) {
			rows.Close()
			return empty, &domain.Error{Code: domain.ErrIntegrity, Message: "stored review event is invalid"}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return empty, err
	}
	if err := rows.Close(); err != nil {
		return empty, err
	}
	result := domain.ReviewEpisodeEventPage{Items: items, ReportRevision: revision, EpisodeID: episodeID}
	if len(items) > pageSize {
		result.Items = items[:pageSize]
		last := result.Items[pageSize-1]
		result.NextCursor = encodeReviewHistoryCursor(reviewHistoryCursor{revision, last.CreatedAt, last.ID})
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return result, nil
}
