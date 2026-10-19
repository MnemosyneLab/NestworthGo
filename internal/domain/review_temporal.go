package domain

import (
	"sync"
	"time"
)

const ReviewTemporalPolicyVersion = "c2-household-temporal-v1"
const ReviewTemporalDependencyScope = "household-all-review-inputs-v1"

// A finite interval is intentional: even with no future source events, each
// rule/origin/suppression calendar contributes its next date/offset boundary.
// This shadow certificate does not authorize skipping capture or planning.
type ReviewTemporalCertificate struct {
	EvaluatedAt      time.Time
	NextTransitionAt time.Time
	PolicyVersion    string
	DependencyScope  string
	ClockEpoch       string
	Source           *ReviewSourceCheckpoint
	ConfigRevision   int
	StateRevision    int
	RuleRevisions    map[ReviewRuleID]int
	// C3a proof is minted only by a full publication on an opt-in repository.
	// Older C2 checkpoints cannot authorize the candidate no-change path.
	NoChangeReviewFingerprint string `json:",omitempty"`
	NoChangeFXProviderKey     string `json:",omitempty"`
}

func (c *ReviewTemporalCertificate) Covers(now time.Time) bool {
	return c != nil && c.PolicyVersion == ReviewTemporalPolicyVersion && c.DependencyScope == ReviewTemporalDependencyScope && c.ClockEpoch != "" && !c.EvaluatedAt.IsZero() && c.NextTransitionAt.After(c.EvaluatedAt) && !now.Before(c.EvaluatedAt) && now.Before(c.NextTransitionAt)
}

// Eligible is a diagnostic only; callers still perform full source capture,
// fingerprints, planning and publication checks in C2.
func (c *ReviewTemporalCertificate) Eligible(now time.Time, current *ReviewTemporalCertificate) bool {
	if !c.Covers(now) || !current.Covers(now) || c.ClockEpoch != current.ClockEpoch || c.Source == nil || current.Source == nil || !c.Source.Valid() || !current.Source.Valid() || c.Source.Token != current.Source.Token || c.Source.Token.Revision < current.Source.JournalFloor || c.ConfigRevision != current.ConfigRevision || c.StateRevision != current.StateRevision || len(c.RuleRevisions) != len(current.RuleRevisions) {
		return false
	}
	for id, revision := range c.RuleRevisions {
		if current.RuleRevisions[id] != revision {
			return false
		}
	}
	return true
}

// Keep every timezone used by the review read closure immutable within one
// process. A new database session changes ClockEpoch, so a persisted certificate
// can never assume that another process loaded identical OS tzdata.
var financialLocations sync.Map

func LoadFinancialLocation(name string) (*time.Location, error) {
	if cached, ok := financialLocations.Load(name); ok {
		return cached.(*time.Location), nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	if name == "Local" {
		// Initialize Go's lazy local zone, then pin a copy. Legacy origins may
		// use Local; new review schedules already reject that name themselves.
		_, _ = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).In(location).Zone()
		snapshot := *location
		location = &snapshot
	}
	actual, _ := financialLocations.LoadOrStore(name, location)
	return actual.(*time.Location), nil
}

// Until the next offset transition the current offset is constant. Stop at
// that transition even when it is not midnight; this safely handles repeated
// and missing midnights and date-line skips without normalizing a wall time.
func ReviewNextCalendarBoundary(now time.Time, timezone string) (time.Time, error) {
	location, err := LoadFinancialLocation(timezone)
	if err != nil || now.IsZero() {
		return time.Time{}, validation("timezone", "a fixed available calendar and time are required")
	}
	local := now.In(location)
	_, offset := local.Zone()
	next := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, time.UTC).Add(-time.Duration(offset) * time.Second)
	_, zoneEnd := local.ZoneBounds()
	if !zoneEnd.IsZero() && zoneEnd.Before(next) {
		next = zoneEnd
	}
	if !next.After(now) {
		return time.Time{}, validation("timezone", "calendar interval did not advance")
	}
	return next.UTC(), nil
}
