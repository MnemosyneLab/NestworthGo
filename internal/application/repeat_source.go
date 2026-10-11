package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func ordinaryRepeatSource(a domain.Activity) bool {
	if a.ProductContext != nil || a.CorrectionGroupID != nil || a.ReversesActivityID != nil {
		return false
	}
	if !((a.Kind == domain.ActivityCashIn && a.Reason == domain.ReasonIncome) || (a.Kind == domain.ActivityCashOut && a.Reason == domain.ReasonExpense)) {
		return false
	}
	return len(a.Effects) == 1 && a.Effects[0].AccountID != nil && a.Effects[0].Money != nil && a.Effects[0].HoldingID == nil
}

// RepeatEligibleActivities reads lifecycle state independently of page filters
// and cursors. It is only a UI hint; preview and commit check again under gates.
func (s *Service) RepeatEligibleActivities(ctx context.Context, activities []domain.Activity) (map[domain.ActivityID]bool, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	eligible := make(map[domain.ActivityID]bool)
	for _, a := range activities {
		if !ordinaryRepeatSource(a) {
			continue
		}
		reversed, err := s.repository.ActivityHasReversal(ctx, a.HouseholdID, a.ID)
		if err != nil {
			return nil, err
		}
		eligible[a.ID] = !reversed
	}
	return eligible, nil
}

// ValidateRepeatSource must run under the preview read or ledger write gate.
// The source identity is never assigned to the fresh command or its receipt.
func (s *Service) ValidateRepeatSource(ctx context.Context, id domain.ActivityID, command any) error {
	a, err := s.Activity(ctx, id)
	if err != nil {
		return err
	}
	if !ordinaryRepeatSource(a) {
		return &domain.Error{Code: domain.ErrValidation, Field: "sourceActivityId", Message: "only ordinary income and expense can be repeated"}
	}
	reversed, err := s.repository.ActivityHasReversal(ctx, a.HouseholdID, id)
	if err != nil {
		return err
	}
	if reversed {
		return &domain.Error{Code: domain.ErrAlreadyUndone, Field: "sourceActivityId", Message: "a reversed source cannot be repeated"}
	}
	switch c := command.(type) {
	case domain.MoneyAddedInput:
		if a.Kind == domain.ActivityCashIn && c.Reason == domain.ReasonIncome {
			return nil
		}
	case domain.MoneyRemovedInput:
		if a.Kind == domain.ActivityCashOut && c.Reason == domain.ReasonExpense && c.HoldingID == nil {
			return nil
		}
	}
	return &domain.Error{Code: domain.ErrValidation, Field: "sourceActivityId", Message: "draft must match the ordinary source type"}
}

func (s *Service) RecordRepeatChangeGuarded(ctx context.Context, sourceID domain.ActivityID, command any, mutationID, hash, token string) (domain.ChangePreview, error) {
	return s.recordChangeWithMutationChecked(ctx, command, mutationID, hash, token, true, func(ctx context.Context) error {
		return s.ValidateRepeatSource(ctx, sourceID, command)
	})
}
