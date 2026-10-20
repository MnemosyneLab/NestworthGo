package mcpserver

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	refused, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: "preview_reconciliation_session", Arguments: map[string]any{"action": "check", "sessionId": id, "expectedRevision": v["revision"]}})
	if err != nil || !refused.IsError {
		t.Fatalf("completed session check: %+v %v", refused, err)
	}
	raw, _ := jsoncodec.Marshal(refused)
	for _, want := range []string{"conflict", "expectedRevision", "completed and read-only", "start a new reconciliation session"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("completed session response missing %q: %s", want, raw)
		}
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
	raw, _ := jsoncodec.Marshal(v)
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

func TestReconciliationSessionNotificationsDoNotInvalidateFinancialPerformance(t *testing.T) {
	fx := newLedgerFixture(t)
	var financial, session atomic.Int32
	fx.service.changed = func() { financial.Add(1) }
	fx.service.SetReconciliationChanged(func() { session.Add(1) })
	c := ledgerSession(t, fx)
	v := sessionAction(t, c, map[string]any{"action": "start", "accountIds": []string{fx.checking}})
	if session.Load() != 1 || financial.Load() != 0 {
		t.Fatalf("session notification counts %d / %d", session.Load(), financial.Load())
	}
	p := reconciliationPreview(t, c, map[string]any{"accountId": fx.checking, "targetBalance": "25", "currency": "USD"})
	reconciliationCommit(t, c, uuid.NewString(), p["planId"].(string))
	if financial.Load() != 1 || session.Load() != 1 {
		t.Fatalf("financial notification counts %d / %d", financial.Load(), session.Load())
	}
	sessionAction(t, c, map[string]any{"action": "pause", "sessionId": v["id"], "expectedRevision": v["revision"]})
	if financial.Load() != 1 || session.Load() != 2 {
		t.Fatal("pause leaked a broad financial invalidation")
	}
}

func TestReconciliationSessionCivilDateFiltersAndPaging(t *testing.T) {
	for _, tc := range []struct {
		name, zone, date string
		instants         []string
		matched          []int
	}{
		{"positive_midnight", "Asia/Tokyo", "2026-09-02", []string{"2026-09-01T14:59:59Z", "2026-09-01T15:00:00Z", "2026-09-02T14:59:59Z", "2026-09-02T15:00:00Z"}, []int{1, 2}},
		{"negative_midnight", "America/Los_Angeles", "2026-09-01", []string{"2026-09-01T06:59:59Z", "2026-09-01T07:00:00Z", "2026-09-02T06:59:59Z", "2026-09-02T07:00:00Z"}, []int{1, 2}},
		{"spring_DST", "America/New_York", "2026-03-08", []string{"2026-03-08T04:59:59Z", "2026-03-08T05:00:00Z", "2026-03-09T03:59:59Z", "2026-03-09T04:00:00Z"}, []int{1, 2}},
		{"fall_DST", "America/New_York", "2026-11-01", []string{"2026-11-01T03:59:59Z", "2026-11-01T04:00:00Z", "2026-11-02T04:59:59Z", "2026-11-02T05:00:00Z"}, []int{1, 2}},
		{"no_origin_UTC", "", "2026-09-01", []string{"2026-08-31T23:59:59Z", "2026-09-01T00:00:00Z", "2026-09-01T23:59:59Z", "2026-09-02T00:00:00Z"}, []int{1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := sqlite.Open(filepath.Join(t.TempDir(), "synthetic.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			app := application.NewService(sqlite.NewRepository(db))
			now, _ := time.Parse(time.RFC3339, tc.instants[0])
			app.SetClock(func() time.Time { return now })
			if err = app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic", BaseCurrency: "USD", MemberNames: []string{"Tester"}}); err != nil {
				t.Fatal(err)
			}
			if tc.zone != "" {
				if _, err = app.StartHistory(ctx, tc.zone); err != nil {
					t.Fatal(err)
				}
			}
			bootstrap, _ := app.Bootstrap(ctx)
			account, err := app.CreateAccount(ctx, application.AccountInput{Name: "Synthetic bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "0", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			s := New(app, t.TempDir(), nil)
			defer s.Close()
			ids := []string{}
			for _, instant := range tc.instants {
				now, _ = time.Parse(time.RFC3339, instant)
				plan, err := app.PreviewReconciliationSession(ctx, application.ReconciliationAction{Action: "start"})
				if err != nil {
					t.Fatal(err)
				}
				v, err := app.CommitReconciliationSession(ctx, plan)
				if err != nil {
					t.Fatal(err)
				}
				ids = append(ids, v.ID)
			}
			for _, sort := range []string{"asc", "desc"} {
				input := ReconciliationReadInput{From: tc.date, To: tc.date, AccountID: account.Account.ID.String(), Status: "in_progress", Sort: sort, Limit: 1}
				result, err := s.listReconciliationSessions(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				page := result.(reconciliationPage)
				first, second := tc.matched[0], tc.matched[1]
				if sort == "desc" {
					first, second = second, first
				}
				if page.Total != 2 || len(page.Items) != 1 || page.Items[0].(domain.ReconciliationSession).ID != ids[first] || page.NextCursor == "" {
					t.Fatalf("first page: %+v", page)
				}
				input.Cursor = page.NextCursor
				result, err = s.listReconciliationSessions(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				page = result.(reconciliationPage)
				if page.Total != 2 || len(page.Items) != 1 || page.Items[0].(domain.ReconciliationSession).ID != ids[second] || page.NextCursor != "" {
					t.Fatalf("second page: %+v", page)
				}
				input.Status = "paused"
				if _, err = s.listReconciliationSessions(ctx, input); err == nil {
					t.Fatal("changed filter reused cursor")
				}
			}
			// Invalid persisted timestamps must produce a safe failure, including short strings.
			for _, bad := range []string{"x", "2026-99-99T00:00:00Z"} {
				if _, err = db.SQL.Exec("UPDATE reconciliation_sessions SET started_at=? WHERE id=?", bad, ids[0]); err != nil {
					t.Fatal(err)
				}
				if _, err = s.listReconciliationSessions(ctx, ReconciliationReadInput{From: tc.date}); err == nil {
					t.Fatal("invalid timestamp accepted")
				}
			}
		})
	}
}

func TestReconciliationSQLiteMissingPlanUnknownAndReceiptRecovery(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }
	fx, db, _ := newPersistentProductFixture(t, clock)
	c := ledgerSession(t, fx)
	preview := call(t, c, "preview_reconciliation_session", map[string]any{"action": "start", "accountIds": []string{fx.checking}, "note": "Opening context"}, false)["data"].(map[string]any)
	planID := preview["planId"].(string)
	operationID := uuid.NewString()
	args := map[string]any{"operationId": operationID, "input": map[string]any{"planId": planID}}
	posted := call(t, c, "commit_reconciliation_session", args, false)["data"].(map[string]any)
	operation, err := fx.service.GetOperation(operationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.SQL.Exec("DELETE FROM app_configuration WHERE key=?", "mcp.plan."+planID); err != nil {
		t.Fatal(err)
	}
	// An already-succeeded outer receipt is independent of plan retention.
	replay := call(t, c, "commit_reconciliation_session", args, false)["data"].(map[string]any)
	if replay["id"] != posted["id"] {
		t.Fatal("succeeded operation receipt changed")
	}
	// Reproduce interruption after business commit but before outer receipt save.
	operation.Status = "pending"
	operation.Result = nil
	path, _ := fx.service.operationPath(operationID)
	if err = fx.service.writePrivate(path, operation); err != nil {
		t.Fatal(err)
	}
	recovered := call(t, c, "commit_reconciliation_session", args, false)["data"].(map[string]any)
	if recovered["result"].(map[string]any)["id"] != planID {
		t.Fatal("business receipt not recovered")
	}

	pending := call(t, c, "preview_reconciliation_session", map[string]any{"action": "start", "accountIds": []string{fx.savings}}, false)["data"].(map[string]any)
	missingID := pending["planId"].(string)
	raw, err := fx.service.loadPreviewPlan(missingID, "reconciliation_session")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.SQL.Exec("DELETE FROM app_configuration WHERE key=?", "mcp.plan."+missingID); err != nil {
		t.Fatal(err)
	}
	pendingOp := uuid.NewString()
	missingArgs := map[string]any{"operationId": pendingOp, "input": map[string]any{"planId": missingID}}
	if code := ledgerErrorCode(t, c, "commit_reconciliation_session", missingArgs); code != "operation_outcome_unknown" {
		t.Fatal(code)
	}
	saved, err := fx.service.GetOperation(pendingOp)
	if err != nil || saved.Status != "unknown" {
		t.Fatalf("missing plan permanently failed: %+v %v", saved, err)
	}
	// Recover older versions' permanently failed not_found operations too.
	saved.Status = "failed"
	saved.Error = safeError(fail("not_found", "old missing plan"))
	path, _ = fx.service.operationPath(pendingOp)
	if err = fx.service.writePrivate(path, saved); err != nil {
		t.Fatal(err)
	}
	if code := ledgerErrorCode(t, c, "commit_reconciliation_session", missingArgs); code != "operation_outcome_unknown" {
		t.Fatal(code)
	}
	var body any
	if err = jsoncodec.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if err = fx.service.writePrivate(filepath.Join(fx.service.dir, "plans", missingID+".json"), body); err != nil {
		t.Fatal(err)
	}
	// Only an explicit retry submits the recovered, unchanged reviewed plan.
	call(t, c, "commit_reconciliation_session", missingArgs, false)
	rows, err := fx.app.ListReconciliationSessions(t.Context())
	if err != nil || len(rows) != 2 {
		t.Fatalf("unexpected executions: %d %v", len(rows), err)
	}
}
