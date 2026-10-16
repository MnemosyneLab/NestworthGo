package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewHistoricalBatchReader interface {
	LoadReviewHistoricalBatch(context.Context, domain.HouseholdID, time.Time) (domain.ReviewHistoricalBatch, error)
}

type ReviewPreviewDay struct {
	On       string                   `json:"on"`
	Result   domain.ReviewDailyResult `json:"result"`
	Coverage ReviewHistoricalCoverage `json:"coverage"`
}

type ReviewRulePreview struct {
	SourceToken    string                    `json:"sourceToken"`
	LatestClosedOn string                    `json:"latestClosedOn"`
	Window         domain.ReviewPolicyWindow `json:"window"`
	Days           []ReviewPreviewDay        `json:"days"`
}

// PreviewReviewRuleV2 evaluates a candidate rule over real civil-day slots.
// It only reads one consistent financial snapshot; it never creates alerts,
// daily result rows, episodes, or valuation snapshots.
func (s *Service) PreviewReviewRuleV2(ctx context.Context, spec domain.ReviewRuleSpec, instrumentIDText, latestClosedOn, expectedSourceToken string) (ReviewRulePreview, error) {
	var empty ReviewRulePreview
	reader, ok := s.repository.(ReviewHistoricalBatchReader)
	if !ok {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "historical review preview is unavailable"}
	}
	var err error
	spec, err = spec.Normalize()
	if err != nil {
		return empty, err
	}
	instrumentID, err := domain.ParseInstrumentID(instrumentIDText)
	if err != nil {
		return empty, err
	}
	if spec.Target.Kind == "instruments" {
		selected := false
		for _, id := range spec.Target.InstrumentIDs {
			selected = selected || id == instrumentID
		}
		if !selected {
			return empty, &domain.Error{Code: domain.ErrValidation, Field: "instrumentId", Message: "instrument is outside the rule target"}
		}
	}
	latestAvailable, err := domain.ReviewLatestClosedDay(s.clock(), spec.Schedule.Timezone)
	if err != nil {
		return empty, err
	}
	if latestClosedOn == "" {
		latestClosedOn = latestAvailable
	}
	if _, err := time.Parse(time.DateOnly, latestClosedOn); err != nil || latestClosedOn > latestAvailable {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "latestClosedOn", Message: "day must be closed in the rule timezone"}
	}
	household, err := s.repository.Household(ctx)
	if err != nil {
		return empty, err
	}
	if household == nil {
		return empty, &domain.Error{Code: domain.ErrUnavailable, Message: "household is unavailable"}
	}
	cutoff, err := domain.ReviewCivilCutoff(latestClosedOn, spec.Schedule.Timezone)
	if err != nil {
		return empty, err
	}
	batch, err := reader.LoadReviewHistoricalBatch(ctx, household.ID, cutoff)
	if err != nil {
		return empty, err
	}
	token, err := domain.ReviewHistoricalFingerprint(batch)
	if err != nil {
		return empty, err
	}
	if expectedSourceToken != "" && expectedSourceToken != token {
		return empty, &domain.Error{Code: domain.ErrConflict, Field: "expectedSourceToken", Message: "review evidence changed; preview again"}
	}
	width := 1
	switch spec.TriggerPolicy.Kind {
	case "consecutive":
		width = spec.TriggerPolicy.Count
	case "rolling":
		width = spec.TriggerPolicy.WindowDays
	}
	end, _ := time.Parse(time.DateOnly, latestClosedOn)
	result := ReviewRulePreview{SourceToken: token, LatestClosedOn: latestClosedOn, Days: make([]ReviewPreviewDay, 0, width)}
	truths := make(map[string]domain.ReviewTruth, width)
	for offset := width - 1; offset >= 0; offset-- {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		on := end.AddDate(0, 0, -offset).Format(time.DateOnly)
		input, coverage, err := ProjectReviewHistoricalDay(ctx, batch, s.FXProviderKey(), spec, instrumentID, on)
		if err != nil {
			return empty, err
		}
		daily, err := domain.EvaluateReviewDaily(spec, input)
		if err != nil {
			return empty, err
		}
		truths[on] = daily.Truth
		result.Days = append(result.Days, ReviewPreviewDay{On: on, Result: daily, Coverage: coverage})
	}
	result.Window, err = domain.EvaluateReviewPolicy(spec.TriggerPolicy, latestClosedOn, truths)
	return result, err
}
