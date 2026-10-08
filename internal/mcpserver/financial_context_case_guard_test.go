package mcpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
)

// Exercise the real SDK HTTP parser, not a mock handler: encoding/json's
// case-insensitive struct matching must not choose a different tool than SDK.
func TestFinancialContextHTTPExactNameGuard(t *testing.T) {
	for _, tool := range []string{"get_financial_context", "get_financial_context_page", "get_financial_context_item", "compare_financial_context", "get_financial_comparison_page"} {
		for _, order := range []string{"upper-first", "upper-last"} {
			for _, variant := range []string{"normal", "long-id", "null-name-long-id", "schema-error", "batch-default", "batch-legacy", "batch-modern"} {
				t.Run(tool+"/"+order+"/"+variant, func(t *testing.T) {
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
					case "get_financial_context_page":
						initial, err := s.buildFinancialContext(t.Context(), s.contextGeneration, application.FinancialContextRequest{})
						if err != nil {
							t.Fatal(err)
						}
						args = map[string]any{"contextId": initial.ContextID, "section": "positions", "cursor": initial.PositionsPage.NextCursor}
					case "get_financial_context_item":
						initial, err := s.buildFinancialContext(t.Context(), s.contextGeneration, application.FinancialContextRequest{})
						if err != nil {
							t.Fatal(err)
						}
						args = map[string]any{"contextId": initial.ContextID, "ref": initial.Content.Scope.AccountRefs[0]}
					case "compare_financial_context":
						args = map[string]any{"leftAsOf": "2026-08-01", "rightAsOf": "current"}
					case "get_financial_comparison_page":
						initial, err := s.buildFinancialComparison(t.Context(), s.contextGeneration, application.FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "current"})
						if err != nil {
							t.Fatal(err)
						}
						args = map[string]any{"comparisonId": initial.ComparisonID, "section": "positions", "cursor": initial.PositionsPage.NextCursor}
					}
					if variant == "schema-error" {
						args[strings.Repeat("x", 70000)] = true
					}
					argJSON, _ := json.Marshal(args)
					fields := fmt.Sprintf(`"name":%q,"Name":"get_context"`, tool)
					if variant == "null-name-long-id" {
						fields = fmt.Sprintf(`"name":%q,"name":null,"Name":"get_context"`, tool)
					}
					if order == "upper-first" {
						fields = fmt.Sprintf(`"Name":"get_context","name":%q`, tool)
					}
					id := `1`
					if variant == "long-id" || variant == "null-name-long-id" {
						b, _ := json.Marshal(strings.Repeat("x", 70000))
						id = string(b)
					}
					payload := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"method":"tools/call","params":{%s,"arguments":%s}}`, id, fields, argJSON)
					batch := strings.HasPrefix(variant, "batch-")
					if batch {
						payload = `[` + payload + `,{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_context","arguments":{}}}]`
					}
					s.contexts.mu.Lock()
					before := len(s.contexts.entries)
					s.contexts.mu.Unlock()
					req, err := http.NewRequest(http.MethodPost, cfg.Endpoint, strings.NewReader(payload))
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Accept", "application/json, text/event-stream")
					req.Header.Set("Authorization", "Bearer "+cfg.Token)
					if variant == "batch-legacy" {
						req.Header.Set("MCP-Protocol-Version", "2025-03-26")
					}
					if variant == "batch-modern" {
						req.Header.Set("MCP-Protocol-Version", "2025-06-18")
					}
					res, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					raw, err := io.ReadAll(res.Body)
					res.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					s.contexts.mu.Lock()
					after := len(s.contexts.entries)
					s.contexts.mu.Unlock()
					t.Logf("status=%d responseBytes=%d cachedBefore=%d cachedAfter=%d", res.StatusCode, len(raw), before, after)
					want := http.StatusOK
					if batch || variant == "long-id" || variant == "null-name-long-id" {
						want = http.StatusBadRequest
					}
					if res.StatusCode != want || len(raw) > contextWireLimit {
						t.Errorf("guard mismatch: status=%d want=%d responseBytes=%d limit=%d", res.StatusCode, want, len(raw), contextWireLimit)
					}
					if (batch || variant == "long-id" || variant == "null-name-long-id") && after != before {
						t.Error("rejected input reached the financial build")
					}
					if variant == "normal" {
						var reply struct {
							Result struct {
								IsError           bool `json:"isError"`
								StructuredContent struct {
									Data json.RawMessage `json:"data"`
								} `json:"structuredContent"`
							} `json:"result"`
						}
						if err := json.Unmarshal(raw, &reply); err != nil || reply.Result.IsError || len(reply.Result.StructuredContent.Data) == 0 {
							t.Errorf("normal SDK dispatch failed: err=%v isError=%v", err, reply.Result.IsError)
						}
					}
					if variant == "schema-error" && !strings.Contains(string(raw), "too_large") {
						t.Error("unbounded SDK schema error bypassed replacement")
					}
				})
			}
		}
	}
}

func TestFinancialContextHTTPShadowKeysAndDuplicateSemantics(t *testing.T) {
	s, _, _ := fixture(t)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.Connection()
	if err != nil {
		t.Fatal(err)
	}
	bigID, _ := json.Marshal(strings.Repeat("x", 70000))
	page := `{"name":"get_financial_comparison_page","arguments":{"comparisonId":"missing","section":"positions","cursor":"missing"}}`
	unrelated := `{"name":"get_context","arguments":{}}`
	for _, tc := range []struct {
		name, fields string
		status       int
		wantID       string
		bounded      bool
	}{
		{"Params-shadow", `"id":` + string(bigID) + `,"params":` + page + `,"Params":` + unrelated, 400, "", true},
		{"ID-shadow", `"id":` + string(bigID) + `,"ID":1,"params":` + page, 400, "", true},
		{"ID-shadow-inverse", `"id":1,"ID":` + string(bigID) + `,"params":` + page, 200, "1", true},
		{"unrelated-Name-shadow", `"id":` + string(bigID) + `,"params":{"name":"get_context","Name":"get_financial_comparison_page","arguments":{}}`, 200, string(bigID), false},
		{"unrelated-Params-shadow", `"id":` + string(bigID) + `,"params":` + unrelated + `,"Params":` + page, 200, string(bigID), false},
		{"duplicate-name-protected-last", `"id":` + string(bigID) + `,"params":{"name":"get_context","name":"get_financial_comparison_page","arguments":{"comparisonId":"missing","section":"positions","cursor":"missing"}}`, 400, "", true},
		{"duplicate-name-unrelated-last", `"id":` + string(bigID) + `,"params":{"name":"get_financial_comparison_page","name":"get_context","arguments":{}}`, 200, string(bigID), false},
		{"duplicate-params-protected-last", `"id":` + string(bigID) + `,"params":` + unrelated + `,"params":` + page, 400, "", true},
		{"duplicate-params-unrelated-last", `"id":` + string(bigID) + `,"params":` + page + `,"params":` + unrelated, 200, string(bigID), false},
		{"duplicate-params-no-merge", `"id":` + string(bigID) + `,"params":` + page + `,"params":{"arguments":{}}`, 200, string(bigID), false},
		{"duplicate-name-null", `"id":` + string(bigID) + `,"params":{"name":"get_financial_comparison_page","name":null,"arguments":{"comparisonId":"missing","section":"positions","cursor":"missing"}}`, 400, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"jsonrpc":"2.0","method":"tools/call",` + tc.fields + `}`
			req, _ := http.NewRequest(http.MethodPost, cfg.Endpoint, strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			var reply struct {
				ID json.RawMessage `json:"id"`
			}
			_ = json.Unmarshal(raw, &reply)
			t.Logf("status=%d responseBytes=%d responseIDBytes=%d", res.StatusCode, len(raw), len(reply.ID))
			if res.StatusCode != tc.status || (tc.bounded && len(raw) > contextWireLimit) {
				t.Errorf("status=%d want=%d bytes=%d bounded=%v", res.StatusCode, tc.status, len(raw), tc.bounded)
			}
			if tc.wantID != "" && string(reply.ID) != tc.wantID {
				t.Errorf("ID mismatch: got %d bytes want %d bytes", len(reply.ID), len(tc.wantID))
			}
		})
	}
}

func TestFinancialContextHTTPParamsShadowBatch(t *testing.T) {
	s, _, _ := fixture(t)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.Connection()
	if err != nil {
		t.Fatal(err)
	}
	page := `{"name":"get_financial_comparison_page","arguments":{"comparisonId":"missing","section":"positions","cursor":"missing"}}`
	unrelated := `{"name":"get_context","arguments":{}}`
	for _, protocol := range []string{"", "2025-03-26", "2025-06-18"} {
		for _, protected := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/protected=%v", protocol, protected), func(t *testing.T) {
				exact, shadow := unrelated, page
				if protected {
					exact, shadow = page, unrelated
				}
				payload := `[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` + exact + `,"Params":` + shadow + `}]`
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
				raw, err := io.ReadAll(res.Body)
				res.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				want := http.StatusOK
				if protected || protocol == "2025-06-18" {
					want = http.StatusBadRequest
				}
				if res.StatusCode != want {
					t.Errorf("status=%d want=%d bytes=%d", res.StatusCode, want, len(raw))
				}
				if protected && !strings.Contains(string(raw), "require a single JSON-RPC request") {
					t.Error("batch bypassed the financial guard")
				}
				if !protected && strings.Contains(string(raw), "require a single JSON-RPC request") {
					t.Error("guard changed an unrelated tool's batch behavior")
				}
			})
		}
	}
}
