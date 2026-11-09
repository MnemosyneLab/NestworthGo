package mcpserver

import (
	"errors"
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
	income := skillExample(t, "cash-and-debt", "income", values)["commands"].([]any)[0].(map[string]any)
	ledger := ledgerPreview(t, c, income)["planId"].(string)
	plans := map[string]string{
		"commit_batch":          ledger,
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
	args := map[string]any{"operationId": operationID, "input": map[string]any{"planId": ledger}}
	receipt := call(t, c, "commit_plan", args, false)["data"].(map[string]any)
	if receipt["tool"] != "commit_batch" || ledgerActivityCount(t, fx.app) == before {
		t.Fatalf("matching commit receipt = %v", receipt)
	}
	if retry := call(t, c, "commit_plan", args, false)["data"].(map[string]any); retry["id"] != receipt["id"] || retry["tool"] != "commit_batch" {
		t.Fatalf("retry did not replay the receipt: %v", retry)
	}
	// The other previews are now stale and must not write.
	for _, kind := range []string{"commit_reconciliation"} {
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
	planID := ledgerPreviewInput(t, c, skillExample(t, "cash-and-debt", "income", values))["planId"].(string)
	args := map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}}
	receipt := call(t, c, "commit_plan", args, false)["data"].(map[string]any)
	path := filepath.Join(fx.service.dir, "plans", planID+".json")
	if err := fx.service.writePrivate(path, map[string]any{"id": planID}); err != nil {
		t.Fatal(err)
	}
	if retry := commitPlanData(t, c, args); retry["id"] != receipt["id"] {
		t.Fatalf("retry with obsolete plan shape = %v", retry)
	}
	wrongPlan := map[string]any{"operationId": args["operationId"], "input": map[string]any{"planId": uuid.NewString()}}
	if code := ledgerErrorCode(t, c, "commit_plan", wrongPlan); code != "conflict" {
		t.Fatalf("different plan on completed operation = %q", code)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if retry := call(t, c, "commit_plan", args, false)["data"].(map[string]any); retry["id"] != receipt["id"] {
		t.Fatalf("retry after pruning = %v", retry)
	}
	recovered := commitPlanData(t, c, map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}})
	if recovered["tool"] != "commit_batch" ||
		ledgerActivityID(t, recovered["result"].(map[string]any)) != ledgerActivityID(t, receipt["result"].(map[string]any)) ||
		ledgerActivityCount(t, fx.app) != 1 {
		t.Fatalf("durable batch receipt recovered under the wrong family or reposted: %v", recovered)
	}
}

func TestCommitPlanObsoletePlanDoesNotResolvePendingOperation(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	planID := ledgerPreview(t, c, incomeRequest(fx.checking, "10"))["planId"].(string)
	operationID := uuid.NewString()
	input := CommitChangeInput{PlanID: planID}
	hash, err := operationHash("commit_batch", input)
	if err != nil {
		t.Fatal(err)
	}
	receiptPath, err := fx.service.operationPath(operationID)
	if err != nil {
		t.Fatal(err)
	}
	prior := Operation{ID: operationID, Tool: "commit_batch", Hash: hash, Status: "pending"}
	if err := fx.service.writePrivate(receiptPath, prior); err != nil {
		t.Fatal(err)
	}
	if err := fx.service.writePrivate(filepath.Join(fx.service.dir, "plans", planID+".json"), map[string]any{"id": planID}); err != nil {
		t.Fatal(err)
	}
	_, err = fx.service.resolveCommitPlanRoute(t.Context(), operationID, planID)
	var marker interface{ NotDispatched() bool }
	if err == nil || safeError(err).Code != "operation_outcome_unknown" || errors.As(err, &marker) {
		t.Fatalf("unresolved prior classified as a new unsubmitted operation: %v", err)
	}
	args := map[string]any{"operationId": operationID, "input": input}
	if code := ledgerErrorCode(t, c, "commit_plan", args); code != "operation_outcome_unknown" {
		t.Fatalf("obsolete plan with pending receipt = %q", code)
	}
	receipt, err := fx.service.GetOperation(operationID)
	if err != nil || receipt.Status != "unknown" || receipt.Hash != prior.Hash || ledgerActivityCount(t, fx.app) != 0 {
		t.Fatalf("ambiguous operation changed: %+v, %v", receipt, err)
	}
}
