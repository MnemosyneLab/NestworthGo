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
	RuleID           domain.ReviewRuleID
	ConditionVersion int
	InstrumentID     domain.InstrumentID
	LatestClosedOn   string
	Days             []ReviewPlannedDay
	LatestWindow     domain.ReviewPolicyWindow
	LatestResult     *domain.ReviewDailyResult
	LatestCoverage   []string
	Incremental      bool
	RecomputedDays   int
	// RetainedOutputDays is allocation accounting only, not publication authority.
	RetainedOutputDays int
	PreviousLatestOn   string
	Position           ReviewPositionTimeline
	EpochIDs           []string
	ActiveSuppression  *domain.ReviewSuppression
}

// PlanReviewRuleInstrument replays every reliable civil day so that a later
// correction can prove a hidden recovery. It computes from a captured batch
// only; publication and episode mutation are separate operations.
func PlanReviewRuleInstrument(ctx context.Context, capture domain.ReviewProjectionCapture, rule domain.ReviewRule, instrumentID domain.InstrumentID, now time.Time, providerKey string) (ReviewRuleInstrumentPlan, error) {
	return planReviewRuleInstrument(ctx, capture, rule, instrumentID, now, newReviewProjectionCache(capture, providerKey))
}

func planReviewRuleInstrument(ctx context.Context, capture domain.ReviewProjectionCapture, rule domain.ReviewRule, instrumentID domain.InstrumentID, now time.Time, cache *reviewProjectionCache) (ReviewRuleInstrumentPlan, error) {
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
	location, _ := domain.LoadFinancialLocation(spec.Schedule.Timezone)
	origin := capture.Historical.History.Origin
	start, err := time.Parse(time.DateOnly, origin.StartedAt.In(location).Format(time.DateOnly))
	if err != nil {
		return empty, err
	}
	timezoneVersion := spec.Schedule.Timezone + "/" + spec.Schedule.CutoffVersion
	if previous, reusable := cache.reusableLatestOn(rule, instrumentID, timezoneVersion, start.Format(time.DateOnly), latest, reviewPolicyWindowWidth(spec.TriggerPolicy)); reusable {
		return planReviewRuleInstrumentIncremental(ctx, capture, rule, spec, instrumentID, now, cache.providerKey, latest, start, previous, reviewPolicyWindowWidth(spec.TriggerPolicy), cache)
	}
	dailyResults := make([]domain.ReviewDailyResult, 0)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		on := day.Format(time.DateOnly)
		key := domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: instrumentID, TimezoneVersion: timezoneVersion, On: on}
		prefixIndex, reuse := cache.suffixEvidence[key]
		var cached domain.ReviewReusableDailyEvidence
		if reuse {
			cached = cache.capture.Suffix.Prefix[prefixIndex].Evidence
			if cache.capture.Suffix.Prefix[prefixIndex].OutputAttested {
				result.RetainedOutputDays++
			}
		}
		input, coverage, daily, fingerprint := cached.Input, ReviewHistoricalCoverage{On: on, Missing: cached.CoverageMissing}, cached.Result, cached.InputFingerprint
		if !reuse {
			input, coverage, daily, err = evaluateReviewHistoricalDateCached(ctx, cache, rule, spec, instrumentID, on)
			if err != nil {
				return empty, err
			}
			fingerprint, err = reviewDailyInputFingerprint(input, coverage)
			if err != nil {
				return empty, err
			}
			result.RecomputedDays++
		}
		result.Days = append(result.Days, ReviewPlannedDay{On: on, Input: input, Coverage: coverage, Result: daily, InputFingerprint: fingerprint})
		dailyResults = append(dailyResults, daily)
	}
	windows, err := domain.EvaluateReviewPolicyTimeline(spec.TriggerPolicy, dailyResults)
	if err != nil {
		return empty, err
	}
	for index := range result.Days {
		result.Days[index].PolicyWindow = windows[result.Days[index].On]
	}
	result.LatestWindow = windows[latest]
	if len(result.Days) > 0 {
		last := result.Days[len(result.Days)-1]
		latestResult := last.Result
		result.LatestResult = &latestResult
		result.LatestCoverage = append([]string{}, last.Coverage.Missing...)
	}
	// Position identity and current eligibility use authoritative effects up
	// to the capture time. Daily predicate votes above remain closed at L.
	// The replay helper has a half-open cutoff, while the captured SQL includes
	// effects effective exactly at now.
	result.Position, result.EpochIDs, err = cache.position(ctx, instrumentID, now.Add(time.Nanosecond))
	if err != nil {
		return empty, err
	}
	result.ActiveSuppression, err = domain.ActiveReviewSuppression(capture.Suppressions, instrumentID, string(rule.ID), now)
	return result, err
}

func reviewPolicyWindowWidth(policy domain.ReviewTriggerPolicy) int {
	switch policy.Kind {
	case "consecutive":
		return policy.Count
	case "rolling":
		return policy.WindowDays
	default:
		return 1
	}
}

func planReviewRuleInstrumentIncremental(ctx context.Context, capture domain.ReviewProjectionCapture, rule domain.ReviewRule, spec domain.ReviewRuleSpec, instrumentID domain.InstrumentID, now time.Time, providerKey string, latest string, historyStart time.Time, previousLatest string, width int, cache *reviewProjectionCache) (ReviewRuleInstrumentPlan, error) {
	result := ReviewRuleInstrumentPlan{
		RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: instrumentID,
		LatestClosedOn: latest, Days: []ReviewPlannedDay{}, EpochIDs: []string{},
		Incremental: true, PreviousLatestOn: previousLatest,
	}
	prior, _ := time.Parse(time.DateOnly, previousLatest)
	end, _ := time.Parse(time.DateOnly, latest)
	// The prior latest date is one day before the first new slot. Keep only
	// width-1 cached anchors plus every new slot so each emitted policy window
	// has exactly the same trailing date range as a full replay.
	first := prior.AddDate(0, 0, 1-width)
	if latest > previousLatest {
		first = prior.AddDate(0, 0, 2-width)
	}
	if first.Before(historyStart) {
		first = historyStart
	}
	dailyResults := make([]domain.ReviewDailyResult, 0, width+int(end.Sub(prior).Hours()/24))
	timezoneVersion := spec.Schedule.Timezone + "/" + spec.Schedule.CutoffVersion
	var latestResult *domain.ReviewDailyResult
	var latestCoverage []string
	for day := first; !day.After(end); day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return ReviewRuleInstrumentPlan{}, err
		}
		on := day.Format(time.DateOnly)
		input, coverage, daily, err := evaluateReviewHistoricalDateCached(ctx, cache, rule, spec, instrumentID, on)
		if err != nil {
			return ReviewRuleInstrumentPlan{}, err
		}
		fingerprint := ""
		key := domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: instrumentID, TimezoneVersion: timezoneVersion, On: on}
		if cached, ok := cache.capture.ReusableDailyEvidence[key]; ok && cached.HasResult && cached.Input.FXProviderKey == providerKey {
			fingerprint = cached.InputFingerprint
		} else {
			fingerprint, err = reviewDailyInputFingerprint(input, coverage)
			if err != nil {
				return ReviewRuleInstrumentPlan{}, err
			}
		}
		dailyResults = append(dailyResults, daily)
		if on == latest {
			copyOfLatest := daily
			latestResult = &copyOfLatest
			latestCoverage = append([]string{}, coverage.Missing...)
		}
		if on > previousLatest {
			result.Days = append(result.Days, ReviewPlannedDay{On: on, Input: input, Coverage: coverage, Result: daily, InputFingerprint: fingerprint})
		}
	}
	windows, err := domain.EvaluateReviewPolicyTimeline(spec.TriggerPolicy, dailyResults)
	if err != nil {
		return ReviewRuleInstrumentPlan{}, err
	}
	for index := range result.Days {
		result.Days[index].PolicyWindow = windows[result.Days[index].On]
	}
	result.LatestWindow = windows[latest]
	result.LatestResult = latestResult
	result.LatestCoverage = latestCoverage
	consecutiveCount := cache.reusableConsecutiveCount(rule, instrumentID, timezoneVersion, previousLatest)
	for index := range result.Days {
		day := &result.Days[index]
		if day.Result.Truth == domain.ReviewTrue {
			consecutiveCount++
		} else {
			consecutiveCount = 0
		}
		// The emitted daily window must carry the same complete streak as the
		// summary, including the prefix outside this bounded overlap.
		day.PolicyWindow.ConsecutiveCount = consecutiveCount
	}
	result.LatestWindow.ConsecutiveCount = consecutiveCount
	result.Position, result.EpochIDs, err = cache.position(ctx, instrumentID, now.Add(time.Nanosecond))
	if err != nil {
		return ReviewRuleInstrumentPlan{}, err
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
