package mcpserver

import (
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestSlimSchemasKeepKindRequirementsAndShrink(t *testing.T) {
	change := roundTripSchema(t, workflowSchema[BatchChangeInput]("preview_batch"))
	changeRaw := mustSchemaJSON(t, change)
	// Compare the same finite contract before/after compression, not the old
	// incomplete schema that lacked enums and field exclusions.
	expanded, err := jsonschema.For[BatchChangeInput](nil)
	if err != nil {
		t.Fatal(err)
	}
	addLedgerSchema(expanded.Properties["commands"].Items, false)
	if len(mustSchemaJSON(t, change.Properties["commands"].Items)) >= len(mustSchemaJSON(t, expanded.Properties["commands"].Items)) {
		t.Fatal("ledger schema compression regressed")
	}

	if len(mustSchemaJSON(t, change.Properties["commands"].Items)) >= 5000 {
		t.Fatalf("preview_batch schema is %d bytes; bounded enum/variant contract budget is 5000", len(changeRaw))
	}
	if strings.Contains(mustSchemaJSON(t, change.Properties["commands"].Items), `"description"`) || strings.Contains(changeRaw, "newValue") || strings.Contains(changeRaw, "mutationId") {
		t.Fatalf("preview_batch schema still publishes removed prose or handler-rejected fields: %s", changeRaw)
	}
	resolved, err := change.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(&map[string]any{"commands": []any{map[string]any{"kind": "money_added", "accountId": "a", "amount": "1", "currency": "USD", "reason": "income", "note": nil}}}); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []map[string]any{
		{"kind": "money_added", "accountId": 1, "amount": "1", "currency": "USD", "reason": "income"},
		{"kind": "money_added", "accountId": "a", "amount": "1", "currency": "USD", "reason": "income", "newValue": "1"},
		{"kind": "money_added", "accountId": "a", "amount": "1", "currency": "USD", "reason": "income", "fee": "1"},
	} {
		batch := singleLedgerInput(invalid)
		if err := resolved.Validate(&batch); err == nil {
			t.Fatalf("invalid change accepted: %v", invalid)
		}
	}
	batch := singleLedgerInput(map[string]any{"kind": "trade", "settlementAccountId": "a", "instrumentId": "i", "side": "buy", "quantity": "1", "gross": "1", "grossCurrency": "USD", "reconciliationLink": map[string]any{"sessionId": "s", "accountId": "a", "itemId": "i", "expectedRevision": 3, "financialRevision": 12}})
	if err := resolved.Validate(&batch); err != nil {
		t.Fatal(err)
	}

	product := roundTripSchema(t, productOperationInputSchema())
	productRaw := mustSchemaJSON(t, product)
	if len(productRaw) >= 7609 {
		t.Fatalf("preview_product_operation schema is %d bytes; measured baseline is 7609", len(productRaw))
	}
	if strings.Contains(productRaw, `"description"`) || strings.Contains(productRaw, "releaseReservationIds") || strings.Contains(productRaw, "reviewedAt") {
		t.Fatalf("product schema published prose or a forbidden field: %s", productRaw)
	}
	if strings.Count(productRaw, `"$ref":"#/$defs/productTerms"`) != 3 || strings.Count(productRaw, `"$ref":"#/$defs/productPolicy"`) != 3 || strings.Count(productRaw, `"$ref":"#/$defs/productSettle"`) != 2 {
		t.Fatalf("shared product refs were inlined or dropped: %s", productRaw)
	}
	assertRequired(t, productRaw, "productTerms", "kind", "name", "note", "startOn", "maturityOn", "interestMode", "annualRate", "annualRatePercent", "maturityInterest", "interestPaidThroughOn")
	assertRequired(t, productRaw, "productPolicy", "accessibleAmountCap", "accessKind", "unlockOn", "settlementDays", "dayBasis", "receiptOnOverride", "normalExitFee", "earlyKind", "earlySettlementDays", "earlyDayBasis", "earlyFee", "earlyAmountMode", "earlyGrossAmount", "note")
	productResolved, err := product.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	open := map[string]any{"kind": "open", "open": productPayload("open")}
	if err := productResolved.Validate(&open); err != nil {
		t.Fatal(err)
	}
	renew := map[string]any{"kind": "renew", "renew": map[string]any{
		"principal": "1000", "openingFee": nil, "terms": open["open"].(map[string]any)["terms"], "policy": open["open"].(map[string]any)["policy"],
		"settle": map[string]any{"productId": "p", "returnedPrincipal": "1000", "interest": "40", "grossProceeds": nil, "fee": "2", "effectiveAt": ""},
	}}
	if err := productResolved.Validate(&renew); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []map[string]any{
		{"kind": "open"},
		{"kind": "open", "open": productPayload("open"), "undo": map[string]any{"operationId": "op"}},
		{"kind": "invented", "undo": map[string]any{"operationId": "op"}},
		{"kind": "settle", "settle": map[string]any{"productId": "p", "releaseReservationIds": []any{"r"}}},
		{"kind": "renew", "renew": map[string]any{}},
	} {
		if err := productResolved.Validate(&invalid); err == nil {
			t.Fatalf("invalid product operation accepted: %v", invalid)
		}
	}
	t.Logf("schema bytes preview_batch=%d preview_product_operation=%d", len(changeRaw), len(productRaw))
}

func productPayload(kind string) map[string]any {
	terms := map[string]any{"kind": "term_deposit", "name": "Synthetic deposit", "note": nil, "startOn": "2026-09-01", "maturityOn": "2026-09-30", "interestMode": "manual_maturity_amount", "annualRate": nil, "annualRatePercent": nil, "maturityInterest": "50", "interestPaidThroughOn": nil}
	policy := map[string]any{"accessibleAmountCap": nil, "accessKind": "on_date", "unlockOn": "2026-09-30", "settlementDays": 0, "dayBasis": "calendar", "receiptOnOverride": nil, "normalExitFee": nil, "earlyKind": "not_allowed", "earlySettlementDays": nil, "earlyDayBasis": nil, "earlyFee": nil, "earlyAmountMode": nil, "earlyGrossAmount": nil, "note": nil}
	switch kind {
	case "open":
		return map[string]any{"accountId": "a", "currency": "USD", "principal": "1000", "openingFee": nil, "effectiveAt": "", "terms": terms, "policy": policy}
	default:
		return nil
	}
}

func assertRequired(t *testing.T, raw, def string, fields ...string) {
	t.Helper()
	var schema map[string]any
	if err := jsoncodec.Unmarshal([]byte(raw), &schema); err != nil {
		t.Fatal(err)
	}
	body, _ := schema["$defs"].(map[string]any)[def].(map[string]any)
	got := map[string]bool{}
	for _, field := range body["required"].([]any) {
		got[field.(string)] = true
	}
	for _, field := range fields {
		if !got[field] {
			t.Errorf("%s missing required %s", def, field)
		}
	}
}

func roundTripSchema(t *testing.T, s *jsonschema.Schema) *jsonschema.Schema {
	t.Helper()
	var out jsonschema.Schema
	if err := jsoncodec.Unmarshal([]byte(mustSchemaJSON(t, s)), &out); err != nil {
		t.Fatal(err)
	}
	return &out
}

func mustSchemaJSON(t *testing.T, s *jsonschema.Schema) string {
	t.Helper()
	raw, err := jsoncodec.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
