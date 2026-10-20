//go:build perf

package application

import (
	"testing"
)

func TestReviewRepeatedEpochWindowOracleScale(t *testing.T) {
	checkReviewEpochReplayOracle(t, reviewPerfConfig{Accounts: 1, Instruments: 1, Holdings: 1, Rules: 4, HistoryDays: 10, Quotes: 10}, 8)
}
