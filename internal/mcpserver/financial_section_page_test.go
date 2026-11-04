package mcpserver

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Existing union/parity tests accumulate typed rows in the initial header DTO.
// Adapt only the test accumulator; production clients consume section/items/page.
func financialPageTestData(t *testing.T, data any) any {
	t.Helper()
	obj, ok := data.(map[string]any)
	if !ok || obj["section"] == nil {
		return data
	}
	allowed := map[string]bool{"ref": true, "contextId": true, "comparisonId": true, "contentHash": true, "capturedAt": true, "cacheExpiresAt": true, "section": true, "items": true, "page": true}
	for key := range obj {
		if !allowed[key] {
			t.Fatalf("continuation repeats header or unrelated progress: %s", key)
		}
	}
	var page FinancialSectionResponse
	raw, err := jsoncodec.Marshal(obj)
	if err != nil || jsoncodec.Unmarshal(raw, &page) != nil {
		t.Fatal("invalid section page", err)
	}
	if page.Page.Returned != len(page.Items) || page.Page.Returned == 0 || page.ContentHash == "" {
		t.Fatal("invalid section progress", page)
	}
	return map[string]any{"contextId": page.ContextID, "comparisonId": page.ComparisonID, "contentHash": page.ContentHash, "capturedAt": page.CapturedAt, "cacheExpiresAt": page.CacheExpiresAt, "content": map[string]any{page.Section: page.Items}, page.Section + "Page": page.Page}
}
