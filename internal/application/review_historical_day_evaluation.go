package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// evaluateReviewHistoricalDate gives a skipped IANA civil date one explicit U
// slot. It does not turn unrelated replay or timezone errors into U.
func evaluateReviewHistoricalDate(ctx context.Context, batch domain.ReviewHistoricalBatch, providerKey string, spec domain.ReviewRuleSpec, instrumentID domain.InstrumentID, on string) (domain.ReviewDailyInput, ReviewHistoricalCoverage, domain.ReviewDailyResult, error) {
	if _, err := domain.ReviewCivilCutoff(on, spec.Schedule.Timezone); err != nil {
		if !domain.ReviewNonexistentCivilDate(err) {
			return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, domain.ReviewDailyResult{}, err
		}
		return domain.ReviewDailyInput{On: on, InstrumentID: instrumentID}, ReviewHistoricalCoverage{On: on, Missing: []string{"nonexistent_local_date"}}, domain.ReviewDailyResult{On: on, Truth: domain.ReviewUnknown, Leaves: []domain.ReviewDailyLeafEvidence{}}, nil
	}
	input, coverage, err := ProjectReviewHistoricalDay(ctx, batch, providerKey, spec, instrumentID, on)
	if err != nil {
		return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, domain.ReviewDailyResult{}, err
	}
	result, err := domain.EvaluateReviewDaily(spec, input)
	return input, coverage, result, err
}

// reviewClosedDayBoundary returns the latest real day-end at or before on.
// Only a skipped IANA date may step back; malformed dates remain errors.
func reviewClosedDayBoundary(on, timezone string) (time.Time, error) {
	day, err := time.Parse(time.DateOnly, on)
	if err != nil {
		return time.Time{}, err
	}
	for skipped := 0; skipped <= 7; skipped++ {
		cutoff, err := domain.ReviewCivilCutoff(day.Format(time.DateOnly), timezone)
		if err == nil {
			return cutoff, nil
		}
		if !domain.ReviewNonexistentCivilDate(err) {
			return time.Time{}, err
		}
		day = day.AddDate(0, 0, -1)
	}
	return time.Time{}, &domain.Error{Code: domain.ErrUnavailable, Field: "timezone", Message: "no nearby real civil-day boundary"}
}
