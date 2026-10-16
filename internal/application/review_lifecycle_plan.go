package application

import (
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewLifecyclePlan struct {
	CurrentEpochID string
	Current        *domain.ReviewEpisodeIdentity
	Closed         []domain.ReviewEpisodeIdentity
	Opened         bool
	Actionable     bool
}

// PlanReviewLifecycle coordinates a rule's current epoch with its persisted
// open episode. Date-keyed policy votes are retained across a position close;
// only the episode and its acknowledgement are scoped to the position epoch.
func PlanReviewLifecycle(capture domain.ReviewProjectionCapture, rule domain.ReviewRule, plan ReviewRuleInstrumentPlan, now time.Time) (ReviewLifecyclePlan, error) {
	result := ReviewLifecyclePlan{Closed: []domain.ReviewEpisodeIdentity{}}
	if rule.ID != plan.RuleID || rule.CurrentConditionVersion != plan.ConditionVersion || now.IsZero() {
		return result, &domain.Error{Code: domain.ErrValidation, Field: "plan", Message: "rule projection and detection time are required"}
	}
	known, held := plan.Position.Known, false
	if known {
		quantity, err := domain.ParseQuantity(plan.Position.Quantity)
		if err != nil {
			return result, err
		}
		held = quantity.Decimal().IsPositive()
	}
	var currentEpoch *ReviewPositionEpoch
	if held && len(plan.Position.Epochs) > 0 && len(plan.EpochIDs) == len(plan.Position.Epochs) {
		last := len(plan.Position.Epochs) - 1
		if plan.Position.Epochs[last].ClosedAt == nil {
			result.CurrentEpochID = plan.EpochIDs[last]
			currentEpoch = &plan.Position.Epochs[last]
		}
	}
	policyByDay := make(map[string]domain.ReviewTruth, len(plan.Days))
	for _, day := range plan.Days {
		policyByDay[day.On] = day.PolicyWindow.Truth
	}
	var reusable *domain.ReviewEpisodeIdentity
	reusableEpochID := ""
	for _, stored := range capture.OpenEpisodes {
		if stored.RuleID != rule.ID || stored.InstrumentID != plan.InstrumentID {
			continue
		}
		identity := domain.ReviewEpisodeIdentity{ID: string(stored.ID), TriggeredOn: stored.TriggeredOn, FirstDetectedAt: stored.FirstDetectedAt, AcknowledgedAt: stored.AcknowledgedAt}
		reason := ""
		switch {
		case rule.ArchivedAt != nil:
			reason = "rule_archived"
		case known && (!held || stored.PositionEpochID != result.CurrentEpochID):
			reason = "position_closed"
		case !plan.Incremental && len(plan.Days) > 0 && stored.TriggeredOn < plan.Days[0].On:
			reason = "evidence_corrected"
		default:
			if reusable != nil {
				return result, &domain.Error{Code: domain.ErrConflict, Field: "episode", Message: "multiple open episodes share one review epoch"}
			}
			reusable = &identity
			reusableEpochID = stored.PositionEpochID
		}
		if reason != "" {
			resolvedAt := now.UTC()
			identity.ResolvedAt, identity.ResolvedReason = &resolvedAt, reason
			result.Closed = append(result.Closed, identity)
		}
	}
	if rule.ArchivedAt != nil || !known || !held || currentEpoch == nil || len(plan.Days) == 0 && !plan.Incremental {
		result.Current = reusable
		if !known && reusable != nil {
			result.CurrentEpochID = reusableEpochID
		}
		return result, nil
	}
	if plan.Incremental {
		return planReviewLifecycleIncremental(capture, rule, plan, now, result, reusable)
	}
	firstOn := plan.Days[0].On
	if currentEpoch.OpenedAt != nil {
		location, err := time.LoadLocation(rule.Spec.Schedule.Timezone)
		if err != nil {
			return result, err
		}
		openedOn := currentEpoch.OpenedAt.In(location).Format(time.DateOnly)
		// A same-day re-buy has a new epoch now but no closed-day vote in
		// that epoch yet. Rolling votes from the prior holding period may
		// already make L sufficient; anchor its first detection to L without
		// inventing a vote for today's unclosed civil day.
		if openedOn > plan.LatestClosedOn {
			firstOn = plan.LatestClosedOn
		} else if openedOn > firstOn {
			firstOn = openedOn
		}
	}
	if reusable != nil && reusable.TriggeredOn > firstOn {
		firstOn = reusable.TriggeredOn
	}
	// Re-evaluate the complete new condition over the old open interval.
	// Only a proven policy F (or a different position epoch) can split it;
	// conditionVersion alone is not an episode identity.
	if firstOn > plan.LatestClosedOn {
		// A timezone edit can move the new latest-closed day behind an
		// already-triggered episode. No new daily evidence exists yet to prove
		// recovery or a fresh interval, so keep the prior episode and its ack.
		result.Current = reusable
		result.Actionable = reusable != nil && domain.ReviewEpisodeActionable(capture.Control.Enabled, rule.Enabled, true, reusable.AcknowledgedAt != nil, plan.LatestWindow.Truth, plan.ActiveSuppression)
		return result, nil
	}
	decision, err := domain.AdvanceReviewEpisode(firstOn, plan.LatestClosedOn, policyByDay, true, true, false, reusable, string(domain.NewReviewEpisodeID()), now)
	if err != nil {
		return result, err
	}
	if decision.Closed != nil {
		result.Closed = append(result.Closed, *decision.Closed)
	}
	result.Current, result.Opened = decision.Current, decision.Opened
	result.Actionable = result.Current != nil && domain.ReviewEpisodeActionable(capture.Control.Enabled, rule.Enabled, true, result.Current.AcknowledgedAt != nil, plan.LatestWindow.Truth, plan.ActiveSuppression)
	return result, nil
}

func planReviewLifecycleIncremental(capture domain.ReviewProjectionCapture, rule domain.ReviewRule, plan ReviewRuleInstrumentPlan, now time.Time, result ReviewLifecyclePlan, reusable *domain.ReviewEpisodeIdentity) (ReviewLifecyclePlan, error) {
	storedEpisodes := make(map[domain.ReviewEpisodeID]struct{}, len(capture.OpenEpisodes))
	for _, episode := range capture.OpenEpisodes {
		if episode.RuleID == rule.ID && episode.InstrumentID == plan.InstrumentID {
			storedEpisodes[episode.ID] = struct{}{}
		}
	}
	current := reusable
	opened := false
	firstTrueAfterLastFalse := ""
	for _, day := range plan.Days {
		if day.On <= plan.PreviousLatestOn {
			continue
		}
		switch day.PolicyWindow.Truth {
		case domain.ReviewFalse:
			if current != nil {
				if _, persisted := storedEpisodes[domain.ReviewEpisodeID(current.ID)]; persisted {
					closed := *current
					resolvedAt := now.UTC()
					closed.ResolvedAt, closed.ResolvedReason = &resolvedAt, "recovered"
					result.Closed = append(result.Closed, closed)
				}
				current = nil
				opened = false
			}
			firstTrueAfterLastFalse = ""
		case domain.ReviewTrue:
			if current == nil && firstTrueAfterLastFalse == "" {
				firstTrueAfterLastFalse = day.On
			}
		case domain.ReviewUnknown:
			// Unknown pauses an episode and does not prove recovery.
		default:
			return result, &domain.Error{Code: domain.ErrValidation, Field: "policyByDay", Message: "unsupported incremental policy truth value"}
		}
	}
	if current == nil && plan.LatestWindow.Truth == domain.ReviewTrue {
		if firstTrueAfterLastFalse == "" {
			return result, &domain.Error{Code: domain.ErrConflict, Field: "episode", Message: "incremental trigger has no newly proven true policy date"}
		}
		current = &domain.ReviewEpisodeIdentity{ID: string(domain.NewReviewEpisodeID()), TriggeredOn: firstTrueAfterLastFalse, FirstDetectedAt: now.UTC()}
		opened = true
	}
	result.Current = current
	result.Opened = current != nil && opened
	result.Actionable = current != nil && domain.ReviewEpisodeActionable(capture.Control.Enabled, rule.Enabled, true, current.AcknowledgedAt != nil, plan.LatestWindow.Truth, plan.ActiveSuppression)
	return result, nil
}
