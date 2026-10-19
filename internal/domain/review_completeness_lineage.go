package domain

import (
	"reflect"
	"strings"
)

// Diagnostic only. Nothing in this protocol grants reuse or publication rights.
const ReviewCompletenessLineageVersion = "completeness-lineage-shadow-v1"

type ReviewLineageResult struct {
	InputFingerprint string
	Truth            ReviewTruth
	Reason           string
}

// One coherent read snapshot, including narrow daily keys/fingerprints and an
// independent bounded window. The normal publication never consumes Window.
type ReviewCompletenessLineage struct {
	Version        string
	Candidate      bool
	Reason         string
	BindingDigest  string
	ProjectionSeal string
	ProviderKey    string
	ClosedOn       map[ReviewRuleID]string
	Inputs         map[ReviewDailyEvidenceKey]string
	Results        map[ReviewDailyEvidenceKey]ReviewLineageResult
	Window         map[ReviewDailyEvidenceKey]ReviewReusableDailyEvidence
}

// Safe for an optional diagnostic observer: no IDs, hashes, source values,
// provider settings or evidence payloads leave the internal capture.
type ReviewCompletenessLineageObservation struct {
	Phase   string
	Verdict string
}

// ValidateReviewCompletenessPrefix uses a freshly prepared full oracle, never
// the proposed window, to prove coverage of every previously published slot.
// Counts alone cannot detect a missing old key replaced by a different key.
func ValidateReviewCompletenessPrefix(p ReviewPublication) string {
	l := p.Capture.CompletenessLineage
	if l == nil {
		return "not_observed"
	}
	if !l.Candidate {
		return l.Reason
	}
	if l.Version != ReviewCompletenessLineageVersion || l.BindingDigest == "" || l.ProjectionSeal == "" {
		return "untrusted_lineage"
	}
	if len(p.Capture.ReusableDailyEvidence) != 0 || !p.Capture.TemporalFullRecompute {
		return "full_oracle_required"
	}
	if p.FXProviderKey != l.ProviderKey {
		return "provider_changed"
	}
	expected := map[ReviewDailyEvidenceKey]bool{}
	latest := map[ReviewDailyEvidenceKey]string{}
	for key := range l.Window {
		group := key
		group.On = ""
		if key.On > latest[group] {
			latest[group] = key.On
		}
	}
	for _, rule := range p.Rules {
		closed, ok := l.ClosedOn[rule.RuleID]
		if !ok {
			return "rule_namespace_changed"
		}
		for _, day := range rule.Days {
			if day.On > closed {
				continue
			}
			key := ReviewDailyEvidenceKey{RuleID: rule.RuleID, ConditionVersion: rule.ConditionVersion, InstrumentID: rule.InstrumentID, TimezoneVersion: rule.TimezoneVersion, On: day.On}
			if expected[key] {
				return "duplicate_oracle_key"
			}
			expected[key] = true
			input, ok := l.Inputs[key]
			if !ok || input != day.InputFingerprint {
				return "input_prefix_missing_or_changed"
			}
			result, ok := l.Results[key]
			if !ok || result.InputFingerprint != day.InputFingerprint || result.Truth != day.Result.Truth || result.Reason != strings.Join(day.CoverageMissing, ",") {
				return "result_prefix_missing_or_changed"
			}
			if cached, exists := l.Window[key]; exists {
				if !cached.HasResult || cached.InputFingerprint != day.InputFingerprint || !reflect.DeepEqual(cached.Input, day.Input) || !reflect.DeepEqual(cached.CoverageMissing, day.CoverageMissing) || !reflect.DeepEqual(cached.Result, day.Result) {
					return "window_payload_changed"
				}
				group := key
				group.On = ""
				if key.On == latest[group] && cached.PriorConsecutiveCount != day.PolicyWindow.ConsecutiveCount {
					return "window_policy_state_changed"
				}
			}
		}
	}
	if len(expected) != len(l.Inputs) || len(expected) != len(l.Results) {
		return "unexpected_daily_key"
	}
	for key := range l.Window {
		if !expected[key] {
			return "unexpected_window_key"
		}
	}
	return "prefix_validated"
}
