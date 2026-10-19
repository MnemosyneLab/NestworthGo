package domain

import "strings"

// SourceRevisionToken describes financial DML, not the effective-time frontier,
// review rules/handling, or a certificate permitting a no-op. C2 only compares
// it alongside the complete authoritative fingerprint at one fixed cutoff.
type SourceRevisionToken struct {
	HouseholdID           HouseholdID
	Epoch                 string
	Revision              int64
	TrackingVersion       string
	ResolverPolicyVersion string
	EvaluatorVersion      string
	MetricPolicyVersion   string
}

func (t SourceRevisionToken) Valid() bool {
	return t.HouseholdID != "" && t.Revision >= 0 && len(t.Epoch) == 32 &&
		strings.Trim(t.Epoch, "0123456789abcdef") == "" && t.TrackingVersion != "" &&
		t.ResolverPolicyVersion != "" && t.EvaluatorVersion != "" && t.MetricPolicyVersion != ""
}

// Floor is deliberately separate from token identity: pruning changes the
// available invalidation interval without changing any financial source fact.
type ReviewSourceCheckpoint struct {
	Token        SourceRevisionToken
	JournalFloor int64
}

func (c ReviewSourceCheckpoint) Valid() bool {
	return c.Token.Valid() && c.JournalFloor >= 0 && c.JournalFloor <= c.Token.Revision
}

type ReviewSourceShadowVerdict string

const (
	ReviewSourceUnavailable   ReviewSourceShadowVerdict = "unavailable"
	ReviewSourceSame          ReviewSourceShadowVerdict = "same"
	ReviewSourceChanged       ReviewSourceShadowVerdict = "source_changed"
	ReviewSourceConservative  ReviewSourceShadowVerdict = "conservative_invalidation"
	ReviewSourceUntracked     ReviewSourceShadowVerdict = "untracked_change"
	ReviewSourceCutoffChanged ReviewSourceShadowVerdict = "cutoff_changed"
	ReviewSourceClockRollback ReviewSourceShadowVerdict = "clock_rollback"
)

// Contains no financial payload, settings, fingerprint/hash or credential. It
// never authorizes reuse: even Same is only a fixed-cutoff shadow observation.
type ReviewSourceShadowObservation struct {
	Verdict           ReviewSourceShadowVerdict
	SameToken         bool
	SameFingerprint   bool
	SameCutoff        bool
	BelowJournalFloor bool
}

func CompareReviewSourceCheckpoints(before, after ReviewProjectionCapture) ReviewSourceShadowObservation {
	o := ReviewSourceShadowObservation{Verdict: ReviewSourceUnavailable, SameFingerprint: before.HistoricalFingerprint == after.HistoricalFingerprint, SameCutoff: before.SourceCutoff.Equal(after.SourceCutoff)}
	if before.SourceRevision == nil || after.SourceRevision == nil || !before.SourceRevision.Valid() || !after.SourceRevision.Valid() || before.HistoricalFingerprint == "" || after.HistoricalFingerprint == "" || before.SourceCutoff.IsZero() || after.SourceCutoff.IsZero() {
		return o
	}
	o.SameToken = before.SourceRevision.Token == after.SourceRevision.Token
	o.BelowJournalFloor = before.SourceRevision.Token.HouseholdID == after.SourceRevision.Token.HouseholdID && before.SourceRevision.Token.Epoch == after.SourceRevision.Token.Epoch && before.SourceRevision.Token.Revision < after.SourceRevision.JournalFloor
	switch {
	case after.SourceCutoff.Before(before.SourceCutoff):
		o.Verdict = ReviewSourceClockRollback
	case !o.SameCutoff:
		o.Verdict = ReviewSourceCutoffChanged
	case o.SameToken && !o.SameFingerprint:
		o.Verdict = ReviewSourceUntracked
	case !o.SameToken && o.SameFingerprint:
		o.Verdict = ReviewSourceConservative
	case !o.SameToken:
		o.Verdict = ReviewSourceChanged
	default:
		o.Verdict = ReviewSourceSame
	}
	return o
}
