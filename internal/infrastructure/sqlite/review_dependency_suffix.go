package sqlite

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// WithReviewDependencySuffix enables D1/D2 and C3 confirmation. It never grants
// legacy/restart authority. WithFullReviewRecompute remains the complete oracle.
func (r *Repository) WithReviewDependencySuffix() *Repository {
	copy := *r.WithCertifiedReviewCandidate()
	copy.reviewDependencySuffix = true
	return &copy
}

func (r *Repository) captureReviewSuffix(ctx context.Context, tx *sql.Tx, c *domain.ReviewProjectionCapture) error {
	if !r.reviewDependencySuffix || !r.CertifiedReviewEnabled() || r.ReviewCompletenessLineageShadowEnabled() {
		return nil
	}
	p := c.PublishedTemporal
	proof := c.Certified
	if p == nil || proof == nil || !proof.MayCertify || !r.authenticReview(p, proof.ProjectionSeal) {
		return nil
	}
	if p.PolicyVersion != domain.ReviewTemporalPolicyVersion || p.DependencyScope != domain.ReviewTemporalDependencyScope || !p.NextTransitionAt.After(p.EvaluatedAt) || p.ClockEpoch != c.Temporal.ClockEpoch || c.SourceCutoff.Before(p.EvaluatedAt) {
		return nil
	}
	if p.Source == nil || c.SourceRevision == nil || p.NoChangeReviewFingerprint != proof.ReviewFingerprint || p.ConfigRevision != c.Control.ConfigRevision || p.StateRevision != c.Control.ReviewStateRevision || len(p.RuleRevisions) != len(c.Rules) || c.Control.ProjectionStatus != "complete" {
		return nil
	}
	old, current := p.Source.Token, c.SourceRevision.Token
	compare := current
	compare.Revision = old.Revision
	if compare != old || old.Revision > current.Revision || old.Revision < c.SourceRevision.JournalFloor {
		return nil
	}
	for _, rule := range c.Rules {
		if !rule.Enabled || rule.ArchivedAt != nil || rule.Evaluator != domain.ReviewEvaluator || p.RuleRevisions[rule.ID] != rule.RowRevision {
			return nil
		}
		before, e := domain.ReviewLatestClosedDay(p.EvaluatedAt, rule.Spec.Schedule.Timezone)
		if e != nil {
			return e
		}
		after, e := domain.ReviewLatestClosedDay(c.SourceCutoff, rule.Spec.Schedule.Timezone)
		if e != nil {
			return e
		}
		if after < before {
			return nil
		}
	}
	// An unchanged complete authoritative fingerprint + source token proves
	// both deterministic descriptions identical to the authenticated prior
	// certificate. Otherwise compute both from this coherent source snapshot.
	if p.SuffixNonQuoteFingerprint == "" {
		return nil
	}
	unchanged := old == current && c.HistoricalFingerprint == c.Control.PublishedFingerprint
	invariant := p.SuffixNonQuoteFingerprint
	seals := p.SuffixQuoteSeals
	var err error
	if !unchanged {
		invariant, err = domain.ReviewNonQuoteFingerprint(c.Historical)
		if err != nil {
			return err
		}
		if invariant != p.SuffixNonQuoteFingerprint {
			return nil
		}
	}
	changes, err := readReviewSuffixChanges(ctx, tx, c.HouseholdID, old.Revision, current.Revision)
	if err != nil {
		return err
	}
	if int64(len(changes)) != current.Revision-old.Revision {
		return nil
	}
	plan := domain.PlanReviewInvalidation(changes, c.Rules)
	if plan.Full {
		return nil
	}
	if !unchanged {
		seals, err = domain.ReviewQuoteDaySeals(c.Historical)
		if err != nil {
			return err
		}
	}
	if !domain.ReviewQuoteChangesCovered(p.SuffixQuoteSeals, seals, plan, c.Rules) {
		return nil
	}
	// Equal revisions must also have identical complete captured sources.
	if old.Revision == current.Revision && c.HistoricalFingerprint != c.Control.PublishedFingerprint {
		return nil
	}
	suffix := &domain.ReviewSuffixCapture{Plan: plan, Prefix: []domain.ReviewSuffixDay{}}
	for _, rule := range c.Rules {
		previous, err := domain.ReviewLatestClosedDay(p.EvaluatedAt, rule.Spec.Schedule.Timezone)
		if err != nil {
			return err
		}
		tz := rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion
		policy := "rule:" + string(rule.ID) + "/v:" + strconv.Itoa(rule.CurrentConditionVersion)
		location, err := domain.LoadFinancialLocation(rule.Spec.Schedule.Timezone)
		if err != nil {
			return err
		}
		origin := c.Historical.History.Origin.StartedAt.In(location).Format(time.DateOnly)
		ids := reviewWindowInstruments(*c, rule)
		through := make(map[domain.InstrumentID]string, len(ids))
		for _, id := range ids {
			on := previous
			for _, d := range plan.Dirty {
				if d.RuleID == rule.ID && d.InstrumentID == id && d.From <= on {
					day, _ := time.Parse(time.DateOnly, d.From)
					on = day.AddDate(0, 0, -1).Format(time.DateOnly)
				}
			}
			through[id] = on
		}
		evidence := map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence{}
		// Same bounded request-table strategy as B: one seek per instrument,
		// with one query/decoder per <=100 instruments rather than per pair.
		for start := 0; start < len(ids); start += 100 {
			end := min(start+100, len(ids))
			values := []string{}
			args := []any{}
			for _, id := range ids[start:end] {
				if through[id] >= origin {
					values = append(values, "(?,?)")
					args = append(args, id.String(), through[id])
				}
			}
			if len(values) == 0 {
				continue
			}
			args = append(args, c.HouseholdID.String(), tz, policy, origin, string(rule.ID), rule.CurrentConditionVersion)
			rows, err := tx.QueryContext(ctx, `WITH requested(instrument_id,through_on) AS (VALUES `+strings.Join(values, ",")+`)
             SELECT d.instrument_id,d.on_date,d.input_fingerprint,d.evidence_json,d.coverage_json,r.evidence_json,r.input_fingerprint,0
             FROM requested i CROSS JOIN investment_review_daily_inputs d
             ON d.instrument_id=i.instrument_id AND d.on_date<=i.through_on AND d.household_id=? AND d.timezone_version=? AND d.input_policy=? AND d.on_date>=?
             LEFT JOIN investment_review_daily_results r ON r.rule_id=? AND r.condition_version=? AND r.instrument_id=d.instrument_id AND r.timezone_version=d.timezone_version AND r.on_date=d.on_date
             ORDER BY d.instrument_id,d.on_date`, args...)
			if err != nil {
				return err
			}
			if err = scanReusableReviewWindow(rows, rule, tz, evidence); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return nil
			}
		}
		first, _ := time.Parse(time.DateOnly, origin)
		for _, id := range ids {
			for day := first; day.Format(time.DateOnly) <= through[id]; day = day.AddDate(0, 0, 1) {
				key := domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: id, TimezoneVersion: tz, On: day.Format(time.DateOnly)}
				value, ok := evidence[key]
				if !ok || !value.HasResult || value.Input.FXProviderKey != p.CertifiedProvider.Key {
					return nil
				}
				suffix.Prefix = append(suffix.Prefix, domain.ReviewSuffixDay{Key: key, Evidence: value})
			}
		}
	}
	c.Suffix = suffix
	proof.SuffixSourceProof = &domain.ReviewSuffixSourceProof{NonQuoteFingerprint: invariant, QuoteSeals: seals}
	// Rebind the exact prefix and dirty scope to the coherent capture. The final
	// publisher still requires source/control/checkpoint/connection CAS and lease.
	proof.SuffixDigest, err = domain.ReviewSuffixFingerprint(suffix)
	if err != nil {
		return err
	}
	proof.SourceMAC, err = r.sourceCaptureMAC(*c)
	return err
}

func readReviewSuffixChanges(ctx context.Context, tx *sql.Tx, id domain.HouseholdID, after, through int64) ([]domain.ReviewSourceChange, error) {
	rows, err := tx.QueryContext(ctx, `SELECT revision,source_table,operation,old_scope_json,new_scope_json FROM financial_source_changes WHERE household_id=? AND revision>? AND revision<=? ORDER BY revision,change_ordinal`, id.String(), after, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ReviewSourceChange{}
	for rows.Next() {
		var c domain.ReviewSourceChange
		var old, new sql.NullString
		if err = rows.Scan(&c.Revision, &c.Table, &c.Operation, &old, &new); err != nil {
			return nil, err
		}
		// Require one contiguous journal entry per source revision, not just count.
		if c.Revision != after+int64(len(result))+1 {
			return nil, nil
		}
		if c.Table == "instrument_quotes" {
			for _, v := range []struct {
				raw    sql.NullString
				target **domain.ReviewQuoteDependency
			}{{old, &c.Old}, {new, &c.New}} {
				if v.raw.Valid {
					var scope domain.ReviewQuoteDependency
					if json.Unmarshal([]byte(v.raw.String), &scope) != nil {
						return nil, nil
					}
					*v.target = &scope
				}
			}
		}
		result = append(result, c)
	}
	return result, errors.Join(rows.Err(), rows.Close())
}

// A suffix can only publish under the exact authenticated capture. Losing an
// authority fence is a conflict, never permission to publish cached facts via C2.
func requireReviewSuffixAuthority(c domain.ReviewProjectionCapture, certified, mayCertify, leased bool) error {
	if c.Suffix != nil && (!certified || !mayCertify || !leased || c.Certified == nil || !c.Certified.MayCertify || !c.Certified.Provider.Pinned || c.Certified.Provider != c.Certified.PreviousProvider) {
		return reviewRuleConflict("suffix authority lost; recapture full projection")
	}
	return nil
}

// Verify the authenticated digest against actual typed values after the final
// injected clock, before any write. SourceMAC authenticates this digest and the
// presence of a suffix, so routine CAS/signing need not rehash large prefixes.
func (r *Repository) verifyReviewSuffixPrefix(c domain.ReviewProjectionCapture) error {
	if c.Suffix == nil {
		return nil
	}
	if c.Certified == nil || c.Certified.SuffixDigest == "" {
		return reviewRuleConflict("suffix prefix authority missing")
	}
	digest, err := domain.ReviewSuffixFingerprint(c.Suffix)
	if err != nil {
		return err
	}
	mac, err := r.sourceCaptureMAC(c)
	if err != nil {
		return err
	}
	if digest != c.Certified.SuffixDigest || !hmac.Equal([]byte(mac), []byte(c.Certified.SourceMAC)) {
		return reviewRuleConflict("suffix prefix changed during preparation")
	}
	return nil
}
