package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func comparisonRows(t *testing.T, c *mcp.ClientSession, cap map[string]any) ([]application.MarketComparisonRow, map[string]application.MarketComparisonMatch, []application.MarketComparisonEvidence) {
	t.Helper()
	var rows []application.MarketComparisonRow
	var matches []application.MarketComparisonMatch
	var evidence []application.MarketComparisonEvidence
	decode := func(v any, p any) {
		b, e := jsoncodec.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		if e = jsoncodec.Unmarshal(b, p); e != nil {
			t.Fatal(e)
		}
	}
	section := "results"
	if _, exists := cap["sections"].(map[string]any)["result_details"]; exists {
		section = "result_details"
	}
	decode(scaleSection(t, c, cap, section), &rows)
	decode(scaleSection(t, c, cap, "matches"), &matches)
	decode(scaleSection(t, c, cap, "evidence"), &evidence)
	all := map[string]application.MarketComparisonMatch{}
	for _, m := range matches {
		all[m.ID] = m
	}
	return rows, all, evidence
}
func TestMarketComparisonExactOrderedEvidenceAndFrozenPages(t *testing.T) {
	p := &scaleForbiddenProvider{}
	fx := newLedgerFixtureWithApp(t, wailstest.NewService(t, application.NewMarketDataRegistry(p)), func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	c := ledgerSession(t, fx)
	nav := map[string]any{"instrumentId": fx.instrument, "kind": "nav", "currency": "USD", "value": "1.00000001", "date": "2026-09-28", "sourceTitle": "Source A"}
	mutate(t, c, "import_market_data", map[string]any{"items": []any{nav}})
	nav["sourceTitle"] = "Source B"
	mutate(t, c, "import_market_data", map[string]any{"items": []any{nav}})
	mutate(t, c, "import_market_data", map[string]any{"items": []any{map[string]any{"instrumentId": fx.instrument, "kind": "close", "currency": "USD", "value": "5", "date": "2026-09-28", "sourceTitle": "Raw close"}, map[string]any{"baseCurrency": "USD", "quoteCurrency": "SGD", "kind": "daily_reference", "value": "3", "date": "2026-09-28", "sourceTitle": "FX"}}})
	if _, e := fx.service.Enable(ReadOnly); e != nil {
		t.Fatal(e)
	}
	c = connect(t, fx.service)
	item := func(id, kind, value, date string) application.MarketComparisonItem {
		return application.MarketComparisonItem{ClientItemID: id, InstrumentID: fx.instrument, Currency: "USD", Kind: kind, Value: value, Date: date}
	}
	items := []application.MarketComparisonItem{item("z", "nav", "1.00000001", "2026-09-28"), item("a", "nav", "1.00000001", "2026-09-28"), item("tail", "nav", "1.00000002", "2026-09-28"), item("close", "close", "5.0000", "2026-09-28"), item("missing", "nav", "0", "2026-09-27"), item("invalid", "nav", "1.0000000000000002", "2026-09-28"), item("unknown", "nav", "1", "2026-09-28"), {ClientItemID: "reverse", BaseCurrency: "SGD", QuoteCurrency: "USD", Value: "0.333333333333", Kind: "daily_reference", Date: "2026-09-28"}, {ClientItemID: "fx", BaseCurrency: "USD", QuoteCurrency: "SGD", Value: "3.000000000001", Kind: "daily_reference", Date: "2026-09-28"}}
	items[6].InstrumentID = uuid.NewString()
	items[0].SourceTitle = "Source A"
	items[1].SourceTitle = "Different source"
	cap := scaleCapture(t, c, "compare_market_data_import", MarketComparisonInput{Items: items, Limit: 1})
	rows, matches, evidence := comparisonRows(t, c, cap)
	for j, want := range []string{"same", "same", "conflict", "same", "missing", "invalid", "unknown_target", "unavailable", "conflict"} {
		if rows[j].ClientItemID != items[j].ClientItemID || rows[j].Ordinal != j || rows[j].Status != want {
			t.Fatalf("row %d = %+v, want %s", j, rows[j], want)
		}
	}
	if rows[0].DuplicateGroup == "" || rows[0].DuplicateGroup != rows[1].DuplicateGroup || !rows[0].BatchConflict || !rows[2].BatchConflict {
		t.Fatal("ordered duplicate/conflict group lost")
	}
	if matches[rows[0].MatchID].SourceEvidence != "matching_active_agent_provenance" || matches[rows[1].MatchID].SourceEvidence != "no_matching_active_agent_provenance" {
		t.Fatal("numeric equality confused with source import")
	}
	if matches[rows[0].MatchID].ActiveEqual != 2 || len(evidence) != 4 {
		t.Fatalf("evidence repeated or source facts missing: %d %+v", len(evidence), matches)
	}
	if matches[rows[2].MatchID].Relations[0].DifferenceKind == "" {
		t.Fatal("missing exact tail explanation")
	}
	closeMatch := matches[rows[3].MatchID]
	foundFormat := false
	for _, r := range closeMatch.Relations {
		foundFormat = foundFormat || r.DifferenceKind == "format_only"
	}
	if !foundFormat {
		t.Fatal("format-only equality lost")
	}
	if matches[rows[7].MatchID].Reason != "reverse_direction_only" {
		t.Fatal("reverse reciprocal used as exact quote")
	}
	// A new local write after capture must not change its rows, evidence or hash.
	before := scaleSection(t, c, cap, "evidence")
	_, err := fx.app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: []application.AgentMarketDataItem{{InstrumentID: fx.instrument, Kind: "nav", Currency: "USD", Value: "9", Date: "2026-09-28", SourceTitle: "Later write"}}})
	if err != nil {
		t.Fatal(err)
	}
	if after := scaleSection(t, c, cap, "evidence"); !reflect.DeepEqual(before, after) {
		t.Fatal("frozen evidence changed after concurrent update")
	}
	cfg, e := fx.service.Connection()
	if e != nil {
		t.Fatal(e)
	}
	unauth, e := http.NewRequestWithContext(t.Context(), http.MethodPost, cfg.Endpoint, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"compare_market_data_import","arguments":{}}}`))
	if e != nil {
		t.Fatal(e)
	}
	unauth.Header.Set("Content-Type", "application/json")
	resp, e := http.DefaultClient.Do(unauth)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := io.ReadAll(resp.Body)
	resp.Body.Close()
	if e != nil || resp.StatusCode != http.StatusUnauthorized || strings.Contains(string(raw), fx.instrument) {
		t.Fatal("unauthorized comparison leaked target", resp.StatusCode, e)
	}
	if p.calls.Load() != 0 {
		t.Fatal("comparison fetched provider")
	}
	if cap["summary"].(map[string]any)["writeAuthorized"] != false {
		t.Fatal("comparison granted write authorization")
	}
	cursor := cap["sections"].(map[string]any)["results"].(map[string]any)["page"].(map[string]any)["nextCursor"].(string)
	call(t, c, "get_read_page", ReadPageInput{CaptureID: cap["captureId"].(string), Section: "evidence", Cursor: cursor}, true)
	call(t, c, "get_read_page", ReadPageInput{CaptureID: cap["captureId"].(string), Section: "results", Cursor: cursor + "x"}, true)
	fx.service.reads.now = func() time.Time { return time.Now().Add(readTTL + time.Minute) }
	call(t, c, "get_read_page", ReadPageInput{CaptureID: cap["captureId"].(string), Section: "results", Cursor: cursor}, true)
}
func TestMarketComparisonCorrectedWithdrawnAndInputGuards(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	q := map[string]any{"instrumentId": fx.instrument, "kind": "nav", "currency": "USD", "value": "0", "date": "2026-09-26", "sourceTitle": "Original zero"}
	first := mutate(t, c, "import_market_data", map[string]any{"items": []any{q}})
	q["operation"] = "correct"
	q["targetQuoteId"] = first["quoteIds"].([]any)[0]
	q["value"] = "2"
	q["date"] = "2026-09-27"
	second := mutate(t, c, "import_market_data", map[string]any{"items": []any{q}})
	mutate(t, c, "import_market_data", map[string]any{"items": []any{map[string]any{"operation": "retract", "targetQuoteId": second["quoteIds"].([]any)[0], "sourceTitle": "Withdrawal on later day"}}})
	items := []application.MarketComparisonItem{{ClientItemID: "corrected", InstrumentID: fx.instrument, Kind: "nav", Currency: "USD", Value: "0", Date: "2026-09-26"}, {ClientItemID: "withdrawn", InstrumentID: fx.instrument, Kind: "nav", Currency: "USD", Value: "2", Date: "2026-09-27"}}
	cap := scaleCapture(t, c, "compare_market_data_import", MarketComparisonInput{Items: items})
	rows, _, ev := comparisonRows(t, c, cap)
	if rows[0].Status != "historical_only" || rows[1].Status != "historical_only" {
		t.Fatal(rows)
	}
	states := map[string]bool{}
	for _, e := range ev {
		states[e.State] = true
		if len(e.AuditIDs) != 2 {
			t.Fatal("audit links lost", e)
		}
	}
	if !states["corrected"] || !states["withdrawn"] || len(scaleSection(t, c, cap, "audits")) != 3 {
		t.Fatal("inactive evidence omitted")
	}
	for _, input := range []any{MarketComparisonInput{}, MarketComparisonInput{Items: []application.MarketComparisonItem{items[0], items[0]}}, map[string]any{"items": items, "displayScale": 13}, map[string]any{"items": items, "operationId": uuid.NewString()}} {
		call(t, c, "compare_market_data_import", input, true)
	}
	// The ordinary authenticated HTTP response also fits the final byte cap.
	status, wire := productHTTP(t, fx.service, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "compare_market_data_import", "arguments": MarketComparisonInput{Items: items}}}, "")
	if status != 200 || len(wire) > contextWireLimit {
		t.Fatal(status, len(wire))
	}
	many := make([]application.MarketComparisonItem, 501)
	for j := range many {
		many[j] = items[0]
		many[j].ClientItemID = fmt.Sprint(j)
	}
	call(t, c, "compare_market_data_import", MarketComparisonInput{Items: many}, true)
	huge := items[0]
	huge.SourceTitle = strings.Repeat("x", 128<<10)
	call(t, c, "compare_market_data_import", MarketComparisonInput{Items: []application.MarketComparisonItem{huge}}, true)
}
func TestMarketComparisonReadPermissionAndNoWrites(t *testing.T) {
	fx := newLedgerFixture(t)
	if _, e := fx.service.Enable(ReadOnly); e != nil {
		t.Fatal(e)
	}
	c := connect(t, fx.service)
	definitions := fx.service.toolRegistry(ReadOnly).Definitions()
	found := false
	for _, d := range definitions {
		if d.Name == "compare_market_data_import" {
			found = true
			if !d.Annotations.ReadOnlyHint {
				t.Fatal("not read-only")
			}
		}
	}
	if !found {
		t.Fatal("not available for reads")
	}
	before, e := fx.app.ListActivities(context.Background(), 100)
	if e != nil {
		t.Fatal(e)
	}
	input := MarketComparisonInput{Items: []application.MarketComparisonItem{{ClientItemID: "zero", InstrumentID: fx.instrument, Currency: "USD", Kind: "nav", Date: "2026-09-28", Value: "0"}}}
	cap := scaleCapture(t, c, "compare_market_data_import", input)
	rows, _, _ := comparisonRows(t, c, cap)
	if rows[0].Status != "missing" {
		t.Fatal(rows)
	}
	after, e := fx.app.ListActivities(t.Context(), 100)
	if e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("comparison mutated activities", e)
	}
	args, _ := jsoncodec.Marshal(input)
	status, wire := productHTTP(t, fx.service, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "compare_market_data_import", "arguments": json.RawMessage(args)}}, "")
	if status != 200 || len(wire) > contextWireLimit {
		t.Fatal(status, len(wire))
	}
}
