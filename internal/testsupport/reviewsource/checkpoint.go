// Package reviewsource contains independent source-checkpoint test oracles.
// Application/runtime packages must not import test support.
package reviewsource

import "github.com/waltwang/nestworth-go/internal/domain"

type Verdict string

const (
	Unavailable   Verdict = "unavailable"
	Same          Verdict = "same"
	Changed       Verdict = "source_changed"
	Conservative  Verdict = "conservative_invalidation"
	Untracked     Verdict = "untracked_change"
	CutoffChanged Verdict = "cutoff_changed"
	ClockRollback Verdict = "clock_rollback"
)

// Contains no financial payload, settings, fingerprint/hash or credential. It
// never authorizes reuse: even Same is only a fixed-cutoff shadow observation.
type Observation struct {
	Verdict           Verdict
	SameToken         bool
	SameFingerprint   bool
	SameCutoff        bool
	BelowJournalFloor bool
}

func Compare(before, after domain.ReviewProjectionCapture) Observation {
	o := Observation{Verdict: Unavailable, SameFingerprint: before.HistoricalFingerprint == after.HistoricalFingerprint, SameCutoff: before.SourceCutoff.Equal(after.SourceCutoff)}
	if before.SourceRevision == nil || after.SourceRevision == nil || !before.SourceRevision.Valid() || !after.SourceRevision.Valid() || before.HistoricalFingerprint == "" || after.HistoricalFingerprint == "" || before.SourceCutoff.IsZero() || after.SourceCutoff.IsZero() {
		return o
	}
	o.SameToken = before.SourceRevision.Token == after.SourceRevision.Token
	o.BelowJournalFloor = before.SourceRevision.Token.HouseholdID == after.SourceRevision.Token.HouseholdID && before.SourceRevision.Token.Epoch == after.SourceRevision.Token.Epoch && before.SourceRevision.Token.Revision < after.SourceRevision.JournalFloor
	switch {
	case after.SourceCutoff.Before(before.SourceCutoff):
		o.Verdict = ClockRollback
	case !o.SameCutoff:
		o.Verdict = CutoffChanged
	case o.SameToken && !o.SameFingerprint:
		o.Verdict = Untracked
	case !o.SameToken && o.SameFingerprint:
		o.Verdict = Conservative
	case !o.SameToken:
		o.Verdict = Changed
	default:
		o.Verdict = Same
	}
	return o
}
