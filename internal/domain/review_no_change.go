package domain

// Internal confirmation metadata; financial evidence and lifecycle rows are
// retained verbatim. No new wire fields or financial algorithm are introduced.
type ReviewNoChangeResult struct {
	RuleInstrumentCount int
	LatestClosedOn      string
	SourceFingerprint   string
}
