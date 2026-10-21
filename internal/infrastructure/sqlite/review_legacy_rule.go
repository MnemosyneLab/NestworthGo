package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

// Supported 21/22 databases may retain legacy_v1 records and immutable source
// bytes. Keep this recovery reader; their format is independent
// of SQLite schema support.
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
