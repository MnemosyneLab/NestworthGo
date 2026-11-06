package einoopenai

import (
	"strings"
	"testing"
)

func TestFullInstructionsUseAdvertisedAnalysisAuthority(t *testing.T) {
	full, read := modelInstructions(true), modelInstructions(false)
	for _, claim := range []string{"strictly read-only closed-period attribution", "max 365 days, household or one account, base valuation"} {
		if strings.Contains(full, claim) || !strings.Contains(read, claim) {
			t.Fatalf("mode policy mixed: %s", claim)
		}
	}
	for _, claim := range []string{"Their schemas and descriptions determine supported scope", "Analysis tools may maintain derived projections", "No extra per-write approval is required"} {
		if !strings.Contains(full, claim) {
			t.Fatalf("Full authority missing: %s", claim)
		}
	}
}
