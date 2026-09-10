package domain

import (
	"strings"
	"time"
)

// MarketDataClock pins backend now and the household history timezone for
// vNext market-data contracts. OS/display timezone changes must not
// reinterpret stored history.
type MarketDataClock struct {
	Now               time.Time
	HouseholdTimezone string
}

func NewMarketDataClock(now time.Time, timezone string) (MarketDataClock, error) {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return MarketDataClock{}, &Error{Code: ErrHistoryTimezoneRequired, Field: "timezone", Message: "confirm a Household timezone before valuing market data"}
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return MarketDataClock{}, &Error{Code: ErrHistoryTimezoneRequired, Field: "timezone", Message: "timezone must be a valid IANA timezone"}
	}
	if now.IsZero() {
		return MarketDataClock{}, &Error{Code: ErrValidation, Field: "now", Message: "backend clock is required"}
	}
	return MarketDataClock{Now: now.UTC(), HouseholdTimezone: timezone}, nil
}

func (c MarketDataClock) Location() (*time.Location, error) {
	return time.LoadLocation(c.HouseholdTimezone)
}

func (c MarketDataClock) TodayLocal() (string, error) {
	location, err := c.Location()
	if err != nil {
		return "", &Error{Code: ErrHistoryTimezoneRequired, Field: "timezone", Message: "timezone must be a valid IANA timezone"}
	}
	return c.Now.In(location).Format("2006-01-02"), nil
}

// HouseholdDayCutoff is the exclusive end of a closed local day: the next
// local midnight minus one millisecond. Snapshot eligibility compares
// economic timestamps against this instant, never market-date equality.
func HouseholdDayCutoff(localDate, timezone string) (time.Time, error) {
	if _, err := ParseMarketDate(localDate); err != nil {
		return time.Time{}, err
	}
	parsed, err := time.Parse("2006-01-02", localDate)
	if err != nil {
		return time.Time{}, &Error{Code: ErrValidation, Field: "localDate", Message: "local date must use YYYY-MM-DD"}
	}
	nextLocal := parsed.AddDate(0, 0, 1).Format("2006-01-02")
	nextMidnight, err := ResolveLocalDateTime(nextLocal, "00:00", timezone)
	if err != nil {
		return time.Time{}, err
	}
	return nextMidnight.Add(-time.Millisecond), nil
}

// ObservationEligible reports whether an economic instant may contribute to a
// valuation at cutoff. A close formed after the cutoff is never eligible.
func ObservationEligible(valueEffectiveAt, cutoff time.Time) bool {
	if valueEffectiveAt.IsZero() || cutoff.IsZero() {
		return false
	}
	return !valueEffectiveAt.After(cutoff)
}

func ParseMarketDate(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	parsed, err := time.Parse("2006-01-02", trimmed)
	if err != nil || parsed.Format("2006-01-02") != trimmed {
		return "", validation("marketDate", "must use YYYY-MM-DD")
	}
	return trimmed, nil
}

func CompareMarketDate(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func InclusiveMarketDates(start, end string) ([]string, error) {
	startDate, err := ParseMarketDate(start)
	if err != nil {
		return nil, err
	}
	endDate, err := ParseMarketDate(end)
	if err != nil {
		return nil, err
	}
	if startDate > endDate {
		return nil, validation("dateRange", "start must not follow end")
	}
	cursor, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, validation("dateRange", "start must use YYYY-MM-DD")
	}
	last, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, validation("dateRange", "end must use YYYY-MM-DD")
	}
	dates := make([]string, 0)
	for !cursor.After(last) {
		dates = append(dates, cursor.Format("2006-01-02"))
		cursor = cursor.AddDate(0, 0, 1)
	}
	return dates, nil
}
