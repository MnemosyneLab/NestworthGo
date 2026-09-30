package mcpserver

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAgentMarketDataPermissions(t *testing.T) {
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		t.Run(mode, func(t *testing.T) {
			s, _, _ := fixture(t)
			if _, err := s.Enable(mode); err != nil {
				t.Fatal(err)
			}
			c := connect(t, s)
			list, err := c.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, tool := range list.Tools {
				found[tool.Name] = true
			}
			if !found["get_market_data"] || !found["list_agent_market_data"] || found["import_market_data"] != (mode == LedgerWrite) || found["set_fx_source"] != (mode == LedgerWrite) {
				t.Fatalf("bad mode tools: %s %v", mode, found)
			}
			if mode != LedgerWrite {
				_, err := c.CallTool(context.Background(), &mcp.CallToolParams{Name: "import_market_data", Arguments: map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{}}}})
				if err == nil {
					t.Fatal("read/directory mode accepted quote mutation")
				}
			}
		})
	}
}

func TestAgentMarketDataImportCorrectWithdrawAndRetry(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	ctx := context.Background()
	id, _ := domain.ParseInstrumentID(fx.instrument)
	if _, err := fx.app.AppendManualInstrumentQuote(ctx, id, "1.01", "2026-09-20", false); err != nil {
		t.Fatal(err)
	}
	item := map[string]any{"instrumentId": fx.instrument, "currency": "USD", "value": "1.0197", "kind": "nav", "date": "2026-09-28", "sourceTitle": "Fund issuer NAV", "sourceUrl": "https://example.com/nav"}
	args := map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{item, map[string]any{"baseCurrency": "USD", "quoteCurrency": "SGD", "value": "1.28", "kind": "daily_reference", "date": "2026-09-28", "sourceTitle": "Reference rates"}}}}
	first := call(t, c, "import_market_data", args, false)["data"].(map[string]any)
	repeated := call(t, c, "import_market_data", args, false)["data"]
	if !reflect.DeepEqual(first, repeated) {
		t.Fatal("retry changed receipt")
	}
	result := first["result"].(map[string]any)
	ids := result["quoteIds"].([]any)
	if len(ids) != 2 || result["inserted"] != float64(2) {
		t.Fatal(result)
	}
	get := func() any {
		return call(t, c, "get_market_data", map[string]any{"instrumentId": fx.instrument}, false)["data"].(map[string]any)["current"]
	}
	current := get().(map[string]any)
	if current["unitPrice"] != "1.0197" || current["sourceKind"] != "agent" {
		t.Fatal(current)
	}
	rate := call(t, c, "get_market_data", map[string]any{"baseCurrency": "USD", "quoteCurrency": "SGD"}, false)["data"].(map[string]any)["current"].(map[string]any)
	if rate["rate"] != "1.28" || rate["sourceKind"] != "agent" {
		t.Fatal(rate)
	}
	// The business idempotency key also recovers a receipt lost after DB commit.
	op, err := fx.service.GetOperation(args["operationId"].(string))
	if err != nil {
		t.Fatal(err)
	}
	op.Status = "pending"
	path, _ := fx.service.operationPath(op.ID)
	if err := fx.service.writePrivate(path, op); err != nil {
		t.Fatal(err)
	}
	recovered := call(t, c, "import_market_data", args, false)["data"].(map[string]any)["result"].(map[string]any)
	if recovered["replayed"] != true || !reflect.DeepEqual(recovered["quoteIds"], result["quoteIds"]) {
		t.Fatal(recovered)
	}
	item["operation"], item["targetQuoteId"], item["value"] = "correct", ids[0], "1.02"
	corrected := mutate(t, c, "import_market_data", map[string]any{"items": []any{item}})
	if q := get().(map[string]any); q["unitPrice"] != "1.02" {
		t.Fatal(q)
	}
	correctedID := corrected["quoteIds"].([]any)[0]
	mutate(t, c, "import_market_data", map[string]any{"items": []any{map[string]any{"operation": "retract", "targetQuoteId": correctedID, "sourceTitle": "Issuer withdrew this NAV"}}})
	if q := get().(map[string]any); q["unitPrice"] != "1.01" || q["sourceKind"] != "manual" {
		t.Fatalf("withdrawal did not restore eligible manual quote: %v", q)
	}
	records := call(t, c, "list_agent_market_data", map[string]any{}, false)["data"].(map[string]any)
	if records["total"] != float64(4) {
		t.Fatal(records)
	}
	active := 0
	for _, raw := range records["items"].([]any) {
		if raw.(map[string]any)["active"] == true {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("only FX should remain active: %v", records)
	}
}

func TestAgentMarketDataBatchValidationIsAtomic(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	valid := map[string]any{"instrumentId": fx.instrument, "currency": "USD", "value": "2", "kind": "latest", "quotedAt": "2026-09-29T11:00:00Z", "sourceTitle": "Issuer"}
	invalid := map[string]any{"instrumentId": fx.instrument, "currency": "USD", "value": "3", "kind": "nav", "date": "2026-10-01", "sourceTitle": "Issuer"}
	call(t, c, "import_market_data", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{valid, invalid}}}, true)
	records := call(t, c, "list_agent_market_data", map[string]any{}, false)["data"].(map[string]any)
	if records["total"] != float64(0) {
		t.Fatal("invalid batch persisted a partial observation")
	}
	for _, patch := range []map[string]any{{"currency": "SGD"}, {"quotedAt": ""}, {"value": "-1"}, {"sourceUrl": "file:///tmp/quote"}, {"sourceTitle": ""}, {"kind": "annualized_yield"}, {"date": "2026-09-28"}} {
		item := map[string]any{}
		for k, v := range valid {
			item[k] = v
		}
		for k, v := range patch {
			item[k] = v
		}
		call(t, c, "import_market_data", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{item}}}, true)
	}
}
