package sqlite

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"time"
)

// A full namespace is partitioned exactly once into authenticated retained
// output and emitted days. No count-only proof, duplicate, overlap or gap.
func validateReviewSuffixPublication(c domain.ReviewProjectionCapture, rules []domain.ReviewRulePublication) (string, error) {
	if c.Suffix == nil {
		return "", nil
	}
	retained := map[domain.ReviewDailyEvidenceKey]bool{}
	for _, p := range c.Suffix.Prefix {
		if !p.OutputAttested {
			continue
		}
		if retained[p.Key] {
			return "", reviewRuleConflict("duplicate attested prefix")
		}
		retained[p.Key] = true
	}
	if len(retained) == 0 {
		return "", nil
	}
	if c.PublishedTemporal == nil || c.PublishedTemporal.Source == nil || c.SourceRevision == nil || c.PublishedTemporal.Source.Token != c.SourceRevision.Token || c.HistoricalFingerprint != c.Control.PublishedFingerprint {
		return "", reviewRuleConflict("output omission requires unchanged complete source")
	}
	type scope struct {
		rule       domain.ReviewRuleID
		instrument domain.InstrumentID
	}
	expected := map[scope]bool{} // true means the full path emits daily output
	for _, rule := range c.Rules {
		eligible := map[domain.InstrumentID]bool{}
		if rule.ArchivedAt == nil && rule.Spec.Target.Kind == "instruments" {
			for _, id := range rule.Spec.Target.InstrumentIDs {
				eligible[id] = true
			}
		} else if rule.ArchivedAt == nil {
			for _, h := range c.Historical.History.Portfolio.Holdings {
				eligible[h.InstrumentID] = true
			}
		}
		for _, id := range reviewWindowInstruments(c, rule) {
			expected[scope{rule.ID, id}] = eligible[id]
		}
	}
	staged := map[scope]bool{}
	minimum := "9999-12-31"
	for _, r := range rules {
		pair := scope{r.RuleID, r.InstrumentID}
		eligible, known := expected[pair]
		if !known || r.TargetExcluded == eligible {
			return "", reviewRuleConflict("suffix target scope changed")
		}

		if staged[pair] {
			return "", reviewRuleConflict("duplicate suffix publication scope")
		}
		staged[pair] = true
		dates := map[string]bool{}
		for _, d := range r.Days {
			key := domain.ReviewDailyEvidenceKey{RuleID: r.RuleID, ConditionVersion: r.ConditionVersion, InstrumentID: r.InstrumentID, TimezoneVersion: r.TimezoneVersion, On: d.On}
			if dates[d.On] || retained[key] {
				return "", reviewRuleConflict("duplicate or overlapping suffix day")
			}
			dates[d.On] = true
			if d.On < minimum {
				minimum = d.On
			}
		}
		if r.TargetExcluded {
			if len(dates) != 0 {
				return "", reviewRuleConflict("excluded suffix emitted days")
			}
			continue
		}
		var spec *domain.ReviewRule
		for i := range c.Rules {
			if c.Rules[i].ID == r.RuleID {
				spec = &c.Rules[i]
				break
			}
		}
		if spec == nil {
			return "", reviewRuleConflict("unknown suffix rule")
		}
		loc, err := domain.LoadFinancialLocation(spec.Spec.Schedule.Timezone)
		if err != nil {
			return "", err
		}
		start := c.Historical.History.Origin.StartedAt.In(loc).Format(time.DateOnly)
		day, err := time.Parse(time.DateOnly, start)
		if err != nil {
			return "", err
		}
		for ; day.Format(time.DateOnly) <= r.LatestClosedOn; day = day.AddDate(0, 0, 1) {
			on := day.Format(time.DateOnly)
			key := domain.ReviewDailyEvidenceKey{RuleID: r.RuleID, ConditionVersion: r.ConditionVersion, InstrumentID: r.InstrumentID, TimezoneVersion: r.TimezoneVersion, On: on}
			if retained[key] {
				delete(retained, key)
			} else if dates[on] {
				delete(dates, on)
			} else {
				return "", reviewRuleConflict("missing suffix publication day")
			}
		}
		if len(dates) != 0 {
			return "", reviewRuleConflict("suffix day outside expected range")
		}
	}
	for _, rule := range c.Rules {
		for _, id := range reviewWindowInstruments(c, rule) {
			if !staged[scope{rule.ID, id}] {
				return "", reviewRuleConflict("missing suffix publication scope")
			}
		}
	}
	// Excluded pairs have no daily output in the full publication either.
	for key := range retained {
		excluded := false
		for _, r := range rules {
			if r.RuleID == key.RuleID && r.InstrumentID == key.InstrumentID && r.TargetExcluded {
				excluded = true
				break
			}
		}
		if !excluded {
			return "", reviewRuleConflict("unused attested suffix key")
		}
	}
	return minimum, nil
}
