package mcpserver

import (
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolGroupMetadataDoesNotFilterDiscovery(t *testing.T) {
	fx := newLedgerFixture(t)
	client := ledgerSession(t, fx)
	plain, err := client.ListTools(t.Context(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.Tools) != 16 || plain.NextCursor == "" {
		t.Fatalf("full first page: tools=%d cursor=%q", len(plain.Tools), plain.NextCursor)
	}
	for _, metadata := range [][]string{{"summary"}, {"private-unrecognized"}} {
		page, err := client.ListTools(t.Context(), &mcp.ListToolsParams{Meta: mcp.Meta{"nestworth.toolGroups": metadata}})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(toolNames(page), toolNames(plain)) || page.NextCursor == "" {
			t.Fatalf("tools/list _meta.nestworth.toolGroups=%v still changes discovery", metadata)
		}
	}
}

func toolNames(page *mcp.ListToolsResult) []string {
	names := make([]string, len(page.Tools))
	for i, tool := range page.Tools {
		names[i] = tool.Name
	}
	return names
}
