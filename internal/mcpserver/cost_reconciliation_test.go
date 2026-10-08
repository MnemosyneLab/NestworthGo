package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestCostReconciliationPreservesCashQuantityAndChangesLaterGain(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	imported := ledgerPreview(t, c, map[string]any{"kind": "position_import", "accountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "10", "unitCost": "25", "currency": "USD", "effectiveAt": "2026-09-20T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), imported["planId"].(string))
	holdingID := batchHoldings(t, c, fx.brokerage)[0].(map[string]any)["id"].(string)
	plan := reconciliationPreview(t, c, map[string]any{"holdingId": holdingID, "totalCost": "300", "currency": "USD"})
	if ledgerActivityCount(t, fx.app) != 1 {
		t.Fatal("cost preview wrote an activity")
	}
	assertReconciliationCost(t, fx, holdingID, "10", "25", "250")
	previews := plan["previews"].([]any)
	if len(previews) != 1 {
		t.Fatalf("pure cost previews=%v", previews)
	}
	preview := previews[0].(map[string]any)
	effects := preview["effects"].([]any)
	if len(effects) != 1 || effects[0].(map[string]any)["target"] != "holding_cost" || effects[0].(map[string]any)["costUnitPrice"] != "30" {
		t.Fatalf("cost effects=%v", effects)
	}
	planID, opID := plan["planId"].(string), uuid.NewString()
	result := batchChanges(t, reconciliationCommit(t, c, opID, planID), 1)
	firstID := result[0].(map[string]any)["activity"].(map[string]any)["id"]
	for _, id := range []string{opID, uuid.NewString()} {
		retry := batchChanges(t, reconciliationCommit(t, c, id, planID), 1)
		if retry[0].(map[string]any)["activity"].(map[string]any)["id"] != firstID {
			t.Fatal("retry duplicated cost correction")
		}
	}
	assertReconciliationCost(t, fx, holdingID, "10", "30", "300")
	if batchCash(t, c, fx.brokerage) != "0" || ledgerActivityCount(t, fx.app) != 2 {
		t.Fatal("cost correction changed cash or duplicated events")
	}
	for _, input := range []map[string]any{
		{"action": "undo", "activityId": firstID},
		{"action": "fix", "activityId": firstID, "replacement": map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "1", "currency": "USD", "reason": "income"}},
	} {
		if code := ledgerErrorCode(t, c, "preview_correction", input); code != "cannot_fix_change" {
			t.Fatalf("cost activity should require a new target: %s", code)
		}
	}

	fx.app.SetClock(func() time.Time { return time.Date(2026, 9, 29, 13, 0, 0, 0, time.UTC) })
	sale := ledgerPreview(t, c, map[string]any{"kind": "trade", "side": "sell", "settlementAccountId": fx.brokerage, "holdingId": holdingID, "instrumentId": fx.instrument, "quantity": "2", "gross": "80", "grossCurrency": "USD", "effectiveAt": "2026-09-29T12:30:00Z"})
	ledgerCommit(t, c, uuid.NewString(), sale["planId"].(string))
	assertReconciliationCost(t, fx, holdingID, "8", "30", "240")
	id, _ := domain.ParseHoldingID(holdingID)
	gain, err := fx.app.HoldingGain(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if gain.RealizedGain.Amount != "20" {
		t.Fatalf("realized gain after corrected cost=%+v", gain)
	}
}

func TestCostReconciliationCombinedTargetAndCurrencyValidation(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	imported := ledgerPreview(t, c, map[string]any{"kind": "position_import", "accountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "10", "unitCost": "25", "currency": "USD", "effectiveAt": "2026-09-20T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), imported["planId"].(string))
	holdingID := batchHoldings(t, c, fx.brokerage)[0].(map[string]any)["id"].(string)
	for _, target := range []map[string]any{
		{"holdingId": holdingID, "totalCost": "300"},
		{"holdingId": holdingID, "totalCost": "300", "currency": "EUR"},
		{"holdingId": holdingID, "totalCost": "300", "currency": "USD", "unitCost": "25"},
		{"holdingId": holdingID, "targetQuantity": "0", "totalCost": "300", "currency": "USD"},
	} {
		if code := ledgerErrorCode(t, c, "preview_reconciliation", map[string]any{"targets": []any{target}}); code == "" {
			t.Fatalf("accepted invalid target %v", target)
		}
	}
	plan := reconciliationPreview(t, c, map[string]any{"holdingId": holdingID, "targetQuantity": "12", "totalCost": "360", "currency": "USD"})
	reconciliationCommit(t, c, uuid.NewString(), plan["planId"].(string))
	assertReconciliationCost(t, fx, holdingID, "12", "30", "360")
	if batchCash(t, c, fx.brokerage) != "0" {
		t.Fatal("target position reconciliation moved cash")
	}
	// Currency-rounded totals must remain usable when division repeats.
	plan = reconciliationPreview(t, c, map[string]any{"holdingId": holdingID, "targetQuantity": "3", "totalCost": "100", "currency": "USD"})
	reconciliationCommit(t, c, uuid.NewString(), plan["planId"].(string))
	assertReconciliationCost(t, fx, holdingID, "3", "33.3333", "100")
}
