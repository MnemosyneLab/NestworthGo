package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const (
	reviewFactMaxDays    = 128
	reviewFactMaxRows    = 4096
	reviewFactMaxRefs    = 16384
	reviewFactMaxBytes   = 8 << 20
	reviewFactBatchBytes = 256 << 10
)

// WithReviewFinancialFactShadow enables a bounded diagnostic on this repository
// copy only. No environment flag or normal initializer selects it. The budget is
// rule-days per publication; lifetime row/byte caps prevent indefinite double writes.
func (r *Repository) WithReviewFinancialFactShadow(ruleDays int) *Repository {
	copy := *r
	copy.reviewFinancialFactShadowDays = max(0, min(ruleDays, reviewFactMaxDays))
	return &copy
}

type reviewFactRequirement struct {
	Metric       string
	Currency     domain.CurrencyCode
	FXMode       string
	LookbackDays int
}

type reviewFactSemanticKey struct {
	Version         string
	Scope           string
	On              string
	Cutoff          string
	Schedule        domain.ReviewRuleSchedule
	DataPolicy      domain.ReviewRuleDataPolicy
	MetricPolicy    string
	ResolverPolicy  string
	Provider        string
	CalendarSession string
	// Algorithm versions are deliberately conservative. New policy versions require
	// a new namespace before any future reader is authorized to reuse these facts.
	FinancialAlgorithms string
	Requirements        []string
}

func reviewFactKey(p domain.ReviewPublication, rule domain.ReviewRule, day domain.ReviewPublicationDay) (string, error) {
	return reviewFactKeyVersion(p, rule, day, "e1-v1")
}

func reviewFactKeyVersion(p domain.ReviewPublication, rule domain.ReviewRule, day domain.ReviewPublicationDay, version string) (string, error) {
	requirements := []string{}
	var visit func(domain.ReviewPredicateNode) error
	visit = func(node domain.ReviewPredicateNode) error {
		if node.Leaf != nil {
			leaf := node.Leaf
			raw, err := jsoncodec.Marshal(reviewFactRequirement{leaf.Metric, leaf.Currency, leaf.FXMode, leaf.LookbackDays})
			if err != nil {
				return err
			}
			requirements = append(requirements, string(raw))
		}
		for _, child := range node.All {
			if err := visit(child); err != nil {
				return err
			}
		}
		for _, child := range node.Any {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(rule.Spec.Predicate); err != nil {
		return "", err
	}
	slices.Sort(requirements)
	requirements = slices.Compact(requirements)
	cutoff, err := domain.ReviewCivilCutoff(day.On, rule.Spec.Schedule.Timezone)
	if err != nil {
		return "", err
	}
	if p.Capture.Temporal == nil || p.Capture.Temporal.ClockEpoch == "" {
		return "", fmt.Errorf("shared fact requires a pinned calendar session")
	}
	raw, err := jsoncodec.Marshal(reviewFactSemanticKey{
		Version: version, Scope: "household-aggregate-all-historical-accounts-v1", On: day.On,
		Cutoff: cutoff.UTC().Format(time.RFC3339Nano), Schedule: rule.Spec.Schedule, DataPolicy: rule.Spec.DataPolicy,
		MetricPolicy: rule.Spec.MetricPolicyVersion, ResolverPolicy: p.Capture.Historical.History.ResolverPolicyVersion,
		Provider: p.FXProviderKey, CalendarSession: p.Capture.Temporal.ClockEpoch,
		FinancialAlgorithms: "historical-replay-cost-basis-v1/daily-close-no-carry-v1/source-selected-price-basis-v1", Requirements: requirements,
	})
	return string(raw), err
}

type reviewFactSample struct {
	item                    domain.ReviewRulePublication
	day                     domain.ReviewPublicationDay
	rule                    domain.ReviewRule
	key                     string
	base, metrics, coverage []byte
	source                  []byte
}

// Called only under the existing publication transaction, after its full source,
// rule, handling, temporal and suffix guards. Nothing here grants reuse authority
// or changes source comparisons, MACs, CAS, final seals or lifecycle writes.
func (r *Repository) publishReviewFactShadow(ctx context.Context, tx *sql.Tx, p domain.ReviewPublication) error {
	if r.reviewFinancialFactShadowDays == 0 {
		return nil
	}
	var factCount, refCount, storedBytes, refBytes int
	if err := tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(length(CAST(id||household_id||instrument_id||semantic_key||fact_kind||source_version||payload_json||coverage_json||created_at AS BLOB))),0) FROM review_financial_daily_facts`).Scan(&factCount, &storedBytes); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(length(CAST(rule_id||condition_version||instrument_id||timezone_version||on_date||fact_id||input_fingerprint AS BLOB))),0) FROM review_result_fact_refs`).Scan(&refCount, &refBytes); err != nil {
		return err
	}
	storedBytes += refBytes
	if factCount >= reviewFactMaxRows || refCount >= reviewFactMaxRefs || storedBytes >= reviewFactMaxBytes {
		return nil
	}
	byRule := map[domain.ReviewRuleID]domain.ReviewRule{}
	for _, rule := range p.Capture.Rules {
		byRule[rule.ID] = rule
	}
	source, err := jsoncodec.Marshal(struct {
		Fingerprint string
		Checkpoint  *domain.ReviewSourceCheckpoint
	}{p.Capture.HistoricalFingerprint, p.Capture.SourceRevision})
	if err != nil {
		return err
	}
	samples := []reviewFactSample{}
	batchBytes := 0
	for _, item := range p.Rules {
		rule, ok := byRule[item.RuleID]
		if !ok {
			return fmt.Errorf("shared fact rule is missing")
		}
		for _, day := range item.Days {
			if len(samples) >= r.reviewFinancialFactShadowDays {
				break
			}
			key, err := reviewFactKey(p, rule, day)
			if err != nil {
				// Baseline emits an explicit unknown slot without leaf evidence for
				// skipped IANA dates. They have no civil cutoff to key a fact by.
				if domain.ReviewNonexistentCivilDate(err) {
					continue
				}
				return err
			}
			base := day.Input
			base.Performance = nil
			base.SelectedFX = nil
			metrics := domain.ReviewDailyInput{On: day.Input.On, InstrumentID: day.Input.InstrumentID, FXProviderKey: day.Input.FXProviderKey, Performance: day.Input.Performance, SelectedFX: day.Input.SelectedFX}
			baseJSON, err := encodeCompleteReviewInput(base)
			if err != nil {
				return err
			}
			metricJSON, err := encodeCompleteReviewInput(metrics)
			if err != nil {
				return err
			}
			coverage, err := jsoncodec.Marshal(day.CoverageMissing)
			if err != nil {
				return err
			}
			// Worst-case admission reserves both versions and refs before any inserts.
			provenance, err := reviewFactSource(p, day, source, rule.Spec.Schedule.Timezone)
			if err != nil {
				return err
			}
			batchBytes += 2*(len(key)+len(provenance)+len(coverage)) + len(baseJSON) + len(metricJSON) + 1024 + 2*(len(day.InputFingerprint)+len(day.On)+len(item.TimezoneVersion))
			if batchBytes > reviewFactBatchBytes || storedBytes+batchBytes > reviewFactMaxBytes || factCount+2*(len(samples)+1) > reviewFactMaxRows || refCount+2*(len(samples)+1) > reviewFactMaxRefs {
				return nil
			}
			samples = append(samples, reviewFactSample{item, day, rule, key, baseJSON, metricJSON, coverage, provenance})
		}
	}
	for _, sample := range samples {
		var persisted [2][]byte
		for index, kind := range []string{"base_v1", "metrics_v1"} {
			payload := sample.base
			if index == 1 {
				payload = sample.metrics
			}
			id, stored, err := writeReviewFactVersion(ctx, tx, p, sample, kind, string(sample.source), payload)
			if err != nil {
				return err
			}
			persisted[index] = stored
			if _, err := tx.ExecContext(ctx, `INSERT INTO review_result_fact_refs(rule_id,condition_version,instrument_id,timezone_version,on_date,fact_id,input_fingerprint) VALUES(?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`, string(sample.item.RuleID), sample.item.ConditionVersion, sample.item.InstrumentID.String(), sample.item.TimezoneVersion, sample.day.On, id, sample.day.InputFingerprint); err != nil {
				return err
			}
		}
		// Reconstruct from persisted complete bytes, including private exact FX rates.
		// The full exact input and rule result are compared; hashes are not an oracle.
		var base, metrics domain.ReviewDailyInput
		if err := decodeReviewInput(persisted[0], &base); err != nil {
			return err
		}
		if err := decodeReviewInput(persisted[1], &metrics); err != nil {
			return err
		}
		base.Performance, base.SelectedFX = metrics.Performance, metrics.SelectedFX
		actual, err := encodeCompleteReviewInput(base)
		if err != nil {
			return err
		}
		expected, err := encodeCompleteReviewInput(sample.day.Input)
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, expected) {
			return fmt.Errorf("shared financial input shadow mismatch")
		}
		result, err := domain.EvaluateReviewDaily(sample.rule.Spec, base)
		if err != nil {
			return err
		}
		actual, err = jsoncodec.Marshal(result)
		if err != nil {
			return err
		}
		expected, err = jsoncodec.Marshal(sample.day.Result)
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, expected) {
			return fmt.Errorf("shared financial result shadow mismatch")
		}
	}
	return nil
}

func writeReviewFactVersion(ctx context.Context, tx *sql.Tx, p domain.ReviewPublication, s reviewFactSample, kind, source string, payload []byte) (string, []byte, error) {
	// Compare every stored byte and namespace field. Fingerprint/source revision
	// only identify provenance; they never replace the exact payload comparison.
	var id, priorPayload, priorCoverage string
	err := tx.QueryRowContext(ctx, `SELECT id,payload_json,coverage_json FROM review_financial_daily_facts WHERE household_id=? AND instrument_id=? AND semantic_key=? AND fact_kind=? AND source_version=? LIMIT 1`, p.Capture.HouseholdID.String(), s.item.InstrumentID.String(), s.key, kind, source).Scan(&id, &priorPayload, &priorCoverage)
	if err == nil && (priorPayload != string(payload) || priorCoverage != string(s.coverage)) {
		return "", nil, fmt.Errorf("shared financial facts disagree within one semantic/source version")
	}
	if err == sql.ErrNoRows {
		id = domain.NewMutationID().String()
		_, err = tx.ExecContext(ctx, `INSERT INTO review_financial_daily_facts(id,household_id,instrument_id,semantic_key,fact_kind,source_version,payload_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, id, p.Capture.HouseholdID.String(), s.item.InstrumentID.String(), s.key, kind, source, string(payload), string(s.coverage), p.At.UTC().Format(time.RFC3339Nano))
	}
	if err != nil {
		return "", nil, err
	}
	var stored, coverage string
	if err := tx.QueryRowContext(ctx, `SELECT payload_json,coverage_json FROM review_financial_daily_facts WHERE id=?`, id).Scan(&stored, &coverage); err != nil {
		return "", nil, err
	}
	if stored != string(payload) || coverage != string(s.coverage) {
		return "", nil, fmt.Errorf("shared financial persisted bytes mismatch")
	}
	return id, []byte(stored), nil
}

// Preserve source candidate metadata and exact prices for the economic dates
// actually represented in this input. Selected FX quotes (including exact rates)
// are already in metrics. The original source checkpoint/fingerprint remains
// attached; this diagnostic does not claim independent source reconstruction.
func reviewFactSource(p domain.ReviewPublication, day domain.ReviewPublicationDay, checkpoint []byte, zone string) ([]byte, error) {
	dates := map[string]bool{day.On: true}
	for _, performance := range day.Input.Performance {
		dates[performance.BaselineOn] = true
		dates[performance.LatestOn] = true
	}
	location, err := domain.LoadFinancialLocation(zone)
	if err != nil {
		return nil, err
	}
	type quoteSource struct {
		Quote          domain.InstrumentQuote
		ExactUnitPrice string
	}
	quotes := []quoteSource{}
	for _, quote := range p.Capture.Historical.History.InstrumentQuoteFacts {
		if quote.InstrumentID != day.Input.InstrumentID {
			continue
		}
		on := quote.EffectiveDate
		if on == "" {
			on = quote.QuotedAt.In(location).Format(time.DateOnly)
		}
		if dates[on] {
			quotes = append(quotes, quoteSource{quote, quote.UnitPrice.String()})
		}
	}
	return jsoncodec.Marshal(struct {
		SourceCheckpoint string
		PriceCandidates  []quoteSource
	}{string(checkpoint), quotes})
}
