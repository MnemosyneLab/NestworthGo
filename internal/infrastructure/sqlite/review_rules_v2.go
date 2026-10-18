package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const reviewRuleSelect = `SELECT r.id,r.household_id,r.name,r.enabled,r.severity,r.evaluator,r.row_revision,r.current_condition_version,r.created_at,r.updated_at,r.archived_at,v.canonical_spec_json,v.semantic_hash
 FROM investment_review_rules r JOIN investment_review_rule_versions v ON v.rule_id=r.id AND v.condition_version=r.current_condition_version`

func scanReviewRule(row rowScanner) (domain.ReviewRule, error) {
	var result domain.ReviewRule
	var id, household, specJSON, created, updated string
	var archived sql.NullString
	var enabled int
	if err := row.Scan(&id, &household, &result.Name, &enabled, &result.Severity, &result.Evaluator, &result.RowRevision, &result.CurrentConditionVersion, &created, &updated, &archived, &specJSON, &result.SemanticHash); err != nil {
		return result, err
	}
	parsed, err := domain.ParseReviewRuleID(id)
	if err != nil {
		return result, err
	}
	result.ID = parsed
	result.HouseholdID, err = domain.ParseHouseholdID(household)
	if err != nil {
		return result, err
	}
	result.Enabled = enabled != 0
	if result.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return result, err
	}
	if result.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return result, err
	}
	if archived.Valid {
		at, err := time.Parse(time.RFC3339Nano, archived.String)
		if err != nil {
			return result, err
		}
		result.ArchivedAt = &at
	}
	result.Spec, err = domain.DecodeReviewRuleSpec([]byte(specJSON))
	return result, err
}

func (r *Repository) ListReviewRulesV2(ctx context.Context, householdID domain.HouseholdID, after string, limit int) ([]domain.ReviewRule, error) {
	if limit < 1 || limit > 100 {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "pageSize", Message: "use 1–100 rules per page"}
	}
	if after != "" {
		if _, err := domain.ParseReviewRuleID(after); err != nil {
			return nil, err
		}
	}
	rows, err := r.database.SQL.QueryContext(ctx, reviewRuleSelect+` WHERE r.household_id=? AND r.evaluator=? AND r.id>? ORDER BY r.id LIMIT ?`, householdID.String(), domain.ReviewEvaluator, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.ReviewRule, 0, limit)
	for rows.Next() {
		item, err := scanReviewRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) MutateReviewRuleV2(ctx context.Context, m domain.ReviewRuleMutation) (domain.ReviewRule, error) {
	var empty domain.ReviewRule
	if m.Operation != "create" && m.Operation != "update" && m.Operation != "archive" {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "operation", Message: "unsupported rule mutation"}
	}
	if _, err := domain.ParseHouseholdID(m.HouseholdID.String()); err != nil {
		return empty, err
	}
	if _, err := domain.ParseReviewRuleID(string(m.RuleID)); err != nil {
		return empty, err
	}
	if _, err := domain.ParseMutationID(string(m.MutationID)); err != nil {
		return empty, err
	}
	var specJSON []byte
	var semanticHash string
	if m.Operation != "archive" {
		var err error
		m.Spec, err = m.Spec.Normalize()
		if err != nil {
			return empty, err
		}
		semanticHash, err = m.Spec.SemanticHash()
		if err != nil {
			return empty, err
		}
		specJSON, err = json.Marshal(m.Spec)
		if err != nil {
			return empty, err
		}
	}
	request, err := json.Marshal(struct {
		Operation        string
		HouseholdID      domain.HouseholdID
		RuleID           domain.ReviewRuleID
		MutationID       domain.MutationID
		ExpectedRevision int
		Name             string
		Enabled          bool
		Severity         string
		SemanticHash     string
	}{m.Operation, m.HouseholdID, m.RuleID, m.MutationID, m.ExpectedRevision, m.Name, m.Enabled, m.Severity, semanticHash})
	if err != nil {
		return empty, err
	}
	digest := sha256.Sum256(request)
	requestHash := hex.EncodeToString(digest[:])
	tx, err := r.database.SQL.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var previousHash, previousJSON string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,result_json FROM investment_review_mutation_receipts WHERE household_id=? AND mutation_id=?`, m.HouseholdID.String(), string(m.MutationID)).Scan(&previousHash, &previousJSON)
	if err == nil {
		if previousHash != requestHash {
			return empty, reviewRuleConflict("mutation ID was already used for a different request")
		}
		var previous domain.ReviewRule
		if err := json.Unmarshal([]byte(previousJSON), &previous); err != nil {
			return empty, err
		}
		if err := tx.Commit(); err != nil {
			return empty, err
		}
		return previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_control(household_id) VALUES(?) ON CONFLICT(household_id) DO NOTHING`, m.HouseholdID.String()); err != nil {
		return empty, err
	}
	var result domain.ReviewRule
	changed := false
	switch m.Operation {
	case "create":
		if m.ExpectedRevision != 0 {
			return empty, reviewRuleConflict("new rule must start at revision zero")
		}
		if err := reviewRuleInstrumentMembership(ctx, tx, m.HouseholdID, m.Spec); err != nil {
			return empty, err
		}
		at := m.At.UTC().Format(time.RFC3339Nano)
		_, err = tx.ExecContext(ctx, `INSERT INTO investment_review_rules(id,household_id,name,enabled,severity,evaluator,row_revision,current_condition_version,created_at,updated_at) VALUES(?,?,?,?,?,'daily_predicate_v1',1,1,?,?)`, string(m.RuleID), m.HouseholdID.String(), m.Name, m.Enabled, m.Severity, at, at)
		if err != nil {
			return empty, mapSQLiteError(err)
		}
		if err := insertReviewRuleVersion(ctx, tx, m.RuleID, 1, specJSON, semanticHash, at, m.Spec.Target); err != nil {
			return empty, err
		}
		changed = true
	case "update", "archive":
		current, err := scanReviewRule(tx.QueryRowContext(ctx, reviewRuleSelect+` WHERE r.household_id=? AND r.evaluator=? AND r.id=?`, m.HouseholdID.String(), domain.ReviewEvaluator, string(m.RuleID)))
		if errors.Is(err, sql.ErrNoRows) {
			return empty, reviewRuleConflict("rule no longer exists")
		}
		if err != nil {
			return empty, err
		}
		if current.ArchivedAt != nil || current.RowRevision != m.ExpectedRevision {
			return empty, reviewRuleConflict("rule changed; reload before saving")
		}
		if current.Evaluator != domain.ReviewEvaluator {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "ruleId", Message: "legacy rules require explicit conversion"}
		}
		if m.Operation == "archive" {
			at := m.At.UTC().Format(time.RFC3339Nano)
			if _, err := tx.ExecContext(ctx, `UPDATE investment_review_rules SET archived_at=?,updated_at=?,row_revision=row_revision+1 WHERE id=? AND row_revision=?`, at, at, string(m.RuleID), m.ExpectedRevision); err != nil {
				return empty, err
			}
			changed = true
		} else if current.Name != m.Name || current.Enabled != m.Enabled || current.Severity != m.Severity || current.SemanticHash != semanticHash {
			if err := reviewRuleInstrumentMembership(ctx, tx, m.HouseholdID, m.Spec); err != nil {
				return empty, err
			}
			version := current.CurrentConditionVersion
			if current.SemanticHash != semanticHash {
				version++
			}
			at := m.At.UTC().Format(time.RFC3339Nano)
			if _, err := tx.ExecContext(ctx, `UPDATE investment_review_rules SET name=?,enabled=?,severity=?,current_condition_version=?,updated_at=?,row_revision=row_revision+1 WHERE id=? AND row_revision=?`, m.Name, m.Enabled, m.Severity, version, at, string(m.RuleID), m.ExpectedRevision); err != nil {
				return empty, err
			}
			if version != current.CurrentConditionVersion {
				if err := insertReviewRuleVersion(ctx, tx, m.RuleID, version, specJSON, semanticHash, at, m.Spec.Target); err != nil {
					return empty, err
				}
				if _, err := tx.ExecContext(ctx, `DELETE FROM investment_review_rule_targets WHERE rule_id=?`, string(m.RuleID)); err != nil {
					return empty, err
				}
				if err := insertReviewRuleTargets(ctx, tx, m.RuleID, m.Spec.Target); err != nil {
					return empty, err
				}
			}
			changed = true
		}
	}
	if changed {
		if _, err := tx.ExecContext(ctx, `UPDATE investment_review_control SET config_revision=config_revision+1,projection_status='pending' WHERE household_id=?`, m.HouseholdID.String()); err != nil {
			return empty, err
		}
	}
	result, err = scanReviewRule(tx.QueryRowContext(ctx, reviewRuleSelect+` WHERE r.household_id=? AND r.evaluator=? AND r.id=?`, m.HouseholdID.String(), domain.ReviewEvaluator, string(m.RuleID)))
	if err != nil {
		return empty, err
	}
	serialized, err := json.Marshal(result)
	if err != nil {
		return empty, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_mutation_receipts(household_id,mutation_id,request_hash,result_json,created_at) VALUES(?,?,?,?,?)`, m.HouseholdID.String(), string(m.MutationID), requestHash, string(serialized), m.At.UTC().Format(time.RFC3339Nano)); err != nil {
		return empty, err
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return result, nil
}

func reviewRuleConflict(message string) error {
	return &domain.Error{Code: domain.ErrConflict, Message: message}
}

func reviewRuleInstrumentMembership(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, spec domain.ReviewRuleSpec) error {
	unique := map[domain.InstrumentID]bool{}
	for _, id := range spec.Target.InstrumentIDs {
		unique[id] = true
	}
	var collect func(domain.ReviewPredicateNode)
	collect = func(node domain.ReviewPredicateNode) {
		if node.Leaf != nil {
			for _, id := range node.Leaf.InstrumentIDs {
				unique[id] = true
			}
			return
		}
		for _, child := range node.All {
			collect(child)
		}
		for _, child := range node.Any {
			collect(child)
		}
	}
	collect(spec.Predicate)
	if len(unique) == 0 {
		return nil
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id.String())
	}
	marks, args := sqlInArgs(ids)
	queryArgs := append([]any{householdID.String()}, args...)
	var found int
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT count(*) FROM instruments WHERE household_id=? AND id IN (%s)`, marks), queryArgs...).Scan(&found); err != nil {
		return err
	}
	if found != len(ids) {
		return &domain.Error{Code: domain.ErrValidation, Field: "target", Message: "every selected instrument must belong to this household"}
	}
	return nil
}

func insertReviewRuleVersion(ctx context.Context, tx *sql.Tx, ruleID domain.ReviewRuleID, version int, specJSON []byte, semanticHash, at string, target domain.ReviewRuleTarget) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_rule_versions(rule_id,condition_version,spec_version,canonical_spec_json,semantic_hash,created_at) VALUES(?,?,?,?,?,?)`, string(ruleID), version, domain.ReviewSpecVersion, string(specJSON), semanticHash, at); err != nil {
		return err
	}
	if version == 1 {
		return insertReviewRuleTargets(ctx, tx, ruleID, target)
	}
	return nil
}
func insertReviewRuleTargets(ctx context.Context, tx *sql.Tx, ruleID domain.ReviewRuleID, target domain.ReviewRuleTarget) error {
	if target.Kind != "instruments" {
		return nil
	}
	for _, id := range target.InstrumentIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_rule_targets(rule_id,instrument_id) VALUES(?,?)`, string(ruleID), id.String()); err != nil {
			return err
		}
	}
	return nil
}
