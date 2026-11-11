package mcpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
)

func TestMarketComparisonSummaryAndFocusedFrozenDetails(t *testing.T) {
	fx := newLedgerFixture(t)
	imported, e := fx.app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: []application.AgentMarketDataItem{{InstrumentID: fx.instrument, Kind: "nav", Currency: "USD", Value: "1", Date: "2026-09-28", SourceTitle: "Captured issuer"}}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = fx.service.Enable(ReadOnly); e != nil {
		t.Fatal(e)
	}
	c := connect(t, fx.service)
	first := application.MarketComparisonItem{ClientItemID: "row/z", InstrumentID: fx.instrument, Kind: "nav", Currency: "USD", Value: "1.00000000", Date: "2026-09-28", SourceTitle: "Different title", SourceURL: "https://example.invalid/first"}
	second := first
	second.ClientItemID = "row/a"
	second.SourceTitle = ""
	second.SourceURL = ""
	cap := scaleCapture(t, c, "compare_market_data_import", MarketComparisonInput{Items: []application.MarketComparisonItem{first, second}, Limit: 1})
	initial := cap["sections"].(map[string]any)["results"].(map[string]any)
	if len(initial["items"].([]any)) != 1 || initial["page"].(map[string]any)["hasMore"] != true {
		t.Fatal("summary did not paginate")
	}
	results := scaleSection(t, c, cap, "results")
	for j, raw := range results {
		row := raw.(map[string]any)
		if row["clientItemId"] != []string{first.ClientItemID, second.ClientItemID}[j] || row["status"] != "same" {
			t.Fatal(row)
		}
		for _, field := range []string{"input", "normalized", "matchId", "value", "sourceTitle"} {
			if _, ok := row[field]; ok {
				t.Fatal("default result was not minimal", field)
			}
		}
	}
	if results[0].(map[string]any)["reason"] != "same_value_different_source" {
		t.Fatal("necessary provenance distinction omitted")
	}
	for _, section := range []string{"result_details", "matches", "evidence", "audits", "targets"} {
		v := cap["sections"].(map[string]any)[section].(map[string]any)
		if len(v["items"].([]any)) != 0 || v["page"].(map[string]any)["returned"] != float64(0) {
			t.Fatal("detail was claimed delivered", section)
		}
	}
	// Later corrections do not alter the captured evidence or extend its TTL.
	_, e = fx.app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: []application.AgentMarketDataItem{{Operation: "correct", TargetQuoteID: imported.QuoteIDs[0], InstrumentID: fx.instrument, Kind: "nav", Currency: "USD", Value: "2", Date: "2026-09-28", SourceTitle: "Later correction"}}})
	if e != nil {
		t.Fatal(e)
	}
	detail := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "results", ID: second.ClientItemID, Limit: 100})
	if detail["captureId"] != cap["captureId"] || detail["expiresAt"] != cap["expiresAt"] {
		t.Fatal("focused detail recaptured or renewed TTL")
	}
	row := detail["summary"].(map[string]any)["result"].(map[string]any)
	input := row["input"].(map[string]any)
	if input["value"] != "1.00000000" {
		t.Fatal("original lexeme lost")
	}
	if _, ok := input["sourceUrl"]; ok {
		t.Fatal("prior row URL leaked through reused decoder")
	}
	if _, ok := row["reason"]; ok {
		t.Fatal("prior result reason leaked")
	}
	if len(detail["summary"].(map[string]any)) != 1 {
		t.Fatal("focused item repeats static delivery metadata")
	}
	for name, raw := range detail["sections"].(map[string]any) {
		section := raw.(map[string]any)
		if section["page"].(map[string]any)["hasMore"] == true || len(section["items"].([]any)) != 1 {
			t.Fatal("small item evidence incomplete", name)
		}
		if strings.HasSuffix(name, "/evidence") {
			ev := section["items"].([]any)[0].(map[string]any)
			if ev["id"] != imported.QuoteIDs[0] || ev["value"] != "1" || ev["state"] != "active" {
				t.Fatal("later correction changed snapshot", ev)
			}
		}
	}
	cache, e := fx.service.reads.entry(fx.service.contextGeneration, cap["captureId"].(string))
	if e != nil {
		t.Fatal(e)
	}
	if len(cache.Sections) != 5 || cache.Sections["result_details"] != nil || len(cache.Details) != 0 {
		t.Fatal("virtual projections duplicated stored evidence")
	}
	call(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "results", ID: "unknown"}, true)
	call(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "evidence", ID: second.ClientItemID}, true)
	fx.service.reads.now = func() time.Time { return time.Now().Add(readTTL + time.Minute) }
	call(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "results", ID: second.ClientItemID}, true)
}
func TestMarketComparisonFocusedDetailCursorsAreBoundAndComplete(t *testing.T) {
	fx := newLedgerFixture(t)
	items := []application.AgentMarketDataItem{}
	for j := 0; j < 40; j++ {
		items = append(items, application.AgentMarketDataItem{InstrumentID: fx.instrument, Kind: "nav", Currency: "USD", Value: "1", Date: "2026-09-28", SourceTitle: "Synthetic repeated source evidence"})
	}
	if _, e := fx.app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: items}); e != nil {
		t.Fatal(e)
	}
	c := ledgerSession(t, fx)
	cap := scaleCapture(t, c, "compare_market_data_import", MarketComparisonInput{Items: []application.MarketComparisonItem{{ClientItemID: "one", InstrumentID: fx.instrument, Currency: "USD", Kind: "nav", Date: "2026-09-28", Value: "1"}}})
	detail := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "results", ID: "one"})
	for _, kind := range []string{"relations", "evidence", "audits"} {
		name := "comparison_item/0/" + kind
		section := detail["sections"].(map[string]any)[name].(map[string]any)
		if section["page"].(map[string]any)["hasMore"] != true {
			t.Fatal("expected bounded focused pages", name)
		}
		rows := scaleSection(t, c, detail, name)
		if len(rows) != 40 {
			t.Fatal("focused evidence omitted", name, len(rows))
		}
	}
	name := "comparison_item/0/evidence"
	cursor := detail["sections"].(map[string]any)[name].(map[string]any)["page"].(map[string]any)["nextCursor"].(string)
	call(t, c, "get_read_page", ReadPageInput{CaptureID: cap["captureId"].(string), Section: "comparison_item/0/audits", Cursor: cursor}, true)
	call(t, c, "get_read_page", ReadPageInput{Cursor: cursor + "x"}, true)
	for _, limit := range []int{1, 100} {
		selected := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "results", ID: "one", Sections: []string{"relations", "relations"}, Limit: limit})
		sections := selected["sections"].(map[string]any)
		if len(sections) != 1 || len(sections["comparison_item/0/relations"].(map[string]any)["items"].([]any)) != min(limit, 40) {
			t.Fatal("selected child limit not honored", selected)
		}
		if len(scaleSection(t, c, selected, "comparison_item/0/relations")) != 40 {
			t.Fatal("selected continuation lost relations")
		}
	}
	fx.service.reads.revoke()
	call(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "results", ID: "one"}, true)
}
