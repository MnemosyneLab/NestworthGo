package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// DerivedDataRepository publishes a fully staged replay in one transaction.
// Source records remain intact if validation or publication fails.
type DerivedDataRepository interface {
	ReplaceDerivedData(context.Context, domain.DerivedDataRebuild) error
}

type DerivedDataRebuildResult struct {
	From           string `json:"from"`
	To             string `json:"to"`
	SnapshotDays   int    `json:"snapshotDays"`
	IncompleteDays int    `json:"incompleteDays"`
}

func (s *Service) RebuildDerivedData(ctx context.Context) (DerivedDataRebuildResult, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return DerivedDataRebuildResult{}, err
	}
	defer unlock()
	repository, ok := s.repository.(DerivedDataRepository)
	if !ok {
		return DerivedDataRebuildResult{}, &domain.Error{Code: domain.ErrUnavailable, Message: "derived data rebuild is unavailable"}
	}
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return DerivedDataRebuildResult{}, err
	}
	if origin == nil {
		return DerivedDataRebuildResult{}, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before rebuilding derived data"}
	}
	now := s.clock()
	batch, err := s.repository.LoadHistoricalSnapshotBatch(ctx, origin.HouseholdID, now)
	if err != nil {
		return DerivedDataRebuildResult{}, err
	}
	state, err := s.changeStateFrom(origin, batch.Portfolio)
	if err != nil {
		return DerivedDataRebuildResult{}, err
	}
	state = correctionOriginState(state, batch.OriginData.Components)
	projections := make([]domain.ActivityProjection, 0, len(batch.Activities))
	for _, activity := range batch.Activities {
		if err := ctx.Err(); err != nil {
			return DerivedDataRebuildResult{}, err
		}
		var views []domain.EndpointView
		state, views, err = domain.ApplyEffects(state, activity.Effects)
		if err != nil {
			return DerivedDataRebuildResult{}, err
		}
		if len(views) == 0 && activity.Kind == domain.ActivityValueUpdate {
			views = activity.Resulting
		}
		projections = append(projections, domain.ActivityProjection{Activity: activity, Resulting: views})
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return DerivedDataRebuildResult{}, err
	}
	from := origin.StartedAt.In(location).Format("2006-01-02")
	today := now.In(location).Format("2006-01-02")
	result := DerivedDataRebuildResult{From: from, To: today}
	publication := domain.DerivedDataRebuild{HouseholdID: origin.HouseholdID, Projections: projections, Current: replayCurrentViews(state, batch), InputGeneration: batch.InputGeneration, AsOf: now}
	quotes := &historicalQuoteCache{instrumentQuotes: make(map[domain.InstrumentID][]domain.InstrumentQuote), fxQuotes: batch.FXQuoteFacts}
	for _, quote := range batch.InstrumentQuoteFacts {
		quotes.instrumentQuotes[quote.InstrumentID] = append(quotes.instrumentQuotes[quote.InstrumentID], quote)
	}
	ctx = context.WithValue(ctx, historicalQuoteCacheKey{}, quotes)
	ctx = context.WithValue(ctx, historicalSnapshotBatchKey{}, &batch)
	first, err := time.Parse("2006-01-02", from)
	if err != nil {
		return DerivedDataRebuildResult{}, err
	}
	for day := first; day.Format("2006-01-02") < today; day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return DerivedDataRebuildResult{}, err
		}
		date := day.Format("2006-01-02")
		midnight, err := domain.ResolveLocalDateTime(day.AddDate(0, 0, 1).Format("2006-01-02"), "00:00", origin.Timezone)
		if err != nil {
			return DerivedDataRebuildResult{}, err
		}
		snapshot, err := s.valueHistoricalSnapshot(ctx, origin, midnight.Add(-time.Millisecond), date)
		if err != nil {
			return DerivedDataRebuildResult{}, err
		}
		snapshot.InputGeneration = batch.InputGeneration
		snapshot.ResolverPolicyVersion = domain.MarketDataResolverPolicy
		publication.Snapshots = append(publication.Snapshots, snapshot)
		publication.ClosedThrough = date
		result.SnapshotDays++
		if !snapshot.Complete {
			result.IncompleteDays++
		}
	}
	if err := repository.ReplaceDerivedData(ctx, publication); err != nil {
		return DerivedDataRebuildResult{}, err
	}
	s.invalidateAnalysis()
	return result, nil
}

// Only observed endpoints receive current values. An account with no opening
// balance and no activity stays unknown instead of acquiring an invented zero.
func replayCurrentViews(state domain.ChangeState, batch domain.HistoricalSnapshotBatch) []domain.EndpointView {
	var effects []domain.ActivityEffect
	for _, activity := range batch.Activities {
		effects = append(effects, activity.RecordedEffects...)
		effects = append(effects, activity.Effects...)
	}
	for _, component := range batch.OriginData.Components {
		e := domain.ActivityEffect{AccountID: component.AccountID, HoldingID: component.HoldingID, Money: component.Amount, Quantity: component.Quantity}
		switch component.Kind {
		case domain.HistoryOriginAccountValue:
			e.Target = domain.EffectTargetAccountValue
		case domain.HistoryOriginAccountCash:
			e.Target = domain.EffectTargetAccountCash
		case domain.HistoryOriginHoldingQuantity:
			e.Target = domain.EffectTargetHoldingQuantity
		default:
			continue
		}
		effects = append(effects, e)
	}
	// Include formerly affected endpoints as well: a superseded correction may
	// have moved the fact to another account or currency. Their old projections
	// must be replaced with the replayed zero instead of remaining visible.
	for _, record := range batch.Portfolio.Accounts {
		if record.LatestValue != nil {
			id, amount := record.Account.ID, record.LatestValue.Amount
			effects = append(effects, domain.ActivityEffect{Target: domain.EffectTargetAccountValue, AccountID: &id, Money: &amount})
		}
	}
	for _, value := range batch.Portfolio.CashValues {
		id, amount := value.AccountID, value.Amount
		if _, ok := state.Cash[id][amount.Currency()]; !ok {
			if state.Cash[id] == nil {
				state.Cash[id] = map[domain.CurrencyCode]domain.Money{}
			}
			state.Cash[id][amount.Currency()], _ = domain.ParseMoney("0", amount.Currency())
		}
		effects = append(effects, domain.ActivityEffect{Target: domain.EffectTargetAccountCash, AccountID: &id, Money: &amount})
	}
	for _, holding := range batch.Portfolio.Holdings {
		id := holding.ID
		effects = append(effects, domain.ActivityEffect{Target: domain.EffectTargetHoldingQuantity, HoldingID: &id})
	}
	for _, baseline := range batch.ZeroAccountBaselines {
		id, amount := baseline.AccountID, baseline.Amount
		effects = append(effects, domain.ActivityEffect{Target: domain.EffectTargetAccountValue, AccountID: &id, Money: &amount})
	}
	return correctionEndpointViews(state, effects)
}
