package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type reviewCoverageCursor struct{ Revision, FilterHash, AfterID string }

// ListReviewCoverageV2 pages current positive holdings by instrument ID and
// retrieves each batch's published rule results in one query. It never
// evaluates a day or mutates a projection from a read path.
func (r *Repository) ListReviewCoverageV2(ctx context.Context, householdID domain.HouseholdID, filter domain.ReviewCoverageFilter, cursor string, pageSize int, now time.Time) (domain.ReviewCoveragePage, error) {
	empty := domain.ReviewCoveragePage{Items: []domain.ReviewCoverageGroup{}}
	if pageSize < 1 || pageSize > 100 || now.IsZero() {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "pageSize", Message: "use 1–100 groups"}
	}
	if filter.Status == "" {
		filter.Status = "all"
	}
	switch filter.Status {
	case "all", "triggered", "unknown", "confirming", "healthy", "paused", "uncomputed":
	default:
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "status", Message: "unsupported coverage filter"}
	}
	if filter.InstrumentType != "" {
		if _, err := domain.ParseInstrumentType(filter.InstrumentType); err != nil {
			return empty, err
		}
	}
	filter.Query = strings.TrimSpace(filter.Query)
	if len([]rune(filter.Query)) > 120 {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "query", Message: "search must have at most 120 characters"}
	}
	filterBytes, _ := jsoncodec.Marshal(filter)
	filterDigest := sha256.Sum256(filterBytes)
	filterHash := hex.EncodeToString(filterDigest[:])
	tx, release, connectionID, err := r.beginReviewStatusRead(ctx)
	if err != nil {
		return empty, err
	}
	defer release()
	defer tx.Rollback()
	control, _, err := reviewControlInTx(ctx, tx, householdID)
	if err != nil {
		return empty, err
	}
	if err := r.reviewTemporalReadStatus(ctx, tx, householdID, &control, now, connectionID); err != nil {
		return empty, err
	}
	stateBytes, _ := jsoncodec.Marshal(control)
	digest := sha256.Sum256(stateBytes)
	revision := hex.EncodeToString(digest[:])
	result := domain.ReviewCoveragePage{Items: []domain.ReviewCoverageGroup{}, ReportRevision: revision, ProjectionStatus: control.ProjectionStatus, PublishedOn: control.PublishedOn}
	afterID := ""
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid coverage cursor"}
		}
		var prior reviewCoverageCursor
		if jsoncodec.Unmarshal(raw, &prior) != nil || prior.Revision != revision || prior.FilterHash != filterHash {
			return empty, &domain.Error{Code: domain.ErrConflict, Field: "cursor", Message: "cursor_expired"}
		}
		if _, err := domain.ParseInstrumentID(prior.AfterID); err != nil {
			return empty, err
		}
		afterID = prior.AfterID
	}
	more := false
	for !more {
		ids, err := reviewCoverageCandidateIDs(ctx, tx, householdID, afterID, 100)
		if err != nil {
			return empty, err
		}
		if len(ids) == 0 {
			break
		}
		groups, err := reviewCoverageGroupsForIDs(ctx, tx, householdID, ids, control.Enabled)
		if err != nil {
			return empty, err
		}
		for _, id := range ids {
			group := groups[id]
			if filter.InstrumentType != "" && string(group.InstrumentType) != filter.InstrumentType || filter.Query != "" && !strings.Contains(strings.ToLower(group.DisplayName), strings.ToLower(filter.Query)) {
				afterID = id
				continue
			}
			matched := group.Status == filter.Status
			for _, rule := range group.Rules {
				matched = matched || rule.State == filter.Status
			}
			if filter.Status != "all" && !matched {
				afterID = id
				continue
			}
			if len(result.Items) == pageSize {
				more = true
				break
			}
			result.Items = append(result.Items, group)
			afterID = id
		}
		if len(ids) < 100 {
			break
		}
	}
	if more {
		raw, _ := jsoncodec.Marshal(reviewCoverageCursor{Revision: revision, FilterHash: filterHash, AfterID: afterID})
		result.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return result, nil
}

const reviewCoverageCandidateSQL = `SELECT i.id FROM instruments i WHERE i.household_id=? AND i.id>? AND EXISTS(SELECT 1 FROM holdings h WHERE h.instrument_id=i.id AND CAST(h.quantity AS NUMERIC)>0) ORDER BY i.id LIMIT ?`

func reviewCoverageCandidateIDs(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, afterID string, limit int) ([]string, error) {
	rows, err := tx.QueryContext(ctx, reviewCoverageCandidateSQL, householdID.String(), afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func reviewCoverageGroupsForIDs(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, ids []string, enabled bool) (map[string]domain.ReviewCoverageGroup, error) {
	placeholders, args := sqlInArgs(ids)
	query := `SELECT i.id,i.name,i.instrument_type,r.id,r.name,r.enabled,s.policy_state,s.latest_closed_on,s.window_start,s.window_end,s.hit_count,s.false_count,s.unknown_count,s.consecutive_count,s.coverage_json FROM instruments i LEFT JOIN (investment_review_evaluation_state s JOIN investment_review_rules r ON r.id=s.rule_id AND r.household_id=? AND r.archived_at IS NULL AND s.policy_state<>'not_applicable') ON s.instrument_id=i.id WHERE i.id IN (` + placeholders + `) ORDER BY i.id,r.id`
	parameters := append([]any{householdID.String()}, args...)
	rows, err := tx.QueryContext(ctx, query, parameters...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := map[string]domain.ReviewCoverageGroup{}
	for rows.Next() {
		var id, name, kind string
		var ruleID, ruleName, policyState, latest, windowStart, windowEnd, coverage sql.NullString
		var ruleEnabled, hits, falseCount, unknownCount, streak sql.NullInt64
		if err := rows.Scan(&id, &name, &kind, &ruleID, &ruleName, &ruleEnabled, &policyState, &latest, &windowStart, &windowEnd, &hits, &falseCount, &unknownCount, &streak, &coverage); err != nil {
			return nil, err
		}
		instrumentID, err := domain.ParseInstrumentID(id)
		if err != nil {
			return nil, err
		}
		group, ok := groups[id]
		if !ok {
			group = domain.ReviewCoverageGroup{InstrumentID: instrumentID, DisplayName: name, InstrumentType: domain.InstrumentType(kind), Status: "uncomputed", Rules: []domain.ReviewCoverageRule{}}
		}
		if ruleID.Valid {
			var missing []string
			if err := jsoncodec.Unmarshal([]byte(coverage.String), &missing); err != nil {
				return nil, err
			}
			truth := domain.ReviewFalse
			if policyState.String == "triggered" {
				truth = domain.ReviewTrue
			} else if policyState.String == "unknown" {
				truth = domain.ReviewUnknown
			}
			state := policyState.String
			if !enabled || ruleEnabled.Int64 == 0 {
				state = "paused"
			}
			group.Rules = append(group.Rules, domain.ReviewCoverageRule{RuleID: domain.ReviewRuleID(ruleID.String), Name: ruleName.String, State: state, LatestClosedOn: latest.String, Window: domain.ReviewPolicyWindow{Truth: truth, WindowStart: windowStart.String, WindowEnd: windowEnd.String, HitCount: int(hits.Int64), FalseCount: int(falseCount.Int64), UnknownCount: int(unknownCount.Int64), ConsecutiveCount: int(streak.Int64)}, Missing: missing})
			if reviewCoverageRank(state) < reviewCoverageRank(group.Status) {
				group.Status = state
			}
		}
		groups[id] = group
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, ok := groups[id]; !ok {
			return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "coverage candidate disappeared within snapshot"}
		}
	}
	return groups, nil
}

func reviewCoverageRank(state string) int {
	switch strings.ToLower(state) {
	case "triggered":
		return 0
	case "unknown":
		return 1
	case "confirming":
		return 2
	case "healthy":
		return 3
	case "paused":
		return 4
	default:
		return 5
	}
}
