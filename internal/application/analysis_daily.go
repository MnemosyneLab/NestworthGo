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
	return computeAnalysisForValuationUniverse(input, query, false)
}

func computeInvestmentAnalysis(input AnalysisInputs, query domain.AnalysisQuery) (domain.PeriodAnalysisResult, error) {
	return computeAnalysisForValuationUniverse(input, query, true)
}

func computeAnalysisForValuationUniverse(input AnalysisInputs, query domain.AnalysisQuery, investmentOnly bool) (domain.PeriodAnalysisResult, error) {
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
	input.Snapshots = snapshotsInAnalysisWindow(input.Snapshots, query.From, query.To)
	universe, err := resolveAnalysisUniverse(input, query)
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
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
	location, err := time.LoadLocation(input.Origin.Timezone)
	if err != nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Field: "timezone", Message: "analysis requires a valid History Origin timezone"}
	}
	start, err := time.ParseInLocation("2006-01-02", query.From, location)
	if err != nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrValidation, Field: "from", Message: "date must use YYYY-MM-DD"}
	}
	end, err := time.ParseInLocation("2006-01-02", query.To, location)
	if err != nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrValidation, Field: "to", Message: "date must use YYYY-MM-DD"}
	}
	totalDays := 0
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		totalDays++
	}
	result := domain.PeriodAnalysisResult{Query: query, AnalysisDayTimezone: input.Origin.Timezone, Days: make([]domain.ComponentDay, 0, totalDays*len(valuationComponents)), Coverage: domain.RateCoverage{TotalDays: totalDays}, Status: domain.CompletenessOK}
	activitiesByDate := make(map[string][]domain.Activity, len(input.Activities))
	for _, activity := range input.Activities {
		localDate := activityLocalDate(activity, input.Origin.Timezone)
		activitiesByDate[localDate] = append(activitiesByDate[localDate], activity)
	}
	previousItems := make(map[string]domain.DailyValuationSnapshotItem)
	firstPreviousDate := start.AddDate(0, 0, -1).Format("2006-01-02")
	if previousSnapshot, ok := byDate[firstPreviousDate]; ok {
		previousItems = snapshotItemsByComponent(previousSnapshot, universe.accounts, universe.instruments)
	}
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
			day, err := buildComponentDay(localDate, component, previousItems[component.Key()], endItems[component.Key()], previousSnapshot, daySnapshot, hasPrevious, hasDay, effectsByComponent[component.Key()], input, query, universe)
			if err != nil {
				return domain.PeriodAnalysisResult{}, err
			}
			if day.Status == domain.CompletenessOK && (universe.hasUnknownAccount(localDate) || universe.hasUnknownAccount(previousDate)) {
				day.Status = domain.CompletenessPartial
			}
			result.Days = append(result.Days, day)
		}
		previousItems = endItems
	}
	if err := finalizeAnalysisReturns(&result, universe, input, query); err != nil {
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

func buildComponentDay(date string, component domain.ComponentID, previous, current domain.DailyValuationSnapshotItem, previousSnapshot, currentSnapshot domain.DailyValuationSnapshot, hasPrevious, hasCurrent bool, effects []classifiedAnalysisEffect, input AnalysisInputs, query domain.AnalysisQuery, universe analysisUniverse) (domain.ComponentDay, error) {
	day := domain.ComponentDay{Date: domain.LocalDate(date), Component: component, AssetBuckets: make(map[domain.AttributionBucket]domain.SignedMoney), AssetBucketExact: make(map[domain.AttributionBucket]decimal.Decimal), ReturnComponents: make(map[domain.ReturnComponent]domain.SignedMoney), DietzCapitalFlows: make([]domain.DietzCapitalFlow, 0), AttributedEffects: make([]domain.AttributedEffect, 0), Status: domain.CompletenessOK}
	// A trade can create a holding component, close it, or create a foreign
	// cash sleeve on the same day. The snapshot legitimately omits a zero
	// balance component at one side of that boundary, but dropping the whole
	// day would also drop the known trade leg and make the household Asset
	// Changes waterfall fail to reconcile. An activity is the only safe proof
	// that the missing side is an opening zero; without one, keep the day
	// unavailable so missing valuation data is never guessed as zero.
	if (len(effects) > 0 || componentHasActivityInRange(component, input.Activities, query.From, query.To)) && previousSnapshot.Complete && currentSnapshot.Complete {
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
		if err := addBucket(day.AssetBuckets, domain.BucketPriceChange, bridge.price, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
			return domain.ComponentDay{}, err
		}
		addExactBucket(day.AssetBucketExact, domain.BucketPriceChange, bridge.price)
		if universe.investmentComponentInUniverse(component) {
			if err := addReturn(day.ReturnComponents, domain.ReturnPriceChange, bridge.price, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
				return domain.ComponentDay{}, err
			}
		}
		priceBucket := domain.BucketPriceChange
		attributedAmount, amountErr := newSigned(bridge.price, valueCurrency(current, query.Valuation, baseCurrency))
		if amountErr != nil {
			return domain.ComponentDay{}, amountErr
		}
		var priceReturn *domain.ReturnComponent
		if universe.investmentComponentInUniverse(component) {
			value := domain.ReturnPriceChange
			priceReturn = &value
		}
		day.AttributedEffects = append(day.AttributedEffects, domain.AttributedEffect{AssetBucket: &priceBucket, ReturnComponent: priceReturn, Amount: attributedAmount, Component: component})
	}
	if bridge.fx.Sign() != 0 {
		if err := addBucket(day.AssetBuckets, domain.BucketFXImpact, bridge.fx, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
			return domain.ComponentDay{}, err
		}
		addExactBucket(day.AssetBucketExact, domain.BucketFXImpact, bridge.fx)
		if universe.investmentComponentInUniverse(component) {
			if err := addReturn(day.ReturnComponents, domain.ReturnFXImpact, bridge.fx, valueCurrency(current, query.Valuation, baseCurrency)); err != nil {
				return domain.ComponentDay{}, err
			}
		}
		fxBucket := domain.BucketFXImpact
		attributedAmount, amountErr := newSigned(bridge.fx, valueCurrency(current, query.Valuation, baseCurrency))
		if amountErr != nil {
			return domain.ComponentDay{}, amountErr
		}
		var fxReturn *domain.ReturnComponent
		if universe.investmentComponentInUniverse(component) {
			value := domain.ReturnFXImpact
			fxReturn = &value
		}
		day.AttributedEffects = append(day.AttributedEffects, domain.AttributedEffect{AssetBucket: &fxBucket, ReturnComponent: fxReturn, Amount: attributedAmount, Component: component})
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
	if closeQuote == nil {
		// A closed position with no closing quote still has unaccounted price
		// movement between the opening mark and the disposal.
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
	q0 := decimal.Zero
	if openQuote != nil {
		q0 = opening.Div(openQuote.UnitPrice.Decimal())
	} else if !opening.IsZero() {
		return holdingBridge{partial: true}, nil
	}
	qc := closing.Div(closeQuote.UnitPrice.Decimal())
	type segment struct{ quantity, price, fx decimal.Decimal }
	segments := make([]segment, 0, 1+len(effects))
	seenTrades := make(map[domain.ActivityID]bool)
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
