package domain

// Only a publication prepared without reusable daily evidence can establish
// this version. Earlier C3 manifests could have certified incomplete history.
// Internal confirmation metadata; financial evidence and lifecycle rows are
// retained verbatim. No new wire fields or financial algorithm are introduced.
type ReviewNoChangeResult struct {
	RuleInstrumentCount  int
	LatestClosedOn       string
	SourceFingerprint    string
	RequireFullRecompute bool
}
