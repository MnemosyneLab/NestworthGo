package domain

import "time"

const FirstVerticalSliceScenarioID = "e2e-tiingo-us-manual-fx-v1"

// FirstVerticalSliceScenario is the independent economic-fact oracle for
// the first Tiingo US history + manual FX end-to-end slice. Provider JSON is
// not an input.
func FirstVerticalSliceScenario() (OracleScenario, error) {
	location, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		return OracleScenario{}, &Error{Code: ErrHistoryTimezoneRequired, Field: "timezone", Message: "timezone must be a valid IANA timezone"}
	}
	now := time.Date(2026, time.September, 10, 0, 5, 0, 0, location)
	clock, err := NewMarketDataClock(now, "Asia/Singapore")
	if err != nil {
		return OracleScenario{}, err
	}
	return OracleScenario{
		ID:                 FirstVerticalSliceScenarioID,
		Clock:              clock,
		BaseCurrency:       "SGD",
		StartingPointLocal: time.Date(2026, time.September, 6, 0, 0, 0, 0, location),
		Holding:            OracleHolding{Quantity: "10", QuoteCurrency: "USD"},
		Cash:               OracleCash{Amount: "1000", Currency: "USD"},
		FX:                 OracleFX{BaseCurrency: "USD", QuoteCurrency: "SGD", Rate: "1.35"},
		Closes: []OracleClose{
			{MarketDate: "2026-09-04", Close: "185.25", SessionTimezone: USEquitySessionTimezone, CloseClock: USEquityRegularCloseClock, SessionKind: SessionKindRegular, Revision: 1},
			{MarketDate: "2026-09-07", Close: "186", SessionTimezone: USEquitySessionTimezone, CloseClock: USEquityRegularCloseClock, SessionKind: SessionKindRegular, Revision: 1},
			{MarketDate: "2026-09-08", Close: "185.25", SessionTimezone: USEquitySessionTimezone, CloseClock: USEquityRegularCloseClock, SessionKind: SessionKindRegular, Revision: 1},
			{MarketDate: "2026-09-08", Close: "185.25000370", SessionTimezone: USEquitySessionTimezone, CloseClock: USEquityRegularCloseClock, SessionKind: SessionKindRegular, Revision: 2, SupersedesRevision: 1},
		},
		PendingMarketDates: []string{"2026-09-09"},
	}, nil
}
