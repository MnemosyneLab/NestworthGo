package domain

import "reflect"

const ReviewWindowOracleVersion = "bounded-full-output-shadow-v1"

// Transient diagnostic output, never a source of publication rows or reuse
// authority. The full publication remains the independently computed oracle.
type ReviewWindowOracle struct {
	Version         string
	Rules           []ReviewRulePublication
	Failed          bool
	PreparedVerdict string // Diagnostic only; validation recomputes instead of trusting it.
}

// ValidateReviewWindowOracle checks complete emitted-day sets and every
// lifecycle field, not just the historical overlap or latest predicate bool.
// It does not authenticate preexisting archived payloads/handling metadata or
// resample in-memory providers. Those remain separate activation gates.
func ValidateReviewWindowOracle(p ReviewPublication) string {
	if ValidateReviewCompletenessPrefix(p) != "prefix_validated" {
		return "window_oracle_prefix_untrusted"
	}
	probe := p.WindowOracle
	if probe == nil {
		return "window_oracle_missing"
	}
	if probe.Version != ReviewWindowOracleVersion {
		return "window_oracle_unknown"
	}
	if probe.Failed {
		return "window_prepare_failed"
	}
	if len(probe.Rules) != len(p.Rules) {
		return "window_oracle_rule_set_changed"
	}
	newIDs := map[string]bool{}
	for index, want := range p.Rules {
		got := probe.Rules[index]
		if got.RuleID != want.RuleID || got.InstrumentID != want.InstrumentID || got.ConditionVersion != want.ConditionVersion || got.TimezoneVersion != want.TimezoneVersion {
			return "window_oracle_rule_set_changed"
		}
		expected, ok := reviewWindowOracleDays(p, want)
		if !ok {
			return "window_oracle_invalid_full_plan"
		}
		if len(got.Days) != len(expected) {
			return "window_oracle_day_set_changed"
		}
		for dayIndex, day := range got.Days {
			if day.On != expected[dayIndex].On {
				return "window_oracle_day_set_changed"
			}
			if !reflect.DeepEqual(day, expected[dayIndex]) {
				return "window_oracle_day_changed"
			}
		}
		got.Days, want.Days = nil, nil
		if got.OpenedEpisode && want.OpenedEpisode && got.CurrentEpisode != nil && want.CurrentEpisode != nil {
			id := got.CurrentEpisode.ID
			if _, err := ParseReviewEpisodeID(id); err != nil || newIDs[id] {
				return "window_oracle_episode_identity_changed"
			}
			for _, stored := range p.Capture.OpenEpisodes {
				if id == string(stored.ID) {
					return "window_oracle_episode_identity_changed"
				}
			}
			for _, closed := range want.ClosedEpisodes {
				if id == closed.ID {
					return "window_oracle_episode_identity_changed"
				}
			}
			newIDs[id] = true
			// The two independent planners allocate different fresh UUIDs. Only
			// this new episode's ID may differ; all other identity/ack links stay.
			current := *got.CurrentEpisode
			current.ID = want.CurrentEpisode.ID
			got.CurrentEpisode = &current
		}
		if !reflect.DeepEqual(got, want) {
			return "window_oracle_lifecycle_changed"
		}
	}
	return "window_oracle_matched"
}

// This derives comparison expectations from full dates and captured anchors,
// independently of the bounded planner's chosen suffix. It never plans work.
func reviewWindowOracleDays(p ReviewPublication, rule ReviewRulePublication) ([]ReviewPublicationDay, bool) {
	for i, day := range rule.Days {
		if day.On == "" || day.On > rule.LatestClosedOn || (i > 0 && day.On <= rule.Days[i-1].On) {
			return nil, false
		}
	}
	if len(rule.Days) == 0 {
		return rule.Days, true
	}
	width := 0
	for _, spec := range p.Capture.Rules {
		if spec.ID != rule.RuleID {
			continue
		}
		switch spec.Spec.TriggerPolicy.Kind {
		case "consecutive":
			width = spec.Spec.TriggerPolicy.Count
		case "rolling":
			width = spec.Spec.TriggerPolicy.WindowDays
		default:
			width = 1
		}
		break
	}
	if width < 1 {
		return nil, false
	}
	for _, episode := range p.Capture.OpenEpisodes {
		if episode.RuleID == rule.RuleID && episode.InstrumentID == rule.InstrumentID && episode.TriggerWindow.WindowStart == "" {
			return rule.Days, true // A legacy opening snapshot needs full repair.
		}
	}
	anchors := map[string]bool{}
	for key, item := range p.Capture.CompletenessLineage.Window {
		if key.RuleID != rule.RuleID || key.InstrumentID != rule.InstrumentID || key.ConditionVersion != rule.ConditionVersion || key.TimezoneVersion != rule.TimezoneVersion {
			continue
		}
		if !item.HasResult || item.Input.FXProviderKey != p.FXProviderKey {
			return rule.Days, true
		}
		anchors[key.On] = true
	}
	if len(anchors) == 0 {
		return rule.Days, true
	}
	last := -1
	for i, day := range rule.Days {
		if anchors[day.On] {
			last = i
		}
	}
	first := last - width + 1
	if first < 0 {
		first = 0
	}
	if last < 0 || len(anchors) != last-first+1 {
		return rule.Days, true
	}
	for _, day := range rule.Days[first : last+1] {
		if !anchors[day.On] {
			return rule.Days, true
		}
	}
	return rule.Days[last+1:], true
}
