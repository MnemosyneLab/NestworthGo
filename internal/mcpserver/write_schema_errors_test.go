package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestWriteSchemaErrorsBeforeExecutionKeepFullReceiptAndUnknown(t *testing.T) {
	set := newToolSet("synthetic", true)
	executed := 0
	receipt := strings.Repeat("synthetic receipt ", contextWireLimit/8)
	registerTool(set, &agentcapability.Definition{Name: "synthetic_write", Effect: agentcapability.Write}, func(_ context.Context, in IDInput) (Response, error) {
		if in.ID == "unknown" {
			return Response{}, fail("operation_outcome_unknown", "inspect original operation and current facts; no automatic replay")
		}
		executed++
		return Response{Data: map[string]any{"receipt": receipt}}, nil
	})
	set.server.AddReceivingMiddleware(writeSchemaErrors(set))
	client := inMemoryClient(t, set.server)
	invalid, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "synthetic_write", Arguments: map[string]any{"id": "id", strings.Repeat("private-property", 10000): "private-value"}})
	if err != nil || !invalid.IsError || executed != 0 {
		t.Fatal("schema did not prevent execution", err, executed)
	}
	raw, _ := jsoncodec.Marshal(invalid)
	if len(raw) > 1024 || strings.Contains(string(raw), "private-") {
		t.Fatal("unsafe SDK diagnostic", len(raw))
	}
	valid, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "synthetic_write", Arguments: IDInput{ID: "id"}})
	if err != nil || valid.IsError || executed != 1 {
		t.Fatal("execution failed", err, executed)
	}
	raw, _ = jsoncodec.Marshal(valid)
	if len(raw) <= contextWireLimit || !strings.Contains(string(raw), receipt) {
		t.Fatal("post-execution receipt lost")
	}
	unknown, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "synthetic_write", Arguments: IDInput{ID: "unknown"}})
	raw, _ = jsoncodec.Marshal(unknown)
	if err != nil || !unknown.IsError || !strings.Contains(string(raw), "operation_outcome_unknown") || strings.Contains(string(raw), "no business execution occurred") {
		t.Fatal("unknown outcome rewritten", err, string(raw))
	}
}

func TestNumericMoneyMCPDiagnosticNamesFieldAndType(t *testing.T) {
	s, _, _ := fixture(t)
	if _, err := s.Enable(ReadWrite); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "preview_batch", Arguments: map[string]any{"commands": []any{map[string]any{"kind": "money_added", "accountId": "synthetic", "amount": 15708.56, "currency": "CNY", "reason": "income"}}}})
	if err != nil || !result.IsError {
		t.Fatal("numeric amount accepted", err)
	}
	var diagnostic struct {
		Message  string
		Field    string
		Rule     string
		Expected []string
	}
	if err := jsoncodec.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &diagnostic); err != nil {
		t.Fatal(err)
	}
	if diagnostic.Field != "commands[0].amount" || diagnostic.Rule != "type" || len(diagnostic.Expected) != 1 || diagnostic.Expected[0] != "string" || !strings.Contains(diagnostic.Message, "commands[0].amount") || !strings.Contains(diagnostic.Message, "JSON type string") {
		t.Fatalf("unhelpful diagnostic: %+v", diagnostic)
	}
	if strings.Contains(diagnostic.Message, "15708") {
		t.Fatal("submitted value leaked")
	}
}
