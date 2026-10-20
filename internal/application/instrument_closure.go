package application

import (
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func closureFactsFromCoverage(id domain.InstrumentID, coverage []domain.InstrumentHistoryCoverage) []domain.InstrumentClosure {
	var out []domain.InstrumentClosure
	for _, c := range coverage {
		if c.InstrumentID == id {
			for _, date := range c.ClosedDates {
				out = append(out, domain.InstrumentClosure{InstrumentID: id, Date: date})
			}
		}
	}
	return out
}

func instrumentObservationDate(instrument domain.Instrument, date string, at time.Time) string {
	if date != "" {
		return date
	}
	if at.IsZero() {
		return ""
	}
	if instrument.MarketCode != nil {
		if schedule, ok := domain.EquitySessionScheduleForMarket(*instrument.MarketCode); ok {
			if loc, err := time.LoadLocation(schedule.Timezone); err == nil {
				return at.In(loc).Format("2006-01-02")
			}
		}
	}
	return at.UTC().Format("2006-01-02")
}
