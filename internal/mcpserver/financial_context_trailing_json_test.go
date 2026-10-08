package mcpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Probe the actual HTTP transport's treatment of a second JSON value. A guard
// parse failure must not allow a financial call to bypass its wire/batch bounds.
func TestFinancialContextHTTPTrailingJSONValue(t *testing.T) {
	for _, tool := range []string{"get_financial_context", "get_financial_context_page", "get_financial_context_item", "compare_financial_context", "get_financial_comparison_page"} {
		for _, variant := range []string{"long-id", "schema-error", "batch", "batch-long-id", "batch-schema-error"} {
			protocols := []string{""}
			if strings.HasPrefix(variant, "batch") {
				protocols = append(protocols, "2025-03-26", "2025-06-18")
			}
			for _, protocol := range protocols {
				for _, suffix := range []string{"", " {}"} {
					name := tool + "/" + variant + "/" + protocol + "/control"
					if suffix != "" {
						name = tool + "/" + variant + "/" + protocol + "/trailing-object"
					}
					t.Run(name, func(t *testing.T) {
						s, app, _ := fixture(t)
						comparisonFixture(t, s, app)
						if _, err := s.Enable(ReadOnly); err != nil {
							t.Fatal(err)
						}
						cfg, err := s.Connection()
						if err != nil {
							t.Fatal(err)
						}
						args := map[string]any{}
						switch tool {
						case "compare_financial_context":
							args = map[string]any{"leftAsOf": "2026-08-01", "rightAsOf": "current"}
						case "get_financial_context_page":
							args = map[string]any{"contextId": "missing", "section": "positions", "cursor": "missing"}
						case "get_financial_comparison_page":
							args = map[string]any{"comparisonId": "missing", "section": "positions", "cursor": "missing"}
						case "get_financial_context_item":
							args = map[string]any{"contextId": "missing", "ref": "account-1"}
						}
						var id any = 1
						if strings.Contains(variant, "long-id") {
							id = strings.Repeat("x", 70000)
						}
						if strings.Contains(variant, "schema-error") {
							args[strings.Repeat("x", 70000)] = true
						}
						rpc := map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}}
						var body any = rpc
						batch := strings.HasPrefix(variant, "batch")
						if batch {
							body = []any{rpc, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "get_context", "arguments": map[string]any{}}}}
						}
						raw, _ := json.Marshal(body)
						payload := string(raw) + suffix
						req, _ := http.NewRequest(http.MethodPost, cfg.Endpoint, strings.NewReader(payload))
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("Accept", "application/json, text/event-stream")
						req.Header.Set("Authorization", "Bearer "+cfg.Token)
						if protocol != "" {
							req.Header.Set("MCP-Protocol-Version", protocol)
						}
						res, err := http.DefaultClient.Do(req)
						if err != nil {
							t.Fatal(err)
						}
						reply, err := io.ReadAll(res.Body)
						res.Body.Close()
						if err != nil {
							t.Fatal(err)
						}
						s.contexts.mu.Lock()
						cached := len(s.contexts.entries)
						s.contexts.mu.Unlock()
						t.Logf("status=%d requestBytes=%d responseBytes=%d cached=%d contextExpired=%v invalidJSON=%v", res.StatusCode, len(payload), len(reply), cached, strings.Contains(string(reply), "context_expired"), strings.Contains(string(reply), "invalid"))
						if len(reply) > contextWireLimit {
							t.Errorf("financial response escaped wire limit: %d > %d", len(reply), contextWireLimit)
						}
						if (batch || strings.Contains(variant, "long-id")) && res.StatusCode != http.StatusBadRequest {
							t.Errorf("expected pre-dispatch HTTP 400, got %d", res.StatusCode)
						}
						if cached != 0 {
							t.Error("protected rejected request executed a financial build")
						}
						if suffix != "" && res.StatusCode == http.StatusBadRequest {
							t.Logf("bounded rejection body: %.240s", reply)
						}
					})
				}
			}
		}
	}
}

func TestFinancialContextHTTPTrailingJSONPreservesSDKBehavior(t *testing.T) {
	for _, tool := range []string{"get_financial_context", "get_financial_context_page", "get_financial_context_item", "compare_financial_context", "get_financial_comparison_page", "get_context"} {
		t.Run(tool, func(t *testing.T) {
			s, app, _ := fixture(t)
			comparisonFixture(t, s, app)
			if _, err := s.Enable(ReadOnly); err != nil {
				t.Fatal(err)
			}
			cfg, err := s.Connection()
			if err != nil {
				t.Fatal(err)
			}
			// Both suffixes are ignored by the SDK. In particular, a protected call
			// appearing only as the second value must not execute or cause rejection.
			for _, suffix := range []string{"", " {}", ` {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_financial_context","arguments":{}}}`} {
				args := map[string]any{}
				switch tool {
				case "compare_financial_context":
					args = map[string]any{"leftAsOf": "2026-08-01", "rightAsOf": "current"}
				case "get_financial_context_page":
					args = map[string]any{"contextId": "missing", "section": "positions", "cursor": "missing"}
				case "get_financial_comparison_page":
					args = map[string]any{"comparisonId": "missing", "section": "positions", "cursor": "missing"}
				case "get_financial_context_item":
					args = map[string]any{"contextId": "missing", "ref": "account-1"}
				}
				rpc := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}}
				raw, _ := json.Marshal(rpc)
				s.contexts.mu.Lock()
				before := len(s.contexts.entries)
				s.contexts.mu.Unlock()
				req, _ := http.NewRequest(http.MethodPost, cfg.Endpoint, strings.NewReader(string(raw)+suffix))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json, text/event-stream")
				req.Header.Set("Authorization", "Bearer "+cfg.Token)
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(res.Body)
				res.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				var reply struct {
					ID     int `json:"id"`
					Result struct {
						IsError bool `json:"isError"`
					} `json:"result"`
				}
				if res.StatusCode != 200 || json.Unmarshal(body, &reply) != nil || reply.ID != 1 {
					t.Fatalf("normal SDK behavior changed: status=%d bytes=%d", res.StatusCode, len(body))
				}
				missing := strings.HasSuffix(tool, "_page") || strings.HasSuffix(tool, "_item")
				if reply.Result.IsError != missing || (missing && !strings.Contains(string(body), "context_expired")) {
					t.Fatal("unexpected result/error")
				}
				s.contexts.mu.Lock()
				after := len(s.contexts.entries)
				s.contexts.mu.Unlock()
				builds := 0
				if tool == "get_financial_context" || tool == "compare_financial_context" {
					builds = 1
				}
				if after-before != builds {
					t.Fatalf("second JSON value executed: builds=%d want=%d", after-before, builds)
				}
			}
		})
	}
	// Preserve unrelated batch behavior under both legacy defaults and newer SDK
	// protocol rejection, even when trailing JSON contains a protected tool name.
	for _, protocol := range []string{"", "2025-03-26", "2025-06-18"} {
		t.Run("unrelated-batch/"+protocol, func(t *testing.T) {
			s, _, _ := fixture(t)
			if _, err := s.Enable(ReadOnly); err != nil {
				t.Fatal(err)
			}
			cfg, err := s.Connection()
			if err != nil {
				t.Fatal(err)
			}
			payload := `[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_context","arguments":{}}},{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_catalog","arguments":{}}}] {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_financial_context","arguments":{}}}`
			req, _ := http.NewRequest(http.MethodPost, cfg.Endpoint, strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
			if protocol != "" {
				req.Header.Set("MCP-Protocol-Version", protocol)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			want := 200
			if protocol == "2025-06-18" {
				want = 400
			}
			if res.StatusCode != want {
				t.Fatalf("status=%d want=%d", res.StatusCode, want)
			}
			if want == 200 {
				var replies []json.RawMessage
				if json.Unmarshal(body, &replies) != nil || len(replies) != 2 {
					t.Fatal("unexpected batch response")
				}
			}
			s.contexts.mu.Lock()
			cached := len(s.contexts.entries)
			s.contexts.mu.Unlock()
			if cached != 0 {
				t.Fatal("ignored suffix executed")
			}
		})
	}
}
