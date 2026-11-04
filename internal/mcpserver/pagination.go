package mcpserver

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// The outer cursor authenticates a live reader's complete query. Its native
// cursor and revision checks remain authoritative. Frozen cursors already carry
// authenticated routing information, so are reused without another wrapper.
const pageCursorPrefix = "p1."
const pageCursorLimit = 16384

type pageCodec struct {
	key        [32]byte
	generation uint64
}
type pageRoute struct {
	Tool       string          `json:"tool"`
	Query      json.RawMessage `json:"query"`
	Generation uint64          `json:"generation"`
}
type pageProjectKey struct{}
type pageProjection func(any) (any, error)

func paginatedLiveTool(name string) bool {
	switch name {
	case "list_activities", "list_product_operations", "list_investment_review_rules", "list_investment_review_suppressions", "list_investment_review_alerts", "list_investment_review_daily_evidence", "list_investment_review_episode_history", "list_investment_review_episode_events", "get_investment_review_coverage", "list_reconciliation_sessions", "get_reconciliation_session_page":
		return true
	}
	return false
}
func reviewPageLimit(limit int) int {
	if limit == 0 {
		return 25
	}
	return limit
}
func readFitsProjected(ctx context.Context, value any) (bool, error) {
	if project, ok := ctx.Value(pageProjectKey{}).(pageProjection); ok {
		var err error
		value, err = project(value)
		if err != nil {
			return false, err
		}
	}
	return readFits(value)
}
func (c *pageCodec) encode(tool string, query map[string]any) (string, error) {
	delete(query, "limit")
	raw, err := jsoncodec.Marshal(query)
	if err != nil {
		return "", err
	}
	raw, err = jsoncodec.Marshal(pageRoute{tool, raw, c.generation})
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, c.key[:])
	mac.Write(raw)
	token := pageCursorPrefix + base64.RawURLEncoding.EncodeToString(append(raw, mac.Sum(nil)...))
	if len(token) > pageCursorLimit {
		return "", fail("too_large", "paging query exceeds cursor budget; narrow filters")
	}
	return token, nil
}
func (c *pageCodec) decode(token string) (pageRoute, error) {
	var route pageRoute
	if len(token) > pageCursorLimit || !strings.HasPrefix(token, pageCursorPrefix) {
		return route, fail("validation", "invalid page cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, pageCursorPrefix))
	if err != nil || len(raw) < sha256.Size {
		return route, fail("validation", "invalid page cursor")
	}
	body, sig := raw[:len(raw)-sha256.Size], raw[len(raw)-sha256.Size:]
	mac := hmac.New(sha256.New, c.key[:])
	mac.Write(body)
	if !hmac.Equal(sig, mac.Sum(nil)) || jsoncodec.Unmarshal(body, &route) != nil || !paginatedLiveTool(route.Tool) {
		return route, fail("validation", "invalid page cursor")
	}
	if route.Generation != c.generation {
		return route, fail("read_revoked", "page generation revoked; restart the original read")
	}
	return route, nil
}

func (set *toolSet) projectPage(name string, input, value any) (any, error) {
	if !paginatedLiveTool(name) {
		return value, nil
	}
	raw, err := jsoncodec.Marshal(input)
	if err != nil {
		return nil, err
	}
	query := map[string]any{}
	if err = decodeReadValue(raw, &query); err != nil {
		return nil, err
	}
	raw, err = jsoncodec.Marshal(value)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	var rows []any
	next := ""
	consistency := "version_bound"
	switch name {
	case "list_investment_review_rules", "list_investment_review_suppressions":
		consistency = "live"
		if err = decodeReadValue(raw, &rows); err != nil {
			return nil, err
		}
		// These identity readers have no lookahead. A nonempty page deliberately
		// requires one final empty page, even when shorter than the row budget.
		if len(rows) > 0 {
			next, _ = rows[len(rows)-1].(map[string]any)["id"].(string)
			if next == "" {
				next, _ = rows[len(rows)-1].(map[string]any)["ID"].(string)
			}
			if next == "" {
				return nil, fail("internal", "identity continuation unavailable")
			}
		}
	default:
		if err = decodeReadValue(raw, &out); err != nil {
			return nil, err
		}
		key := "items"
		switch name {
		case "list_activities":
			key = "activities"
			consistency = "live"
		case "list_product_operations":
			key = "operations"
			consistency = "live"
		}
		rows, _ = out[key].([]any)
		delete(out, key)
		if name == "list_activities" {
			if out["hasMore"] == true {
				cursor, ok := out["next"].(map[string]any)
				if !ok {
					return nil, fail("internal", "activity continuation unavailable")
				}
				query["afterEffectiveAt"], query["afterCreatedAt"], query["afterId"] = cursor["effectiveAt"], cursor["createdAt"], cursor["id"]
				next = "activity"
			}
		} else if name == "list_product_operations" {
			next, _ = out["next"].(string)
		} else {
			next, _ = out["nextCursor"].(string)
		}
	}
	if rows == nil {
		rows = []any{}
	}
	page := map[string]any{"returned": len(rows), "hasMore": next != "", "nextCursor": nil, "consistency": consistency}
	if total, ok := out["total"]; ok {
		page["total"] = total
		delete(out, "total")
	}
	if next != "" {
		if name != "list_activities" {
			query["cursor"] = next
		}
		cursor, err := set.paging.encode(name, query)
		if err != nil {
			return nil, err
		}
		page["nextCursor"] = cursor
	}
	for _, key := range []string{"next", "nextCursor", "hasMore"} {
		delete(out, key)
	}
	out["items"], out["page"] = rows, page
	return out, nil
}

func (s *Service) continuePage(ctx context.Context, set *toolSet, g uint64, in ReadPageInput) (any, error) {
	if _, err := readLimit(in.Limit); err != nil {
		return nil, err
	}
	if strings.HasPrefix(in.Cursor, pageCursorPrefix) {
		if in.CaptureID != "" || in.Section != "" {
			return nil, fail("validation", "cursor carries its own scope; omit captureId and section")
		}
		route, err := set.paging.decode(in.Cursor)
		if err != nil {
			return nil, err
		}
		// Check the original permission/lifetime fence before touching the reader.
		if !s.contexts.active(g) {
			return nil, fail("read_revoked", "page generation revoked; restart the original read")
		}
		read := set.pages[route.Tool]
		if read == nil {
			return nil, fail("validation", "page reader unavailable")
		}
		return read(ctx, route.Query, reviewPageLimit(in.Limit))
	}
	if cursor, err := s.reads.decode(in.Cursor); err == nil {
		if (in.CaptureID != "" && in.CaptureID != cursor.CaptureID) || (in.Section != "" && in.Section != cursor.Section) {
			return nil, fail("validation", "cursor must match capture and section")
		}
		in.CaptureID, in.Section = cursor.CaptureID, cursor.Section
		return s.reads.page(g, in)
	}
	cursor, err := s.contexts.decodeCursor(in.Cursor)
	if err != nil {
		return nil, fail("validation", "invalid page cursor")
	}
	if (in.CaptureID != "" && in.CaptureID != cursor.ContextID) || (in.Section != "" && in.Section != cursor.Section) {
		return nil, fail("validation", "cursor must match financial capture and section")
	}
	// Type and item scope are recovered only from authenticated routing fields;
	// all native TTL/hash/generation/ref validation still runs in the owner.
	s.contexts.mu.Lock()
	entry, exists := s.contexts.entries[cursor.ContextID]
	comparison := exists && entry.comparison != nil
	s.contexts.mu.Unlock()
	if strings.HasPrefix(cursor.Section, "item:") {
		target := strings.TrimPrefix(cursor.Section, "item:")
		split := strings.LastIndex(target, ":")
		if split < 1 {
			return nil, fail("validation", "invalid item page scope")
		}
		return s.financialContextItem(ctx, g, FinancialContextItemInput{ContextID: cursor.ContextID, Ref: target[:split], Section: target[split+1:], Cursor: in.Cursor, Limit: in.Limit})
	}
	if comparison {
		return s.financialComparisonPage(ctx, g, FinancialComparisonPageInput{ComparisonID: cursor.ContextID, Section: cursor.Section, Cursor: in.Cursor, Limit: in.Limit})
	}
	return s.financialContextPage(ctx, g, FinancialContextPageInput{ContextID: cursor.ContextID, Section: cursor.Section, Cursor: in.Cursor, Limit: in.Limit})
}

// Preserve the internal string cursor API while publishing one terminal null
// convention across frozen and live business pages.
func (p FinancialContextPageInfo) MarshalJSON() ([]byte, error) {
	var next *string
	if p.NextCursor != "" {
		next = &p.NextCursor
	}
	return jsoncodec.Marshal(ReadPageInfo{Total: p.Total, Returned: p.Returned, HasMore: p.HasMore, NextCursor: next, Consistency: "frozen"})
}

// Internal readers keep their native cursor DTOs. Discovery exposes only the
// initial query; every continuation uses get_read_page's one opaque cursor.
func paginationInputSchema[I any](name string, supplied any) any {
	if !paginatedLiveTool(name) {
		return supplied
	}
	var schema *jsonschema.Schema
	if supplied == nil {
		var err error
		schema, err = jsonschema.For[I](nil)
		if err != nil {
			panic(err)
		}
	} else {
		raw, err := jsoncodec.Marshal(supplied)
		if err != nil {
			panic(err)
		}
		schema = &jsonschema.Schema{}
		if err = jsoncodec.Unmarshal(raw, schema); err != nil {
			panic(err)
		}
	}
	for _, key := range []string{"cursor", "afterEffectiveAt", "afterCreatedAt", "afterId"} {
		delete(schema.Properties, key)
	}
	schema.DependentRequired = nil
	if limit := schema.Properties["limit"]; limit != nil {
		low, high := float64(0), float64(100)
		limit.Minimum, limit.Maximum = &low, &high
		limit.Description = "Rows default 25/max 100; byte-limited. Continue with get_read_page."
	}
	return schema
}
