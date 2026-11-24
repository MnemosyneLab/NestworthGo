package application

import (
	"context"
	"slices"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ExistingTradeCostPreview adds native cost and realized gain to the ordinary
// trade preview. A nil basis means the stored cost evidence was insufficient.
type ExistingTradeCostPreview struct {
	PreviousCost  *domain.MoneyView
	RemainingCost *domain.MoneyView
	AverageCost   *domain.UnitPrice
	RealizedGain  *domain.SignedMoneyView
}

// PreviewExistingTradeCost projects the prospective activity through the same cost
// replay used by HoldingGain. Supply the guarded preview for an existing
// Holding while retaining the same WithRead scope. The effective time must
// follow its last affected event so the preview cannot reorder later lots.
func (s *Service) PreviewExistingTradeCost(ctx context.Context, preview domain.ChangePreview) (ExistingTradeCostPreview, error) {
	activity := preview.Activity
	trade := activity.TradeDetail
	if trade == nil || (trade.Side != domain.TradeBuy && trade.Side != domain.TradeSell) {
		return ExistingTradeCostPreview{}, &domain.Error{Code: domain.ErrInvalidTrade, Field: "side"}
	}
	endpoint := slices.IndexFunc(preview.Resulting, func(view domain.EndpointView) bool {
		return view.Target == domain.EffectTargetHoldingQuantity && view.HoldingID != nil && *view.HoldingID == trade.HoldingID
	})
	if endpoint < 0 {
		return ExistingTradeCostPreview{}, &domain.Error{Code: domain.ErrIntegrity, Field: "holdingId", Message: "trade preview is missing its resulting quantity"}
	}
	quantity := preview.Resulting[endpoint].Quantity
	currency := trade.Gross.Currency()
	var unavailable ExistingTradeCostPreview
	if quantity == "0" {
		unavailable.RemainingCost = &domain.MoneyView{Amount: "0", Currency: currency}
	}
	sequenceField, kind := "saleSequence", domain.CostBasisSell
	if trade.Side == domain.TradeBuy {
		sequenceField, kind = "buySequence", domain.CostBasisBuy
	}
	if err := s.checkTradeSequence(ctx, activity.HouseholdID, trade, activity.EffectiveAt, sequenceField); err != nil {
		return ExistingTradeCostPreview{}, err
	}
	inputs, err := s.repository.ReadGainSnapshot(ctx)
	if err != nil {
		return ExistingTradeCostPreview{}, err
	}
	holding, _, err := gainHolding(inputs.Portfolio, trade.HoldingID)
	if err != nil {
		return ExistingTradeCostPreview{}, err
	}
	for _, event := range inputs.CostEvents[holding.ID] {
		if !event.EffectiveAt.Before(activity.EffectiveAt) {
			return ExistingTradeCostPreview{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: sequenceField}
		}
	}
	starting, events, err := newGainSnapshotReplay(inputs).preparedEvents(ctx, holding.ID, nil)
	if err != nil {
		if missingCostBasis(err) {
			return unavailable, nil
		}
		return ExistingTradeCostPreview{}, err
	}
	var view ExistingTradeCostPreview
	if trade.Side == domain.TradeBuy {
		before, err := domain.ReplayCostBasis(starting, events)
		if err != nil {
			if missingCostBasis(err) {
				return unavailable, nil
			}
			return ExistingTradeCostPreview{}, err
		}
		if !before.Current.Quantity.Decimal().Equal(holding.Quantity.Decimal()) {
			return unavailable, nil
		}
		view.PreviousCost, err = tradeLotCost(before.Current, currency)
		if err != nil {
			return ExistingTradeCostPreview{}, err
		}
	}
	// Append the validated trade with its exact gross and fee.
	result, err := domain.ReplayCostBasis(starting, append(events, domain.CostBasisEvent{
		ActivityID: activity.ID, EffectiveAt: activity.EffectiveAt, CreatedAt: activity.CreatedAt,
		Kind: kind, Quantity: trade.Quantity, UnitPrice: &trade.UnitPrice,
		Gross: &trade.Gross, Fee: trade.Fee, Currency: currency,
	}))
	if err != nil {
		if missingCostBasis(err) {
			return unavailable, nil
		}
		return ExistingTradeCostPreview{}, err
	}
	// If stored quantity and cost evidence disagree, the ordinary
	// HoldingGain read also treats the basis as unavailable.
	if quantity != result.Current.Quantity.Canonical() {
		return unavailable, nil
	}
	view.RemainingCost, err = tradeLotCost(result.Current, currency)
	if err != nil {
		return ExistingTradeCostPreview{}, err
	}
	view.AverageCost = &result.Current.AverageUnitCost
	if trade.Side == domain.TradeSell {
		gain := result.Realized[len(result.Realized)-1].RealizedGain
		view.RealizedGain = &domain.SignedMoneyView{Amount: gain.CanonicalAmount(), Currency: currency}
	}
	return view, nil
}

func tradeLotCost(lot domain.CostLot, currency domain.CurrencyCode) (*domain.MoneyView, error) {
	total, err := lot.Quantity.Multiply(lot.AverageUnitCost)
	if err != nil {
		return nil, err
	}
	cost, err := moneyView(total, currency)
	if err != nil {
		return nil, err
	}
	return &cost, nil
}

// The gain snapshot applies corrections retrospectively. Inspect the audit
// stream as well so a later reversal cannot change the basis of an insertion.
func (s *Service) checkTradeSequence(ctx context.Context, household domain.HouseholdID, trade *domain.TradeDetail, at time.Time, sequenceField string) error {
	query := domain.ActivityQuery{InstrumentID: &trade.InstrumentID, Limit: 50}
	for {
		page, err := s.repository.ListActivityPage(ctx, household, query)
		if err != nil {
			return err
		}
		for _, activity := range page.Activities {
			if activity.EffectiveAt.Before(at) {
				return nil
			}
			for _, effect := range activity.Effects {
				if effect.HoldingID != nil && *effect.HoldingID == trade.HoldingID &&
					(effect.Target == domain.EffectTargetHoldingQuantity || effect.Target == domain.EffectTargetHoldingCost) {
					return &domain.Error{Code: domain.ErrInvalidChangeTime, Field: sequenceField}
				}
			}
		}
		if !page.HasMore || page.Next == nil {
			return nil
		}
		query.After = page.Next
	}
}
