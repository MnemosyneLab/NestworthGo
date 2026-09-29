package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"time"
)

// prepareRecordedChange validates an insertion against the state at its date,
// then replays subsequent events before publishing any new facts.
func (s *Service) prepareRecordedChange(ctx context.Context, state domain.ChangeState, command any) (domain.ChangePreview, domain.ActivityCommit, error) {
	at := commandEffectiveTime(command)
	if at.IsZero() || !at.Before(state.Now) {
		preview, err := domain.PreviewChange(state, command)
		return preview, domain.ActivityCommit{Activity: preview.Activity, Effects: preview.Effects, Resulting: preview.Resulting}, err
	}
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	components, err := s.repository.ListHistoryOriginComponents(ctx, origin.ID)
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	activities, err := s.repository.ListActivitiesUntil(ctx, state.HouseholdID, state.Now)
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	state = correctionOriginState(state, components)
	index := len(activities)
	for i, a := range activities {
		if a.EffectiveAt.After(at) {
			index = i
			break
		}
		state, _, err = domain.ApplyEffects(state, a.Effects)
		if err != nil {
			return domain.ChangePreview{}, domain.ActivityCommit{}, err
		}
	}
	preview, err := domain.PreviewChange(state, command)
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	preview.Activity.Resulting = preview.Resulting
	timeline := make([]domain.Activity, 0, len(activities)+1)
	timeline = append(timeline, activities[:index]...)
	timeline = append(timeline, preview.Activity)
	timeline = append(timeline, activities[index:]...)
	timeline, err = domain.ReplayMoneyEffects(timeline, components)
	if err != nil {
		return domain.ChangePreview{}, domain.ActivityCommit{}, err
	}
	var projections []domain.ActivityProjection
	for _, activity := range timeline[index:] {
		var views []domain.EndpointView
		state, views, err = domain.ApplyEffects(state, activity.Effects)
		if err != nil {
			return domain.ChangePreview{}, domain.ActivityCommit{}, &domain.Error{Code: domain.ErrInvalidChange, Message: "this historical entry would invalidate a later balance or holding quantity"}
		}
		if len(views) == 0 && activity.Kind == domain.ActivityValueUpdate {
			views = activity.Resulting
		}
		projections = append(projections, domain.ActivityProjection{Activity: activity, Resulting: views})
	}
	commit := domain.ActivityCommit{Activity: preview.Activity, Effects: preview.Effects, Resulting: correctionEndpointViews(state, preview.Effects), Replay: projections}
	return preview, commit, nil
}

func commandEffectiveTime(command any) time.Time {
	switch input := command.(type) {
	case domain.MoneyAddedInput:
		return input.EffectiveAt
	case domain.MoneyRemovedInput:
		return input.EffectiveAt
	case domain.CashDividendInput:
		return input.EffectiveAt
	case domain.CashTransferInput:
		return input.EffectiveAt
	case domain.FXConversionInput:
		return input.EffectiveAt
	case domain.PositionTransferInput:
		return input.EffectiveAt
	case domain.PositionAdjustmentInput:
		return input.EffectiveAt
	case domain.PositionImportInput:
		return input.EffectiveAt
	case domain.TradeInput:
		return input.EffectiveAt
	case domain.ValueUpdateInput:
		return input.EffectiveAt
	case domain.DebtDrawInput:
		return input.EffectiveAt
	case domain.DebtPaymentInput:
		return input.EffectiveAt
	}
	return time.Time{}
}
