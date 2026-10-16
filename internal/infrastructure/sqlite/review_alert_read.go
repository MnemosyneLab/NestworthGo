package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type reviewAlertCursor struct {
	Revision   string `json:"revision"`
	FilterHash string `json:"filterHash"`
	AfterID    string `json:"afterId"`
}

func (r *Repository) GetReviewAlertV2(ctx context.Context, householdID domain.HouseholdID, episodeID domain.ReviewEpisodeID, now time.Time) (domain.ReviewAlertGroup, error) {
	var empty domain.ReviewAlertGroup
	if now.IsZero() {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "now", Message: "read time is required"}
	}
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var instrumentID string
	err = tx.QueryRowContext(ctx, `SELECT e.instrument_id FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id WHERE e.id=? AND r.household_id=? AND e.resolved_at IS NULL AND EXISTS(SELECT 1 FROM holdings h WHERE h.instrument_id=e.instrument_id AND CAST(h.quantity AS NUMERIC)>0)`, string(episodeID), householdID.String()).Scan(&instrumentID)
	if err == sql.ErrNoRows {
		return empty, &domain.Error{Code: domain.ErrNotFound, Field: "episodeId", Message: "current review episode not found"}
	}
	if err != nil {
		return empty, err
	}
	control, _, err := reviewControlInTx(ctx, tx, householdID)
	if err != nil {
		return empty, err
	}
	suppressions, err := reviewSuppressionsInTx(ctx, tx, householdID)
	if err != nil {
		return empty, err
	}
	byInstrument := map[domain.InstrumentID][]domain.ReviewSuppression{}
	for _, item := range suppressions {
		byInstrument[item.InstrumentID] = append(byInstrument[item.InstrumentID], item)
	}
	groups, err := reviewAlertGroupsForIDs(ctx, tx, householdID, []string{instrumentID}, byInstrument, control.Enabled, now)
	if err != nil {
		return empty, err
	}
	group, ok := groups[instrumentID]
	if !ok {
		return empty, &domain.Error{Code: domain.ErrNotFound, Field: "episodeId", Message: "current review episode not found"}
	}
	found := false
	for _, rule := range group.Rules {
		found = found || rule.EpisodeID == episodeID
	}
	if !found {
		return empty, &domain.Error{Code: domain.ErrNotFound, Field: "episodeId", Message: "current review episode not found"}
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return group, nil
}

// ListReviewAlertGroupsV2 is a pure snapshot read with a revision-bound
// cursor. It scans candidate instruments in bounded pages, then applies
// handling filters after resolving each suppression's own civil timezone.
func (r *Repository) ListReviewAlertGroupsV2(ctx context.Context, householdID domain.HouseholdID, filter domain.ReviewAlertFilter, cursor string, pageSize int, now time.Time) (domain.ReviewAlertPage, error) {
	empty := domain.ReviewAlertPage{Items: []domain.ReviewAlertGroup{}}
	if pageSize < 1 || pageSize > 100 || now.IsZero() {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "pageSize", Message: "use 1–100 groups and a read time"}
	}
	filter.Status = strings.TrimSpace(filter.Status)
	if filter.Status == "" {
		filter.Status = "all"
	}
	switch filter.Status {
	case "all", "unignored", "pending", "acknowledged", "ignored", "unknown", "paused":
	default:
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "status", Message: "unsupported review status filter"}
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
	filterBytes, err := json.Marshal(filter)
	if err != nil {
		return empty, err
	}
	filterDigest := sha256.Sum256(filterBytes)
	filterHash := hex.EncodeToString(filterDigest[:])
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	control, _, err := reviewControlInTx(ctx, tx, householdID)
	if err != nil {
		return empty, err
	}
	suppressions, err := reviewSuppressionsInTx(ctx, tx, householdID)
	if err != nil {
		return empty, err
	}
	// The report can change at a suppression's local midnight even without a
	// database write. Hash each temporary scope's civil date, not the current
	// minute, so a reader can page a stable report throughout the day.
	type suppressionDay struct{ ID, Today string }
	days := make([]suppressionDay, 0, len(suppressions))
	for _, item := range suppressions {
		if item.Mode != "until_date" {
			continue
		}
		location, err := time.LoadLocation(item.Timezone)
		if err != nil {
			return empty, err
		}
		days = append(days, suppressionDay{ID: item.ID, Today: now.In(location).Format(time.DateOnly)})
	}
	stateBytes, _ := json.Marshal(struct {
		Control domain.ReviewProjectionControl
		Days    []suppressionDay
	}{control, days})
	stateDigest := sha256.Sum256(stateBytes)
	revision := hex.EncodeToString(stateDigest[:])
	result := domain.ReviewAlertPage{Items: []domain.ReviewAlertGroup{}, ReportRevision: revision, ProjectionStatus: control.ProjectionStatus, PublishedOn: control.PublishedOn}
	afterID := ""
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid review cursor"}
		}
		var previous reviewAlertCursor
		if json.Unmarshal(decoded, &previous) != nil {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "invalid review cursor"}
		}
		if previous.Revision != revision || previous.FilterHash != filterHash {
			return empty, &domain.Error{Code: domain.ErrConflict, Field: "cursor", Message: "cursor_expired"}
		}
		if _, err := domain.ParseInstrumentID(previous.AfterID); err != nil {
			return empty, err
		}
		afterID = previous.AfterID
	}
	byInstrument := make(map[domain.InstrumentID][]domain.ReviewSuppression)
	for _, item := range suppressions {
		byInstrument[item.InstrumentID] = append(byInstrument[item.InstrumentID], item)
	}
	for len(result.Items) < pageSize {
		ids, err := reviewAlertCandidateIDs(ctx, tx, householdID, afterID, 100)
		if err != nil {
			return empty, err
		}
		if len(ids) == 0 {
			break
		}
		groups, err := reviewAlertGroupsForIDs(ctx, tx, householdID, ids, byInstrument, control.Enabled, now)
		if err != nil {
			return empty, err
		}
		for _, id := range ids {
			afterID = id
			group, exists := groups[id]
			if !exists || !reviewAlertMatchesFilter(group, filter) {
				continue
			}
			result.Items = append(result.Items, group)
			if len(result.Items) == pageSize {
				break
			}
		}
		if len(ids) < 100 || len(result.Items) == pageSize {
			break
		}
	}
	if len(result.Items) == pageSize {
		// A later candidate is not necessarily a later matching group. Look
		// ahead through bounded candidate batches so Next never opens an empty
		// filtered page.
		probeAfter, hasMore := afterID, false
		for !hasMore {
			ids, err := reviewAlertCandidateIDs(ctx, tx, householdID, probeAfter, 100)
			if err != nil {
				return empty, err
			}
			if len(ids) == 0 {
				break
			}
			groups, err := reviewAlertGroupsForIDs(ctx, tx, householdID, ids, byInstrument, control.Enabled, now)
			if err != nil {
				return empty, err
			}
			for _, id := range ids {
				probeAfter = id
				if group, ok := groups[id]; ok && reviewAlertMatchesFilter(group, filter) {
					hasMore = true
					break
				}
			}
			if len(ids) < 100 {
				break
			}
		}
		if hasMore {
			encoded, _ := json.Marshal(reviewAlertCursor{Revision: revision, FilterHash: filterHash, AfterID: afterID})
			result.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
		}
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return result, nil
}

func reviewAlertMatchesFilter(group domain.ReviewAlertGroup, filter domain.ReviewAlertFilter) bool {
	if filter.InstrumentType != "" && string(group.InstrumentType) != filter.InstrumentType || filter.Query != "" && !strings.Contains(strings.ToLower(group.DisplayName), strings.ToLower(filter.Query)) {
		return false
	}
	if filter.Status == "all" {
		return true
	}
	for _, rule := range group.Rules {
		if filter.Status == "unignored" {
			if rule.Status != "ignored" && rule.Status != "paused" {
				return true
			}
		} else if rule.Status == filter.Status {
			return true
		}
	}
	return false
}

const reviewAlertCandidateSQL = `SELECT DISTINCT i.id FROM investment_review_evaluation_state s JOIN investment_review_rules r ON r.id=s.rule_id JOIN investment_review_episodes e ON e.id=s.episode_id JOIN instruments i ON i.id=s.instrument_id WHERE r.household_id=? AND i.id>? AND e.resolved_at IS NULL AND EXISTS(SELECT 1 FROM holdings h WHERE h.instrument_id=i.id AND CAST(h.quantity AS NUMERIC)>0) ORDER BY i.id`

func reviewAlertCandidateIDs(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, afterID string, limit int) ([]string, error) {
	rows, err := tx.QueryContext(ctx, reviewAlertCandidateSQL+` LIMIT ?`, householdID.String(), afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func reviewSuppressionsInTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID) ([]domain.ReviewSuppression, error) {
	rows, err := tx.QueryContext(ctx, reviewSuppressionSelect+` WHERE household_id=? ORDER BY id`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ReviewSuppression{}
	for rows.Next() {
		item, err := scanReviewSuppression(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func reviewAlertGroupsForIDs(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, ids []string, suppressions map[domain.InstrumentID][]domain.ReviewSuppression, masterEnabled bool, now time.Time) (map[string]domain.ReviewAlertGroup, error) {
	placeholders, args := sqlInArgs(ids)
	query := `SELECT i.id,i.name,i.instrument_type,i.quote_currency,r.id,r.name,r.severity,r.enabled,v.canonical_spec_json,e.id,e.revision,e.triggered_on,e.first_hit_on,e.first_detected_at,e.acknowledged_at,s.latest_closed_on,s.policy_state,s.window_start,s.window_end,s.hit_count,s.false_count,s.unknown_count,s.consecutive_count,e.evidence_json,d.amount_exact,d.cost_exact FROM investment_review_evaluation_state s JOIN investment_review_rules r ON r.id=s.rule_id JOIN investment_review_rule_versions v ON v.rule_id=r.id AND v.condition_version=r.current_condition_version JOIN investment_review_episodes e ON e.id=s.episode_id JOIN instruments i ON i.id=s.instrument_id LEFT JOIN investment_review_daily_inputs d ON d.household_id=r.household_id AND d.instrument_id=i.id AND d.timezone_version=(SELECT timezone_version FROM investment_review_daily_results dr WHERE dr.rule_id=r.id AND dr.condition_version=r.current_condition_version AND dr.instrument_id=i.id AND dr.on_date=s.latest_closed_on LIMIT 1) AND d.on_date=s.latest_closed_on AND d.input_policy='rule:'||r.id||'/v:'||r.current_condition_version WHERE r.household_id=? AND i.id IN (` + placeholders + `) AND e.resolved_at IS NULL ORDER BY i.id,r.id`
	// The input policy is rule+condition scoped; use the evaluation state's
	// timezone and closed date so a later rule edit cannot leak an old input.
	parameters := append([]any{householdID.String()}, args...)
	rows, err := tx.QueryContext(ctx, query, parameters...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := make(map[string]domain.ReviewAlertGroup, len(ids))
	for rows.Next() {
		var id, name, kind, currency, ruleID, ruleName, severity, specJSON, episodeID, triggered, firstHit, detected, latest, state, windowStart, windowEnd, evidenceJSON string
		var ruleEnabled, episodeRevision, hits, falseDays, unknownDays, streak int
		var acknowledged, amount, cost sql.NullString
		if err := rows.Scan(&id, &name, &kind, &currency, &ruleID, &ruleName, &severity, &ruleEnabled, &specJSON, &episodeID, &episodeRevision, &triggered, &firstHit, &detected, &acknowledged, &latest, &state, &windowStart, &windowEnd, &hits, &falseDays, &unknownDays, &streak, &evidenceJSON, &amount, &cost); err != nil {
			return nil, err
		}
		instrumentID, err := domain.ParseInstrumentID(id)
		if err != nil {
			return nil, err
		}
		rid, err := domain.ParseReviewRuleID(ruleID)
		if err != nil {
			return nil, err
		}
		eid, err := domain.ParseReviewEpisodeID(episodeID)
		if err != nil {
			return nil, err
		}
		spec, err := domain.DecodeReviewRuleSpec([]byte(specJSON))
		if err != nil {
			return nil, err
		}
		firstDetectedAt, err := time.Parse(time.RFC3339Nano, detected)
		if err != nil {
			return nil, err
		}
		entry := domain.ReviewAlertRule{RuleID: rid, Name: ruleName, Severity: severity, Enabled: ruleEnabled != 0, EpisodeID: eid, EpisodeRevision: episodeRevision, TriggeredOn: triggered, FirstHitOn: firstHit, FirstDetectedAt: firstDetectedAt, LatestClosedOn: latest, Window: domain.ReviewPolicyWindow{WindowStart: windowStart, WindowEnd: windowEnd, HitCount: hits, FalseCount: falseDays, UnknownCount: unknownDays, ConsecutiveCount: streak}, Coverage: []string{}, TriggerPolicy: spec.TriggerPolicy}
		if acknowledged.Valid {
			value, err := time.Parse(time.RFC3339Nano, acknowledged.String)
			if err != nil {
				return nil, err
			}
			entry.AcknowledgedAt = &value
		}
		if amount.Valid {
			entry.AmountExact = &amount.String
		}
		if cost.Valid {
			entry.CostExact = &cost.String
		}
		var evidence struct {
			LatestResult *domain.ReviewDailyResult
			Coverage     []string
		}
		if err := json.Unmarshal([]byte(evidenceJSON), &evidence); err != nil {
			return nil, err
		}
		entry.LatestResult = evidence.LatestResult
		if evidence.Coverage != nil {
			entry.Coverage = evidence.Coverage
		}
		truth := domain.ReviewFalse
		if state == "triggered" {
			truth = domain.ReviewTrue
			entry.Window.Truth = domain.ReviewTrue
		} else if state == "unknown" {
			truth = domain.ReviewUnknown
			entry.Window.Truth = domain.ReviewUnknown
		} else {
			entry.Window.Truth = domain.ReviewFalse
		}
		active, err := domain.ActiveReviewSuppression(suppressions[instrumentID], instrumentID, ruleID, now)
		if err != nil {
			return nil, err
		}
		if active != nil {
			entry.SuppressionID, entry.SuppressedUntil = active.ID, active.UntilOn
		}
		entry.Actionable = domain.ReviewEpisodeActionable(masterEnabled, entry.Enabled, true, entry.AcknowledgedAt != nil, truth, active)
		switch {
		case !masterEnabled || !entry.Enabled:
			entry.Status = "paused"
		case active != nil:
			entry.Status = "ignored"
		case truth == domain.ReviewUnknown:
			entry.Status = "unknown"
		case entry.AcknowledgedAt != nil:
			entry.Status = "acknowledged"
		default:
			entry.Status = "pending"
		}
		group, exists := groups[id]
		if !exists {
			group = domain.ReviewAlertGroup{InstrumentID: instrumentID, DisplayName: name, InstrumentType: domain.InstrumentType(kind), Currency: domain.CurrencyCode(currency), Rules: []domain.ReviewAlertRule{}}
		}
		group.Rules = append(group.Rules, entry)
		groups[id] = group
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	priority := map[string]int{"pending": 0, "unknown": 1, "acknowledged": 2, "ignored": 3, "paused": 4}
	for id, group := range groups {
		sort.Slice(group.Rules, func(i, j int) bool {
			a, b := group.Rules[i], group.Rules[j]
			if priority[a.Status] != priority[b.Status] {
				return priority[a.Status] < priority[b.Status]
			}
			if a.Severity != b.Severity {
				return a.Severity > b.Severity
			}
			return a.RuleID < b.RuleID
		})
		group.Status = group.Rules[0].Status
		groups[id] = group
	}
	return groups, nil
}
