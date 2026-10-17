package application

import (
	"context"
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
			entry := domain.ReviewRulePublication{
				RuleID: rule.ID, RuleRevision: rule.RowRevision, ConditionVersion: rule.CurrentConditionVersion,
				InstrumentID: instrumentID, TimezoneVersion: rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion,
				LatestClosedOn: plan.LatestClosedOn, LatestResult: plan.LatestResult, LatestCoverage: append([]string{}, plan.LatestCoverage...), FirstHitOn: plan.FirstHitOn,
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
