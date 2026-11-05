package mcpserver

// The shipped outputSchema of every mcp-read/1 tool is deliberately compact (see
// output_schemas.go). This file keeps the strict contract the earlier schemas
// described as a test oracle: real responses are validated against it, so the
// wire schema can stay small without letting the response shape drift.
func fullReadOutputSchema(name string) any {
	switch readOutputKind(name) {
	case readKindBusinessPage:
		return fullWrapReadOutput(fullBusinessPageSchema())
	case readKindUnifiedPage:
		return fullWrapReadOutput(fullUnifiedPageSchema())
	case readKindEnvelope:
		return fullWrapReadOutput(fullReadEnvelopeSchema())
	case readKindContext:
		return fullWrapReadOutput(fullFinancialOutputEnvelope("contextId"))
	case readKindComparison:
		return fullWrapReadOutput(fullFinancialOutputEnvelope("comparisonId"))
	case readKindMarketData:
		return fullWrapReadOutput(map[string]any{"oneOf": []any{currentQuoteOutputSchema(), fullReadEnvelopeSchema()}})
	case readKindRepairJob:
		fields := map[string]any{}
		for _, k := range []string{"jobId", "phase", "outcome", "errorCode"} {
			fields[k] = map[string]any{"type": "string"}
		}
		for _, k := range []string{"sequence", "estimatedRequests", "completedRequests", "targetCount", "completedTargets", "snapshotDaysPlanned", "snapshotDaysRebuilt", "committedBatches", "itemCount", "blockerCount", "prerequisiteCount"} {
			fields[k] = map[string]any{"type": "integer"}
		}
		fields["nextEligibilityAt"] = map[string]any{"type": []string{"string", "null"}}
		progress := map[string]any{"type": "object", "required": []string{"job", "healthIncluded"}, "properties": map[string]any{"job": map[string]any{"type": "object", "properties": fields, "required": []string{"jobId", "sequence", "phase", "outcome", "itemCount", "blockerCount", "prerequisiteCount"}}, "healthIncluded": map[string]any{"const": false}}, "additionalProperties": false}
		return fullWrapReadOutput(map[string]any{"oneOf": []any{progress, fullReadEnvelopeSchema()}})
	}
	return nil
}

func fullWrapReadOutput(data any) any {
	out := map[string]any{"type": "object", "properties": map[string]any{"data": data}, "required": []string{"data"}, "additionalProperties": false}
	if fullUsesFrozenPageSchema(data) {
		out["$defs"] = map[string]any{"frozenPage": fullFrozenPageSchema()}
	}
	return out
}

const fullFrozenPageSchemaRef = "#/$defs/frozenPage"

func fullFrozenPageSchema() any {
	return map[string]any{"type": "object", "required": []string{"total", "returned", "hasMore", "nextCursor", "consistency"}, "properties": map[string]any{
		"total": map[string]any{"type": "integer", "minimum": 0}, "returned": map[string]any{"type": "integer", "minimum": 0}, "hasMore": map[string]any{"type": "boolean"}, "nextCursor": map[string]any{"type": []string{"string", "null"}}, "consistency": map[string]any{"const": "frozen"},
	}}
}
func fullUsesFrozenPageSchema(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if v["$ref"] == fullFrozenPageSchemaRef {
			return true
		}
		for _, child := range v {
			if fullUsesFrozenPageSchema(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if fullUsesFrozenPageSchema(child) {
				return true
			}
		}
	}
	return false
}

func fullReadEnvelopeSchema() any {
	page := map[string]any{"$ref": fullFrozenPageSchemaRef}
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
	// delivery exists only on a top-level initial capture. Continuation pages and
	// focused item details (get_read_item, comparison items) never carry it, so the
	// property is optional on the envelope but exact when present.
	props["delivery"] = map[string]any{"type": "object", "required": []string{"selectionComplete", "pendingSections"}, "properties": map[string]any{"selectionComplete": map[string]any{"type": "boolean"}, "pendingSections": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "additionalProperties": false}
	props["section"] = map[string]any{"type": "string"}
	props["items"] = rows
	props["page"] = page
	return map[string]any{"type": "object", "properties": props, "required": required, "oneOf": []any{map[string]any{"required": []string{"summary"}, "not": map[string]any{"required": []string{"page"}}}, map[string]any{"required": []string{"section", "items", "page"}, "not": map[string]any{"required": []string{"summary"}}}}}
}

func fullFinancialOutputEnvelope(id string) any {
	required := []string{id, "contentHash", "capturedAt", "cacheExpiresAt", "generatedAt", "content", "positionsPage", "gapsPage", "evidencePage"}
	props := map[string]any{}
	for _, k := range required {
		props[k] = map[string]any{"type": "string"}
	}
	page := map[string]any{"$ref": fullFrozenPageSchemaRef}
	props["content"] = map[string]any{"type": "object"}
	for _, k := range []string{"positionsPage", "gapsPage", "evidencePage"} {
		props[k] = page
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func fullBusinessPageSchema() any {
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

func fullUnifiedPageSchema() any {
	page := fullBusinessPageSchema().(map[string]any)
	page["allOf"] = []any{map[string]any{
		"if":   map[string]any{"properties": map[string]any{"page": map[string]any{"properties": map[string]any{"consistency": map[string]any{"const": "frozen"}}}}},
		"then": map[string]any{"required": []string{"contentHash", "capturedAt"}, "anyOf": []any{map[string]any{"required": []string{"captureId", "expiresAt"}}, map[string]any{"required": []string{"contextId", "cacheExpiresAt"}}, map[string]any{"required": []string{"comparisonId", "cacheExpiresAt"}}}},
	}}
	page["properties"].(map[string]any)["schemaVersion"] = map[string]any{"const": "mcp-read/1"}
	return page
}
