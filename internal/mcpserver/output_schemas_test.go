package mcpserver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
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
	base := map[string]any{"schemaVersion": "mcp-read/2", "captureId": "capture", "captureKind": "synthetic", "capturedAt": "time", "expiresAt": "time", "contentHash": "hash", "summaryHash": "summaryhash", "section": "rows", "items": []any{}, "page": map[string]any{"total": 0, "returned": 0, "hasMore": false, "nextCursor": nil, "consistency": "frozen"}}
	good := map[string]any{"data": base}
	if err := resolved.Validate(&good); err != nil {
		t.Fatal(err)
	}
	base["schemaVersion"] = "unexpected"
	if err := resolved.Validate(&good); err == nil {
		t.Fatal("wrong envelope version accepted")
	}
	base["schemaVersion"] = "mcp-read/2"
	delete(base, "captureId")
	if err := resolved.Validate(&good); err == nil {
		t.Fatal("capture identity loss accepted")
	}
	for _, name := range []string{"commit_plan", "update_instruments_batch", "get_operation"} {
		if readOutputSchema(name) != nil {
			t.Fatal("receipt acquired a new post-execution output gate", name)
		}
	}
}

func TestSDKSingleRepresentationWireEvidence(t *testing.T) {
	fx := newLedgerFixture(t)
	client, recorder := connectPayload(t, fx.service)
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_account_valuations", Arguments: map[string]any{"accountIds": []string{fx.checking}}})
	if err != nil || result.IsError || len(result.Content) != 0 || result.StructuredContent == nil {
		t.Fatal(err, result)
	}
	structured, err := jsoncodec.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	metric := recorder.total()
	if metric.Calls != 1 || metric.Bytes <= len(structured) || metric.Bytes >= len(structured)*2 {
		t.Fatal("wire must contain one complete payload plus envelope overhead", metric, len(structured))
	}
	var wrapped Response
	if err := jsoncodec.Unmarshal(structured, &wrapped); err != nil {
		t.Fatal(err)
	}
	reserved, err := financialContextWireSize(wrapped.Data)
	if err != nil || reserved < metric.Bytes || reserved > metric.Bytes+2048 {
		t.Fatal("wire budget differs from actual transport", reserved, metric, err)
	}
	t.Logf("structured JSON=%d final HTTP=%d reserved=%d", len(structured), metric.Bytes, reserved)
}

// The strict read contract must describe delivery exactly, and real responses
// must validate against it and the compact discovered schema: present and complete on a top-level initial capture,
// absent on focused item details. Continuation pages use a separate schema.
func TestDeliveryOutputSchemaMatchesResponses(t *testing.T) {
	fx := newLedgerFixture(t)
	client := ledgerSession(t, fx)
	schemas := discoverOutputSchemas(t, client)
	// Schema shape: both fields declared and required, array of strings.
	probe := func(delivery map[string]any) error {
		out := map[string]any{"data": map[string]any{"schemaVersion": "mcp-read/2", "captureId": "c", "captureKind": "k", "capturedAt": "t", "expiresAt": "t", "contentHash": "h", "summaryHash": "s", "summary": map[string]any{}, "delivery": delivery}}
		return schemas["get_overview"].strict.Validate(&out)
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
		data := schemas.call(t, client, tool, args)
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
	overview := check("get_overview", map[string]any{}, true)
	if overview["sections"] != nil || len(overview["availableSections"].([]any)) != len(overviewSections) {
		t.Fatalf("default overview did not disclose its optional selectors or sent unrequested evidence: %v", overview)
	}
	summary := overview["summary"].(map[string]any)
	if summary["displayValuation"] == nil || summary["missingInputsCount"] == nil ||
		summary["assets"] != nil || summary["liabilities"] != nil || summary["netWorth"] != nil {
		t.Fatalf("overview omitted valuation quality or duplicated amounts: %v", summary)
	}
	check("analyze_period", map[string]any{"query": map[string]any{"from": "2026-09-01", "to": "2026-09-28", "scopeKind": "household", "valuation": "base", "basis": "investment"}}, true)
	// A forced small limit leaves rows pending: delivery names the sections.
	limited := check("get_overview", map[string]any{"limit": 1, "sections": []string{"byAccountType"}}, true)
	if limited["delivery"].(map[string]any)["selectionComplete"] == true {
		for name, section := range limited["sections"].(map[string]any) {
			if section.(map[string]any)["page"].(map[string]any)["hasMore"] == true {
				t.Fatalf("section %s has more rows but delivery claims completeness", name)
			}
		}
	}
	// Focused account details now inline selected rows and report delivery.
	valuations := check("get_account_valuations", map[string]any{"limit": 1}, true)
	check("get_read_item", map[string]any{"captureId": valuations["captureId"], "section": "accounts", "id": firstItemID(t, valuations, "accounts")}, true)
	accounts := valuations["sections"].(map[string]any)["accounts"].(map[string]any)
	page := accounts["page"].(map[string]any)
	assertSchemaPage(t, page, true, "frozen")
	next := schemas.call(t, client, "get_read_page", ReadPageInput{Cursor: page["nextCursor"].(string)})
	assertSchemaPage(t, next["page"].(map[string]any), false, "frozen")
	if next["captureId"] != valuations["captureId"] || next["section"] != "accounts" {
		t.Fatalf("account valuations continuation changed capture or section: %v", next)
	}
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
		return map[string]any{"data": map[string]any{"schemaVersion": "mcp-read/2", "captureId": "c", "capturedAt": "t", "summary": map[string]any{}}}
	}
	value := good()
	if err := envelope.Validate(&value); err != nil {
		t.Fatal(err)
	}
	for field, mutate := range map[string]func(map[string]any){
		"wrong version": func(d map[string]any) { d["schemaVersion"] = "unexpected" },
		"no captureId":  func(d map[string]any) { delete(d, "captureId") },
		"no capturedAt": func(d map[string]any) { delete(d, "capturedAt") },
	} {
		value = good()
		mutate(value["data"].(map[string]any))
		if envelope.Validate(&value) == nil {
			t.Errorf("compact envelope schema accepted %s", field)
		}
	}
	page := resolve(readOutputSchema("get_read_page"))
	pageValue := map[string]any{"data": map[string]any{"schemaVersion": "mcp-read/2", "items": []any{}, "page": map[string]any{"hasMore": false, "nextCursor": nil}}}
	if err := page.Validate(&pageValue); err != nil {
		t.Fatal(err)
	}
	pageValue = map[string]any{"data": map[string]any{"schemaVersion": "mcp-read/2", "items": []any{}, "page": map[string]any{"hasMore": false}}}
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

type responseSchemas struct {
	published *jsonschema.Resolved
	strict    *jsonschema.Resolved
}

type outputSchemas map[string]responseSchemas

func discoverOutputSchemas(t *testing.T, client *mcp.ClientSession) outputSchemas {
	t.Helper()
	listed, err := listAllTools(t.Context(), client)
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(name string, value any) *jsonschema.Resolved {
		t.Helper()
		if value == nil {
			return nil
		}
		raw, err := jsoncodec.Marshal(value)
		if err != nil {
			t.Fatal(name, err)
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
	schemas := make(outputSchemas, len(listed.Tools))
	for _, tool := range listed.Tools {
		schemas[tool.Name] = responseSchemas{
			published: resolve(tool.Name, tool.OutputSchema),
			strict:    resolve(tool.Name, fullReadOutputSchema(tool.Name)),
		}
	}
	return schemas
}

func (s outputSchemas) call(t *testing.T, client *mcp.ClientSession, tool string, args any) map[string]any {
	t.Helper()
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil || result.IsError {
		t.Fatal(tool, err, result)
	}
	return s.validate(t, tool, result.StructuredContent)
}

func (s outputSchemas) validate(t *testing.T, tool string, structured any) map[string]any {
	t.Helper()
	for name, schema := range map[string]*jsonschema.Resolved{"published outputSchema": s[tool].published, "strict oracle": s[tool].strict} {
		if schema == nil {
			t.Fatalf("%s has no %s", tool, name)
		}
		if err := schema.Validate(structured); err != nil {
			t.Fatalf("%s violates %s: %v", tool, name, err)
		}
	}
	return structured.(map[string]any)["data"].(map[string]any)
}

// Corrupt one field of the actual response. Some nested types are intentionally
// checked only by the strict oracle, since discovery publishes a compact schema.
func (s outputSchemas) rejects(t *testing.T, tool string, data map[string]any, key string, change func(map[string]any), both bool) {
	t.Helper()
	corrupt := jsonObject(t, data)
	change(corrupt)
	out := map[string]any{"data": corrupt}
	if s[tool].strict.Validate(out) == nil {
		t.Errorf("%s strict oracle accepted invalid %s", tool, key)
	}
	if both && s[tool].published.Validate(out) == nil {
		t.Errorf("%s published outputSchema accepted invalid %s", tool, key)
	}
}

func (s outputSchemas) genericEnvelope(t *testing.T, tool string) *jsonschema.Resolved {
	t.Helper()
	entry := s[tool]
	if entry.published == nil || entry.strict != nil ||
		entry.published.Validate(map[string]any{"data": "generic payload"}) != nil ||
		entry.published.Validate(map[string]any{}) == nil {
		t.Fatalf("%s should advertise only a generic data envelope", tool)
	}
	return entry.published
}

func assertSchemaPage(t *testing.T, page map[string]any, more bool, consistency string) {
	t.Helper()
	cursor, _ := page["nextCursor"].(string)
	if page["hasMore"] != more || page["consistency"] != consistency ||
		(more && cursor == "") || (!more && page["nextCursor"] != nil) {
		t.Fatalf("unexpected page state: %v", page)
	}
}

func TestFinancialOutputSchemasMatchInitialAndContinuation(t *testing.T) {
	s, app, _ := fixture(t)
	comparisonFixture(t, s, app)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	schemas := discoverOutputSchemas(t, client)
	comparison := map[string]any{"leftAsOf": "2026-08-01", "rightAsOf": "2026-08-02"}
	for _, tc := range []struct {
		tool, id string
		args     any
	}{
		{"get_financial_context", "contextId", Empty{}},
		{"compare_financial_context", "comparisonId", comparison},
		{"compare_financial_attribution", "comparisonId", comparison},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			args := jsonObject(t, tc.args)
			args["limit"] = 1
			initial := schemas.call(t, client, tc.tool, args)
			schemas.rejects(t, tc.tool, initial, tc.id, func(d map[string]any) { delete(d, tc.id) }, true)
			schemas.rejects(t, tc.tool, initial, "content type", func(d map[string]any) { d["content"] = []any{} }, false)

			first := initial["positionsPage"].(map[string]any)
			assertSchemaPage(t, first, true, "frozen")
			// The fixture has missing valuation evidence. Check its independent
			// gap cursor as well as the positions cursor.
			gaps := initial["gapsPage"].(map[string]any)
			assertSchemaPage(t, gaps, true, "frozen")
			gapPage := schemas.call(t, client, "get_read_page", ReadPageInput{Cursor: gaps["nextCursor"].(string)})
			assertSchemaPage(t, gapPage["page"].(map[string]any), false, "frozen")
			if gapPage["section"] != "gaps" || len(gapPage["items"].([]any)) == 0 {
				t.Fatalf("missing valuation evidence not returned: %v", gapPage)
			}
			next := schemas.call(t, client, "get_read_page", ReadPageInput{Cursor: first["nextCursor"].(string), Limit: 1})
			assertSchemaPage(t, next["page"].(map[string]any), true, "frozen")
			last := schemas.call(t, client, "get_read_page", ReadPageInput{Cursor: next["page"].(map[string]any)["nextCursor"].(string)})
			assertSchemaPage(t, last["page"].(map[string]any), false, "frozen")
			if last[tc.id] != initial[tc.id] || last["capturedAt"] != initial["capturedAt"] ||
				len(last["items"].([]any)) == 0 {
				t.Fatalf("terminal page lost frozen identity or rows: %v", last)
			}
			schemas.rejects(t, "get_read_page", last, "page", func(d map[string]any) { delete(d, "page") }, true)
			schemas.rejects(t, "get_read_page", last, "page type", func(d map[string]any) { d["page"] = "done" }, true)
			schemas.rejects(t, "get_read_page", last, "frozen capturedAt", func(d map[string]any) { delete(d, "capturedAt") }, false)
		})
	}

	// Focused financial items only advertise the SDK's generic Response
	// envelope. Their continuations use get_read_page's read contract.
	schemas.genericEnvelope(t, "get_financial_context_item")
	capture := decodeContext(t, call(t, client, "get_financial_context", Empty{}, false))
	item := call(t, client, "get_financial_context_item", FinancialContextItemInput{
		ContextID: capture.ContextID, Ref: capture.Content.Scope.AccountRefs[0],
	}, false)["data"].(map[string]any)
	if item["schemaVersion"] != "financial-context-item/2" {
		t.Fatalf("financial item version = %v", item["schemaVersion"])
	}
	for _, removed := range []string{"contentHash", "generatedAt", "cacheExpiresAt"} {
		if _, exists := item[removed]; exists {
			t.Errorf("financial item still emits %s", removed)
		}
	}
}

func TestBusinessOutputSchemasMatchInitialAndContinuation(t *testing.T) {
	s, _, _, _ := historyFixture(t)
	t.Cleanup(s.Close)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	schemas := discoverOutputSchemas(t, client)
	const tool = "list_activities"
	first := schemas.call(t, client, tool, map[string]any{"limit": 1})
	page := first["page"].(map[string]any)
	assertSchemaPage(t, page, true, "live")
	schemas.rejects(t, tool, first, "items", func(d map[string]any) { delete(d, "items") }, true)
	schemas.rejects(t, tool, first, "hasMore type", func(d map[string]any) { d["page"].(map[string]any)["hasMore"] = "yes" }, false)
	last := schemas.call(t, client, "get_read_page", ReadPageInput{Cursor: page["nextCursor"].(string), Limit: 1})
	assertSchemaPage(t, last["page"].(map[string]any), false, "live")
	if len(first["items"].([]any)) != 1 || len(last["items"].([]any)) != 1 {
		t.Fatalf("expected one row in each activity page")
	}

	// The live review-rule list can also return a valid empty initial page.
	empty := schemas.call(t, client, "list_investment_review_rules", Empty{})
	assertSchemaPage(t, empty["page"].(map[string]any), false, "live")
	if len(empty["items"].([]any)) != 0 {
		t.Fatal("expected no review rules")
	}
}

func TestRepairOutputSchemasMatchStartAndProgress(t *testing.T) {
	s, app := dataHealthFixture(t, true)
	if _, err := s.Enable(ReadWrite); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	schemas := discoverOutputSchemas(t, client)
	const startTool = "start_data_repair"
	// This write only advertises the SDK's generic Response envelope; check
	// receipt fields explicitly, without pretending it has a strict oracle.
	startSchema := schemas.genericEnvelope(t, startTool)

	// Hold the real job at its started event so polling does not race completion.
	started, release := make(chan string, 1), make(chan struct{})
	completed := make(chan application.SyncJobSnapshot, 1)
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	app.SetMarketDataSyncListener(func(event string, job application.SyncJobSnapshot) {
		switch event {
		case application.SyncEventStarted:
			started <- job.JobID
			<-release
		case application.SyncEventCompleted:
			completed <- job
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	operationID := uuid.NewString()
	type reply struct {
		value *mcp.CallToolResult
		err   error
	}
	submitted := make(chan reply, 1)
	go func() {
		result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: startTool, Arguments: Mutation[Empty]{OperationID: operationID}})
		submitted <- reply{result, err}
	}()
	var jobID string
	select {
	case jobID = <-started:
	case response := <-submitted:
		t.Fatalf("start returned before creating a job: %v %v", response.err, response.value)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	progress := schemas.call(t, client, "get_data_repair_job", DataRepairJobInput{JobID: jobID})
	job := progress["job"].(map[string]any)
	if job["jobId"] != jobID || job["outcome"] != "running" || progress["healthIncluded"] != false {
		t.Fatalf("unexpected running job: %v", progress)
	}
	unblock()
	response := <-submitted
	if response.err != nil || response.value.IsError {
		t.Fatal(response.err, response.value)
	}
	if err := startSchema.Validate(response.value.StructuredContent); err != nil {
		t.Fatalf("start receipt violates the published outputSchema: %v", err)
	}
	data := response.value.StructuredContent.(map[string]any)["data"].(map[string]any)
	result := data["result"].(map[string]any)
	if data["id"] != operationID || data["tool"] != startTool || data["status"] != "succeeded" ||
		result["job"].(map[string]any)["jobId"] != jobID || result["attached"] != false || result["conflict"] != false {
		t.Fatalf("unexpected start receipt: %v", data)
	}
	select {
	case terminal := <-completed:
		if terminal.JobID != jobID || terminal.Outcome != application.SyncOutcomeSucceeded {
			t.Fatalf("unexpected terminal job: %+v", terminal)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	progress = schemas.call(t, client, "get_data_repair_job", DataRepairJobInput{JobID: jobID})
	if job := progress["job"].(map[string]any); job["jobId"] != jobID || job["outcome"] != application.SyncOutcomeSucceeded {
		t.Fatalf("unexpected successful job: %v", progress)
	}
}
