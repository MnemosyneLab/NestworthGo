package mcpserver

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type FinancialComparisonPageInput struct {
	ComparisonID string `json:"comparisonId"`
	Section      string `json:"section"`
	Cursor       string `json:"cursor"`
	Limit        int    `json:"limit,omitempty"`
}
type FinancialComparisonInput struct {
	application.FinancialComparisonRequest
	FinancialInitialPageInput
}

type FinancialComparisonResponse struct {
	ComparisonID  string                                 `json:"comparisonId"`
	CapturedAt    string                                 `json:"capturedAt"`
	Content       application.FinancialComparisonContent `json:"content"`
	PositionsPage FinancialContextPageInfo               `json:"positionsPage"`
	GapsPage      FinancialContextPageInfo               `json:"gapsPage"`
	EvidencePage  FinancialContextPageInfo               `json:"evidencePage"`
}

func (s *Service) financialComparisonTools(server *toolSet, generation uint64) {
	analysisTool(server, "compare_financial_attribution", "Recapture a two-state comparison together with closed-period attribution; it never certifies an older comparisonId. Same date, scope and disclosure inputs as compare_financial_context; a closed A/B attributes A+1 through B. Scope is the household or one account UUID, and a current right endpoint is unavailable. Check attribution.status and mismatchReasons first: only a compatible link separates net-worth drivers from investment returns; incompatible must stop and unavailable keeps the missing evidence. change.netWorth = explainedDelta + residual + precisionAdjustment; adjustments are neither profit nor unknown residual. Fetch needed sections with get_read_page({cursor,limit?}) and finish every relevant cursor before a complete-list, absence or audit claim.", func(ctx context.Context, in FinancialComparisonInput) (any, error) {
		return s.buildFinancialAttribution(ctx, generation, in)
	})

	readTool(server, "compare_financial_context", "Compare two states from one read-only capture: leftAsOf is a closed YYYY-MM-DD date, rightAsOf a closed date or current. Minimal disclosure by default; named disclosure needs explicit user intent. Changes are right minus left, never returns or attribution; missing differences are null. No history means unavailable. Use sections to choose inline positions, gaps and evidence, finishing every relevant cursor before a complete-list or absence claim.", func(ctx context.Context, in FinancialComparisonInput) (any, error) {
		return s.buildFinancialComparison(ctx, generation, in)
	})
}
func (s *Service) buildFinancialAttribution(ctx context.Context, g uint64, in FinancialComparisonInput) (response FinancialComparisonResponse, err error) {
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
func (s *Service) buildFinancialComparison(ctx context.Context, g uint64, in FinancialComparisonInput) (FinancialComparisonResponse, error) {
	return s.captureFinancialComparison(ctx, g, in, false)
}
func (s *Service) captureFinancialComparison(ctx context.Context, g uint64, in FinancialComparisonInput, attribution bool) (FinancialComparisonResponse, error) {
	opts, err := in.FinancialInitialPageInput.normalized()
	if err != nil {
		return FinancialComparisonResponse{}, err
	}
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
	if attribution {
		result, err = s.app.BuildFinancialAttribution(ctx, in.FinancialComparisonRequest)
	} else {
		result, err = s.app.BuildFinancialComparison(ctx, in.FinancialComparisonRequest)
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
	response, err := c.comparisonResponse(id, entry, g, opts)
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
func (c *financialContextCache) comparisonResponse(id string, e cachedFinancialContext, g uint64, opts FinancialInitialPageInput) (FinancialComparisonResponse, error) {
	full := e.comparison.Content
	r := FinancialComparisonResponse{ComparisonID: id, CapturedAt: e.comparison.CapturedAt.UTC().Format(time.RFC3339Nano), Content: full}
	err := c.financialInitialPage(id, g, opts, "comparison", &r,
		func(section string) error {
			_, err := c.comparisonSectionResponse(id, e, g, section, 0, 1)
			return err
		},
		makeFinancialInitialSection("gaps", full.Gaps, &r.Content.Gaps, &r.GapsPage),
		makeFinancialInitialSection("positions", full.Positions, &r.Content.Positions, &r.PositionsPage),
		makeFinancialInitialSection("evidence", full.Evidence, &r.Content.Evidence, &r.EvidencePage),
	)
	return r, err
}
