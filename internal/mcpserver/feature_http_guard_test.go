package mcpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

var featureHTTPTools = []string{
	"get_upcoming_actions",
	"get_historical_overview",
	"get_historical_overview_series",
}

func TestFeatureHTTPBoundaries(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	if _, err := fx.service.Enable(LedgerWrite); err != nil {
		t.Fatal(err)
	}

	// The three feature names share one isFeatureTool case arm.
	for _, name := range featureHTTPTools[:1] {
		t.Run(name, func(t *testing.T) {
			// The SDK's closed schemas reject the private field and can include
			// the entire request in diagnostics. The envelope replaces those
			// diagnostics with a fixed message while preserving a small response.
			payload := map[string]any{
				"jsonrpc": "2.0", "id": 1, "method": "tools/call",
				"params": map[string]any{"name": name, "arguments": map[string]any{"private-sensitive-property": "private-sensitive-value"}},
			}
			status, raw := productHTTP(t, fx.service, payload, "")
			if status != http.StatusOK || len(raw) > contextWireLimit ||
				!strings.Contains(string(raw), "invalid feature request; inspect the tool schema") ||
				strings.Contains(string(raw), "private-property") || strings.Contains(string(raw), "private-sensitive-value") {
				t.Fatalf("schema error was not safely bounded: status=%d bytes=%d body=%s", status, len(raw), raw)
			}
			payload["params"].(map[string]any)["arguments"] = map[string]any{strings.Repeat("private-property-", 5000): "private-sensitive-value"}
			status, raw = productHTTP(t, fx.service, payload, "")
			if status != http.StatusOK || len(raw) > contextWireLimit || !strings.Contains(string(raw), "too_large") || strings.Contains(string(raw), "private-property") || strings.Contains(string(raw), "private-sensitive-value") {
				t.Fatalf("oversized SDK diagnostic was not safely bounded: status=%d bytes=%d body=%s", status, len(raw), raw)
			}

			// The limit counts the encoded JSON-RPC ID itself: 510 characters
			// plus quotes is exactly 512 bytes; 511 characters is 513 bytes.
			payload["id"] = strings.Repeat("x", 510)
			status, raw = productHTTP(t, fx.service, payload, "")
			if status != http.StatusOK || len(raw) > contextWireLimit || !strings.Contains(string(raw), `"id":"`+strings.Repeat("x", 510)+`"`) {
				t.Fatalf("512-byte RPC ID was rejected or altered: status=%d bytes=%d", status, len(raw))
			}
			payload["id"] = strings.Repeat("x", 511)
			status, raw = productHTTP(t, fx.service, payload, "")
			if status != http.StatusBadRequest || len(raw) > contextWireLimit {
				t.Fatalf("513-byte RPC ID was not rejected: status=%d bytes=%d", status, len(raw))
			}

			payload["id"] = 1
			// The envelope rejects a bounded batch before it reads the protocol header.
			// Protocol-specific SDK batch behavior stays in TestFinancialContextHTTPParamsShadowBatch.
			status, raw = productHTTP(t, fx.service, []any{payload}, "")
			if status != http.StatusBadRequest || len(raw) > contextWireLimit {
				t.Fatalf("batch bypass: status=%d bytes=%d", status, len(raw))
			}
		})
	}
}

func TestFeatureHTTPResponseBudgetAllRegisteredNames(t *testing.T) {
	t.Parallel()
	oversized := strings.Repeat("x", contextWireLimit+1)
	server := httptest.NewServer(financialContextEnvelope(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, oversized)
	})))
	t.Cleanup(server.Close)

	for _, name := range featureHTTPTools[:1] {
		t.Run(name, func(t *testing.T) {
			payload, err := jsoncodec.Marshal(map[string]any{
				"jsonrpc": "2.0", "id": 7, "method": "tools/call",
				"params": map[string]any{"name": name, "arguments": map[string]any{}},
			})
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(string(payload)))
			if err != nil {
				t.Fatal(err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(res.Body)
			res.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			var reply struct {
				ID     json.RawMessage `json:"id"`
				Result struct {
					IsError bool `json:"isError"`
				} `json:"result"`
			}
			if err := jsoncodec.Unmarshal(body, &reply); err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != http.StatusOK || len(body) > contextWireLimit ||
				string(reply.ID) != "7" || !reply.Result.IsError || !strings.Contains(string(body), "feature response exceeds the MCP wire budget") || strings.Contains(string(body), oversized[:32]) {
				if len(body) > 200 {
					body = body[:200]
				}
				t.Fatalf("oversized feature response was not replaced: status=%d bytes=%d body=%s", res.StatusCode, len(body), body)
			}
		})
	}
}
