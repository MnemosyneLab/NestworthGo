package mcpserver

import (
	"github.com/google/jsonschema-go/jsonschema"
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
		{"market targets and windows", workflowSchema[MarketBatchInput]("get_market_data_batch"),
			[]map[string]any{{"targets": []any{map[string]any{"instrumentId": "id"}}}, {"targets": []any{map[string]any{"baseCurrency": "USD", "quoteCurrency": "CNY"}}, "view": "history", "historyLayout": "records", "fromDate": "2026-09-01", "toDate": "2026-09-02"}},
			[]map[string]any{{"targets": []any{map[string]any{}}}, {"targets": []any{map[string]any{"instrumentId": "id", "baseCurrency": "USD", "quoteCurrency": "CNY"}}}, {"targets": []any{map[string]any{"baseCurrency": "USD"}}}, {"targets": []any{map[string]any{"instrumentId": "id"}}, "fromDate": "2026-09-01"}, {"targets": []any{map[string]any{"instrumentId": "id"}}, "historyLayout": "records"}, {"targets": []any{map[string]any{"instrumentId": "id"}}, "fromDate": "2026-09-01", "toDate": "2026-09-02", "range": "30d"}}},
		{"ledger kind", workflowSchema[history.ChangeCommandRequest]("preview_change"), nil, nil},
		{"repair view", workflowSchema[DataRepairJobInput]("get_data_repair_job"), []map[string]any{{"jobId": "id"}, {"jobId": "id", "view": "evidence"}}, []map[string]any{{"jobId": "id", "view": "full"}}},
	}
	cases[2].valid = []map[string]any{{"kind": "money_added", "accountId": "a", "amount": "1.0001", "currency": "USD", "reason": "income"}, {"kind": "position_transfer", "fromHoldingId": "a", "toAccountId": "b", "quantity": "1"}}
	cases[2].invalid = []map[string]any{{"kind": "money_added"}, {"kind": "invented"}, {"kind": "position_transfer", "fromHoldingId": "a", "quantity": "1"}, {"kind": "position_transfer", "fromHoldingId": "a", "toHoldingId": "b", "toAccountId": "c", "quantity": "1"}, {"kind": "money_added", "accountId": "a", "amount": "1", "currency": "USD", "reason": "income", "effectiveLocalDate": "2026-09-01"}}
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
