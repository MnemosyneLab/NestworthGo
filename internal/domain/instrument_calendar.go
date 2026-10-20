package domain

import "time"

// DefaultInstrumentClosedDate defines the default publication calendar by type.
// Crypto and other instruments have no default weekend closure.
func DefaultInstrumentClosedDate(kind InstrumentType, date string) bool {
	switch kind {
	case InstrumentStock, InstrumentETF, InstrumentMutualFund, InstrumentBankInvestmentProduct, InstrumentBond, InstrumentPreciousMetal:
		day, err := time.Parse("2006-01-02", date)
		return err == nil && (day.Weekday() == time.Saturday || day.Weekday() == time.Sunday)
	}
	return false
}

// InstrumentClosedCarry validates every date after an eligible anchor. It never
// supplies an opening price or fills a missing intervening trading day.
func InstrumentClosedCarry(instrument Instrument, from, to string, closures []InstrumentClosure) bool {
	if from == "" || to == "" || from > to {
		return false
	}
	if from == to {
		return DefaultInstrumentClosedDate(instrument.Type, to) || HasInstrumentClosure(instrument.ID, to, closures)
	}
	dates, err := InclusiveMarketDates(from, to)
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
		if !DefaultInstrumentClosedDate(instrument.Type, date) && !closed[date] {
			return false
		}
	}
	return true
}

// HasInstrumentClosure matches a validated closure fact by instrument and date.
func HasInstrumentClosure(id InstrumentID, date string, closures []InstrumentClosure) bool {
	for _, c := range closures {
		if c.InstrumentID == id && c.Date == date {
			return true
		}
	}
	return false
}
