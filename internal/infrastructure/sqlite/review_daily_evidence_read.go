package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type reviewDailyEvidenceCursor struct {
	Revision string `json:"revision"`
	BeforeOn string `json:"beforeOn"`
}

// ListReviewDailyEvidenceV2 reads a bounded slice of the current episode's
// persisted votes under the condition version proven by its latest evidence. Historical episodes
// need their own version-aware event view before they can be exposed here.
func (r *Repository) ListReviewDailyEvidenceV2(ctx context.Context, householdID domain.HouseholdID, episodeID domain.ReviewEpisodeID, cursor string, pageSize int) (domain.ReviewDailyEvidencePage, error) {
	empty := domain.ReviewDailyEvidencePage{Items: []domain.ReviewDailyResult{}}
	if pageSize < 1 || pageSize > 100 {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "pageSize", Message: "use 1–100 days"}
	}
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	control, _, err := reviewControlInTx(ctx, tx, householdID)
	if err != nil {
		return empty, err
	}
	var ruleID, instrumentID, triggeredOn, latest, episodeEvidenceJSON string
	var episodeRevision int
	err = tx.QueryRowContext(ctx, `SELECT e.rule_id,e.instrument_id,e.triggered_on,e.latest_closed_on,e.revision,e.evidence_json
	 FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id
	 WHERE e.id=? AND e.resolved_at IS NULL AND r.household_id=? AND r.evaluator=?`, string(episodeID), householdID.String(), domain.ReviewEvaluator).Scan(&ruleID, &instrumentID, &triggeredOn, &latest, &episodeRevision, &episodeEvidenceJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, &domain.Error{Code: domain.ErrNotFound, Field: "episodeId", Message: "review episode not found"}
	}
	if err != nil {
		return empty, err
	}
	var storedEvidence reviewEpisodeEvidence
	if err := jsoncodec.Unmarshal([]byte(episodeEvidenceJSON), &storedEvidence); err != nil {
		return empty, &domain.Error{Code: domain.ErrIntegrity, Message: "stored review episode evidence is invalid"}
	}
	spec, conditionVersion, proven, err := reviewEvidenceVersion(ctx, tx, ruleID, instrumentID, storedEvidence.LatestResult)
	if err != nil {
		return empty, err
	}
	if !proven {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "review evidence condition version unavailable"}
	}
	timezoneVersion := spec.Schedule.Timezone + "/" + spec.Schedule.CutoffVersion
	// The latest rolling window advances as the episode continues. Evidence
	// pagination instead starts at the window that originally caused this
	// episode to open, including its false and unknown votes.
	firstOn := storedEvidence.TriggerWindow.WindowStart
	if firstOn == "" {
		// Compatibility for an episode created before trigger windows were
		// stored. A normal reconciliation rebuilds its projection from history.
		width := reviewDailyEvidenceWindowWidth(spec.TriggerPolicy)
		triggerDay, err := time.Parse(time.DateOnly, triggeredOn)
		if err != nil {
			return empty, &domain.Error{Code: domain.ErrIntegrity, Message: "stored review trigger date is invalid"}
		}
		firstOn = triggerDay.AddDate(0, 0, 1-width).Format(time.DateOnly)
	}
	if _, err := time.Parse(time.DateOnly, firstOn); err != nil || firstOn > triggeredOn || firstOn > latest {
		return empty, &domain.Error{Code: domain.ErrIntegrity, Message: "stored review evidence range is invalid"}
	}
	revisionBytes, err := jsoncodec.Marshal(struct {
		Control          domain.ReviewProjectionControl
		EpisodeID        domain.ReviewEpisodeID
		EpisodeRevision  int
		ConditionVersion int
		FirstOn          string
	}{control, episodeID, episodeRevision, conditionVersion, firstOn})
	if err != nil {
		return empty, err
	}
	digest := sha256.Sum256(revisionBytes)
	revision := hex.EncodeToString(digest[:])
	before := "9999-12-31"
	if cursor != "" {
		if len(cursor) > 512 {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid evidence cursor"}
		}
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid evidence cursor"}
		}
		var previous reviewDailyEvidenceCursor
		if jsoncodec.Unmarshal(raw, &previous) != nil || previous.Revision != revision {
			return empty, &domain.Error{Code: domain.ErrConflict, Field: "cursor", Message: "cursor_expired"}
		}
		if _, err := time.Parse(time.DateOnly, previous.BeforeOn); err != nil || previous.BeforeOn <= firstOn || previous.BeforeOn > latest {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid evidence date"}
		}
		before = previous.BeforeOn
	}
	rows, err := tx.QueryContext(ctx, `SELECT on_date,evidence_json FROM investment_review_daily_results
	 WHERE rule_id=? AND condition_version=? AND instrument_id=? AND timezone_version=?
		 AND on_date>=? AND on_date<=? AND on_date<? ORDER BY on_date DESC LIMIT ?`, ruleID, conditionVersion, instrumentID, timezoneVersion, firstOn, latest, before, pageSize+1)
	if err != nil {
		return empty, err
	}
	items := make([]domain.ReviewDailyResult, 0, pageSize+1)
	for rows.Next() {
		var on, evidence string
		if err := rows.Scan(&on, &evidence); err != nil {
			rows.Close()
			return empty, err
		}
		var item domain.ReviewDailyResult
		if err := jsoncodec.Unmarshal([]byte(evidence), &item); err != nil || item.On != on {
			rows.Close()
			return empty, &domain.Error{Code: domain.ErrIntegrity, Message: "stored daily review evidence is invalid"}
		}
		item.Predicate = &spec.Predicate
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return empty, err
	}
	if err := rows.Close(); err != nil {
		return empty, err
	}
	result := domain.ReviewDailyEvidencePage{Items: items, EpisodeID: episodeID, ReportRevision: revision}
	if len(items) > pageSize {
		result.Items = items[:pageSize]
		encoded, _ := jsoncodec.Marshal(reviewDailyEvidenceCursor{Revision: revision, BeforeOn: result.Items[len(result.Items)-1].On})
		result.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return result, nil
}

func reviewDailyEvidenceWindowWidth(policy domain.ReviewTriggerPolicy) int {
	switch policy.Kind {
	case "consecutive":
		return policy.Count
	case "rolling":
		return policy.WindowDays
	default:
		return 1
	}
}
