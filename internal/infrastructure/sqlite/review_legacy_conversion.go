package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ConvertReviewLegacyRuleV2 creates two independent daily rules and archives
// the old evaluator in one transaction. It does not copy irreversible v1
// acknowledgements to semantically different daily predicates.
func (r *Repository) ConvertReviewLegacyRuleV2(ctx context.Context, m domain.ReviewLegacyConversionMutation) ([]domain.ReviewRule, error) {
	if m.HouseholdID == "" || m.LegacyRuleID != legacyReviewRuleID(m.HouseholdID) || m.LowYieldRuleID == m.LossRuleID || m.LowYieldRuleID == m.LegacyRuleID || m.LossRuleID == m.LegacyRuleID || m.ExpectedRevision < 1 || m.ExpectedConfigRevision < 1 || m.ExpectedStateRevision < 0 || len(m.ExpectedSourceToken) != 64 || len(m.ExpectedTemporalToken) != 64 || m.At.IsZero() {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "conversion", Message: "a current legacy rule, distinct new IDs, revision and preview tokens are required"}
	}
	request, err := json.Marshal(struct {
		HouseholdID            domain.HouseholdID
		LegacyRuleID           domain.ReviewRuleID
		ExpectedRevision       int
		ExpectedConfigRevision int
		ExpectedStateRevision  int
		ExpectedSourceToken    string
		ExpectedTemporalToken  string
		LowYieldRuleID         domain.ReviewRuleID
		LossRuleID             domain.ReviewRuleID
		TargetKind             string
		Timezone               string
		Trigger                domain.ReviewTriggerPolicy
	}{m.HouseholdID, m.LegacyRuleID, m.ExpectedRevision, m.ExpectedConfigRevision, m.ExpectedStateRevision, m.ExpectedSourceToken, m.ExpectedTemporalToken, m.LowYieldRuleID, m.LossRuleID, m.TargetKind, m.Timezone, m.Trigger})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(request)
	requestHash := hex.EncodeToString(digest[:])
	tx, err := r.database.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var previousHash, previousJSON string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,result_json FROM investment_review_mutation_receipts WHERE household_id=? AND mutation_id=?`, m.HouseholdID.String(), string(m.MutationID)).Scan(&previousHash, &previousJSON)
	if err == nil {
		if previousHash != requestHash {
			return nil, reviewRuleConflict("conversion mutation ID was reused for a different request")
		}
		var previous []domain.ReviewRule
		if err := json.Unmarshal([]byte(previousJSON), &previous); err != nil {
			return nil, err
		}
		return previous, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	legacy, err := scanReviewRule(tx.QueryRowContext(ctx, reviewRuleSelect+` WHERE r.id=? AND r.household_id=?`, string(m.LegacyRuleID), m.HouseholdID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, reviewRuleConflict("legacy rule no longer exists")
	}
	if err != nil {
		return nil, err
	}
	if legacy.Evaluator != domain.ReviewLegacyEvaluator || legacy.LegacySpec == nil || legacy.ArchivedAt != nil || legacy.RowRevision != m.ExpectedRevision {
		return nil, reviewRuleConflict("legacy rule changed; preview conversion again")
	}
	var configRevision, stateRevision int
	if err := tx.QueryRowContext(ctx, `SELECT config_revision,review_state_revision FROM investment_review_control WHERE household_id=?`, m.HouseholdID.String()).Scan(&configRevision, &stateRevision); err != nil {
		return nil, err
	}
	if configRevision != m.ExpectedConfigRevision || stateRevision != m.ExpectedStateRevision {
		return nil, reviewRuleConflict("review handling changed; preview conversion again")
	}
	lowSpec, lossSpec, err := domain.BuildReviewLegacyConversion(*legacy.LegacySpec, m.TargetKind, m.Timezone, m.Trigger)
	if err != nil {
		return nil, err
	}
	for _, spec := range []domain.ReviewRuleSpec{lowSpec, lossSpec} {
		if err := reviewRuleInstrumentMembership(ctx, tx, m.HouseholdID, spec); err != nil {
			return nil, err
		}
	}
	// The preview token covers authoritative financial inputs as of the save
	// time. A change makes the promised new-alert count stale.
	actual, err := loadReviewHistoricalBatchQuery(ctx, tx, m.HouseholdID, m.At)
	if err != nil {
		return nil, err
	}
	token, err := domain.ReviewHistoricalFingerprint(actual)
	if err != nil {
		return nil, err
	}
	if token != m.ExpectedSourceToken {
		return nil, reviewRuleConflict("review evidence changed; preview conversion again")
	}
	suppressions, err := reviewSuppressionsInTx(ctx, tx, m.HouseholdID)
	if err != nil {
		return nil, err
	}
	temporalToken, err := domain.ReviewLegacyConversionTemporalToken(m.At, m.Timezone, actual.History.Origin.Timezone, suppressions)
	if err != nil {
		return nil, err
	}
	if temporalToken != m.ExpectedTemporalToken {
		return nil, reviewRuleConflict("review day or ignore expiry changed; preview conversion again")
	}
	stamp := m.At.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE investment_review_rules SET archived_at=?,updated_at=?,enabled=0,row_revision=row_revision+1 WHERE id=? AND row_revision=? AND archived_at IS NULL`, stamp, stamp, string(m.LegacyRuleID), m.ExpectedRevision); err != nil {
		return nil, err
	}
	proposals := []struct {
		ID   domain.ReviewRuleID
		Name string
		Spec domain.ReviewRuleSpec
	}{{m.LowYieldRuleID, "Low yield (converted)", lowSpec}, {m.LossRuleID, "Loss (converted)", lossSpec}}
	result := make([]domain.ReviewRule, 0, 2)
	for _, proposal := range proposals {
		serialized, err := json.Marshal(proposal.Spec)
		if err != nil {
			return nil, err
		}
		hash, err := proposal.Spec.SemanticHash()
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_rules(id,household_id,name,enabled,severity,evaluator,row_revision,current_condition_version,created_at,updated_at) VALUES(?,?,?,?,?,'daily_predicate_v1',1,1,?,?)`, string(proposal.ID), m.HouseholdID.String(), proposal.Name, legacy.Enabled, legacy.Severity, stamp, stamp); err != nil {
			return nil, mapSQLiteError(err)
		}
		if err := insertReviewRuleVersion(ctx, tx, proposal.ID, 1, serialized, hash, stamp, proposal.Spec.Target); err != nil {
			return nil, err
		}
		created, err := scanReviewRule(tx.QueryRowContext(ctx, reviewRuleSelect+` WHERE r.id=?`, string(proposal.ID)))
		if err != nil {
			return nil, err
		}
		result = append(result, created)
	}
	// Old absolute snooze dates keep their original history-origin timezone.
	// Only a matching new rule scope is installed; permanent v2 suppressions
	// already survive independently and are never cleared here.
	oldZone := actual.History.Origin.Timezone
	if oldZone == "" {
		oldZone = m.Timezone
	}
	for rawID, until := range legacy.LegacySpec.Configuration.SnoozedUntil {
		if _, err := domain.ReviewCivilCutoff(until, oldZone); err != nil {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "snoozeDate", Message: "legacy resume day is not a real local date; resolve before conversion"}
		}
		instrumentID, err := domain.ParseInstrumentID(rawID)
		if err != nil {
			return nil, err
		}
		if lowSpec.Target.Kind == "instruments" {
			included := false
			for _, id := range lowSpec.Target.InstrumentIDs {
				included = included || id == instrumentID
			}
			if !included {
				continue
			}
		}
		var exists int
		err = tx.QueryRowContext(ctx, `SELECT 1 FROM instruments WHERE id=? AND household_id=?`, rawID, m.HouseholdID.String()).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			continue // preserved in the archived raw source
		}
		if err != nil {
			return nil, err
		}
		for _, proposal := range proposals {
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_suppressions(id,household_id,instrument_id,rule_id,mode,until_on,timezone,revision,created_at,updated_at) VALUES(?,?,?,?,'until_date',?,?,1,?,?)`, string(domain.NewReviewSuppressionID()), m.HouseholdID.String(), rawID, string(proposal.ID), until, oldZone, stamp, stamp); err != nil {
				return nil, err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE investment_review_control SET config_revision=config_revision+1,review_state_revision=review_state_revision+1,projection_status='pending' WHERE household_id=?`, m.HouseholdID.String()); err != nil {
		return nil, err
	}
	serializedResult, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_mutation_receipts(household_id,mutation_id,request_hash,result_json,created_at) VALUES(?,?,?,?,?)`, m.HouseholdID.String(), string(m.MutationID), requestHash, string(serializedResult), stamp); err != nil {
		return nil, err
	}
	return result, tx.Commit()
}
