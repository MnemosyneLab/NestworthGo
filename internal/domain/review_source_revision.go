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
