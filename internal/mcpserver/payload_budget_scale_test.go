//go:build perf

package mcpserver

import "testing"

func TestMCPPayloadBudgetsLarge(t *testing.T) { runPayloadBudgets(t, true) }
