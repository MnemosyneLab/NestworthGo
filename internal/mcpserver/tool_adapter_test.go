package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Captured from the unchanged main 1701e5d5d9bb10ec9db846245766bab6f7a740e5
// via real HTTP ListTools, sorted by tool name then jsoncodec.MarshalIndent
// (two spaces, no trailing newline). These are immutable external-contract
// baselines, not regenerated expectations from the implementation under test.
func TestExternalDiscoveryMatchesMain1701e5d(t *testing.T) {
	for _, test := range []struct {
		mode  string
		count int
		hash  string
	}{
		{ReadOnly, 54, "4530963b4346ddc00b29153c648a08f6f483aab3b71f72893c52b3767303ea41"},
		{DirectoryWrite, 73, "033db03f203423e7cb00b307c1b0900a65ccf60a268903320941db4c3f2df70f"},
		{LedgerWrite, 103, "e7931cf9ad2188e2a33b7d7fc27222fcbe58f73434031e81a68681712290261a"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			t.Parallel()
			s, _, _ := fixture(t)
			if _, err := s.Enable(test.mode); err != nil {
				t.Fatal(err)
			}
			listed, err := connect(t, s).ListTools(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			sort.Slice(listed.Tools, func(i, j int) bool { return listed.Tools[i].Name < listed.Tools[j].Name })
			raw, err := jsoncodec.MarshalIndent(listed.Tools, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(raw)
			if len(listed.Tools) != test.count || hex.EncodeToString(hash[:]) != test.hash {
				t.Fatalf("external %s contract changed: tools=%d sha256=%x; expected tools=%d sha256=%s", test.mode, len(listed.Tools), hash, test.count, test.hash)
			}
			definitions := s.toolRegistry(test.mode).Definitions()
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
