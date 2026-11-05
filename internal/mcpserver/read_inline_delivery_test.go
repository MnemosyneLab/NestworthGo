package mcpserver

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func inlineRows(n, payload int) []json.RawMessage {
	rows := make([]json.RawMessage, n)
	for i := range rows {
		rows[i] = json.RawMessage(fmt.Sprintf(`{"id":%d,"pad":%q}`, i, strings.Repeat("x", payload)))
	}
	return rows
}

func storeInline(t *testing.T, sections map[string][]json.RawMessage, limit int) ReadEnvelope {
	t.Helper()
	c := newBoundedReadCache()
	c.activate(1)
	r, err := c.store(1, readCapture{Kind: "test", Summary: map[string]any{"complete": false}, Sections: sections}, limit)
	if err != nil {
		t.Fatal(err)
	}
	if r.Delivery == nil {
		t.Fatal("initial read response lacks delivery")
	}
	return r
}

func TestAutoInlineFillsSmallResultsAndReportsDelivery(t *testing.T) {
	// Fewer rows than the cap: everything is inline, so the selection is complete
	// even though the summary reports incomplete data. The two are independent.
	r := storeInline(t, map[string][]json.RawMessage{"a": inlineRows(12, 8), "b": inlineRows(3, 8)}, autoInlineLimit)
	if len(r.Sections["a"].Items) != 12 || len(r.Sections["b"].Items) != 3 {
		t.Fatalf("small results were not inlined: %d/%d", len(r.Sections["a"].Items), len(r.Sections["b"].Items))
	}
	if !r.Delivery.SelectionComplete || len(r.Delivery.PendingSections) != 0 {
		t.Fatalf("complete selection reported as %+v", r.Delivery)
	}
	if got := r.Summary.(map[string]any)["complete"]; got != false {
		t.Fatalf("delivery must not rewrite data completeness: %v", got)
	}

	// More rows than the per-section cap: capped, with a cursor and a pending name.
	r = storeInline(t, map[string][]json.RawMessage{"a": inlineRows(60, 8), "b": inlineRows(2, 8)}, autoInlineLimit)
	if n := len(r.Sections["a"].Items); n != autoInlineMaxRows {
		t.Fatalf("section was not capped at %d rows: %d", autoInlineMaxRows, n)
	}
	if r.Delivery.SelectionComplete || strings.Join(r.Delivery.PendingSections, ",") != "a" || r.Sections["a"].Page.NextCursor == nil {
		t.Fatalf("partial selection reported as %+v", r.Delivery)
	}
}

func TestAutoInlineRespectsByteBudgetAndKeepsFloor(t *testing.T) {
	// 3 KiB rows: the byte budget stops inlining well before 25 rows, but the
	// previous five-row behavior is never reduced.
	r := storeInline(t, map[string][]json.RawMessage{"a": inlineRows(40, 3<<10)}, autoInlineLimit)
	n := len(r.Sections["a"].Items)
	if n < autoInlineFloorRows || n >= autoInlineMaxRows {
		t.Fatalf("byte budget did not bound inlining: %d rows", n)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > autoInlineWireBudget+3<<10 {
		t.Fatalf("auto inline exceeded its budget: %d bytes", len(raw))
	}
	if r.Delivery.SelectionComplete {
		t.Fatal("a truncated section was reported complete")
	}

	// Rows too large for the budget still return the legacy five.
	r = storeInline(t, map[string][]json.RawMessage{"a": inlineRows(10, 5<<10)}, autoInlineLimit)
	if n := len(r.Sections["a"].Items); n != autoInlineFloorRows {
		t.Fatalf("floor changed: %d", n)
	}

	// No section starves another: a heavy first section leaves the second its floor.
	r = storeInline(t, map[string][]json.RawMessage{"a": inlineRows(40, 3<<10), "b": inlineRows(40, 8)}, autoInlineLimit)
	if len(r.Sections["b"].Items) < autoInlineFloorRows {
		t.Fatalf("later section starved: %d", len(r.Sections["b"].Items))
	}
}

func TestExplicitLimitKeepsExactMeaningAndSentinelIsNotAccepted(t *testing.T) {
	r := storeInline(t, map[string][]json.RawMessage{"a": inlineRows(12, 8)}, 3)
	if len(r.Sections["a"].Items) != 3 || r.Delivery.SelectionComplete || strings.Join(r.Delivery.PendingSections, ",") != "a" {
		t.Fatalf("explicit limit changed: %d rows, %+v", len(r.Sections["a"].Items), r.Delivery)
	}
	if autoInlineWhenUnset(0) != autoInlineLimit || autoInlineWhenUnset(7) != 7 {
		t.Fatal("unset/explicit limit mapping changed")
	}
	// A caller cannot request auto mode by passing the internal sentinel.
	if _, err := readLimit(autoInlineWhenUnset(math.MinInt)); err == nil {
		t.Fatal("sentinel value accepted as an explicit limit")
	}
	if _, err := readLimit(autoInlineWhenUnset(-5)); err == nil {
		t.Fatal("negative limit accepted")
	}
}
