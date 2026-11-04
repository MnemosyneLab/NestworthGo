//go:build perf

package sqlite

import "testing"

func TestTargetReadPushdownLargeSynthetic(t *testing.T) { testPushdownScale(t, 5000) }
