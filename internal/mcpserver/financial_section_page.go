package mcpserver

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Continuations refer to the initial immutable header by capture ID.
// Only the requested section's rows and progress are returned. In particular,
// comparison attribution/precision and context basis are not resent per page.
type FinancialSectionResponse struct {
	Ref          string                   `json:"ref,omitempty"`
	ContextID    string                   `json:"contextId,omitempty"`
	ComparisonID string                   `json:"comparisonId,omitempty"`
	CapturedAt   string                   `json:"capturedAt"`
	Section      string                   `json:"section"`
	Items        []json.RawMessage        `json:"items"`
	Page         FinancialContextPageInfo `json:"page"`
}

func (c *financialContextCache) contextSectionResponse(id string, e cachedFinancialContext, g uint64, section string, offset, limit int) (FinancialSectionResponse, error) {
	r := FinancialSectionResponse{ContextID: id, CapturedAt: e.result.CapturedAt.UTC().Format(time.RFC3339Nano), Section: section}
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
	r := FinancialSectionResponse{ComparisonID: id, CapturedAt: e.comparison.CapturedAt.UTC().Format(time.RFC3339Nano), Section: section}
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

// Related-item continuation keeps only its frozen target identity and requested
// section. The initial target/disclosure/basis header remains authoritative.
func compactFinancialItemPage(r FinancialContextItemResponse, section string) (FinancialSectionResponse, error) {
	out := FinancialSectionResponse{ContextID: r.ContextID, Ref: r.Ref, CapturedAt: r.CapturedAt, Section: section}
	var err error
	switch section {
	case "positions":
		out.Items, err = readRows(r.Positions)
		out.Page = r.PositionsPage
	case "gaps":
		out.Items, err = readRows(r.Gaps)
		out.Page = r.GapsPage
	case "evidence":
		out.Items, err = readRows(r.Evidence)
		out.Page = r.EvidencePage
	default:
		return out, fail("validation", "unknown item section")
	}
	return out, err
}
func financialItemWireSize(in FinancialContextItemInput, r FinancialContextItemResponse) (int, error) {
	if !in.compactPage {
		return financialContextWireSize(r)
	}
	page, err := compactFinancialItemPage(r, in.Section)
	if err != nil {
		return 0, err
	}
	return financialContextWireSize(page)
}

var financialSections = []string{"gaps", "positions", "evidence"}

func (in FinancialInitialPageInput) normalized() (FinancialInitialPageInput, error) {
	in.defaultLimit = in.Limit == 0
	limit, err := readLimit(in.Limit)
	if err != nil {
		return in, err
	}
	in.Limit = limit
	if in.Sections == nil {
		in.Sections = financialSections
	}
	for i, section := range in.Sections {
		if !slices.Contains(financialSections, section) {
			return in, fail("validation", "unknown section")
		}
		if slices.Contains(in.Sections[:i], section) {
			return in, fail("validation", "financial sections must be unique")
		}
	}
	return in, nil
}

func (in FinancialInitialPageInput) inlineBudget(headerSize int) int {
	if in.defaultLimit {
		return max(autoInlineWireBudget, headerSize)
	}
	return contextWireLimit
}

type financialInitialSection struct {
	name    string
	total   int
	setPage func(FinancialContextPageInfo)
}

func makeFinancialInitialSection[T any](name string, rows []T, output *[]T, info *FinancialContextPageInfo) financialInitialSection {
	return financialInitialSection{name: name, total: len(rows), setPage: func(page FinancialContextPageInfo) {
		*output = rows[:page.Returned:page.Returned]
		if *output == nil {
			*output = []T{}
		}
		*info = page
	}}
}

func (c *financialContextCache) financialInitialPage(id string, g uint64, opts FinancialInitialPageInput, label string, response any, checkStandalone func(string) error, sections ...financialInitialSection) error {
	for _, section := range sections {
		section.setPage(c.pageInfo(id, section.name, g, 0, 0, section.total))
	}
	headerSize, err := financialContextWireSize(response)
	if err != nil {
		return err
	}
	if headerSize > contextWireLimit {
		return fail("too_large", label+" summary exceeds the MCP wire budget")
	}
	budget := opts.inlineBudget(headerSize)
	for _, section := range sections {
		if section.total == 0 || !slices.Contains(opts.Sections, section.name) {
			continue
		}
		// Try the full requested prefix first. If it overflows, all smaller
		// candidates retain a cursor, so the wire size grows with each row.
		count := min(opts.Limit, section.total)
		setPage := func(n int) { section.setPage(c.pageInfo(id, section.name, g, 0, n, section.total)) }
		setPage(count)
		if size, err := financialContextWireSize(response); err != nil {
			return err
		} else if size <= budget {
			continue
		}
		low, high := 0, count
		for high-low > 1 {
			mid := low + (high-low)/2
			setPage(mid)
			size, err := financialContextWireSize(response)
			if err != nil {
				return err
			}
			if size <= budget {
				low = mid
			} else {
				high = mid
			}
		}
		setPage(low)
		if low == 0 {
			// Earlier sections or the summary may crowd out this section.
			// A selected first row still has to fit on a standalone page.
			if err := checkStandalone(section.name); err != nil {
				return err
			}
		}
	}
	return nil
}
