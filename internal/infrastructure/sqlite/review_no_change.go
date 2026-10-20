package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"slices"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// ConfirmCertifiedReviewNoChange holds BEGIN IMMEDIATE through authenticated
// source/projection/provider/clock verification and the mandatory checkpoint CAS.
// A missed confirmation authorizes only complete capture and preparation.
func (r *Repository) ConfirmCertifiedReviewNoChange(ctx context.Context, id domain.HouseholdID, captureNow time.Time, clock func() time.Time, providerSelection domain.ReviewProviderSelection) (domain.ReviewNoChangeResult, bool, error) {
	selection := providerSelection
	var empty domain.ReviewNoChangeResult
	if !r.CertifiedReviewEnabled() {
		return empty, false, nil
	}
	// Any failed confirmation must rebuild all dates before recertification.
	// This also covers early exits (missing/expired proof, metadata changes)
	// that occur before the persisted count manifest can be compared.
	empty.RequireFullRecompute = true
	if id == "" || captureNow.IsZero() || clock == nil {
		return empty, false, &domain.Error{Code: domain.ErrValidation, Message: "complete no-change confirmation arguments are required"}
	}
	epoch, rollback := r.database.reviewClock.observe(captureNow)
	if rollback {
		return empty, false, nil
	}
	conn, connectionID, err := r.certifiedConnection(ctx)
	if err != nil {
		return empty, false, err
	}
	defer conn.Close()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return empty, false, err
	}
	defer tx.Rollback()
	memoGeneration := r.reviewMemoGeneration()
	var cookie, foreignKeys, version int
	for _, item := range []struct {
		query string
		into  *int
	}{{`PRAGMA schema_version`, &cookie}, {`PRAGMA foreign_keys`, &foreignKeys}, {`PRAGMA user_version`, &version}} {
		if err := tx.QueryRowContext(ctx, item.query).Scan(item.into); err != nil {
			return empty, false, err
		}
	}
	if cookie != r.database.reviewVerifiedSchemaVersion || cookie == 0 || foreignKeys != 1 || version != CurrentSchemaVersion {
		return domain.ReviewNoChangeResult{RequireFullRecompute: true}, false, nil
	}
	_, trusted, err := r.verifiedCertifiedTracking(ctx, tx)
	if err != nil {
		return empty, false, err
	}
	if !trusted {
		return empty, false, nil
	}
	control, found, err := reviewControlInTx(ctx, tx, id)
	if err != nil || !found || control.ProjectionStatus != "complete" || control.PublishedFingerprint == "" {
		return empty, false, err
	}
	certificate, err := readReviewTemporal(ctx, tx, id)
	if err != nil {
		return empty, false, err
	}
	completenessVersion := domain.ReviewCertifiedCompletenessVersion
	if !certificate.Covers(captureNow) || certificate.NoChangeCompletenessVersion != completenessVersion || certificate.ClockEpoch != epoch || certificate.Source == nil || certificate.NoChangeReviewFingerprint == "" || certificate.NoChangeFXProviderKey != selection.Key {
		return empty, false, nil
	}
	if certificate.CertifiedNeedsReconcile {
		return empty, false, nil
	}
	if !selection.Pinned || certificate.CertifiedProvider != selection {
		return empty, false, nil
	}
	connection, e := r.certifiedConnectionCheckpoint(ctx, tx, connectionID)
	if e != nil {
		return empty, false, e
	}
	if connectionID == 0 || connection != certificate.CertifiedConnection {
		return empty, false, nil
	}
	// The signed post-commit connection counters prove that no SQLite
	// DML, own commit (including DDL) or external commit has intervened
	// since the complete row seal.
	// Reading every historical JSON again would defeat same-day no-op.
	if certificate.CertifiedProjectionSeal == "" || !r.validReviewMAC(certificate) {
		return empty, false, nil
	}

	var resolver string
	if err := tx.QueryRowContext(ctx, `SELECT resolver_policy_version FROM history_snapshot_state WHERE household_id=?`, id.String()).Scan(&resolver); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return empty, false, nil
		}
		return empty, false, err
	}
	current, err := readReviewSourceCheckpoint(ctx, tx, id, resolver)
	if err != nil {
		return empty, false, err
	}
	proof := *certificate
	proof.Source, proof.ConfigRevision, proof.StateRevision = current, control.ConfigRevision, control.ReviewStateRevision
	proof.RuleRevisions = map[domain.ReviewRuleID]int{}
	rules, err := noChangeReviewRules(ctx, tx, id)
	if err != nil {
		return empty, false, err
	}
	for _, rule := range rules {
		proof.RuleRevisions[rule.ID] = rule.RowRevision
		before, err := domain.ReviewLatestClosedDay(certificate.EvaluatedAt, rule.Spec.Schedule.Timezone)
		if err != nil {
			return empty, false, err
		}
		after, err := domain.ReviewLatestClosedDay(captureNow, rule.Spec.Schedule.Timezone)
		if err != nil {
			return empty, false, err
		}
		if before != after {
			return empty, false, nil
		}
	}
	if !certificate.Eligible(captureNow, &proof) {
		return empty, false, nil
	}
	reviewFingerprint, err := noChangeReviewFingerprint(ctx, tx, id)
	if err != nil {
		return empty, false, err
	}
	if reviewFingerprint != certificate.NoChangeReviewFingerprint {
		return empty, false, nil
	}
	counts, err := noChangeProjectionCounts(ctx, tx, id)
	if err != nil {
		return empty, false, err
	}
	if !slices.Equal(counts, certificate.NoChangeProjectionCounts) {
		// Rebuild all dates rather than trusting a surviving rolling window.
		return domain.ReviewNoChangeResult{RequireFullRecompute: true}, false, nil
	}
	result, err := noChangeReviewSummary(ctx, tx, id, rules, captureNow)
	if err != nil {
		return empty, false, err
	}
	now := clock().UTC()
	finalEpoch, finalRollback := r.database.reviewClock.observe(now)
	if finalRollback || finalEpoch != epoch || now.Before(captureNow) || !certificate.Covers(now) || certificate.NoChangeFXProviderKey != selection.Key {
		return empty, false, reviewRuleConflict("review no-change temporal boundary or clock changed")
	}
	for _, rule := range rules {
		before, err := domain.ReviewLatestClosedDay(captureNow, rule.Spec.Schedule.Timezone)
		if err != nil {
			return empty, false, err
		}
		after, err := domain.ReviewLatestClosedDay(now, rule.Spec.Schedule.Timezone)
		if err != nil {
			return empty, false, err
		}
		if before != after {
			return empty, false, reviewRuleConflict("review no-change closed day advanced")
		}
	}
	// AffectedRows==1 is mandatory even for this self-update: an ignored CAS
	// must not turn a failed confirmation into a successful latest projection.
	cas, err := tx.ExecContext(ctx, `UPDATE investment_review_control SET input_generation=input_generation WHERE household_id=? AND enabled=? AND config_revision=? AND input_generation=? AND review_state_revision=? AND projection_status='complete' AND published_on IS ? AND published_input_fingerprint=?`, id.String(), control.Enabled, control.ConfigRevision, control.InputGeneration, control.ReviewStateRevision, nullableEmpty(control.PublishedOn), control.PublishedFingerprint)
	if err != nil {
		return empty, false, err
	}
	if n, err := cas.RowsAffected(); err != nil || n != 1 {
		return empty, false, reviewRuleConflict("review no-change control CAS failed")
	}
	oldJSON, err := jsoncodec.Marshal(certificate)
	if err != nil {
		return empty, false, err
	}
	certificate.EvaluatedAt = now
	if err := r.prepareCertifiedCounter(ctx, tx, certificate, connectionID); err != nil {
		return empty, false, err
	}
	certificate.CertifiedMAC, err = r.certifiedMAC(*certificate)
	if err != nil {
		return empty, false, err
	}

	newJSON, err := jsoncodec.Marshal(certificate)
	if err != nil {
		return empty, false, err
	}
	cas, err = tx.ExecContext(ctx, `UPDATE review_temporal_checkpoints SET evaluated_at=?,certificate_json=? WHERE household_id=? AND certificate_json=?`, now.Format(time.RFC3339Nano), string(newJSON), id.String(), string(oldJSON))
	if err != nil {
		return empty, false, err
	}
	if n, err := cas.RowsAffected(); err != nil || n != 1 {
		return empty, false, reviewRuleConflict("review no-change checkpoint CAS failed")
	}
	if err := tx.Commit(); err != nil {
		return empty, false, err
	}
	r.commitReviewMemoNoop(oldJSON, *certificate, memoGeneration)
	result.SourceFingerprint = control.PublishedFingerprint
	return result, true, nil
}

// Count complete household namespaces, including old conditions and resolved
// lifecycle history, without loading daily evidence JSON. This completeness
// check does not replace SQLite integrity checks or the full financial oracle.
func noChangeProjectionCounts(ctx context.Context, tx *sql.Tx, id domain.HouseholdID) ([]int64, error) {
	statements := []string{
		`SELECT count(*) FROM investment_review_daily_inputs WHERE household_id=?`,
		`SELECT count(*) FROM investment_review_daily_results d JOIN investment_review_rules r ON r.id=d.rule_id WHERE r.household_id=?`,
		`SELECT count(*) FROM investment_review_position_epochs WHERE household_id=?`,
		`SELECT count(*) FROM investment_review_evaluation_state s JOIN investment_review_rules r ON r.id=s.rule_id WHERE r.household_id=?`,
		`SELECT count(*) FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id WHERE r.household_id=?`,
		`SELECT count(*) FROM investment_review_episode_events v JOIN investment_review_episodes e ON e.id=v.episode_id JOIN investment_review_rules r ON r.id=e.rule_id WHERE r.household_id=?`,
	}
	counts := make([]int64, len(statements))
	for i, statement := range statements {
		if err := tx.QueryRowContext(ctx, statement, id.String()).Scan(&counts[i]); err != nil {
			return nil, err
		}
	}
	return counts, nil
}

// Independent review metadata/handling proof, without daily/episode evidence
// JSON. Rule payloads are small configuration, never financial source data or
// app_configuration (which can contain credentials). Including raw rule spec
// text catches even an out-of-band edit that forgot to advance its revision.
func noChangeReviewFingerprint(ctx context.Context, tx *sql.Tx, id domain.HouseholdID) (string, error) {
	digest := sha256.New()
	encoder := jsoncodec.NewEncoder(digest)
	if err := encoder.Encode("c3a-review-metadata-v1"); err != nil {
		return "", err
	}
	for _, statement := range []string{
		`SELECT r.*,v.canonical_spec_json,v.semantic_hash FROM investment_review_rules r JOIN investment_review_rule_versions v ON v.rule_id=r.id AND v.condition_version=r.current_condition_version WHERE r.household_id=? ORDER BY r.id`,
		`SELECT t.* FROM investment_review_rule_targets t JOIN investment_review_rules r ON r.id=t.rule_id WHERE r.household_id=? ORDER BY t.rule_id,t.instrument_id`,
		`SELECT * FROM investment_review_suppressions WHERE household_id=? ORDER BY id`,
		`SELECT e.id,e.rule_id,e.instrument_id,e.position_epoch_id,e.opened_condition_version,e.first_hit_on,e.triggered_on,e.first_detected_at,e.resolved_at,e.resolved_reason,e.acknowledged_at,e.revision FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id WHERE r.household_id=? AND e.resolved_at IS NULL ORDER BY e.id`,
	} {
		if err := encoder.Encode(statement); err != nil {
			return "", err
		}
		rows, err := tx.QueryContext(ctx, statement, id.String())
		if err != nil {
			return "", err
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
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
		}
		err = errors.Join(err, rows.Err(), rows.Close())
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func noChangeReviewRules(ctx context.Context, tx *sql.Tx, id domain.HouseholdID) ([]domain.ReviewRule, error) {
	rows, err := tx.QueryContext(ctx, reviewRuleSelect+` WHERE r.household_id=? AND r.evaluator=? ORDER BY r.id`, id.String(), domain.ReviewEvaluator)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []domain.ReviewRule
	for rows.Next() {
		rule, err := scanReviewRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func noChangeReviewSummary(ctx context.Context, tx *sql.Tx, id domain.HouseholdID, rules []domain.ReviewRule, now time.Time) (domain.ReviewNoChangeResult, error) {
	result := domain.ReviewNoChangeResult{}
	capture := domain.ReviewProjectionCapture{Rules: rules, EvaluatedInstruments: map[domain.ReviewRuleID][]domain.InstrumentID{}}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT h.instrument_id FROM holdings h JOIN accounts a ON a.id=h.account_id WHERE a.household_id=?`, id.String())
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var holding domain.Holding
		if err := rows.Scan(&holding.InstrumentID); err != nil {
			rows.Close()
			return result, err
		}
		capture.Historical.History.Portfolio.Holdings = append(capture.Historical.History.Portfolio.Holdings, holding)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return result, err
	}
	// The complete candidate universe includes old evaluated instruments and
	// open episodes, even for closed holdings or an archived/replaced rule.
	rows, err = tx.QueryContext(ctx, `SELECT s.rule_id,s.instrument_id FROM investment_review_evaluation_state s JOIN investment_review_rules r ON r.id=s.rule_id WHERE r.household_id=? UNION SELECT e.rule_id,e.instrument_id FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id WHERE r.household_id=? AND e.resolved_at IS NULL`, id.String(), id.String())
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var rule domain.ReviewRuleID
		var instrument domain.InstrumentID
		if err := rows.Scan(&rule, &instrument); err != nil {
			rows.Close()
			return result, err
		}
		capture.EvaluatedInstruments[rule] = append(capture.EvaluatedInstruments[rule], instrument)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return result, err
	}
	for _, rule := range rules {
		count := len(reviewWindowInstruments(capture, rule))
		result.RuleInstrumentCount += count
		if count > 0 {
			on, err := domain.ReviewLatestClosedDay(now, rule.Spec.Schedule.Timezone)
			if err != nil {
				return result, err
			}
			if on > result.LatestClosedOn {
				result.LatestClosedOn = on
			}
		}
	}
	return result, nil
}
