package domain

import "sort"

// Frozen pre-index evaluator: differential oracle for reservation association.
func evaluateLiquidityLegacyAssociation(query LiquidityQuery, sources []LiquiditySource, reservations []LiquidityReservation, convert ConvertToBase) (LiquidityOverview, error) {
	localDate, timezone, err := LocalCivilDate(query.AsOf, query.Timezone)
	if err != nil {
		return LiquidityOverview{}, err
	}
	horizons, err := LiquidityHorizons(localDate, query.CustomHorizonOn)
	if err != nil {
		return LiquidityOverview{}, err
	}
	overview := LiquidityOverview{
		AsOf:         query.AsOf,
		LocalDate:    localDate,
		Timezone:     timezone,
		BaseCurrency: query.BaseCurrency,
		Assumptions:  []LiquidityAssumption{AssumptionCurrentPricesAndFX, AssumptionAssetsOnly, AssumptionOutstandingDebtOmitted},
	}
	for _, source := range sources {
		if source.CurrentNativeAmount != "" {
			if _, err := ParseNativeAmount(source.CurrentNativeAmount); err != nil {
				return LiquidityOverview{}, err
			}
		}
	}
	activeBySource := map[string]Money{}
	for _, reservation := range reservations {
		if err := reservation.Validate(); err != nil {
			return LiquidityOverview{}, err
		}
		if !reservation.Active() {
			continue
		}
		key := reservation.Source.Key()
		found := false
		for _, source := range sources {
			if source.Ref.Key() == key && !source.Archived && !source.Liability && !source.QuantityZero && liquiditySourcePositiveOrUnknown(source) {
				found = true
				break
			}
		}
		if !found {
			overview.UnresolvedReservations = append(overview.UnresolvedReservations, UnresolvedReservation{Reservation: reservation, Reason: "source is missing, archived, or no longer positive"})
			continue
		}
		current, ok := activeBySource[key]
		if !ok {
			activeBySource[key] = reservation.Amount
			continue
		}
		sum, err := current.Add(reservation.Amount)
		if err != nil {
			return LiquidityOverview{}, err
		}
		activeBySource[key] = sum
	}
	results := make([]LiquiditySourceResult, 0, len(sources))
	for _, source := range sources {
		if err := source.Ref.Validate(); err != nil {
			return LiquidityOverview{}, err
		}
		result, err := evaluateSource(query, localDate, horizons, source, activeBySource[source.Ref.Key()], convert)
		if err != nil {
			return LiquidityOverview{}, err
		}
		results = append(results, result)
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].DisplayName != results[j].DisplayName {
			return results[i].DisplayName < results[j].DisplayName
		}
		return results[i].SourceKey < results[j].SourceKey
	})
	overview.Sources = results
	overview.Buckets = make([]LiquidityBucket, 0, len(horizons))
	for _, horizon := range horizons {
		bucket, err := aggregateBucket(horizon, query.BaseCurrency, results, convert)
		if err != nil {
			return LiquidityOverview{}, err
		}
		overview.Buckets = append(overview.Buckets, bucket)
	}
	if err := assertCumulativeMonotonic(overview.Buckets); err != nil {
		return LiquidityOverview{}, err
	}
	return overview, nil
}
