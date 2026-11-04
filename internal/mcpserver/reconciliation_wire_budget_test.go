package mcpserver

import (
	"context"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestReconciliationPagesFitSDKWireAndAdvanceExactly(t *testing.T) {
	// The former internal-only 96 KiB check accepted this page even though the
	// SDK's duplicated text/structured result cannot fit the 64 KiB wire budget.
	items := make([]any, 12)
	for i := range items {
		items[i] = map[string]any{"id": fmt.Sprint(i), "evidence": strings.Repeat("\"数据\\", 700)}
	}
	input := ReconciliationReadInput{SessionID: "synthetic", Section: "links", Limit: 12}
	seen := 0
	calls, measured := 0, 0
	for {
		value, err := pageReconciliation(input, items, 7)
		if err != nil {
			t.Fatal(err)
		}
		page := value.(reconciliationPage)
		if len(page.Items) == 0 {
			t.Fatal("non-advancing page")
		}
		n, err := financialContextWireSize(page)
		if err != nil || n > contextWireLimit {
			t.Fatalf("wire=%d err=%v", n, err)
		}
		calls++
		measured += n
		for _, raw := range page.Items {
			if raw.(map[string]any)["id"] != fmt.Sprint(seen) {
				t.Fatalf("duplicate or omitted row %d: %+v", seen, raw)
			}
			seen++
		}
		if page.NextCursor == "" {
			break
		}
		input.Cursor = page.NextCursor
	}
	if calls <= 1 || seen != len(items) {
		t.Fatalf("pages=%d seen=%d", calls, seen)
	}
	t.Logf("synthetic reconciliation items=%d pages=%d estimated SDK wire plus allowance=%d", seen, calls, measured)
	// Content-bound live cursors reject mutation instead of mixing pages.
	first, err := pageReconciliation(ReconciliationReadInput{Limit: 1}, items, 7)
	if err != nil {
		t.Fatal(err)
	}
	items[0] = map[string]any{"id": "changed"}
	_, err = pageReconciliation(ReconciliationReadInput{Limit: 1, Cursor: first.(reconciliationPage).NextCursor}, items, 7)
	if err == nil || !strings.Contains(err.Error(), "stale_cursor") {
		t.Fatal(err)
	}
}

func TestReconciliationSingleOversizedRowFailsExplicitly(t *testing.T) {
	_, err := pageReconciliation(ReconciliationReadInput{}, []any{strings.Repeat("x", contextWireLimit)}, 1)
	if err == nil || !strings.Contains(err.Error(), "too_large") {
		t.Fatal(err)
	}
	// Empty terminal pages remain successful and carry the revision.
	value, err := pageReconciliation(ReconciliationReadInput{}, nil, 9)
	if err != nil || value.(reconciliationPage).Revision != 9 {
		t.Fatalf("empty page %v %v", value, err)
	}
	// Confirm this fixture is below the old DTO ceiling and over the wire cap.
	raw, _ := jsoncodec.Marshal(reconciliationPage{Items: []any{strings.Repeat("x", 40<<10)}, Total: 1})
	if len(raw) > 96<<10 {
		t.Fatal("invalid regression fixture")
	}
	size, _ := financialContextWireSize(reconciliationPage{Items: []any{strings.Repeat("x", 40<<10)}, Total: 1})
	if size <= contextWireLimit {
		t.Fatal("fixture does not exercise SDK duplication")
	}
}

func TestReconciliationPageActualHTTPWireBudget(t *testing.T) {
	items := make([]any, 12)
	for i := range items {
		items[i] = map[string]any{"id": fmt.Sprint(i), "evidence": strings.Repeat("\"数据\\", 700)}
	}
	set := newToolSet("synthetic pagination regression", true)
	readTool(set, "get_reconciliation_session_page", "synthetic", func(_ context.Context, in ReconciliationReadInput) (any, error) {
		return pageReconciliation(in, items, 7)
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return set.server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	server := httptest.NewServer(financialContextEnvelope(handler))
	t.Cleanup(server.Close)
	recorder := &payloadRecorder{t: t, metrics: map[string]payloadMetric{}}
	client, err := mcp.NewClient(&mcp.Implementation{Name: "synthetic-wire", Version: "1"}, nil).Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: recorder}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	cursor := ""
	seen := 0
	for {
		page := call(t, client, "get_reconciliation_session_page", ReconciliationReadInput{Section: "links", Cursor: cursor, Limit: 12}, false)["data"].(map[string]any)
		rows := page["items"].([]any)
		if len(rows) == 0 {
			t.Fatal("empty continuation")
		}
		for _, v := range rows {
			if v.(map[string]any)["id"] != fmt.Sprint(seen) {
				t.Fatal("mixed rows")
			}
			seen++
		}
		next, ok := page["nextCursor"].(string)
		if !ok || next == "" {
			break
		}
		cursor = next
	}
	if seen != len(items) {
		t.Fatalf("seen %d", seen)
	}
	measured := recorder.total()
	t.Logf("actual final HTTP JSON-RPC: calls=%d bytes=%d max=%d", measured.Calls, measured.Bytes, measured.Max)
	if measured.Calls <= 1 || measured.Max > contextWireLimit {
		t.Fatal(measured)
	}
}

func TestReconciliationReadBudgetNeverTruncatesExecutedWriteOrRecovery(t *testing.T) {
	for _, name := range []string{"commit_reconciliation_session", "commit_reconciliation", "get_operation", "commit_product_operation", "commit_product_terms", "commit_product_valuation", "commit_save_liquidity_policy", "commit_reset_liquidity_policy", "update_instruments_batch", "start_data_repair", "import_market_data"} {
		body := strings.Repeat("x", contextWireLimit+1)
		executed := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { executed = true; w.Write([]byte(body)) })
		raw, _ := jsoncodec.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": map[string]any{}}})
		response := httptest.NewRecorder()
		financialContextEnvelope(next).ServeHTTP(response, httptest.NewRequest("POST", "/mcp", strings.NewReader(string(raw))))
		if !executed || response.Body.String() != body {
			t.Fatalf("%s execution receipt/recovery was intercepted", name)
		}
	}
}

func TestReconciliationReadHTTPExactByteBoundary(t *testing.T) {
	for _, name := range []string{"list_reconciliation_sessions", "get_reconciliation_session", "get_reconciliation_session_page", "get_reconciliation_item"} {
		for _, size := range []int{contextWireLimit, contextWireLimit + 1} {
			request, _ := jsoncodec.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": map[string]any{}}})
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("{}" + strings.Repeat(" ", size-2))) })
			response := httptest.NewRecorder()
			financialContextEnvelope(next).ServeHTTP(response, httptest.NewRequest("POST", "/mcp", strings.NewReader(string(request))))
			if size == contextWireLimit && response.Body.Len() != size {
				t.Fatalf("%s boundary=%d", name, response.Body.Len())
			}
			if size > contextWireLimit && (response.Body.Len() > contextWireLimit || !strings.Contains(response.Body.String(), "too_large")) {
				t.Fatalf("%s oversized=%d", name, response.Body.Len())
			}
		}
	}
}
