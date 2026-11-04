package mcpserver

import (
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestTaskDiscoveryMeasuredSubsetAndUnchangedPermissions(t *testing.T) {
	fx := newLedgerFixture(t)
	client, recorder := connectPayload(t, fx.service)
	all, err := listAllTools(t.Context(), client)
	if err != nil {
		t.Fatal(err)
	}
	fullBytes, fullCalls := recorder.discoveryBytes, recorder.discoveryCalls
	for _, tool := range all.Tools {
		if discoveryGroup(tool.Name) == "" {
			t.Fatal("uncategorized registered tool", tool.Name)
		}
	}
	arguments := skillExample(t, "connection", "task-tool-discovery", nil)
	raw, _ := jsoncodec.Marshal(arguments)
	var focusedParams mcp.ListToolsParams
	if err := jsoncodec.Unmarshal(raw, &focusedParams); err != nil {
		t.Fatal(err)
	}
	focused, err := client.ListTools(t.Context(), &focusedParams)
	if err != nil {
		t.Fatal(err)
	}
	if len(focused.Tools) != 9 || focused.NextCursor != "" {
		t.Fatal("summary tool profile", len(focused.Tools), focused.NextCursor)
	}
	originals := map[string]*mcp.Tool{}
	for _, tool := range all.Tools {
		originals[tool.Name] = tool
	}
	for _, tool := range focused.Tools {
		if g := discoveryGroup(tool.Name); g != "summary" && g != "shared" {
			t.Fatal("unrequested tool", tool.Name)
		}
		if !reflect.DeepEqual(jsonObject(t, tool), jsonObject(t, originals[tool.Name])) {
			t.Fatal("profile altered schema/permission hints", tool.Name)
		}
	}
	bytes, calls := recorder.discoveryBytes-fullBytes, recorder.discoveryCalls-fullCalls
	if calls != 1 || bytes >= fullBytes || bytes > 24<<10 || recorder.discoveryMax > contextWireLimit {
		t.Fatal("discovery subset did not bound delivery", bytes, calls, fullBytes)
	}
	// Group choice filters discovery only, not the existing household authority.
	call(t, client, "get_catalog", Empty{}, false)
	t.Logf("actual SDK discovery: full=%d tools/%d calls/%d bytes; summary=%d tools/%d calls/%d bytes", len(all.Tools), fullCalls, fullBytes, len(focused.Tools), calls, bytes)
}

func TestTaskDiscoverySignedGroupBoundPages(t *testing.T) {
	fx := newLedgerFixture(t)
	client := ledgerSession(t, fx)
	groups := []string{"summary", "analysis", "directory"}
	params := &mcp.ListToolsParams{Meta: mcp.Meta{toolGroupsMeta: groups}}
	seen := map[string]bool{}
	cursor := ""
	for {
		page, err := client.ListTools(t.Context(), params)
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range page.Tools {
			if seen[tool.Name] {
				t.Fatal("duplicate profile tool", tool.Name)
			}
			seen[tool.Name] = true
		}
		if cursor == "" {
			cursor = page.NextCursor
		}
		if page.NextCursor == "" {
			break
		}
		params.Cursor = page.NextCursor
	}
	expected := map[string]bool{}
	for _, tool := range fx.service.toolRegistry(LedgerWrite).Definitions() {
		g := discoveryGroup(tool.Name)
		if g == "shared" || g == "summary" || g == "analysis" || g == "directory" {
			expected[tool.Name] = true
		}
	}
	if !reflect.DeepEqual(seen, expected) || cursor == "" {
		t.Fatal("incomplete profile union", len(seen), len(expected))
	}
	for _, metadata := range []map[string]any{{toolGroupsMeta: []string{"market"}}, {toolGroupsMeta: []string{"private-unrecognized"}}} {
		status, raw := productHTTP(t, fx.service, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{"_meta": metadata, "cursor": cursor}}, "")
		if status != 400 && status != 200 {
			t.Fatal(status)
		}
		var reply map[string]any
		if jsoncodec.Unmarshal(raw, &reply) != nil || reply["error"] == nil || strings.Contains(string(raw), "private-unrecognized") {
			t.Fatal("profile mismatch/tampering accepted or echoed", string(raw))
		}
	}
}

func TestTaskDiscoveryReadOnlyStillRejectsWrites(t *testing.T) {
	fx := newLedgerFixture(t)
	if _, err := fx.service.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, fx.service)
	page, err := client.ListTools(t.Context(), &mcp.ListToolsParams{Meta: mcp.Meta{toolGroupsMeta: []string{"ledger"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tools) != 3 {
		t.Fatal("read-only ledger profile gained tools", len(page.Tools))
	}
	for _, tool := range page.Tools {
		if discoveryGroup(tool.Name) != "shared" {
			t.Fatal(tool.Name)
		}
	}
	_, err = client.CallTool(t.Context(), &mcp.CallToolParams{Name: "commit_change", Arguments: map[string]any{"operationId": "00000000-0000-4000-8000-000000000001", "input": map[string]any{"planId": "synthetic"}}})
	if err == nil {
		t.Fatal("profile broadened permission")
	}
}
