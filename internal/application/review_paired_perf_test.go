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
	samples := 30
	if value := os.Getenv("NW_REVIEW_PERF_SAMPLES"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			t.Fatal("invalid NW_REVIEW_PERF_SAMPLES")
		}
		samples = parsed
	}
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
