package mcpserver

import "testing"

func TestProductSchemaConstraintsAndDefaults(t *testing.T) {
	resolved, err := productOperationInputSchema().Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		edit  func(map[string]any)
		valid bool
	}{
		{"defaults", func(p map[string]any) {
			policy := p["policy"].(map[string]any)
			policy["accessKind"] = ""
			policy["earlyKind"] = ""
			policy["unlockOn"] = nil
		}, true},
		{"empty local defaults", func(p map[string]any) {
			p["effectiveAt"] = "2026-09-01T12:00:00Z"
			p["effectiveLocalDate"] = ""
			p["effectiveLocalTime"] = ""
		}, true},
		{"local pair", func(p map[string]any) { p["effectiveLocalDate"] = "2026-09-01"; p["effectiveLocalTime"] = "12:00" }, true},
		{"conflicting time", func(p map[string]any) {
			p["effectiveAt"] = "2026-09-01T12:00:00Z"
			p["effectiveLocalDate"] = "2026-09-01"
			p["effectiveLocalTime"] = "12:00"
		}, false},
		{"missing local clock", func(p map[string]any) { p["effectiveLocalDate"] = "2026-09-01" }, false},
		{"empty local clock", func(p map[string]any) { p["effectiveLocalDate"] = "2026-09-01"; p["effectiveLocalTime"] = "" }, false},
		{"interest enum", func(p map[string]any) { p["terms"].(map[string]any)["interestMode"] = "invented" }, false},
		{"maturity required", func(p map[string]any) { p["terms"].(map[string]any)["maturityOn"] = nil }, false},
		{"policy enum", func(p map[string]any) { p["policy"].(map[string]any)["accessKind"] = "invented" }, false},
		{"settlement maximum", func(p map[string]any) { p["policy"].(map[string]any)["settlementDays"] = 366 }, false},
		{"settlement minimum", func(p map[string]any) { p["policy"].(map[string]any)["settlementDays"] = -1 }, false},
		{"settlement basis", func(p map[string]any) { p["policy"].(map[string]any)["dayBasis"] = nil }, false},
		{"early requirements", func(p map[string]any) { p["policy"].(map[string]any)["earlyKind"] = "allowed" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := productPayload("open")
			tc.edit(p)
			input := map[string]any{"kind": "open", "open": p}
			err := resolved.Validate(&input)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}
