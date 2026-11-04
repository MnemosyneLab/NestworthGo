package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Captured from the unchanged main 82f3be0e981d5cd3a8cdaae061a4b76b179b8c29
// via real HTTP ListTools, sorted by tool name then jsoncodec.MarshalIndent
// (two spaces, no trailing newline). These are immutable external-contract
// baselines, not regenerated expectations from the implementation under test.
// Relative to the earlier 1701e5d capture, upstream #198 changed only the two
// historical-overview tool descriptions; all modes keep the same tool schemas.
func TestExternalDiscoveryMatchesMain82f3be0(t *testing.T) {
	for _, test := range []struct {
		mode  string
		count int
		hash  string
	}{
		{ReadOnly, 54, "c02f6b3026c818f83cbe80cc45213b6d231b79f575de36a8b97bf37c35290080"},
		{DirectoryWrite, 73, "d4e617f2f51354d975f876b9d2fd1a68a2dccc1784e4f1161046b56ee8373ce1"},
		{LedgerWrite, 103, "e00154e5f62d4de957f8ba16d35e4e9003c4a77b3d4a01643202ffd65c0d24f4"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			t.Parallel()
			s, _, _ := fixture(t)
			if _, err := s.Enable(test.mode); err != nil {
				t.Fatal(err)
			}
			client := connect(t, s)
			listed, err := client.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			sort.Slice(listed.Tools, func(i, j int) bool { return listed.Tools[i].Name < listed.Tools[j].Name })
			// The new read tool has its own schema/permission contract tests.
			// Keep every pre-existing discovery byte checked against main.
			baseline := make([]*mcp.Tool, 0, len(listed.Tools))
			for _, tool := range listed.Tools {
				if tool.Name != "compare_market_data_import" {
					baseline = append(baseline, tool)
				}
			}
			if len(listed.Tools) != test.count+1 {
				t.Fatalf("expected exactly one additive read tool: %d", len(listed.Tools))
			}
			raw, err := jsoncodec.MarshalIndent(baseline, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(raw)
			if len(baseline) != test.count || hex.EncodeToString(hash[:]) != test.hash {
				t.Fatalf("external %s contract changed: tools=%d sha256=%x; expected tools=%d sha256=%s", test.mode, len(listed.Tools), hash, test.count, test.hash)
			}
			registry := s.toolRegistry(test.mode)
			instructions := client.InitializeResult().Instructions
			if instructions != registry.Instructions() || !strings.Contains(instructions, "endpoint_bridge/2 reconciliation semantics") || strings.Contains(instructions, "endpoint_bridge/1") {
				t.Fatal("shared/external instructions lost current historical reconciliation semantics")
			}
			definitions := registry.Definitions()
			if len(definitions) != len(listed.Tools) {
				t.Fatal("local registry lost tools")
			}
			for i, d := range definitions {
				local, err := jsoncodec.Marshal(d)
				if err != nil {
					t.Fatal(err)
				}
				// Effect is host policy and intentionally absent from the wire.
				if !reflect.DeepEqual(jsonObject(t, json.RawMessage(local)), jsonObject(t, listed.Tools[i])) {
					t.Errorf("%s local/external metadata differ", d.Name)
				}
				if listed.Tools[i].Annotations.IdempotentHint && d.Effect != agentcapability.Write {
					t.Errorf("%s mutation lacks a write boundary", d.Name)
				}
			}
		})
	}
}

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
	prior := mcp.NewServer(&mcp.Implementation{Name: "Nestworth", Version: "2.0.0"}, nil)
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
		if !reflect.DeepEqual(oldResult, newResult) {
			t.Errorf("%s %s result differs: %+v / %+v", test.name, test.raw, oldResult, newResult)
		}
	}
}
