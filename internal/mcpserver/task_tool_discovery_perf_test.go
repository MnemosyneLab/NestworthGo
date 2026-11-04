//go:build perf

package mcpserver

import (
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Exhaustive real HTTP/SDK coverage belongs in opt-in perf validation rather
// than adding thousands of discovery requests to every ordinary package check.
func TestTaskDiscoveryAllCategoryCombinationsWireBudget(t *testing.T) {
	fx := newLedgerFixture(t)
	client, recorder := connectPayload(t, fx.service)
	categories := []string{"context", "summary", "analysis", "directory", "ledger", "market", "products", "review", "reconciliation", "health"}
	definitions := fx.service.toolRegistry(LedgerWrite).Definitions()
	maxBytes, combinations := 0, 0
	var maxGroups []string
	maxPage := 0
	for mask := 1; mask < 1<<len(categories); mask++ {
		groups := []string{}
		selected := map[string]bool{"shared": true}
		for i, group := range categories {
			if mask&(1<<i) != 0 {
				groups = append(groups, group)
				selected[group] = true
			}
		}
		expected := map[string]bool{}
		for _, definition := range definitions {
			if selected[discoveryGroup(definition.Name)] {
				expected[definition.Name] = true
			}
		}
		seen := map[string]bool{}
		cursors := map[string]bool{}
		params := &mcp.ListToolsParams{Meta: mcp.Meta{toolGroupsMeta: groups}}
		for pageNumber := 1; ; pageNumber++ {
			recorder.mu.Lock()
			recorder.discoveryMax = 0
			recorder.mu.Unlock()
			page, err := client.ListTools(t.Context(), params)
			if err != nil {
				t.Fatalf("groups=%v page=%d: %v", groups, pageNumber, err)
			}
			recorder.mu.Lock()
			wireBytes := recorder.discoveryMax
			recorder.mu.Unlock()
			if wireBytes == 0 || wireBytes > contextWireLimit || len(page.Tools) == 0 || len(page.Tools) > 16 {
				t.Fatalf("groups=%v page=%d: wire=%d tools=%d", groups, pageNumber, wireBytes, len(page.Tools))
			}
			if wireBytes > maxBytes {
				maxBytes, maxGroups, maxPage = wireBytes, append([]string(nil), groups...), pageNumber
			}
			for _, tool := range page.Tools {
				if !expected[tool.Name] || seen[tool.Name] {
					t.Fatalf("groups=%v: unexpected/duplicate tool %s", groups, tool.Name)
				}
				seen[tool.Name] = true
			}
			if page.NextCursor == "" {
				break
			}
			if cursors[page.NextCursor] {
				t.Fatalf("groups=%v: repeated cursor", groups)
			}
			cursors[page.NextCursor] = true
			params.Cursor = page.NextCursor
		}
		if !reflect.DeepEqual(seen, expected) {
			t.Fatalf("groups=%v: incomplete union got=%d want=%d", groups, len(seen), len(expected))
		}
		combinations++
	}
	t.Logf("actual SDK/HTTP: combinations=%d pages=%d total bytes=%d maximum=%d groups=%v page=%d", combinations, recorder.discoveryCalls, recorder.discoveryBytes, maxBytes, maxGroups, maxPage)
}
