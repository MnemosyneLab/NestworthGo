package application

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// finalizeAnalysisReturns turns the component-level orthogonal projections
// into one daily Modified Dietz series and a geometrically linked period rate.
// AssetBuckets are intentionally not read here: return amount and Dietz
// capital are independent projections of the same classified effects.
func finalizeAnalysisReturns(result *domain.PeriodAnalysisResult, universe analysisUniverse, input AnalysisInputs, query domain.AnalysisQuery) error {
	if result == nil {
		return fmt.Errorf("analysis result is nil")
	}
	location, err := time.LoadLocation(input.Origin.Timezone)
	if err != nil {
		return err
	}
	valuationCurrency := input.Portfolio.Household.BaseCurrency
	if query.Valuation == domain.ValuationNative && len(universe.context.Universe.Currencies) > 0 {
		valuationCurrency = universe.context.Universe.Currencies[0]
	}
	start, err := time.ParseInLocation("2006-01-02", string(query.From), location)
	if err != nil {
		return err
	}
	end, err := time.ParseInLocation("2006-01-02", string(query.To), location)
	if err != nil {
		return err
	}

	byDate := make(map[domain.LocalDate][]*domain.ComponentDay)
	for index := range result.Days {
		day := &result.Days[index]
		if universe.investmentComponentInUniverse(day.Component) {
			byDate[day.Date] = append(byDate[day.Date], day)
		}
	}
	result.DailyReturns = make([]domain.DailyReturn, 0, result.Coverage.TotalDays)
	periodReturn := decimal.Zero
	periodBeginning := decimal.Zero
	hasPeriodBeginning := false
	hasPeriodCurrency := false
	linked := decimal.NewFromInt(1)
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		localDate := domain.LocalDate(date.Format("2006-01-02"))
		componentDays := byDate[localDate]
		daily := domain.DailyReturn{Date: localDate, Status: domain.CompletenessUnavailable}
		amount := decimal.Zero
		beginning := decimal.Zero
		flows := make([]domain.DietzCapitalFlow, 0)
		complete := len(componentDays) > 0
		for _, componentDay := range componentDays {
			if componentDay.Status != domain.CompletenessOK {
				complete = false
			}
			if componentDay.ReturnAmount != nil {
				amount = amount.Add(componentDay.ReturnAmount.Amount())
				hasPeriodCurrency = true
			}
			if componentDay.Status == domain.CompletenessOK {
				beginning = beginning.Add(componentDay.BeginningValue.Amount())
				if localDate == query.From {
					periodBeginning = periodBeginning.Add(componentDay.BeginningValue.Amount())
					hasPeriodBeginning = true
				}
				hasPeriodCurrency = true
			}
			flows = append(flows, componentDay.DietzCapitalFlows...)
		}
		if len(componentDays) == 0 {
			complete = false
		}
		if len(componentDays) > 0 {
			daily.Status = domain.CompletenessPartial
			if complete {
				daily.Status = domain.CompletenessOK
			}
			amountMoney, moneyErr := newSigned(amount, valuationCurrency)
			if moneyErr != nil {
				return moneyErr
			}
			daily.Amount = &amountMoney
			periodReturn = periodReturn.Add(amount)
		}

		dayStart, dayEnd := localDayBounds(date, location)
		capital := beginning
		for _, flow := range flows {
			capital = capital.Add(flow.Amount.Amount().Mul(dietzFlowWeight(flow.EffectiveAt, dayStart, dayEnd)))
		}
		if len(componentDays) > 0 {
			capitalMoney, moneyErr := newSigned(capital, valuationCurrency)
			if moneyErr != nil {
				return moneyErr
			}
			daily.InvestedCapital = &capitalMoney
			if complete && capital.IsPositive() {
				rate := amount.Div(capital)
				daily.Rate = &rate
				daily.Status = domain.CompletenessOK
				linked = linked.Mul(decimal.NewFromInt(1).Add(rate))
				result.Coverage.RatedDays++
			}
		}
		result.DailyReturns = append(result.DailyReturns, daily)
	}

	if hasPeriodCurrency {
		returnMoney, moneyErr := newSigned(periodReturn, valuationCurrency)
		if moneyErr != nil {
			return moneyErr
		}
		result.ReturnAmount = &returnMoney
		if hasPeriodBeginning {
			beginningMoney, moneyErr := newSigned(periodBeginning, valuationCurrency)
			if moneyErr != nil {
				return moneyErr
			}
			result.InvestedCapital = &beginningMoney
		}
	}
	if result.Coverage.RatedDays == result.Coverage.TotalDays && result.Coverage.RatedDays > 0 {
		result.Status = domain.CompletenessOK
	} else if result.Coverage.RatedDays > 0 {
		result.Status = domain.CompletenessPartial
	} else {
		result.Status = domain.CompletenessUnavailable
	}
	if result.Coverage.RatedDays*2 >= result.Coverage.TotalDays && result.Coverage.RatedDays > 0 {
		result.ReturnRate = &linked
		*result.ReturnRate = result.ReturnRate.Sub(decimal.NewFromInt(1))
	}
	return nil
}

func localDayBounds(date time.Time, location *time.Location) (time.Time, time.Time) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	return start, end
}

func analysisDayFlowWeight(date, timezone string, effectiveAt time.Time) decimal.Decimal {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return decimal.Zero
	}
	localDate, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil {
		return decimal.Zero
	}
	dayStart, dayEnd := localDayBounds(localDate, location)
	return dietzFlowWeight(effectiveAt, dayStart, dayEnd)
}

func dietzFlowWeight(effectiveAt, dayStart, dayEnd time.Time) decimal.Decimal {
	dayLength := dayEnd.Sub(dayStart)
	if dayLength <= 0 {
		return decimal.Zero
	}
	remaining := dayEnd.Sub(effectiveAt)
	if remaining < 0 {
		return decimal.Zero
	}
	if remaining > dayLength {
		return decimal.NewFromInt(1)
	}
	return decimal.NewFromInt(remaining.Nanoseconds()).Div(decimal.NewFromInt(dayLength.Nanoseconds()))
}

// GeometricLink is exported for deterministic callers and group-folding
// tests. Rates are already daily Modified Dietz rates; no zero-rate is
// substituted for an omitted day.
func GeometricLink(rates []decimal.Decimal) decimal.Decimal {
	linked := decimal.NewFromInt(1)
	for _, rate := range rates {
		linked = linked.Mul(decimal.NewFromInt(1).Add(rate))
	}
	return linked.Sub(decimal.NewFromInt(1))
}

// ModifiedDietzRate applies the same event-time weighting used by the kernel.
// It returns false when invested capital is not positive; callers must not
// report that day as a synthetic 0% day.
func ModifiedDietzRate(beginning, returnAmount decimal.Decimal, flows []domain.DietzCapitalFlow, dayStart, dayEnd time.Time) (decimal.Decimal, bool) {
	capital := beginning
	for _, flow := range flows {
		capital = capital.Add(flow.Amount.Amount().Mul(dietzFlowWeight(flow.EffectiveAt, dayStart, dayEnd)))
	}
	if !capital.IsPositive() {
		return decimal.Zero, false
	}
	return returnAmount.Div(capital), true
}

type AnalysisGroupBy string

const (
	GroupByInstrument AnalysisGroupBy = "instrument"
	GroupByAccount    AnalysisGroupBy = "account"
	GroupByCurrency   AnalysisGroupBy = "currency"
	GroupByAssetClass AnalysisGroupBy = "asset_class"
)

type ReturnGroup struct {
	Key          string
	ReturnAmount decimal.Decimal
	ReturnRate   *decimal.Decimal
	Coverage     domain.RateCoverage
	Status       domain.Completeness
}

// FoldReturnGroups folds the already-computed component days. It never
// replays activities. Return amounts are additive, while Dietz capital is
// rebuilt from each group's beginning values and the potential capital legs
// retained on AttributedEffect. This lets a group reclassify a household-
// internal trade as capital entering the instrument group, without doing a
// second engine pass.
func FoldReturnGroups(result domain.PeriodAnalysisResult, by AnalysisGroupBy) ([]ReturnGroup, error) {
	type groupDay struct {
		amount          decimal.Decimal
		beginning       decimal.Decimal
		legacyCapital   decimal.Decimal
		flows           []domain.DietzCapitalFlow
		complete        bool
		hasFlowMetadata bool
	}
	groups := make(map[string]map[domain.LocalDate]*groupDay)
	for _, day := range result.Days {
		// ComponentDay is also the Asset Changes granularity. Return folding must
		// select the same InvestmentUniverse as the daily linker, so excluded
		// cash/simple components and liabilities do not create return rows.
		if !returnEligibleComponent(day.Component, result.Query.IncludeCash) {
			continue
		}
		key := returnGroupKey(day.Component, by)
		if key == "" {
			return nil, fmt.Errorf("unsupported analysis group: %s", by)
		}
		if _, ok := groups[key]; !ok {
			groups[key] = make(map[domain.LocalDate]*groupDay)
		}
		bucket := groups[key][day.Date]
		if bucket == nil {
			bucket = &groupDay{complete: true}
			groups[key][day.Date] = bucket
		}
		if day.Status != domain.CompletenessOK {
			bucket.complete = false
		}
		if day.ReturnAmount != nil {
			bucket.amount = bucket.amount.Add(day.ReturnAmount.Amount())
		}
		if len(day.AttributedEffects) == 0 && day.InvestedCapital != nil {
			// Preserve compatibility with callers that construct ComponentDay
			// values by hand without the effect-level flow metadata.
			bucket.legacyCapital = bucket.legacyCapital.Add(day.InvestedCapital.Amount())
		} else {
			bucket.beginning = bucket.beginning.Add(day.BeginningValue.Amount())
		}
		for _, attributed := range day.AttributedEffects {
			flow := attributed.PotentialDietzCapitalFlow
			if flow == nil {
				flow = attributed.DietzCapitalFlow
			}
			if flow == nil {
				continue
			}
			bucket.flows = append(bucket.flows, *flow)
			bucket.hasFlowMetadata = true
		}
	}

	var location *time.Location
	for _, days := range groups {
		for _, day := range days {
			if !day.hasFlowMetadata {
				continue
			}
			if result.AnalysisDayTimezone == "" {
				return nil, fmt.Errorf("analysis day timezone is required for group Dietz folding")
			}
			var err error
			location, err = time.LoadLocation(result.AnalysisDayTimezone)
			if err != nil {
				return nil, err
			}
			break
		}
		if location != nil {
			break
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	resultGroups := make([]ReturnGroup, 0, len(keys))
	for _, key := range keys {
		group := ReturnGroup{Key: key, Coverage: domain.RateCoverage{TotalDays: result.Coverage.TotalDays}}
		rates := make([]decimal.Decimal, 0, result.Coverage.TotalDays)
		for date, daily := range groups[key] {
			group.ReturnAmount = group.ReturnAmount.Add(daily.amount)
			capital := daily.beginning.Add(daily.legacyCapital)
			if len(daily.flows) > 0 {
				localDate, err := time.ParseInLocation("2006-01-02", string(date), location)
				if err != nil {
					return nil, err
				}
				dayStart, dayEnd := localDayBounds(localDate, location)
				for _, flow := range daily.flows {
					capital = capital.Add(flow.Amount.Amount().Mul(dietzFlowWeight(flow.EffectiveAt, dayStart, dayEnd)))
				}
			}
			if daily.complete && capital.IsPositive() {
				rates = append(rates, daily.amount.Div(capital))
			}
		}
		group.Coverage.RatedDays = len(rates)
		if group.Coverage.RatedDays == group.Coverage.TotalDays && group.Coverage.RatedDays > 0 {
			group.Status = domain.CompletenessOK
		} else if group.Coverage.RatedDays > 0 {
			group.Status = domain.CompletenessPartial
		} else {
			group.Status = domain.CompletenessUnavailable
		}
		if len(rates)*2 >= result.Coverage.TotalDays && len(rates) > 0 {
			rate := GeometricLink(rates)
			group.ReturnRate = &rate
		}
		resultGroups = append(resultGroups, group)
	}
	return resultGroups, nil
}

func returnEligibleComponent(component domain.ComponentID, includeCash bool) bool {
	if component.InstrumentID != nil {
		return true
	}
	return includeCash && component.Cash && (component.AssetClass == "" || component.AssetClass == string(domain.BucketCash))
}

func returnGroupKey(component domain.ComponentID, by AnalysisGroupBy) string {
	switch by {
	case GroupByAccount:
		return component.AccountID.String()
	case GroupByCurrency:
		return component.Currency.String()
	case GroupByInstrument:
		if component.InstrumentID == nil {
			return domain.BucketCash
		}
		return component.InstrumentID.String()
	case GroupByAssetClass:
		if component.AssetClass != "" {
			return component.AssetClass
		}
		return "unknown"
	default:
		return ""
	}
}
