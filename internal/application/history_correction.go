package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type correctionPlan struct {
	inverse     domain.ChangePreview
	replacement domain.ChangePreview
	projections []domain.ActivityProjection
	current     []domain.EndpointView
}

// Rewind to the original transaction, replace it, then validate every following
// event. Inverting the purchase against today's sold-down position is unsafe.
func (s *Service) replayCorrection(ctx context.Context, original domain.Activity, command any) (correctionPlan, error) {
	auditOriginal := original
	state, err := s.changeState(ctx)
	if err != nil {
		return correctionPlan{}, err
	}
	activities, err := s.repository.ListActivitiesUntil(ctx, original.HouseholdID, s.clock())
	if err != nil {
		return correctionPlan{}, err
	}
	index := -1
	for i, a := range activities {
		if a.ID == original.ID {
			index = i
			original = a
			break
		}
	}
	if index < 0 {
		return correctionPlan{}, &domain.Error{Code: domain.ErrCannotFixChange, Message: "the change is no longer active"}
	}
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return correctionPlan{}, err
	}
	components, err := s.repository.ListHistoryOriginComponents(ctx, origin.ID)
	if err != nil {
		return correctionPlan{}, err
	}
	state = correctionOriginState(state, components)
	for _, a := range activities[:index] {
		state, _, err = domain.ApplyEffects(state, a.Effects)
		if err != nil {
			return correctionPlan{}, err
		}
	}
	// The reversal is audit evidence. Its original magnitudes may now be a
	// no-op economically (for example after an earlier balance correction).
	inverse := correctionReversal(auditOriginal, original, s.clock())
	replacement, err := previewCorrectionAt(state, command, original.EffectiveAt)
	if err != nil {
		return correctionPlan{}, err
	}
	inverse.Activity.EffectiveAt = original.EffectiveAt
	inverse.Activity.EffectiveLocalDate = original.EffectiveLocalDate
	replacement.Activity.Resulting = replacement.Resulting
	hypothetical := append([]domain.Activity{}, activities...)
	hypothetical[index] = replacement.Activity
	hypothetical, err = domain.ReplayMoneyEffects(hypothetical, components)
	if err != nil {
		return correctionPlan{}, err
	}
	projections := []domain.ActivityProjection{}
	for _, a := range hypothetical[index:] {
		var views []domain.EndpointView
		state, views, err = domain.ApplyEffects(state, a.Effects)
		if err != nil {
			return correctionPlan{}, &domain.Error{Code: domain.ErrCannotFixChange, Message: "the correction would invalidate a later balance or holding quantity"}
		}
		if len(views) == 0 && a.Kind == domain.ActivityValueUpdate {
			views = a.Resulting
		}
		projections = append(projections, domain.ActivityProjection{Activity: a, Resulting: views})
	}
	inverse.Resulting = correctionEndpointViews(state, inverse.Effects)
	current := correctionEndpointViews(state, append(append([]domain.ActivityEffect{}, inverse.Effects...), replacement.Effects...))
	return correctionPlan{inverse: inverse, replacement: replacement, projections: projections, current: current}, nil
}

func correctionEndpointViews(state domain.ChangeState, effects []domain.ActivityEffect) []domain.EndpointView {
	views := []domain.EndpointView{}
	for _, e := range effects {
		v := domain.EndpointView{Target: e.Target, AccountID: e.AccountID, HoldingID: e.HoldingID}
		if e.HoldingID != nil {
			h := state.Holdings[*e.HoldingID]
			v.Name = h.InstrumentName
			v.Quantity = h.Current.Canonical()
		} else if e.AccountID != nil {
			a := state.Accounts[*e.AccountID]
			v.Name = a.Name
			amount := a.Current
			if e.Target == domain.EffectTargetAccountCash {
				amount = state.Cash[*e.AccountID][e.Money.Currency()]
			}
			v.Amount = amount.CanonicalAmount()
			v.Currency = amount.Currency()
		}
		duplicate := false
		for _, old := range views {
			if old.Target == v.Target && ((old.HoldingID != nil && v.HoldingID != nil && *old.HoldingID == *v.HoldingID) || (old.AccountID != nil && v.AccountID != nil && *old.AccountID == *v.AccountID && old.Currency == v.Currency)) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			views = append(views, v)
		}
	}
	return views
}

func previewCorrectionAt(state domain.ChangeState, command any, at time.Time) (domain.ChangePreview, error) {
	switch input := command.(type) {
	case domain.MoneyAddedInput:
		input.EffectiveAt = at
		command = input
	case domain.MoneyRemovedInput:
		input.EffectiveAt = at
		command = input
	case domain.CashDividendInput:
		input.EffectiveAt = at
		command = input
	case domain.CashTransferInput:
		input.EffectiveAt = at
		command = input
	case domain.FXConversionInput:
		input.EffectiveAt = at
		command = input
	case domain.PositionTransferInput:
		input.EffectiveAt = at
		command = input
	case domain.PositionAdjustmentInput:
		input.EffectiveAt = at
		command = input
	case domain.PositionCostAdjustmentInput:
		input.EffectiveAt = at
		command = input
	case domain.TradeInput:
		input.EffectiveAt = at
		command = input
	case domain.ValueUpdateInput:
		input.EffectiveAt = at
		command = input
	case domain.DebtDrawInput:
		input.EffectiveAt = at
		command = input
	case domain.DebtPaymentInput:
		input.EffectiveAt = at
		command = input
	}
	return domain.PreviewChange(state, command)
}

func correctionOriginState(state domain.ChangeState, components []domain.HistoryOriginComponent) domain.ChangeState {
	for id, a := range state.Accounts {
		a.Current, _ = domain.ParseMoney("0", a.Currency)
		state.Accounts[id] = a
	}
	state.Cash = make(map[domain.AccountID]map[domain.CurrencyCode]domain.Money)
	for id, h := range state.Holdings {
		h.Current, _ = domain.ParseQuantity("0")
		state.Holdings[id] = h
	}
	for _, c := range components {
		switch c.Kind {
		case domain.HistoryOriginAccountValue:
			if c.AccountID != nil && c.Amount != nil {
				a := state.Accounts[*c.AccountID]
				a.Current = *c.Amount
				state.Accounts[*c.AccountID] = a
			}
		case domain.HistoryOriginAccountCash:
			if c.AccountID != nil && c.Amount != nil {
				if state.Cash[*c.AccountID] == nil {
					state.Cash[*c.AccountID] = map[domain.CurrencyCode]domain.Money{}
				}
				state.Cash[*c.AccountID][c.Amount.Currency()] = *c.Amount
			}
		case domain.HistoryOriginHoldingQuantity:
			if c.HoldingID != nil && c.Quantity != nil {
				h := state.Holdings[*c.HoldingID]
				h.Current = *c.Quantity
				state.Holdings[*c.HoldingID] = h
			}
		}
	}
	return state
}

func correctionReversal(audit, effective domain.Activity, now time.Time) domain.ChangePreview {
	activity := domain.Activity{ID: domain.NewActivityID(), HouseholdID: audit.HouseholdID, Kind: domain.ActivityReversal, Reason: domain.ReasonOther, EffectiveAt: effective.EffectiveAt, EffectiveLocalDate: effective.EffectiveLocalDate, CreatedAt: now, ReversesActivityID: &audit.ID}
	effects := make([]domain.ActivityEffect, 0, len(audit.Effects))
	for i := len(audit.Effects) - 1; i >= 0; i-- {
		effect := audit.Effects[i]
		effect.ID = domain.NewActivityEffectID()
		effect.ActivityID = activity.ID
		effect.Sequence = len(effects) + 1
		if effect.Direction == domain.EffectAdded {
			effect.Direction = domain.EffectRemoved
		} else {
			effect.Direction = domain.EffectAdded
		}
		effects = append(effects, effect)
	}
	activity.Effects = effects
	return domain.ChangePreview{Activity: activity, Effects: effects}
}
