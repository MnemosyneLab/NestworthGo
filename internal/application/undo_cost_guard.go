package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// rejectDependentHoldingUndo protects cost-basis replay from a current-time
// reversal that retroactively removes an acquisition or disposition. Cost-basis
// reads exclude a reversed activity at its original historical position. If a
// later quantity event used the same Holding, that later event may become
// impossible or acquire a different basis even though today's inverse effects
// still fit. A historical Fix replays those later events before committing.
func (s *Service) rejectDependentHoldingUndo(ctx context.Context, original domain.Activity, effects []domain.ActivityEffect, asOf time.Time) error {
	affected := make(map[domain.HoldingID]bool)
	for _, effect := range effects {
		if effect.Target == domain.EffectTargetHoldingQuantity && effect.HoldingID != nil {
			affected[*effect.HoldingID] = true
		}
	}
	if len(affected) == 0 {
		return nil
	}
	timeline, err := s.repository.ListActivitiesUntil(ctx, original.HouseholdID, asOf)
	if err != nil {
		return err
	}
	index := -1
	for i, activity := range timeline {
		if activity.ID == original.ID {
			index = i
			break
		}
	}
	if index < 0 {
		return &domain.Error{Code: domain.ErrInvalidChange, Field: "activityId", Message: "the change is no longer active in history"}
	}
	for _, activity := range timeline[index+1:] {
		for _, effect := range activity.Effects {
			if (effect.Target == domain.EffectTargetHoldingQuantity || effect.Target == domain.EffectTargetHoldingCost) && effect.HoldingID != nil && affected[*effect.HoldingID] {
				return &domain.Error{Code: domain.ErrInvalidChange, Field: "activityId", Message: "a later position change depends on this Holding; fix the historical change instead"}
			}
		}
	}
	return nil
}
