package application

import (
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// A closed day validates carry, never supplies an opening price or fills a
// missing intervening trading day. Crypto has no default weekend closure.
func defaultInstrumentClosedDate(kind domain.InstrumentType, date string) bool {
	switch kind {
	case domain.InstrumentStock, domain.InstrumentETF, domain.InstrumentMutualFund, domain.InstrumentBankInvestmentProduct:
		day, err := time.Parse("2006-01-02", date)
		return err == nil && (day.Weekday() == time.Saturday || day.Weekday() == time.Sunday)
	}
	return false
}
func instrumentClosedCarry(instrument domain.Instrument, from, to string, closures []domain.InstrumentClosure) bool {
	if from == "" || to == "" || from > to {
		return false
	}
	if from == to {
		return defaultInstrumentClosedDate(instrument.Type, to) || hasInstrumentClosure(instrument.ID, to, closures)
	}
	dates, err := domain.InclusiveMarketDates(from, to)
	if err != nil {
		return false
	}
	closed := map[string]bool{}
	for _, c := range closures {
		if c.InstrumentID == instrument.ID {
			closed[c.Date] = true
		}
	}
	for _, date := range dates {
		if date == from {
			continue
		}
		if !defaultInstrumentClosedDate(instrument.Type, date) && !closed[date] {
			return false
		}
	}
	return true
}
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

func hasInstrumentClosure(id domain.InstrumentID, date string, closures []domain.InstrumentClosure) bool {
	for _, c := range closures {
		if c.InstrumentID == id && c.Date == date {
			return true
		}
	}
	return false
}
