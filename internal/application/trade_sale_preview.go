package application

import (
	"context"
	"slices"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// SaleCostPreview exposes the native-currency basis and gain separately
// from the cash and quantity endpoints in the ordinary trade preview.
type SaleCostPreview struct {
	RemainingCost *domain.MoneyView
	RealizedGain  *domain.SignedMoneyView
}

// PreviewSaleCost projects the prospective activity through the same cost
// replay used by HoldingGain. Call it with a guarded trade preview while
// retaining the same WithRead scope. This projection supports a sale after
// the holding's last cost event, so its preview cannot reorder later lots.
func (s *Service) PreviewSaleCost(ctx context.Context, preview domain.ChangePreview) (SaleCostPreview, error) {
	activity := preview.Activity
	trade := activity.TradeDetail
	if trade == nil || trade.Side != domain.TradeSell || activity.Kind != domain.ActivitySell {
		return SaleCostPreview{}, &domain.Error{Code: domain.ErrInvalidTrade, Field: "side"}
	}
	endpoint := slices.IndexFunc(preview.Resulting, func(view domain.EndpointView) bool {
		return view.Target == domain.EffectTargetHoldingQuantity && view.HoldingID != nil && *view.HoldingID == trade.HoldingID
	})
	if endpoint < 0 {
		return SaleCostPreview{}, &domain.Error{Code: domain.ErrIntegrity, Field: "holdingId", Message: "sale preview is missing its resulting quantity"}
	}
	quantity := preview.Resulting[endpoint].Quantity
	currency := trade.Gross.Currency()
	var unavailable SaleCostPreview
	if quantity == "0" {
		unavailable.RemainingCost = &domain.MoneyView{Amount: "0", Currency: currency}
	}
	if err := s.checkSaleSequence(ctx, activity.HouseholdID, trade, activity.EffectiveAt); err != nil {
		return SaleCostPreview{}, err
	}
	inputs, err := s.repository.ReadGainSnapshot(ctx)
	if err != nil {
		return SaleCostPreview{}, err
	}
	holding, _, err := gainHolding(inputs.Portfolio, trade.HoldingID)
	if err != nil {
		return SaleCostPreview{}, err
	}
	for _, event := range inputs.CostEvents[holding.ID] {
		if !event.EffectiveAt.Before(activity.EffectiveAt) {
			return SaleCostPreview{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "saleSequence"}
		}
	}
	starting, events, err := newGainSnapshotReplay(inputs).preparedEvents(ctx, holding.ID, nil)
	if err != nil {
		if missingCostBasis(err) {
			return unavailable, nil
		}
		return SaleCostPreview{}, err
	}
	// Replay appends one already validated prospective Sell. Gross and fee
	// retain their native precision for the realized result.
	result, err := domain.ReplayCostBasis(starting, append(events, domain.CostBasisEvent{
		ActivityID: activity.ID, EffectiveAt: activity.EffectiveAt, CreatedAt: activity.CreatedAt,
		Kind: domain.CostBasisSell, Quantity: trade.Quantity, UnitPrice: &trade.UnitPrice,
		Gross: &trade.Gross, Fee: trade.Fee, Currency: currency,
	}))
	if err != nil {
		if missingCostBasis(err) {
			return unavailable, nil
		}
		return SaleCostPreview{}, err
	}
	// If stored quantity and cost evidence disagree, the ordinary
	// HoldingGain read also treats the basis as unavailable.
	if quantity != result.Current.Quantity.Canonical() {
		return unavailable, nil
	}
	remaining, err := result.Current.Quantity.Multiply(result.Current.AverageUnitCost)
	if err != nil {
		return SaleCostPreview{}, err
	}
	cost, err := moneyView(remaining, currency)
	if err != nil {
		return SaleCostPreview{}, err
	}
	gain := result.Realized[len(result.Realized)-1].RealizedGain
	return SaleCostPreview{
		RemainingCost: &cost, RealizedGain: &domain.SignedMoneyView{Amount: gain.CanonicalAmount(), Currency: currency},
	}, nil
}

// The gain snapshot applies corrections retrospectively. Inspect the audit
// stream as well so a later reversal cannot change the basis of an insertion.
func (s *Service) checkSaleSequence(ctx context.Context, household domain.HouseholdID, trade *domain.TradeDetail, at time.Time) error {
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
					return &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "saleSequence"}
				}
			}
		}
		if !page.HasMore || page.Next == nil {
			return nil
		}
		query.After = page.Next
	}
}
