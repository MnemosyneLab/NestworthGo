package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBoundedReadHTTPExactByteBoundaryAndSelectiveLegacy(t *testing.T) {
	for _, name := range []string{"get_read_page", "get_read_item", "get_market_data_batch", "update_instruments_batch", "set_investment_review_enrollment_batch", "set_investment_review_snooze_batch", "acknowledge_investment_reviews_batch", "list_instruments", "list_agent_market_data"} {
		for _, size := range []int{contextWireLimit, contextWireLimit + 1} {
			request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": map[string]any{"responseMode": "paged"}}}
			b, _ := json.Marshal(request)
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("{}" + strings.Repeat(" ", size-2)))
			})
			w := httptest.NewRecorder()
			financialContextEnvelope(next).ServeHTTP(w, httptest.NewRequest("POST", "/mcp", strings.NewReader(string(b))))
			if size == contextWireLimit && w.Body.Len() != size {
				t.Fatalf("boundary %s %d", name, w.Body.Len())
			}
			if size > contextWireLimit && (w.Body.Len() > contextWireLimit || !strings.Contains(w.Body.String(), "too_large")) {
				t.Fatalf("oversize %s %d", name, w.Body.Len())
			}
		}
	}
	for _, name := range []string{"list_instruments", "list_agent_market_data", "get_market_data"} {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": map[string]any{}}})
		w := httptest.NewRecorder()
		financialContextEnvelope(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("x", contextWireLimit+1))) })).ServeHTTP(w, httptest.NewRequest("POST", "/mcp", strings.NewReader(string(b))))
		if w.Body.Len() != contextWireLimit+1 {
			t.Fatalf("legacy capped %s", name)
		}
	}
}
func TestBoundedReadLegacyOmittedArgumentsRemainUncapped(t *testing.T) {
	for _, name := range []string{"list_instruments", "list_agent_market_data"} {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name}})
		w := httptest.NewRecorder()
		financialContextEnvelope(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("x", contextWireLimit+1))) })).ServeHTTP(w, httptest.NewRequest("POST", "/mcp", strings.NewReader(string(b))))
		if w.Body.Len() != contextWireLimit+1 {
			t.Fatalf("legacy omitted arguments capped %s", name)
		}
	}
}

func TestBoundedReadEvictionFixedTTLAndImmutableInput(t *testing.T) {
	c := newBoundedReadCache()
	c.activate(1)
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	rows, _ := readRows([]string{"a", "b"})
	query := map[string]any{"filter": map[string]any{"ids": []string{"b", "a", "a"}}}
	r, err := c.store(1, readCapture{Kind: "test", Query: query, Sections: map[string][]json.RawMessage{"items": rows}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	cursor := *r.Sections["items"].Page.NextCursor
	rows[1] = json.RawMessage(`"changed"`)
	query["filter"] = nil
	now = now.Add(4 * time.Minute)
	p, err := c.page(1, ReadPageInput{r.CaptureID, "items", cursor, 1})
	if err != nil || string(p.Items[0]) != `"b"` {
		t.Fatal("capture input mutable", err)
	}
	if p.ExpiresAt != r.ExpiresAt {
		t.Fatal("TTL renewed")
	}
	for range readCacheCount {
		if _, err := c.store(1, readCapture{Kind: "test", Sections: map[string][]json.RawMessage{"items": rows}}, 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.page(1, ReadPageInput{r.CaptureID, "items", cursor, 1}); err == nil || !strings.Contains(err.Error(), "read_expired") {
		t.Fatal(err)
	}
}
func TestBoundedReadRestoreAndPermissionRevocation(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	capture := scaleCapture(t, c, "list_instruments", map[string]any{"responseMode": "paged"})
	id := capture["captureId"].(string)
	g := fx.service.contextGeneration
	if err := fx.service.RevokeForRestore(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.reads.page(g, ReadPageInput{CaptureID: id, Section: "instruments", Cursor: "anything"}); err == nil || !strings.Contains(err.Error(), "read_revoked") {
		t.Fatal(err)
	}
	if _, err := fx.service.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.reads.page(g, ReadPageInput{CaptureID: id, Section: "instruments", Cursor: "anything"}); err == nil || !strings.Contains(err.Error(), "read_revoked") {
		t.Fatal(err)
	}
}

func TestBoundedReadPagedOptionPresenceAndLegacyAgentLimit(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	for _, name := range []string{"get_liquidity_overview", "list_products", "get_investment_reviews", "get_upcoming_actions", "list_instruments"} {
		for _, option := range []string{"filter", "sort", "limit"} {
			value := any(nil)
			if option == "limit" {
				value = 0
			}
			call(t, c, name, map[string]any{option: value}, true)
		}
	}
	call(t, c, "list_agent_market_data", map[string]any{"limit": 0, "offset": 0}, false)
	call(t, c, "list_agent_market_data", map[string]any{"filter": nil}, true)
	for _, field := range []string{"limit", "responseMode"} {
		payload := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "list_instruments", "arguments": map[string]any{field: map[string]any{"private": strings.Repeat("private", 12000)}}}}
		status, wire := productHTTP(t, fx.service, payload, "")
		if status != 200 || len(wire) > contextWireLimit || strings.Contains(string(wire), "privateprivate") {
			t.Fatalf("paging option %s schema error not bounded", field)
		}
	}
}
