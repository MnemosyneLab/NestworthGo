package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func sessionAction(t *testing.T, c *mcp.ClientSession, in map[string]any) map[string]any {
	t.Helper()
	p := call(t, c, "preview_reconciliation_session", in, false)["data"].(map[string]any)
	result := call(t, c, "commit_reconciliation_session", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": p["planId"]}}, false)["data"].(map[string]any)
	return result["result"].(map[string]any)
}
func sessionHeader(t *testing.T, c *mcp.ClientSession, id string) map[string]any {
	t.Helper()
	return call(t, c, "get_reconciliation_session", map[string]any{"id": id}, false)["data"].(map[string]any)
}
func TestReconciliationSessionToolPermissions(t *testing.T) {
	fx := newLedgerFixture(t)
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		if _, err := fx.service.Enable(mode); err != nil {
			t.Fatal(err)
		}
		c := connect(t, fx.service)
		list, err := c.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		for _, tool := range list.Tools {
			names[tool.Name] = true
		}
		for _, name := range []string{"list_reconciliation_sessions", "get_reconciliation_session", "get_reconciliation_session_page", "get_reconciliation_item"} {
			if !names[name] {
				t.Fatalf("missing %s in %s", name, mode)
			}
		}
		for _, name := range []string{"preview_reconciliation_session", "commit_reconciliation_session"} {
			if names[name] != (mode == LedgerWrite) {
				t.Fatalf("%s in %s", name, mode)
			}
		}
		c.Close()
	}
}
func TestReconciliationSessionHTTPWorkflowBothCorrectionRoutes(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	v := sessionAction(t, c, map[string]any{"action": "start", "accountIds": []string{fx.checking}})
	id := v["id"].(string)
	header := sessionHeader(t, c, id)
	if _, ok := header["accounts"]; ok {
		t.Fatal("header embedded all accounts")
	}
	page := call(t, c, "get_reconciliation_session_page", map[string]any{"sessionId": id, "section": "items", "limit": 1}, false)["data"].(map[string]any)
	if page["total"].(float64) != 2 {
		t.Fatal(page)
	}
	cursor := page["nextCursor"].(string)
	item := page["items"].([]any)[0].(map[string]any)["item"].(map[string]any)
	link := map[string]any{"sessionId": id, "accountId": fx.checking, "itemId": item["id"], "expectedRevision": header["revision"], "financialRevision": item["financialRevision"]}
	correction := call(t, c, "preview_reconciliation", map[string]any{"reconciliationLink": link, "targets": []any{map[string]any{"accountId": fx.checking, "targetBalance": "25", "currency": "USD"}}}, false)["data"].(map[string]any)
	reconciliationCommit(t, c, uuid.NewString(), correction["planId"].(string))
	if code := ledgerErrorCode(t, c, "get_reconciliation_session_page", map[string]any{"sessionId": id, "section": "items", "limit": 1, "cursor": cursor}); code != "stale_cursor" {
		t.Fatalf("stale paging: %s", code)
	}
	full := call(t, c, "get_reconciliation_item", map[string]any{"sessionId": id, "accountId": fx.checking, "itemId": item["id"]}, false)["data"].(map[string]any)
	item = full["item"].(map[string]any)
	if item["outcome"] != "pending" {
		t.Fatal("correction confirmed evidence")
	}
	link["expectedRevision"] = full["sessionRevision"]
	link["financialRevision"] = item["financialRevision"]
	missing := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "2", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z", "reconciliationLink": link})
	ledgerCommit(t, c, uuid.NewString(), missing["planId"].(string))
	links := call(t, c, "get_reconciliation_session_page", map[string]any{"sessionId": id, "section": "links"}, false)["data"].(map[string]any)
	if links["total"].(float64) != 2 {
		t.Fatal(links)
	}
	header = sessionHeader(t, c, id)
	v = sessionAction(t, c, map[string]any{"action": "pause", "sessionId": id, "expectedRevision": header["revision"]})
	v = sessionAction(t, c, map[string]any{"action": "resume", "sessionId": id, "expectedRevision": v["revision"]})
	v = sessionAction(t, c, map[string]any{"action": "finish", "sessionId": id, "expectedRevision": v["revision"], "allowPartial": true, "note": "Synthetic source still pending"})
	if v["partial"] != true || v["counts"].(map[string]any)["confirmed"].(float64) != 0 {
		t.Fatal(v)
	}
	original := sessionHeader(t, c, id)
	if original["completedAt"] != v["completedAt"] {
		t.Fatal("completed evidence changed")
	}
}
func TestReconciliationPageBoundsAndWireBudget(t *testing.T) {
	rows := make([]any, 51)
	for i := range rows {
		rows[i] = map[string]any{"id": i, "note": strings.Repeat("x", 2000), "source": strings.Repeat("s", 512)}
	}
	in := ReconciliationReadInput{Limit: 25}
	v, err := pageReconciliation(in, rows, 3)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(v)
	if len(raw) > 96*1024 {
		t.Fatal(len(raw))
	}
	page := v.(reconciliationPage)
	if len(page.Items) != 25 || page.NextCursor == "" {
		t.Fatal(page)
	}
	in.Cursor = page.NextCursor
	v, err = pageReconciliation(in, rows, 3)
	if err != nil || len(v.(reconciliationPage).Items) != 25 {
		t.Fatal(err)
	}
	if _, err = pageReconciliation(in, rows, 4); err == nil {
		t.Fatal("stale cursor accepted")
	}
	in.Limit = 51
	if _, err = pageReconciliation(in, rows, 3); err == nil {
		t.Fatal("oversized page accepted")
	}
}

func TestReconciliationLinkedBatchUsesOneAtomicScope(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	v := sessionAction(t, c, map[string]any{"action": "start", "accountIds": []string{fx.checking}})
	id := v["id"].(string)
	point := call(t, c, "get_reconciliation_item", map[string]any{"sessionId": id, "accountId": fx.checking, "itemId": "balance"}, false)["data"].(map[string]any)
	item := point["item"].(map[string]any)
	link := map[string]any{"sessionId": id, "accountId": fx.checking, "itemId": "balance", "expectedRevision": point["sessionRevision"], "financialRevision": item["financialRevision"]}
	p := call(t, c, "preview_batch", map[string]any{"reconciliationLink": link, "commands": []any{map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "1", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"}, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "2", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"}}}, false)["data"].(map[string]any)
	call(t, c, "commit_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": p["planId"]}}, false)
	links := call(t, c, "get_reconciliation_session_page", map[string]any{"sessionId": id, "section": "links"}, false)["data"].(map[string]any)
	if links["total"].(float64) != 2 {
		t.Fatal(links)
	}
}
