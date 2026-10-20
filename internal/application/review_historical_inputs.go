package application

import (
	"context"
	"slices"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReviewHistoricalCoverage accompanies one day. A missing field never turns
// into a zero amount or false predicate without an authoritative starting
// point. This read-only calculation does not write history snapshots.
type ReviewHistoricalCoverage struct {
	On      string   `json:"on"`
	Missing []string `json:"missing"`
}

// ProjectReviewHistoricalDay consumes a batch captured in one repository read
// transaction. An absent cost-event entry is unknown, not an empty history.
// It is a pure projection suitable for preview or staged reconciliation.
func ProjectReviewHistoricalDay(ctx context.Context, captured domain.ReviewHistoricalBatch, providerKey string, spec domain.ReviewRuleSpec, instrumentID domain.InstrumentID, on string) (domain.ReviewDailyInput, ReviewHistoricalCoverage, error) {
	var err error
	spec, err = spec.Normalize()
	if err != nil {
		return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, err
	}
	cutoff, err := domain.ReviewCivilCutoff(on, spec.Schedule.Timezone)
	if err != nil {
		return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, err
	}
	if captured.History.Origin.StartedAt.IsZero() || !captured.History.Origin.StartedAt.Before(cutoff) {
		return domain.ReviewDailyInput{On: on, InstrumentID: instrumentID, FXProviderKey: providerKey}, ReviewHistoricalCoverage{On: on, Missing: []string{"history_origin"}}, nil
	}
	portfolio, err := (HistoricalReplay{batch: &captured.History}).Snapshot(ctx, &captured.History.Origin, cutoff.Add(-time.Nanosecond))
	if err != nil {
		return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, err
	}
	return projectReviewHistoricalDayFromPortfolio(ctx, captured, providerKey, spec, instrumentID, on, cutoff, portfolio)
}

func projectReviewHistoricalDayFromPortfolio(ctx context.Context, captured domain.ReviewHistoricalBatch, providerKey string, spec domain.ReviewRuleSpec, instrumentID domain.InstrumentID, on string, cutoff time.Time, portfolio domain.PortfolioSnapshot) (domain.ReviewDailyInput, ReviewHistoricalCoverage, error) {
	quoteSeries := domain.NewReviewDailyQuoteSeries(instrumentID, portfolio.InstrumentQuotes, portfolio.InstrumentClosures...)
	return projectReviewHistoricalDayFromPortfolioWithQuoteSeries(ctx, captured, providerKey, spec, instrumentID, on, cutoff, portfolio, quoteSeries)
}

func projectReviewHistoricalDayFromPortfolioWithQuoteSeries(ctx context.Context, captured domain.ReviewHistoricalBatch, providerKey string, spec domain.ReviewRuleSpec, instrumentID domain.InstrumentID, on string, cutoff time.Time, portfolio domain.PortfolioSnapshot, quoteSeries domain.ReviewDailyQuoteSeries) (domain.ReviewDailyInput, ReviewHistoricalCoverage, error) {
	batch := captured.History
	input := domain.ReviewDailyInput{On: on, InstrumentID: instrumentID, FXProviderKey: providerKey, Performance: map[int]domain.QuotePerformance{}, SelectedFX: map[domain.CurrencyCode]domain.ReviewSelectedFX{}}
	coverage := ReviewHistoricalCoverage{On: on, Missing: []string{}}
	if batch.Origin.StartedAt.IsZero() || !batch.Origin.StartedAt.Before(cutoff) {
		coverage.Missing = append(coverage.Missing, "history_origin")
		return input, coverage, nil
	}
	// HistoricalReplay.Snapshot includes an effective fact at its cutoff. The
	// review day is half-open, so callers pass the last instant before the next day.
	positionAt := cutoff.Add(-time.Nanosecond)
	var instrument *domain.Instrument
	for index := range portfolio.Instruments {
		if portfolio.Instruments[index].ID == instrumentID {
			instrument = &portfolio.Instruments[index]
			break
		}
	}
	if instrument == nil {
		// The ID did not exist at this economic day. Once the complete origin
		// proves the ID was absent, its position is known zero.
		zero := "0"
		input.Quantity = &zero
		return input, coverage, nil
	}
	// Type/currency can be edited without historical values in older schemas.
	// A later updated_at cannot prove what those fields were on D.
	if !instrument.UpdatedAt.After(positionAt) {
		kind, currency := instrument.Type, instrument.QuoteCurrency
		input.InstrumentType = &kind
		input.NativeCurrency = &currency
	} else {
		coverage.Missing = append(coverage.Missing, "historical_instrument_metadata")
	}
	// Cost replay and quantity coverage only read these two origin fields.
	// Keep membership even for a nil quantity, and preserve last-entry wins.
	// Copying the full (metadata-heavy) component into every daily map creates
	// substantial allocation during full preparation without adding evidence.
	baselines := make(map[domain.HoldingID]reviewOriginCostBaseline)
	for _, component := range batch.OriginData.Components {
		if component.Kind == domain.HistoryOriginHoldingQuantity && component.HoldingID != nil {
			baselines[*component.HoldingID] = reviewOriginCostBaseline{Quantity: component.Quantity, UnitCost: component.UnitCost}
		}
	}
	holdings := make([]domain.Holding, 0)
	quantity := decimal.Zero
	quantityKnown := true
	for _, holding := range portfolio.Holdings {
		if err := ctx.Err(); err != nil {
			return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, err
		}
		if holding.InstrumentID != instrumentID {
			continue
		}
		holdings = append(holdings, holding)
		_, hasOrigin := baselines[holding.ID]
		if !hasOrigin && !holding.CreatedAt.After(batch.Origin.StartedAt) {
			quantityKnown = false
		}
		quantity = quantity.Add(holding.Quantity.Decimal())
	}
	if quantityKnown {
		known, err := domain.NewQuantity(quantity)
		if err != nil {
			return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, err
		}
		value := known.Canonical()
		input.Quantity = &value
	} else {
		coverage.Missing = append(coverage.Missing, "historical_quantity")
	}
	if input.Quantity != nil && quantity.IsPositive() && input.NativeCurrency != nil {
		historyCoverage := append([]domain.InstrumentHistoryCoverage(nil), portfolio.InstrumentHistoryCoverage...)
		for _, closure := range portfolio.InstrumentClosures {
			if closure.InstrumentID == instrument.ID {
				historyCoverage = append(historyCoverage, domain.InstrumentHistoryCoverage{InstrumentID: instrument.ID, ClosedDates: []string{closure.Date}})
			}
		}
		selection := selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(*instrument, portfolio.InstrumentQuotes, historyCoverage, on, positionAt)
		quote := selection.quote
		if quote == nil || !selection.coverageComplete {
			coverage.Missing = append(coverage.Missing, "daily_close_missing")
		} else {
			exact := quantity.Mul(quote.UnitPrice.Decimal()).String()
			input.AmountExact = &exact
		}
	}
	needsCost, lookbacks, currencies := reviewRequiredInputs(spec.Predicate)
	if needsCost && input.Quantity != nil && quantity.IsPositive() {
		cost := decimal.Zero
		known := true
		for _, holding := range holdings {
			if holding.Quantity.IsZero() {
				continue
			}
			lot, ok := reviewCostLot(holding.ID, portfolio.Holdings, baselines, captured.CostEvents, batch.Origin.StartedAt, cutoff, nil, map[domain.HoldingID]bool{})
			if !ok || !lot.Current.Quantity.Decimal().Equal(holding.Quantity.Decimal()) {
				known = false
				break
			}
			cost = cost.Add(lot.Current.Quantity.Decimal().Mul(lot.Current.AverageUnitCost.Decimal()))
		}
		if known {
			value := cost.String()
			input.CostExact = &value
		} else {
			coverage.Missing = append(coverage.Missing, "historical_cost")
		}
	}
	if input.InstrumentType != nil {
		for _, lookback := range lookbacks {
			item, reason := quoteSeries.ResolveDailyPerformance(*instrument, on, lookback, spec.DataPolicy.BaselineMaxGapDays)
			if item == nil {
				coverage.Missing = append(coverage.Missing, reason)
				continue
			}
			input.Performance[lookback] = *item
		}
	}
	if input.NativeCurrency != nil && portfolio.Household != nil {
		for _, target := range currencies {
			if target == *input.NativeCurrency {
				continue
			}
			preference := implicitFXPreferenceTo(portfolio, *input.NativeCurrency, target)
			quote := selectHistoricalFXQuoteAtMarketDate(preference, portfolio.FXQuotes, *input.NativeCurrency, target, providerKey, on, positionAt)
			if quote == nil {
				coverage.Missing = append(coverage.Missing, "fx_missing")
				continue
			}
			kind := "timestamped"
			if quote.ObservationKind == string(FXObservationDailyReference) && quote.EffectiveDate != "" {
				kind = "daily_reference"
			}
			input.SelectedFX[target] = domain.ReviewSelectedFX{Quote: *quote, EconomicKind: kind}
		}
	}
	slices.Sort(coverage.Missing)
	coverage.Missing = slices.Compact(coverage.Missing)
	return input, coverage, nil
}

func reviewRequiredInputs(node domain.ReviewPredicateNode) (bool, []int, []domain.CurrencyCode) {
	var cost bool
	lookbacks := map[int]bool{}
	currencies := map[domain.CurrencyCode]bool{}
	var visit func(domain.ReviewPredicateNode)
	visit = func(n domain.ReviewPredicateNode) {
		if n.Leaf != nil {
			if n.Leaf.Metric == "cost" {
				cost = true
			}
			if n.Leaf.Metric == "price_change" || n.Leaf.Metric == "annualized" {
				lookbacks[n.Leaf.LookbackDays] = true
			}
			if (n.Leaf.Metric == "amount" || n.Leaf.Metric == "cost") && n.Leaf.FXMode == "convert_on_day" {
				currencies[n.Leaf.Currency] = true
			}
			return
		}
		for _, child := range n.All {
			visit(child)
		}
		for _, child := range n.Any {
			visit(child)
		}
	}
	visit(node)
	periods := make([]int, 0, len(lookbacks))
	for n := range lookbacks {
		periods = append(periods, n)
	}
	sort.Ints(periods)
	targets := make([]domain.CurrencyCode, 0, len(currencies))
	for c := range currencies {
		targets = append(targets, c)
	}
	slices.Sort(targets)
	return cost, periods, targets
}

// reviewCostLot never derives a starting quantity from today's holding. It
// recursively resolves transfer-source cost just before the atomic transfer.
// An unproven baseline or missing event batch yields unknown cost.
type reviewOriginCostBaseline struct {
	Quantity *domain.Quantity
	UnitCost *domain.UnitPrice
}

func reviewCostLot(id domain.HoldingID, holdings []domain.Holding, baselines map[domain.HoldingID]reviewOriginCostBaseline, eventsByHolding map[domain.HoldingID][]domain.CostBasisEvent, originAt, cutoff time.Time, boundary *domain.CostBasisBoundary, active map[domain.HoldingID]bool) (domain.CostBasisResult, bool) {
	if active[id] {
		return domain.CostBasisResult{}, false
	}
	active[id] = true
	defer delete(active, id)
	var holding *domain.Holding
	for index := range holdings {
		if holdings[index].ID == id {
			holding = &holdings[index]
			break
		}
	}
	if holding == nil {
		return domain.CostBasisResult{}, false
	}
	baseline, hasOrigin := baselines[id]
	if !hasOrigin && !holding.CreatedAt.After(originAt) {
		return domain.CostBasisResult{}, false
	}
	events, hasEvents := eventsByHolding[id]
	if !hasEvents {
		return domain.CostBasisResult{}, false
	}
	selected := make([]domain.CostBasisEvent, 0, len(events)+1)
	if hasOrigin && baseline.Quantity != nil {
		selected = append(selected, domain.CostBasisEvent{Kind: domain.CostBasisStartingPoint, Quantity: *baseline.Quantity, UnitCost: baseline.UnitCost})
	}
	for _, event := range events {
		if !event.EffectiveAt.Before(cutoff) || (boundary != nil && !event.Before(*boundary)) {
			continue
		}
		if event.EffectiveAt.Before(originAt) {
			return domain.CostBasisResult{}, false
		}
		if event.Kind == domain.CostBasisTransferIn && event.UnitCost == nil && event.SourceHoldingID != nil {
			transferBoundary := event.Boundary()
			source, ok := reviewCostLot(*event.SourceHoldingID, holdings, baselines, eventsByHolding, originAt, cutoff, &transferBoundary, active)
			if !ok || source.Current.Quantity.IsZero() {
				return domain.CostBasisResult{}, false
			}
			unit := source.Current.AverageUnitCost
			event.UnitCost = &unit
		}
		selected = append(selected, event)
	}
	result, err := domain.ReplayCostBasis(nil, selected)
	return result, err == nil
}
