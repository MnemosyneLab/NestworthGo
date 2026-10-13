package domain

import "time"

// ResolveCivilDateCutoff resolves the last millisecond belonging to a civil
// date. day must be a UTC date label, not a household-local midnight instant.
// Transaction-time resolution separately rejects ambiguous/missing wall times.
func ResolveCivilDateCutoff(day time.Time, location *time.Location) (time.Time, error) {
	nextDay := day.AddDate(0, 0, 1)
	// IANA UTC offsets fit inside this window, including date-line changes.
	windowEnd := nextDay.Add(48 * time.Hour)
	var lastEnd time.Time
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
			return time.Time{}, &Error{Code: ErrInvalidChangeTime, Field: "timezone", Message: "cannot resolve the day boundary: timezone interval did not advance"}
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
		if start.Before(end) && end.After(lastEnd) {
			lastEnd = end
		}
		cursor = zoneEnd
	}
	if lastEnd.IsZero() {
		return time.Time{}, &Error{Code: ErrInvalidChangeTime, Field: "localDate", Message: "the selected civil date does not exist in the household timezone"}
	}
	return lastEnd.Add(-time.Millisecond), nil
}
