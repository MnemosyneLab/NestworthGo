package domain

import "time"

// ResolveCivilDateCutoff resolves the last millisecond belonging to a civil
// date. day must be a UTC date label, not a household-local midnight instant.
// Transaction-time resolution separately rejects ambiguous/missing wall times.
func ResolveCivilDateCutoff(day time.Time, location *time.Location) (time.Time, error) {
	_, end, err := resolveCivilDateBounds(day, location)
	if err != nil {
		return time.Time{}, err
	}
	return end.Add(-time.Millisecond), nil
}

// ResolveCivilDateStart resolves the earliest actual instant whose local date
// label is day. A midnight skipped by a clock change resolves to the first
// valid instant on that date; an entirely skipped civil date returns an error.
func ResolveCivilDateStart(day time.Time, location *time.Location) (time.Time, error) {
	start, _, err := resolveCivilDateBounds(day, location)
	return start, err
}

func resolveCivilDateBounds(day time.Time, location *time.Location) (time.Time, time.Time, error) {
	nextDay := day.AddDate(0, 0, 1)
	// IANA UTC offsets fit inside this window, including date-line changes.
	windowEnd := nextDay.Add(48 * time.Hour)
	var firstStart, lastEnd time.Time
	for cursor := day.Add(-48 * time.Hour); cursor.Before(windowEnd); {
		local := cursor.In(location)
		_, offset := local.Zone()
		_, zoneEnd := local.ZoneBounds()
		if zoneEnd.IsZero() || zoneEnd.After(windowEnd) {
			zoneEnd = windowEnd
		}
		// Some runtime/tzdata combinations do not advance at a future POSIX
		// year boundary. Never loop forever or invent an offset across it.
		if !zoneEnd.After(cursor) {
			return time.Time{}, time.Time{}, &Error{Code: ErrInvalidChangeTime, Field: "timezone", Message: "cannot resolve the day boundary: timezone interval did not advance"}
		}
		// Intersect this constant-offset interval with the requested wall date.
		start := day.Add(-time.Duration(offset) * time.Second)
		end := nextDay.Add(-time.Duration(offset) * time.Second)
		if start.Before(cursor) {
			start = cursor
		}
		if end.After(zoneEnd) {
			end = zoneEnd
		}
		if start.Before(end) {
			if firstStart.IsZero() || start.Before(firstStart) {
				firstStart = start
			}
			if end.After(lastEnd) {
				lastEnd = end
			}
		}
		cursor = zoneEnd
	}
	if lastEnd.IsZero() {
		return time.Time{}, time.Time{}, &Error{Code: ErrInvalidChangeTime, Field: "localDate", Message: "the selected civil date does not exist in the household timezone"}
	}
	return firstStart, lastEnd, nil
}
