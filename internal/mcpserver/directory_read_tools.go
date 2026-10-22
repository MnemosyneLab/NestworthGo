package mcpserver

import (
	"context"
	"encoding/json"
	"sort"
)

// One projection for legacy discovery arrays that can grow with a household.
// The optional mode preserves full legacy contracts; item reads retain notes.
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
			row, e := readSummary(full)
			if e != nil {
				return c, e
			}
			compact := row
			if account, ok := row["account"].(map[string]any); ok {
				compact = account
			}
			for _, key := range []string{"note", "iconKey"} {
				delete(compact, key)
			}
			row["id"] = id
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i]["id"].(string) < rows[j]["id"].(string) })
		c.Sections[section], e = readRows(rows)
		return c, e
	})
}
