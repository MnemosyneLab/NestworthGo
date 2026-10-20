package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"time"
)

// ReviewSourceChange contains only the non-secret journal scope required by D1.
// Unknown sources or incomplete OLD/NEW tombstones require full recomputation.
type ReviewSourceChange struct {
	Revision         int64
	Table, Operation string
	Old, New         *ReviewQuoteDependency
}
type ReviewQuoteDependency struct {
	InstrumentID  InstrumentID `json:"instrument_id"`
	EffectiveDate string       `json:"effective_date"`
}
type ReviewDirtyPair struct {
	RuleID       ReviewRuleID
	InstrumentID InstrumentID
	From         string
}
type ReviewInvalidationPlan struct {
	Full   bool
	Reason string
	Dirty  []ReviewDirtyPair
}

// ReviewSuffixCapture is authenticated by the repository's source MAC. Prefix
// values come from the sealed daily rows, never a caller's cache or a count.
// Policy and lifecycle replay deliberately start at origin in this first D2.
type ReviewSuffixCapture struct {
	Plan   ReviewInvalidationPlan
	Prefix []ReviewSuffixDay
}
type ReviewSuffixDay struct {
	Key      ReviewDailyEvidenceKey
	Evidence ReviewReusableDailyEvidence
}

// PlanReviewInvalidation never truncates propagation to a rolling window.
// A quote can be a later date's lookback baseline, so its suffix is unbounded.
// Other dependency families currently fall back to origin for the household.
func PlanReviewInvalidation(changes []ReviewSourceChange, rules []ReviewRule) ReviewInvalidationPlan {
	result := ReviewInvalidationPlan{Reason: "quote_suffix", Dirty: []ReviewDirtyPair{}}
	dirty := map[InstrumentID]string{}
	for _, c := range changes {
		if c.Table == "history_snapshot_state" && c.Operation == "update" {
			continue
		}
		if c.Table != "instrument_quotes" || (c.Operation != "insert" && c.Operation != "update" && c.Operation != "delete") || (c.Operation != "insert" && c.Old == nil) || (c.Operation != "delete" && c.New == nil) {
			return ReviewInvalidationPlan{Full: true, Reason: "unsupported_dependency"}
		}
		for _, scope := range []*ReviewQuoteDependency{c.Old, c.New} {
			if scope == nil {
				continue
			}
			if _, err := time.Parse(time.DateOnly, scope.EffectiveDate); err != nil || scope.InstrumentID == "" {
				return ReviewInvalidationPlan{Full: true, Reason: "incomplete_quote_scope"}
			}
			if old, ok := dirty[scope.InstrumentID]; !ok || scope.EffectiveDate < old {
				dirty[scope.InstrumentID] = scope.EffectiveDate
			}
		}
	}
	// Explicit targets are only a filter on current eligibility. Retain dirty
	// entries for historical/evaluated instruments too; capture builds that union.
	for _, rule := range rules {
		for id, on := range dirty {
			result.Dirty = append(result.Dirty, ReviewDirtyPair{rule.ID, id, on})
		}
	}
	sort.Slice(result.Dirty, func(i, j int) bool {
		a, b := result.Dirty[i], result.Dirty[j]
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		return a.InstrumentID < b.InstrumentID
	})
	return result
}

// ReviewNonQuoteFingerprint separately binds every captured dependency other
// than instrument quote payloads and the non-economic invalidation counter.
// This detects time activation even when quote DML also changed the full hash.
func ReviewNonQuoteFingerprint(batch ReviewHistoricalBatch) (string, error) {
	batch.History.InstrumentQuoteFacts = nil
	batch.History.Portfolio.InstrumentQuotes = nil
	batch.History.InputGeneration = 0
	return ReviewHistoricalFingerprint(batch)
}

// ReviewQuoteDaySeal authenticates exact captured quote candidates, including
// prices/private scalars, source priority, revisions and dates. It is not a
// daily financial result or a persisted authority across key loss.
type ReviewQuoteDaySeal struct {
	InstrumentID    InstrumentID
	On, Fingerprint string
}

func ReviewQuoteDaySeals(batch ReviewHistoricalBatch) ([]ReviewQuoteDaySeal, error) {
	type key struct {
		id InstrumentID
		on string
	}
	groups := map[key][]InstrumentQuote{}
	for _, q := range batch.History.InstrumentQuoteFacts {
		k := key{q.InstrumentID, q.EffectiveDate}
		groups[k] = append(groups[k], q)
	}
	result := make([]ReviewQuoteDaySeal, 0, len(groups))
	for k, quotes := range groups {
		sort.Slice(quotes, func(i, j int) bool { return quotes[i].ID < quotes[j].ID })
		value, err := ReviewHistoricalFingerprint(ReviewHistoricalBatch{History: HistoricalSnapshotBatch{InstrumentQuoteFacts: quotes}})
		if err != nil {
			return nil, err
		}
		result = append(result, ReviewQuoteDaySeal{InstrumentID: k.id, On: k.on, Fingerprint: value})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].InstrumentID != result[j].InstrumentID {
			return result[i].InstrumentID < result[j].InstrumentID
		}
		return result[i].On < result[j].On
	})
	return result, nil
}

// ReviewQuoteChangesCovered independently verifies every changed quote bucket
// against the authenticated previous complete set. A damaged scope cannot move
// the dirty frontier forward or conceal a changed OLD instrument/date.
func ReviewQuoteChangesCovered(before, after []ReviewQuoteDaySeal, plan ReviewInvalidationPlan, rules []ReviewRule) bool {
	type key struct {
		id InstrumentID
		on string
	}
	prior, current := map[key]string{}, map[key]string{}
	for _, s := range before {
		prior[key{s.InstrumentID, s.On}] = s.Fingerprint
	}
	for _, s := range after {
		current[key{s.InstrumentID, s.On}] = s.Fingerprint
	}
	changed := map[key]bool{}
	for k, v := range prior {
		if current[k] != v {
			changed[k] = true
		}
	}
	for k, v := range current {
		if prior[k] != v {
			changed[k] = true
		}
	}
	type pair struct {
		rule       ReviewRuleID
		instrument InstrumentID
	}
	frontiers := map[pair]string{}
	for _, d := range plan.Dirty {
		k := pair{d.RuleID, d.InstrumentID}
		if old, ok := frontiers[k]; !ok || d.From < old {
			frontiers[k] = d.From
		}
	}
	for k := range changed {
		if _, err := time.Parse(time.DateOnly, k.on); err != nil {
			return false
		}
		for _, r := range rules {
			from, covered := frontiers[pair{r.ID, k.id}]
			if !covered || from > k.on {
				return false
			}
		}
	}
	return true
}

// ReviewSuffixFingerprint binds both JSON and exact private domain scalars
// (notably selected FX rates) to the process-local capture authority.
func ReviewSuffixFingerprint(suffix *ReviewSuffixCapture) (string, error) {
	if suffix == nil {
		return "", nil
	}
	raw, err := json.Marshal(suffix)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	digest.Write(raw)
	digest.Write([]byte{0})
	if err = writeReviewFingerprintScalars(digest, reflect.ValueOf(suffix)); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
