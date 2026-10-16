package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewPlannedDay struct {
	On               string
	Input            domain.ReviewDailyInput
	Coverage         ReviewHistoricalCoverage
	Result           domain.ReviewDailyResult
	PolicyWindow     domain.ReviewPolicyWindow
	InputFingerprint string
}

type ReviewRuleInstrumentPlan struct {
	RuleID            domain.ReviewRuleID
	ConditionVersion  int
	InstrumentID      domain.InstrumentID
	LatestClosedOn    string
	Days              []ReviewPlannedDay
	LatestWindow      domain.ReviewPolicyWindow
	Position          ReviewPositionTimeline
	EpochIDs          []string
	ActiveSuppression *domain.ReviewSuppression
}

// PlanReviewRuleInstrument replays every reliable civil day so that a later
// correction can prove a hidden recovery. It computes from a captured batch
// only; publication and episode mutation are separate operations.
func PlanReviewRuleInstrument(ctx context.Context, capture domain.ReviewProjectionCapture, rule domain.ReviewRule, instrumentID domain.InstrumentID, now time.Time, providerKey string) (ReviewRuleInstrumentPlan, error) {
	var empty ReviewRuleInstrumentPlan
	if rule.HouseholdID != capture.HouseholdID || rule.Evaluator != domain.ReviewEvaluator {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "rule", Message: "rule does not belong to this daily review capture"}
	}
	spec, err := rule.Spec.Normalize()
	if err != nil {
		return empty, err
	}
	latest, err := domain.ReviewLatestClosedDay(now, spec.Schedule.Timezone)
	if err != nil {
		return empty, err
	}
	result := ReviewRuleInstrumentPlan{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: instrumentID, LatestClosedOn: latest, Days: []ReviewPlannedDay{}, EpochIDs: []string{}}
	end, _ := time.Parse(time.DateOnly, latest)
	location, _ := time.LoadLocation(spec.Schedule.Timezone)
	origin := capture.Historical.History.Origin
	start, err := time.Parse(time.DateOnly, origin.StartedAt.In(location).Format(time.DateOnly))
	if err != nil {
		return empty, err
	}
	truths := map[string]domain.ReviewTruth{}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		on := day.Format(time.DateOnly)
		input, coverage, daily, err := evaluateReviewHistoricalDate(ctx, capture.Historical, providerKey, spec, instrumentID, on)
		if err != nil {
			return empty, err
		}
		truths[on] = daily.Truth
		window, err := domain.EvaluateReviewPolicy(spec.TriggerPolicy, on, truths)
		if err != nil {
			return empty, err
		}
		fingerprint, err := reviewDailyInputFingerprint(input, coverage)
		if err != nil {
			return empty, err
		}
		result.Days = append(result.Days, ReviewPlannedDay{On: on, Input: input, Coverage: coverage, Result: daily, PolicyWindow: window, InputFingerprint: fingerprint})
	}
	result.LatestWindow, err = domain.EvaluateReviewPolicy(spec.TriggerPolicy, latest, truths)
	if err != nil {
		return empty, err
	}
	cutoff, err := reviewClosedDayBoundary(latest, spec.Schedule.Timezone)
	if err != nil {
		return empty, err
	}
	result.Position, err = ProjectReviewPositionTimeline(ctx, capture.Historical.History, instrumentID, cutoff)
	if err != nil {
		return empty, err
	}
	for _, epoch := range result.Position.Epochs {
		id, err := ReviewPositionEpochID(capture.HouseholdID, instrumentID, origin.ID, epoch)
		if err != nil {
			return empty, err
		}
		result.EpochIDs = append(result.EpochIDs, id)
	}
	result.ActiveSuppression, err = domain.ActiveReviewSuppression(capture.Suppressions, instrumentID, string(rule.ID), now)
	return result, err
}

func reviewDailyInputFingerprint(input domain.ReviewDailyInput, coverage ReviewHistoricalCoverage) (string, error) {
	raw, err := json.Marshal(struct {
		Input    domain.ReviewDailyInput
		Coverage ReviewHistoricalCoverage
	}{input, coverage})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
