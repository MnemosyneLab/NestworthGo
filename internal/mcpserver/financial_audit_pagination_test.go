package mcpserver

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
)

// Full audit walkers use explicit bulk limits, not a larger production default.
// Exercise both financial routes without a database or another large fixture.
func TestFinancialAuditPaginationDefaultAndBulk(t *testing.T) {
	for _, comparison := range []bool{false, true} {
		t.Run(fmt.Sprintf("comparison=%v", comparison), func(t *testing.T) {
			c := newFinancialContextCache()
			g := c.activate()
			now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
			c.now = func() time.Time { return now }
			entry := cachedFinancialContext{expires: now.Add(contextTTL)}
			entry.result = application.FinancialContextResult{CapturedAt: now, ContentHash: "frozen-hash"}
			result := application.FinancialComparisonResult{CapturedAt: now, ContentHash: "frozen-hash"}
			for i := 0; i < 126; i++ {
				ref := fmt.Sprintf("gap-%03d", i)
				entry.result.Content.Gaps = append(entry.result.Content.Gaps, application.FinancialContextGap{EntityRef: ref})
				result.Content.Gaps = append(result.Content.Gaps, application.FinancialComparisonGap{FinancialContextGap: application.FinancialContextGap{EntityRef: ref}})
			}
			var want any = entry.result.Content.Gaps
			if comparison {
				entry.comparison = &result
				want = result.Content.Gaps
			}
			c.entries[financialFixtureID] = entry
			s := &Service{contexts: c, reads: newBoundedReadCache()}
			set := newToolSet("synthetic", false)
			for _, limit := range []int{-1, 101} {
				if _, err := s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: c.cursor(financialFixtureID, "gaps", g, 0), Limit: limit}); err == nil {
					t.Fatalf("accepted invalid limit %d", limit)
				}
			}
			for _, tc := range []struct{ limit, first, calls int }{{0, 25, 6}, {100, 100, 2}} {
				cursor := c.cursor(financialFixtureID, "gaps", g, 0)
				rows := []json.RawMessage{}
				calls := 0
				for cursor != "" {
					value, err := s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: cursor, Limit: tc.limit})
					if err != nil {
						t.Fatal(err)
					}
					page := value.(FinancialSectionResponse)
					if calls == 0 && page.Page.Returned != tc.first {
						t.Fatalf("limit=%d returned=%d want=%d", tc.limit, page.Page.Returned, tc.first)
					}
					if page.CapturedAt != now.UTC().Format(time.RFC3339Nano) || page.Page.Total != 126 || page.Page.Returned != len(page.Items) || len(page.Items) == 0 {
						t.Fatal("lost frozen identity or progress", page)
					}
					if comparison && page.ComparisonID != financialFixtureID || !comparison && page.ContextID != financialFixtureID {
						t.Fatal("lost capture identity", page)
					}
					rows = append(rows, page.Items...)
					cursor = page.Page.NextCursor
					if page.Page.HasMore != (cursor != "") {
						t.Fatal("inconsistent terminal page")
					}
					calls++
					if calls > 126 {
						t.Fatal("non-advancing cursor")
					}
				}
				if calls != tc.calls {
					t.Fatalf("limit=%d calls=%d want=%d", tc.limit, calls, tc.calls)
				}
				featureJSONEqual(t, rows, want)
			}
		})
	}
}
