package domain

// InferredInstrumentClosure implements the default repair policy for an empty
// date between known equity closes. It is an inference, not exchange-calendar
// evidence, and must never be persisted as a provider-confirmed observation.
// Leading history and the still-open tail remain eligible for backfill.
func InferredInstrumentClosure(coverage InstrumentHistoryCoverage, date string) bool {
	if InstrumentUsesCryptoDailyBar(coverage.InstrumentType, coverage.Market) {
		return false
	}
	if _, ok := EquitySessionScheduleForMarket(coverage.Market); !ok {
		return false
	}
	for _, pending := range coverage.UnverifiedDates {
		if pending == date {
			return false
		}
	}
	before, after := false, false
	for _, close := range coverage.CloseMarketDates {
		if close == date {
			return false
		}
		if close < date {
			before = true
		}
		if close > date {
			after = true
		}
	}
	return before && after
}
