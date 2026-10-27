package sqlite

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// An episode may survive condition edits. Neither its opening version nor the
// current rule version identifies LatestResult. Prove the version from the
// persisted daily vote's complete canonical bytes; ambiguous/legacy evidence
// stays without condition context rather than borrowing a different predicate.
func reviewEvidenceVersion(ctx context.Context, q schemaQuery, ruleID, instrumentID string, result *domain.ReviewDailyResult) (domain.ReviewRuleSpec, int, bool, error) {
	var spec domain.ReviewRuleSpec
	if result == nil {
		return spec, 0, false, nil
	}
	copy := *result
	copy.Predicate = nil
	raw, err := jsoncodec.Marshal(copy)
	if err != nil {
		return spec, 0, false, err
	}
	rows, err := q.QueryContext(ctx, `SELECT d.condition_version,v.canonical_spec_json FROM investment_review_daily_results d JOIN investment_review_rule_versions v ON v.rule_id=d.rule_id AND v.condition_version=d.condition_version WHERE d.rule_id=? AND d.instrument_id=? AND d.on_date=? AND d.evidence_json=? LIMIT 2`, ruleID, instrumentID, result.On, string(raw))
	if err != nil {
		return spec, 0, false, err
	}
	defer rows.Close()
	version, count := 0, 0
	for rows.Next() {
		var candidate int
		var encoded string
		if err := rows.Scan(&candidate, &encoded); err != nil {
			return spec, 0, false, err
		}
		count++
		if count > 1 {
			continue
		}
		spec, err = domain.DecodeReviewRuleSpec([]byte(encoded))
		if err != nil {
			return spec, 0, false, err
		}
		version = candidate
	}
	if err := rows.Err(); err != nil {
		return spec, 0, false, err
	}
	return spec, version, count == 1, nil
}
