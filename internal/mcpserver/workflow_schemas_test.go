package mcpserver

import (
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"testing"
)

func TestWorkflowStructuralDependencies(t *testing.T) {
	cases := []struct {
		name           string
		schema         *jsonschema.Schema
		valid, invalid []map[string]any
	}{
		{"activity cursor", workflowSchema[ListActivitiesInput]("list_activities"),
			[]map[string]any{{}, {"afterEffectiveAt": "t", "afterCreatedAt": "t", "afterId": "id"}},
			[]map[string]any{{"afterId": "id"}, {"afterEffectiveAt": "t", "afterId": "id"}, {"limit": 101}}},
		{"market targets and windows", workflowSchema[MarketDataInput]("get_market_data"),
			[]map[string]any{{"instrumentId": "id"}, {"baseCurrency": "USD", "quoteCurrency": "CNY"}, {"targets": []any{map[string]any{"instrumentId": "id"}}}, {"targets": []any{map[string]any{"baseCurrency": "USD", "quoteCurrency": "CNY"}}, "view": "history", "historyLayout": "records", "fromDate": "2026-09-01", "toDate": "2026-09-02"}},
			[]map[string]any{{}, {"instrumentId": "id", "baseCurrency": "USD", "quoteCurrency": "CNY"}, {"instrumentId": "id", "view": "history"}, {"baseCurrency": "USD"}, {"targets": []any{map[string]any{}}}, {"targets": []any{map[string]any{"instrumentId": "id", "baseCurrency": "USD", "quoteCurrency": "CNY"}}}, {"targets": []any{map[string]any{"baseCurrency": "USD"}}}, {"targets": []any{map[string]any{"instrumentId": "id"}}, "fromDate": "2026-09-01"}, {"targets": []any{map[string]any{"instrumentId": "id"}}, "historyLayout": "records"}, {"targets": []any{map[string]any{"instrumentId": "id"}}, "fromDate": "2026-09-01", "toDate": "2026-09-02", "range": "30d"}, {"instrumentId": "id", "targets": []any{map[string]any{"instrumentId": "id"}}}}},
		{"ledger kind", workflowSchema[BatchChangeInput]("preview_batch"), nil, nil},
		{"repair view", workflowSchema[DataRepairJobInput]("get_data_repair_job"), []map[string]any{{"jobId": "id"}, {"jobId": "id", "view": "evidence"}}, []map[string]any{{"jobId": "id", "view": "full"}}},
	}
	cases[2].valid = []map[string]any{{"kind": "money_added", "accountId": "a", "amount": "1.0001", "currency": "USD", "reason": "income"}, {"kind": "position_transfer", "fromHoldingId": "a", "toAccountId": "b", "quantity": "1"}, {"kind": "money_added", "accountId": "a", "amount": "1", "currency": "USD", "reason": "income", "note": nil}}
	cases[2].invalid = []map[string]any{{"kind": "money_added"}, {"kind": "invented"}, {"kind": "position_transfer", "fromHoldingId": "a", "quantity": "1"}, {"kind": "position_transfer", "fromHoldingId": "a", "toHoldingId": "b", "toAccountId": "c", "quantity": "1"}, {"kind": "money_added", "accountId": "a", "amount": "1", "currency": "USD", "reason": "income", "effectiveLocalDate": "2026-09-01"}, {"kind": "money_added", "accountId": "a", "amount": "1", "currency": "USD", "reason": "income", "added": true}, {"kind": "value_update", "accountId": "a", "newValue": "1", "newValueCurrency": "USD"}}
	for i := range cases[2].valid {
		cases[2].valid[i] = singleLedgerInput(cases[2].valid[i])
	}
	for i := range cases[2].invalid {
		cases[2].invalid[i] = singleLedgerInput(cases[2].invalid[i])
	}
	cases[2].invalid = append(cases[2].invalid, map[string]any{"commands": []any{}}, map[string]any{"commands": make([]any, 101)})
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resolved, err := c.schema.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range c.valid {
				if err := resolved.Validate(&v); err != nil {
					t.Fatalf("valid request rejected: %v: %v", v, err)
				}
			}
			for _, v := range c.invalid {
				if err := resolved.Validate(&v); err == nil {
					t.Fatalf("invalid structure accepted: %v", v)
				}
			}
		})
	}
}

// Independently enumerate the MCP boundary's supported commands. DTO variants
// used by the GUI must not become advertised MCP capabilities accidentally.
func TestPreviewBatchSchemaMatchesHandlerAllowlist(t *testing.T) {
	expected := map[string]bool{"position_import": true, "money_added": true, "money_removed": true, "cash_dividend": true, "cash_transfer": true, "fx_conversion": true, "position_transfer": true, "trade": true, "debt_draw": true, "debt_payment": true}
	schema := workflowSchema[BatchChangeInput]("preview_batch")
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, branch := range schema.Properties["commands"].Items.OneOf {
		kind := branch.Properties["kind"].Enum[0].(string)
		if !expected[kind] || seen[kind] {
			t.Fatal("unexpected/duplicate advertised kind", kind)
		}
		seen[kind] = true
		input := map[string]any{"kind": kind}
		for _, field := range branch.Required {
			if field != "kind" {
				input[field] = "synthetic"
			}
		}
		if kind == "position_transfer" {
			input["toAccountId"] = "synthetic"
		}
		batch := singleLedgerInput(input)
		if err := resolved.Validate(&batch); err != nil {
			t.Fatalf("%s: advertised shape invalid: %v", kind, err)
		}
		raw, _ := jsoncodec.Marshal(input)
		var command history.ChangeCommandRequest
		if err := jsoncodec.Unmarshal(raw, &command); err != nil {
			t.Fatal(err)
		}
		if err := validateChangeRequest(command); err != nil {
			t.Fatalf("%s: schema advertises a handler-rejected command: %v", kind, err)
		}
	}
	if len(seen) != len(expected) {
		t.Fatal("supported commands missing", seen)
	}
	for _, input := range []history.ChangeCommandRequest{
		{Kind: history.ChangePositionAdjustment, HoldingID: "synthetic", Quantity: "1"},
		{Kind: history.ChangeValueUpdate, AccountID: "synthetic", NewValue: "1", NewValueCurrency: "USD", Reason: "income"},
	} {
		values := jsonObject(t, input)
		if err := resolved.Validate(&values); err == nil {
			t.Fatal("unsupported kind accepted by schema", input.Kind)
		}
		if err := validateChangeRequest(input); err == nil {
			t.Fatal("handler authority expanded", input.Kind)
		}
	}
}
