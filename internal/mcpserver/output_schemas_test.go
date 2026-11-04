package mcpserver

import (
	"bytes"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"reflect"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestStableReadEnvelopeOutputSchema(t *testing.T) {
	raw, _ := jsoncodec.Marshal(readOutputSchema("get_read_page"))
	var schema jsonschema.Schema
	if jsoncodec.Unmarshal(raw, &schema) != nil {
		t.Fatal("schema decode")
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]any{"schemaVersion": "mcp-read/1", "captureId": "capture", "captureKind": "synthetic", "capturedAt": "time", "expiresAt": "time", "contentHash": "hash", "summaryHash": "summaryhash", "section": "rows", "items": []any{}, "page": map[string]any{"total": 0, "returned": 0, "hasMore": false, "nextCursor": nil}}
	good := map[string]any{"data": base}
	if err := resolved.Validate(&good); err != nil {
		t.Fatal(err)
	}
	base["schemaVersion"] = "unexpected"
	if err := resolved.Validate(&good); err == nil {
		t.Fatal("wrong envelope version accepted")
	}
	base["schemaVersion"] = "mcp-read/1"
	delete(base, "contentHash")
	if err := resolved.Validate(&good); err == nil {
		t.Fatal("capture identity/hash loss accepted")
	}
	for _, name := range []string{"commit_change", "commit_product_operation", "update_instruments_batch", "get_operation"} {
		if readOutputSchema(name) != nil {
			t.Fatal("receipt acquired a new post-execution output gate", name)
		}
	}
}

func TestSDKDualRepresentationWireEvidence(t *testing.T) {
	fx := newLedgerFixture(t)
	client, recorder := connectPayload(t, fx.service)
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_account_snapshot", Arguments: map[string]any{"id": fx.checking}})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatal(err, result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("missing SDK text fallback")
	}
	structured, _ := jsoncodec.Marshal(result.StructuredContent)
	var a, b any
	da := jsoncodec.NewDecoder(strings.NewReader(text.Text))
	da.UseNumber()
	db := jsoncodec.NewDecoder(bytes.NewReader(structured))
	db.UseNumber()
	if da.Decode(&a) != nil || db.Decode(&b) != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("representations differ")
	}
	metric := recorder.total()
	if metric.Calls != 1 || metric.Bytes <= len(text.Text)+len(structured) {
		t.Fatal("wire measurement omitted SDK/envelope overhead", metric)
	}
	t.Logf("actual SDK dual representation: text JSON=%d structured JSON=%d final HTTP=%d; consumer/model token use is not measured", len(text.Text), len(structured), metric.Bytes)
}
