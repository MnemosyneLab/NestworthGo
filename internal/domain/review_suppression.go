package domain

import "time"

// ReviewSuppression is independent from any episode or acknowledgement. An
// all-rule suppression also applies to rules created after it was set.
type ReviewSuppression struct {
	ID           string
	HouseholdID  HouseholdID
	InstrumentID InstrumentID
	RuleID       string // empty means this instrument and every rule
	Mode         string // permanent or until_date
	UntilOn      string // first unsuppressed civil day in Timezone
	Timezone     string
	Revision     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ReviewSuppressionMutation struct {
	Operation        string // set or clear
	HouseholdID      HouseholdID
	SuppressionID    ReviewSuppressionID
	MutationID       MutationID
	InstrumentID     InstrumentID
	RuleID           ReviewRuleID // empty for all rules
	Mode             string
	UntilOn          string
	Timezone         string
	RelativeDays     int
	CustomUntil      string
	ExpectedRevision int
	At               time.Time
}

// ReviewSuppressionUntil calculates the committed resume day once. Retries
// must reuse the receipt's value instead of extending a duration from now.
func ReviewSuppressionUntil(now time.Time, timezone string, days int, customUntil string) (string, error) {
	if now.IsZero() {
		return "", validation("now", "an operation time is required")
	}
	location, err := LoadFinancialLocation(timezone)
	if err != nil || timezone == "Local" {
		return "", validation("timezone", "a fixed IANA timezone is required")
	}
	if days == 0 && customUntil == "" {
		return "", nil // permanent
	}
	if (days != 7 && days != 30) || customUntil != "" {
		if days != 0 || customUntil == "" {
			return "", validation("untilOn", "choose permanent, 7/30 days, or one future date")
		}
	}
	local := now.In(location)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	until := customUntil
	if days != 0 {
		until = today.AddDate(0, 0, days).Format(time.DateOnly)
	}
	if _, err := ReviewCivilCutoff(until, timezone); err != nil {
		return "", err // skipped civil date must not silently normalize
	}
	if until <= today.Format(time.DateOnly) {
		return "", validation("untilOn", "resume day must be in the future")
	}
	return until, nil
}

// ActiveReviewSuppression picks the strongest matching active suppression for
// explanation. A permanent entry takes precedence, then the latest temporary
// resume day. The scope does not change an episode's acknowledgement.
func ActiveReviewSuppression(entries []ReviewSuppression, instrumentID InstrumentID, ruleID string, now time.Time) (*ReviewSuppression, error) {
	if now.IsZero() {
		return nil, validation("now", "an evaluation time is required")
	}
	var chosen *ReviewSuppression
	for index := range entries {
		item := &entries[index]
		if item.InstrumentID != instrumentID || (item.RuleID != "" && item.RuleID != ruleID) {
			continue
		}
		location, err := LoadFinancialLocation(item.Timezone)
		if err != nil || item.Timezone == "Local" {
			return nil, validation("timezone", "suppression has an invalid IANA timezone")
		}
		active := item.Mode == "permanent" && item.UntilOn == ""
		if item.Mode == "until_date" {
			if _, err := ReviewCivilCutoff(item.UntilOn, item.Timezone); err != nil {
				return nil, err
			}
			active = now.In(location).Format(time.DateOnly) < item.UntilOn
		} else if item.Mode != "permanent" || item.UntilOn != "" {
			return nil, validation("suppression", "invalid suppression mode or resume date")
		}
		if !active {
			continue
		}
		if chosen == nil || (item.Mode == "permanent" && chosen.Mode != "permanent") || (item.Mode == chosen.Mode && item.UntilOn > chosen.UntilOn) {
			chosen = item
		}
	}
	return chosen, nil
}

func ReviewEpisodeActionable(masterEnabled, ruleEnabled, currentHeld, acknowledged bool, policyState ReviewTruth, suppression *ReviewSuppression) bool {
	return masterEnabled && ruleEnabled && currentHeld && !acknowledged && suppression == nil && policyState == ReviewTrue
}
