package sqlite

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// A new handle has no authority over frozen evidence. Support only a namespace
// with no lifecycle history and one active first condition per daily rule.
// Such a namespace contains only recomputable projections. The signed flag
// permits complete replacement, never bounded preparation or confirmation.
func certifiedBootstrapSupported(ctx context.Context, tx *sql.Tx, c domain.ReviewProjectionCapture, counts []int64) (bool, error) {
	if len(counts) != 6 || counts[4] != 0 || counts[5] != 0 || len(c.OpenEpisodes) != 0 || len(c.Rules) == 0 || !c.Control.Enabled || c.Control.ProjectionStatus == "needs_repair" {
		return false, nil
	}
	for _, rule := range c.Rules {
		if rule.Evaluator != domain.ReviewEvaluator || !rule.Enabled || rule.ArchivedAt != nil || rule.CurrentConditionVersion != 1 {
			return false, nil
		}
	}
	var allRules, allVersions int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM investment_review_rules WHERE household_id=?`, c.HouseholdID.String()).Scan(&allRules); err != nil {
		return false, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM investment_review_rule_versions v JOIN investment_review_rules r ON r.id=v.rule_id WHERE r.household_id=?`, c.HouseholdID.String()).Scan(&allVersions); err != nil {
		return false, err
	}
	return allRules == len(c.Rules) && allVersions == len(c.Rules), nil
}

// Every semantic column of these four tables was just unconditionally written
// from full source preparation. Exact key sets rule out extra retained rows in
// any older policy, target, timezone, condition or epoch namespace. No old
// projection is deleted to make the check pass. Unknown history keeps fallback.
func verifyCertifiedBootstrapNamespace(ctx context.Context, tx *sql.Tx, p domain.ReviewPublication) (bool, error) {
	sets := []map[[5]string]bool{{}, {}, {}, {}}
	for _, item := range p.Rules {
		if item.TargetExcluded {
			continue
		}
		policy := "rule:" + string(item.RuleID) + "/v:" + strconv.Itoa(item.ConditionVersion)
		for _, day := range item.Days {
			sets[0][[5]string{string(item.InstrumentID), item.TimezoneVersion, day.On, policy}] = true
			sets[1][[5]string{string(item.RuleID), strconv.Itoa(item.ConditionVersion), string(item.InstrumentID), item.TimezoneVersion, day.On}] = true
		}
		for _, epoch := range item.Epochs {
			sets[2][[5]string{epoch.ID}] = true
			sets[3][[5]string{string(item.RuleID), string(item.InstrumentID), epoch.ID}] = true
		}
		if item.CurrentEpochID != "" {
			sets[3][[5]string{string(item.RuleID), string(item.InstrumentID), item.CurrentEpochID}] = true
		}
	}
	queries := []string{
		`SELECT instrument_id,timezone_version,on_date,input_policy,'' FROM investment_review_daily_inputs WHERE household_id=?`,
		`SELECT d.rule_id,CAST(d.condition_version AS TEXT),d.instrument_id,d.timezone_version,d.on_date FROM investment_review_daily_results d JOIN investment_review_rules r ON r.id=d.rule_id WHERE r.household_id=?`,
		`SELECT id,'','','','' FROM investment_review_position_epochs WHERE household_id=?`,
		`SELECT s.rule_id,s.instrument_id,s.position_epoch_id,'','' FROM investment_review_evaluation_state s JOIN investment_review_rules r ON r.id=s.rule_id WHERE r.household_id=?`,
	}
	for i, query := range queries {
		rows, err := tx.QueryContext(ctx, query, p.Capture.HouseholdID.String())
		if err != nil {
			return false, err
		}
		matched := true
		for rows.Next() {
			var key [5]string
			if err := rows.Scan(&key[0], &key[1], &key[2], &key[3], &key[4]); err != nil {
				rows.Close()
				return false, err
			}
			if !sets[i][key] {
				matched = false
			}
			delete(sets[i], key)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return false, err
		}
		if !matched || len(sets[i]) != 0 {
			return false, nil
		}
	}
	return true, nil
}
