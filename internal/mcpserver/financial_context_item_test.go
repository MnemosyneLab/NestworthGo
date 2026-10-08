package mcpserver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func itemFixture(t *testing.T) (*Service, uint64) {
	t.Helper()
	c := newFinancialContextCache()
	g := c.activate()
	amount, zero, rate := "100", "0", "0.92"
	content := application.FinancialContextContent{Disclosure: "minimal", AsOf: application.FinancialContextAsOf{Mode: "current"},
		Positions: []application.FinancialContextPosition{
			{Ref: "account-1", Kind: "account", BaseAmount: &amount, Included: true},
			{Ref: "position-1", ParentRef: "account-1", Kind: "cash", BaseAmount: &zero, NativeAmount: &zero, Complete: true, Included: true, EvidenceRefs: []string{"evidence-1", "evidence-3"}},
			{Ref: "position-2", ParentRef: "account-1", Kind: "holding", BaseAmount: &amount, Included: true, EvidenceRefs: []string{"evidence-2", "evidence-3"}},
			{Ref: "position-3", ParentRef: "account-1", Kind: "holding", Status: "archived", Included: false, ExclusionReason: "archived", EvidenceRefs: []string{}},
			{Ref: "account-2", Kind: "account"},
			{Ref: "position-4", ParentRef: "account-2", Kind: "cash", EvidenceRefs: []string{"evidence-3", "evidence-4"}},
			{Ref: "account-3", Kind: "account", Status: "not_created", ExclusionReason: "not_created"},
		}, Gaps: []application.FinancialContextGap{
			{EntityRef: "position-1", DependencyRef: "evidence-3", Code: "manual_valuation"},
			{EntityRef: "position-2", DependencyRef: "fx:USD/CNY", Code: "missing_fx"},
			{EntityRef: "position-4", DependencyRef: "evidence-3", Code: "manual_valuation"},
			{EntityRef: "position-4", DependencyRef: "position-4", Code: "missing_account_value"},
		}, Evidence: []application.FinancialContextEvidence{
			{Ref: "evidence-1", Kind: "balance", Value: &zero}, {Ref: "evidence-2", Kind: "price", Value: &amount},
			{Ref: "evidence-3", Kind: "fx", Value: &rate, BaseCurrency: "HKD", QuoteCurrency: "CNY"}, {Ref: "evidence-4", Kind: "balance"},
		}}
	now := time.Now()
	entry := cachedFinancialContext{result: application.FinancialContextResult{CapturedAt: now, ContentHash: "synthetic-hash", Content: content}, expires: now.Add(contextTTL), size: 1}
	c.entries["package-a"] = entry
	c.bytes = 1
	return &Service{contexts: c}, g // No application/DB: item reads can only use the frozen cache.
}
func readItem(t *testing.T, s *Service, g uint64, in FinancialContextItemInput) FinancialContextItemResponse {
	t.Helper()
	r, err := s.financialContextItem(t.Context(), g, in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func decodeItem(t *testing.T, obj map[string]any) FinancialContextItemResponse {
	t.Helper()
	raw, _ := json.Marshal(obj["data"])
	var r FinancialContextItemResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestFinancialContextItemRelations(t *testing.T) {
	t.Parallel()
	s, g := itemFixture(t)
	account := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1", Section: "positions"})
	if account.Type != "account" || account.Position.Ref != "account-1" || len(account.Positions) != 3 || account.PositionsPage.Total != 3 {
		t.Fatal(account)
	}
	if *account.Position.BaseAmount != "100" || *account.Positions[0].NativeAmount != "0" || account.Positions[2].Included || account.Positions[2].ExclusionReason != "archived" {
		t.Fatal("rollup/zero/excluded altered", account)
	}
	evidence := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1", Section: "evidence"})
	if len(evidence.Evidence) != 3 || evidence.Evidence[2].Ref != "evidence-3" {
		t.Fatal("shared FX not deduplicated", evidence)
	}
	gaps := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1", Section: "gaps"})
	if len(gaps.Gaps) != 2 || gaps.Gaps[1].DependencyRef != "fx:USD/CNY" {
		t.Fatal("unrelated or missing gaps", gaps)
	}
	position := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "position-2", Section: "evidence"})
	if position.Type != "position" || len(position.Positions) != 0 || len(position.Evidence) != 2 || position.GapsPage.Total != 1 {
		t.Fatal(position)
	}
	fx := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "evidence-3", Section: "positions"})
	if fx.Type != "evidence" || fx.EvidenceItem.Ref != "evidence-3" || len(fx.Evidence) != 0 || len(fx.Positions) != 3 || fx.GapsPage.Total != 2 {
		t.Fatal("evidence must show direct users only", fx)
	}
	empty := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-3", Section: "positions"})
	if empty.Position.Status != "not_created" || empty.PositionsPage.HasMore || len(empty.Positions) != 0 {
		t.Fatal(empty)
	}
	for _, r := range []FinancialContextItemResponse{account, evidence, gaps, position, fx, empty} {
		if r.ContentHash != "synthetic-hash" || r.CapturedAt != account.CapturedAt || r.CacheExpiresAt != account.CacheExpiresAt {
			t.Fatal("lost capture", r)
		}
	}
	// Historical mode is copied from the same frozen content without rebuilding.
	entry := s.contexts.entries["package-a"]
	entry.result.Content.AsOf = application.FinancialContextAsOf{Mode: "closed_day", LocalDate: "2026-01-01"}
	s.contexts.entries["package-a"] = entry
	historical := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1"})
	if historical.AsOf.Mode != "closed_day" || historical.AsOf.LocalDate != "2026-01-01" {
		t.Fatal(historical)
	}
}
func TestFinancialContextItemValidationExpiryAndIsolation(t *testing.T) {
	t.Parallel()
	s, g := itemFixture(t)
	initial := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1"})
	cursor := initial.PositionsPage.NextCursor
	s.contexts.entries["package-b"] = s.contexts.entries["package-a"]
	for _, in := range []FinancialContextItemInput{
		{ContextID: "package-a", Ref: "unknown"}, {ContextID: "package-a", Ref: "fx:USD/CNY"}, {ContextID: "package-a", Ref: "account-999"},
		{ContextID: "package-a", Ref: "account-1", Limit: 101}, {ContextID: "package-a", Ref: "account-1", Limit: -1},
		{ContextID: "package-a", Ref: "account-1", Section: "transactions"}, {ContextID: "package-a", Ref: "account-1", Cursor: cursor},
		{ContextID: "package-a", Ref: "account-1", Section: "gaps", Cursor: cursor}, {ContextID: "package-a", Ref: "account-2", Section: "positions", Cursor: cursor},
		{ContextID: "package-b", Ref: "account-1", Section: "positions", Cursor: cursor},
		{ContextID: "package-a", Ref: "account-1", Section: "positions", Cursor: cursor + "x"},
		{ContextID: "package-a", Ref: "account-1", Section: "positions", Cursor: s.contexts.cursor("package-a", "positions", g, 1)},
		{ContextID: "package-a", Ref: "account-1", Section: "positions", Cursor: s.contexts.cursor("package-a", "item:account-1:positions", g, 3)},
		{ContextID: "package-a", Ref: "account-1", Section: "positions", Cursor: s.contexts.cursor("package-a", "item:account-1:positions", g+1, 1)},
	} {
		if _, err := s.financialContextItem(t.Context(), g, in); err == nil || !strings.Contains(err.Error(), "validation") {
			t.Fatalf("accepted %+v: %v", in, err)
		}
	}
	// An alias unique to A does not resolve in B; identical spellings remain local to each context.
	entry := s.contexts.entries["package-b"]
	entry.result.Content.Positions = nil
	s.contexts.entries["package-b"] = entry
	if _, err := s.financialContextItem(t.Context(), g, FinancialContextItemInput{ContextID: "package-b", Ref: "account-1"}); err == nil {
		t.Fatal("cross-package membership accepted")
	}
	entry = s.contexts.entries["package-a"]
	s.contexts.now = func() time.Time { return entry.expires }
	if _, err := s.financialContextItem(t.Context(), g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1"}); err == nil || !strings.Contains(err.Error(), "context_expired") {
		t.Fatal(err)
	}
	s.contexts.revoke()
	if _, err := s.financialContextItem(t.Context(), g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1"}); err == nil || !strings.Contains(err.Error(), "context_revoked") {
		t.Fatal(err)
	}
}
func TestFinancialContextItemBudgetAndPagination(t *testing.T) {
	t.Parallel()
	s, g := itemFixture(t)
	entry := s.contexts.entries["package-a"]
	for i := 5; i < 110; i++ {
		entry.result.Content.Positions = append(entry.result.Content.Positions, application.FinancialContextPosition{Ref: fmt.Sprintf("position-%d", i), ParentRef: "account-1", Kind: "holding"})
	}
	s.contexts.entries["package-a"] = entry
	first := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1", Section: "positions", Limit: 100})
	if first.PositionsPage.Returned != 100 || !first.PositionsPage.HasMore {
		t.Fatal(first.PositionsPage)
	}
	next := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1", Section: "positions", Cursor: first.PositionsPage.NextCursor, Limit: 100})
	if next.PositionsPage.Returned != 8 || next.PositionsPage.HasMore {
		t.Fatal(next.PositionsPage)
	}
	// Use large synthetic decimals to exercise duplicated/escaped SDK wire sizing.
	s, g = itemFixture(t)
	entry = s.contexts.entries["package-a"]
	large := strings.Repeat("9", 11500)
	entry.result.Content.Positions[0].BaseAmount = &large
	entry.result.Content.Positions[1].BaseAmount = &large
	entry.result.Content.Evidence[0].Value = &large
	s.contexts.entries["package-a"] = entry
	initial := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1"})
	if initial.EvidencePage.Returned != 0 || !initial.EvidencePage.HasMore {
		t.Fatal("expected zero-row deferral", initial.EvidencePage)
	}
	page := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1", Section: "evidence", Cursor: initial.EvidencePage.NextCursor, Limit: 1})
	if page.EvidencePage.Returned != 1 {
		t.Fatal(page.EvidencePage)
	}
	for _, r := range []FinancialContextItemResponse{initial, page} {
		size, err := financialContextWireSize(r)
		if err != nil || size > contextWireLimit {
			t.Fatal(size, err)
		}
	}
	oversized := strings.Repeat("9", contextWireLimit)
	entry.result.Content.Evidence[0].Value = &oversized
	s.contexts.entries["package-a"] = entry
	for _, section := range []string{"", "evidence"} {
		if _, err := s.financialContextItem(t.Context(), g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1", Section: section, Limit: 1}); err == nil || !strings.Contains(err.Error(), "too_large") {
			t.Fatal(err)
		}
	}
}
func TestFinancialContextItemConcurrentRevocation(t *testing.T) {
	t.Parallel()
	s, g := itemFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Go(func() {
			for j := 0; j < 20; j++ {
				r, err := s.financialContextItem(t.Context(), g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1"})
				if err == nil && r.ContentHash != "synthetic-hash" {
					t.Error("mixed capture")
				}
			}
		})
	}
	s.contexts.revoke()
	wg.Wait()
	if _, err := s.financialContextItem(t.Context(), g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1"}); err == nil {
		t.Fatal("revoked read succeeded")
	}
}
func TestFinancialContextItemHTTPFrozenAndSkill(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		t.Run(mode, func(t *testing.T) {
			s, app, changes := fixture(t)
			now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
			app.SetClock(func() time.Time { return now })
			members, err := app.ListMembers(t.Context(), false)
			if err != nil {
				t.Fatal(err)
			}
			note := "PRIVATE NOTE https://private.invalid"
			account, err := app.CreateAccount(t.Context(), application.AccountInput{Name: "PRIVATE NAME", Note: &note, AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "EUR", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = app.AppendAccountValue(t.Context(), account.Account.ID, "100", ""); err != nil {
				t.Fatal(err)
			}
			if _, err = app.StartHistory(t.Context(), "UTC"); err != nil {
				t.Fatal(err)
			}
			now = now.AddDate(0, 0, 2)
			if _, err = s.Enable(mode); err != nil {
				t.Fatal(err)
			}
			client := connect(t, s)
			listed, err := client.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, tool := range listed.Tools {
				if tool.Name == "get_financial_context_item" {
					found = true
					if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
						t.Fatal("missing read-only annotation")
					}
				}
			}
			if !found {
				t.Fatal("item not discoverable")
			}
			capture := decodeContext(t, call(t, client, "get_financial_context", map[string]any{}, false))
			args := skillExample(t, "analysis", "financial-context-item", map[string]string{"contextId": capture.ContextID, "ref": capture.Content.Scope.AccountRefs[0]})
			before := decodeItem(t, call(t, client, "get_financial_context_item", args, false))
			if before.Position.Complete || len(before.Positions) != 1 || *before.Positions[0].NativeAmount != "100" || before.Positions[0].BaseAmount != nil || len(before.Gaps) != 1 || before.Gaps[0].Code != "missing_fx" {
				t.Fatal(before)
			}
			if _, err = app.AppendAccountValue(t.Context(), account.Account.ID, "200", ""); err != nil {
				t.Fatal(err)
			}
			after := decodeItem(t, call(t, client, "get_financial_context_item", args, false))
			after.GeneratedAt = before.GeneratedAt
			if !reflect.DeepEqual(before, after) {
				t.Fatal("ordinary write changed frozen item")
			}
			for _, ref := range []string{before.Positions[0].Ref, before.Evidence[0].Ref} {
				r := decodeItem(t, call(t, client, "get_financial_context_item", FinancialContextItemInput{ContextID: capture.ContextID, Ref: ref}, false))
				if r.ContentHash != capture.ContentHash {
					t.Fatal(r)
				}
			}
			historical := decodeContext(t, call(t, client, "get_financial_context", map[string]any{"asOf": "2026-09-01"}, false))
			historicalItem := decodeItem(t, call(t, client, "get_financial_context_item", FinancialContextItemInput{ContextID: historical.ContextID, Ref: historical.Content.Scope.AccountRefs[0]}, false))
			if historicalItem.AsOf.Mode != "closed_day" || *historicalItem.Positions[0].NativeAmount != "100" || historicalItem.ContentHash != historical.ContentHash {
				t.Fatal(historicalItem)
			}
			named := decodeContext(t, call(t, client, "get_financial_context", map[string]any{"disclosure": "named"}, false))
			namedItem := decodeItem(t, call(t, client, "get_financial_context_item", skillExample(t, "analysis", "financial-context-item", map[string]string{"contextId": named.ContextID, "ref": named.Content.Scope.AccountRefs[0]}), false))
			if namedItem.Disclosure != "named" || namedItem.Position.Name != "PRIVATE NAME" || namedItem.Position.Ref != account.Account.ID.String() {
				t.Fatal(namedItem)
			}
			namedContent := skillContextDetails(t, client, named)
			for _, p := range namedContent.Positions {
				item := decodeItem(t, call(t, client, "get_financial_context_item", FinancialContextItemInput{ContextID: named.ContextID, Ref: p.Ref}, false))
				if item.Disclosure != "named" || item.ContentHash != named.ContentHash || item.Position == nil || !reflect.DeepEqual(*item.Position, p) {
					t.Fatal("named position differs from captured projection", item, p)
				}
				if (p.Kind == "account") != (item.Type == "account") {
					t.Fatal("UUID shape confused row kind", item)
				}
			}
			for _, e := range namedContent.Evidence {
				item := decodeItem(t, call(t, client, "get_financial_context_item", FinancialContextItemInput{ContextID: named.ContextID, Ref: e.Ref}, false))
				if item.Type != "evidence" || item.EvidenceItem == nil || !reflect.DeepEqual(*item.EvidenceItem, e) {
					t.Fatal(item)
				}
			}
			call(t, client, "get_financial_context_item", FinancialContextItemInput{ContextID: capture.ContextID, Ref: account.Account.ID.String()}, true)
			call(t, client, "get_financial_context_item", map[string]any{"contextId": capture.ContextID, "ref": before.Ref, "disclosure": "named"}, true)
			if _, err = app.AppendAccountValue(t.Context(), account.Account.ID, "300", ""); err != nil {
				t.Fatal(err)
			}
			frozenNamed := decodeItem(t, call(t, client, "get_financial_context_item", FinancialContextItemInput{ContextID: named.ContextID, Ref: namedItem.Ref}, false))
			frozenNamed.GeneratedAt = namedItem.GeneratedAt
			if !reflect.DeepEqual(frozenNamed, namedItem) {
				t.Fatal("named item recaptured after ordinary write")
			}
			raw, _ := json.Marshal(before)
			for _, secret := range []string{"PRIVATE NAME", "PRIVATE NOTE", "private.invalid", account.Account.ID.String(), "household"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("item leaked " + secret)
				}
			}
			if changes.Load() != 0 {
				t.Fatal("item emitted write event")
			}
		})
	}
}

func TestFinancialContextItemSkillPagination(t *testing.T) {
	t.Parallel()
	s, _, _ := fixture(t)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	synthetic, _ := itemFixture(t)
	s.contexts.mu.Lock()
	s.contexts.entries["package-a"] = synthetic.contexts.entries["package-a"]
	s.contexts.bytes = 1
	s.contexts.mu.Unlock()
	client := connect(t, s)
	first := decodeItem(t, call(t, client, "get_financial_context_item", skillExample(t, "analysis", "financial-context-item", map[string]string{"contextId": "package-a", "ref": "account-1"}), false))
	for section, info := range map[string]FinancialContextPageInfo{"positions": first.PositionsPage, "gaps": first.GapsPage, "evidence": first.EvidencePage} {
		count := info.Returned
		for info.HasMore {
			page := decodeItem(t, call(t, client, "get_financial_context_item", skillExample(t, "analysis", "financial-context-item-page", map[string]string{"contextId": "package-a", "ref": "account-1", "section": section, "cursor": info.NextCursor}), false))
			switch section {
			case "positions":
				info = page.PositionsPage
			case "gaps":
				info = page.GapsPage
			case "evidence":
				info = page.EvidencePage
			}
			if info.Returned == 0 {
				t.Fatal("nonadvancing page")
			}
			count += info.Returned
		}
		if count != info.Total {
			t.Fatal("lost rows", count, info)
		}
	}
}

func TestFinancialContextItemSourceTextDoesNotLeak(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	if _, err := fx.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: fx.brokerage, InstrumentID: fx.instrument, Quantity: "2", UnitCost: "5"}); err != nil {
		t.Fatal(err)
	}
	client := ledgerSession(t, fx)
	mutate(t, client, "import_market_data", map[string]any{"items": []any{map[string]any{"instrumentId": fx.instrument, "currency": "USD", "value": "7", "kind": "close", "date": "2026-09-28", "sourceTitle": "PRIVATE SOURCE TITLE", "sourceUrl": "https://private.invalid/source"}}})
	capture := decodeContext(t, call(t, client, "get_financial_context", map[string]any{"scope": map[string]any{"kind": "accounts", "accountIds": []string{fx.brokerage}}}, false))
	result := decodeItem(t, call(t, client, "get_financial_context_item", FinancialContextItemInput{ContextID: capture.ContextID, Ref: capture.Content.Scope.AccountRefs[0], Section: "evidence"}, false))
	var priceRef string
	for _, e := range result.Evidence {
		if e.Kind == "price" {
			priceRef = e.Ref
			if e.SourceKind != "agent" || e.Value == nil || *e.Value != "7" {
				t.Fatal(e)
			}
		}
	}
	if priceRef == "" {
		t.Fatal("source evidence missing")
	}
	item := decodeItem(t, call(t, client, "get_financial_context_item", FinancialContextItemInput{ContextID: capture.ContextID, Ref: priceRef}, false))
	for _, r := range []FinancialContextItemResponse{result, item} {
		raw, _ := json.Marshal(r)
		for _, secret := range []string{"PRIVATE SOURCE TITLE", "private.invalid", "Example stock", "Brokerage", fx.brokerage, fx.instrument} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("source/identity leaked: " + secret)
			}
		}
	}
}

func TestFinancialContextItemNamedMembershipAndCursors(t *testing.T) {
	t.Parallel()
	s, g := itemFixture(t)
	entry := s.contexts.entries["package-a"]
	entry.result.Content.Disclosure = "named"
	refs := map[string]string{}
	for i, p := range entry.result.Content.Positions {
		refs[p.Ref] = fmt.Sprintf("%08d-0000-4000-8000-000000000000", i+1)
	}
	for i := range entry.result.Content.Positions {
		p := &entry.result.Content.Positions[i]
		p.Ref = refs[p.Ref]
		p.ParentRef = refs[p.ParentRef]
		p.Name = fmt.Sprintf("Already disclosed name %d", i)
	}
	for i := range entry.result.Content.Gaps {
		gap := &entry.result.Content.Gaps[i]
		gap.EntityRef = refs[gap.EntityRef]
		if ref := refs[gap.DependencyRef]; ref != "" {
			gap.DependencyRef = ref
		}
	}
	s.contexts.entries["package-a"] = entry
	s.contexts.entries["package-b"] = entry
	for _, tc := range []struct{ ref, kind string }{{refs["account-1"], "account"}, {refs["position-2"], "position"}, {"evidence-3", "evidence"}} {
		r := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: tc.ref, Section: "positions", Limit: 1})
		if r.Type != tc.kind || r.Disclosure != "named" || r.ContentHash != entry.result.ContentHash {
			t.Fatal(r)
		}
		if r.Position != nil {
			found := false
			for _, p := range entry.result.Content.Positions {
				if p.Ref == r.Position.Ref {
					found = true
					if !reflect.DeepEqual(*r.Position, p) {
						t.Fatal("target no longer exact disclosed subset")
					}
				}
			}
			if !found {
				t.Fatal("invented target")
			}
		}
		for _, p := range r.Positions {
			found := false
			for _, original := range entry.result.Content.Positions {
				if reflect.DeepEqual(p, original) {
					found = true
				}
			}
			if !found {
				t.Fatal("expanded or modified disclosed position", p)
			}
		}
		if tc.kind == "position" {
			continue
		}
		if !r.PositionsPage.HasMore {
			t.Fatal("fixture must page")
		}
		for _, input := range []FinancialContextItemInput{
			{ContextID: "package-b", Ref: tc.ref, Section: "positions", Cursor: r.PositionsPage.NextCursor},
			{ContextID: "package-a", Ref: tc.ref, Section: "evidence", Cursor: r.PositionsPage.NextCursor},
			{ContextID: "package-a", Ref: refs["account-2"], Section: "positions", Cursor: r.PositionsPage.NextCursor},
		} {
			if _, err := s.financialContextItem(t.Context(), g, input); err == nil {
				t.Fatal("accepted mismatched named cursor", input)
			}
		}
	}
	for _, ref := range []string{"account-1", "position-2", "not-present"} {
		if _, err := s.financialContextItem(t.Context(), g, FinancialContextItemInput{ContextID: "package-a", Ref: ref}); err == nil {
			t.Fatal("inferred membership from ref spelling", ref)
		}
	}
	other := entry
	other.result.Content.Positions = nil
	other.result.Content.Evidence = nil
	s.contexts.entries["package-b"] = other
	for _, ref := range []string{refs["account-1"], refs["position-2"], "evidence-3"} {
		if _, err := s.financialContextItem(t.Context(), g, FinancialContextItemInput{ContextID: "package-b", Ref: ref}); err == nil {
			t.Fatal("accepted ref absent from this package", ref)
		}
	}
	// An impossible/ambiguous cache member fails rather than choosing by UUID shape.
	entry.result.Content.Evidence = append(entry.result.Content.Evidence, application.FinancialContextEvidence{Ref: refs["account-1"]})
	s.contexts.entries["package-a"] = entry
	if _, err := s.financialContextItem(t.Context(), g, FinancialContextItemInput{ContextID: "package-a", Ref: refs["account-1"]}); err == nil {
		t.Fatal("accepted ambiguous ref")
	}
}

func TestFinancialContextItemOversizedSectionDiagnostic(t *testing.T) {
	t.Parallel()
	for _, oversizedSection := range []string{"positions", "gaps", "evidence"} {
		t.Run(oversizedSection, func(t *testing.T) {
			s, g := itemFixture(t)
			entry := s.contexts.entries["package-a"]
			oversized := strings.Repeat("9", contextWireLimit)
			switch oversizedSection {
			case "positions":
				entry.result.Content.Positions[1].NativeAmount = &oversized
			case "gaps":
				entry.result.Content.Gaps[0].AffectedMetrics = []string{oversized}
			case "evidence":
				entry.result.Content.Evidence[0].Value = &oversized
			}
			s.contexts.entries["package-a"] = entry
			for _, section := range []string{"", oversizedSection} {
				_, err := s.financialContextItem(t.Context(), g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1", Section: section, Limit: 1})
				want := fail("too_large", "item "+oversizedSection+" row and required target exceed the MCP wire budget")
				if !reflect.DeepEqual(err, want) {
					t.Fatalf("section %q: got %v, want %v", section, err, want)
				}
			}
			// An overview failure does not prevent reading other sections of this same package.
			for _, section := range []string{"positions", "gaps", "evidence"} {
				if section == oversizedSection {
					continue
				}
				r := readItem(t, s, g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1", Section: section, Limit: 1})
				if r.ContentHash != entry.result.ContentHash {
					t.Fatal("recovery changed package")
				}
			}
		})
	}
	s, g := itemFixture(t)
	_, err := s.financialContextItem(t.Context(), g, FinancialContextItemInput{ContextID: "package-a", Ref: "account-1", Section: "PRIVATE INPUT"})
	if !reflect.DeepEqual(err, fail("validation", "unknown section")) {
		t.Fatalf("unvalidated section was echoed: %v", err)
	}
}
