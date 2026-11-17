package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func inMemoryClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-contract", Version: "1"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func TestTypedMCPValidationAndErrorsMatchPriorAdapter(t *testing.T) {
	t.Parallel()
	prior := mcp.NewServer(&mcp.Implementation{Name: "Nestworth", Version: mcpImplementationVersion}, nil)
	closed, destructive := false, false
	handler := func(_ context.Context, in IDInput) (Response, error) {
		if in.ID == "refuse" {
			return Response{}, fail("validation", "synthetic business refusal")
		}
		return Response{Data: in.ID}, nil
	}
	mcp.AddTool(prior, &mcp.Tool{Name: "get_synthetic", Description: "synthetic", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &destructive, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in IDInput) (*mcp.CallToolResult, Response, error) {
		result, err := handler(ctx, in)
		return nil, result, err
	})
	current := newToolSet("", true)
	readTool(current, "get_synthetic", "synthetic", func(ctx context.Context, in IDInput) (any, error) {
		result, err := handler(ctx, in)
		return result.Data, err
	})
	oldClient, newClient := inMemoryClient(t, prior), inMemoryClient(t, current.server)
	for _, test := range []struct{ name, raw string }{
		{"get_synthetic", `{"id":"synthetic"}`},
		{"get_synthetic", `{"id":"refuse"}`},
		{"get_synthetic", `{}`},
		{"get_synthetic", `{"id":12}`},
		{"get_synthetic", `{"id":"x","extra":true}`},
		{"get_synthetic", `null`},
		{"get_synthetic", `[]`},
		{"get_synthetic", `{"id":"x"} {}`},
		{"unknown", `{}`},
	} {
		oldResult, oldErr := oldClient.CallTool(t.Context(), &mcp.CallToolParams{Name: test.name, Arguments: json.RawMessage(test.raw)})
		newResult, newErr := newClient.CallTool(t.Context(), &mcp.CallToolParams{Name: test.name, Arguments: json.RawMessage(test.raw)})
		if (oldErr == nil) != (newErr == nil) || (oldErr != nil && oldErr.Error() != newErr.Error()) {
			t.Errorf("%s %s errors differ: %v / %v", test.name, test.raw, oldErr, newErr)
		}
		if oldResult != nil && !oldResult.IsError && newResult != nil && !newResult.IsError {
			if len(newResult.Content) != 0 || newResult.StructuredContent == nil {
				t.Fatal("success must have one structured representation", newResult)
			}
			oldResult.Content = newResult.Content
		}
		if !reflect.DeepEqual(oldResult, newResult) {
			t.Errorf("%s %s result differs: %+v / %+v", test.name, test.raw, oldResult, newResult)
		}
	}
}

func TestFrozenContributionDiscoveryContract(t *testing.T) {
	s, _, _ := fixture(t)
	t.Cleanup(s.Close)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	listed, err := listAllTools(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, tool := range listed.Tools {
		if tool.Name != "list_contributions" {
			continue
		}
		found++
		raw, err := jsoncodec.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := jsoncodec.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		props := schema["properties"].(map[string]any)
		if props["offset"] != nil || props["query"] == nil || props["limit"] == nil || tool.Annotations.ReadOnlyHint {
			t.Fatal("capture schema/effect changed", schema)
		}
	}
	if found != 1 {
		t.Fatal(found)
	}
}

// Cursor rejects an entire tools/list page when any outputSchema property is a
// boolean schema, so receipts publish none and read schemas use objects only.
func TestDiscoveryOutputSchemasHaveNoBooleanProperties(t *testing.T) {
	s, _, _ := fixture(t)
	t.Cleanup(s.Close)
	if _, err := s.Enable(ReadWrite); err != nil {
		t.Fatal(err)
	}
	listed, err := listAllTools(t.Context(), connect(t, s))
	if err != nil {
		t.Fatal(err)
	}
	var check func(name string, node any)
	check = func(name string, node any) {
		switch v := node.(type) {
		case map[string]any:
			if props, ok := v["properties"].(map[string]any); ok {
				for key, prop := range props {
					if _, ok := prop.(map[string]any); !ok {
						t.Errorf("%s outputSchema property %q is not an object schema: %v", name, key, prop)
					}
				}
			}
			for _, child := range v {
				check(name, child)
			}
		case []any:
			for _, child := range v {
				check(name, child)
			}
		}
	}
	writes := 0
	for _, tool := range listed.Tools {
		if tool.Annotations != nil && !tool.Annotations.ReadOnlyHint {
			writes++
		}
		if tool.OutputSchema == nil {
			continue
		}
		if readOutputSchema(tool.Name) == nil {
			t.Errorf("%s publishes an inferred outputSchema", tool.Name)
		}
		raw, err := jsoncodec.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema any
		if err := jsoncodec.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		check(tool.Name, schema)
	}
	if writes == 0 {
		t.Fatal("read-write discovery listed no write tools")
	}
}

// Match real MCP discovery clients: a short page is not the entire tool set.
func listAllTools(ctx context.Context, client *mcp.ClientSession) (*mcp.ListToolsResult, error) {
	all := &mcp.ListToolsResult{}
	params := &mcp.ListToolsParams{}
	for {
		page, err := client.ListTools(ctx, params)
		if err != nil {
			return nil, err
		}
		all.Tools = append(all.Tools, page.Tools...)
		if page.NextCursor == "" {
			return all, nil
		}
		params.Cursor = page.NextCursor
	}
}
