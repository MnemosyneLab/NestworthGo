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

// The strict oracle keeps the full pre-compaction contract.
func TestStableReadEnvelopeOutputSchema(t *testing.T) {
	raw, _ := jsoncodec.Marshal(fullReadOutputSchema("get_read_page"))
	var schema jsonschema.Schema
	if jsoncodec.Unmarshal(raw, &schema) != nil {
		t.Fatal("schema decode")
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]any{"schemaVersion": "mcp-read/1", "captureId": "capture", "captureKind": "synthetic", "capturedAt": "time", "expiresAt": "time", "contentHash": "hash", "summaryHash": "summaryhash", "section": "rows", "items": []any{}, "page": map[string]any{"total": 0, "returned": 0, "hasMore": false, "nextCursor": nil, "consistency": "frozen"}}
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
	for _, name := range []string{"commit_plan", "update_instruments_batch", "get_operation"} {
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

// The strict read contract must describe delivery exactly, and real responses
// must validate against it and the compact discovered schema: present and complete on a top-level initial capture,
// absent on continuation pages and focused item details.
func TestDeliveryOutputSchemaMatchesResponses(t *testing.T) {
	fx := newLedgerFixture(t)
	client := ledgerSession(t, fx)
	listed, err := listAllTools(t.Context(), client)
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(name string, schemaValue any) *jsonschema.Resolved {
		raw, err := jsoncodec.Marshal(schemaValue)
		if err != nil {
			t.Fatal(err)
		}
		var schema jsonschema.Schema
		if err := jsoncodec.Unmarshal(raw, &schema); err != nil {
			t.Fatal(name, err)
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			t.Fatal(name, err)
		}
		return resolved
	}
	// validators is the strict test oracle; shipped is the compact schema that
	// tools/list actually advertises. Real responses must satisfy both.
	validators, shipped := map[string]*jsonschema.Resolved{}, map[string]*jsonschema.Resolved{}
	for _, tool := range listed.Tools {
		if readOutputKind(tool.Name) != readKindEnvelope {
			continue
		}
		validators[tool.Name] = resolve(tool.Name, fullReadOutputSchema(tool.Name))
		shipped[tool.Name] = resolve(tool.Name, tool.OutputSchema)
	}
	if len(validators) < 20 {
		t.Fatalf("expected the mcp-read/1 tools to be discovered, found %d", len(validators))
	}
	// Schema shape: both fields declared and required, array of strings.
	probe := func(delivery map[string]any) error {
		out := map[string]any{"data": map[string]any{"schemaVersion": "mcp-read/1", "captureId": "c", "captureKind": "k", "capturedAt": "t", "expiresAt": "t", "contentHash": "h", "summaryHash": "s", "summary": map[string]any{}, "delivery": delivery}}
		return validators["get_overview"].Validate(&out)
	}
	if err := probe(map[string]any{"selectionComplete": true, "pendingSections": []any{}}); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]map[string]any{
		"missing pendingSections":   {"selectionComplete": true},
		"missing selectionComplete": {"pendingSections": []any{}},
		"non-string pending":        {"selectionComplete": false, "pendingSections": []any{1}},
		"non-array pending":         {"selectionComplete": false, "pendingSections": "a"},
		"non-bool complete":         {"selectionComplete": "yes", "pendingSections": []any{}},
		"unknown field":             {"selectionComplete": true, "pendingSections": []any{}, "extra": 1},
	} {
		if probe(bad) == nil {
			t.Errorf("schema accepted an invalid delivery: %s", name)
		}
	}

	check := func(tool string, args any, wantDelivery bool) map[string]any {
		t.Helper()
		result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || result.IsError {
			t.Fatal(tool, err, result)
		}
		structured, ok := result.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("%s returned %T", tool, result.StructuredContent)
		}
		if err := validators[tool].Validate(&structured); err != nil {
			t.Fatalf("%s response violates the strict read contract: %v", tool, err)
		}
		if err := shipped[tool].Validate(&structured); err != nil {
			t.Fatalf("%s response violates its discovered outputSchema: %v", tool, err)
		}
		data := structured["data"].(map[string]any)
		delivery, has := data["delivery"].(map[string]any)
		if has != wantDelivery {
			t.Fatalf("%s delivery present=%v, want %v", tool, has, wantDelivery)
		}
		if has {
			if _, ok := delivery["selectionComplete"].(bool); !ok {
				t.Fatalf("%s selectionComplete missing: %v", tool, delivery)
			}
			if _, ok := delivery["pendingSections"].([]any); !ok {
				t.Fatalf("%s pendingSections missing or null: %v", tool, delivery)
			}
		}
		return data
	}
	check("get_overview", map[string]any{}, true)
	check("analyze_period", map[string]any{"query": map[string]any{"from": "2026-09-01", "to": "2026-09-28", "scopeKind": "household", "valuation": "base", "basis": "investment"}}, true)
	// A forced small limit leaves rows pending: delivery names the sections.
	limited := check("get_overview", map[string]any{"limit": 1}, true)
	if limited["delivery"].(map[string]any)["selectionComplete"] == true {
		for name, section := range limited["sections"].(map[string]any) {
			if section.(map[string]any)["page"].(map[string]any)["hasMore"] == true {
				t.Fatalf("section %s has more rows but delivery claims completeness", name)
			}
		}
	}
	// Focused item details and continuation pages never carry delivery.
	valuations := check("get_account_valuations", map[string]any{}, true)
	check("get_read_item", map[string]any{"captureId": valuations["captureId"], "section": "accounts", "id": firstItemID(t, valuations, "accounts")}, false)
}

func firstItemID(t *testing.T, data map[string]any, section string) string {
	t.Helper()
	items := data["sections"].(map[string]any)[section].(map[string]any)["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("section %s is empty", section)
	}
	row := items[0].(map[string]any)
	for _, key := range []string{"id", "accountId", "key"} {
		if v, ok := row[key].(string); ok {
			return v
		}
	}
	t.Fatalf("no identifier in %v", row)
	return ""
}

// The advertised schema is compact on purpose. It must still identify the
// envelope version, the capture identity and the paging fields, and it must stay
// small because the same envelope is repeated by about thirty tools.
func TestCompactReadOutputSchemaKeepsRecognitionFields(t *testing.T) {
	resolve := func(value any) *jsonschema.Resolved {
		raw, _ := jsoncodec.Marshal(value)
		var schema jsonschema.Schema
		if err := jsoncodec.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			t.Fatal(err)
		}
		return resolved
	}
	envelope := resolve(readOutputSchema("get_overview"))
	good := func() map[string]any {
		return map[string]any{"data": map[string]any{"schemaVersion": "mcp-read/1", "captureId": "c", "expiresAt": "t", "contentHash": "h", "summary": map[string]any{}}}
	}
	value := good()
	if err := envelope.Validate(&value); err != nil {
		t.Fatal(err)
	}
	for field, mutate := range map[string]func(map[string]any){
		"wrong version":  func(d map[string]any) { d["schemaVersion"] = "mcp-read/2" },
		"no captureId":   func(d map[string]any) { delete(d, "captureId") },
		"no expiresAt":   func(d map[string]any) { delete(d, "expiresAt") },
		"no contentHash": func(d map[string]any) { delete(d, "contentHash") },
	} {
		value = good()
		mutate(value["data"].(map[string]any))
		if envelope.Validate(&value) == nil {
			t.Errorf("compact envelope schema accepted %s", field)
		}
	}
	page := resolve(readOutputSchema("get_read_page"))
	pageValue := map[string]any{"data": map[string]any{"schemaVersion": "mcp-read/1", "items": []any{}, "page": map[string]any{"hasMore": false, "nextCursor": nil}}}
	if err := page.Validate(&pageValue); err != nil {
		t.Fatal(err)
	}
	pageValue = map[string]any{"data": map[string]any{"schemaVersion": "mcp-read/1", "items": []any{}, "page": map[string]any{"hasMore": false}}}
	if page.Validate(&pageValue) == nil {
		t.Error("compact page schema accepted a page without nextCursor")
	}
	for _, name := range []string{"get_overview", "get_read_page", "list_accounts", "get_financial_context", "get_data_repair_job"} {
		raw, _ := jsoncodec.Marshal(readOutputSchema(name))
		if len(raw) > 420 {
			t.Errorf("%s output schema is %d bytes; keep the repeated envelope compact", name, len(raw))
		}
	}
}
