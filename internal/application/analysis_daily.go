package application

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type holdingBridge struct {
	price      decimal.Decimal
	fx         decimal.Decimal
	exactPrice *big.Rat
	exactFX    *big.Rat
	partial    bool
}

func computeAnalysis(input AnalysisInputs, query domain.AnalysisQuery) (domain.PeriodAnalysisResult, error) {
	return computeAnalysisContext(context.Background(), input, query)
}

func computeAnalysisContext(ctx context.Context, input AnalysisInputs, query domain.AnalysisQuery) (domain.PeriodAnalysisResult, error) {
	return computeAnalysisForValuationUniverseContext(ctx, input, query, false)
}

func computeAnalysisForValuationUniverse(input AnalysisInputs, query domain.AnalysisQuery, investmentOnly bool) (domain.PeriodAnalysisResult, error) {
	return computeAnalysisForValuationUniverseContext(context.Background(), input, query, investmentOnly)
}

func computeAnalysisForValuationUniverseContext(ctx context.Context, input AnalysisInputs, query domain.AnalysisQuery, investmentOnly bool) (domain.PeriodAnalysisResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	if err := query.Validate(); err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	start, _, totalDays, err := analysisDateRange(query.From, query.To)
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	if input.Portfolio.Household == nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrNotFound, Message: "household was not found"}
	}
	if input.Origin.Timezone == "" {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Field: "timezone", Message: "analysis requires the History Origin timezone"}
	}
	location, err := time.LoadLocation(input.Origin.Timezone)
	if err != nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Field: "timezone", Message: "analysis requires a valid History Origin timezone"}
	}
	input.Snapshots, err = snapshotsInAnalysisWindow(input.Snapshots, query.From, query.To, location)
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	universe, err := resolveAnalysisUniverse(input, query)
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	universe.displayEvidence = newSnapshotDisplayEvidence(domain.HistoricalSnapshotBatch{Portfolio: input.Portfolio, InstrumentQuoteFacts: input.InstrumentQuotes, FXQuoteFacts: input.FXQuotes}, time.Time{}, 12*time.Hour)
	universe.displayEvidence.balances = make(map[balanceDisplayKey]balanceDisplayValue)
	valuationComponents := universe.context.Universe.Components
	if investmentOnly {
		valuationComponents = universe.context.InvestmentUniverse.Components
	}
	if query.Valuation == domain.ValuationNative && len(investmentCurrencies(valuationComponents)) > 1 {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrValidation, Field: "valuation", Message: "native valuation requires a single component currency"}
	}
	byDate := make(map[string]domain.DailyValuationSnapshot, len(input.Snapshots))
	for _, snapshot := range input.Snapshots {
		byDate[snapshot.LocalDate] = snapshot
	}
	result := domain.PeriodAnalysisResult{Query: query, AnalysisDayTimezone: input.Origin.Timezone, Days: make([]domain.ComponentDay, 0, totalDays*len(valuationComponents)), Coverage: domain.RateCoverage{TotalDays: totalDays}, Status: domain.CompletenessOK}
	activitiesByDate := make(map[string][]domain.Activity, len(input.Activities))
	for _, activity := range input.Activities {
		localDate := activityLocalDate(activity, input.Origin.Timezone)
		activitiesByDate[localDate] = append(activitiesByDate[localDate], activity)
	}
	previousItems := make(map[string]domain.DailyValuationSnapshotItem)
	firstPreviousLabel, err := analysisPreviousExistingCivilDateLabel(start, location)
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	firstPreviousDate := firstPreviousLabel.Format("2006-01-02")
	if previousSnapshot, ok := byDate[firstPreviousDate]; ok {
		previousItems = snapshotItemsByComponent(previousSnapshot, universe.accounts, universe.instruments)
	}
	for dayIndex := 0; dayIndex < totalDays; dayIndex++ {
		if err := ctx.Err(); err != nil {
			return domain.PeriodAnalysisResult{}, err
		}
		date := start.AddDate(0, 0, dayIndex)
		localDate := date.Format("2006-01-02")
		daySnapshot, hasDay := byDate[localDate]
		_, _, civilDateExists, boundsErr := analysisCivilDayBounds(date, location)
		if boundsErr != nil {
			return domain.PeriodAnalysisResult{}, boundsErr
		}
		hasDay = hasDay && civilDateExists
		if !hasDay {
			daySnapshot = domain.DailyValuationSnapshot{}
		}
		previousLabel, previousLabelErr := analysisPreviousExistingCivilDateLabel(date, location)
		if previousLabelErr != nil {
			return domain.PeriodAnalysisResult{}, previousLabelErr
		}
		previousDate := previousLabel.Format("2006-01-02")
		previousSnapshot, hasPrevious := byDate[previousDate]
		var previous *domain.DailyValuationSnapshot
		if hasPrevious {
			previous = &previousSnapshot
		}
		endItems := snapshotItemsByComponent(daySnapshot, universe.accounts, universe.instruments)
		var effects []classifiedAnalysisEffect
		if hasDay {
			for _, activity := range activitiesByDate[localDate] {
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
		for _, component := range valuationComponents {
			key := component.Key()
			day, err := buildComponentDay(localDate, component, previousItems[key], endItems[key], previousSnapshot, daySnapshot, hasPrevious, hasDay, effectsByComponent[key], input, query, universe)
			if err != nil {
				return domain.PeriodAnalysisResult{}, err
			}
			if day.Status == domain.CompletenessOK && (universe.hasUnknownAccount(localDate) || universe.hasUnknownAccount(previousDate)) {
				day.Status = domain.CompletenessPartial
			}
			if universe.hasUnknownAccount(localDate) && day.DisplayEnding != nil {
				day.DisplayEnding.Total = nil
				day.DisplayEnding.Quality.Availability = domain.ValuationPartial
				day.DisplayEnding.Quality.Coverage = domain.CoverageProvisional
				day.DisplayEnding.Quality.Reasons = append(day.DisplayEnding.Quality.Reasons, domain.ReasonEvidenceUnavailable)
			}
			result.Days = append(result.Days, day)
		}
		// A skipped label has no observation and must not break continuity between
		// its surrounding existing days. A missing observation on an existing
		// day still clears the baseline so analysis never bridges a genuine gap.
		if civilDateExists {
			previousItems = endItems
		}
	}
	if err := finalizeAnalysisReturns(ctx, &result, universe, input, query); err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	return result, nil
}

func investmentCurrencies(components domain.Components) map[domain.CurrencyCode]struct{} {
	currencies := make(map[domain.CurrencyCode]struct{})
	for _, component := range components {
		if component.Currency != "" {
			currencies[component.Currency] = struct{}{}
		}
	}
	return currencies
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

func buildComponentDay(date string, component domain.ComponentID, previous, current domain.DailyValuationSnapshotItem, previousSnapshot, currentSnapshot domain.DailyValuationSnapshot, hasPrevious, hasCurrent bool, effects []classifiedAnalysisEffect, input AnalysisInputs, query domain.AnalysisQuery, universe analysisUniverse) (day domain.ComponentDay, err error) {
	day = domain.ComponentDay{Date: domain.LocalDate(date), Component: component, AssetBuckets: make(map[domain.AttributionBucket]domain.SignedMoney), AssetBucketExact: make(map[domain.AttributionBucket]decimal.Decimal), ReturnComponents: make(map[domain.ReturnComponent]domain.SignedMoney), DietzCapitalFlows: make([]domain.DietzCapitalFlow, 0), AttributedEffects: make([]domain.AttributedEffect, 0), Status: domain.CompletenessOK}
	defer func() {
		if err != nil {
			return
		}
		applyFundingGapMark(&day, previous, current)
	}()
	account := universe.accounts[component.AccountID]
	// A complete snapshot before an account existed legitimately omits it.
	// Existing rows always win, including backdated activity and missing values.
	if hasPrevious && previousSnapshot.Complete && previous.AccountID == "" && accountAbsentBeforeCreation(account, previousSnapshot.CutoffAt, input) {
		previous = zeroValuationItem(component, current)
	}
	if hasCurrent && currentSnapshot.Complete && current.AccountID == "" && accountAbsentBeforeCreation(account, currentSnapshot.CutoffAt, input) {
		current = zeroValuationItem(component, previous)
	}
	// A trade can create a holding component, close it, or create a foreign
	// cash sleeve on the same day. The snapshot legitimately omits a zero
	// balance component at one side of that boundary, but dropping the whole
	// day would also drop the known trade leg and make the household Asset
	// Changes waterfall fail to reconcile. An activity is the only safe proof
	// that the missing side is an opening zero; without one, keep the day
	// unavailable so missing valuation data is never guessed as zero.
	openingKnown := previousSnapshot.Complete && currentSnapshot.Complete
	if !component.Cash && snapshotFundingGapAnnotated(previousSnapshot) && snapshotFundingGapAnnotated(currentSnapshot) {
		openingKnown = true
	}
	if (len(effects) > 0 || componentHasActivityInRange(component, input.Activities, query.From, query.To)) && openingKnown {
		previousMissing := previous.AccountID == ""
		currentMissing := current.AccountID == ""
		if previousMissing && !currentMissing && current.Complete {
			previous = zeroValuationItem(component, current)
			hasPrevious = true
		}
		if currentMissing && !previousMissing && previous.Complete {
			current = zeroValuationItem(component, previous)
			hasCurrent = true
		}
		if previousMissing && currentMissing {
			previous = zeroValuationItem(component, domain.DailyValuationSnapshotItem{})
			current = zeroValuationItem(component, previous)
			hasPrevious, hasCurrent = true, true
		}
	}
	// Independent level projection precedes the unchanged strict-return guard.
	display, displayExact, displayErr := analysisDisplayEnding(current, hasCurrent, account, input, query, universe.displayEvidence, date)
	if displayErr != nil {
		return domain.ComponentDay{}, displayErr
	}
	day.DisplayEnding = &display
	day.DisplayEndingExact = displayExact
	day.DisplayLiability = account.IsLiability()
	if !hasPrevious || !hasCurrent || !previous.Complete || !current.Complete {
		day.Status = domain.CompletenessUnavailable
		if hasPrevious && hasCurrent && (previous.Complete || current.Complete) {
			day.Status = domain.CompletenessPartial
		}
		if fundingGapItemReason(previous, current) != "" && current.Complete && input.Portfolio.Household != nil {
			ending, endOK, endErr := snapshotItemValue(current, account, query.Valuation, input.Portfolio.Household.BaseCurrency)
			if endErr != nil {
				return domain.ComponentDay{}, endErr
			}
			if endOK {
				signed, signErr := newSigned(ending, valueCurrency(current, query.Valuation, input.Portfolio.Household.BaseCurrency))
				if signErr != nil {
					return domain.ComponentDay{}, signErr
				}
				day.EndingValue = signed
				day.Status = domain.CompletenessPartial
			}
		}
		return day, nil
	}
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
	day.EndingValue, err = newSigned(ending, valueCurrency(current, query.Valuation, baseCurrency))
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
				addExactBucket(day.AssetBucketExact, *effect.bucket, effect.amount)
				directTotal = directTotal.Add(effect.amount)
			} else if effect.neutral {
				neutralTotal = neutralTotal.Add(effect.amount)
			}
		}
		if effect.returnComponent != nil && effect.returnKnown {
			if err := addReturn(day.ReturnComponents, *effect.returnComponent, effect.returnAmount, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
				return domain.ComponentDay{}, err
			}
		}
		var flow domain.SignedMoney
		if effect.dietzKnown {
			var flowErr error
			flow, flowErr = newSigned(effect.dietzAmount, valueCurrency(current, query.Valuation, baseCurrency))
			if flowErr != nil {
				return domain.ComponentDay{}, flowErr
			}
			day.DietzCapitalFlows = append(day.DietzCapitalFlows, domain.DietzCapitalFlow{Amount: flow, EffectiveAt: effect.activity.EffectiveAt})
			weighted := effect.dietzAmount.Mul(analysisDayFlowWeight(date, input.Origin.Timezone, effect.activity.EffectiveAt))
			weightedFlow, flowErr := newSigned(weighted, valueCurrency(current, query.Valuation, baseCurrency))
			if flowErr != nil {
				return domain.ComponentDay{}, flowErr
			}
			day.DietzFlow, flowErr = addSignedValues(day.DietzFlow, weightedFlow, valueCurrency(current, query.Valuation, baseCurrency))
			if flowErr != nil {
				return domain.ComponentDay{}, flowErr
			}
		}
		var potentialDietzFlow *domain.DietzCapitalFlow
		if effect.potentialDietzKnown && !effect.dietzKnown {
			potentialMoney, potentialErr := newSigned(effect.potentialDietzAmount, valueCurrency(current, query.Valuation, baseCurrency))
			if potentialErr != nil {
				return domain.ComponentDay{}, potentialErr
			}
			potentialDietzFlow = &domain.DietzCapitalFlow{Amount: potentialMoney, EffectiveAt: effect.activity.EffectiveAt}
		}
		if effect.bucket != nil || effect.returnComponent != nil || effect.dietzKnown || potentialDietzFlow != nil {
			projectionAmount := effect.amount
			if effect.returnComponent != nil && effect.returnKnown {
				projectionAmount = effect.returnAmount
			}
			attributedAmount, amountErr := newSigned(projectionAmount, valueCurrency(current, query.Valuation, baseCurrency))
			if amountErr != nil {
				return domain.ComponentDay{}, amountErr
			}
			var dietzFlow *domain.DietzCapitalFlow
			if effect.dietzKnown {
				flowCopy := domain.DietzCapitalFlow{Amount: flow, EffectiveAt: effect.activity.EffectiveAt}
				dietzFlow = &flowCopy
			}
			day.AttributedEffects = append(day.AttributedEffects, domain.AttributedEffect{
				AssetBucket: effect.bucket, ReturnComponent: effect.returnComponent,
				DietzCapitalFlow: dietzFlow, PotentialDietzCapitalFlow: potentialDietzFlow,
				Amount: attributedAmount, SourceEffect: effect.effect, Component: effect.component,
				RelatedHoldingID: effect.relatedHolding, RelatedInstrumentID: effect.relatedInstrument,
			})
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
		priceAmount := bridge.price
		var attributedAmount domain.SignedMoney
		if bridge.exactPrice != nil {
			priceAmount = decimalAtMoneyBoundary(bridge.exactPrice)
			attributedAmount, err = newSignedRational(bridge.exactPrice, valueCurrency(current, query.Valuation, baseCurrency))
		} else {
			bridgePriceExact := rationalFromDecimal(bridge.price)
			priceAmount = decimalAtMoneyBoundary(bridgePriceExact)
			attributedAmount, err = newSignedRational(bridgePriceExact, valueCurrency(current, query.Valuation, baseCurrency))
		}
		if err != nil {
			return domain.ComponentDay{}, err
		}
		if err := addBucket(day.AssetBuckets, domain.BucketPriceChange, priceAmount, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
			return domain.ComponentDay{}, err
		}
		addExactBucket(day.AssetBucketExact, domain.BucketPriceChange, bridge.price)
		if universe.investmentComponentInUniverse(component) {
			if err := addReturn(day.ReturnComponents, domain.ReturnPriceChange, priceAmount, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
				return domain.ComponentDay{}, err
			}
		}
		priceBucket := domain.BucketPriceChange
		var priceReturn *domain.ReturnComponent
		if universe.investmentComponentInUniverse(component) {
			value := domain.ReturnPriceChange
			priceReturn = &value
		}
		day.AttributedEffects = append(day.AttributedEffects, domain.AttributedEffect{AssetBucket: &priceBucket, ReturnComponent: priceReturn, Amount: attributedAmount, Component: component})
	}
	if bridge.fx.Sign() != 0 {
		fxAmount := bridge.fx
		var attributedAmount domain.SignedMoney
		if bridge.exactFX != nil {
			fxAmount = decimalAtMoneyBoundary(bridge.exactFX)
			attributedAmount, err = newSignedRational(bridge.exactFX, valueCurrency(current, query.Valuation, baseCurrency))
		} else {
			bridgeFXExact := rationalFromDecimal(bridge.fx)
			fxAmount = decimalAtMoneyBoundary(bridgeFXExact)
			attributedAmount, err = newSignedRational(bridgeFXExact, valueCurrency(current, query.Valuation, baseCurrency))
		}
		if err != nil {
			return domain.ComponentDay{}, err
		}
		if err := addBucket(day.AssetBuckets, domain.BucketFXImpact, fxAmount, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
			return domain.ComponentDay{}, err
		}
		addExactBucket(day.AssetBucketExact, domain.BucketFXImpact, bridge.fx)
		if universe.investmentComponentInUniverse(component) {
			if err := addReturn(day.ReturnComponents, domain.ReturnFXImpact, fxAmount, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
				return domain.ComponentDay{}, err
			}
		}
		fxBucket := domain.BucketFXImpact
		var fxReturn *domain.ReturnComponent
		if universe.investmentComponentInUniverse(component) {
			value := domain.ReturnFXImpact
			fxReturn = &value
		}
		day.AttributedEffects = append(day.AttributedEffects, domain.AttributedEffect{AssetBucket: &fxBucket, ReturnComponent: fxReturn, Amount: attributedAmount, Component: component})
	}
	knownInternal := neutralTotal
	currency := valueCurrency(current, query.Valuation, baseCurrency)
	tolerance := residualTolerance(currency, begin)
	var unexplained decimal.Decimal
	var residualMoney domain.SignedMoney
	var hasResidual bool
	if bridge.exactPrice == nil && bridge.exactFX == nil {
		// Components without an exact FX bridge (notably base-currency cash)
		// already have finite decimal drivers. Keep their high-volume residual
		// reconciliation on the decimal path instead of constructing and
		// reducing exact Rat values that cannot improve the result.
		unexplained = delta.Sub(directTotal).Sub(knownInternal).Sub(bridge.price).Sub(bridge.fx)
		residualMoney, err = newSigned(unexplained, currency)
		if err != nil {
			return domain.ComponentDay{}, err
		}
		hasResidual = unexplained.Abs().GreaterThan(tolerance)
	} else {
		bridgePriceExact := bridge.exactPrice
		if bridgePriceExact == nil {
			bridgePriceExact = rationalFromDecimal(bridge.price)
		}
		bridgeFXExact := bridge.exactFX
		if bridgeFXExact == nil {
			bridgeFXExact = rationalFromDecimal(bridge.fx)
		}
		unexplainedExact := rationalFromDecimal(delta)
		unexplainedExact.Sub(unexplainedExact, rationalFromDecimal(directTotal))
		unexplainedExact.Sub(unexplainedExact, rationalFromDecimal(knownInternal))
		unexplainedExact.Sub(unexplainedExact, bridgePriceExact)
		unexplainedExact.Sub(unexplainedExact, bridgeFXExact)
		var projectionErr error
		unexplained, projectionErr = analysisRationalDecimal(unexplainedExact)
		if projectionErr != nil {
			return domain.ComponentDay{}, projectionErr
		}
		residualMoney, err = newSignedRational(unexplainedExact, currency)
		if err != nil {
			return domain.ComponentDay{}, err
		}
		hasResidual = new(big.Rat).Abs(new(big.Rat).Set(unexplainedExact)).Cmp(rationalFromDecimal(tolerance)) > 0
	}
	if hasResidual {
		day.Status = domain.CompletenessPartial
		bucket := domain.BucketResidual
		if err := addBucket(day.AssetBuckets, bucket, residualMoney.Amount(), valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
			return domain.ComponentDay{}, err
		}
		addExactBucket(day.AssetBucketExact, bucket, unexplained)
		day.Residual = &domain.Residual{Amount: residualMoney, Tolerance: tolerance, Visible: true}
	} else if bridge.partial {
		day.Status = domain.CompletenessPartial
	}
	// Asset Changes is defined over the whole resolved universe, while Return
	// Analysis is defined over its independent InvestmentUniverse. Keep return
	// fields absent on excluded cash/liability components so a later group fold
	// cannot accidentally put them back into a return denominator.
	if !universe.investmentComponentInUniverse(component) {
		return day, nil
	}
	total, sumErr := sumReturnComponents(day.ReturnComponents)
	if sumErr != nil {
		return domain.ComponentDay{}, sumErr
	}
	returnMoney, returnErr := newSigned(total.Amount(), valueCurrency(current, query.Valuation, baseCurrency))
	if returnErr != nil {
		return domain.ComponentDay{}, returnErr
	}
	day.ReturnAmount = &returnMoney
	if invested, investedErr := addSignedValues(day.BeginningValue, day.DietzFlow, valueCurrency(current, query.Valuation, baseCurrency)); investedErr == nil {
		day.InvestedCapital = &invested
		if day.Status == domain.CompletenessOK && invested.Amount().IsPositive() && day.ReturnAmount != nil {
			rate := day.ReturnAmount.Amount().Div(invested.Amount())
			day.ReturnRate = &rate
		}
	}
	return day, nil
}

func componentHasActivityInRange(component domain.ComponentID, activities []domain.Activity, from, to domain.LocalDate) bool {
	for _, activity := range activities {
		if activity.EffectiveLocalDate < from || activity.EffectiveLocalDate > to {
			continue
		}
		for _, effect := range activity.Effects {
			if componentEffectMatches(component, effect) {
				return true
			}
		}
	}
	return false
}

func accountAbsentBeforeCreation(account domain.Account, cutoff time.Time, input AnalysisInputs) bool {
	if cutoff.IsZero() || account.CreatedAt.IsZero() || !account.CreatedAt.After(cutoff) {
		return false
	}
	for _, snapshot := range input.Snapshots {
		if snapshot.CutoffAt.After(cutoff) {
			continue
		}
		for _, item := range snapshot.Items {
			if item.AccountID == account.ID {
				return false
			}
		}
	}
	for _, activity := range input.Activities {
		if activity.EffectiveAt.After(cutoff) {
			continue
		}
		for _, effect := range activity.Effects {
			if effect.AccountID != nil && *effect.AccountID == account.ID {
				return false
			}
			if effect.HoldingID != nil {
				for _, holding := range input.Portfolio.Holdings {
					if holding.ID == *effect.HoldingID && holding.AccountID == account.ID {
						return false
					}
				}
			}
		}
	}
	return true
}

func componentEffectMatches(component domain.ComponentID, effect domain.ActivityEffect) bool {
	if component.HoldingID != nil {
		return effect.HoldingID != nil && *effect.HoldingID == *component.HoldingID
	}
	if component.Cash && effect.HoldingID == nil && effect.AccountID != nil {
		return *effect.AccountID == component.AccountID
	}
	return effect.AccountID != nil && *effect.AccountID == component.AccountID
}

func zeroValuationItem(component domain.ComponentID, template domain.DailyValuationSnapshotItem) domain.DailyValuationSnapshotItem {
	item := domain.DailyValuationSnapshotItem{
		AccountID:       component.AccountID,
		HoldingID:       component.HoldingID,
		InstrumentID:    component.InstrumentID,
		NativeAmount:    "0",
		NativeCurrency:  component.Currency,
		BaseAmountExact: "0",
		Complete:        true,
	}
	if item.NativeCurrency == "" {
		item.NativeCurrency = template.NativeCurrency
	}
	return item
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

func newSignedRational(value *big.Rat, currency domain.CurrencyCode) (domain.SignedMoney, error) {
	if err := domain.CheckExactMoneyRange(value); err != nil {
		return domain.SignedMoney{}, err
	}
	return domain.NewSignedMoney(decimalAtMoneyBoundary(value), currency)
}

func analysisRationalDecimal(value *big.Rat) (decimal.Decimal, error) {
	return domain.ExactFXIntermediateDecimal(value)
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

func addExactBucket(buckets map[domain.AttributionBucket]decimal.Decimal, bucket domain.AttributionBucket, amount decimal.Decimal) {
	if buckets == nil || amount.IsZero() {
		return
	}
	buckets[bucket] = buckets[bucket].Add(amount)
}

func addReturn(components map[domain.ReturnComponent]domain.SignedMoney, component domain.ReturnComponent, amount decimal.Decimal, currency domain.CurrencyCode) error {
	if amount.IsZero() {
		return nil
	}
	current := decimal.Zero
	if existing, ok := components[component]; ok {
		current = existing.Amount()
	}
	value, err := newSigned(current.Add(amount), currency)
	if err != nil {
		return err
	}
	components[component] = value
	return nil
}

func addSignedValues(left, right domain.SignedMoney, currency domain.CurrencyCode) (domain.SignedMoney, error) {
	return newSigned(left.Amount().Add(right.Amount()), currency)
}

func sumReturnComponents(components map[domain.ReturnComponent]domain.SignedMoney) (domain.SignedMoney, error) {
	if len(components) == 0 {
		return domain.SignedMoney{}, nil
	}
	var currency domain.CurrencyCode
	total := decimal.Zero
	for _, value := range components {
		currency = value.Currency()
		total = total.Add(value.Amount())
	}
	return newSigned(total, currency)
}

func residualTolerance(currency domain.CurrencyCode, beginning decimal.Decimal) decimal.Decimal {
	places := domain.CurrencyFractionDigits(currency)
	floor := decimal.New(2, int32(-places))
	relative := beginning.Abs().Mul(decimal.New(1, -9))
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
	quote := fxQuoteAt(input.FXQuotes, input.Portfolio.FXPreferences, input.Portfolio.Household.ID, currency, base, at)
	if quote == nil {
		return decimal.Zero, &domain.Error{Code: domain.ErrUnavailable, Field: "fxRate", Message: "an FX rate is unavailable for the activity"}
	}
	return domain.ConvertFXAmount(value, currency, base, *quote)
}

func fxQuoteAt(quotes []domain.FXQuote, preferences []domain.FXPreference, householdID domain.HouseholdID, native, base domain.CurrencyCode, cutoff time.Time) *domain.FXQuote {
	preferred := domain.QuoteSourceKind("")
	for _, preference := range preferences {
		if preference.HouseholdID == householdID && ((preference.CurrencyA == native && preference.CurrencyB == base) || (preference.CurrencyA == base && preference.CurrencyB == native)) {
			preferred = preference.SourceKind
			break
		}
	}
	var selection currentQuoteSelection[*domain.FXQuote]
	for index := range quotes {
		quote := &quotes[index]
		if quote.HouseholdID != householdID || quote.QuotedAt.After(cutoff) || (preferred != "" && !quoteSourceAllowed(preferred, quote.SourceKind)) {
			continue
		}
		if !((quote.BaseCurrency == native && quote.QuoteCurrency == base) || (quote.BaseCurrency == base && quote.QuoteCurrency == native)) {
			continue
		}
		selection.add(quote, fxQuoteOrder(*quote))
	}
	selected := selection.selected()
	return selected
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
	var selection currentQuoteSelection[*domain.InstrumentQuote]
	for index := range quotes {
		quote := &quotes[index]
		if quote.InstrumentID != *item.InstrumentID || quote.QuotedAt.After(cutoff) {
			continue
		}
		selection.add(quote, instrumentQuoteOrder(*quote))
	}
	return selection.selected()
}

func (u analysisUniverse) fxQuoteForItem(item domain.DailyValuationSnapshotItem, input AnalysisInputs, cutoff time.Time) (*domain.FXQuote, bool) {
	base := input.Portfolio.Household.BaseCurrency
	if item.NativeCurrency == base {
		return nil, true
	}
	if item.FXQuoteID != nil {
		for _, quote := range input.FXQuotes {
			if quote.ID.String() == *item.FXQuoteID {
				if quote.HouseholdID == input.Portfolio.Household.ID && ((quote.BaseCurrency == item.NativeCurrency && quote.QuoteCurrency == base) || (quote.BaseCurrency == base && quote.QuoteCurrency == item.NativeCurrency)) {
					return &quote, true
				}
				return nil, false
			}
		}
		return nil, false
	}
	quote := fxQuoteAt(input.FXQuotes, input.Portfolio.FXPreferences, input.Portfolio.Household.ID, item.NativeCurrency, base, cutoff)
	return quote, quote != nil
}

func convertAtFXQuote(amount decimal.Decimal, native, base domain.CurrencyCode, quote *domain.FXQuote) (*big.Rat, bool, error) {
	if native == base {
		return rationalFromDecimal(amount), true, nil
	}
	if quote == nil {
		return nil, false, nil
	}
	converted, err := domain.ConvertFXAmountExact(amount, native, base, *quote)
	return converted, err == nil, err
}

func convertForAnalysis(amount decimal.Decimal, native, base domain.CurrencyCode, quote *domain.FXQuote, valuation domain.Valuation) (*big.Rat, bool, error) {
	if valuation == domain.ValuationNative {
		return rationalFromDecimal(amount), true, nil
	}
	return convertAtFXQuote(amount, native, base, quote)
}

func projectedHoldingBridge(price, fx *big.Rat, partial bool) (holdingBridge, error) {
	priceProjection, err := analysisRationalDecimal(price)
	if err != nil {
		return holdingBridge{}, err
	}
	fxProjection, err := analysisRationalDecimal(fx)
	if err != nil {
		return holdingBridge{}, err
	}
	return holdingBridge{price: priceProjection, fx: fxProjection, exactPrice: price, exactFX: fx, partial: partial}, nil
}

func computeCashBridge(component domain.ComponentID, previous, current domain.DailyValuationSnapshotItem, previousCutoff, currentCutoff time.Time, effects []classifiedAnalysisEffect, input AnalysisInputs, query domain.AnalysisQuery, universe analysisUniverse) (holdingBridge, error) {
	if query.Valuation == domain.ValuationNative {
		return holdingBridge{}, nil
	}
	base := input.Portfolio.Household.BaseCurrency
	if previous.NativeCurrency == base && current.NativeCurrency == base {
		// A base-currency cash component cannot have an FX driver. Avoid
		// reparsing native amounts and looking up quotes on the hot path.
		return holdingBridge{}, nil
	}
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
	var openFXQuote *domain.FXQuote
	openOK := true
	if previous.NativeCurrency != base && !openingNative.IsZero() {
		openFXQuote, openOK = universe.fxQuoteForItem(previous, input, previousCutoff)
	}
	var closeFXQuote *domain.FXQuote
	closeOK := true
	if current.NativeCurrency != base {
		closeFXQuote, closeOK = universe.fxQuoteForItem(current, input, currentCutoff)
	}
	if !closeOK || (!openOK && !openingNative.IsZero()) {
		return holdingBridge{partial: true}, nil
	}
	openingAtClose, closeAvailable, err := convertAtFXQuote(openingNative, component.Currency, base, closeFXQuote)
	if err != nil {
		return holdingBridge{}, err
	}
	if !closeAvailable {
		return holdingBridge{partial: true}, nil
	}
	openingAtOpen := new(big.Rat)
	if openingNative.IsZero() {
		openingAtOpen.SetInt64(0)
	} else {
		convertedOpening, openAvailable, conversionErr := convertAtFXQuote(openingNative, component.Currency, base, openFXQuote)
		if conversionErr != nil {
			return holdingBridge{}, conversionErr
		}
		if !openAvailable {
			return holdingBridge{partial: true}, nil
		}
		openingAtOpen.Set(convertedOpening)
	}
	fxImpact := new(big.Rat).Sub(openingAtClose, openingAtOpen)
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
		var eventFXQuote *domain.FXQuote
		var eventOK bool
		if component.Currency == base {
			eventOK = true
		} else {
			eventFXQuote = fxQuoteAt(input.FXQuotes, input.Portfolio.FXPreferences, input.Portfolio.Household.ID, component.Currency, base, effect.activity.EffectiveAt)
			eventOK = eventFXQuote != nil
		}
		if !eventOK {
			return holdingBridge{partial: true}, nil
		}
		movementAtClose, closeAvailable, conversionErr := convertAtFXQuote(movementNative, component.Currency, base, closeFXQuote)
		if conversionErr != nil {
			return holdingBridge{}, conversionErr
		}
		movementAtEvent, eventAvailable, conversionErr := convertAtFXQuote(movementNative, component.Currency, base, eventFXQuote)
		if conversionErr != nil {
			return holdingBridge{}, conversionErr
		}
		if !closeAvailable || !eventAvailable {
			return holdingBridge{partial: true}, nil
		}
		fxImpact.Add(fxImpact, new(big.Rat).Sub(movementAtClose, movementAtEvent))
	}
	return projectedHoldingBridge(new(big.Rat), fxImpact, partial)
}

func computeHoldingBridge(component domain.ComponentID, previous, current domain.DailyValuationSnapshotItem, previousCutoff, currentCutoff time.Time, effects []classifiedAnalysisEffect, input AnalysisInputs, query domain.AnalysisQuery, universe analysisUniverse) (holdingBridge, error) {
	if previous.NativeAmount == "" || current.NativeAmount == "" {
		return holdingBridge{partial: true}, nil
	}
	opening, err := decimal.NewFromString(previous.NativeAmount)
	if err != nil {
		return holdingBridge{}, err
	}
	closing, err := decimal.NewFromString(current.NativeAmount)
	if err != nil {
		return holdingBridge{}, err
	}
	// Complete zero valuations without daily activity have no market exposure,
	// including days before the first purchase. They do not require a quote.
	// Keep active days on the normal path: a same-day buy/sell can start and end
	// at zero while still contributing a price return.
	if opening.IsZero() && closing.IsZero() && len(effects) == 0 {
		return holdingBridge{}, nil
	}
	openQuote := universe.quoteForItem(previous, input.InstrumentQuotes, previousCutoff)
	closeQuote := universe.quoteForItem(current, input.InstrumentQuotes, currentCutoff)
	if closeQuote == nil {
		// A closed position with no closing quote still has unaccounted price
		// movement between the opening mark and the disposal.
		return holdingBridge{partial: true}, nil
	}
	q0 := decimal.Zero
	if openQuote != nil {
		q0 = opening.Div(openQuote.UnitPrice.Decimal())
	} else if !opening.IsZero() {
		return holdingBridge{partial: true}, nil
	}
	qc := closing.Div(closeQuote.UnitPrice.Decimal())
	type segment struct {
		quantity, price decimal.Decimal
		fxQuote         *domain.FXQuote
	}
	segments := make([]segment, 0, 1+len(effects))
	seenTrades := make(map[domain.ActivityID]bool)
	var openFXQuote, closeFXQuote *domain.FXQuote
	openOK, closeOK := true, true
	if query.Valuation == domain.ValuationBase && !q0.IsZero() {
		openFXQuote, openOK = universe.fxQuoteForItem(previous, input, previousCutoff)
		closeFXQuote, closeOK = universe.fxQuoteForItem(current, input, currentCutoff)
	} else if query.Valuation == domain.ValuationBase {
		closeFXQuote, closeOK = universe.fxQuoteForItem(current, input, currentCutoff)
	}
	if !openOK || !closeOK {
		return holdingBridge{partial: true}, nil
	}
	if !q0.IsZero() {
		segments = append(segments, segment{quantity: q0, price: openQuote.UnitPrice.Decimal(), fxQuote: openFXQuote})
	}
	expected := q0
	transferredOut := decimal.Zero
	for _, effect := range effects {
		if effect.activity.TradeDetail != nil && effect.activity.TradeDetail.HoldingID == *component.HoldingID {
			// A trade fee may be projected onto this holding as a return-only
			// effect in addition to the physical quantity leg. The trade path is
			// an activity-level fact and must be walked once, not once per effect.
			if seenTrades[effect.activity.ID] {
				continue
			}
			seenTrades[effect.activity.ID] = true
			quantity := effect.activity.TradeDetail.Quantity.Decimal()
			if effect.activity.TradeDetail.Side == domain.TradeSell {
				quantity = quantity.Neg()
			}
			var eventFXQuote *domain.FXQuote
			eventOK := true
			if query.Valuation == domain.ValuationBase {
				if effect.activity.TradeDetail.Gross.Currency() != input.Portfolio.Household.BaseCurrency {
					eventFXQuote = fxQuoteAt(input.FXQuotes, input.Portfolio.FXPreferences, input.Portfolio.Household.ID, effect.activity.TradeDetail.Gross.Currency(), input.Portfolio.Household.BaseCurrency, effect.activity.EffectiveAt)
					eventOK = eventFXQuote != nil
				}
			}
			if !eventOK {
				return holdingBridge{partial: true}, nil
			}
			segments = append(segments, segment{quantity: quantity, price: effect.activity.TradeDetail.UnitPrice.Decimal(), fxQuote: eventFXQuote})
			expected = expected.Add(quantity)
		}
		if effect.activity.Kind == domain.ActivityPositionTransfer && effect.effect.Quantity != nil && effect.effect.HoldingID != nil && *effect.effect.HoldingID == *component.HoldingID {
			// A priced adjustment establishes a new valuation segment just
			// like a trade. Keep its notional in Adjustment and attribute only
			// the movement after that entry to price/FX returns.
			if effect.effect.Classification == domain.ClassificationRemeasurement && effect.effect.CostUnitPrice != nil {
				quantity := effect.effect.Quantity.Decimal()
				if effect.effect.Direction == domain.EffectRemoved {
					quantity = quantity.Neg()
				}
				var eventFXQuote *domain.FXQuote
				if query.Valuation == domain.ValuationBase && component.Currency != input.Portfolio.Household.BaseCurrency {
					eventFXQuote = fxQuoteAt(input.FXQuotes, input.Portfolio.FXPreferences, input.Portfolio.Household.ID, component.Currency, input.Portfolio.Household.BaseCurrency, effect.activity.EffectiveAt)
					if eventFXQuote == nil {
						return holdingBridge{partial: true}, nil
					}
				}
				segments = append(segments, segment{quantity: quantity, price: effect.effect.CostUnitPrice.Decimal(), fxQuote: eventFXQuote})
			}
			if effect.effect.Direction == domain.EffectAdded {
				expected = expected.Add(effect.effect.Quantity.Decimal())
			} else if effect.effect.Classification == domain.ClassificationInternalTransfer || (effect.effect.CostUnitPrice == nil && effect.bucket != nil && *effect.bucket == domain.BucketAdjustment && effect.amountKnown) {
				expected = expected.Sub(effect.effect.Quantity.Decimal())
				transferredOut = transferredOut.Add(effect.effect.Quantity.Decimal())
			} else {
				expected = expected.Sub(effect.effect.Quantity.Decimal())
			}
		}
	}
	quantityMismatch := expected.Sub(qc).Abs().GreaterThan(decimal.NewFromFloat(1e-8))
	// Unpriced position adjustments carry only quantity evidence. Treat the
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
	price, fx := new(big.Rat), new(big.Rat)
	for _, item := range segments {
		priceNative := item.quantity.Mul(closeQuote.UnitPrice.Decimal().Sub(item.price))
		priceBase, available, conversionErr := convertForAnalysis(priceNative, component.Currency, input.Portfolio.Household.BaseCurrency, item.fxQuote, query.Valuation)
		if conversionErr != nil {
			return holdingBridge{}, conversionErr
		}
		if !available {
			return holdingBridge{partial: true}, nil
		}
		price.Add(price, priceBase)
		if query.Valuation == domain.ValuationBase {
			closingNotional := item.quantity.Mul(closeQuote.UnitPrice.Decimal())
			closingAtClose, closeAvailable, conversionErr := convertAtFXQuote(closingNotional, component.Currency, input.Portfolio.Household.BaseCurrency, closeFXQuote)
			if conversionErr != nil {
				return holdingBridge{}, conversionErr
			}
			closingAtSegment, segmentAvailable, conversionErr := convertAtFXQuote(closingNotional, component.Currency, input.Portfolio.Household.BaseCurrency, item.fxQuote)
			if conversionErr != nil {
				return holdingBridge{}, conversionErr
			}
			if !closeAvailable || !segmentAvailable {
				return holdingBridge{partial: true}, nil
			}
			fx.Add(fx, new(big.Rat).Sub(closingAtClose, closingAtSegment))
		}
	}
	return projectedHoldingBridge(price, fx, quantityMismatch)
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

func analysisDisplayEnding(item domain.DailyValuationSnapshotItem, present bool, account domain.Account, input AnalysisInputs, query domain.AnalysisQuery, e snapshotDisplayEvidence, date string) (domain.ValuationAmount, *decimal.Decimal, error) {
	// Unquoted complete balances have no date-dependent market quality. Reuse
	// their immutable projection within this capture instead of parsing, rounding
	// and allocating the same value for every component/day. Quoted and
	// provisional values retain the normal selected-evidence path.
	cacheable := present && item.AccountID != "" && item.Complete && item.QuoteID == nil && item.FXQuoteID == nil && e.balances != nil
	var key balanceDisplayKey
	if cacheable {
		amount := snapshotItemBaseAmountString(item)
		if query.Valuation == domain.ValuationNative {
			amount = item.NativeAmount
		}
		cacheable = amount != ""
		key = balanceDisplayKey{currency: valueCurrency(item, query.Valuation, input.Portfolio.Household.BaseCurrency), amount: amount, liability: account.IsLiability()}
		if cached, ok := e.balances[key]; ok {
			exact := cached.exact
			return cached.view, &exact, nil
		}
	}
	e.marketDate = date
	c, missing, uncertain := snapshotDisplayItem(item, e)
	if !present || item.AccountID == "" {
		c.Available = false
		c.BaseAmountExact = ""
		missing = []domain.MissingInputView{{Kind: domain.MissingAccountValue}}
	}
	currency := valueCurrency(item, query.Valuation, input.Portfolio.Household.BaseCurrency)
	if query.Valuation == domain.ValuationNative {
		c.BaseAmountExact = item.NativeAmount
		c.Available = present && item.AccountID != "" && item.NativeAmount != ""
		c.FXEvidence = nil
		filtered := []domain.MissingInputView{}
		for _, m := range missing {
			if m.Kind != domain.MissingFXRate {
				filtered = append(filtered, m)
			}
		}
		missing = filtered
	}
	if c.BaseAmountExact != "" && account.IsLiability() {
		value, err := decimal.NewFromString(c.BaseAmountExact)
		if err != nil {
			return domain.ValuationAmount{}, nil, err
		}
		c.BaseAmountExact = value.Neg().String()
	}
	var a displayAccumulator
	a.uncertain = uncertain
	if err := a.add(c, missing, date, false); err != nil {
		return domain.ValuationAmount{}, nil, err
	}
	value, err := a.amount(currency, true)
	if err != nil {
		return value, nil, err
	}
	if value.KnownSubtotal == nil {
		return value, nil, nil
	}
	exact := a.sum
	if cacheable {
		e.balances[key] = balanceDisplayValue{view: value, exact: exact}
	}
	return value, &exact, nil
}
