package mcpserver

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func assertHistoricalMatches(t *testing.T, c *mcp.ClientSession, cap map[string]any) []any {
	t.Helper()
	rows := scaleSection(t, c, cap, "rows")
	counts := map[string][2]int{"left": {}, "right": {}}
	seen := map[string]bool{}
	previous := ""
	for _, raw := range rows {
		row := raw.(map[string]any)
		id := row["id"].(string)
		if seen[id] || id < previous {
			t.Fatal("duplicate or unstable rows")
		}
		seen[id] = true
		previous = id
		for side, count := range counts {
			if cell, ok := row[side].(map[string]any); ok && cell["countedLeaf"] == true {
				count[0]++
				count[1] += int(cell["pendingContribution"].(float64))
				counts[side] = count
			}
		}
	}
	summary := cap["summary"].(map[string]any)
	if len(rows) != int(summary["matchedRowCount"].(float64)) {
		t.Fatal("matched rows differ")
	}
	for side, count := range counts {
		key := "matchedLeft"
		if side == "right" {
			key = "matchedRight"
		}
		expected := summary[key].(map[string]any)
		if count[0] != int(expected["includedLeafCount"].(float64)) || count[1] != int(expected["pendingCount"].(float64)) {
			t.Fatalf("filtered %s leaf contributions %v/%v", side, count, expected)
		}
	}
	return rows
}
func TestHistoricalReadClientFiltersAndFrozenEvidence(t *testing.T) {
	fx := responseReviewFixture(t, 8)
	c := ledgerSession(t, fx)
	initial := scaleCapture(t, c, "get_historical_overview", map[string]any{"date": "2026-10-01", "compareTo": "2026-10-10", "limit": 1})
	rows := assertHistoricalMatches(t, c, initial)
	var id, instrument string
	for _, raw := range rows {
		row := raw.(map[string]any)
		if row["kind"] == "holding" {
			id, instrument = row["id"].(string), row["instrumentId"].(string)
			break
		}
	}
	if id == "" {
		t.Fatal("no holding")
	}
	itemArgs := ReadItemInput{CaptureID: initial["captureId"].(string), Section: "rows", ID: id}
	evidence := scaleCapture(t, c, "get_read_item", itemArgs)
	detail := evidence["summary"].(map[string]any)
	price := detail["left"].(map[string]any)["price"].(map[string]any)
	if price["value"] != "1.23456789" || price["source"] != "agent" || price["freshness"] != "manual" || price["marketDate"] != "2026-09-01" {
		t.Fatal(price)
	}
	// A mutable source change cannot change the selected rows or detail of a capture.
	if _, err := fx.app.UpdateInstrument(t.Context(), domain.InstrumentID(instrument), application.InstrumentInput{Name: "Changed synthetic name"}); err != nil {
		t.Fatal(err)
	}
	again := scaleCapture(t, c, "get_read_item", itemArgs)
	featureJSONEqual(t, again, evidence)
	assertHistoricalMatches(t, c, initial)
	for _, filter := range []map[string]any{
		{"instrumentIds": []string{instrument}}, {"accountIds": []string{fx.brokerage}, "kinds": []string{"holding"}},
		{"quality": "pending"}, {"quality": "missing"}, {"quality": "verified", "included": true},
		{"reasons": []string{"carried_price"}}, {"kinds": []string{"account"}}, {"accountIds": []string{domain.NewAccountID().String()}},
	} {
		cap := scaleCapture(t, c, "get_historical_overview", map[string]any{"date": "2026-10-01", "compareTo": "2026-10-10", "filter": filter, "limit": 1})
		assertHistoricalMatches(t, c, cap)
		if cap["summary"].(map[string]any)["totalsScope"] != "household" {
			t.Fatal("ambiguous totals")
		}
	}
	for _, args := range []map[string]any{{"date": "2026-10-01", "filter": map[string]any{"side": "right"}}, {"date": "2026-10-01", "filter": map[string]any{"quality": "pretend_healthy"}}, {"date": "2026-10-01", "filter": map[string]any{"accountIds": []string{"bad"}}}, {"date": "2026-10-01", "view": "arbitrary"}, {"date": "2026-10-01", "limit": 101}} {
		call(t, c, "get_historical_overview", args, true)
	}
	// Earlier test captures may have been evicted by the bounded eight-entry cache.
	initial = scaleCapture(t, c, "get_historical_overview", map[string]any{"date": "2026-10-01", "compareTo": "2026-10-10", "limit": 1})
	// Same-capture heavy headers and contributions preserve the GUI bridge.
	header := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: initial["captureId"].(string), Section: "overview", ID: "endpoints"})
	if len(scaleSection(t, c, header, "categories")) == 0 {
		t.Fatal("lost categories")
	}
	scaleSection(t, c, header, "contributions")
	call(t, c, "get_read_item", ReadItemInput{CaptureID: initial["captureId"].(string), Section: "rows", ID: "absent"}, true)
	// Cross-section, cross-capture and expired cursors fail deterministically.
	first := initial["sections"].(map[string]any)["rows"].(map[string]any)["page"].(map[string]any)["nextCursor"].(string)
	other := scaleCapture(t, c, "get_historical_overview", map[string]any{"date": "2026-10-01", "limit": 1})
	for _, in := range []ReadPageInput{{initial["captureId"].(string), "categories", first, 1}, {other["captureId"].(string), "rows", first, 1}, {initial["captureId"].(string), "rows", first + "x", 1}} {
		call(t, c, "get_read_page", in, true)
	}
	fx.service.reads.mu.Lock()
	fx.service.reads.now = func() time.Time { return time.Now().Add(readTTL + time.Minute) }
	fx.service.reads.mu.Unlock()
	call(t, c, "get_read_item", itemArgs, true)
}
func TestHistoricalAndAccountCompleteWireMeasurements(t *testing.T) {
	for _, n := range []int{3, 80} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			fx := responseReviewFixture(t, n)
			c := ledgerSession(t, fx)
			for name, args := range map[string]map[string]any{
				"get_historical_overview":        {"date": "2026-10-01", "compareTo": "2026-10-10"},
				"get_historical_overview_series": {"from": "2026-10-01", "to": "2026-10-10"},
				"get_account_valuations":         {"responseMode": "paged"},
			} {
				result, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
				if err != nil || result.IsError {
					t.Fatalf("%s: %v %+v", name, err, result)
				}
				raw, err := jsoncodec.Marshal(map[string]any{"jsonrpc": "2.0", "id": strings.Repeat("x", 510), "result": result})
				if err != nil {
					t.Fatal(err)
				}
				if len(raw) > contextWireLimit {
					t.Fatalf("%s wire=%d", name, len(raw))
				}
				// Exercise the actual HTTP guard as well as the client-visible SDK result.
				status, wire := productHTTP(t, fx.service, map[string]any{"jsonrpc": "2.0", "id": strings.Repeat("x", 510), "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}}, "")
				if status != 200 || len(wire) > contextWireLimit || strings.Contains(string(wire), `"isError":true`) {
					t.Fatalf("%s %d/%d: %s", name, status, len(wire), wire)
				}
				t.Logf("synthetic holdings=%d %s first-page complete JSON-RPC bytes=%d", n, name, len(wire))
				cap := scaleCapture(t, c, name, args)
				section := "rows"
				if name == "get_account_valuations" {
					section = "accounts"
				}
				scaleSection(t, c, cap, section)
				if name == "get_historical_overview_series" {
					if len(scaleSection(t, c, cap, "points")) != 10 {
						t.Fatal("lost points")
					}
				}
			}
			single := scaleCapture(t, c, "get_historical_overview_series", map[string]any{"from": "2026-10-01", "to": "2026-10-01", "limit": 1})
			if len(scaleSection(t, c, single, "points")) != 1 {
				t.Fatal("single day lost")
			}
		})
	}
}
