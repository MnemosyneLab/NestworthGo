package domain

import (
	"fmt"
	"strings"
	"time"
)

const maxLiquidityHorizonYears = 10

// ParseCivilDate validates and canonicalizes a YYYY-MM-DD date.
func ParseCivilDate(field, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", validation(field, "must use YYYY-MM-DD")
	}
	canonical, err := normalizeDate(field, &trimmed)
	if err != nil {
		return "", err
	}
	if canonical == nil {
		return "", validation(field, "must use YYYY-MM-DD")
	}
	return *canonical, nil
}

func parseCivilTime(field, value string) (time.Time, error) {
	canonical, err := ParseCivilDate(field, value)
	if err != nil {
		return time.Time{}, err
	}
	parsed, err := time.Parse(dateLayout, canonical)
	if err != nil {
		return time.Time{}, validation(field, "must use YYYY-MM-DD")
	}
	return parsed, nil
}

func compareCivilDates(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

// CivilDaysBetween returns the number of civil calendar days in [start, end).
func CivilDaysBetween(start, end string) (int, error) {
	startTime, err := parseCivilTime("startOn", start)
	if err != nil {
		return 0, err
	}
	endTime, err := parseCivilTime("endOn", end)
	if err != nil {
		return 0, err
	}
	if endTime.Before(startTime) {
		return 0, validation("endOn", "must not precede startOn")
	}
	return int(endTime.Sub(startTime).Hours() / 24), nil
}

// AddCalendarDays adds n calendar days to a YYYY-MM-DD date. Negative n is allowed.
func AddCalendarDays(date string, days int) (string, error) {
	parsed, err := parseCivilTime("date", date)
	if err != nil {
		return "", err
	}
	return parsed.AddDate(0, 0, days).Format(dateLayout), nil
}

// AddWeekdays skips Saturday and Sunday only. Holidays are never inferred.
func AddWeekdays(date string, days int) (string, error) {
	if days < 0 {
		return "", validation("settlementDays", "must not be negative")
	}
	parsed, err := parseCivilTime("date", date)
	if err != nil {
		return "", err
	}
	remaining := days
	for remaining > 0 {
		parsed = parsed.AddDate(0, 0, 1)
		weekday := parsed.Weekday()
		if weekday != time.Saturday && weekday != time.Sunday {
			remaining--
		}
	}
	return parsed.Format(dateLayout), nil
}

// SettlementReceiptOn maps an action-eligible date plus a settlement lag onto
// an estimated receipt date. A zero lag returns the eligible date. For N > 0,
// counting begins on the following date.
func SettlementReceiptOn(eligibleOn string, days int, basis DayBasis) (string, error) {
	if days < 0 || days > 365 {
		return "", validation("settlementDays", "must be between 0 and 365")
	}
	if days == 0 {
		return ParseCivilDate("eligibleOn", eligibleOn)
	}
	switch basis {
	case DayBasisCalendar:
		return AddCalendarDays(eligibleOn, days)
	case DayBasisWeekdays:
		return AddWeekdays(eligibleOn, days)
	default:
		return "", validation("dayBasis", "must be calendar or weekdays")
	}
}

// LocalCivilDate returns the YYYY-MM-DD date of asOf in timezone. A missing
// timezone falls back to UTC for read-only liquidity evaluation.
func LocalCivilDate(asOf time.Time, timezone string) (string, string, error) {
	zone := strings.TrimSpace(timezone)
	if zone == "" {
		return asOf.UTC().Format(dateLayout), "UTC", nil
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return "", "", validation("timezone", "must be a valid IANA timezone")
	}
	return asOf.In(location).Format(dateLayout), zone, nil
}

// LiquidityHorizons returns today, today+7, today+30, and an optional custom
// date. Custom dates must be today or later and at most 10 calendar years ahead.
func LiquidityHorizons(today string, custom *string) ([]string, error) {
	today, err := ParseCivilDate("today", today)
	if err != nil {
		return nil, err
	}
	plus7, err := AddCalendarDays(today, 7)
	if err != nil {
		return nil, err
	}
	plus30, err := AddCalendarDays(today, 30)
	if err != nil {
		return nil, err
	}
	horizons := []string{today, plus7, plus30}
	if custom == nil {
		return horizons, nil
	}
	customOn, err := ParseCivilDate("customHorizonOn", *custom)
	if err != nil {
		return nil, err
	}
	if compareCivilDates(customOn, today) < 0 {
		return nil, validation("customHorizonOn", "must be today or later")
	}
	todayTime, err := parseCivilTime("today", today)
	if err != nil {
		return nil, err
	}
	limit := todayTime.AddDate(maxLiquidityHorizonYears, 0, 0).Format(dateLayout)
	if compareCivilDates(customOn, limit) > 0 {
		return nil, validation("customHorizonOn", fmt.Sprintf("must be at most %d calendar years ahead", maxLiquidityHorizonYears))
	}
	for _, existing := range horizons {
		if existing == customOn {
			return horizons, nil
		}
	}
	return append(horizons, customOn), nil
}
