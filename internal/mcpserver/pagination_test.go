package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestUnifiedPageFrozenScopeAndFinancialItem(t *testing.T) {
	s, g := itemFixture(t) // No app or DB: universal routing must preserve frozen-only reads.
	s.reads = newBoundedReadCache()
	s.reads.activate(g)
	set := newToolSet("synthetic", false)
	rows, err := readRows([]map[string]string{{"id": "a", "targetKey": "instrument/A"}, {"id": "b", "targetKey": "fx/USD/HKD"}, {"id": "c", "targetKey": "instrument/A"}})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := s.reads.store(g, readCapture{Kind: "synthetic", Summary: map[string]any{}, Sections: map[string][]json.RawMessage{"records": rows}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: *initial.Sections["records"].Page.NextCursor, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	next := page.(ReadEnvelope)
	if next.ContentHash != initial.ContentHash || next.ExpiresAt != initial.ExpiresAt || next.CaptureID != initial.CaptureID || next.Section != "records" || string(next.Items[0]) != string(rows[1]) {
		t.Fatal("lost frozen identity, target or order", next)
	}
	if _, err = s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: *next.Page.NextCursor, CaptureID: "other"}); err == nil {
		t.Fatal("accepted swapped capture")
	}
	if _, err = s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: *next.Page.NextCursor, Section: "other"}); err == nil {
		t.Fatal("accepted swapped section")
	}
	last, err := s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: *next.Page.NextCursor})
	if err != nil || last.(ReadEnvelope).Page.HasMore || last.(ReadEnvelope).Page.NextCursor != nil {
		t.Fatal(last, err)
	}
	first := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1", Section: "positions", Limit: 1})
	item, err := s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: first.PositionsPage.NextCursor, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	detail := item.(FinancialSectionResponse)
	if detail.ContextID != first.ContextID || detail.Ref != first.Ref || detail.ContentHash != first.ContentHash || detail.CacheExpiresAt != first.CacheExpiresAt || !strings.Contains(string(detail.Items[0]), `"ref":"position-2"`) {
		t.Fatal("lost item scope", detail)
	}
	// Reuse the authenticated whole-package cursor without resending any identity.
	native := s.contexts.cursor("package-a", "evidence", g, 1)
	value, err := s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: native, Limit: 1})
	if err != nil || value.(FinancialSectionResponse).Section != "evidence" {
		t.Fatal(value, err)
	}
	s.contexts.now = func() time.Time { return time.Now().Add(contextTTL + time.Second) }
	_, err = s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: native})
	if err == nil || !strings.Contains(err.Error(), "context_expired") {
		t.Fatal("lost financial expiry", err)
	}
	s.reads.revoke()
	_, err = s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: *next.Page.NextCursor})
	if err == nil || !strings.Contains(err.Error(), "read_revoked") {
		t.Fatal("lost read revocation", err)
	}
}

func TestUnifiedPageLiveQueryAuthentication(t *testing.T) {
	c := &pageCodec{generation: 7}
	c.key[0] = 19
	input := map[string]any{"sessionId": "session-A", "section": "attempts", "accountId": "account-A", "itemId": "item-A", "status": "confirmed", "sort": "desc", "cursor": "native-evidence-version", "limit": 1}
	token, err := c.encode("get_reconciliation_session_page", input)
	if err != nil {
		t.Fatal(err)
	}
	route, err := c.decode(token)
	if err != nil || route.Tool != "get_reconciliation_session_page" {
		t.Fatal(route, err)
	}
	got := map[string]any{}
	if err = jsoncodec.Unmarshal(route.Query, &got); err != nil {
		t.Fatal(err)
	}
	if _, exists := got["limit"]; exists {
		t.Fatal("row budget became scope")
	}
	if !reflect.DeepEqual(got, input) {
		t.Fatal("query not retained", got, input)
	}
	if _, err = c.decode(token + "x"); err == nil {
		t.Fatal("accepted tamper")
	}
	changed := *c
	changed.generation++
	if _, err = changed.decode(token); err == nil || !strings.Contains(err.Error(), "read_revoked") {
		t.Fatal("accepted changed generation", err)
	}
	changed = *c
	changed.key[0]++
	if _, err = changed.decode(token); err == nil {
		t.Fatal("accepted cross-session cursor")
	}
	forged, err := c.encode("commit_plan", map[string]any{"operationId": uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.decode(forged); err == nil {
		t.Fatal("routed write as page")
	}
	if _, err = c.decode(strings.Repeat("x", pageCursorLimit+1)); err == nil {
		t.Fatal("unbounded cursor")
	}
}

func TestUnifiedRulesFinalEmptyAndInProcessParity(t *testing.T) {
	fx := newLedgerFixture(t)
	external := ledgerSession(t, fx)
	spec := jsonObject(t, testReviewV2Spec())
	for range 3 {
		call(t, external, "create_investment_review_rule", map[string]any{"operationId": uuid.NewString(), "input": ReviewV2RuleInput{RuleID: uuid.NewString(), MutationID: uuid.NewString(), Name: "Synthetic paging rule", Enabled: true, Severity: "notice", Spec: spec}}, false)
	}
	local := newLocal(t, fx, ReadOnly)
	for _, adapter := range []string{"http", "local"} {
		t.Run(adapter, func(t *testing.T) {
			read := func(name string, args any) map[string]any {
				if adapter == "local" {
					return localData(t, local, name, args, false)
				}
				return call(t, external, name, args, false)["data"].(map[string]any)
			}
			page := read("list_investment_review_rules", map[string]any{"limit": 2})
			seen := map[string]bool{}
			calls := 0
			for {
				calls++
				for _, row := range page["items"].([]any) {
					id := row.(map[string]any)["id"].(string)
					if seen[id] {
						t.Fatal("duplicate rule")
					}
					seen[id] = true
				}
				info := page["page"].(map[string]any)
				if info["consistency"] != "live" {
					t.Fatal(info)
				}
				if info["hasMore"] == false {
					if info["nextCursor"] != nil || len(page["items"].([]any)) != 0 {
						t.Fatal("false completeness", page)
					}
					break
				}
				if calls > 4 {
					t.Fatal("nonprogressing cursor")
				}
				page = read("get_read_page", map[string]any{"cursor": info["nextCursor"], "limit": 2})
			}
			if len(seen) != 3 || calls != 3 {
				t.Fatal("missing rules or final empty", len(seen), calls)
			}
		})
	}
}

func TestUnifiedDiscoveryLimitAndContinuation(t *testing.T) {
	s, _, _ := fixture(t)
	for _, d := range s.buildToolSet(ReadOnly, false).Definitions() {
		if !paginatedLiveTool(d.Name) {
			continue
		}
		schema := jsonObject(t, d.InputSchema)
		props := schema["properties"].(map[string]any)
		for _, key := range []string{"pageSize", "afterEffectiveAt", "afterCreatedAt", "afterId", "cursor"} {
			if _, ok := props[key]; ok {
				t.Fatalf("%s still advertises %s", d.Name, key)
			}
		}
		if props["limit"] == nil || d.OutputSchema == nil {
			t.Fatal("missing uniform schema", d.Name)
		}
	}
}

// The mapper must run during page selection, not only after the native reader
// already chose a boundary that omits the routing cursor's SDK wire overhead.
func TestUnifiedPageBudgetIncludesProjectedCursor(t *testing.T) {
	fx := newLedgerFixture(t)
	set := newToolSet("synthetic", false)
	in := ReconciliationReadInput{SessionID: strings.Repeat("scope-", 200), Section: "links"}
	project := pageProjection(func(v any) (any, error) { return set.projectPage("get_reconciliation_session_page", in, v) })
	ctx := context.WithValue(t.Context(), pageProjectKey{}, project)
	items := []any{strings.Repeat("数据\\\"", 900), strings.Repeat("数据\\\"", 900), strings.Repeat("数据\\\"", 900)}
	_, err := fitLiveReadPage(ctx, fx.app, 100, func(ctx context.Context, size int) (any, error) {
		return pageReconciliation(ReconciliationReadInput{Limit: size}, items, 7, ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{FinancialContextPageInfo{}, ReadPageInfo{Consistency: "frozen"}} {
		m := jsonObject(t, value)
		if v, ok := m["nextCursor"]; !ok || v != nil {
			t.Fatal("inconsistent terminal cursor", m)
		}
	}
}

func TestUnifiedItemDeferredRowUsesCompactBudgetWithoutSkipping(t *testing.T) {
	s, g := itemFixture(t)
	s.reads = newBoundedReadCache()
	s.reads.activate(g)
	set := newToolSet("synthetic", false)
	entry := s.contexts.entries["package-a"]
	entry.result.Content.Positions[0].Name = strings.Repeat("target", 2500)
	entry.result.Content.Positions[1].Name = strings.Repeat("child", 4000)
	s.contexts.entries["package-a"] = entry
	initial := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1"})
	if initial.PositionsPage.Returned != 0 || !initial.PositionsPage.HasMore {
		t.Fatal("unread large child mistaken for absence", initial.PositionsPage)
	}
	value, err := s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: initial.PositionsPage.NextCursor, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	page := value.(FinancialSectionResponse)
	if len(page.Items) != 1 || page.Ref != "account-1" || !strings.Contains(string(page.Items[0]), `"ref":"position-1"`) {
		t.Fatal("skipped child", page)
	}
	object := jsonObject(t, page)
	for _, key := range []string{"position", "evidenceItem", "basis", "positionsPage", "gapsPage", "evidencePage"} {
		if _, exists := object[key]; exists {
			t.Fatal("repeated initial header/progress", key)
		}
	}
	if size, err := financialContextWireSize(page); err != nil || size > contextWireLimit {
		t.Fatal(size, err)
	}
	// A truly oversized NEXT row must fail at that exact boundary for every
	// requested count. It must never advance to the following smaller row.
	entry.result.Content.Positions[2].Name = strings.Repeat("x", contextWireLimit)
	s.contexts.entries["package-a"] = entry
	for _, limit := range []int{1, 100} {
		_, err = s.continuePage(t.Context(), set, g, ReadPageInput{Cursor: page.Page.NextCursor, Limit: limit})
		if err == nil || !strings.Contains(err.Error(), "too_large") {
			t.Fatal("oversized row skipped", limit, err)
		}
	}
}
