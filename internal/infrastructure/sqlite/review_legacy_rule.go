package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func legacyReviewRuleID(householdID domain.HouseholdID) domain.ReviewRuleID {
	return domain.ReviewRuleID(uuid.NewSHA1(uuid.NameSpaceURL, []byte("nestworth/review/legacy_v1/"+householdID.String())).String())
}

func legacyReviewHousehold(key string) (domain.HouseholdID, bool) {
	if !strings.HasPrefix(key, "investment_review/") {
		return "", false
	}
	id, err := domain.ParseHouseholdID(strings.TrimPrefix(key, "investment_review/"))
	return id, err == nil && key == "investment_review/"+id.String()
}

// upsertReviewLegacyRule is retained unchanged for the v18-to-v19 migration.
// Its result is hidden from live V2 reads by the evaluator filter.
func upsertReviewLegacyRule(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, raw []byte, at time.Time) error {
	configuration, err := domain.DecodeReviewLegacyConfiguration(raw)
	if err != nil {
		return err
	}
	id := legacyReviewRuleID(householdID)
	// Embed the original object bytes intact. Legacy IDs and acknowledgement
	// aliases cannot be reconstructed from a normalized re-serialization.
	specJSON := []byte(`{"specVersion":1,"evaluator":"legacy_v1","configuration":` + string(raw) + `}`)
	digest := sha256.Sum256(specJSON)
	semanticHash := hex.EncodeToString(digest[:])
	var currentVersion, currentRevision int
	var previousHash string
	var archived sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT r.current_condition_version,r.row_revision,v.semantic_hash,r.archived_at FROM investment_review_rules r JOIN investment_review_rule_versions v ON v.rule_id=r.id AND v.condition_version=r.current_condition_version WHERE r.id=? AND r.household_id=? AND r.evaluator=?`, string(id), householdID.String(), domain.ReviewLegacyEvaluator).Scan(&currentVersion, &currentRevision, &previousHash, &archived)
	if errors.Is(err, sql.ErrNoRows) {
		stamp := at.UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_rules(id,household_id,name,enabled,severity,evaluator,row_revision,current_condition_version,created_at,updated_at) VALUES(?,?,?,?,'warning',?,1,1,?,?)`, string(id), householdID.String(), "Legacy investment review", configuration.Policy.Enabled, domain.ReviewLegacyEvaluator, stamp, stamp); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_rule_versions(rule_id,condition_version,spec_version,canonical_spec_json,semantic_hash,created_at) VALUES(?,1,1,?,?,?)`, string(id), string(specJSON), semanticHash, stamp); err != nil {
			return err
		}
	} else {
		if err != nil {
			return err
		}
		if archived.Valid {
			return &domain.Error{Code: domain.ErrConflict, Message: "legacy review rule was converted and archived"}
		}
		if previousHash == semanticHash {
			return nil
		}
		stamp := at.UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE investment_review_rules SET enabled=?,row_revision=row_revision+1,current_condition_version=?,updated_at=? WHERE id=? AND row_revision=?`, configuration.Policy.Enabled, currentVersion+1, stamp, string(id), currentRevision); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_rule_versions(rule_id,condition_version,spec_version,canonical_spec_json,semantic_hash,created_at) VALUES(?,?,?,?,?,?)`, string(id), currentVersion+1, 1, string(specJSON), semanticHash, stamp); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM investment_review_rule_targets WHERE rule_id=?`, string(id)); err != nil {
			return err
		}
	}
	for rawID, enrolled := range configuration.Enrolled {
		if !enrolled {
			continue
		}
		// A deleted old instrument remains in the immutable source but cannot
		// become a live target with a foreign key to a missing row.
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM instruments WHERE id=? AND household_id=?`, rawID, householdID.String()).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_rule_targets(rule_id,instrument_id) VALUES(?,?)`, string(id), rawID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_control(household_id) VALUES(?) ON CONFLICT(household_id) DO NOTHING`, householdID.String()); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE investment_review_control SET config_revision=config_revision+1 WHERE household_id=?`, householdID.String())
	return err
}

func (r *ConfigurationRepository) loadLegacyReviewConfiguration(householdID domain.HouseholdID) ([]byte, bool, error) {
	var specJSON string
	var archived sql.NullString
	err := r.database.SQL.QueryRow(`SELECT v.canonical_spec_json,r.archived_at FROM investment_review_rules r JOIN investment_review_rule_versions v ON v.rule_id=r.id AND v.condition_version=r.current_condition_version WHERE r.id=? AND r.household_id=? AND r.evaluator=?`, string(legacyReviewRuleID(householdID)), householdID.String(), domain.ReviewLegacyEvaluator).Scan(&specJSON, &archived)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	spec, err := domain.DecodeReviewLegacySpec([]byte(specJSON))
	if err != nil {
		return nil, false, fmt.Errorf("stored legacy review rule is invalid: %w", err)
	}
	if !archived.Valid {
		var source struct {
			Configuration json.RawMessage `json:"configuration"`
		}
		if err := jsoncodec.Unmarshal([]byte(specJSON), &source); err != nil {
			return nil, false, err
		}
		return append([]byte(nil), source.Configuration...), true, nil
	}
	spec.Configuration.Policy.Enabled = false
	data, err := jsoncodec.Marshal(spec.Configuration)
	return data, err == nil, err
}
