//go:build perf

package application

import (
	"os"
	"strconv"
	"testing"
)

// Distribution evidence: samples are independent clones, paths alternate
// order, and all correctness/oracle work occurs outside reported timings.
func TestReviewPairedPerformanceEvidence(t *testing.T) {
	samples := reviewPerfSampleCount(t, 3)
	cycles := 0
	if value := os.Getenv("NW_REVIEW_PERF_EPOCH_CYCLES"); value != "" {
		var err error
		cycles, err = strconv.Atoi(value)
		if err != nil || cycles < 0 {
			t.Fatal("invalid NW_REVIEW_PERF_EPOCH_CYCLES")
		}
	}
	runReviewPerfEpochPairs(t, reviewPerfEnvironmentConfig(t), samples, cycles)
}
