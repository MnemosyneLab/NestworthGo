package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestStructuredRecoveryPreservesUnknownAndSuccessReceipt(t *testing.T) {
	set := newToolSet("synthetic", true)
	receipt := strings.Repeat("synthetic-receipt", 10000)
	registerTool(set, &agentcapability.Definition{Name: "synthetic_write", Effect: agentcapability.Write}, func(_ context.Context, in IDInput) (Response, error) {
		if in.ID == "unknown" {
			return Response{}, fail("operation_outcome_unknown", "retain original plan and operation")
		}
		return Response{Data: receipt}, nil
	})
	set.server.AddReceivingMiddleware(structuredRecovery())
	client := inMemoryClient(t, set.server)
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "synthetic_write", Arguments: IDInput{ID: "unknown"}})
	if err != nil || !result.IsError {
		t.Fatal(err, result)
	}
	raw, _ := jsoncodec.Marshal(result.Meta[recoveryMeta])
	if result.StructuredContent != nil {
		t.Fatal("error polluted success output schema")
	}
	var structured ErrorRecoveryResult
	if jsoncodec.Unmarshal(raw, &structured) != nil || structured.Error.Code != "operation_outcome_unknown" || structured.Recovery.AutomaticRetry || !structured.Recovery.PreserveIntent || structured.Recovery.Action != "inspect_operation_and_current_state" {
		t.Fatal(string(raw))
	}
	if !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "operation_outcome_unknown") {
		t.Fatal("text recovery lost")
	}
	success, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "synthetic_write", Arguments: IDInput{ID: "succeed"}})
	raw, _ = jsoncodec.Marshal(success)
	if err != nil || success.IsError || !strings.Contains(string(raw), receipt) || len(raw) < contextWireLimit {
		t.Fatal("success receipt changed", err, len(raw))
	}
}
func TestStructuredReadRecoveryThroughSDK(t *testing.T) {
	fx := newLedgerFixture(t)
	client := ledgerSession(t, fx)
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_read_page", Arguments: ReadPageInput{CaptureID: "unavailable", Section: "rows", Cursor: "synthetic"}})
	if err != nil || !result.IsError {
		t.Fatal(err, result)
	}
	raw, _ := jsoncodec.Marshal(result.Meta[recoveryMeta])
	if result.StructuredContent != nil {
		t.Fatal("error polluted success output schema")
	}
	var structured ErrorRecoveryResult
	if jsoncodec.Unmarshal(raw, &structured) != nil || structured.Error.Code != "read_expired" || !structured.Recovery.DiscardCapture || structured.Recovery.AutomaticRetry {
		t.Fatal("read expiry hint", string(raw))
	}
}
