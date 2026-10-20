package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// E2a uses an existing daily input row as the complete, atomic current selector.
// E1 samples are never consulted. base_v1 in this distinct semantic namespace
// contains one COMPLETE input, avoiding independently selected base/metric pairs.
const reviewSharedInputVersion = "e2a-complete-input-v1"
const reviewSharedInputDays = 64

type reviewSharedInputRef struct{ SharedFactV1 string }

// WithReviewSharedInputs enables bounded production-format writes on this copy.
// Read compatibility is unconditional. Normal initialization leaves writes off.
func (r *Repository) WithReviewSharedInputs(enabled bool) *Repository {
	copy := *r
	copy.reviewSharedInputs = enabled
	return &copy
}

func sharedInputKey(p domain.ReviewPublication, rule domain.ReviewRule, day domain.ReviewPublicationDay) (string, error) {
	return reviewFactKeyVersion(p, rule, day, reviewSharedInputVersion)
}

type reviewSharedInputPlan map[domain.ReviewDailyEvidenceKey]string

func sharedDayKey(item domain.ReviewRulePublication, day domain.ReviewPublicationDay) domain.ReviewDailyEvidenceKey {
	return domain.ReviewDailyEvidenceKey{RuleID: item.RuleID, ConditionVersion: item.ConditionVersion, InstrumentID: item.InstrumentID, TimezoneVersion: item.TimezoneVersion, On: day.On}
}

// Admission is all-or-nothing for this small full publication, before inserts.
// Unsupported/nonexistent dates remain inline. No retained prefix is converted.
func (r *Repository) publishSharedInputs(ctx context.Context, tx *sql.Tx, p domain.ReviewPublication) (reviewSharedInputPlan, error) {
	plan := reviewSharedInputPlan{}
	if !r.reviewSharedInputs || r.reviewFullRecompute || p.Capture.Suffix != nil || len(p.Capture.ReusableDailyEvidence) > 0 || p.CertifiedFullOracle != nil {
		return plan, nil
	}
	rules := map[domain.ReviewRuleID]domain.ReviewRule{}
	for _, rule := range p.Capture.Rules {
		rules[rule.ID] = rule
	}
	source, err := jsoncodec.Marshal(struct {
		Fingerprint string
		Checkpoint  *domain.ReviewSourceCheckpoint
	}{p.Capture.HistoricalFingerprint, p.Capture.SourceRevision})
	if err != nil {
		return nil, err
	}
	groups := map[string][]reviewFactSample{}
	count, budget := 0, 0
	seen := map[domain.ReviewDailyEvidenceKey]bool{}
	for _, item := range p.Rules {
		for _, day := range item.Days {
			keyID := sharedDayKey(item, day)
			if seen[keyID] {
				return plan, nil
			}
			seen[keyID] = true
			count++
			if count > reviewSharedInputDays {
				return plan, nil
			}
			rule, ok := rules[item.RuleID]
			if !ok {
				return nil, fmt.Errorf("shared input rule missing")
			}
			key, err := sharedInputKey(p, rule, day)
			if domain.ReviewNonexistentCivilDate(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			payload, err := encodeCompleteReviewInput(day.Input)
			if err != nil {
				return nil, err
			}
			coverage, err := jsoncodec.Marshal(day.CoverageMissing)
			if err != nil {
				return nil, err
			}
			provenance, err := reviewFactSource(p, day, source, rule.Spec.Schedule.Timezone)
			if err != nil {
				return nil, err
			}
			// Length-delimited JSON grouping compares all actual bytes, not a hash.
			group, err := jsoncodec.Marshal([]string{item.InstrumentID.String(), key, string(provenance), string(payload), string(coverage)})
			if err != nil {
				return nil, err
			}
			groups[string(group)] = append(groups[string(group)], reviewFactSample{item: item, day: day, rule: rule, key: key, base: payload, coverage: coverage, source: provenance})
			budget += len(group) + 512
			if budget > reviewFactBatchBytes {
				return plan, nil
			}
		}
	}
	var rows, refs, stored int
	if err := tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(length(CAST(id||household_id||instrument_id||semantic_key||fact_kind||source_version||payload_json||coverage_json||created_at AS BLOB))),0) FROM review_financial_daily_facts`).Scan(&rows, &stored); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM review_result_fact_refs`).Scan(&refs); err != nil {
		return nil, err
	}
	// Conservative lifetime admission includes diagnostics and worst-case refs.
	var refBytes int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(length(CAST(rule_id||condition_version||instrument_id||timezone_version||on_date||fact_id||input_fingerprint AS BLOB))),0) FROM review_result_fact_refs`).Scan(&refBytes); err != nil {
		return nil, err
	}
	if rows+count > reviewFactMaxRows || refs+count > reviewFactMaxRefs || stored+refBytes+budget > reviewFactMaxBytes {
		return plan, nil
	}
	for _, samples := range groups {
		if len(samples) < 2 || samples[0].item.RuleID == samples[1].item.RuleID {
			continue
		}
		first := samples[0]
		id, _, err := writeReviewFactVersion(ctx, tx, p, first, "base_v1", string(first.source), first.base)
		if err != nil {
			return nil, err
		}
		raw, err := jsoncodec.Marshal(reviewSharedInputRef{SharedFactV1: id})
		if err != nil {
			return nil, err
		}
		for _, s := range samples {
			if _, err := tx.ExecContext(ctx, `INSERT INTO review_result_fact_refs(rule_id,condition_version,instrument_id,timezone_version,on_date,fact_id,input_fingerprint) VALUES(?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`, string(s.item.RuleID), s.item.ConditionVersion, s.item.InstrumentID.String(), s.item.TimezoneVersion, s.day.On, id, s.day.InputFingerprint); err != nil {
				return nil, err
			}
			// Independent persisted readback verifies ref/namespace and exact payload.
			resolver := reviewSharedInputResolver{ctx: ctx, tx: tx, household: p.Capture.HouseholdID}
			actual, err := resolver.resolve(string(raw), sharedDayKey(s.item, s.day), string(s.coverage), s.day.InputFingerprint)
			if err != nil {
				return nil, err
			}
			if actual != string(s.base) {
				return nil, fmt.Errorf("shared input readback mismatch")
			}
			var decoded domain.ReviewDailyInput
			if err := decodeReviewInput([]byte(actual), &decoded); err != nil {
				return nil, err
			}
			result, err := domain.EvaluateReviewDaily(s.rule.Spec, decoded)
			if err != nil {
				return nil, err
			}
			got, err := jsoncodec.Marshal(result)
			if err != nil {
				return nil, err
			}
			want, err := jsoncodec.Marshal(s.day.Result)
			if err != nil {
				return nil, err
			}
			if string(got) != string(want) {
				return nil, fmt.Errorf("shared input result mismatch")
			}
			plan[sharedDayKey(s.item, s.day)] = string(raw)
		}
	}
	return plan, nil
}

type reviewSharedInputResolver struct {
	ctx       context.Context
	tx        queryer
	household domain.HouseholdID
}

func (r *reviewSharedInputResolver) resolve(raw string, key domain.ReviewDailyEvidenceKey, coverage, fingerprint string) (string, error) {
	// Legacy complete inputs never contain this reserved first-party field.
	if !strings.Contains(raw, `"SharedFactV1"`) {
		return raw, nil
	}
	var ref reviewSharedInputRef
	if err := jsoncodec.Unmarshal([]byte(raw), &ref); err != nil || ref.SharedFactV1 == "" {
		return "", fmt.Errorf("invalid shared input selector")
	}
	canonical, err := jsoncodec.Marshal(ref)
	if err != nil || string(canonical) != raw {
		return "", fmt.Errorf("noncanonical shared input selector")
	}
	if r == nil || r.tx == nil {
		return "", fmt.Errorf("shared input resolver unavailable")
	}
	var payload, storedCoverage, semantic, source string
	err = r.tx.QueryRowContext(r.ctx, `SELECT f.payload_json,f.coverage_json,f.semantic_key,f.source_version FROM review_financial_daily_facts f JOIN review_result_fact_refs x ON x.fact_id=f.id JOIN investment_review_rules rule ON rule.id=x.rule_id WHERE f.id=? AND f.household_id=? AND rule.household_id=f.household_id AND f.instrument_id=? AND f.fact_kind='base_v1' AND x.rule_id=? AND x.condition_version=? AND x.instrument_id=f.instrument_id AND x.timezone_version=? AND x.on_date=? AND x.input_fingerprint=?`, ref.SharedFactV1, r.household.String(), key.InstrumentID.String(), string(key.RuleID), key.ConditionVersion, key.TimezoneVersion, key.On, fingerprint).Scan(&payload, &storedCoverage, &semantic, &source)
	if err != nil {
		return "", fmt.Errorf("shared input reference integrity: %w", err)
	}
	var namespace reviewFactSemanticKey
	if err := jsoncodec.Unmarshal([]byte(semantic), &namespace); err != nil {
		return "", err
	}
	if namespace.Version != reviewSharedInputVersion || namespace.On != key.On || namespace.Schedule.Timezone+"/"+namespace.Schedule.CutoffVersion != key.TimezoneVersion || storedCoverage != coverage || source == "" {
		return "", fmt.Errorf("shared input namespace integrity")
	}
	return payload, nil
}
