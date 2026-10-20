//go:build perf

package application

import "testing"

// The exhaustive safety cross-product stays out of ordinary PR checks.
func TestReviewDependencySuffixExtendedOracle(t *testing.T) {
	testReviewDependencySuffixFullDurableOracle(t, true)
}
