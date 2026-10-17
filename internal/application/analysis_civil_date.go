package application

import (
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Analysis periods may cover all retained history. The century bound prevents
// malformed API ranges from allocating or iterating an unbounded daily result.
const maxAnalysisRangeDays = 36_600

func analysisDateRange(from, to domain.LocalDate) (time.Time, time.Time, int, error) {
	start, err := time.Parse(time.DateOnly, string(from))
	if err != nil {
		return time.Time{}, time.Time{}, 0, &domain.Error{Code: domain.ErrValidation, Field: "from", Message: "date must use YYYY-MM-DD"}
	}
	end, err := time.Parse(time.DateOnly, string(to))
	if err != nil {
		return time.Time{}, time.Time{}, 0, &domain.Error{Code: domain.ErrValidation, Field: "to", Message: "date must use YYYY-MM-DD"}
	}
	days := int((end.Unix()-start.Unix())/int64(24*time.Hour/time.Second)) + 1
	if days < 1 || days > maxAnalysisRangeDays {
		return time.Time{}, time.Time{}, 0, &domain.Error{Code: domain.ErrValidation, Field: "range", Message: "analysis range may not exceed 100 years"}
	}
	return start, end, days, nil
}

// analysisCivilDateCutoffAtOrBefore resolves the selected UTC civil label with
// the historical cutoff contract. A small bounded lookback lets an endpoint
// on a completely skipped date stop at the last real preceding civil day.
func analysisCivilDateCutoffAtOrBefore(day time.Time, location *time.Location) (time.Time, error) {
	label := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	for skipped := 0; skipped <= 2; skipped++ {
		cutoff, err := historicalOverviewDayCutoff(label, location)
		if err == nil {
			return cutoff, nil
		}
		if !nonexistentHistoricalCivilDate(err) {
			return time.Time{}, err
		}
		label = label.AddDate(0, 0, -1)
	}
	return time.Time{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "localDate", Message: "cannot find a preceding civil date in the household timezone"}
}

// analysisCivilDayBounds treats date as a UTC date label. It returns no bounds
// for a completely skipped civil date; callers keep its label unavailable and
// do not value an observation on that nonexistent day.
func analysisCivilDayBounds(date time.Time, location *time.Location) (start, end time.Time, exists bool, err error) {
	label := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	cutoff, err := historicalOverviewDayCutoff(label, location)
	if nonexistentHistoricalCivilDate(err) {
		return time.Time{}, time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	previous, err := analysisCivilDateCutoffAtOrBefore(label.AddDate(0, 0, -1), location)
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	start = previous.Add(time.Millisecond)
	end = cutoff.Add(time.Millisecond)
	if !end.After(start) {
		return time.Time{}, time.Time{}, false, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "timezone", Message: "civil day bounds did not advance"}
	}
	return start, end, true, nil
}
