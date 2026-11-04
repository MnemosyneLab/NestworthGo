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
// These five description-only deltas document the reviewed compact-read
// contract. Assert each exact new description, then project it back so the
// immutable baseline still detects unrelated schema/permission/tool changes.
var compactReadDiscoveryChanges = map[string]struct{ before, afterHash string }{
	"get_read_item":                 {before: "Read full item evidence from a frozen products, historical, account-valuations or directory capture. Supply the row id and section. Returned child sections have independent signed cursors; follow them before claiming completeness. Does not recapture or renew TTL.", afterHash: "7c3301fac4956b07a8a2b4bfd0cc091ce28ec44e45ddab462a27ad55236fee1b"},
	"list_accounts":                 {before: "List account definitions with ownership and supported filters. Use this result for account IDs and editable fields. The bounded read contract provides stable frozen pages (default 25/max 100); get_read_item with section=accounts and id retains complete metadata/notes.", afterHash: "e50990229e67d60614b59e177d36a7cc6be1d1341e35fdf1b2ec19b2aae9b660"},
	"list_instruments":              {before: "Search existing local instruments by name, symbol or shortName before creating one. The bounded read contract supports frozen filter/types/IDs and stable sorting, with metadataVersion for atomic metadata batches. Follow every cursor. Distinguish market and quote currency.", afterHash: "3bc0e887aa22ac10b004dc2f3ad04ac5197af0303befd461c26acc3fd47a3906"},
	"get_financial_context_page":    {before: "Read all three sections (positions, gaps, evidence) from the exact frozen context and matching next cursor. Zero returned rows with nextCursor means deferred, not complete. Aliases cannot be matched across packages or used as UUIDs. Send one JSON-RPC object, never a batch. Default limit 50, maximum 100. Expired, evicted or revoked results require a new package and restart of every section; discard old pages and never mix packages or reuse old cursors with a new contextId. Pages never refresh or extend the fixed TTL. A too_large single row or summary cannot be fixed by lowering limit.", afterHash: "9ceec9bbc0cb364c847229fec366d011690fb7165f247e91a9eb9b993fa11acf"},
	"get_financial_comparison_page": {before: "Read a frozen comparison section (positions, gaps, evidence), using its comparisonId and exact matching nextCursor. Default limit 50, maximum 100. Zero returned rows with a nextCursor are deferred. Never mix contexts, comparisons, sides, sections or cursors. Expired, evicted or revoked packages require restarting every section; pages do not extend TTL. A too_large summary or row cannot be repaired with smaller pages. Changes are not returns; an attached attribution link is frozen in the same hash and summary and never recomputed by pages. Send a single JSON-RPC object, never a batch.", afterHash: "572a9d1e27a492a1a958d20fe352ac4b41bb024543bd9a289e3544eed1af2d58"},
}

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
			// The batch-history extension intentionally adds three optional inputs
			// and documents them. Project only that reviewed delta back to the
			// immutable baseline; every other tool/field must still match.
			baseline := append([]*mcp.Tool{}, listed.Tools...)
			for i, tool := range baseline {
				if delta, ok := compactReadDiscoveryChanges[tool.Name]; ok {
					hash := sha256.Sum256([]byte(tool.Description))
					if hex.EncodeToString(hash[:]) != delta.afterHash {
						t.Fatalf("unreviewed compact-read discovery delta: %s", tool.Name)
					}
					copy := *tool
					copy.Description = delta.before
					baseline[i] = &copy
				}
				if tool.Name != "get_market_data_batch" {
					continue
				}
				copy := *tool
				schemaJSON, err := jsoncodec.Marshal(tool.InputSchema)
				if err != nil {
					t.Fatal(err)
				}
				var schema map[string]any
				if err := jsoncodec.Unmarshal(schemaJSON, &schema); err != nil {
					t.Fatal(err)
				}
				properties := schema["properties"].(map[string]any)
				for _, field := range []string{"fromDate", "toDate", "historyLayout"} {
					if properties[field] == nil {
						t.Fatalf("missing optional batch extension %s", field)
					}
					delete(properties, field)
				}
				copy.InputSchema = schema
				copy.Description = "Capture 1 to 100 distinct instrument IDs or oriented FX pairs in one strictly local read-only transaction. current (default) omits history; coverage preserves counts and gapAssessment=not_evaluated; history separates selected points from raw observations per target. Follow every section cursor with get_read_page before claiming completeness. No provider refresh or writes."
				baseline[i] = &copy
			}
			raw, err := jsoncodec.MarshalIndent(baseline, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(raw)
			if len(listed.Tools) != test.count || hex.EncodeToString(hash[:]) != test.hash {
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
