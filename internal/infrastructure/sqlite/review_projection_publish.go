package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// PublishReviewProjection is the only writer of daily review projections. A
// source-token check in this same transaction rejects a financial edit that
// happened after the read snapshot, including deletions and withdrawals.
func (r *Repository) PublishReviewProjection(ctx context.Context, publication domain.ReviewPublication, now time.Time) error {
	capture := publication.Capture
	if capture.HouseholdID == "" || capture.SourceCutoff.IsZero() || capture.HistoricalFingerprint == "" || publication.At.IsZero() {
		return &domain.Error{Code: domain.ErrValidation, Field: "publication", Message: "a complete review capture is required"}
	}
	if err := validateReviewPublicationTime(capture, publication.Rules, now); err != nil {
		return err
	}
	tx, err := r.database.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	actual, err := loadReviewHistoricalBatchQuery(ctx, tx, capture.HouseholdID, capture.SourceCutoff)
	if err != nil {
		return err
	}
	actualFingerprint, err := domain.ReviewHistoricalFingerprint(actual)
	if err != nil {
		return err
	}
	if actualFingerprint != capture.HistoricalFingerprint {
		return reviewRuleConflict("financial evidence changed during review calculation")
	}
	control, found, err := reviewControlInTx(ctx, tx, capture.HouseholdID)
	if err != nil {
		return err
	}
	if found && control != capture.Control || !found && capture.Control != (domain.ReviewProjectionControl{Enabled: true, ConfigRevision: 1, ProjectionStatus: "pending"}) {
		return reviewRuleConflict("review configuration or handling changed during calculation")
	}
	if err := verifyReviewOpenEpisodes(ctx, tx, capture); err != nil {
		return err
	}
	byRule := make(map[domain.ReviewRuleID]domain.ReviewRule, len(capture.Rules))
	for _, rule := range capture.Rules {
		byRule[rule.ID] = rule
	}
	for _, item := range publication.Rules {
		rule, ok := byRule[item.RuleID]
		if !ok || rule.RowRevision != item.RuleRevision || rule.CurrentConditionVersion != item.ConditionVersion || item.InstrumentID == "" || item.TimezoneVersion != rule.Spec.Schedule.Timezone+"/"+rule.Spec.Schedule.CutoffVersion {
			return reviewRuleConflict("staged review rule changed")
		}
		var revision, condition int
		if err := tx.QueryRowContext(ctx, `SELECT row_revision,current_condition_version FROM investment_review_rules WHERE id=? AND household_id=?`, string(item.RuleID), capture.HouseholdID.String()).Scan(&revision, &condition); err != nil {
			return err
		}
		if revision != item.RuleRevision || condition != item.ConditionVersion {
			return reviewRuleConflict("review rule changed during calculation")
		}
	}
	if !found {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_control(household_id) VALUES(?)`, capture.HouseholdID.String()); err != nil {
			return err
		}
	}
	for _, item := range publication.Rules {
		if err := publishReviewRule(ctx, tx, capture, item, publication.At); err != nil {
			return err
		}
	}
	publishedOn := control.PublishedOn
	for _, item := range publication.Rules {
		if item.LatestClosedOn > publishedOn {
			publishedOn = item.LatestClosedOn
		}
	}
	status := "complete"
	if control.ProjectionStatus == "needs_repair" {
		status = "needs_repair"
	}
	generation := control.InputGeneration
	if control.PublishedFingerprint != actualFingerprint {
		generation++
	}
	if _, err := tx.ExecContext(ctx, `UPDATE investment_review_control SET input_generation=?,projection_status=?,published_on=?,published_input_fingerprint=? WHERE household_id=? AND (input_generation<>? OR projection_status<>? OR published_on IS NOT ? OR published_input_fingerprint IS NOT ?)`, generation, status, nullableEmpty(publishedOn), actualFingerprint, capture.HouseholdID.String(), generation, status, nullableEmpty(publishedOn), actualFingerprint); err != nil {
		return err
	}
	return tx.Commit()
}

func validateReviewPublicationTime(capture domain.ReviewProjectionCapture, rules []domain.ReviewRulePublication, now time.Time) error {
	if now.IsZero() {
		return &domain.Error{Code: domain.ErrValidation, Field: "now", Message: "a publication time is required"}
	}
	byRule := make(map[domain.ReviewRuleID]domain.ReviewRule, len(capture.Rules))
	for _, rule := range capture.Rules {
		byRule[rule.ID] = rule
	}
	for _, item := range rules {
		rule, exists := byRule[item.RuleID]
		if !exists {
			return reviewRuleConflict("review rule disappeared")
		}
		latest, err := domain.ReviewLatestClosedDay(now, rule.Spec.Schedule.Timezone)
		if err != nil {
			return err
		}
		if latest != item.LatestClosedOn {
			return reviewRuleConflict("review day boundary advanced during calculation")
		}
	}
	return nil
}

func reviewControlInTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID) (domain.ReviewProjectionControl, bool, error) {
	result := domain.ReviewProjectionControl{Enabled: true, ConfigRevision: 1, ProjectionStatus: "pending"}
	var enabled int
	var published, fingerprint sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT enabled,config_revision,input_generation,review_state_revision,projection_status,published_on,published_input_fingerprint FROM investment_review_control WHERE household_id=?`, householdID.String()).Scan(&enabled, &result.ConfigRevision, &result.InputGeneration, &result.ReviewStateRevision, &result.ProjectionStatus, &published, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return result, false, nil
	}
	if err != nil {
		return result, false, err
	}
	result.Enabled, result.PublishedOn, result.PublishedFingerprint = enabled != 0, published.String, fingerprint.String
	return result, true, nil
}

func verifyReviewOpenEpisodes(ctx context.Context, tx *sql.Tx, capture domain.ReviewProjectionCapture) error {
	expected := make(map[string]int, len(capture.OpenEpisodes))
	for _, episode := range capture.OpenEpisodes {
		expected[string(episode.ID)] = episode.Revision
	}
	rows, err := tx.QueryContext(ctx, `SELECT e.id,e.revision FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id WHERE r.household_id=? AND e.resolved_at IS NULL`, capture.HouseholdID.String())
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var revision int
		if err := rows.Scan(&id, &revision); err != nil {
			rows.Close()
			return err
		}
		if expected[id] != revision {
			rows.Close()
			return reviewRuleConflict("review episode or acknowledgement changed")
		}
		delete(expected, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(expected) != 0 {
		return reviewRuleConflict("review episode changed")
	}
	return nil
}

func publishReviewRule(ctx context.Context, tx *sql.Tx, capture domain.ReviewProjectionCapture, item domain.ReviewRulePublication, at time.Time) error {
	if item.TargetExcluded {
		if len(item.Days) != 0 || len(item.Epochs) != 0 || item.CurrentEpisode != nil || item.OpenedEpisode {
			return &domain.Error{Code: domain.ErrValidation, Field: "target", Message: "an excluded target cannot publish daily evidence or an open episode"}
		}
		if err := publishReviewEpisodes(ctx, tx, capture, item, at); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM investment_review_evaluation_state WHERE rule_id=? AND instrument_id=?`, string(item.RuleID), item.InstrumentID.String())
		return err
	}
	for _, epoch := range item.Epochs {
		if epoch.InstrumentID != item.InstrumentID {
			return &domain.Error{Code: domain.ErrValidation, Field: "epoch", Message: "epoch instrument mismatch"}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO investment_review_position_epochs(id,household_id,instrument_id,opened_at,opening_activity_id,closed_at,closing_activity_id,start_unknown,source_fingerprint) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET closed_at=excluded.closed_at,closing_activity_id=excluded.closing_activity_id,source_fingerprint=excluded.source_fingerprint WHERE closed_at IS NOT excluded.closed_at OR closing_activity_id IS NOT excluded.closing_activity_id OR source_fingerprint<>excluded.source_fingerprint`, epoch.ID, capture.HouseholdID.String(), item.InstrumentID.String(), nullableTimePtr(epoch.OpenedAt), nullableID(epoch.OpeningID), nullableTimePtr(epoch.ClosedAt), nullableID(epoch.ClosingID), boolValue(epoch.StartUnknown), capture.HistoricalFingerprint)
		if err != nil {
			return err
		}
	}
	atText := at.UTC().Format(time.RFC3339Nano)
	inputPolicy := "rule:" + string(item.RuleID) + "/v:" + strconv.Itoa(item.ConditionVersion)
	for _, day := range item.Days {
		if err := ctx.Err(); err != nil {
			return err
		}
		if day.On != day.Input.On || day.Result.On != day.On || day.Input.InstrumentID != item.InstrumentID {
			return &domain.Error{Code: domain.ErrValidation, Field: "day", Message: "review date or instrument mismatch"}
		}
		inputJSON, err := json.Marshal(day.Input)
		if err != nil {
			return err
		}
		coverageJSON, err := json.Marshal(day.CoverageMissing)
		if err != nil {
			return err
		}
		var kind, currency any
		if day.Input.InstrumentType != nil {
			kind = string(*day.Input.InstrumentType)
		}
		if day.Input.NativeCurrency != nil {
			currency = day.Input.NativeCurrency.String()
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO investment_review_daily_inputs(household_id,instrument_id,timezone_version,on_date,input_policy,quantity,native_currency,instrument_type,amount_exact,cost_exact,evidence_json,coverage_json,input_fingerprint,computed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(household_id,instrument_id,timezone_version,on_date,input_policy) DO UPDATE SET quantity=excluded.quantity,native_currency=excluded.native_currency,instrument_type=excluded.instrument_type,amount_exact=excluded.amount_exact,cost_exact=excluded.cost_exact,evidence_json=excluded.evidence_json,coverage_json=excluded.coverage_json,input_fingerprint=excluded.input_fingerprint,computed_at=excluded.computed_at WHERE input_fingerprint<>excluded.input_fingerprint`, capture.HouseholdID.String(), item.InstrumentID.String(), item.TimezoneVersion, day.On, inputPolicy, nullableReviewPointer(day.Input.Quantity), currency, kind, nullableReviewPointer(day.Input.AmountExact), nullableReviewPointer(day.Input.CostExact), string(inputJSON), string(coverageJSON), day.InputFingerprint, atText)
		if err != nil {
			return err
		}
		resultJSON, err := json.Marshal(day.Result)
		if err != nil {
			return err
		}
		truth := reviewTruthSQL(day.Result.Truth)
		_, err = tx.ExecContext(ctx, `INSERT INTO investment_review_daily_results(rule_id,condition_version,instrument_id,timezone_version,on_date,predicate_truth,reason,evidence_json,input_fingerprint,evaluated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(rule_id,condition_version,instrument_id,timezone_version,on_date) DO UPDATE SET predicate_truth=excluded.predicate_truth,reason=excluded.reason,evidence_json=excluded.evidence_json,input_fingerprint=excluded.input_fingerprint,revised_at=excluded.evaluated_at WHERE predicate_truth<>excluded.predicate_truth OR input_fingerprint<>excluded.input_fingerprint OR evidence_json<>excluded.evidence_json`, string(item.RuleID), item.ConditionVersion, item.InstrumentID.String(), item.TimezoneVersion, day.On, truth, nullableEmpty(strings.Join(day.CoverageMissing, ",")), string(resultJSON), day.InputFingerprint, atText)
		if err != nil {
			return err
		}
	}
	if err := publishReviewEpisodes(ctx, tx, capture, item, at); err != nil {
		return err
	}
	return publishReviewEvaluationState(ctx, tx, item)
}

func nullableReviewPointer(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func reviewTruthSQL(value domain.ReviewTruth) string {
	if value == domain.ReviewTrue || value == domain.ReviewFalse {
		return string(value)
	}
	return "unknown"
}

func reviewEvidenceFingerprint(value any) (string, string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(raw)
	return string(raw), hex.EncodeToString(digest[:]), nil
}

func publishReviewEpisodes(ctx context.Context, tx *sql.Tx, capture domain.ReviewProjectionCapture, item domain.ReviewRulePublication, at time.Time) error {
	expected := make(map[domain.ReviewEpisodeID]domain.ReviewStoredEpisode, len(capture.OpenEpisodes))
	for _, episode := range capture.OpenEpisodes {
		expected[episode.ID] = episode
	}
	atText := at.UTC().Format(time.RFC3339Nano)
	for _, closed := range item.ClosedEpisodes {
		stored, exists := expected[domain.ReviewEpisodeID(closed.ID)]
		if !exists || stored.RuleID != item.RuleID || stored.InstrumentID != item.InstrumentID || closed.ResolvedAt == nil || closed.ResolvedReason == "" {
			return reviewRuleConflict("invalid review episode resolution")
		}
		changed, err := tx.ExecContext(ctx, `UPDATE investment_review_episodes SET resolved_at=?,resolved_reason=?,revision=revision+1 WHERE id=? AND revision=? AND resolved_at IS NULL`, closed.ResolvedAt.UTC().Format(time.RFC3339Nano), closed.ResolvedReason, closed.ID, stored.Revision)
		if err != nil {
			return err
		}
		count, err := changed.RowsAffected()
		if err != nil || count != 1 {
			return reviewRuleConflict("review episode changed during resolution")
		}
		payload, _, err := reviewEvidenceFingerprint(closed)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_episode_events(id,episode_id,event_key,kind,payload_json,created_at) VALUES(?,?,?,?,?,?)`, string(domain.NewReviewEpisodeID()), closed.ID, "close/"+closed.ID, "resolved", payload, atText); err != nil {
			return err
		}
	}
	if item.CurrentEpisode == nil {
		return nil
	}
	current := item.CurrentEpisode
	if item.CurrentEpochID == "" {
		return reviewRuleConflict("open review episode has no current position epoch")
	}
	evidence := struct {
		Window       domain.ReviewPolicyWindow
		LatestResult *domain.ReviewDailyResult
		Coverage     []string
	}{Window: item.LatestWindow, Coverage: item.LatestCoverage}
	if len(item.Days) != 0 {
		evidence.LatestResult = &item.Days[len(item.Days)-1].Result
	}
	payload, fingerprint, err := reviewEvidenceFingerprint(evidence)
	if err != nil {
		return err
	}
	firstHit := ""
	for _, day := range item.Days {
		if day.On >= item.LatestWindow.WindowStart && day.On <= item.LatestClosedOn && day.Result.Truth == domain.ReviewTrue {
			firstHit = day.On
			break
		}
	}
	if firstHit == "" {
		firstHit = current.TriggeredOn // a retained episode may be U today
	}
	if item.OpenedEpisode {
		if _, exists := expected[domain.ReviewEpisodeID(current.ID)]; exists || current.AcknowledgedAt != nil || item.LatestWindow.Truth != domain.ReviewTrue {
			return reviewRuleConflict("invalid new review episode")
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO investment_review_episodes(id,rule_id,instrument_id,position_epoch_id,opened_condition_version,first_hit_on,triggered_on,first_detected_at,window_start,window_end,latest_closed_on,hit_count,false_count,unknown_count,consecutive_count,evidence_json,evidence_fingerprint,revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1)`, current.ID, string(item.RuleID), item.InstrumentID.String(), item.CurrentEpochID, item.ConditionVersion, firstHit, current.TriggeredOn, current.FirstDetectedAt.UTC().Format(time.RFC3339Nano), item.LatestWindow.WindowStart, item.LatestWindow.WindowEnd, item.LatestClosedOn, item.LatestWindow.HitCount, item.LatestWindow.FalseCount, item.LatestWindow.UnknownCount, item.LatestWindow.ConsecutiveCount, payload, fingerprint)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_episode_events(id,episode_id,event_key,kind,payload_json,created_at) VALUES(?,?,?,?,?,?)`, string(domain.NewReviewEpisodeID()), current.ID, "open/"+current.ID, "opened", payload, atText); err != nil {
			return err
		}
		return nil
	}
	stored, exists := expected[domain.ReviewEpisodeID(current.ID)]
	if !exists || stored.PositionEpochID != item.CurrentEpochID || stored.RuleID != item.RuleID || stored.InstrumentID != item.InstrumentID {
		return reviewRuleConflict("review episode identity changed")
	}
	var previousFingerprint, previousFirstHit, previousLatest string
	if err := tx.QueryRowContext(ctx, `SELECT evidence_fingerprint,first_hit_on,latest_closed_on FROM investment_review_episodes WHERE id=? AND revision=? AND resolved_at IS NULL`, current.ID, stored.Revision).Scan(&previousFingerprint, &previousFirstHit, &previousLatest); err != nil {
		return err
	}
	if previousFingerprint == fingerprint && previousFirstHit == firstHit && previousLatest == item.LatestClosedOn {
		return nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE investment_review_episodes SET first_hit_on=?,window_start=?,window_end=?,latest_closed_on=?,hit_count=?,false_count=?,unknown_count=?,consecutive_count=?,evidence_json=?,evidence_fingerprint=?,revision=revision+1 WHERE id=? AND revision=? AND resolved_at IS NULL`, firstHit, item.LatestWindow.WindowStart, item.LatestWindow.WindowEnd, item.LatestClosedOn, item.LatestWindow.HitCount, item.LatestWindow.FalseCount, item.LatestWindow.UnknownCount, item.LatestWindow.ConsecutiveCount, payload, fingerprint, current.ID, stored.Revision)
	return err
}

func publishReviewEvaluationState(ctx context.Context, tx *sql.Tx, item domain.ReviewRulePublication) error {
	coverage, err := json.Marshal(item.LatestCoverage)
	if err != nil {
		return err
	}
	epochIDs := make([]string, 0, len(item.Epochs)+1)
	for _, epoch := range item.Epochs {
		epochIDs = append(epochIDs, epoch.ID)
	}
	if item.CurrentEpochID != "" && !slices.Contains(epochIDs, item.CurrentEpochID) {
		epochIDs = append(epochIDs, item.CurrentEpochID)
	}
	for _, epochID := range epochIDs {
		current := epochID == item.CurrentEpochID
		state := "not_applicable"
		var episodeID any
		window := item.LatestWindow
		if current {
			switch window.Truth {
			case domain.ReviewTrue:
				state = "triggered"
			case domain.ReviewUnknown:
				state = "unknown"
			default:
				state = "healthy"
				if window.HitCount > 0 {
					state = "confirming"
				}
			}
			if !item.PositionKnown {
				state = "unknown"
			}
			if item.CurrentEpisode != nil {
				episodeID = item.CurrentEpisode.ID
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO investment_review_evaluation_state(rule_id,instrument_id,position_epoch_id,latest_closed_on,policy_state,window_start,window_end,hit_count,false_count,unknown_count,consecutive_count,episode_id,coverage_json,publication_revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,1) ON CONFLICT(rule_id,instrument_id,position_epoch_id) DO UPDATE SET latest_closed_on=excluded.latest_closed_on,policy_state=excluded.policy_state,window_start=excluded.window_start,window_end=excluded.window_end,hit_count=excluded.hit_count,false_count=excluded.false_count,unknown_count=excluded.unknown_count,consecutive_count=excluded.consecutive_count,episode_id=excluded.episode_id,coverage_json=excluded.coverage_json,publication_revision=investment_review_evaluation_state.publication_revision+1 WHERE latest_closed_on<>excluded.latest_closed_on OR policy_state<>excluded.policy_state OR window_start<>excluded.window_start OR window_end<>excluded.window_end OR hit_count<>excluded.hit_count OR false_count<>excluded.false_count OR unknown_count<>excluded.unknown_count OR consecutive_count<>excluded.consecutive_count OR episode_id IS NOT excluded.episode_id OR coverage_json<>excluded.coverage_json`, string(item.RuleID), item.InstrumentID.String(), epochID, item.LatestClosedOn, state, window.WindowStart, window.WindowEnd, window.HitCount, window.FalseCount, window.UnknownCount, window.ConsecutiveCount, episodeID, string(coverage))
		if err != nil {
			return err
		}
	}
	return nil
}

func reviewPublicationConflict(format string, values ...any) error {
	return reviewRuleConflict(fmt.Sprintf(format, values...))
}
