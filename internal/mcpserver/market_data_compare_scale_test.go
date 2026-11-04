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
	for _, limit := range []int{0, 100} {
		recorder.mu.Lock()
		recorder.metrics = map[string]payloadMetric{}
		recorder.mu.Unlock()
		start := recorder.total()
		cap := scaleCapture(t, c, "compare_market_data_import", MarketComparisonInput{Items: inputs, Limit: limit})
		maxReserved := 0
		counts := map[string]int{}
		for _, section := range []string{"results", "matches", "evidence", "audits", "targets"} {
			callsBefore := recorder.total().Calls
			rows, reserved := comparisonWalk(t, c, cap, section, limit)
			if limit == 100 {
				initial := cap["sections"].(map[string]any)[section].(map[string]any)["items"].([]any)
				entry, e := fx.service.reads.entry(fx.service.contextGeneration, cap["captureId"].(string))
				if e != nil {
					t.Fatal(e)
				}
				bytes := 0
				for _, row := range entry.Sections[section] {
					bytes += len(row)
				}
				t.Logf("section=%s rows=%d initial=%d continuationCalls=%d storedRowBytes=%d", section, len(rows), len(initial), recorder.total().Calls-callsBefore, bytes)
			}
			counts[section] = len(rows)
			if section != "results" {
				field := map[string]string{"matches": "id", "evidence": "id", "audits": "recordId", "targets": "key"}[section]
				seen := map[string]bool{}
				for _, raw := range rows {
					id, _ := raw.(map[string]any)[field].(string)
					if id == "" || seen[id] {
						t.Fatal("missing/duplicate evidence identity", section, id)
					}
					seen[id] = true
				}
			}
			maxReserved = max(maxReserved, reserved)
			if section == "results" {
				for j, raw := range rows {
					row := raw.(map[string]any)
					if row["clientItemId"] != inputs[j].ClientItemID || row["status"] != "same" {
						t.Fatal("comparison dropped/reordered/classified workload row", j, row)
					}
				}
			}
		}
		end := recorder.total()
		if counts["results"] != 233 || counts["matches"] != 233 || counts["evidence"] != 233 || counts["audits"] != 233 || counts["targets"] != 35 {
			t.Fatal(counts)
		}
		entry, e := fx.service.reads.entry(fx.service.contextGeneration, cap["captureId"].(string))
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("comparison limit=%d counts=%v calls=%d actualTotalBytes=%d actualMaxBytes=%d reservedMaxBytes=%d captureBytes=%d", limit, counts, end.Calls-start.Calls, end.Bytes-start.Bytes, end.Max, maxReserved, entry.size)
		maxCalls := 40
		if limit == 100 {
			maxCalls = 15
			if len(cap["sections"].(map[string]any)["targets"].(map[string]any)["items"].([]any)) != 35 {
				t.Fatal("small shared metadata deferred into an extra call")
			}
		}
		if maxReserved > contextWireLimit || entry.size > readCaptureLimit || end.Calls-start.Calls > maxCalls {
			t.Fatal("comparison bounds inflated")
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
