package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func ordinaryRepeatSource(a domain.Activity) bool {
	if a.ProductContext != nil || a.DividendDetail != nil || a.TradeDetail != nil || a.CorrectionGroupID != nil || a.ReversesActivityID != nil {
		return false
	}
	if !repeatReasonAllowed(a.Kind, a.Reason) {
		return false
	}
	return len(a.Effects) == 1 && a.Effects[0].AccountID != nil && a.Effects[0].Money != nil && a.Effects[0].HoldingID == nil
}

// Keep this narrower than the domain money-reason catalog: reconciliation and
// specialized financial records must never acquire a Repeat draft.
func repeatReasonAllowed(kind domain.ActivityKind, reason domain.ActivityReason) bool {
	switch kind {
	case domain.ActivityCashIn:
		switch reason {
		case domain.ReasonIncome, domain.ReasonInterest, domain.ReasonContribution, domain.ReasonGift, domain.ReasonOther:
			return true
		}
	case domain.ActivityCashOut:
		switch reason {
		case domain.ReasonExpense, domain.ReasonFee, domain.ReasonTax, domain.ReasonInterest, domain.ReasonOther:
			return true
		}
	}
	return false
}

func repeatSourceReason(a domain.Activity) string {
	switch {
	case a.CorrectionGroupID != nil || a.ReversesActivityID != nil:
		return "corrected"
	case a.ProductContext != nil:
		return "product"
	// Holding-attributed fee/tax is stored in DividendDetail, while the cash
	// effect itself has no HoldingID. It is not a cash dividend.
	case a.Kind == domain.ActivityCashOut && (a.Reason == domain.ReasonFee || a.Reason == domain.ReasonTax) && a.DividendDetail != nil && a.TradeDetail == nil:
		return "holding_attribution"
	case a.DividendDetail != nil || a.TradeDetail != nil:
		return "specialized"
	case !repeatReasonAllowed(a.Kind, a.Reason):
		return "unsupported_reason"
	case len(a.Effects) != 1 || a.Effects[0].AccountID == nil || a.Effects[0].Money == nil:
		return "not_single_account"
	case a.Effects[0].HoldingID != nil:
		return "holding_attribution"
	default:
		return "eligible"
	}
}

// RepeatSourceStatuses derives stable UI explanations using the full ledger.
func (s *Service) RepeatSourceStatuses(ctx context.Context, activities []domain.Activity) (map[domain.ActivityID]string, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	statuses := make(map[domain.ActivityID]string, len(activities))
	for _, a := range activities {
		status := repeatSourceReason(a)
		if status == "eligible" {
			reversed, err := s.repository.ActivityHasReversal(ctx, a.HouseholdID, a.ID)
			if err != nil {
				return nil, err
			}
			if reversed {
				status = "reversed"
			}
		}
		statuses[a.ID] = status
	}
	return statuses, nil
}

// RepeatEligibleActivities reads lifecycle state independently of page filters
// and cursors. It is only a UI hint; preview and commit check again under gates.
func (s *Service) RepeatEligibleActivities(ctx context.Context, activities []domain.Activity) (map[domain.ActivityID]bool, error) {
	statuses, err := s.RepeatSourceStatuses(ctx, activities)
	if err != nil {
		return nil, err
	}
	eligible := make(map[domain.ActivityID]bool)
	for id, status := range statuses {
		eligible[id] = status == "eligible"
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
		return &domain.Error{Code: domain.ErrValidation, Field: "sourceActivityId", Message: "only ordinary single-account cash changes can be repeated"}
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
		if a.Kind == domain.ActivityCashIn && repeatReasonAllowed(domain.ActivityCashIn, c.Reason) {
			return nil
		}
	case domain.MoneyRemovedInput:
		if a.Kind == domain.ActivityCashOut && repeatReasonAllowed(domain.ActivityCashOut, c.Reason) && c.HoldingID == nil {
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
