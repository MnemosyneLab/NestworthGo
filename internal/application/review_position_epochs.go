package application

import (
	"context"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewPositionEpoch struct {
	OpenedAt     *time.Time
	OpenedBy     *domain.ActivityID
	ClosedAt     *time.Time
	ClosedBy     *domain.ActivityID
	StartUnknown bool
}

type ReviewPositionTimeline struct {
	Known    bool
	Reason   string
	Quantity string
	Epochs   []ReviewPositionEpoch
}

// ProjectReviewPositionTimeline groups every quantity effect by its business
// Activity before detecting zero crossings. A two-legged account transfer is
// one atomic no-op for the instrument, while a one-legged transfer can close
// it. It never infers an earlier quantity from today's Holding row.
func ProjectReviewPositionTimeline(ctx context.Context, batch domain.HistoricalSnapshotBatch, instrumentID domain.InstrumentID, cutoff time.Time) (ReviewPositionTimeline, error) {
	result := ReviewPositionTimeline{Epochs: []ReviewPositionEpoch{}}
	if batch.Origin.StartedAt.IsZero() || !batch.Origin.StartedAt.Before(cutoff) {
		result.Reason = "history_origin"
		return result, nil
	}
	baselineByHolding := map[domain.HoldingID]domain.Quantity{}
	for _, component := range batch.OriginData.Components {
		if component.Kind == domain.HistoryOriginHoldingQuantity && component.HoldingID != nil && component.InstrumentID != nil && *component.InstrumentID == instrumentID && component.Quantity != nil {
			baselineByHolding[*component.HoldingID] = *component.Quantity
		}
	}
	holdingInstrument := map[domain.HoldingID]domain.InstrumentID{}
	quantity := decimal.Zero
	for _, holding := range batch.Portfolio.Holdings {
		if err := ctx.Err(); err != nil {
			return ReviewPositionTimeline{}, err
		}
		holdingInstrument[holding.ID] = holding.InstrumentID
		if holding.InstrumentID != instrumentID || holding.CreatedAt.After(batch.Origin.StartedAt) {
			continue
		}
		baseline, ok := baselineByHolding[holding.ID]
		if !ok {
			result.Reason = "historical_quantity"
			return result, nil
		}
		quantity = quantity.Add(baseline.Decimal())
	}
	for id := range baselineByHolding {
		if holdingInstrument[id] != instrumentID {
			result.Reason = "history_origin_mismatch"
			return result, nil
		}
	}
	if quantity.IsPositive() {
		result.Epochs = append(result.Epochs, ReviewPositionEpoch{StartUnknown: true})
	}
	activities := append([]domain.Activity(nil), batch.Activities...)
	sort.SliceStable(activities, func(i, j int) bool {
		if !activities[i].EffectiveAt.Equal(activities[j].EffectiveAt) {
			return activities[i].EffectiveAt.Before(activities[j].EffectiveAt)
		}
		if !activities[i].CreatedAt.Equal(activities[j].CreatedAt) {
			return activities[i].CreatedAt.Before(activities[j].CreatedAt)
		}
		return activities[i].ID < activities[j].ID
	})
	everEffect := map[domain.HoldingID]bool{}
	for _, activity := range activities {
		for _, effect := range activity.Effects {
			if effect.Quantity != nil && effect.HoldingID != nil {
				everEffect[*effect.HoldingID] = true
			}
		}
	}
	for _, activity := range activities {
		if err := ctx.Err(); err != nil {
			return ReviewPositionTimeline{}, err
		}
		if activity.EffectiveAt.Before(batch.Origin.StartedAt) || !activity.EffectiveAt.Before(cutoff) {
			continue
		}
		delta := decimal.Zero
		for _, effect := range activity.Effects {
			if effect.Quantity == nil || effect.HoldingID == nil {
				continue
			}
			owner := holdingInstrument[*effect.HoldingID]
			if effect.InstrumentID != nil && owner != "" && *effect.InstrumentID != owner {
				result.Reason = "holding_instrument_mismatch"
				return result, nil
			}
			if owner != instrumentID {
				continue
			}
			if effect.Direction == domain.EffectAdded {
				delta = delta.Add(effect.Quantity.Decimal())
			} else if effect.Direction == domain.EffectRemoved {
				delta = delta.Sub(effect.Quantity.Decimal())
			} else {
				result.Reason = "quantity_effect_direction"
				return result, nil
			}
		}
		if delta.IsZero() {
			continue
		}
		before := quantity
		quantity = quantity.Add(delta)
		if quantity.IsNegative() {
			result.Reason = "negative_historical_quantity"
			return result, nil
		}
		if !before.IsPositive() && quantity.IsPositive() {
			at := activity.EffectiveAt
			id := activity.ID
			result.Epochs = append(result.Epochs, ReviewPositionEpoch{OpenedAt: &at, OpenedBy: &id})
		}
		if before.IsPositive() && !quantity.IsPositive() {
			at := activity.EffectiveAt
			id := activity.ID
			last := len(result.Epochs) - 1
			result.Epochs[last].ClosedAt = &at
			result.Epochs[last].ClosedBy = &id
		}
	}
	for _, holding := range batch.Portfolio.Holdings {
		if holding.InstrumentID == instrumentID && holding.CreatedAt.After(batch.Origin.StartedAt) && holding.CreatedAt.Before(cutoff) && holding.Quantity.Decimal().IsPositive() && !everEffect[holding.ID] {
			result.Reason = "historical_quantity"
			return result, nil
		}
	}
	known, err := domain.NewQuantity(quantity)
	if err != nil {
		return ReviewPositionTimeline{}, err
	}
	result.Known = true
	result.Quantity = known.Canonical()
	return result, nil
}
