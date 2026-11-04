//go:build perf

package mcpserver

import "testing"

func TestPushdownRequestLargeWireEvidence(t *testing.T) { testPushdownRequestEvidence(t, 4960) }
