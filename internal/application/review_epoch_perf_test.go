//go:build perf

package application

import (
	"testing"
)

func TestReviewRepeatedEpochWindowOracleScale(t *testing.T) {
	runReviewPerfEpochPairs(t, reviewPerfConfig{Accounts: 1, Instruments: 1, Holdings: 1, Rules: 4, HistoryDays: 10, Quotes: 10}, 1, 8)
}
