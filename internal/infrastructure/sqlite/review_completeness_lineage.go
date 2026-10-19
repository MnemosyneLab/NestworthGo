package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strconv"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// This immutable configuration enables diagnostic reads only. It does not
// enable C3 confirmation, window reuse, new writes, or a publication shortcut.
// The synchronous observer must be concurrency-safe and must not reenter DB.
func (r *Repository) WithReviewCompletenessLineageShadow(observer func(domain.ReviewCompletenessLineageObservation)) *Repository {
	copy := *r
	copy.reviewCompletenessLineageShadow = true
	copy.reviewCompletenessLineageObserver = observer
	return &copy
}

func (r *Repository) ReviewCompletenessLineageShadowEnabled() bool {
	return r.reviewCompletenessLineageShadow && !r.reviewFullRecompute
}

func (r *Repository) observeReviewLineage(phase, verdict string) {
	if r.reviewCompletenessLineageObserver != nil {
		r.reviewCompletenessLineageObserver(domain.ReviewCompletenessLineageObservation{Phase: phase, Verdict: verdict})
	}
}

func (r *Repository) captureReviewLineage(ctx context.Context, tx *sql.Tx, capture domain.ReviewProjectionCapture, epoch string) (*domain.ReviewCompletenessLineage, error) {
	l := &domain.ReviewCompletenessLineage{Version: domain.ReviewCompletenessLineageVersion, Reason: "untrusted_checkpoint"}
	previous := capture.PublishedTemporal
	completenessVersion := domain.ReviewNoChangeCompletenessVersion
	if r.CertifiedReviewEnabled() {
		completenessVersion = domain.ReviewCertifiedCompletenessVersion
	}
	if previous == nil || previous.EvaluatedAt.IsZero() || !previous.NextTransitionAt.After(previous.EvaluatedAt) || previous.NoChangeCompletenessVersion != completenessVersion || previous.NoChangeReviewFingerprint == "" || capture.Control.ProjectionStatus != "complete" || capture.Control.PublishedFingerprint == "" {
		return l, nil
	}
	if previous.PolicyVersion != domain.ReviewTemporalPolicyVersion || previous.DependencyScope != domain.ReviewTemporalDependencyScope || previous.ClockEpoch != epoch || capture.SourceCutoff.Before(previous.EvaluatedAt) {
		l.Reason = "clock_or_policy_changed"
		return l, nil
	}
	if previous.Source == nil || capture.SourceRevision == nil || !previous.Source.Valid() || !capture.SourceRevision.Valid() || previous.Source.Token != capture.SourceRevision.Token || previous.Source.Token.Revision < capture.SourceRevision.JournalFloor || capture.HistoricalFingerprint != capture.Control.PublishedFingerprint {
		l.Reason = "source_changed"
		return l, nil
	}
	var cookie, fk, version int
	for _, p := range []struct {
		query string
		dest  *int
	}{{`PRAGMA schema_version`, &cookie}, {`PRAGMA foreign_keys`, &fk}, {`PRAGMA user_version`, &version}} {
		if err := tx.QueryRowContext(ctx, p.query).Scan(p.dest); err != nil {
			return nil, err
		}
	}
	if cookie == 0 || cookie != r.database.reviewVerifiedSchemaVersion || fk != 1 || version != CurrentSchemaVersion {
		l.Reason = "tracking_unverified"
		return l, nil
	}
	if err := verifyFinancialSourceTracking(ctx, tx); err != nil {
		l.Reason = "tracking_unverified"
		return l, ctx.Err()
	}
	if previous.ConfigRevision != capture.Control.ConfigRevision || previous.StateRevision != capture.Control.ReviewStateRevision || len(previous.RuleRevisions) != len(capture.Rules) {
		l.Reason = "review_changed"
		return l, nil
	}
	l.ClosedOn = map[domain.ReviewRuleID]string{}
	advanced := false
	for _, rule := range capture.Rules {
		if !rule.Enabled || rule.ArchivedAt != nil || previous.RuleRevisions[rule.ID] != rule.RowRevision {
			l.Reason = "unsupported_or_changed_rule"
			return l, nil
		}
		old, err := domain.ReviewLatestClosedDay(previous.EvaluatedAt, rule.Spec.Schedule.Timezone)
		if err != nil {
			return nil, err
		}
		current, err := domain.ReviewLatestClosedDay(capture.SourceCutoff, rule.Spec.Schedule.Timezone)
		if err != nil {
			return nil, err
		}
		if current < old {
			l.Reason = "closed_day_rollback"
			return l, nil
		}
		advanced = advanced || current > old
		l.ClosedOn[rule.ID] = old
	}
	if !previous.Covers(capture.SourceCutoff) && !advanced {
		l.Reason = "intra_day_transition"
		return l, nil
	}
	fingerprint, err := noChangeReviewFingerprint(ctx, tx, capture.HouseholdID)
	if err != nil {
		return nil, err
	}
	if fingerprint != previous.NoChangeReviewFingerprint {
		l.Reason = "review_changed"
		return l, nil
	}
	counts, err := noChangeProjectionCounts(ctx, tx, capture.HouseholdID)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(counts, previous.NoChangeProjectionCounts) {
		l.Reason = "counts_changed"
		return l, nil
	}
	bound, err := json.Marshal(struct {
		Household                  domain.HouseholdID
		Source                     *domain.ReviewSourceCheckpoint
		Control                    domain.ReviewProjectionControl
		Fingerprint, Epoch, Review string
	}{capture.HouseholdID, capture.SourceRevision, capture.Control, capture.HistoricalFingerprint, epoch, fingerprint})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bound)
	l.BindingDigest = hex.EncodeToString(digest[:])
	l.ProviderKey = previous.NoChangeFXProviderKey
	l.ProjectionSeal, l.Inputs, l.Results, err = reviewLineageProjectionSnapshot(ctx, tx, capture)
	if err != nil {
		return nil, err
	}
	// Load only a separate bounded hypothetical window. The actual capture
	// remains full, and publication repeats these same reads under its lock.
	shadow := capture
	shadow.TemporalFullRecompute = false
	if err := r.loadReusableReviewWindow(ctx, tx, &shadow); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		l.Reason = "window_unreadable"
		return l, nil
	}
	l.Window = shadow.ReusableDailyEvidence
	l.Candidate, l.Reason = true, "snapshot_candidate"
	return l, nil
}

// Seal narrow daily keys/fingerprints plus lifecycle metadata in stable order.
// No full daily input/result JSON is read. Current namespaces are also retained
// for independent full-oracle prefix coverage, detecting count-preserving gaps.
func reviewLineageProjectionSnapshot(ctx context.Context, tx *sql.Tx, capture domain.ReviewProjectionCapture) (string, map[domain.ReviewDailyEvidenceKey]string, map[domain.ReviewDailyEvidenceKey]domain.ReviewLineageResult, error) {
	inputs := map[domain.ReviewDailyEvidenceKey]string{}
	results := map[domain.ReviewDailyEvidenceKey]domain.ReviewLineageResult{}
	policies := map[string]domain.ReviewRule{}
	rules := map[string]domain.ReviewRule{}
	for _, rule := range capture.Rules {
		policies["rule:"+string(rule.ID)+"/v:"+strconv.Itoa(rule.CurrentConditionVersion)] = rule
		rules[string(rule.ID)] = rule
	}
	digest := sha256.New()
	encoder := json.NewEncoder(digest)
	statements := []string{
		`SELECT instrument_id,timezone_version,on_date,input_policy,input_fingerprint FROM investment_review_daily_inputs WHERE household_id=? ORDER BY instrument_id,timezone_version,on_date,input_policy`,
		`SELECT d.rule_id,d.condition_version,d.instrument_id,d.timezone_version,d.on_date,d.input_fingerprint,d.predicate_truth,COALESCE(d.reason,'') FROM investment_review_daily_results d JOIN investment_review_rules r ON r.id=d.rule_id WHERE r.household_id=? ORDER BY d.rule_id,d.condition_version,d.instrument_id,d.timezone_version,d.on_date`,
		`SELECT * FROM investment_review_position_epochs WHERE household_id=? ORDER BY id`,
		`SELECT s.* FROM investment_review_evaluation_state s JOIN investment_review_rules r ON r.id=s.rule_id WHERE r.household_id=? ORDER BY s.rule_id,s.instrument_id,s.position_epoch_id`,
		`SELECT e.id,e.rule_id,e.instrument_id,e.position_epoch_id,e.opened_condition_version,e.first_hit_on,e.triggered_on,e.first_detected_at,e.resolved_at,e.resolved_reason,e.acknowledged_at,e.revision,e.evidence_fingerprint FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id WHERE r.household_id=? ORDER BY e.id`,
		`SELECT v.* FROM investment_review_episode_events v JOIN investment_review_episodes e ON e.id=v.episode_id JOIN investment_review_rules r ON r.id=e.rule_id WHERE r.household_id=? ORDER BY v.id`,
	}
	for index, statement := range statements {
		if err := encoder.Encode(index); err != nil {
			return "", nil, nil, err
		}
		rows, err := tx.QueryContext(ctx, statement, capture.HouseholdID.String())
		if err != nil {
			return "", nil, nil, err
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", nil, nil, err
		}
		values, dest := make([]any, len(columns)), make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		for rows.Next() {
			if err = rows.Scan(dest...); err != nil {
				break
			}
			if err = encoder.Encode(values); err != nil {
				break
			}
			text := func(i int) string {
				switch v := values[i].(type) {
				case string:
					return v
				case []byte:
					return string(v)
				default:
					return ""
				}
			}
			if index == 0 {
				if rule, ok := policies[text(3)]; ok && text(1) == rule.Spec.Schedule.Timezone+"/"+rule.Spec.Schedule.CutoffVersion {
					inputs[domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: domain.InstrumentID(text(0)), TimezoneVersion: text(1), On: text(2)}] = text(4)
				}
			} else if index == 1 {
				condition, ok := values[1].(int64)
				if rule, exists := rules[text(0)]; exists && ok && condition == int64(rule.CurrentConditionVersion) && text(3) == rule.Spec.Schedule.Timezone+"/"+rule.Spec.Schedule.CutoffVersion {
					truth := domain.ReviewTruth(text(6))
					if text(6) == "unknown" {
						// SQL uses a literal sentinel; the domain deliberately uses
						// its empty value for unknown (also encoded in result JSON).
						truth = domain.ReviewUnknown
					}
					results[domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: domain.InstrumentID(text(2)), TimezoneVersion: text(3), On: text(4)}] = domain.ReviewLineageResult{InputFingerprint: text(5), Truth: truth, Reason: text(7)}
				}
			}
		}
		err = errors.Join(err, rows.Err(), rows.Close())
		if err != nil {
			return "", nil, nil, err
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), inputs, results, nil
}
