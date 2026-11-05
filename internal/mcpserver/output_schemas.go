package mcpserver

// Read output schemas are deliberately compact. The same `mcp-read/1` envelope is
// repeated by about thirty tools, so its fully typed form cost roughly 1.4 KB per
// tool in every tools/list. The wire schema now names only what a client needs to
// recognise and continue a read: the envelope version, the capture identity and
// the paging fields. The strict contract (field types, the summary/page
// exclusion, delivery, frozen-page counters) is kept as a test oracle in
// output_schemas_full_test.go, and real responses are validated against it.
// Write and recovery receipts deliberately acquire no output schema at all.
//
// Consequence: a client that validates structuredContent against the discovered
// schema no longer rejects, for example, a non-boolean hasMore. The server's own
// tests still do.
const (
	readKindEnvelope     = "envelope"
	readKindBusinessPage = "business_page"
	readKindUnifiedPage  = "unified_page"
	readKindContext      = "financial_context"
	readKindComparison   = "financial_comparison"
	readKindRepairJob    = "repair_job"
	readKindMarketData   = "market_data"
)

// readOutputKind classifies every tool that has a read output schema. It is the
// single membership list for both the shipped and the test-oracle schemas.
func readOutputKind(name string) string {
	if paginatedLiveTool(name) {
		return readKindBusinessPage
	}
	switch name {
	case "get_read_page":
		return readKindUnifiedPage
	case "get_market_data":
		return readKindMarketData
	case "get_read_item", "list_members", "list_institutions", "list_groups", "list_accounts", "list_holdings", "list_instruments", "get_account_valuations", "get_account_snapshot", "get_overview", "get_historical_overview", "list_agent_market_data", "compare_market_data_import", "list_products", "get_product", "get_liquidity_overview", "get_upcoming_actions", "scan_data_health", "preview_data_repair", "preview_investment_review_rule", "analyze_period", "list_contributions", "get_contribution_item", "get_return_day", "get_asset_driver_detail":
		return readKindEnvelope
	case "get_financial_context":
		return readKindContext
	case "compare_financial_context", "compare_financial_attribution":
		return readKindComparison
	case "get_data_repair_job":
		return readKindRepairJob
	}
	return ""
}

func readOutputSchema(name string) any {
	var data any
	switch readOutputKind(name) {
	case readKindBusinessPage:
		data = compactPageSchema(false)
	case readKindUnifiedPage:
		data = compactPageSchema(true)
	case readKindEnvelope:
		data = compactEnvelopeSchema()
	case readKindContext:
		data = compactFinancialSchema("contextId")
	case readKindComparison:
		data = compactFinancialSchema("comparisonId")
	case readKindRepairJob:
		progress := map[string]any{"required": []string{"job", "healthIncluded"}, "properties": map[string]any{"healthIncluded": map[string]any{"const": false}}}
		data = map[string]any{"oneOf": []any{progress, compactEnvelopeSchema()}}
	case readKindMarketData:
		data = map[string]any{"oneOf": []any{currentQuoteOutputSchema(), compactEnvelopeSchema()}}
	default:
		return nil
	}
	return map[string]any{"type": "object", "properties": map[string]any{"data": data}, "required": []string{"data"}}
}

func compactEnvelopeSchema() any {
	return map[string]any{"type": "object", "required": []string{"schemaVersion", "captureId", "expiresAt", "contentHash"}, "properties": map[string]any{"schemaVersion": map[string]any{"const": "mcp-read/1"}}}
}

// currentQuoteOutputSchema is the single-target get_market_data result.
// Batch calls on the same tool return the read envelope instead.
func currentQuoteOutputSchema() any {
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"current"},
		"properties": map[string]any{"current": map[string]any{"anyOf": []any{map[string]any{"type": "object"}, map[string]any{"type": "null"}}}},
	}
}

func compactPageSchema(versioned bool) any {
	props := map[string]any{"page": map[string]any{"type": "object", "required": []string{"hasMore", "nextCursor"}}}
	if versioned {
		props["schemaVersion"] = map[string]any{"const": "mcp-read/1"}
	}
	return map[string]any{"type": "object", "required": []string{"items", "page"}, "properties": props}
}

func compactFinancialSchema(id string) any {
	return map[string]any{"type": "object", "required": []string{id, "contentHash", "content"}}
}
