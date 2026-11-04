package mcpserver

// Stable read envelopes describe transport shape, not a lossy GUI projection.
// Summary/row payloads retain exact existing business values and their owning
// contracts. Write/recovery receipts deliberately acquire no new output gate.
func readOutputSchema(name string) any {
	if paginatedLiveTool(name) {
		return wrapReadOutput(businessPageSchema())
	}
	switch name {
	case "get_read_page":
		return wrapReadOutput(unifiedPageSchema())
	case "get_read_item", "list_members", "list_institutions", "list_groups", "list_accounts", "list_holdings", "list_instruments", "get_account_valuations", "get_account_snapshot", "get_overview", "get_historical_overview", "get_historical_overview_series", "get_market_data_batch", "list_agent_market_data", "compare_market_data_import", "list_products", "get_product", "get_liquidity_overview", "get_upcoming_actions", "scan_data_health", "preview_data_repair", "preview_investment_review_rule", "analyze_period", "list_contributions", "get_contribution_item", "get_return_day", "get_asset_driver_detail":
		return wrapReadOutput(readEnvelopeSchema())
	case "get_financial_context":
		return wrapReadOutput(financialOutputEnvelope("contextId", false))
	case "compare_financial_context", "compare_financial_attribution":
		return wrapReadOutput(financialOutputEnvelope("comparisonId", false))
	case "get_financial_context_page":
		return wrapReadOutput(financialOutputEnvelope("contextId", true))
	case "get_financial_comparison_page":
		return wrapReadOutput(financialOutputEnvelope("comparisonId", true))
	case "get_data_repair_job":
		fields := map[string]any{}
		for _, k := range []string{"jobId", "phase", "outcome", "errorCode"} {
			fields[k] = map[string]any{"type": "string"}
		}
		for _, k := range []string{"sequence", "estimatedRequests", "completedRequests", "targetCount", "completedTargets", "snapshotDaysPlanned", "snapshotDaysRebuilt", "committedBatches", "itemCount", "blockerCount", "prerequisiteCount"} {
			fields[k] = map[string]any{"type": "integer"}
		}
		fields["nextEligibilityAt"] = map[string]any{"type": []string{"string", "null"}}
		progress := map[string]any{"type": "object", "required": []string{"job", "healthIncluded"}, "properties": map[string]any{"job": map[string]any{"type": "object", "properties": fields, "required": []string{"jobId", "sequence", "phase", "outcome", "itemCount", "blockerCount", "prerequisiteCount"}}, "healthIncluded": map[string]any{"const": false}}, "additionalProperties": false}
		return wrapReadOutput(map[string]any{"oneOf": []any{progress, readEnvelopeSchema()}})
	}
	return nil
}

func wrapReadOutput(data any) any {
	out := map[string]any{"type": "object", "properties": map[string]any{"data": data}, "required": []string{"data"}, "additionalProperties": false}
	if usesFrozenPageSchema(data) {
		out["$defs"] = map[string]any{"frozenPage": frozenPageSchema()}
	}
	return out
}

const frozenPageSchemaRef = "#/$defs/frozenPage"

func frozenPageSchema() any {
	return map[string]any{"type": "object", "required": []string{"total", "returned", "hasMore", "nextCursor", "consistency"}, "properties": map[string]any{
		"total": map[string]any{"type": "integer", "minimum": 0}, "returned": map[string]any{"type": "integer", "minimum": 0}, "hasMore": map[string]any{"type": "boolean"}, "nextCursor": map[string]any{"type": []string{"string", "null"}}, "consistency": map[string]any{"const": "frozen"},
	}}
}
func usesFrozenPageSchema(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if v["$ref"] == frozenPageSchemaRef {
			return true
		}
		for _, child := range v {
			if usesFrozenPageSchema(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if usesFrozenPageSchema(child) {
				return true
			}
		}
	}
	return false
}

func readEnvelopeSchema() any {
	page := map[string]any{"$ref": frozenPageSchemaRef}
	rows := map[string]any{"type": "array", "items": true}
	props := map[string]any{}
	required := []string{"schemaVersion", "captureId", "captureKind", "capturedAt", "expiresAt", "contentHash", "summaryHash"}
	for _, k := range required {
		props[k] = map[string]any{"type": "string"}
	}
	props["schemaVersion"] = map[string]any{"const": "mcp-read/1"}
	props["query"] = true
	props["summary"] = true
	props["sections"] = map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "object", "properties": map[string]any{"items": rows, "page": page}, "required": []string{"items", "page"}}}
	props["section"] = map[string]any{"type": "string"}
	props["items"] = rows
	props["page"] = page
	return map[string]any{"type": "object", "properties": props, "required": required, "oneOf": []any{map[string]any{"required": []string{"summary"}, "not": map[string]any{"required": []string{"page"}}}, map[string]any{"required": []string{"section", "items", "page"}, "not": map[string]any{"required": []string{"summary"}}}}}
}

func financialOutputEnvelope(id string, continuation bool) any {
	required := []string{id, "contentHash", "capturedAt", "cacheExpiresAt"}
	props := map[string]any{}
	for _, k := range required {
		props[k] = map[string]any{"type": "string"}
	}
	page := map[string]any{"$ref": frozenPageSchemaRef}
	if continuation {
		required = append(required, "section", "items", "page")
		props["section"] = map[string]any{"enum": []string{"positions", "gaps", "evidence"}}
		props["items"] = map[string]any{"type": "array", "items": true}
		props["page"] = page
	} else {
		required = append(required, "generatedAt", "content", "positionsPage", "gapsPage", "evidencePage")
		props["generatedAt"] = map[string]any{"type": "string"}
		props["content"] = map[string]any{"type": "object"}
		for _, k := range []string{"positionsPage", "gapsPage", "evidencePage"} {
			props[k] = page
		}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func businessPageSchema() any {
	return map[string]any{"type": "object", "required": []string{"items", "page"}, "properties": map[string]any{
		"items": map[string]any{"type": "array", "items": true},
		"page": map[string]any{"type": "object", "required": []string{"returned", "hasMore", "nextCursor", "consistency"}, "properties": map[string]any{
			"returned":    map[string]any{"type": "integer", "minimum": 0},
			"total":       map[string]any{"type": "integer", "minimum": 0},
			"hasMore":     map[string]any{"type": "boolean"},
			"nextCursor":  map[string]any{"type": []string{"string", "null"}},
			"consistency": map[string]any{"enum": []string{"live", "version_bound", "frozen"}},
		}},
	}}
}

func unifiedPageSchema() any {
	page := businessPageSchema().(map[string]any)
	page["allOf"] = []any{map[string]any{
		"if":   map[string]any{"properties": map[string]any{"page": map[string]any{"properties": map[string]any{"consistency": map[string]any{"const": "frozen"}}}}},
		"then": map[string]any{"required": []string{"contentHash", "capturedAt"}, "anyOf": []any{map[string]any{"required": []string{"captureId", "expiresAt"}}, map[string]any{"required": []string{"contextId", "cacheExpiresAt"}}, map[string]any{"required": []string{"comparisonId", "cacheExpiresAt"}}}},
	}}
	page["properties"].(map[string]any)["schemaVersion"] = map[string]any{"const": "mcp-read/1"}
	return map[string]any{"anyOf": []any{page, map[string]any{"type": "object", "required": []string{"contextId", "ref", "contentHash", "capturedAt", "cacheExpiresAt", "positionsPage", "gapsPage", "evidencePage"}}}}
}
