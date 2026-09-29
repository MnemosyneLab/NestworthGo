package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func reconciliationPreview(t *testing.T, c *mcp.ClientSession, targets ...map[string]any) map[string]any {
	t.Helper()
	data := call(t, c, "preview_reconciliation", map[string]any{"targets": targets}, false)["data"].(map[string]any)
	if _, err := uuid.Parse(data["planId"].(string)); err != nil {
		t.Fatalf("invalid reconciliation plan ID: %v", data)
	}
	if _, err := time.Parse(time.RFC3339Nano, data["expiresAt"].(string)); err != nil {
		t.Fatalf("invalid reconciliation expiry: %v", data)
	}
	return data
}

func reconciliationCommit(t *testing.T, c *mcp.ClientSession, operationID, planID string) map[string]any {
	t.Helper()
	data := call(t, c, "commit_reconciliation", map[string]any{
		"operationId": operationID, "input": map[string]any{"planId": planID},
	}, false)["data"].(map[string]any)
	if data["status"] != "succeeded" {
		t.Fatalf("reconciliation receipt: %v", data)
	}
	return data["result"].(map[string]any)
}

func TestReconciliationToolsRequireLedgerPermission(t *testing.T) {
	fx := newLedgerFixture(t)
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		if _, err := fx.service.Enable(mode); err != nil {
			t.Fatal(err)
		}
		c := connect(t, fx.service)
		listed, err := c.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, tool := range listed.Tools {
			found[tool.Name] = true
		}
		for _, name := range []string{"preview_reconciliation", "commit_reconciliation"} {
			if found[name] != (mode == LedgerWrite) {
				t.Fatalf("%s exposed=%v in mode %s", name, found[name], mode)
			}
		}
		c.Close()
	}
}

func TestReconciliationPreviewCommitAndReplay(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	imported := ledgerPreview(t, c, map[string]any{
		"kind": "position_import", "accountId": fx.brokerage, "instrumentId": fx.instrument,
		"quantity": "10", "unitCost": "25", "currency": "USD", "effectiveAt": "2026-09-20T12:00:00Z",
	})
	ledgerCommit(t, c, uuid.NewString(), imported["planId"].(string))
	holdings := batchHoldings(t, c, fx.brokerage)
	if len(holdings) != 1 {
		t.Fatalf("imported holdings=%v", holdings)
	}
	holdingID := holdings[0].(map[string]any)["id"].(string)
	if code := ledgerErrorCode(t, c, "preview_reconciliation", map[string]any{"targets": []any{map[string]any{"holdingId": holdingID, "targetQuantity": "12"}}}); code != "cost_basis_required" {
		t.Fatalf("added quantity without unit cost code=%s", code)
	}
	before := ledgerActivityCount(t, fx.app)
	plan := reconciliationPreview(t, c,
		map[string]any{"accountId": fx.checking, "targetBalance": "25", "currency": "USD"},
		map[string]any{"accountId": fx.brokerage, "targetBalance": "30", "currency": "USD"},
		map[string]any{"holdingId": holdingID, "targetQuantity": "12", "unitCost": "30"},
	)
	commands := plan["commands"].([]any)
	resolvedTargets := plan["targets"].([]any)
	if len(resolvedTargets) != 3 || resolvedTargets[2].(map[string]any)["holdingId"] != holdingID || resolvedTargets[2].(map[string]any)["unitCost"] != "30" {
		t.Fatalf("preview target echo=%v", resolvedTargets)
	}
	for i, kind := range []string{"value_update", "money_added", "position_adjustment"} {
		if commands[i].(map[string]any)["kind"] != kind {
			t.Fatalf("command %d=%v want %s", i, commands[i], kind)
		}
	}
	if commands[0].(map[string]any)["reason"] != "reconciliation" || commands[1].(map[string]any)["reason"] != "reconciliation" {
		t.Fatalf("missing reconciliation reason: %v", commands)
	}
	if got := ledgerActivityCount(t, fx.app); got != before {
		t.Fatalf("preview posted activities: %d -> %d", before, got)
	}
	if got := ledgerCash(t, c, fx.checking); got != "0" {
		t.Fatalf("preview changed checking balance=%s", got)
	}
	if got := batchCash(t, c, fx.brokerage); got != "0" {
		t.Fatalf("preview changed brokerage cash=%s", got)
	}
	if got := batchHoldings(t, c, fx.brokerage)[0].(map[string]any)["quantity"]; got != "10" {
		t.Fatalf("preview changed holding quantity=%v", got)
	}
	opID, planID := uuid.NewString(), plan["planId"].(string)
	first := batchChanges(t, reconciliationCommit(t, c, opID, planID), 3)
	if got := ledgerCash(t, c, fx.checking); got != "25" {
		t.Fatalf("checking=%s want 25", got)
	}
	if got := batchCash(t, c, fx.brokerage); got != "30" {
		t.Fatalf("brokerage cash=%s want 30", got)
	}
	if got := batchHoldings(t, c, fx.brokerage)[0].(map[string]any)["quantity"]; got != "12" {
		t.Fatalf("holding quantity=%v want 12", got)
	}
	assertReconciliationCost(t, fx, holdingID, "12", "25.8333", "310")
	firstID := first[0].(map[string]any)["activity"].(map[string]any)["id"]
	for _, retryID := range []string{opID, uuid.NewString()} {
		retry := batchChanges(t, reconciliationCommit(t, c, retryID, planID), 3)
		if retry[0].(map[string]any)["activity"].(map[string]any)["id"] != firstID {
			t.Fatalf("replay changed first activity: %v", retry)
		}
	}
	if got := ledgerActivityCount(t, fx.app); got != before+3 {
		t.Fatalf("replay duplicated activities: %d", got)
	}
	if code := ledgerErrorCode(t, c, "commit_reconciliation", map[string]any{"operationId": opID, "input": map[string]any{"planId": uuid.NewString()}}); code != "conflict" {
		t.Fatalf("reused operation with different plan code=%s", code)
	}
	reduction := reconciliationPreview(t, c, map[string]any{"holdingId": holdingID, "targetQuantity": "9"})
	batchChanges(t, reconciliationCommit(t, c, uuid.NewString(), reduction["planId"].(string)), 1)
	assertReconciliationCost(t, fx, holdingID, "9", "25.8333", "232.5")
}

func assertReconciliationCost(t *testing.T, fx ledgerFixture, holdingID, quantity, average, total string) {
	t.Helper()
	id, err := domain.ParseHoldingID(holdingID)
	if err != nil {
		t.Fatal(err)
	}
	gain, err := fx.app.HoldingGain(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if gain.Quantity != quantity || gain.AverageCost.Amount != average || gain.TotalCost.Amount != total || gain.TotalCost.Currency != "USD" {
		t.Fatalf("holding gain quantity=%s average=%s total=%s %s; want %s %s %s USD", gain.Quantity, gain.AverageCost.Amount, gain.TotalCost.Amount, gain.TotalCost.Currency, quantity, average, total)
	}
}

func TestReconciliationReducesCashAndQuantityWithoutNewCost(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	seed := batchPreview(t, c,
		map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "100", "currency": "USD", "reason": "contribution", "effectiveAt": "2026-09-20T12:00:00Z"},
		map[string]any{"kind": "position_import", "accountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "5", "unitCost": "10", "currency": "USD", "effectiveAt": "2026-09-20T12:00:00Z"},
	)
	batchCommit(t, c, uuid.NewString(), seed["planId"].(string))
	holdingID := batchHoldings(t, c, fx.brokerage)[0].(map[string]any)["id"].(string)
	plan := reconciliationPreview(t, c,
		map[string]any{"accountId": fx.brokerage, "targetBalance": "70", "currency": "USD"},
		map[string]any{"holdingId": holdingID, "targetQuantity": "3"},
	)
	commands := plan["commands"].([]any)
	if commands[0].(map[string]any)["kind"] != "money_removed" || commands[0].(map[string]any)["amount"] != "30" || commands[1].(map[string]any)["added"] != nil {
		t.Fatalf("unexpected reduction commands=%v", commands)
	}
	batchChanges(t, reconciliationCommit(t, c, uuid.NewString(), plan["planId"].(string)), 2)
	if got := batchCash(t, c, fx.brokerage); got != "70" {
		t.Fatalf("brokerage cash=%s want 70", got)
	}
	if got := batchHoldings(t, c, fx.brokerage)[0].(map[string]any)["quantity"]; got != "3" {
		t.Fatalf("holding quantity=%v want 3", got)
	}
}

func TestReconciliationRejectsInvalidTargetsAndAtomicFailure(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	valid := map[string]any{"accountId": fx.checking, "targetBalance": "50", "currency": "USD"}
	cases := []struct {
		name    string
		targets []any
	}{
		{"empty", []any{}},
		{"missing currency", []any{map[string]any{"accountId": fx.checking, "targetBalance": "50"}}},
		{"mixed endpoints", []any{map[string]any{"accountId": fx.checking, "holdingId": uuid.NewString(), "targetBalance": "50", "currency": "USD"}}},
		{"total cost", []any{map[string]any{"holdingId": uuid.NewString(), "targetQuantity": "5", "totalCost": "100"}}},
		{"second target invalid", []any{valid, map[string]any{"accountId": fx.savings, "targetBalance": "-1", "currency": "USD"}}},
		{"duplicate", []any{valid, valid}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := ledgerErrorCode(t, c, "preview_reconciliation", map[string]any{"targets": tc.targets}); code == "" {
				t.Fatal("invalid targets had no error code")
			}
		})
	}
	if got := ledgerActivityCount(t, fx.app); got != 0 {
		t.Fatalf("invalid previews posted %d activities", got)
	}
	if got := ledgerCash(t, c, fx.checking); got != "0" {
		t.Fatalf("invalid previews changed balance=%s", got)
	}
}

func TestReconciliationRejectsArchivedHolding(t *testing.T) {
	fx := newLedgerFixture(t)
	ctx := context.Background()
	holding, err := fx.app.CreateHolding(ctx, application.HoldingInput{
		AccountID: fx.brokerage, InstrumentID: fx.instrument, Quantity: "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.app.ArchiveHolding(ctx, holding.ID, true); err != nil {
		t.Fatal(err)
	}
	c := ledgerSession(t, fx)
	if code := ledgerErrorCode(t, c, "preview_reconciliation", map[string]any{"targets": []any{map[string]any{
		"holdingId": holding.ID.String(), "targetQuantity": "1", "unitCost": "10",
	}}}); code != "conflict" {
		t.Fatalf("archived holding code=%s", code)
	}
	if got := ledgerActivityCount(t, fx.app); got != 0 {
		t.Fatalf("archived preview posted %d activities", got)
	}
}

func TestReconciliationRejectsManagedHolding(t *testing.T) {
	fx := newLedgerFixture(t)
	ctx := context.Background()
	maturity, calendar, zero := "2026-12-20", "calendar", 0
	command := application.ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &application.RecordExistingProductCommand{
		AccountID: fx.brokerage, Currency: "USD", Principal: "100", TotalCostBasis: "100", CurrentValue: "100",
		CashExcludesProduct: true, EffectiveAt: "2026-09-29T12:00:00Z",
		Terms:  application.ProductTermsInput{Kind: "term_deposit", Name: "Managed deposit", StartOn: "2026-09-29", MaturityOn: &maturity, InterestMode: "none"},
		Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &maturity, EarlyKind: "not_allowed", SettlementDays: &zero, DayBasis: &calendar},
	}}
	preview, err := fx.app.PreviewProductOperation(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := fx.app.RecordProductOperation(ctx, command, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := fx.app.Product(ctx, receipt.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	c := ledgerSession(t, fx)
	before := ledgerActivityCount(t, fx.app)
	if code := ledgerErrorCode(t, c, "preview_reconciliation", map[string]any{"targets": []any{map[string]any{
		"holdingId": detail.Contract.HoldingID.String(), "targetQuantity": "2", "unitCost": "100",
	}}}); code != "managed_position_conflict" {
		t.Fatalf("managed holding code=%s", code)
	}
	if got := ledgerActivityCount(t, fx.app); got != before {
		t.Fatalf("managed preview posted activities: %d -> %d", before, got)
	}
}

func TestReconciliationRejectsStaleExpiredAndWrongPlanType(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	target := map[string]any{"accountId": fx.checking, "targetBalance": "50", "currency": "USD"}
	plan := reconciliationPreview(t, c, target)
	planID := plan["planId"].(string)
	for _, name := range []string{"commit_change", "commit_batch"} {
		if code := ledgerErrorCode(t, c, name, map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}}); code != "validation" {
			t.Fatalf("%s accepted reconciliation plan: %s", name, code)
		}
	}
	single := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.savings, "amount": "1", "currency": "USD", "reason": "income"})
	if code := ledgerErrorCode(t, c, "commit_reconciliation", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": single["planId"]}}); code != "validation" {
		t.Fatalf("reconciliation accepted change plan: %s", code)
	}
	ledgerCommit(t, c, uuid.NewString(), single["planId"].(string))
	if code := ledgerErrorCode(t, c, "commit_reconciliation", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}}); code != "stale_preview" {
		t.Fatalf("stale reconciliation code=%s", code)
	}
	expired := reconciliationPreview(t, c, target)
	expiredID := expired["planId"].(string)
	path := filepath.Join(fx.service.dir, "plans", expiredID+".json")
	raw, err := fx.service.readPrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	saved["expiresAt"] = time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
	if err := fx.service.writePrivate(path, saved); err != nil {
		t.Fatal(err)
	}
	if code := ledgerErrorCode(t, c, "commit_reconciliation", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": expiredID}}); code != "stale_preview" {
		t.Fatalf("expired reconciliation code=%s", code)
	}
	if got := ledgerActivityCount(t, fx.app); got != 1 {
		t.Fatalf("rejected commits posted %d activities", got)
	}
}
