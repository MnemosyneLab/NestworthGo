package mcpserver

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
)

// Refs are interpreted only inside ContextID. They are not persistent identities.
type FinancialContextItemInput struct {
	ContextID string `json:"contextId"`
	Ref       string `json:"ref"`
	Section   string `json:"section,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type FinancialContextItemResponse struct {
	ContextID      string                            `json:"contextId"`
	ContentHash    string                            `json:"contentHash"`
	CapturedAt     string                            `json:"capturedAt"`
	GeneratedAt    string                            `json:"generatedAt"`
	CacheExpiresAt string                            `json:"cacheExpiresAt"`
	SchemaVersion  string                            `json:"schemaVersion"`
	Disclosure     string                            `json:"disclosure"`
	AsOf           application.FinancialContextAsOf  `json:"asOf"`
	Basis          application.FinancialContextBasis `json:"basis"`
	Ref            string                            `json:"ref"`
	Type           string                            `json:"type"`
	// The account target is a rollup, never an additional component to sum.
	Position      *application.FinancialContextPosition  `json:"position,omitempty"`
	EvidenceItem  *application.FinancialContextEvidence  `json:"evidenceItem,omitempty"`
	Positions     []application.FinancialContextPosition `json:"positions"`
	Gaps          []application.FinancialContextGap      `json:"gaps"`
	Evidence      []application.FinancialContextEvidence `json:"evidence"`
	PositionsPage FinancialContextPageInfo               `json:"positionsPage"`
	GapsPage      FinancialContextPageInfo               `json:"gapsPage"`
	EvidencePage  FinancialContextPageInfo               `json:"evidencePage"`
}

func (s *Service) financialContextItem(ctx context.Context, g uint64, in FinancialContextItemInput) (FinancialContextItemResponse, error) {
	var empty FinancialContextItemResponse
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if in.Limit == 0 {
		in.Limit = 50
	}
	if in.Limit < 1 || in.Limit > 100 {
		return empty, fail("validation", "limit must be 1 to 100")
	}
	if in.Section != "" && in.Section != "positions" && in.Section != "gaps" && in.Section != "evidence" {
		return empty, fail("validation", "unknown section")
	}
	if in.Cursor != "" && in.Section == "" {
		return empty, fail("validation", "cursor requires section")
	}
	c := s.contexts
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.revoked || g != c.generation {
		return empty, fail("context_revoked", "connection generation was revoked")
	}
	c.prune(c.now())
	entry, ok := c.entries[in.ContextID]
	if !ok {
		return empty, fail("context_expired", "context expired or was evicted; request a new context")
	}
	return c.itemResponse(in, entry, g)
}

func (c *financialContextCache) itemResponse(in FinancialContextItemInput, e cachedFinancialContext, g uint64) (FinancialContextItemResponse, error) {
	full := e.result.Content
	if full.Disclosure != "minimal" {
		return FinancialContextItemResponse{}, fail("validation", "item drill-down requires a minimal context; named references contain raw identities")
	}
	r := FinancialContextItemResponse{ContextID: in.ContextID, ContentHash: e.result.ContentHash,
		CapturedAt: e.result.CapturedAt.UTC().Format(time.RFC3339Nano), GeneratedAt: c.now().UTC().Format(time.RFC3339Nano),
		CacheExpiresAt: e.expires.UTC().Format(time.RFC3339Nano), SchemaVersion: "financial-context-item/1", Disclosure: full.Disclosure,
		AsOf: full.AsOf, Basis: full.Basis, Ref: in.Ref,
		Positions: []application.FinancialContextPosition{}, Gaps: []application.FinancialContextGap{}, Evidence: []application.FinancialContextEvidence{}}
	// Resolve by membership AND row kind, never by prefix alone. No raw-ID lookup.
	for _, p := range full.Positions {
		if p.Ref != in.Ref {
			continue
		}
		if p.Kind == "account" && strings.HasPrefix(p.Ref, "account-") {
			r.Type = "account"
		}
		if p.Kind != "account" && strings.HasPrefix(p.Ref, "position-") {
			r.Type = "position"
		}
		if r.Type != "" {
			p.Name = ""
			r.Position = &p
		}
	}
	for _, e := range full.Evidence {
		if e.Ref == in.Ref && strings.HasPrefix(e.Ref, "evidence-") {
			r.Type = "evidence"
			r.EvidenceItem = &e
		}
	}
	if r.Type == "" {
		return r, fail("validation", "ref is not an account, position or evidence in this context")
	}
	selected := map[string]bool{}
	evidenceRefs := map[string]bool{}
	positions := []application.FinancialContextPosition{}
	gaps := []application.FinancialContextGap{}
	evidence := []application.FinancialContextEvidence{}
	if r.Position != nil {
		selected[in.Ref] = true
		for _, ref := range r.Position.EvidenceRefs {
			evidenceRefs[ref] = true
		}
	}
	for _, p := range full.Positions {
		include := r.Type == "account" && p.ParentRef == in.Ref && p.Kind != "account"
		if r.Type == "evidence" {
			for _, ref := range p.EvidenceRefs {
				if ref == in.Ref {
					include = true
				}
			}
		}
		if include {
			p.Name = ""
			positions = append(positions, p)
			selected[p.Ref] = true
			if r.Type != "evidence" {
				for _, ref := range p.EvidenceRefs {
					evidenceRefs[ref] = true
				}
			}
		}
	}
	for _, gap := range full.Gaps {
		include := selected[gap.EntityRef]
		if r.Type == "evidence" {
			include = gap.DependencyRef == in.Ref
		}
		if include {
			gaps = append(gaps, gap)
			if r.Type != "evidence" {
				evidenceRefs[gap.DependencyRef] = true
			}
		}
	}
	if r.Type != "evidence" {
		for _, e := range full.Evidence {
			if evidenceRefs[e.Ref] {
				evidence = append(evidence, e)
			}
		}
	}
	// Bind the existing authenticated cursor to this operation, target and section.
	cursorSection := func(section string) string { return "item:" + in.Ref + ":" + section }
	r.PositionsPage = c.pageInfo(in.ContextID, cursorSection("positions"), g, 0, 0, len(positions))
	r.GapsPage = c.pageInfo(in.ContextID, cursorSection("gaps"), g, 0, 0, len(gaps))
	r.EvidencePage = c.pageInfo(in.ContextID, cursorSection("evidence"), g, 0, 0, len(evidence))
	offset := 0
	if in.Cursor != "" {
		cur, err := c.decodeCursor(in.Cursor)
		if err != nil || cur.ContextID != in.ContextID || cur.Section != cursorSection(in.Section) || cur.Generation != g || cur.Offset < 0 {
			return r, fail("validation", "cursor does not match context, target, section and generation")
		}
		offset = cur.Offset
	}
	if size, err := financialContextWireSize(r); err != nil {
		return r, err
	} else if size > contextWireLimit {
		return r, fail("too_large", "context item target exceeds the MCP wire budget")
	}
	sections := []string{in.Section}
	limit := in.Limit
	if in.Section == "" {
		sections = []string{"gaps", "positions", "evidence"}
		limit = 1
	}
	for _, section := range sections {
		var err error
		switch section {
		case "positions":
			err = itemPage(c, in, e, g, section, positions, &r.Positions, &r.PositionsPage, offset, limit, &r)
		case "gaps":
			err = itemPage(c, in, e, g, section, gaps, &r.Gaps, &r.GapsPage, offset, limit, &r)
		case "evidence":
			err = itemPage(c, in, e, g, section, evidence, &r.Evidence, &r.EvidencePage, offset, limit, &r)
		}
		if err != nil {
			return r, err
		}
	}
	if size, err := financialContextWireSize(r); err != nil {
		return r, err
	} else if size > contextWireLimit {
		return r, fail("too_large", "context item exceeds the MCP wire budget")
	}
	return r, nil
}

func itemPage[T any](c *financialContextCache, in FinancialContextItemInput, entry cachedFinancialContext, g uint64, section string, rows []T, output *[]T, info *FinancialContextPageInfo, offset, limit int, r *FinancialContextItemResponse) error {
	if offset > len(rows) || (in.Cursor != "" && offset == len(rows)) {
		return fail("validation", "cursor offset is past the section")
	}
	pageInfo := func(n int) FinancialContextPageInfo {
		return c.pageInfo(in.ContextID, "item:"+in.Ref+":"+section, g, offset, n, len(rows))
	}
	for i := offset; i < len(rows) && len(*output) < limit; i++ {
		*output = append(*output, rows[i])
		*info = pageInfo(len(*output))
		size, err := financialContextWireSize(*r)
		if err != nil {
			return err
		}
		if size > contextWireLimit {
			*output = (*output)[:len(*output)-1]
			if len(*output) == 0 {
				if in.Section != "" {
					return fail("too_large", "item row and required target exceed the MCP wire budget")
				}
				// A crowded initial section may defer, but its standalone first page must fit.
				trial := in
				trial.Section, trial.Cursor, trial.Limit = section, "", 1
				if _, err := c.itemResponse(trial, entry, g); err != nil {
					return err
				}
			}
			break
		}
	}
	*info = pageInfo(len(*output))
	return nil
}
