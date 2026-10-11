package holding

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type CashBalancePreviewDTO struct {
	PlanID      string  `json:"planId"`
	Current     string  `json:"current"`
	Target      string  `json:"target"`
	Delta       string  `json:"delta"`
	Currency    string  `json:"currency"`
	Timezone    string  `json:"timezone"`
	EffectiveAt string  `json:"effectiveAt"`
	Reason      *string `json:"reason,omitempty"`
}

func (s *Service) PreviewCashBalance(ctx context.Context, accountID, amount, currency string, reason *string) (CashBalancePreviewDTO, error) {
	id, err := domain.ParseAccountID(accountID)
	if err != nil {
		return CashBalancePreviewDTO{}, apierror.Wrap(err)
	}
	plan, err := s.app.PreviewCashBalance(ctx, id, amount, currency, reason)
	if err != nil {
		return CashBalancePreviewDTO{}, apierror.Wrap(err)
	}
	return CashBalancePreviewDTO{PlanID: s.cashPlans.Put(plan), Current: plan.Current, Target: plan.Target, Delta: plan.Delta, Currency: plan.Currency, Timezone: plan.Timezone, EffectiveAt: wire.FormatTime(plan.EffectiveAt), Reason: plan.Reason}, nil
}
func (s *Service) ConfirmCashBalance(ctx context.Context, planID string) (wire.AccountCashValueDTO, error) {
	plan, err := s.cashPlans.Get(planID)
	if err != nil {
		return wire.AccountCashValueDTO{}, apierror.Wrap(err)
	}
	value, err := s.app.ConfirmCashBalance(ctx, plan)
	if err != nil {
		return wire.AccountCashValueDTO{}, apierror.Wrap(err)
	}
	return wire.FromAccountCashValue(value), nil
}
