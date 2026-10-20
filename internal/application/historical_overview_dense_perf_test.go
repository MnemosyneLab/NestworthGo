//go:build perf

package application

import "testing"

func TestHistoricalOverviewDenseFullScale(t *testing.T) {
	if analysisRaceDetector {
		t.Skip("full scale is a non-race runtime check; race instrumentation exhausted the shard")
	}
	runHistoricalOverviewDenseScale(t, 100, 1000, 25000, 86433, 1000, 1100, "0.108")
}
