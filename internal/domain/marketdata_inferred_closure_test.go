package domain

import "testing"

func TestInferredClosureOnlyInsideVerifiedEquityHistory(t *testing.T) {
	base := InstrumentHistoryCoverage{InstrumentType: "etf", Market: "US", CloseMarketDates: []string{"2026-09-14", "2026-09-16"}}
	for _, test := range []struct {
		name, date string
		change     func(*InstrumentHistoryCoverage)
		want       bool
	}{
		{name: "Tuesday holiday", date: "2026-09-15", want: true},
		{name: "existing close", date: "2026-09-14"},
		{name: "leading history", date: "2026-09-11"},
		{name: "tail", date: "2026-09-17"},
		{name: "crypto", date: "2026-09-15", change: func(c *InstrumentHistoryCoverage) { c.InstrumentType = "crypto"; c.Market = "CRYPTO" }},
		{name: "explicitly unverified", date: "2026-09-15", change: func(c *InstrumentHistoryCoverage) { c.UnverifiedDates = []string{"2026-09-15"} }},
		{name: "unknown market", date: "2026-09-15", change: func(c *InstrumentHistoryCoverage) { c.Market = "UNKNOWN" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			coverage := base
			if test.change != nil {
				test.change(&coverage)
			}
			if got := InferredInstrumentClosure(coverage, test.date); got != test.want {
				t.Fatalf("got %v want %v", got, test.want)
			}
		})
	}
}
