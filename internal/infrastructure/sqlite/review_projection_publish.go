package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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
	return r.PublishReviewProjectionWithClock(ctx, publication, func() time.Time { return now })
}

// The clock is sampled after the full recheck and revision checks, immediately
// before publication writes. A time passed before this potentially long work
// cannot detect an activation that occurs while the recheck is running.
func (r *Repository) PublishReviewProjectionWithClock(ctx context.Context, publication domain.ReviewPublication, clock func() time.Time) error {
	return r.publishReviewProjection(ctx, publication, clock, false)
}

// The application's guard holds its provider read lease through this entire
// transaction. A direct legacy caller receives only full-source/full-row work.
func (r *Repository) PublishCertifiedReviewProjection(ctx context.Context, publication domain.ReviewPublication, clock func() time.Time, guard func(func() error) error) error {
	if guard == nil {
		return r.PublishReviewProjectionWithClock(ctx, publication, clock)
	}
	return guard(func() error { return r.publishReviewProjection(ctx, publication, clock, true) })
}

func (r *Repository) publishReviewProjection(ctx context.Context, publication domain.ReviewPublication, clock func() time.Time, providerLeased bool) (publishErr error) {
	if r.CertifiedReviewEnabled() && providerLeased && publication.Capture.Certified != nil && publication.FXProviderKey != publication.Capture.Certified.Provider.Key {
		return reviewRuleConflict("staged review provider differs from the held selection")
	}
	if r.CertifiedReviewEnabled() && len(publication.Capture.ReusableDailyEvidence) > 0 {
		if publication.CertifiedFullOracle == nil {
			return reviewRuleConflict("certified full oracle is missing")
		}
		if !providerLeased || domain.ValidateReviewCertifiedOutput(publication) != "window_oracle_matched" {
			publication = domain.ReviewFullPublication(publication)
		}
	}

	capture := publication.Capture
	lineageVerdict := "authoritative_guard_rejected"
	if r.ReviewCompletenessLineageShadowEnabled() {
		defer func() {
			if publishErr != nil {
				lineageVerdict = "authoritative_guard_rejected"
			}
			r.observeReviewLineage("publication", lineageVerdict)
		}()
	}
	certifiedSourceUsed := false
	if r.CertifiedReviewEnabled() {
		defer func() {
			verdict := "full_source_recheck"
			if certifiedSourceUsed {
				verdict = "verified_source_cas"
			}
			if publishErr != nil {
				verdict = "rejected"
			}
			r.observeReviewLineage("certified_publication", verdict)
		}()
	}
	if capture.HouseholdID == "" || capture.SourceCutoff.IsZero() || capture.HistoricalFingerprint == "" || publication.At.IsZero() || clock == nil {
		return &domain.Error{Code: domain.ErrValidation, Field: "publication", Message: "a complete review capture is required"}
	}
	if capture.Temporal == nil {
		return reviewRuleConflict("review temporal capture is missing; recapture before publication")
	}
	var tx *sql.Tx
	var err error
	var connectionID uint64
	if r.CertifiedReviewEnabled() {
		var conn *sql.Conn
		conn, connectionID, err = r.certifiedConnection(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		tx, err = conn.BeginTx(ctx, nil)
	} else {
		tx, err = r.database.SQL.BeginTx(ctx, nil)
	}
	if err != nil {
		return err
	}
	defer tx.Rollback()
	checkpoint, certified, mayCertify, err := r.verifyCertifiedPublication(ctx, tx, capture, providerLeased, connectionID)
	if err != nil {
		return err
	}
	certifiedSourceUsed = certified
	actualFingerprint := capture.HistoricalFingerprint
	if !certified {
		// Unknown, changed or damaged tracking never substitutes a token for
		// the authoritative original source load and fingerprint recheck.
		actual, err := loadReviewHistoricalBatchQuery(ctx, tx, capture.HouseholdID, capture.SourceCutoff)
		if err != nil {
			return err
		}
		actualFingerprint, err = domain.ReviewHistoricalFingerprint(actual)
		if err != nil {
			return err
		}
		checkpoint, err = readReviewSourceCheckpoint(ctx, tx, capture.HouseholdID, actual.History.ResolverPolicyVersion)
		if err != nil {
			return err
		}
	}

	if r.CertifiedReviewEnabled() && !certified && len(capture.ReusableDailyEvidence) > 0 {
		publication = domain.ReviewFullPublication(publication)
		capture = publication.Capture
	}
	observation := domain.CompareReviewSourceCheckpoints(capture, domain.ReviewProjectionCapture{SourceCutoff: capture.SourceCutoff, HistoricalFingerprint: actualFingerprint, SourceRevision: checkpoint})
	if r.reviewSourceShadowObserver != nil {
		r.reviewSourceShadowObserver(observation)
	}
	// C2 retains the complete fingerprint check even when tokens agree. A
	// known token changing is an additional conservative conflict, never a
	// replacement for the control/rule/handling/time checks below.
	if actualFingerprint != capture.HistoricalFingerprint {
		return reviewRuleConflict("financial evidence changed during review calculation")
	}
	if capture.SourceRevision != nil && (checkpoint == nil || !observation.SameToken) {
		return reviewRuleConflict("financial source revision changed during review calculation")
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
	stagedRules := make(map[domain.ReviewRuleID]struct{}, len(publication.Rules))
	for _, item := range publication.Rules {
		rule, ok := byRule[item.RuleID]
		if !ok || rule.RowRevision != item.RuleRevision || rule.CurrentConditionVersion != item.ConditionVersion || item.InstrumentID == "" || item.TimezoneVersion != rule.Spec.Schedule.Timezone+"/"+rule.Spec.Schedule.CutoffVersion {
			return reviewRuleConflict("staged review rule changed")
		}
		stagedRules[item.RuleID] = struct{}{}
	}
	// One publication commonly contains one item per rule/instrument pair.
	// Rule revisions are household-wide, so check each staged rule once.
	for _, rule := range capture.Rules {
		if _, staged := stagedRules[rule.ID]; !staged {
			continue
		}
		var revision, condition int
		if err := tx.QueryRowContext(ctx, `SELECT row_revision,current_condition_version FROM investment_review_rules WHERE id=? AND household_id=?`, string(rule.ID), capture.HouseholdID.String()).Scan(&revision, &condition); err != nil {
			return err
		}
		if revision != rule.RowRevision || condition != rule.CurrentConditionVersion {
			return reviewRuleConflict("review rule changed during calculation")
		}
	}
	if r.ReviewCompletenessLineageShadowEnabled() {
		diagnosticPublication := publication
		if publication.CertifiedFullOracle != nil {
			diagnosticPublication = domain.ReviewFullPublication(publication)
			diagnosticPublication.WindowOracle = &domain.ReviewWindowOracle{Version: domain.ReviewWindowOracleVersion, Rules: publication.Rules}
		}
		lineageVerdict = domain.ValidateReviewCompletenessPrefix(diagnosticPublication)
		if lineageVerdict == "prefix_validated" {
			lineageVerdict = domain.ValidateReviewWindowOracle(diagnosticPublication)
		}
		if lineageVerdict == "window_oracle_matched" {
			current := capture
			current.Control, current.SourceRevision, current.HistoricalFingerprint = control, checkpoint, actualFingerprint
			current.PublishedTemporal, err = readReviewTemporal(ctx, tx, capture.HouseholdID)
			if err != nil {
				return err
			}
			proof, err := r.captureReviewLineage(ctx, tx, current, capture.Temporal.ClockEpoch, mayCertify)
			if err != nil {
				return err
			}
			switch {
			case !proof.Candidate:
				lineageVerdict = proof.Reason
			case proof.BindingDigest != capture.CompletenessLineage.BindingDigest:
				lineageVerdict = "binding_changed"
			case !reflect.DeepEqual(proof.Window, capture.CompletenessLineage.Window):
				lineageVerdict = "window_changed"
			case proof.ProjectionSeal != capture.CompletenessLineage.ProjectionSeal:
				lineageVerdict = "projection_changed"
			default:
				lineageVerdict = "lineage_validated_shadow"
			}
		}
	}
	// C2 also repeats the uncut future-source fence inside this write
	// reservation. The conservative all-activity fence can change without a
	// fingerprint change when an excluded future fact has untracked DML.
	frontier, err := buildReviewTemporal(ctx, tx, capture, capture.Temporal.ClockEpoch)
	if err != nil {
		return err
	}
	now := clock()
	epoch, rollback := r.database.reviewClock.observe(now)
	if rollback || capture.Temporal == nil || capture.Temporal.ClockEpoch != epoch || !capture.Temporal.Covers(now) || !frontier.Covers(now) {
		return reviewRuleConflict("review temporal boundary or clock changed during calculation")
	}
	// Persist the intersection of both proofs. Keeping only the captured end
	// could overrun a newly discovered boundary; keeping only the refreshed
	// end could extend the staged calculation past its original proof. Copy
	// the certificate so publication never mutates the caller's capture.
	publishedTemporal := *capture.Temporal
	if frontier.NextTransitionAt.Before(publishedTemporal.NextTransitionAt) {
		publishedTemporal.NextTransitionAt = frontier.NextTransitionAt
	}
	if err := validateReviewPublicationTime(capture, publication.Rules, now); err != nil {
		return err
	}
	// Consume private genesis eligibility before any publication writes while
	// holding the writer reservation. A captured initial claim cannot survive
	// an intervening C2/C3 publication followed by SQL erasure of all markers.
	if r.CertifiedReviewEnabled() && capture.Certified != nil && capture.Certified.InitialPublication {
		if !certified || !mayCertify || !capture.Certified.MayCertify || !r.database.reviewInitialPublicationAllowed.CompareAndSwap(true, false) {
			mayCertify = false
			r.database.reviewInitialPublicationAllowed.Store(false)
		}
	} else {
		r.database.reviewInitialPublicationAllowed.Store(false)
	}
	if !found {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_control(household_id) VALUES(?)`, capture.HouseholdID.String()); err != nil {
			return err
		}
	}
	// Bootstrap never trusts retained derived values. Only a signed, supported
	// history-free capture can request full replacement under this reservation.
	bootstrap := reviewBootstrapProvenanceEnabled && certified && mayCertify && capture.Certified != nil && capture.Certified.MayBootstrap && capture.TemporalFullRecompute && len(capture.ReusableDailyEvidence) == 0 && publication.CertifiedFullOracle == nil
	inputSQL := `INSERT INTO investment_review_daily_inputs(household_id,instrument_id,timezone_version,on_date,input_policy,quantity,native_currency,instrument_type,amount_exact,cost_exact,evidence_json,coverage_json,input_fingerprint,computed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(household_id,instrument_id,timezone_version,on_date,input_policy) DO UPDATE SET quantity=excluded.quantity,native_currency=excluded.native_currency,instrument_type=excluded.instrument_type,amount_exact=excluded.amount_exact,cost_exact=excluded.cost_exact,evidence_json=excluded.evidence_json,coverage_json=excluded.coverage_json,input_fingerprint=excluded.input_fingerprint,computed_at=excluded.computed_at WHERE input_fingerprint<>excluded.input_fingerprint`
	if r.CertifiedReviewEnabled() {
		inputSQL += ` OR evidence_json<>excluded.evidence_json OR coverage_json<>excluded.coverage_json`
	}
	if bootstrap {
		inputSQL = strings.Split(inputSQL, " WHERE ")[0]
	}
	inputStatement, err := tx.PrepareContext(ctx, inputSQL)
	if err != nil {
		return err
	}
	defer inputStatement.Close()
	resultSQL := `INSERT INTO investment_review_daily_results(rule_id,condition_version,instrument_id,timezone_version,on_date,predicate_truth,reason,evidence_json,input_fingerprint,evaluated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(rule_id,condition_version,instrument_id,timezone_version,on_date) DO UPDATE SET predicate_truth=excluded.predicate_truth,reason=excluded.reason,evidence_json=excluded.evidence_json,input_fingerprint=excluded.input_fingerprint,revised_at=excluded.evaluated_at WHERE predicate_truth<>excluded.predicate_truth OR input_fingerprint<>excluded.input_fingerprint OR evidence_json<>excluded.evidence_json`
	if bootstrap {
		resultSQL = strings.Split(resultSQL, " WHERE ")[0] + ",evaluated_at=excluded.evaluated_at,revised_at=NULL"
	}
	resultStatement, err := tx.PrepareContext(ctx, resultSQL)
	if err != nil {
		return err
	}
	defer resultStatement.Close()
	existingStateEpochs, err := reviewEvaluationStateEpochs(ctx, tx, publication.Rules)
	if err != nil {
		return err
	}
	for _, item := range publication.Rules {
		if err := publishReviewRule(ctx, tx, capture, item, publication.At, inputStatement, resultStatement, existingStateEpochs[reviewEvaluationStateKey(item.RuleID, item.InstrumentID)], bootstrap); err != nil {
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
	if r.CertifiedReviewEnabled() {
		result, err := tx.ExecContext(ctx, `UPDATE investment_review_control SET input_generation=?,projection_status=?,published_on=?,published_input_fingerprint=? WHERE household_id=? AND enabled=? AND config_revision=? AND input_generation=? AND review_state_revision=? AND projection_status=? AND published_on IS ? AND published_input_fingerprint IS ?`, generation, status, nullableEmpty(publishedOn), actualFingerprint, capture.HouseholdID.String(), control.Enabled, control.ConfigRevision, control.InputGeneration, control.ReviewStateRevision, control.ProjectionStatus, nullableEmpty(control.PublishedOn), nullableEmpty(control.PublishedFingerprint))
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return reviewRuleConflict("certified control CAS failed")
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE investment_review_control SET input_generation=?,projection_status=?,published_on=?,published_input_fingerprint=? WHERE household_id=? AND (input_generation<>? OR projection_status<>? OR published_on IS NOT ? OR published_input_fingerprint IS NOT ?)`, generation, status, nullableEmpty(publishedOn), actualFingerprint, capture.HouseholdID.String(), generation, status, nullableEmpty(publishedOn), actualFingerprint); err != nil {
			return err
		}
	}
	// Never bless an incremental C2 stage handed to an opt-in publisher. A
	// fresh capture has no C3 fields; explicitly clear them for direct callers.
	publishedTemporal.NoChangeCompletenessVersion = ""
	publishedTemporal.NoChangeProjectionCounts = nil
	publishedTemporal.NoChangeReviewFingerprint = ""
	publishedTemporal.NoChangeFXProviderKey = ""
	if r.ReviewNoChangeEnabled() && (len(capture.ReusableDailyEvidence) == 0 || (certified && publication.CertifiedFullOracle != nil && domain.ValidateReviewCertifiedOutput(publication) == "window_oracle_matched")) {
		publishedTemporal.NoChangeCompletenessVersion = domain.ReviewNoChangeCompletenessVersion
		publishedTemporal.NoChangeProjectionCounts, err = noChangeProjectionCounts(ctx, tx, capture.HouseholdID)
		if err != nil {
			return err
		}
		publishedTemporal.NoChangeFXProviderKey = publication.FXProviderKey
		publishedTemporal.NoChangeReviewFingerprint, err = noChangeReviewFingerprint(ctx, tx, capture.HouseholdID)
		if err != nil {
			return err
		}
	}
	publishedTemporal.EvaluatedAt = now.UTC()
	if bootstrap {
		bootstrap, err = verifyCertifiedBootstrapNamespace(ctx, tx, publication)
		if err != nil {
			return err
		}
	}
	if err := r.certifyPublishedReview(ctx, tx, capture, &publishedTemporal, mayCertify, connectionID, bootstrap); err != nil {
		return err
	}
	if r.CertifiedReviewEnabled() {
		if err := r.persistCertifiedTemporal(ctx, tx, capture, publishedTemporal, now); err != nil {
			return err
		}
	} else {
		if err := persistReviewTemporal(ctx, tx, capture.HouseholdID, publishedTemporal, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if publishedTemporal.CertifiedMAC != "" {
		r.database.reviewCertificateKey.issued.Store(true)
	}
	return nil
}

func validateReviewPublicationTime(capture domain.ReviewProjectionCapture, rules []domain.ReviewRulePublication, now time.Time) error {
	if now.IsZero() {
		return &domain.Error{Code: domain.ErrValidation, Field: "now", Message: "a publication time is required"}
	}
	if now.Before(capture.SourceCutoff) {
		return reviewRuleConflict("review clock moved backward during calculation")
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
	certificate, err := readReviewTemporal(ctx, tx, householdID)
	if err != nil {
		return result, false, err
	}
	result.PublishedTemporalRevision = reviewTemporalDigest(certificate)
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

func publishReviewRule(ctx context.Context, tx *sql.Tx, capture domain.ReviewProjectionCapture, item domain.ReviewRulePublication, at time.Time, inputStatement, resultStatement *sql.Stmt, existingStateEpochs []string, bootstrap bool) error {
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
		epochSQL := `INSERT INTO investment_review_position_epochs(id,household_id,instrument_id,opened_at,opening_activity_id,closed_at,closing_activity_id,start_unknown,source_fingerprint) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET closed_at=excluded.closed_at,closing_activity_id=excluded.closing_activity_id,source_fingerprint=excluded.source_fingerprint WHERE closed_at IS NOT excluded.closed_at OR closing_activity_id IS NOT excluded.closing_activity_id OR source_fingerprint<>excluded.source_fingerprint`
		if bootstrap {
			epochSQL = strings.Split(epochSQL, " DO UPDATE SET ")[0] + " DO UPDATE SET household_id=excluded.household_id,instrument_id=excluded.instrument_id,opened_at=excluded.opened_at,opening_activity_id=excluded.opening_activity_id,closed_at=excluded.closed_at,closing_activity_id=excluded.closing_activity_id,start_unknown=excluded.start_unknown,source_fingerprint=excluded.source_fingerprint"
		}
		_, err := tx.ExecContext(ctx, epochSQL, epoch.ID, capture.HouseholdID.String(), item.InstrumentID.String(), nullableTimePtr(epoch.OpenedAt), nullableID(epoch.OpeningID), nullableTimePtr(epoch.ClosedAt), nullableID(epoch.ClosingID), boolValue(epoch.StartUnknown), capture.HistoricalFingerprint)
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
		if capture.Certified != nil {
			inputJSON, err = encodeCompleteReviewInput(day.Input)
		}
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
		_, err = inputStatement.ExecContext(ctx, capture.HouseholdID.String(), item.InstrumentID.String(), item.TimezoneVersion, day.On, inputPolicy, nullableReviewPointer(day.Input.Quantity), currency, kind, nullableReviewPointer(day.Input.AmountExact), nullableReviewPointer(day.Input.CostExact), string(inputJSON), string(coverageJSON), day.InputFingerprint, atText)
		if err != nil {
			return err
		}
		resultJSON, err := json.Marshal(day.Result)
		if err != nil {
			return err
		}
		truth := reviewTruthSQL(day.Result.Truth)
		_, err = resultStatement.ExecContext(ctx, string(item.RuleID), item.ConditionVersion, item.InstrumentID.String(), item.TimezoneVersion, day.On, truth, nullableEmpty(strings.Join(day.CoverageMissing, ",")), string(resultJSON), day.InputFingerprint, atText)
		if err != nil {
			return err
		}
	}
	if err := publishReviewEpisodes(ctx, tx, capture, item, at); err != nil {
		return err
	}
	return publishReviewEvaluationState(ctx, tx, item, existingStateEpochs, bootstrap)
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
	evidence := reviewEpisodeEvidence{
		Window: item.LatestWindow, TriggerWindow: item.TriggerWindow, FirstHitOn: item.FirstHitOn,
		TriggerResult: item.TriggerResult, TriggerCoverage: item.TriggerCoverage,
		LatestResult: item.LatestResult, Coverage: item.LatestCoverage,
	}
	payload, fingerprint, err := reviewEvidenceFingerprint(evidence)
	if err != nil {
		return err
	}
	firstHit := item.FirstHitOn
	if firstHit == "" {
		firstHit = current.TriggeredOn // a retained episode may be U today
	}
	if item.OpenedEpisode {
		if _, exists := expected[domain.ReviewEpisodeID(current.ID)]; exists || current.AcknowledgedAt != nil || item.LatestWindow.Truth != domain.ReviewTrue {
			return reviewRuleConflict("invalid new review episode")
		}
		if item.TriggerWindow.WindowStart == "" || item.TriggerWindow.WindowEnd != current.TriggeredOn || item.FirstHitOn == "" {
			return reviewRuleConflict("new review episode has no initial trigger evidence")
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO investment_review_episodes(id,rule_id,instrument_id,position_epoch_id,opened_condition_version,first_hit_on,triggered_on,first_detected_at,window_start,window_end,latest_closed_on,hit_count,false_count,unknown_count,consecutive_count,evidence_json,evidence_fingerprint,revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1)`, current.ID, string(item.RuleID), item.InstrumentID.String(), item.CurrentEpochID, item.ConditionVersion, firstHit, current.TriggeredOn, current.FirstDetectedAt.UTC().Format(time.RFC3339Nano), item.LatestWindow.WindowStart, item.LatestWindow.WindowEnd, item.LatestClosedOn, item.LatestWindow.HitCount, item.LatestWindow.FalseCount, item.LatestWindow.UnknownCount, item.LatestWindow.ConsecutiveCount, payload, fingerprint)
		if err != nil {
			return err
		}
		openedEvidence := evidence
		openedEvidence.Window = item.TriggerWindow
		openedEvidence.LatestResult = item.TriggerResult
		openedEvidence.Coverage = item.TriggerCoverage
		openedPayload, _, err := reviewEvidenceFingerprint(openedEvidence)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_episode_events(id,episode_id,event_key,kind,payload_json,created_at) VALUES(?,?,?,?,?,?)`, string(domain.NewReviewEpisodeID()), current.ID, "open/"+current.ID, "opened", openedPayload, atText); err != nil {
			return err
		}
		return nil
	}
	stored, exists := expected[domain.ReviewEpisodeID(current.ID)]
	if !exists || stored.PositionEpochID != item.CurrentEpochID || stored.RuleID != item.RuleID || stored.InstrumentID != item.InstrumentID {
		return reviewRuleConflict("review episode identity changed")
	}
	var previousFingerprint, previousFirstHit, previousTriggered, previousLatest string
	if err := tx.QueryRowContext(ctx, `SELECT evidence_fingerprint,first_hit_on,triggered_on,latest_closed_on FROM investment_review_episodes WHERE id=? AND revision=? AND resolved_at IS NULL`, current.ID, stored.Revision).Scan(&previousFingerprint, &previousFirstHit, &previousTriggered, &previousLatest); err != nil {
		return err
	}
	if previousFingerprint == fingerprint && previousFirstHit == firstHit && previousTriggered == current.TriggeredOn && previousLatest == item.LatestClosedOn {
		return nil
	}
	changed, err := tx.ExecContext(ctx, `UPDATE investment_review_episodes SET first_hit_on=?,triggered_on=?,window_start=?,window_end=?,latest_closed_on=?,hit_count=?,false_count=?,unknown_count=?,consecutive_count=?,evidence_json=?,evidence_fingerprint=?,revision=revision+1 WHERE id=? AND revision=? AND resolved_at IS NULL`, firstHit, current.TriggeredOn, item.LatestWindow.WindowStart, item.LatestWindow.WindowEnd, item.LatestClosedOn, item.LatestWindow.HitCount, item.LatestWindow.FalseCount, item.LatestWindow.UnknownCount, item.LatestWindow.ConsecutiveCount, payload, fingerprint, current.ID, stored.Revision)
	if err != nil {
		return err
	}
	count, err := changed.RowsAffected()
	if err != nil || count != 1 {
		return reviewRuleConflict("review episode changed during evidence update")
	}
	openedEvidence := evidence
	openedEvidence.Window = item.TriggerWindow
	openedEvidence.LatestResult = item.TriggerResult
	openedEvidence.Coverage = item.TriggerCoverage
	openedPayload, _, err := reviewEvidenceFingerprint(openedEvidence)
	if err != nil {
		return err
	}
	var previousOpenedPayload string
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM investment_review_episode_events WHERE episode_id=? AND kind='opened'`, current.ID).Scan(&previousOpenedPayload); err != nil {
		return err
	}
	if previousOpenedPayload != openedPayload {
		if _, err := tx.ExecContext(ctx, `UPDATE investment_review_episode_events SET payload_json=? WHERE episode_id=? AND kind='opened'`, openedPayload, current.ID); err != nil {
			return err
		}
	}
	if previousLatest == item.LatestClosedOn && (previousFingerprint != fingerprint || previousFirstHit != firstHit) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_episode_events(id,episode_id,event_key,kind,payload_json,created_at) VALUES(?,?,?,?,?,?)`, string(domain.NewReviewEpisodeID()), current.ID, "evidence/"+current.ID+"/"+strconv.Itoa(stored.Revision+1), "evidence_corrected", payload, atText); err != nil {
			return err
		}
	}
	return nil
}

func publishReviewEvaluationState(ctx context.Context, tx *sql.Tx, item domain.ReviewRulePublication, existingEpochs []string, bootstrap bool) error {
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
		stateSQL := `INSERT INTO investment_review_evaluation_state(rule_id,instrument_id,position_epoch_id,latest_closed_on,policy_state,window_start,window_end,hit_count,false_count,unknown_count,consecutive_count,episode_id,coverage_json,publication_revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,1) ON CONFLICT(rule_id,instrument_id,position_epoch_id) DO UPDATE SET latest_closed_on=excluded.latest_closed_on,policy_state=excluded.policy_state,window_start=excluded.window_start,window_end=excluded.window_end,hit_count=excluded.hit_count,false_count=excluded.false_count,unknown_count=excluded.unknown_count,consecutive_count=excluded.consecutive_count,episode_id=excluded.episode_id,coverage_json=excluded.coverage_json,publication_revision=investment_review_evaluation_state.publication_revision+1 WHERE latest_closed_on<>excluded.latest_closed_on OR policy_state<>excluded.policy_state OR window_start<>excluded.window_start OR window_end<>excluded.window_end OR hit_count<>excluded.hit_count OR false_count<>excluded.false_count OR unknown_count<>excluded.unknown_count OR consecutive_count<>excluded.consecutive_count OR episode_id IS NOT excluded.episode_id OR coverage_json<>excluded.coverage_json`
		if bootstrap {
			stateSQL = strings.Split(stateSQL, " WHERE ")[0] + ",dirty_from=NULL"
		}
		_, err := tx.ExecContext(ctx, stateSQL, string(item.RuleID), item.InstrumentID.String(), epochID, item.LatestClosedOn, state, window.WindowStart, window.WindowEnd, window.HitCount, window.FalseCount, window.UnknownCount, window.ConsecutiveCount, episodeID, string(coverage))
		if err != nil {
			return err
		}
	}
	if item.PositionKnown {
		validEpochs := make(map[string]struct{}, len(epochIDs))
		for _, epochID := range epochIDs {
			validEpochs[epochID] = struct{}{}
		}
		staleEpochs := []string{}
		for _, epochID := range existingEpochs {
			if _, ok := validEpochs[epochID]; !ok {
				staleEpochs = append(staleEpochs, epochID)
			}
		}
		for _, epochID := range staleEpochs {
			if _, err := tx.ExecContext(ctx, `DELETE FROM investment_review_evaluation_state WHERE rule_id=? AND instrument_id=? AND position_epoch_id=?`, string(item.RuleID), item.InstrumentID.String(), epochID); err != nil {
				return err
			}
		}
	}
	return nil
}

func reviewEvaluationStateKey(ruleID domain.ReviewRuleID, instrumentID domain.InstrumentID) string {
	return string(ruleID) + "\x00" + string(instrumentID)
}

// Existing epoch IDs are captured once in the publication transaction. The
// same consistent row set feeds stale-state cleanup for each target pair.
func reviewEvaluationStateEpochs(ctx context.Context, tx *sql.Tx, publications []domain.ReviewRulePublication) (map[string][]string, error) {
	ruleIDs := make([]string, 0)
	seen := make(map[domain.ReviewRuleID]struct{})
	for _, item := range publications {
		if item.TargetExcluded || !item.PositionKnown {
			continue
		}
		if _, ok := seen[item.RuleID]; ok {
			continue
		}
		seen[item.RuleID] = struct{}{}
		ruleIDs = append(ruleIDs, string(item.RuleID))
	}
	result := make(map[string][]string)
	if len(ruleIDs) == 0 {
		return result, nil
	}
	placeholders, args := sqlInArgs(ruleIDs)
	rows, err := tx.QueryContext(ctx, `SELECT rule_id,instrument_id,position_epoch_id FROM investment_review_evaluation_state WHERE rule_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ruleID, instrumentID, epochID string
		if err := rows.Scan(&ruleID, &instrumentID, &epochID); err != nil {
			return nil, err
		}
		key := ruleID + "\x00" + instrumentID
		result[key] = append(result[key], epochID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func reviewPublicationConflict(format string, values ...any) error {
	return reviewRuleConflict(fmt.Sprintf(format, values...))
}
