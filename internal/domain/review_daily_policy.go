package domain

import "time"

// ReviewTruth is evidence, not a display status. Its zero value deliberately
// means unknown so an absent historical day cannot contribute a hit.
type ReviewTruth string

const (
	ReviewUnknown ReviewTruth = ""
	ReviewTrue    ReviewTruth = "true"
	ReviewFalse   ReviewTruth = "false"
)

func (v ReviewTruth) valid() bool { return v == ReviewUnknown || v == ReviewTrue || v == ReviewFalse }

// ReviewAll and ReviewAny implement three-valued logic. The historical
// positive-quantity gate belongs outside the user's predicate: callers must
// combine it with ReviewAll, even when the user selects an any group.
func ReviewAll(values ...ReviewTruth) ReviewTruth {
	result := ReviewTrue
	for _, v := range values {
		if v == ReviewFalse {
			return ReviewFalse
		}
		if v != ReviewTrue {
			result = ReviewUnknown
		}
	}
	return result
}

func ReviewAny(values ...ReviewTruth) ReviewTruth {
	result := ReviewFalse
	for _, v := range values {
		if v == ReviewTrue {
			return ReviewTrue
		}
		if v != ReviewFalse {
			result = ReviewUnknown
		}
	}
	return result
}

// ReviewTriggerPolicy is separate from the metric's lookback window.
// Normalize rejects parameters belonging to a different policy kind.
type ReviewTriggerPolicy struct {
	Kind       string `json:"kind"`
	Count      int    `json:"count,omitempty"`
	WindowDays int    `json:"windowDays,omitempty"`
	MinHits    int    `json:"minHits,omitempty"`
}

func (p ReviewTriggerPolicy) Normalize() (ReviewTriggerPolicy, error) {
	valid := false
	switch p.Kind {
	case "single":
		valid = p.Count == 0 && p.WindowDays == 0 && p.MinHits == 0
	case "consecutive":
		valid = p.Count >= 1 && p.Count <= 365 && p.WindowDays == 0 && p.MinHits == 0
		if valid && p.Count == 1 {
			return ReviewTriggerPolicy{Kind: "single"}, nil
		}
	case "rolling":
		valid = p.Count == 0 && p.WindowDays >= 1 && p.WindowDays <= 365 && p.MinHits >= 1 && p.MinHits <= p.WindowDays
		if valid && p.WindowDays == 1 {
			return ReviewTriggerPolicy{Kind: "single"}, nil
		}
	}
	if !valid {
		return ReviewTriggerPolicy{}, validation("triggerPolicy", "use single, consecutive(1–365), or rolling(1–365 days, 1–windowDays hits) without unrelated parameters")
	}
	return p, nil
}

type ReviewPolicyWindow struct {
	Truth            ReviewTruth `json:"truth"`
	WindowStart      string      `json:"windowStart"`
	WindowEnd        string      `json:"windowEnd"`
	HitCount         int         `json:"hitCount"`
	FalseCount       int         `json:"falseCount"`
	UnknownCount     int         `json:"unknownCount"`
	ConsecutiveCount int         `json:"consecutiveCount"`
}

// EvaluateReviewPolicy consumes a date-keyed projection, never a sequence of
// polling results. latestClosedDay must be derived from the rule's fixed
// timezone, not the latest available quote. Missing dates count as unknown.
// H/F/U counts belong to the policy window. ConsecutiveCount spans the full
// trailing true suffix of known dates; today's preview is never examined.
func EvaluateReviewPolicy(p ReviewTriggerPolicy, latestClosedDay string, days map[string]ReviewTruth) (ReviewPolicyWindow, error) {
	p, err := p.Normalize()
	if err != nil {
		return ReviewPolicyWindow{}, err
	}
	end, err := time.Parse(time.DateOnly, latestClosedDay)
	if err != nil {
		return ReviewPolicyWindow{}, validation("latestClosedDay", "must be a valid ISO civil date")
	}
	width, required := 1, 1
	if p.Kind == "consecutive" {
		width, required = p.Count, p.Count
	} else if p.Kind == "rolling" {
		width, required = p.WindowDays, p.MinHits
	}
	result := ReviewPolicyWindow{WindowStart: end.AddDate(0, 0, 1-width).Format(time.DateOnly), WindowEnd: latestClosedDay}
	suffix := true
	for offset := 0; offset < width; offset++ {
		day := end.AddDate(0, 0, -offset).Format(time.DateOnly)
		v := days[day]
		if !v.valid() {
			return ReviewPolicyWindow{}, validation("dailyPredicate", "contains an unsupported truth value")
		}
		switch v {
		case ReviewTrue:
			result.HitCount++
			if suffix {
				result.ConsecutiveCount++
			}
		case ReviewFalse:
			result.FalseCount++
			suffix = false
		default:
			result.UnknownCount++
			suffix = false
		}
	}
	// Window H/F/U counts belong to this policy's fixed window. The display
	// streak is independent: when the entire window ends in T, continue over
	// earlier calendar dates until the first F, U, or absent day.
	if result.ConsecutiveCount == width {
		for offset := width; ; offset++ {
			day := end.AddDate(0, 0, -offset).Format(time.DateOnly)
			v := days[day]
			if !v.valid() {
				return ReviewPolicyWindow{}, validation("dailyPredicate", "contains an unsupported truth value")
			}
			if v != ReviewTrue {
				break
			}
			result.ConsecutiveCount++
		}
	}
	switch {
	case result.HitCount >= required:
		result.Truth = ReviewTrue
	case result.HitCount+result.UnknownCount < required:
		result.Truth = ReviewFalse
	default:
		result.Truth = ReviewUnknown
	}
	return result, nil
}
