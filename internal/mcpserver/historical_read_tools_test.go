package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
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
		featureJSONEqual(t, cap["summary"].(map[string]any)["left"], initial["summary"].(map[string]any)["left"])
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

func TestHistoricalOversizedItemClientRecovery(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	payload := map[string]any{"id": "oversized", "evidence": strings.Repeat("synthetic😀", 10000)}
	rows, err := readRows([]any{payload})
	if err != nil {
		t.Fatal(err)
	}
	capture, err := fx.service.reads.store(fx.service.contextGeneration, readCapture{Kind: "historical_overview", Summary: map[string]int{"matchedRowCount": 1}, Sections: map[string][]json.RawMessage{"rows": rows}, Details: map[string]any{"rows/oversized": payload}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	cursor := capture.Sections["rows"].Page.NextCursor
	if cursor == nil || capture.Sections["rows"].Page.Returned != 0 {
		t.Fatal("oversized item was silently skipped")
	}
	for _, request := range []struct {
		name string
		args any
	}{
		{"get_read_page", ReadPageInput{CaptureID: capture.CaptureID, Section: "rows", Cursor: *cursor, Limit: 1}},
		{"get_read_item", ReadItemInput{CaptureID: capture.CaptureID, Section: "rows", ID: "oversized"}},
	} {
		res, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: request.name, Arguments: request.args})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := jsoncodec.Marshal(res)
		if !res.IsError || len(raw) > contextWireLimit || !strings.Contains(string(raw), "single_item") || !strings.Contains(string(raw), "too_large") {
			t.Fatalf("unrecoverable oversized item response: %s", raw)
		}
	}
}

func TestHistoricalFundingCashReasonFilterAndItem(t *testing.T) {
	fx := newLedgerFixtureWithClock(t, func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	fx.app.SetClock(func() time.Time { return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC) })
	if _, err := fx.app.AppendAccountCashValue(t.Context(), domain.AccountID(fx.brokerage), "150000", "USD", "2026-10-08T04:00:00Z"); err != nil {
		t.Fatal(err)
	}
	guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "open", Open: &application.OpenProductCommand{AccountID: fx.brokerage, Currency: "USD", Principal: "100000", EffectiveAt: "2026-10-01T04:00:00Z", Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Synthetic funding gap", StartOn: "2026-10-01", MaturityOn: productString("2026-12-01"), InterestMode: "none"}, Policy: productPolicy("2026-12-01")}})
	c := ledgerSession(t, fx)
	cap := scaleCapture(t, c, "get_historical_overview", map[string]any{"date": "2026-10-02", "compareTo": "2026-10-09", "filter": map[string]any{"accountIds": []string{fx.brokerage}, "reasons": []string{"account_value_missing"}, "side": "left"}})
	rows := assertHistoricalMatches(t, c, cap)
	if len(rows) != 1 {
		t.Fatalf("funding cause omitted by reason filter: %v", rows)
	}
	row := rows[0].(map[string]any)
	item := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "rows", ID: row["id"].(string)})
	cell := item["summary"].(map[string]any)["left"].(map[string]any)
	quality := cell["displayValuation"].(map[string]any)["quality"].(map[string]any)
	if cell["complete"] != false || cell["baseAmount"] != nil || quality["coverage"] != "unavailable" || quality["unpricedCount"] != float64(1) || cell["pendingContribution"] != float64(0) {
		t.Fatal(cell)
	}
	summary := cap["summary"].(map[string]any)["left"].(map[string]any)
	if summary["assets"] != nil || summary["knownAssets"] != "100000" {
		t.Fatal("funding gap changed financial values", summary)
	}
	aggregate := summary["displayValuation"].(map[string]any)["netWorth"].(map[string]any)["quality"].(map[string]any)
	if aggregate["unpricedCount"] != quality["unpricedCount"] || aggregate["coverage"] != "provisional" {
		t.Fatal("leaf-to-aggregate missing mapping", aggregate, quality)
	}
	// A category's many missing identities are separately pageable as well.
	header := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "overview", ID: "endpoints"})
	for _, raw := range scaleSection(t, c, header, "categories") {
		category := raw.(map[string]any)
		detail := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "categories", ID: category["id"].(string)})
		for section := range detail["sections"].(map[string]any) {
			scaleSection(t, c, detail, section)
		}
	}
}
