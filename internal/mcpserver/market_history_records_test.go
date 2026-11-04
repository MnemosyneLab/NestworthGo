package mcpserver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Count actual SDK tools/call round trips, including target metadata pages.
func measuredMarketSection(t *testing.T, c *mcp.ClientSession, cap map[string]any, name string) ([]any, int, int) {
	t.Helper()
	s := cap["sections"].(map[string]any)[name].(map[string]any)
	rows := append([]any{}, s["items"].([]any)...)
	p := s["page"].(map[string]any)
	calls, maxBytes := 0, 0
	for p["hasMore"] == true {
		next := scaleCapture(t, c, "get_read_page", map[string]any{"captureId": cap["captureId"], "section": name, "cursor": p["nextCursor"], "limit": 100})
		calls++
		b, _ := jsoncodec.Marshal(next)
		var envelope ReadEnvelope
		if err := decodeReadValue(b, &envelope); err != nil {
			t.Fatal(err)
		}
		size, err := financialContextWireSize(envelope)
		if err != nil || size > contextWireLimit {
			t.Fatal(size, err)
		}
		maxBytes = max(maxBytes, size)
		if next["contentHash"] != cap["contentHash"] {
			t.Fatal("snapshot changed")
		}
		items := next["items"].([]any)
		if len(items) == 0 {
			t.Fatal("no page progress")
		}
		rows = append(rows, items...)
		p = next["page"].(map[string]any)
	}
	if len(rows) != int(p["total"].(float64)) {
		t.Fatal("incomplete read")
	}
	return rows, calls, maxBytes
}

func TestMarketHistoryRecords35Targets233Quotes(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	targets := []any{}
	for n := range 35 {
		id := fx.instrument
		if n > 0 {
			i, err := fx.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: fmt.Sprintf("Synthetic %02d", n), Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			id = i.ID.String()
		}
		targets = append(targets, map[string]any{"instrumentId": id})
		count := 6
		if n < 23 {
			count = 7
		}
		for d := range count {
			if _, err := fx.app.AppendManualInstrumentQuote(t.Context(), domain.InstrumentID(id), "1.12345678", fmt.Sprintf("2026-09-%02dT12:00:00Z", d+1), false); err != nil {
				t.Fatal(err)
			}
		}
	}
	before, err := fx.app.ListActivities(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"targets": targets, "view": "history", "fromDate": "2026-09-01", "toDate": "2026-09-07", "limit": 100}
	legacy := scaleCapture(t, c, "get_market_data_batch", args)
	legacyTargets, legacyCalls, _ := measuredMarketSection(t, c, legacy, "targets")
	legacyCalls++
	for _, raw := range legacyTargets {
		row := raw.(map[string]any)
		cap := map[string]any{"captureId": legacy["captureId"], "contentHash": legacy["contentHash"], "sections": row["sections"]}
		for _, field := range []string{"pointsSection", "observationsSection"} {
			_, calls, _ := measuredMarketSection(t, c, cap, row[field].(string))
			legacyCalls += calls
		}
	}
	args["historyLayout"] = "records"
	unified := scaleCapture(t, c, "get_market_data_batch", args)
	initialBytes, _ := jsoncodec.Marshal(unified)
	var initial ReadEnvelope
	if err := decodeReadValue(initialBytes, &initial); err != nil {
		t.Fatal(err)
	}
	initialWire, _ := financialContextWireSize(initial)
	rows, calls, maxWire := measuredMarketSection(t, c, unified, "records")
	metadata, metadataCalls, _ := measuredMarketSection(t, c, unified, "targets")
	if len(rows) != 233 || len(metadata) != 35 {
		t.Fatalf("rows/targets=%d/%d", len(rows), len(metadata))
	}
	seen := map[string]bool{}
	for _, raw := range rows {
		r := raw.(map[string]any)
		id := r["quoteId"].(string)
		if seen[id] || r["value"] != "1.12345678" || r["currency"] != "USD" || r["chartSelected"] != true || r["sourceKind"] != "manual" || r["dateBasis"] != "quoted_at_in_capture_timezone" {
			t.Fatal(r)
		}
		seen[id] = true
	}
	fx.service.reads.mu.Lock()
	frozen := fx.service.reads.entries[unified["captureId"].(string)]
	fx.service.reads.mu.Unlock()
	if len(frozen.Sections) != 2 || len(frozen.Sections["records"]) != 233 {
		t.Fatal("duplicated history storage")
	}
	if calls+metadataCalls+1 >= legacyCalls {
		t.Fatal("no call reduction")
	}
	status, body := productHTTP(t, fx.service, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "get_market_data_batch", "arguments": args}}, "")
	if status != 200 || len(body) > contextWireLimit || strings.Contains(string(body), `"isError":true`) {
		t.Fatalf("wire status=%d bytes=%d", status, len(body))
	}
	t.Logf("separate HTTP wire probe: initial actual response bytes=%d", len(body))
	t.Logf("synthetic targets=35 quotes=233 legacy_calls=%d records_calls=%d (capture=1 records_pages=%d metadata_pages=%d) initial_rows=%d initial_reserved_wire_bytes=%d max_page_reserved_wire_bytes=%d capture_bytes=%d wire_limit=%d capture_limit=%d", legacyCalls, calls+metadataCalls+1, calls, metadataCalls, initial.Sections["records"].Page.Returned, initialWire, maxWire, frozen.size, contextWireLimit, readCaptureLimit)
	after, err := fx.app.ListActivities(t.Context(), 100)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("read mutated activities", err)
	}
	// Pages remain frozen after a new write; a new capture sees the new quote.
	if _, err := fx.app.AppendManualInstrumentQuote(t.Context(), domain.InstrumentID(fx.instrument), "2", "2026-09-02T14:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	again, _, _ := measuredMarketSection(t, c, unified, "records")
	if !reflect.DeepEqual(rows, again) {
		t.Fatal("capture changed after write")
	}
	newCap := scaleCapture(t, c, "get_market_data_batch", args)
	if newCap["summary"].(map[string]any)["recordCount"] != float64(234) {
		t.Fatal("new capture stale")
	}
}

func TestMarketRecordsValidationStatusesAndWithdrawal(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	base := map[string]any{"targets": []any{map[string]any{"instrumentId": fx.instrument}}, "view": "history", "historyLayout": "records"}
	for _, extra := range []map[string]any{
		{"fromDate": "2026-09-01"}, {"toDate": "2026-09-01"}, {"fromDate": "", "toDate": ""},
		{"fromDate": "2026-02-29", "toDate": "2026-03-01"}, {"fromDate": "2026-09-02", "toDate": "2026-09-01"},
		{"fromDate": "2026-09-01", "toDate": "2026-09-01", "range": "all"}, {"view": "current"}, {"historyLayout": "other"},
	} {
		args := map[string]any{}
		for k, v := range base {
			args[k] = v
		}
		for k, v := range extra {
			args[k] = v
		}
		call(t, c, "get_market_data_batch", args, true)
	}
	unknown := uuid.NewString()
	base["targets"] = []any{map[string]any{"instrumentId": fx.instrument}, map[string]any{"instrumentId": unknown}}
	base["fromDate"], base["toDate"] = "2026-09-28", "2026-09-28"
	cap := scaleCapture(t, c, "get_market_data_batch", base)
	if len(scaleSection(t, c, cap, "records")) != 0 {
		t.Fatal("invented quote")
	}
	for _, raw := range scaleSection(t, c, cap, "targets") {
		r := raw.(map[string]any)
		want := "no_quotes_in_range"
		if r["target"].(map[string]any)["instrumentId"] == unknown {
			want = "not_found"
		}
		if r["historyStatus"] != want {
			t.Fatal(r)
		}
	}
	item := map[string]any{"instrumentId": fx.instrument, "currency": "USD", "value": "1.0197", "kind": "nav", "date": "2026-09-28", "sourceTitle": "Synthetic issuer"}
	receipt := mutate(t, c, "import_market_data", map[string]any{"items": []any{item}})
	id := receipt["quoteIds"].([]any)[0]
	cap = scaleCapture(t, c, "get_market_data_batch", base)
	rows := scaleSection(t, c, cap, "records")
	if len(rows) != 1 || rows[0].(map[string]any)["date"] != "2026-09-28" || rows[0].(map[string]any)["observationKind"] != "close" || rows[0].(map[string]any)["priceBasis"] != "agent_unit_nav_v1" || rows[0].(map[string]any)["sourceKind"] != "agent" {
		t.Fatal(rows)
	}
	item["operation"], item["targetQuoteId"], item["value"] = "correct", id, "1.01970001"
	correction := mutate(t, c, "import_market_data", map[string]any{"items": []any{item}})
	correctedID := correction["quoteIds"].([]any)[0]
	corrected := scaleCapture(t, c, "get_market_data_batch", base)
	correctedRows := scaleSection(t, c, corrected, "records")
	if len(correctedRows) != 1 || correctedRows[0].(map[string]any)["quoteId"] != correctedID || correctedRows[0].(map[string]any)["value"] != "1.01970001" {
		t.Fatal("correction or exact tail lost", correctedRows)
	}
	mutate(t, c, "import_market_data", map[string]any{"items": []any{map[string]any{"operation": "retract", "targetQuoteId": correctedID, "sourceTitle": "Synthetic withdrawal"}}})
	newCap := scaleCapture(t, c, "get_market_data_batch", base)
	if len(scaleSection(t, c, newCap, "records")) != 0 || len(scaleSection(t, c, cap, "records")) != 1 {
		t.Fatal("withdrawal/frozen contract")
	}
	// Existing bounded-cache enforcement also applies to records.
	fx.service.reads.mu.Lock()
	entry := fx.service.reads.entries[cap["captureId"].(string)]
	cur := fx.service.reads.cursor(cap["captureId"].(string), "records", entry, 0)
	fx.service.reads.mu.Unlock()
	call(t, c, "get_read_page", map[string]any{"captureId": cap["captureId"], "section": "records", "cursor": cur + "x"}, true)
	fx.service.reads.now = func() time.Time { return time.Now().Add(readTTL + time.Second) }
	call(t, c, "get_read_page", map[string]any{"captureId": cap["captureId"], "section": "records", "cursor": cur}, true)
}

func TestMarketRecordsOriginalFXThroughSDK(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	q, err := fx.app.AppendManualFXQuote(t.Context(), "USD", "SGD", "3.123456789012", "2026-09-28T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	cap := scaleCapture(t, c, "get_market_data_batch", map[string]any{"targets": []any{map[string]any{"baseCurrency": "SGD", "quoteCurrency": "USD"}}, "view": "history", "historyLayout": "records", "fromDate": "2026-09-28", "toDate": "2026-09-28"})
	rows := scaleSection(t, c, cap, "records")
	if len(rows) != 1 {
		t.Fatal(rows)
	}
	r := rows[0].(map[string]any)
	if r["quoteId"] != q.ID.String() || r["value"] != "3.123456789012" || r["baseCurrency"] != "USD" || r["quoteCurrency"] != "SGD" || r["currency"] != "SGD" || r["displayValue"] == r["value"] || r["target"].(map[string]any)["baseCurrency"] != "SGD" {
		t.Fatal("original FX direction/value lost", r)
	}
}

func TestMarketRecordsSingleItemAndCaptureBudgets(t *testing.T) {
	c := newBoundedReadCache()
	c.activate(1)
	row := marketHistoryRecord{marketHistoryPoint: marketHistoryPoint{SourceKey: strings.Repeat("x", contextWireLimit)}, EvidenceKind: "active_observation"}
	rows, err := readRows([]marketHistoryRecord{row})
	if err != nil {
		t.Fatal(err)
	}
	cap, err := c.store(1, readCapture{Kind: "market_data", Sections: map[string][]json.RawMessage{"records": rows}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	page := cap.Sections["records"].Page
	if page.Returned != 0 || !page.HasMore {
		t.Fatal("oversized row skipped or treated as empty", page)
	}
	if _, err := c.page(1, ReadPageInput{CaptureID: cap.CaptureID, Section: "records", Cursor: *page.NextCursor}); err == nil || !strings.Contains(err.Error(), "single_item") {
		t.Fatal(err)
	}
	row.SourceKey = strings.Repeat("x", readCaptureLimit)
	if _, err := readRows([]marketHistoryRecord{row}); err == nil || !strings.Contains(err.Error(), "too_large") {
		t.Fatal("oversized records accepted", err)
	}
}

func TestUnifiedHistoryRawFXAndSelection(t *testing.T) {
	rate, err := domain.ParseFxRate("3.123456789012")
	if err != nil {
		t.Fatal(err)
	}
	id := domain.NewFXQuoteID()
	at := time.Date(2026, 3, 9, 3, 30, 0, 0, time.UTC)
	p := domain.QuoteSeriesPoint{ID: id.String(), Value: "0.320158102766", QuotedAt: at, SourceKind: domain.QuoteSourceProvider}
	other := p
	other.ID = uuid.NewString()
	other.SourceKind = domain.QuoteSourceAgent
	r := application.MarketReadResultTarget{Target: application.MarketReadTarget{BaseCurrency: "SGD", QuoteCurrency: "USD"}, Series: domain.QuoteSeries{Observations: []domain.QuoteSeriesPoint{p, other}, Points: []domain.QuoteSeriesPoint{other}}, RawFXQuotes: []domain.FXQuote{{ID: id, BaseCurrency: "USD", QuoteCurrency: "SGD", Rate: rate}, {ID: domain.FXQuoteID(other.ID), BaseCurrency: "USD", QuoteCurrency: "SGD", Rate: rate}}}
	rows, err := unifiedHistoryRows(r, ReadSort{}, "America/New_York")
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	for _, row := range rows {
		if row.Date != "2026-03-08" {
			t.Fatal("UTC date used instead of local date", row)
		}
		if row.QuoteID == id.String() && (row.Value != "3.123456789012" || row.DisplayValue != p.Value || row.BaseCurrency != "USD" || row.QuoteCurrency != "SGD" || row.ChartSelected) {
			t.Fatal("raw FX conflated with display", row)
		}
		if row.QuoteID == other.ID && !row.ChartSelected {
			t.Fatal("selection lost")
		}
	}
	r.RawFXQuotes = nil
	if _, err := unifiedHistoryRows(r, ReadSort{}, "UTC"); err == nil {
		t.Fatal("missing raw FX evidence treated as exact")
	}
	_, err = unifiedHistoryRows(r, ReadSort{}, "Invalid/Timezone")
	if err == nil {
		t.Fatal("invalid configured timezone silently accepted")
	}
	// Enforce the complete frozen budget, rather than returning partial evidence.
	c := newBoundedReadCache()
	c.activate(1)
	_, err = c.store(1, readCapture{Kind: "market_data", Summary: strings.Repeat("x", readCaptureLimit)}, 100)
	if err == nil || !strings.Contains(err.Error(), "too_large") {
		t.Fatal(err)
	}
}
