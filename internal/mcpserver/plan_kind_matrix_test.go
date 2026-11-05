package mcpserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// commit_plan derives the family from the stored plan. The receipt keeps the
// historical per-kind tool name, so hashes, replay rules and host review
// bindings written before the unified tool stay valid.
func TestCommitPlanRoutesByStoredPlanKind(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	values := map[string]string{"accountId": fx.checking, "effectiveAt": "2026-09-29T12:00:00Z"}
	income := skillExample(t, "cash-and-debt", "income", values)
	plans := map[string]string{
		"commit_change":         ledgerPreview(t, c, income)["planId"].(string),
		"commit_batch":          batchPreview(t, c, income, income)["planId"].(string),
		"commit_reconciliation": call(t, c, "preview_reconciliation", skillExample(t, "reconciliation-and-corrections", "balance-target", values), false)["data"].(map[string]any)["planId"].(string),
	}
	// Plans are version-bound, so committing one makes the others stale; the
	// route is checked per plan without committing it.
	for want, planID := range plans {
		route, err := fx.service.resolveCommitPlanRoute(t.Context(), uuid.NewString(), planID)
		if err != nil || route.receiptTool != want || !route.recoverable {
			t.Fatalf("%s plan routed to %+v, %v", want, route, err)
		}
	}
	before := ledgerActivityCount(t, fx.app)
	operationID := uuid.NewString()
	args := map[string]any{"operationId": operationID, "input": map[string]any{"planId": plans["commit_change"]}}
	receipt := call(t, c, "commit_plan", args, false)["data"].(map[string]any)
	if receipt["tool"] != "commit_change" || ledgerActivityCount(t, fx.app) == before {
		t.Fatalf("matching commit receipt = %v", receipt)
	}
	if retry := call(t, c, "commit_plan", args, false)["data"].(map[string]any); retry["id"] != receipt["id"] || retry["tool"] != "commit_change" {
		t.Fatalf("retry did not replay the receipt: %v", retry)
	}
	// The other previews are now stale and must not write.
	for _, kind := range []string{"commit_batch", "commit_reconciliation"} {
		if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": plans[kind]}}); code != "stale_preview" {
			t.Fatalf("%s plan code = %q", kind, code)
		}
	}
}

func TestCommitPlanRejectsUnusablePlanReferences(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": "not-a-uuid"}}); code != "validation" {
		t.Fatalf("invalid plan id code = %q", code)
	}
	if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": uuid.NewString()}}); code != "operation_outcome_unknown" {
		t.Fatalf("missing plan code = %q", code)
	}
	unknown := uuid.NewString()
	var err error
	if err = fx.service.writePrivate(filepath.Join(fx.service.dir, "plans", unknown+".json"), map[string]any{"id": unknown, "type": "forged_kind"}); err != nil {
		t.Fatal(err)
	}
	if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": unknown}}); code != "validation" {
		t.Fatalf("unknown plan kind code = %q", code)
	}
}

// A used plan whose file was pruned must still replay its original receipt
// for the same operationId.
func TestCommitPlanReplaysReceiptAfterPlanFileIsPruned(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	values := map[string]string{"accountId": fx.checking, "effectiveAt": "2026-09-29T12:00:00Z"}
	planID := ledgerPreview(t, c, skillExample(t, "cash-and-debt", "income", values))["planId"].(string)
	args := map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}}
	receipt := call(t, c, "commit_plan", args, false)["data"].(map[string]any)
	if err := os.Remove(filepath.Join(fx.service.dir, "plans", planID+".json")); err != nil {
		t.Fatal(err)
	}
	if retry := call(t, c, "commit_plan", args, false)["data"].(map[string]any); retry["id"] != receipt["id"] {
		t.Fatalf("retry after pruning = %v", retry)
	}
	if code := ledgerErrorCode(t, c, "commit_plan", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}}); code != "operation_outcome_unknown" {
		t.Fatalf("new operation for pruned plan = %q", code)
	}
}
