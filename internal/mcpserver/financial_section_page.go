package mcpserver

import (
	"encoding/json"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Continuations refer to the initial immutable header by ID and content hash.
// Only the requested section's rows and progress are returned. In particular,
// comparison attribution/precision and context basis are not resent per page.
type FinancialSectionResponse struct {
	ContextID      string                   `json:"contextId,omitempty"`
	ComparisonID   string                   `json:"comparisonId,omitempty"`
	ContentHash    string                   `json:"contentHash"`
	CapturedAt     string                   `json:"capturedAt"`
	CacheExpiresAt string                   `json:"cacheExpiresAt"`
	Section        string                   `json:"section"`
	Items          []json.RawMessage        `json:"items"`
	Page           FinancialContextPageInfo `json:"page"`
}

func (c *financialContextCache) contextSectionResponse(id string, e cachedFinancialContext, g uint64, section string, offset, limit int) (FinancialSectionResponse, error) {
	r := FinancialSectionResponse{ContextID: id, ContentHash: e.result.ContentHash, CapturedAt: e.result.CapturedAt.UTC().Format(time.RFC3339Nano), CacheExpiresAt: e.expires.UTC().Format(time.RFC3339Nano), Section: section}
	switch section {
	case "positions":
		return financialSectionPage(c, r, id, g, offset, limit, e.result.Content.Positions)
	case "gaps":
		return financialSectionPage(c, r, id, g, offset, limit, e.result.Content.Gaps)
	case "evidence":
		return financialSectionPage(c, r, id, g, offset, limit, e.result.Content.Evidence)
	}
	return r, fail("validation", "unknown section")
}

func (c *financialContextCache) comparisonSectionResponse(id string, e cachedFinancialContext, g uint64, section string, offset, limit int) (FinancialSectionResponse, error) {
	r := FinancialSectionResponse{ComparisonID: id, ContentHash: e.comparison.ContentHash, CapturedAt: e.comparison.CapturedAt.UTC().Format(time.RFC3339Nano), CacheExpiresAt: e.expires.UTC().Format(time.RFC3339Nano), Section: section}
	switch section {
	case "positions":
		return financialSectionPage(c, r, id, g, offset, limit, e.comparison.Content.Positions)
	case "gaps":
		return financialSectionPage(c, r, id, g, offset, limit, e.comparison.Content.Gaps)
	case "evidence":
		return financialSectionPage(c, r, id, g, offset, limit, e.comparison.Content.Evidence)
	}
	return r, fail("validation", "unknown section")
}

func financialSectionPage[T any](c *financialContextCache, r FinancialSectionResponse, id string, g uint64, offset, limit int, rows []T) (FinancialSectionResponse, error) {
	if offset < 0 || offset >= len(rows) {
		return r, fail("validation", "cursor offset is past the section")
	}
	r.Items = []json.RawMessage{}
	for i := offset; i < len(rows) && len(r.Items) < limit; i++ {
		row, err := jsoncodec.Marshal(rows[i])
		if err != nil {
			return r, err
		}
		r.Items = append(r.Items, row)
		r.Page = c.pageInfo(id, r.Section, g, offset, len(r.Items), len(rows))
		fits, err := readFits(r)
		if err != nil {
			return r, err
		}
		if !fits {
			r.Items = r.Items[:len(r.Items)-1]
			if len(r.Items) == 0 {
				return r, fail("too_large", "single section row exceeds the MCP wire budget; narrow scope or use the App")
			}
			break
		}
	}
	r.Page = c.pageInfo(id, r.Section, g, offset, len(r.Items), len(rows))
	return r, nil
}
