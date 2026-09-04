package application

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type holdingBridge struct {
	price   decimal.Decimal
	fx      decimal.Decimal
	partial bool
}

func computeAnalysis(input AnalysisInputs, query domain.AnalysisQuery) (domain.PeriodAnalysisResult, error) {
	if err := query.Validate(); err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	if input.Portfolio.Household == nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrNotFound, Message: "household was not found"}
	}
	if input.Origin.Timezone == "" {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Field: "timezone", Message: "analysis requires the History Origin timezone"}
	}
	if _, err := time.LoadLocation(input.Origin.Timezone); err != nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Field: "timezone", Message: "analysis requires a valid History Origin timezone"}
	}
	universe, err := resolveAnalysisUniverse(input, query)
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	if query.Valuation == domain.ValuationNative && len(universe.context.Universe.Currencies) > 1 {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrValidation, Field: "valuation", Message: "native valuation requires a single component currency"}
	}
	byDate := make(map[string]domain.DailyValuationSnapshot, len(input.Snapshots))
	for _, snapshot := range input.Snapshots {
		byDate[snapshot.LocalDate] = snapshot
	}
	start, err := time.Parse("2006-01-02", query.From)
	if err != nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrValidation, Field: "from", Message: "date must use YYYY-MM-DD"}
	}
	end, err := time.Parse("2006-01-02", query.To)
	if err != nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrValidation, Field: "to", Message: "date must use YYYY-MM-DD"}
	}
	result := domain.PeriodAnalysisResult{Query: query, Days: make([]domain.ComponentDay, 0), Coverage: domain.RateCoverage{TotalDays: int(end.Sub(start).Hours()/24) + 1}}
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		localDate := date.Format("2006-01-02")
		daySnapshot, hasDay := byDate[localDate]
		previousDate := date.AddDate(0, 0, -1).Format("2006-01-02")
		previousSnapshot, hasPrevious := byDate[previousDate]
		var previous *domain.DailyValuationSnapshot
		if hasPrevious {
			previous = &previousSnapshot
		}
		endItems := snapshotItemsByComponent(daySnapshot, universe.accounts, universe.instruments)
		var effects []classifiedAnalysisEffect
		if hasDay {
			for _, activity := range input.Activities {
				if activityLocalDate(activity, input.Origin.Timezone) != localDate {
					continue
				}
				classified, classifyErr := universe.classifyActivity(activity, daySnapshot, previous, input, query)
				if classifyErr != nil {
					return domain.PeriodAnalysisResult{}, classifyErr
				}
				effects = append(effects, classified...)
			}
		}
		effectsByComponent := make(map[string][]classifiedAnalysisEffect)
		for _, effect := range effects {
			effectsByComponent[effect.component.Key()] = append(effectsByComponent[effect.component.Key()], effect)
		}
		previousItems := make(map[string]domain.DailyValuationSnapshotItem)
		if previous != nil {
			previousItems = snapshotItemsByComponent(*previous, universe.accounts, universe.instruments)
		}
		for _, component := range universe.context.Universe.Components {
			day, err := buildComponentDay(localDate, component, previousItems[component.Key()], endItems[component.Key()], previousSnapshot, daySnapshot, hasPrevious, hasDay, effectsByComponent[component.Key()], input, query, universe)
			if err != nil {
				return domain.PeriodAnalysisResult{}, err
			}
			result.Days = append(result.Days, day)
		}
	}
	return result, nil
}

func snapshotItemsByComponent(snapshot domain.DailyValuationSnapshot, accounts map[domain.AccountID]domain.Account, instruments map[domain.InstrumentID]domain.Instrument) map[string]domain.DailyValuationSnapshotItem {
	result := make(map[string]domain.DailyValuationSnapshotItem, len(snapshot.Items))
	for _, item := range snapshot.Items {
		account, ok := accounts[item.AccountID]
		if !ok {
			continue
		}
		component := componentIDForItem(item, account)
		if component.InstrumentID != nil {
			if instrument, ok := instruments[*component.InstrumentID]; ok {
				component.Currency = instrument.QuoteCurrency
			}
		}
		result[component.Key()] = item
	}
	return result
}

func buildComponentDay(date string, component domain.ComponentID, previous, current domain.DailyValuationSnapshotItem, previousSnapshot, currentSnapshot domain.DailyValuationSnapshot, hasPrevious, hasCurrent bool, effects []classifiedAnalysisEffect, input AnalysisInputs, query domain.AnalysisQuery, universe analysisUniverse) (domain.ComponentDay, error) {
	day := domain.ComponentDay{Date: domain.LocalDate(date), Component: component, AssetBuckets: make(map[domain.AttributionBucket]domain.SignedMoney), ReturnComponents: make(map[domain.ReturnComponent]domain.SignedMoney), Status: domain.CompletenessOK}
	if !hasPrevious || !hasCurrent || !previous.Complete || !current.Complete {
		day.Status = domain.CompletenessUnavailable
		if hasPrevious && hasCurrent && (previous.Complete || current.Complete) {
			day.Status = domain.CompletenessPartial
		}
		return day, nil
	}
	account := universe.accounts[component.AccountID]
	baseCurrency := input.Portfolio.Household.BaseCurrency
	begin, beginOK, err := snapshotItemValue(previous, account, query.Valuation, baseCurrency)
	if err != nil {
		return domain.ComponentDay{}, err
	}
	ending, endOK, err := snapshotItemValue(current, account, query.Valuation, baseCurrency)
	if err != nil {
		return domain.ComponentDay{}, err
	}
	if !beginOK || !endOK {
		day.Status = domain.CompletenessPartial
		return day, nil
	}
	day.BeginningValue, err = newSigned(begin, valueCurrency(previous, query.Valuation, baseCurrency))
	if err != nil {
		return domain.ComponentDay{}, err
	}
	delta := ending.Sub(begin)
	directTotal := decimal.Zero
	neutralTotal := decimal.Zero
	for _, effect := range effects {
		if effect.amountKnown {
			if effect.bucket != nil {
				if err := addBucket(day.AssetBuckets, *effect.bucket, effect.amount, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
					return domain.ComponentDay{}, err
				}
				directTotal = directTotal.Add(effect.amount)
			} else if effect.neutral {
				neutralTotal = neutralTotal.Add(effect.amount)
			}
		}
	}
	bridge := holdingBridge{}
	if component.HoldingID != nil {
		bridge, err = computeHoldingBridge(component, previous, current, previousSnapshot.CutoffAt, currentSnapshot.CutoffAt, effects, input, query, universe)
	} else if component.InstrumentID == nil {
		bridge, err = computeCashBridge(component, previous, current, previousSnapshot.CutoffAt, currentSnapshot.CutoffAt, effects, input, query, universe)
	}
	if err != nil {
		return domain.ComponentDay{}, err
	}
	if bridge.price.Sign() != 0 {
		if err := addBucket(day.AssetBuckets, domain.BucketPriceChange, bridge.price, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
			return domain.ComponentDay{}, err
		}
	}
	if bridge.fx.Sign() != 0 {
		if err := addBucket(day.AssetBuckets, domain.BucketFXImpact, bridge.fx, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
			return domain.ComponentDay{}, err
		}
	}
	driverTotal := directTotal.Add(bridge.price).Add(bridge.fx)
	knownInternal := neutralTotal
	unexplained := delta.Sub(driverTotal).Sub(knownInternal)
	tolerance := residualTolerance(valueCurrency(current, query.Valuation, baseCurrency), begin)
	residualMoney, residualErr := newSigned(unexplained, valueCurrency(current, query.Valuation, baseCurrency))
	if residualErr != nil {
		return domain.ComponentDay{}, residualErr
	}
	if unexplained.Abs().GreaterThan(tolerance) {
		day.Status = domain.CompletenessPartial
		bucket := domain.BucketResidual
		if err := addBucket(day.AssetBuckets, bucket, unexplained, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
			return domain.ComponentDay{}, err
		}
		day.Residual = &domain.Residual{Amount: residualMoney, Tolerance: tolerance, Visible: true}
	} else if bridge.partial {
		day.Status = domain.CompletenessPartial
	}
	return day, nil
}

func snapshotItemValue(item domain.DailyValuationSnapshotItem, account domain.Account, valuation domain.Valuation, baseCurrency domain.CurrencyCode) (decimal.Decimal, bool, error) {
	if !item.Complete {
		return decimal.Zero, false, nil
	}
	if valuation == domain.ValuationNative {
		if item.NativeAmount == "" {
			return decimal.Zero, false, nil
		}
		value, err := decimal.NewFromString(item.NativeAmount)
		return signedForAccount(value, account), err == nil, err
	}
	if item.BaseAmountExact != "" {
		value, err := decimal.NewFromString(item.BaseAmountExact)
		return signedForAccount(value, account), err == nil, err
	}
	if item.BaseAmount == nil {
		return decimal.Zero, false, nil
	}
	return signedForAccount(item.BaseAmount.Amount(), account), true, nil
}

func signedForAccount(value decimal.Decimal, account domain.Account) decimal.Decimal {
	if account.IsLiability() {
		return value.Neg()
	}
	return value
}

func valueCurrency(item domain.DailyValuationSnapshotItem, valuation domain.Valuation, baseCurrency domain.CurrencyCode) domain.CurrencyCode {
	if valuation == domain.ValuationNative && item.NativeCurrency != "" {
		return item.NativeCurrency
	}
	return baseCurrency
}

func newSigned(value decimal.Decimal, currency domain.CurrencyCode) (domain.SignedMoney, error) {
	return domain.NewSignedMoney(value, currency)
}

func addBucket(buckets map[domain.AttributionBucket]domain.SignedMoney, bucket domain.AttributionBucket, amount decimal.Decimal, currency domain.CurrencyCode) error {
	if amount.IsZero() {
		return nil
	}
	current := decimal.Zero
	if existing, ok := buckets[bucket]; ok {
		current = existing.Amount()
	}
	value, err := newSigned(current.Add(amount), currency)
	if err != nil {
		return err
	}
	buckets[bucket] = value
	return nil
}

func residualTolerance(currency domain.CurrencyCode, beginning decimal.Decimal) decimal.Decimal {
	places := domain.CurrencyFractionDigits(currency)
	minor := decimal.New(1, int32(-places))
	floor := minor.Mul(decimal.NewFromInt(2))
	relative := beginning.Abs().Mul(decimal.NewFromFloat(1e-9))
	if relative.GreaterThan(floor) {
		return relative
	}
	return floor
}

func (u analysisUniverse) convertAmount(value decimal.Decimal, currency domain.CurrencyCode, at time.Time, input AnalysisInputs, query domain.AnalysisQuery, targetCurrency domain.CurrencyCode) (decimal.Decimal, error) {
	if query.Valuation == domain.ValuationNative {
		if currency != targetCurrency {
			return decimal.Zero, &domain.Error{Code: domain.ErrValidation, Field: "valuation", Message: "native amount currency does not match the selected universe"}
		}
		return value, nil
	}
	base := input.Portfolio.Household.BaseCurrency
	if currency == base {
		return value, nil
	}
	rate, ok := fxRateAt(input.FXQuotes, input.Portfolio.FXPreferences, input.Portfolio.Household.ID, currency, base, at)
	if !ok {
		return decimal.Zero, &domain.Error{Code: domain.ErrUnavailable, Field: "fxRate", Message: "an FX rate is unavailable for the activity"}
	}
	return value.Mul(rate), nil
}

func fxRateAt(quotes []domain.FXQuote, preferences []domain.FXPreference, householdID domain.HouseholdID, native, base domain.CurrencyCode, cutoff time.Time) (decimal.Decimal, bool) {
	preferred := domain.QuoteSourceKind("")
	for _, preference := range preferences {
		if preference.HouseholdID == householdID && ((preference.CurrencyA == native && preference.CurrencyB == base) || (preference.CurrencyA == base && preference.CurrencyB == native)) {
			preferred = preference.SourceKind
			break
		}
	}
	var selected *domain.FXQuote
	for index := range quotes {
		quote := &quotes[index]
		if quote.HouseholdID != householdID || quote.QuotedAt.After(cutoff) || (preferred != "" && quote.SourceKind != preferred) {
			continue
		}
		if !((quote.BaseCurrency == native && quote.QuoteCurrency == base) || (quote.BaseCurrency == base && quote.QuoteCurrency == native)) {
			continue
		}
		if selected == nil || quote.QuotedAt.After(selected.QuotedAt) || (quote.QuotedAt.Equal(selected.QuotedAt) && quote.CreatedAt.After(selected.CreatedAt)) {
			selected = quote
		}
	}
	if selected == nil {
		return decimal.Zero, false
	}
	if selected.BaseCurrency == native && selected.QuoteCurrency == base {
		return selected.Rate.Decimal(), true
	}
	return decimal.NewFromInt(1).Div(selected.Rate.Decimal()), true
}

func (u analysisUniverse) quoteForItem(item domain.DailyValuationSnapshotItem, quotes []domain.InstrumentQuote, cutoff time.Time) *domain.InstrumentQuote {
	if item.InstrumentID == nil {
		return nil
	}
	if item.QuoteID != nil {
		for index := range quotes {
			if quotes[index].ID.String() == *item.QuoteID && quotes[index].InstrumentID == *item.InstrumentID && quotes[index].Currency == item.NativeCurrency {
				return &quotes[index]
			}
		}
		return nil
	}
	var selected *domain.InstrumentQuote
	for index := range quotes {
		quote := &quotes[index]
		if quote.InstrumentID != *item.InstrumentID || quote.QuotedAt.After(cutoff) {
			continue
		}
		if selected == nil || quote.QuotedAt.After(selected.QuotedAt) || (quote.QuotedAt.Equal(selected.QuotedAt) && quote.CreatedAt.After(selected.CreatedAt)) {
			selected = quote
		}
	}
	return selected
}

func (u analysisUniverse) fxForItem(item domain.DailyValuationSnapshotItem, input AnalysisInputs, cutoff time.Time) (decimal.Decimal, bool) {
	base := input.Portfolio.Household.BaseCurrency
	if item.NativeCurrency == base {
		return decimal.NewFromInt(1), true
	}
	if item.FXQuoteID != nil {
		for _, quote := range input.FXQuotes {
			if quote.ID.String() == *item.FXQuoteID {
				if quote.BaseCurrency == item.NativeCurrency && quote.QuoteCurrency == base {
					return quote.Rate.Decimal(), true
				}
				if quote.BaseCurrency == base && quote.QuoteCurrency == item.NativeCurrency {
					return decimal.NewFromInt(1).Div(quote.Rate.Decimal()), true
				}
				return decimal.Zero, false
			}
		}
		return decimal.Zero, false
	}
	return fxRateAt(input.FXQuotes, input.Portfolio.FXPreferences, input.Portfolio.Household.ID, item.NativeCurrency, base, cutoff)
}

func computeCashBridge(component domain.ComponentID, previous, current domain.DailyValuationSnapshotItem, previousCutoff, currentCutoff time.Time, effects []classifiedAnalysisEffect, input AnalysisInputs, query domain.AnalysisQuery, universe analysisUniverse) (holdingBridge, error) {
	if query.Valuation == domain.ValuationNative {
		return holdingBridge{}, nil
	}
	base := input.Portfolio.Household.BaseCurrency
	// Cash FX is a path calculation.  Keep each recorded cash movement at its
	// event FX; otherwise an omitted/incorrect movement can be silently
	// absorbed by the ending-value difference and leave no residual.
	if previous.NativeAmount == "" || current.NativeAmount == "" {
		return holdingBridge{partial: true}, nil
	}
	openingNative, err := decimal.NewFromString(previous.NativeAmount)
	if err != nil {
		return holdingBridge{}, err
	}
	if account, ok := universe.accounts[component.AccountID]; ok && account.IsLiability() {
		openingNative = openingNative.Neg()
	}
	openFX, openOK := decimal.NewFromInt(1), true
	if previous.NativeCurrency != base && !openingNative.IsZero() {
		openFX, openOK = universe.fxForItem(previous, input, previousCutoff)
	}
	closeFX, closeOK := decimal.NewFromInt(1), true
	if current.NativeCurrency != base {
		closeFX, closeOK = universe.fxForItem(current, input, currentCutoff)
	}
	if !closeOK || (!openOK && !openingNative.IsZero()) {
		return holdingBridge{partial: true}, nil
	}
	// The opening balance is translated from Xopen to Xc.  The expression is
	// written separately to make the architecture contract explicit.
	fxImpact := openingNative.Mul(closeFX.Sub(openFX))
	partial := false
	for _, effect := range effects {
		if !effect.amountKnown || effect.effect.Money == nil {
			if effect.effect.Money != nil {
				partial = true
			}
			continue
		}
		if effect.effect.Target != domain.EffectTargetAccountCash && effect.effect.Target != domain.EffectTargetAccountValue {
			continue
		}
		movementNative := signedDirection(effect.effect.Direction, effect.effect.Money.Amount())
		if account, ok := universe.accounts[component.AccountID]; ok && account.IsLiability() && effect.effect.Target == domain.EffectTargetAccountValue {
			movementNative = movementNative.Neg()
		}
		var eventFX decimal.Decimal
		var eventOK bool
		if component.Currency == base {
			eventFX, eventOK = decimal.NewFromInt(1), true
		} else {
			eventFX, eventOK = fxRateAt(input.FXQuotes, input.Portfolio.FXPreferences, input.Portfolio.Household.ID, component.Currency, base, effect.activity.EffectiveAt)
		}
		if !eventOK {
			return holdingBridge{partial: true}, nil
		}
		fxImpact = fxImpact.Add(movementNative.Mul(closeFX.Sub(eventFX)))
	}
	return holdingBridge{fx: fxImpact, partial: partial}, nil
}

func computeHoldingBridge(component domain.ComponentID, previous, current domain.DailyValuationSnapshotItem, previousCutoff, currentCutoff time.Time, effects []classifiedAnalysisEffect, input AnalysisInputs, query domain.AnalysisQuery, universe analysisUniverse) (holdingBridge, error) {
	if previous.NativeAmount == "" || current.NativeAmount == "" {
		return holdingBridge{partial: true}, nil
	}
	openQuote := universe.quoteForItem(previous, input.InstrumentQuotes, previousCutoff)
	closeQuote := universe.quoteForItem(current, input.InstrumentQuotes, currentCutoff)
	if closeQuote == nil && current.NativeAmount != "0" {
		return holdingBridge{partial: true}, nil
	}
	if closeQuote == nil {
		return holdingBridge{}, nil
	}
	opening, err := decimal.NewFromString(previous.NativeAmount)
	if err != nil {
		return holdingBridge{}, err
	}
	closing, err := decimal.NewFromString(current.NativeAmount)
	if err != nil {
		return holdingBridge{}, err
	}
	q0 := decimal.Zero
	if openQuote != nil {
		q0 = opening.Div(openQuote.UnitPrice.Decimal())
	} else if !opening.IsZero() {
		return holdingBridge{partial: true}, nil
	}
	qc := closing.Div(closeQuote.UnitPrice.Decimal())
	type segment struct{ quantity, price, fx decimal.Decimal }
	segments := make([]segment, 0, 1+len(effects))
	openFX, openOK := decimal.NewFromInt(1), true
	closeFX, closeOK := decimal.NewFromInt(1), true
	if query.Valuation == domain.ValuationBase && !q0.IsZero() {
		openFX, openOK = universe.fxForItem(previous, input, previousCutoff)
		closeFX, closeOK = universe.fxForItem(current, input, currentCutoff)
	} else if query.Valuation == domain.ValuationBase {
		closeFX, closeOK = universe.fxForItem(current, input, currentCutoff)
	}
	if !openOK || !closeOK {
		return holdingBridge{partial: true}, nil
	}
	if !q0.IsZero() {
		segments = append(segments, segment{quantity: q0, price: openQuote.UnitPrice.Decimal(), fx: openFX})
	}
	expected := q0
	transferredOut := decimal.Zero
	for _, effect := range effects {
		if effect.activity.TradeDetail != nil && effect.activity.TradeDetail.HoldingID == *component.HoldingID {
			quantity := effect.activity.TradeDetail.Quantity.Decimal()
			if effect.activity.TradeDetail.Side == domain.TradeSell {
				quantity = quantity.Neg()
			}
			eventFX := decimal.NewFromInt(1)
			eventOK := true
			if query.Valuation == domain.ValuationBase {
				if effect.activity.TradeDetail.Gross.Currency() == input.Portfolio.Household.BaseCurrency {
					eventFX = decimal.NewFromInt(1)
					eventOK = true
				} else {
					eventFX, eventOK = fxRateAt(input.FXQuotes, input.Portfolio.FXPreferences, input.Portfolio.Household.ID, effect.activity.TradeDetail.Gross.Currency(), input.Portfolio.Household.BaseCurrency, effect.activity.EffectiveAt)
				}
			}
			if !eventOK {
				return holdingBridge{partial: true}, nil
			}
			segments = append(segments, segment{quantity: quantity, price: effect.activity.TradeDetail.UnitPrice.Decimal(), fx: eventFX})
			expected = expected.Add(quantity)
		}
		if effect.activity.Kind == domain.ActivityPositionTransfer && effect.effect.Quantity != nil && effect.effect.HoldingID != nil && *effect.effect.HoldingID == *component.HoldingID {
			if effect.effect.Direction == domain.EffectAdded {
				expected = expected.Add(effect.effect.Quantity.Decimal())
			} else if effect.effect.Classification == domain.ClassificationInternalTransfer {
				expected = expected.Sub(effect.effect.Quantity.Decimal())
				transferredOut = transferredOut.Add(effect.effect.Quantity.Decimal())
			} else {
				expected = expected.Sub(effect.effect.Quantity.Decimal())
			}
		}
	}
	quantityMismatch := expected.Sub(qc).Abs().GreaterThan(decimal.NewFromFloat(1e-8))
	// Position adjustments carry quantity evidence but no price.  Treat the
	// adjustment as a corporate-action restatement only when the native
	// notional proves that quantity and price moved inversely.  Otherwise the
	// unexplained amount remains a visible residual instead of fabricating a
	// zero-valued acquisition or price change.
	if openQuote != nil && corporateActionRestatement(component, effects, q0, qc, openQuote.UnitPrice.Decimal(), closeQuote.UnitPrice.Decimal()) && len(segments) == 1 {
		ratio := qc.Div(q0)
		segments[0].quantity = qc
		segments[0].price = openQuote.UnitPrice.Decimal().Div(ratio)
	}
	if transferredOut.Sign() > 0 && len(segments) == 1 {
		segments[0].quantity = segments[0].quantity.Sub(transferredOut)
		if segments[0].quantity.IsNegative() {
			return holdingBridge{partial: true}, nil
		}
	}
	price, fx := decimal.Zero, decimal.Zero
	for _, item := range segments {
		price = price.Add(item.quantity.Mul(closeQuote.UnitPrice.Decimal().Sub(item.price)).Mul(item.fx))
		if query.Valuation == domain.ValuationBase {
			fx = fx.Add(item.quantity.Mul(item.price).Mul(closeFX.Sub(item.fx)))
			fx = fx.Add(item.quantity.Mul(closeQuote.UnitPrice.Decimal().Sub(item.price)).Mul(closeFX.Sub(item.fx)))
		}
	}
	return holdingBridge{price: price, fx: fx, partial: quantityMismatch}, nil
}

func corporateActionRestatement(component domain.ComponentID, effects []classifiedAnalysisEffect, openingQuantity, closingQuantity, openingPrice, closingPrice decimal.Decimal) bool {
	if openingQuantity.IsZero() || closingQuantity.IsZero() || openingPrice.IsZero() || closingPrice.IsZero() || openingQuantity.Equal(closingQuantity) {
		return false
	}
	quantityDelta := decimal.Zero
	seen := false
	for _, effect := range effects {
		if effect.activity.TradeDetail != nil || effect.activity.Kind != domain.ActivityPositionTransfer || effect.effect.HoldingID == nil || *effect.effect.HoldingID != *component.HoldingID || effect.effect.Classification != domain.ClassificationRemeasurement || effect.effect.CostUnitPrice != nil || effect.effect.Quantity == nil {
			continue
		}
		seen = true
		quantity := effect.effect.Quantity.Decimal()
		if effect.effect.Direction == domain.EffectRemoved {
			quantity = quantity.Neg()
		}
		quantityDelta = quantityDelta.Add(quantity)
	}
	if !seen || !quantityDelta.Equal(closingQuantity.Sub(openingQuantity)) {
		return false
	}
	// The two notional values must agree to the same tolerance used for
	// holding quantities.  This accepts decimal quote precision without
	// mistaking a manual value change for a split.
	openingNotional := openingQuantity.Mul(openingPrice)
	closingNotional := closingQuantity.Mul(closingPrice)
	tolerance := decimal.NewFromFloat(1e-8).Mul(decimal.Max(openingNotional.Abs(), closingNotional.Abs()).Add(decimal.NewFromInt(1)))
	return openingNotional.Sub(closingNotional).Abs().LessThanOrEqual(tolerance)
}

func inferredQuantity(item domain.DailyValuationSnapshotItem, quote *domain.InstrumentQuote) (decimal.Decimal, error) {
	if item.NativeAmount == "0" {
		return decimal.Zero, nil
	}
	if quote == nil {
		return decimal.Zero, fmt.Errorf("holding quote is unavailable")
	}
	value, err := decimal.NewFromString(item.NativeAmount)
	if err != nil {
		return decimal.Zero, err
	}
	return value.Div(quote.UnitPrice.Decimal()), nil
}
