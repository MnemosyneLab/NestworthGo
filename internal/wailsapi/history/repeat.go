package history

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type repeatPlan struct {
	command                 any
	sourceID                domain.ActivityID
	mutationID, hash, token string
	expires                 time.Time
}
type RepeatPreviewDTO struct {
	PlanID    string                 `json:"planId"`
	Resulting []wire.EndpointViewDTO `json:"resulting"`
}

// Repeat accepts only an ordinary income/expense draft. It does not accept
// transaction identity; sourceID is read-only eligibility metadata.
func (s *Service) PreviewRepeatChange(ctx context.Context, sourceID string, request ChangeCommandRequest) (RepeatPreviewDTO, error) {
	id, err := domain.ParseActivityID(sourceID)
	if err != nil {
		return RepeatPreviewDTO{}, apierror.Wrap(err)
	}
	if !((request.Kind == ChangeMoneyAdded && request.Reason == "income") || (request.Kind == ChangeMoneyRemoved && request.Reason == "expense")) {
		return RepeatPreviewDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrValidation, Message: "only ordinary income and expense can be repeated"})
	}
	// Whitelist fields to exclude identity, lifecycle and unrelated command data.
	clean := ChangeCommandRequest{Kind: request.Kind, AccountID: request.AccountID, Amount: request.Amount, Currency: request.Currency, Reason: request.Reason, Note: request.Note, EffectiveAt: request.EffectiveAt, EffectiveLocalDate: request.EffectiveLocalDate, EffectiveLocalTime: request.EffectiveLocalTime, MutationID: uuid.NewString()}
	if clean.EffectiveAt == "" && (clean.EffectiveLocalDate == "" || clean.EffectiveLocalTime == "") {
		return RepeatPreviewDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrValidation, Field: "effectiveAt", Message: "choose an effective time"})
	}
	if clean.Note != nil {
		note := *clean.Note
		clean.Note = &note
	}
	var command any
	result, token, err := s.app.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
		householdID, timezone, err := s.resolveHistoryContext(ctx)
		if err != nil {
			return nil, err
		}
		command, err = clean.ToCommand(householdID, timezone)
		if err != nil {
			return nil, err
		}
		if err := s.app.ValidateRepeatSource(ctx, id, command); err != nil {
			return nil, err
		}
		return command, nil
	})
	if err != nil {
		return RepeatPreviewDTO{}, apierror.Wrap(err)
	}
	_, hash, err := clean.MutationEnvelope()
	if err != nil {
		return RepeatPreviewDTO{}, apierror.Wrap(err)
	}
	plan := repeatPlan{command: command, sourceID: id, mutationID: clean.MutationID, hash: hash, token: token, expires: time.Now().Add(10 * time.Minute)}
	return RepeatPreviewDTO{PlanID: s.repeatPlans.Put(plan), Resulting: wire.FromChangePreview(result).Resulting}, nil
}
func (s *Service) ConfirmRepeatChange(ctx context.Context, planID string) (wire.ChangePreviewDTO, error) {
	plan, err := s.repeatPlans.Get(planID)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	token := plan.token
	if time.Now().After(plan.expires) {
		token = "expired"
	}
	result, err := s.app.RecordRepeatChangeGuarded(ctx, plan.sourceID, plan.command, plan.mutationID, plan.hash, token)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	return wire.FromChangePreview(result), nil
}
