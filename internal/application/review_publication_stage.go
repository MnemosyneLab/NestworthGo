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
	for _, rule := range capture.Rules {
		if err := ctx.Err(); err != nil {
			return domain.ReviewPublication{}, err
		}
		if rule.Evaluator != domain.ReviewEvaluator {
			continue // legacy evaluator remains isolated
		}
		if rule.ArchivedAt != nil {
			// An archived rule needs only to close any still-open episodes. Its
			// historical predicate cannot produce another alert, so replaying
			// every old day wastes work and risks stale evidence writes.
			latest, err := domain.ReviewLatestClosedDay(now, rule.Spec.Schedule.Timezone)
			if err != nil {
				return domain.ReviewPublication{}, err
			}
			for _, stored := range capture.OpenEpisodes {
				if stored.RuleID != rule.ID {
					continue
				}
				closedAt := now.UTC()
				result.Rules = append(result.Rules, domain.ReviewRulePublication{
					RuleID: rule.ID, RuleRevision: rule.RowRevision, ConditionVersion: rule.CurrentConditionVersion,
					InstrumentID: stored.InstrumentID, TimezoneVersion: rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion,
					LatestClosedOn: latest, ClosedEpisodes: []domain.ReviewEpisodeIdentity{{ID: string(stored.ID), TriggeredOn: stored.TriggeredOn, FirstDetectedAt: stored.FirstDetectedAt, AcknowledgedAt: stored.AcknowledgedAt, ResolvedAt: &closedAt, ResolvedReason: "rule_archived"}},
				})
			}
			continue
		}
		instrumentIDs := map[domain.InstrumentID]bool{}
		if rule.Spec.Target.Kind == "instruments" {
			for _, id := range rule.Spec.Target.InstrumentIDs {
				instrumentIDs[id] = true
			}
		} else {
			for _, holding := range capture.Historical.History.Portfolio.Holdings {
				instrumentIDs[holding.InstrumentID] = true
			}
		}
		ordered := make([]domain.InstrumentID, 0, len(instrumentIDs))
		for instrumentID := range instrumentIDs {
			ordered = append(ordered, instrumentID)
		}
		slices.Sort(ordered)
		for _, instrumentID := range ordered {
			plan, err := PlanReviewRuleInstrument(ctx, capture, rule, instrumentID, now, providerKey)
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
				LatestClosedOn: plan.LatestClosedOn, LatestWindow: plan.LatestWindow, PositionKnown: plan.Position.Known, CurrentEpochID: lifecycle.CurrentEpochID,
				CurrentEpisode: lifecycle.Current, ClosedEpisodes: lifecycle.Closed, OpenedEpisode: lifecycle.Opened,
				Days: make([]domain.ReviewPublicationDay, 0, len(plan.Days)), Epochs: make([]domain.ReviewPublicationEpoch, 0, len(plan.Position.Epochs)),
			}
			for _, day := range plan.Days {
				entry.Days = append(entry.Days, domain.ReviewPublicationDay{On: day.On, Input: day.Input, CoverageMissing: day.Coverage.Missing, Result: day.Result, PolicyWindow: day.PolicyWindow, InputFingerprint: day.InputFingerprint})
			}
			if len(plan.Days) > 0 {
				entry.LatestCoverage = plan.Days[len(plan.Days)-1].Coverage.Missing
			}
			for index, epoch := range plan.Position.Epochs {
				entry.Epochs = append(entry.Epochs, domain.ReviewPublicationEpoch{ID: plan.EpochIDs[index], InstrumentID: instrumentID, OpenedAt: epoch.OpenedAt, OpeningID: epoch.OpenedBy, ClosedAt: epoch.ClosedAt, ClosingID: epoch.ClosedBy, StartUnknown: epoch.StartUnknown})
			}
			result.Rules = append(result.Rules, entry)
		}
	}
	return result, nil
}
