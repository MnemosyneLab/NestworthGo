package mcpserver

import (
	"context"
	"encoding/json"
	"sort"
)

// Frozen compact directory rows; item reads retain complete notes and metadata.
func (s *Service) pagedDirectory(ctx context.Context, g uint64, section string, query any, limit int, values any) (ReadEnvelope, error) {
	return s.buildRead(ctx, g, limit, func(context.Context) (readCapture, error) {
		encoded, e := readSummary(map[string]any{"items": values})
		if e != nil {
			return readCapture{}, e
		}
		items, ok := encoded["items"].([]any)
		if !ok {
			items = []any{}
		}
		rows := []map[string]any{}
		c := readCapture{Kind: "directory", Query: query, Summary: map[string]any{"section": section, "matchedCount": len(items), "countsScope": "request_filters"}, Sections: map[string][]json.RawMessage{}, Details: map[string]any{}}
		for _, item := range items {
			full := item.(map[string]any)
			entity := full
			if account, ok := full["account"].(map[string]any); ok {
				entity = account
			}
			id, ok := entity["id"].(string)
			if !ok {
				return c, fail("internal", "directory item has no identity")
			}
			c.Details[section+"/"+id] = full
			var row map[string]any
			switch section {
			case "members", "groups":
				row = readFields(entity, "id", "name", "archivedAt")
			case "institutions":
				row = readFields(entity, "id", "name", "institutionType", "countryCode", "archivedAt")
			case "accounts":
				row = readFields(full, "ownership", "institutionName", "groupName")
				row["account"] = readFields(entity, "id", "name", "institutionId", "groupId", "accountType", "balanceSheetRole", "trackingMode", "defaultCurrency", "includeInNetWorth", "includeInPortfolio", "includeInLiquidAssets", "openedOn", "closedOn", "archivedAt")
				row["id"] = id
			case "holdings":
				row = readFields(entity, "id", "accountId", "instrumentId", "quantity", "archivedAt")
			default:
				return c, fail("internal", "unknown directory section")
			}
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i]["id"].(string) < rows[j]["id"].(string) })
		c.Sections[section], e = readRows(rows)
		return c, e
	})
}

// Explicit projections prevent future GUI metadata from expanding MCP lists.
// Values are from a fresh serialization; the complete frozen detail is retained.
func readFields(full map[string]any, keys ...string) map[string]any {
	row := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := full[key]; ok {
			row[key] = value
		}
	}
	return row
}
