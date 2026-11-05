package mcpserver

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
)

func comparisonWalk(t *testing.T, c *mcp.ClientSession, cap map[string]any, section string, limit int) ([]any, int) {
	t.Helper()
	s := cap["sections"].(map[string]any)[section].(map[string]any)
	rows := append([]any{}, s["items"].([]any)...)
	p := s["page"].(map[string]any)
	maxReserved, e := financialContextWireSize(cap)
	if e != nil {
		t.Fatal(e)
	}
	for p["hasMore"] == true {
		next := scaleCapture(t, c, "get_read_page", ReadPageInput{CaptureID: cap["captureId"].(string), Section: section, Cursor: p["nextCursor"].(string), Limit: limit})
		if next["contentHash"] != cap["contentHash"] {
			t.Fatal("capture changed")
		}
		if len(next["items"].([]any)) == 0 {
			t.Fatal("page failed to advance")
		}
		rows = append(rows, next["items"].([]any)...)
		p = next["page"].(map[string]any)
		size, e := financialContextWireSize(next)
		if e != nil {
			t.Fatal(e)
		}
		maxReserved = max(maxReserved, size)
	}
	if len(rows) != int(p["total"].(float64)) {
		t.Fatal("incomplete section", section, len(rows), p)
	}
	return rows, maxReserved
}
func TestMarketComparison35Targets233RowsHTTPBudgets(t *testing.T) {
	fx := newLedgerFixture(t)
	ids := []string{fx.instrument}
	for j := 1; j < 35; j++ {
		i, e := fx.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: fmt.Sprintf("Synthetic %02d", j), Type: "stock", QuoteCurrency: "USD", QuoteSource: "agent"})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, i.ID.String())
	}
	inputs := []application.MarketComparisonItem{}
	imports := []application.AgentMarketDataItem{}
	for j := range 233 {
		day := time.Date(2026, 9, 1+j/35, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
		i := application.MarketComparisonItem{ClientItemID: fmt.Sprintf("csv/%03d", j), InstrumentID: ids[j%35], Currency: "USD", Kind: "nav", Value: fmt.Sprintf("1.%08d", j), Date: day}
		inputs = append(inputs, i)
		imports = append(imports, application.AgentMarketDataItem{InstrumentID: i.InstrumentID, Currency: i.Currency, Kind: i.Kind, Value: i.Value, Date: i.Date, SourceTitle: "Synthetic issuer"})
	}
	// Populate isolated synthetic setup in the authoritative writer's 100-row
	// batches. No fixture setup call is included in the read-call measurement.
	for start := 0; start < len(imports); start += 100 {
		if _, e := fx.app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: imports[start:min(start+100, len(imports))]}); e != nil {
			t.Fatal(e)
		}
	}
	c, recorder := connectPayload(t, fx.service)
	for _, view := range []string{"summary", "full"} {
		for _, limit := range []int{0, 25, 100} {
			recorder.mu.Lock()
			recorder.metrics = map[string]payloadMetric{}
			recorder.mu.Unlock()
			start := recorder.total()
			cap := scaleCapture(t, c, "compare_market_data_import", MarketComparisonInput{Items: inputs, Limit: limit, View: map[string]string{"summary": "", "full": "full"}[view]})
			results, maxReserved := comparisonWalk(t, c, cap, "results", limit)
			for j, raw := range results {
				row := raw.(map[string]any)
				if row["clientItemId"] != inputs[j].ClientItemID || row["status"] != "same" {
					t.Fatal("lost/reordered classification", j, row)
				}
				if view == "summary" {
					for _, field := range []string{"input", "normalized", "matchId", "value", "currentValue", "sourceTitle"} {
						if _, exists := row[field]; exists {
							t.Fatal("detail leaked into compact result", field)
						}
					}
				}
			}
			if len(results) != 233 {
				t.Fatal("incomplete results")
			}
			classified := recorder.total()
			if view == "summary" {
				want := 3
				if limit == 0 || limit == 25 {
					want = 10
				}
				if classified.Calls-start.Calls != want {
					t.Fatal("summary call budget changed", classified.Calls-start.Calls, want)
				}
				for _, section := range []string{"result_details", "matches", "evidence", "audits", "targets"} {
					page := cap["sections"].(map[string]any)[section].(map[string]any)
					if len(page["items"].([]any)) != 0 || page["page"].(map[string]any)["hasMore"] != true {
						t.Fatal("unread evidence not marked deferred", section)
					}
				}
			}
			t.Logf("classification view=%s limit=%d calls=%d wireBytes=%d maxActual=%d maxReserved=%d", view, limit, classified.Calls-start.Calls, classified.Bytes-start.Bytes, classified.Max, maxReserved)
			if view == "summary" {
				before := recorder.total()
				detail := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "results", ID: inputs[0].ClientItemID})
				if detail["contentHash"] != cap["contentHash"] {
					t.Fatal("focused detail recaptured")
				}
				row := detail["summary"].(map[string]any)["result"].(map[string]any)
				if row["input"].(map[string]any)["value"] != inputs[0].Value {
					t.Fatal("exact original input lost")
				}
				for name, raw := range detail["sections"].(map[string]any) {
					section := raw.(map[string]any)
					if section["page"].(map[string]any)["hasMore"] == true {
						t.Fatal("small focused detail unexpectedly deferred", name)
					}
					if len(section["items"].([]any)) != 1 {
						t.Fatal("focused evidence lost or unrelated evidence included", name)
					}
				}
				after := recorder.total()
				t.Logf("focused detail view=%s limit=%d extraCalls=%d wireBytes=%d", view, limit, after.Calls-before.Calls, after.Bytes-before.Bytes)
				if after.Calls-before.Calls != 1 {
					t.Fatal("focused detail required an extra capture")
				}
			}
			detailStart := recorder.total()
			counts := map[string]int{"results": len(results)}
			names := []string{"matches", "evidence", "audits", "targets"}
			if view == "summary" {
				names = append([]string{"result_details"}, names...)
			}
			for _, section := range names {
				rows, reserved := comparisonWalk(t, c, cap, section, limit)
				counts[section] = len(rows)
				maxReserved = max(maxReserved, reserved)
				field := map[string]string{"result_details": "clientItemId", "matches": "id", "evidence": "id", "audits": "recordId", "targets": "key"}[section]
				seen := map[string]bool{}
				for _, raw := range rows {
					id, _ := raw.(map[string]any)[field].(string)
					if id == "" || seen[id] {
						t.Fatal("missing/duplicate evidence identity", section, id)
					}
					seen[id] = true
				}
			}
			end := recorder.total()
			entry, e := fx.service.reads.entry(fx.service.contextGeneration, cap["captureId"].(string))
			if e != nil {
				t.Fatal(e)
			}
			// Exclude the separate focused detail experiment from complete-traversal totals.
			calls := classified.Calls - start.Calls + end.Calls - detailStart.Calls
			bytes := classified.Bytes - start.Bytes + end.Bytes - detailStart.Bytes
			t.Logf("complete view=%s limit=%d counts=%v calls=%d wireBytes=%d maxActual=%d maxReserved=%d storedBytes=%d", view, limit, counts, calls, bytes, end.Max, maxReserved, entry.size)
			if counts["matches"] != 233 || counts["evidence"] != 233 || counts["audits"] != 233 || counts["targets"] != 35 || view == "summary" && counts["result_details"] != 233 {
				t.Fatal(counts)
			}
			if maxReserved > contextWireLimit || entry.size > readCaptureLimit {
				t.Fatal("budget exceeded")
			}
		}
	}
	targets := []application.MarketReadTarget{}
	for _, id := range ids {
		targets = append(targets, application.MarketReadTarget{InstrumentID: id})
	}
	recorder.mu.Lock()
	recorder.metrics = map[string]payloadMetric{}
	recorder.mu.Unlock()
	start := recorder.total()
	legacy := scaleCapture(t, c, "get_market_data_batch", MarketBatchInput{Targets: targets, View: "history", Range: "all", Limit: 100})
	rows, _ := comparisonWalk(t, c, legacy, "targets", 100)
	observations := 0
	for _, raw := range rows {
		r := raw.(map[string]any)
		child := map[string]any{"captureId": legacy["captureId"], "contentHash": legacy["contentHash"], "sections": r["sections"]}
		for _, field := range []string{"pointsSection", "observationsSection"} {
			values, _ := comparisonWalk(t, c, child, r[field].(string), 100)
			if field == "observationsSection" {
				observations += len(values)
			}
		}
	}
	end := recorder.total()
	if observations != 233 {
		t.Fatal(observations)
	}
	t.Logf("legacy history limit=100 observations=%d calls=%d actualTotalBytes=%d actualMaxBytes=%d; no comparison classifications or withdrawn proof", observations, end.Calls-start.Calls, end.Bytes-start.Bytes, end.Max)
}
