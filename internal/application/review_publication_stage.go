package application

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// PrepareReviewPublication runs outside a write transaction. The repository
// will reject this entire staged result if any authoritative fact, rule,
// acknowledgement or ignore changed before publication.
func PrepareReviewPublication(ctx context.Context, capture domain.ReviewProjectionCapture, now time.Time, providerKey string) (domain.ReviewPublication, error) {
	result := domain.ReviewPublication{Capture: capture, At: now, Rules: []domain.ReviewRulePublication{}}
	cache := newReviewProjectionCache(capture, providerKey)
	for _, rule := range capture.Rules {
		if err := ctx.Err(); err != nil {
			return domain.ReviewPublication{}, err
		}
		if rule.Evaluator != domain.ReviewEvaluator {
			continue // legacy evaluator remains isolated
		}
		latest, err := domain.ReviewLatestClosedDay(now, rule.Spec.Schedule.Timezone)
		if err != nil {
			return domain.ReviewPublication{}, err
		}
		eligible := map[domain.InstrumentID]bool{}
		if rule.ArchivedAt == nil && rule.Spec.Target.Kind == "instruments" {
			for _, id := range rule.Spec.Target.InstrumentIDs {
				eligible[id] = true
			}
		} else if rule.ArchivedAt == nil {
			for _, holding := range capture.Historical.History.Portfolio.Holdings {
				eligible[holding.InstrumentID] = true
			}
		}
		candidates := map[domain.InstrumentID]bool{}
		for id := range eligible {
			candidates[id] = true
		}
		for _, id := range capture.EvaluatedInstruments[rule.ID] {
			candidates[id] = true
		}
		for _, stored := range capture.OpenEpisodes {
			if stored.RuleID == rule.ID {
				candidates[stored.InstrumentID] = true
			}
		}
		ordered := make([]domain.InstrumentID, 0, len(candidates))
		for instrumentID := range candidates {
			ordered = append(ordered, instrumentID)
		}
		slices.Sort(ordered)
		for _, instrumentID := range ordered {
			if !eligible[instrumentID] {
				reason := "rule_replaced"
				if rule.ArchivedAt != nil {
					reason = "rule_archived"
				}
				closed := make([]domain.ReviewEpisodeIdentity, 0)
				for _, stored := range capture.OpenEpisodes {
					if stored.RuleID == rule.ID && stored.InstrumentID == instrumentID {
						closedAt := now.UTC()
						closed = append(closed, domain.ReviewEpisodeIdentity{ID: string(stored.ID), TriggeredOn: stored.TriggeredOn, FirstDetectedAt: stored.FirstDetectedAt, AcknowledgedAt: stored.AcknowledgedAt, ResolvedAt: &closedAt, ResolvedReason: reason})
					}
				}
				result.Rules = append(result.Rules, domain.ReviewRulePublication{RuleID: rule.ID, RuleRevision: rule.RowRevision, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: instrumentID, TimezoneVersion: rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion, LatestClosedOn: latest, ClosedEpisodes: closed, TargetExcluded: true})
				continue
			}
			plan, err := planReviewRuleInstrument(ctx, capture, rule, instrumentID, now, cache)
			if err != nil {
				return domain.ReviewPublication{}, err
			}
			lifecycle, err := PlanReviewLifecycle(capture, rule, plan, now)
			if err != nil {
				return domain.ReviewPublication{}, err
			}
			triggerWindow, firstHit, triggerResult, triggerCoverage, err := reviewEpisodeTriggerEvidence(capture, rule, plan, lifecycle.Current)
			if err != nil {
				return domain.ReviewPublication{}, err
			}
			entry := domain.ReviewRulePublication{
				RuleID: rule.ID, RuleRevision: rule.RowRevision, ConditionVersion: rule.CurrentConditionVersion,
				InstrumentID: instrumentID, TimezoneVersion: rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion,
				LatestClosedOn: plan.LatestClosedOn, LatestResult: plan.LatestResult, LatestCoverage: append([]string{}, plan.LatestCoverage...),
				FirstHitOn: firstHit, TriggerWindow: triggerWindow, TriggerResult: triggerResult, TriggerCoverage: triggerCoverage,
				LatestWindow: plan.LatestWindow, PositionKnown: plan.Position.Known, CurrentEpochID: lifecycle.CurrentEpochID,
				CurrentEpisode: lifecycle.Current, ClosedEpisodes: lifecycle.Closed, OpenedEpisode: lifecycle.Opened,
				Days: make([]domain.ReviewPublicationDay, 0, len(plan.Days)), Epochs: make([]domain.ReviewPublicationEpoch, 0, len(plan.Position.Epochs)),
			}
			for _, day := range plan.Days {
				entry.Days = append(entry.Days, domain.ReviewPublicationDay{On: day.On, Input: day.Input, CoverageMissing: day.Coverage.Missing, Result: day.Result, PolicyWindow: day.PolicyWindow, InputFingerprint: day.InputFingerprint})
			}
			for index, epoch := range plan.Position.Epochs {
				entry.Epochs = append(entry.Epochs, domain.ReviewPublicationEpoch{ID: plan.EpochIDs[index], InstrumentID: instrumentID, OpenedAt: epoch.OpenedAt, OpeningID: epoch.OpenedBy, ClosedAt: epoch.ClosedAt, ClosingID: epoch.ClosedBy, StartUnknown: epoch.StartUnknown})
			}
			result.Rules = append(result.Rules, entry)
		}
	}
	return result, nil
}

func reviewEpisodeTriggerEvidence(capture domain.ReviewProjectionCapture, rule domain.ReviewRule, plan ReviewRuleInstrumentPlan, current *domain.ReviewEpisodeIdentity) (domain.ReviewPolicyWindow, string, *domain.ReviewDailyResult, []string, error) {
	if current == nil {
		return domain.ReviewPolicyWindow{}, "", nil, nil, nil
	}
	var stored *domain.ReviewStoredEpisode
	for index := range capture.OpenEpisodes {
		candidate := &capture.OpenEpisodes[index]
		if candidate.ID == domain.ReviewEpisodeID(current.ID) {
			stored = candidate
			break
		}
	}
	if stored != nil && stored.TriggerWindow.WindowStart != "" {
		// Keep the opening snapshot stable across routine date-window rotation
		// and rule edits. A same-version source correction is replayed below.
		if plan.Incremental || stored.OpenedConditionVersion != plan.ConditionVersion || capture.Control.PublishedFingerprint == capture.HistoricalFingerprint {
			return stored.TriggerWindow, stored.FirstHitOn, stored.TriggerResult, append([]string{}, stored.TriggerCoverage...), nil
		}
	}

	type dayEvidence struct {
		result   domain.ReviewDailyResult
		coverage []string
	}
	days := make(map[string]dayEvidence, len(plan.Days)+len(capture.ReusableDailyEvidence))
	for _, day := range plan.Days {
		days[day.On] = dayEvidence{result: day.Result, coverage: append([]string{}, day.Coverage.Missing...)}
	}
	timezoneVersion := rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion
	for key, cached := range capture.ReusableDailyEvidence {
		if key.RuleID == rule.ID && key.ConditionVersion == rule.CurrentConditionVersion && key.InstrumentID == plan.InstrumentID && key.TimezoneVersion == timezoneVersion && cached.HasResult {
			days[key.On] = dayEvidence{result: cached.Result, coverage: append([]string{}, cached.CoverageMissing...)}
		}
	}
	trigger, ok := days[current.TriggeredOn]
	if ok && trigger.result.On == current.TriggeredOn {
		window := domain.ReviewPolicyWindow{}
		for _, planned := range plan.Days {
			if planned.On == current.TriggeredOn {
				window = planned.PolicyWindow
				break
			}
		}
		if window.WindowStart == "" {
			// In bounded replay the trigger date may be in the reusable overlap.
			// Re-evaluate this single policy window from its captured daily votes.
			truths := make(map[string]domain.ReviewTruth, reviewPolicyWindowWidth(rule.Spec.TriggerPolicy))
			start, err := time.Parse(time.DateOnly, current.TriggeredOn)
			if err != nil {
				return domain.ReviewPolicyWindow{}, "", nil, nil, err
			}
			width := reviewPolicyWindowWidth(rule.Spec.TriggerPolicy)
			for offset := width - 1; offset >= 0; offset-- {
				date := start.AddDate(0, 0, -offset).Format(time.DateOnly)
				if prior, exists := days[date]; exists {
					truths[date] = prior.result.Truth
				} else {
					truths[date] = domain.ReviewUnknown
				}
			}
			window, err = domain.EvaluateReviewPolicy(rule.Spec.TriggerPolicy, current.TriggeredOn, truths)
			if err != nil {
				return domain.ReviewPolicyWindow{}, "", nil, nil, err
			}
		}
		if window.Truth == domain.ReviewTrue && window.WindowEnd == current.TriggeredOn {
			firstHit := ""
			first, err := time.Parse(time.DateOnly, window.WindowStart)
			if err != nil {
				return domain.ReviewPolicyWindow{}, "", nil, nil, err
			}
			last, _ := time.Parse(time.DateOnly, window.WindowEnd)
			for date := first; !date.After(last); date = date.AddDate(0, 0, 1) {
				if evidence, exists := days[date.Format(time.DateOnly)]; exists && evidence.result.Truth == domain.ReviewTrue {
					firstHit = evidence.result.On
					break
				}
			}
			if firstHit != "" {
				return window, firstHit, &trigger.result, trigger.coverage, nil
			}
		}
	}
	if stored != nil && stored.TriggerWindow.WindowStart != "" {
		// A timezone change can temporarily put the latest closed date before
		// an existing trigger date. Preserve its captured opening evidence.
		return stored.TriggerWindow, stored.FirstHitOn, stored.TriggerResult, append([]string{}, stored.TriggerCoverage...), nil
	}
	return domain.ReviewPolicyWindow{}, "", nil, nil, fmt.Errorf("review episode %s has no reconstructable trigger window", current.ID)
}
