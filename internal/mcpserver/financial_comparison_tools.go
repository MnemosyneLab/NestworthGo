package mcpserver

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"time"
)

type FinancialComparisonPageInput struct {
	ComparisonID string `json:"comparisonId"`
	Section      string `json:"section"`
	Cursor       string `json:"cursor"`
	Limit        int    `json:"limit,omitempty"`
}
type FinancialComparisonResponse struct {
	ComparisonID   string                                 `json:"comparisonId"`
	ContentHash    string                                 `json:"contentHash"`
	CapturedAt     string                                 `json:"capturedAt"`
	GeneratedAt    string                                 `json:"generatedAt"`
	CacheExpiresAt string                                 `json:"cacheExpiresAt"`
	Content        application.FinancialComparisonContent `json:"content"`
	PositionsPage  FinancialContextPageInfo               `json:"positionsPage"`
	GapsPage       FinancialContextPageInfo               `json:"gapsPage"`
	EvidencePage   FinancialContextPageInfo               `json:"evidencePage"`
}

func (s *Service) financialComparisonTools(server *toolSet, generation uint64) {
	analysisTool(server, "compare_financial_attribution", "Recapture a two-state comparison together with closed-period attribution; it never certifies an older comparisonId. Same date, scope and disclosure inputs as compare_financial_context; a closed A/B attributes A+1 through B. Scope is the household or one account UUID, and a current right endpoint is unavailable. Check attribution.status and mismatchReasons first: only a compatible link separates net-worth drivers from investment returns; incompatible must stop and unavailable keeps the missing evidence. change.netWorth = explainedDelta + residual + precisionAdjustment; adjustments are neither profit nor unknown residual. Fetch needed sections with get_read_page({cursor,limit?}) and finish every relevant cursor before a complete-list, absence or audit claim.", func(ctx context.Context, in application.FinancialComparisonRequest) (any, error) {
		return s.buildFinancialAttribution(ctx, generation, in)
	})

	readTool(server, "compare_financial_context", "Compare two states from one read-only capture: leftAsOf is a closed YYYY-MM-DD date, rightAsOf a closed date or current. Minimal disclosure by default; named disclosure needs explicit user intent. Changes are right minus left, never returns or attribution; missing differences are null. No history means unavailable. Read only the positions, gaps and evidence sections you need, finishing every cursor before a complete-list or absence claim.", func(ctx context.Context, in application.FinancialComparisonRequest) (any, error) {
		return s.buildFinancialComparison(ctx, generation, in)
	})
}
func (s *Service) buildFinancialAttribution(ctx context.Context, g uint64, in application.FinancialComparisonRequest) (response FinancialComparisonResponse, err error) {
	if !s.contexts.active(g) {
		return response, fail("context_revoked", "connection generation was revoked")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = s.app.WithWrite(ctx, func(ctx context.Context) error {
		var buildErr error
		response, buildErr = s.captureFinancialComparison(ctx, g, in, true)
		return buildErr
	})
	return
}
func (s *Service) buildFinancialComparison(ctx context.Context, g uint64, in application.FinancialComparisonRequest) (FinancialComparisonResponse, error) {
	return s.captureFinancialComparison(ctx, g, in, false)
}
func (s *Service) captureFinancialComparison(ctx context.Context, g uint64, in application.FinancialComparisonRequest, attribution bool) (FinancialComparisonResponse, error) {
	c := s.contexts
	if !c.active(g) {
		return FinancialComparisonResponse{}, fail("context_revoked", "connection generation was revoked")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case c.builds <- struct{}{}:
		defer func() { <-c.builds }()
	case <-ctx.Done():
		return FinancialComparisonResponse{}, fail("context_busy", "context build budget elapsed")
	}
	if !c.active(g) {
		return FinancialComparisonResponse{}, fail("context_revoked", "connection generation was revoked")
	}
	var result application.FinancialComparisonResult
	var err error
	if attribution {
		result, err = s.app.BuildFinancialAttribution(ctx, in)
	} else {
		result, err = s.app.BuildFinancialComparison(ctx, in)
	}
	if err != nil {
		return FinancialComparisonResponse{}, err
	}
	if err := ctx.Err(); err != nil {
		return FinancialComparisonResponse{}, fail("context_busy", "context build budget elapsed")
	}
	encoded, err := jsoncodec.Marshal(result.Content)
	if err != nil {
		return FinancialComparisonResponse{}, err
	}
	if len(encoded) > contextPackageLimit {
		return FinancialComparisonResponse{}, fail("too_large", "comparison exceeds package budget; select fewer accounts")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.revoked || g != c.generation {
		return FinancialComparisonResponse{}, fail("context_revoked", "connection generation was revoked")
	}
	now := c.now()
	id := uuid.NewString()
	entry := cachedFinancialContext{comparison: &result, expires: now.Add(contextTTL), size: len(encoded)}
	response, err := c.comparisonResponse(id, entry, g, "", 0, 1)
	if err != nil {
		return FinancialComparisonResponse{}, err
	}
	c.prune(now)
	for len(c.entries) >= contextCacheCount || c.bytes+entry.size > contextCacheLimit {
		c.evictOldest()
	}
	c.entries[id] = entry
	c.bytes += entry.size
	return response, nil
}
func (s *Service) financialComparisonPage(ctx context.Context, g uint64, in FinancialComparisonPageInput) (FinancialSectionResponse, error) {
	if err := ctx.Err(); err != nil {
		return FinancialSectionResponse{}, err
	}
	if in.Limit == 0 {
		in.Limit = 25
	}
	if in.Limit < 1 || in.Limit > 100 {
		return FinancialSectionResponse{}, fail("validation", "limit must be 1 to 100")
	}
	if in.Section != "positions" && in.Section != "gaps" && in.Section != "evidence" {
		return FinancialSectionResponse{}, fail("validation", "unknown section")
	}
	c := s.contexts
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.revoked || g != c.generation {
		return FinancialSectionResponse{}, fail("context_revoked", "connection generation was revoked")
	}
	c.prune(c.now())
	entry, ok := c.entries[in.ComparisonID]
	if !ok || entry.comparison == nil {
		return FinancialSectionResponse{}, fail("context_expired", "comparison expired or was evicted; request a new comparison")
	}
	cursor, err := c.decodeCursor(in.Cursor)
	if err != nil || cursor.ContextID != in.ComparisonID || cursor.Section != in.Section || cursor.Generation != g || cursor.Offset < 0 {
		return FinancialSectionResponse{}, fail("validation", "cursor does not match comparison, section and generation")
	}
	return c.comparisonSectionResponse(in.ComparisonID, entry, g, in.Section, cursor.Offset, in.Limit)
}
func (c *financialContextCache) comparisonResponse(id string, e cachedFinancialContext, g uint64, section string, offset, limit int) (FinancialComparisonResponse, error) {
	content := e.comparison.Content
	content.Positions = []application.FinancialComparisonPosition{}
	content.Gaps = []application.FinancialComparisonGap{}
	content.Evidence = []application.FinancialComparisonEvidence{}
	r := FinancialComparisonResponse{ComparisonID: id, ContentHash: e.comparison.ContentHash, CapturedAt: e.comparison.CapturedAt.UTC().Format(time.RFC3339Nano), GeneratedAt: c.now().UTC().Format(time.RFC3339Nano), CacheExpiresAt: e.expires.UTC().Format(time.RFC3339Nano), Content: content}
	full := e.comparison.Content
	r.PositionsPage = c.pageInfo(id, "positions", g, 0, 0, len(full.Positions))
	r.GapsPage = c.pageInfo(id, "gaps", g, 0, 0, len(full.Gaps))
	r.EvidencePage = c.pageInfo(id, "evidence", g, 0, 0, len(full.Evidence))
	size, err := financialContextWireSize(r)
	if err != nil {
		return r, err
	}
	if size > contextWireLimit {
		return r, fail("too_large", "comparison summary exceeds the MCP wire budget")
	}
	sections := []string{section}
	if section == "" {
		sections = []string{"gaps", "positions", "evidence"}
	}
	for _, name := range sections {
		total := len(full.Positions)
		if name == "gaps" {
			total = len(full.Gaps)
		}
		if name == "evidence" {
			total = len(full.Evidence)
		}
		start := offset
		if section == "" {
			start = 0
		}
		if start >= total && section != "" {
			return r, fail("validation", "cursor offset is past the section")
		}
		returned := 0
		for i := start; i < total && returned < limit; i++ {
			switch name {
			case "positions":
				r.Content.Positions = append(r.Content.Positions, full.Positions[i])
			case "gaps":
				r.Content.Gaps = append(r.Content.Gaps, full.Gaps[i])
			case "evidence":
				r.Content.Evidence = append(r.Content.Evidence, full.Evidence[i])
			}
			info := c.pageInfo(id, name, g, start, returned+1, total)
			switch name {
			case "positions":
				r.PositionsPage = info
			case "gaps":
				r.GapsPage = info
			case "evidence":
				r.EvidencePage = info
			}
			size, err = financialContextWireSize(r)
			if err != nil {
				return r, err
			}
			if size > contextWireLimit {
				switch name {
				case "positions":
					r.Content.Positions = r.Content.Positions[:len(r.Content.Positions)-1]
				case "gaps":
					r.Content.Gaps = r.Content.Gaps[:len(r.Content.Gaps)-1]
				case "evidence":
					r.Content.Evidence = r.Content.Evidence[:len(r.Content.Evidence)-1]
				}
				if returned == 0 {
					if section != "" {
						return r, fail("too_large", fmt.Sprintf("%s row and required summary exceed the MCP wire budget", name))
					}
					// An initial section may be crowded out by earlier sections.
					// Fail only if its first row cannot fit with the summary alone;
					// otherwise pageInfo below supplies an offset-zero continuation.
					if _, err := c.comparisonSectionResponse(id, e, g, name, start, 1); err != nil {
						return r, err
					}
				}
				break
			}
			returned++
		}
		info := c.pageInfo(id, name, g, start, returned, total)
		switch name {
		case "positions":
			r.PositionsPage = info
		case "gaps":
			r.GapsPage = info
		case "evidence":
			r.EvidencePage = info
		}
	}
	if size, err := financialContextWireSize(r); err != nil {
		return r, err
	} else if size > contextWireLimit {
		return r, fail("too_large", "comparison page exceeds the MCP wire budget")
	}
	return r, nil
}
