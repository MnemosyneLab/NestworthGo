//go:build perf

package application

import "testing"

// Retains the complete epoch setup/oracles and instruments the actual Service
// call, not a reimplementation of reconciliation.
func TestReviewEpochStageDiagnostics(t *testing.T) {
	t.Setenv("NW_REVIEW_PERF_STAGE_TRACE", "1")
	runReviewPerfEpochPairs(t, reviewPerfConfig{Accounts: 1, Instruments: 1, Holdings: 1, Rules: 4, HistoryDays: 30, Quotes: 30}, 30, 64, "no-op")
}
