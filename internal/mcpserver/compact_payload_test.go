package mcpserver

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Reuse the ordinary/perf household and recorder instead of preparing another
// fixture. The measurement walker accepts both shapes so the identical probe
// can also run in an isolated baseline checkout for paired wire evidence.
func measureCompactReadJourneys(t *testing.T, c *mcp.ClientSession, rec *payloadRecorder, f payloadHousehold) {
	t.Helper()
	for _, tool := range []string{"list_accounts", "list_instruments"} {
		section, id := "accounts", f.checking
		if tool == "list_instruments" {
			section, id = "instruments", f.instrument
		}
		start := rec.total()
		capture := scaleCapture(t, c, tool, map[string]any{"limit": 5})
		rows := scaleSection(t, c, capture, section)
		if len(rows) == 0 {
			t.Fatal("empty normal management journey")
		}
		// Older instrument lists had no full-item route; measure identification
		// separately, then measure same-capture detail when supported.
		rec.journey(tool+" identification all pages", start, 12, 1<<20)
		if tool == "list_accounts" || rows[0].(map[string]any)["note"] == nil {
			scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: capture["captureId"].(string), Section: section, ID: id})
			rec.journey(tool+" all pages plus one detail", start, 13, 1<<20)
		}
	}
	for _, tool := range []string{"get_financial_context", "compare_financial_context"} {
		start := rec.total()
		args := map[string]any{}
		pageTool, idKey := "get_financial_context_page", "contextId"
		if tool == "compare_financial_context" {
			args = map[string]any{"leftAsOf": f.timepoint, "rightAsOf": "current"}
			pageTool, idKey = "get_financial_comparison_page", "comparisonId"
		}
		initial := scaleCapture(t, c, tool, args)
		for _, section := range []string{"positions", "gaps", "evidence"} {
			info := initial[section+"Page"].(map[string]any)
			count := int(info["returned"].(float64))
			for info["hasMore"] == true {
				page := scaleCapture(t, c, pageTool, map[string]any{idKey: initial[idKey], "section": section, "cursor": info["nextCursor"]})
				if page["contentHash"] != initial["contentHash"] || page[idKey] != initial[idKey] {
					t.Fatal("mixed capture")
				}
				if page["page"] != nil {
					info = page["page"].(map[string]any)
				} else {
					info = page[section+"Page"].(map[string]any)
				}
				if info["returned"].(float64) <= 0 {
					t.Fatal("page did not advance")
				}
				count += int(info["returned"].(float64))
			}
			if count != int(info["total"].(float64)) {
				t.Fatal("incomplete section", section)
			}
		}
		rec.journey(tool+" all sections", start, 64, 4<<20)
	}
	call(t, c, "list_instruments", map[string]any{"limit": 101}, true)
	call(t, c, "get_financial_context_page", FinancialContextPageInput{ContextID: "absent", Section: "positions", Cursor: "invalid"}, true)
}
