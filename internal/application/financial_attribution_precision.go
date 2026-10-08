package application

import (
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type attributionPrecisionProof struct {
	beginning, ending, exactDelta decimal.Decimal
}

// This is a monetary projection of verified snapshots and existing engine
// buckets, not another attribution calculation. Neither an unknown exact driver
// gap nor the engine's residual tolerance is evidence of a rounding difference.
func proveAttributionPrecision(input AnalysisInputs, result domain.PeriodAnalysisResult, comparison *FinancialComparisonContent) (attributionPrecisionProof, string, error) {
	var proof attributionPrecisionProof
	universe, err := resolveAnalysisUniverse(input, result.Query)
	if err != nil {
		return proof, "", err
	}
	type boundary struct {
		exact, projected decimal.Decimal
		components       map[string]decimal.Decimal
	}
	boundaries := make(map[string]boundary, len(input.Snapshots))
	for _, snapshot := range input.Snapshots {
		items := snapshotItemsByComponent(snapshot, universe.accounts, universe.instruments)
		value := boundary{exact: decimal.Zero, projected: decimal.Zero, components: make(map[string]decimal.Decimal, len(items))}
		for _, component := range universe.context.Universe.Components {
			item, present := items[component.Key()]
			if !present {
				// Before this proof every engine component/day was checked for
				// usable boundaries. Its existing absence/zero rules apply.
				continue
			}
			exact, available, err := snapshotItemValue(item, universe.accounts[component.AccountID], result.Query.Valuation, input.Portfolio.Household.BaseCurrency)
			if err != nil {
				return proof, "", err
			}
			if !available {
				return proof, "analysis_endpoint_mismatch", nil
			}
			projected, err := newSigned(exact, input.Portfolio.Household.BaseCurrency)
			if err != nil {
				return proof, "", err
			}
			value.exact = value.exact.Add(exact)
			value.projected = value.projected.Add(projected.Amount())
			value.components[component.Key()] = projected.Amount()
		}
		boundaries[snapshot.LocalDate] = value
	}
	start, _ := time.Parse("2006-01-02", result.Query.From)
	end, _ := time.Parse("2006-01-02", result.Query.To)
	left, leftOK := boundaries[start.AddDate(0, 0, -1).Format("2006-01-02")]
	right, rightOK := boundaries[result.Query.To]
	if !leftOK || !rightOK {
		return proof, "analysis_endpoint_mismatch", nil
	}
	proof = attributionPrecisionProof{left.projected, right.projected, right.exact.Sub(left.exact)}
	currency := input.Portfolio.Household.BaseCurrency.String()
	if !sameAttributionAmount(contextAmount(historicalString(left.exact.String()), currency), comparison.Left.Summary.NetWorth) || !sameAttributionAmount(contextAmount(historicalString(right.exact.String()), currency), comparison.Right.Summary.NetWorth) {
		return proof, "analysis_endpoint_mismatch", nil
	}
	if !sameAttributionAmount(contextAmount(historicalString(proof.exactDelta.String()), currency), comparison.Change.NetWorth) {
		return proof, "driver_reconciliation_mismatch", nil
	}
	exactDrivers := make(map[string]decimal.Decimal)
	for _, day := range result.Days {
		date, _ := time.Parse("2006-01-02", day.Date)
		previous, previousOK := boundaries[date.AddDate(0, 0, -1).Format("2006-01-02")]
		current, currentOK := boundaries[day.Date]
		if !previousOK || !currentOK || day.BeginningValue.Currency() != input.Portfolio.Household.BaseCurrency || day.EndingValue.Currency() != input.Portfolio.Household.BaseCurrency || !day.BeginningValue.Amount().Equal(previous.components[day.Component.Key()]) || !day.EndingValue.Amount().Equal(current.components[day.Component.Key()]) {
			return proof, "analysis_endpoint_mismatch", nil
		}
		// Reuse the same exact/fallback contract as aggregateAssetWaterfall.
		buckets, _ := aggregateAssetWaterfall(domain.PeriodAnalysisResult{Days: []domain.ComponentDay{day}})
		for _, amount := range buckets {
			exactDrivers[day.Date] = exactDrivers[day.Date].Add(amount)
		}
	}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		previous, previousOK := boundaries[day.AddDate(0, 0, -1).Format("2006-01-02")]
		current, currentOK := boundaries[date]
		if !previousOK || !currentOK || !exactDrivers[date].Equal(current.exact.Sub(previous.exact)) {
			return proof, "driver_reconciliation_mismatch", nil
		}
	}
	return proof, "", nil
}
