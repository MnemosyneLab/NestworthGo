package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func batchPreview(t *testing.T, c *mcp.ClientSession, commands ...map[string]any) map[string]any {
	t.Helper()
	data := call(t, c, "preview_batch", map[string]any{"commands": commands}, false)["data"].(map[string]any)
	if _, err := uuid.Parse(data["planId"].(string)); err != nil {
		t.Fatalf("invalid batch plan ID: %v", data)
	}
	if _, err := time.Parse(time.RFC3339Nano, data["expiresAt"].(string)); err != nil {
		t.Fatalf("invalid batch plan expiry: %v", data)
	}
	if got := len(data["previews"].([]any)); got != len(commands) {
		t.Fatalf("batch previews=%d want=%d: %v", got, len(commands), data)
	}
	return data
}

func batchCommit(t *testing.T, c *mcp.ClientSession, operationID, planID string) map[string]any {
	t.Helper()
	data := call(t, c, "commit_batch", map[string]any{
		"operationId": operationID, "input": map[string]any{"planId": planID},
	}, false)["data"].(map[string]any)
	if data["status"] != "succeeded" {
		t.Fatalf("batch commit receipt: %v", data)
	}
	return data["result"].(map[string]any)
}

func batchChanges(t *testing.T, result map[string]any, count int) []any {
	t.Helper()
	changes, ok := result["changes"].([]any)
	if !ok || len(changes) != count {
		t.Fatalf("batch changes=%v want %d", result, count)
	}
	for index, raw := range changes {
		activity := raw.(map[string]any)["activity"].(map[string]any)
		if _, err := uuid.Parse(activity["id"].(string)); err != nil {
			t.Fatalf("change %d activity ID: %v", index, activity)
		}
	}
	return changes
}

func batchHoldings(t *testing.T, c *mcp.ClientSession, accountID string) []any {
	t.Helper()
	return call(t, c, "list_holdings", map[string]any{"accountId": accountID}, false)["data"].([]any)
}

func batchCash(t *testing.T, c *mcp.ClientSession, accountID string) string {
	t.Helper()
	snapshot := call(t, c, "get_account_snapshot", IDInput{ID: accountID}, false)["data"].(map[string]any)
	for _, raw := range snapshot["components"].([]any) {
		component := raw.(map[string]any)
		if component["holdingId"] == nil && component["nativeCurrency"] == "USD" {
			return component["nativeAmount"].(string)
		}
	}
	return "0"
}

func TestBatchFundingBuySellAndReplay(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	plan := batchPreview(t, c,
		map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "1000", "currency": "USD", "reason": "contribution", "effectiveAt": "2026-09-20T12:00:00Z"},
		map[string]any{"kind": "trade", "side": "buy", "settlementAccountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "10", "gross": "200", "grossCurrency": "USD", "fee": "5", "feeCurrency": "USD", "effectiveAt": "2026-09-21T12:00:00Z"},
		map[string]any{"kind": "trade", "side": "sell", "settlementAccountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "4", "gross": "120", "grossCurrency": "USD", "fee": "2", "feeCurrency": "USD", "effectiveAt": "2026-09-22T12:00:00Z"},
	)
	if ledgerActivityCount(t, fx.app) != 0 || len(batchHoldings(t, c, fx.brokerage)) != 0 || batchCash(t, c, fx.brokerage) != "0" {
		t.Fatal("batch preview mutated the ledger")
	}
	commands := plan["commands"].([]any)
	if len(commands) != 3 {
		t.Fatalf("resolved commands=%v", commands)
	}
	opID, planID := uuid.NewString(), plan["planId"].(string)
	first := batchChanges(t, batchCommit(t, c, opID, planID), 3)
	if got := ledgerCash(t, c, fx.brokerage); got != "913" {
		t.Fatalf("cash after funding, buy, sell=%s want=913", got)
	}
	holdings := batchHoldings(t, c, fx.brokerage)
	if len(holdings) != 1 || holdings[0].(map[string]any)["quantity"] != "6" {
		t.Fatalf("holdings after batch=%v", holdings)
	}
	if got := ledgerActivityCount(t, fx.app); got != 3 {
		t.Fatalf("activities after batch=%d", got)
	}
	firstID := first[0].(map[string]any)["activity"].(map[string]any)["id"]
	for _, retryID := range []string{opID, uuid.NewString()} {
		retry := batchChanges(t, batchCommit(t, c, retryID, planID), 3)
		if retry[0].(map[string]any)["activity"].(map[string]any)["id"] != firstID {
			t.Fatalf("retry %s changed committed activities: %v", retryID, retry)
		}
	}
	if got := ledgerActivityCount(t, fx.app); got != 3 {
		t.Fatalf("retry duplicated activities: %d", got)
	}
	if code := ledgerErrorCode(t, c, "commit_batch", map[string]any{"operationId": opID, "input": map[string]any{"planId": uuid.NewString()}}); code != "conflict" {
		t.Fatalf("reused operation with different plan code=%s", code)
	}
}

func TestBatchRejectsInvalidEntryWithoutPosting(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	valid := map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "100", "currency": "USD", "reason": "contribution", "effectiveAt": "2026-09-20T12:00:00Z"}
	invalid := map[string]any{"kind": "trade", "side": "buy", "settlementAccountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "5", "gross": "200", "grossCurrency": "USD", "effectiveAt": "2026-09-21T12:00:00Z"}
	if code := ledgerErrorCode(t, c, "preview_batch", map[string]any{"commands": []any{valid, invalid}}); code == "" {
		t.Fatal("invalid second entry had no error code")
	}
	if got := batchCash(t, c, fx.brokerage); got != "0" {
		t.Fatalf("rejected batch changed cash=%s", got)
	}
	if got := batchHoldings(t, c, fx.brokerage); len(got) != 0 {
		t.Fatalf("rejected batch made holdings=%v", got)
	}
	if got := ledgerActivityCount(t, fx.app); got != 0 {
		t.Fatalf("rejected batch posted %d activities", got)
	}
}

func TestBatchRejectsStaleExpiredAndWrongCommitTool(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	command := map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "50", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"}
	batch := batchPreview(t, c, command)
	if code := ledgerErrorCode(t, c, "commit_change", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": batch["planId"]}}); code == "" {
		t.Fatal("single commit accepted a batch plan")
	}
	single := ledgerPreview(t, c, command)
	if code := ledgerErrorCode(t, c, "commit_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": single["planId"]}}); code == "" {
		t.Fatal("batch commit accepted a single plan")
	}
	ledgerCommit(t, c, uuid.NewString(), single["planId"].(string))
	if code := ledgerErrorCode(t, c, "commit_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": batch["planId"]}}); code != "stale_preview" {
		t.Fatalf("stale batch code=%s", code)
	}
	expired := batchPreview(t, c, command)
	planID := expired["planId"].(string)
	path := filepath.Join(fx.service.dir, "plans", planID+".json")
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
	if code := ledgerErrorCode(t, c, "commit_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}}); code != "stale_preview" {
		t.Fatalf("expired batch code=%s", code)
	}
	if got := ledgerActivityCount(t, fx.app); got != 1 {
		t.Fatalf("rejected commits posted %d activities", got)
	}
}

func TestBatchInputBoundsChronologyAndKind(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	base := map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "1", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"}
	cases := []struct {
		name     string
		commands []any
	}{
		{"empty", []any{}},
		{"out of order", []any{base, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "1", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-19T12:00:00Z"}}},
		{"unknown kind", []any{map[string]any{"kind": "balance_reconciliation", "accountId": fx.checking, "amount": "1", "currency": "USD"}}},
	}
	oversized := make([]any, 101)
	for i := range oversized {
		oversized[i] = base
	}
	cases = append(cases, struct {
		name     string
		commands []any
	}{"over 100", oversized})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := ledgerErrorCode(t, c, "preview_batch", map[string]any{"commands": tc.commands}); code == "" {
				t.Fatal("invalid batch had no error code")
			}
		})
	}
	if got := ledgerActivityCount(t, fx.app); got != 0 {
		t.Fatalf("invalid inputs posted %d activities", got)
	}
}

func TestBatchOmittedTimeIsFrozenForEveryCommand(t *testing.T) {
	fx := newLedgerFixtureWithClock(t, func() time.Time { return time.Now().UTC().Add(time.Minute) })
	c := ledgerSession(t, fx)
	plan := batchPreview(t, c,
		map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "3", "currency": "USD", "reason": "income"},
		map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "4", "currency": "USD", "reason": "income"},
	)
	commands := plan["commands"].([]any)
	first := commands[0].(map[string]any)["effectiveAt"].(string)
	second := commands[1].(map[string]any)["effectiveAt"].(string)
	if first == "" || first != second {
		t.Fatalf("omitted times were not frozen together: %v", commands)
	}
	batchChanges(t, batchCommit(t, c, uuid.NewString(), plan["planId"].(string)), 2)
	if got := ledgerCash(t, c, fx.checking); got != "7" {
		t.Fatalf("same-time changes cash=%s", got)
	}
}

func TestPositionImportPreviewCommitAndDuplicate(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	request := map[string]any{"kind": "position_import", "accountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "8", "unitCost": "12.5", "currency": "USD", "effectiveAt": "2026-09-20T12:00:00Z"}
	plan := ledgerPreview(t, c, request)
	if got := batchHoldings(t, c, fx.brokerage); len(got) != 0 {
		t.Fatalf("import preview made holding: %v", got)
	}
	if got := batchCash(t, c, fx.brokerage); got != "0" {
		t.Fatalf("import preview changed cash=%s", got)
	}
	if got := ledgerActivityCount(t, fx.app); got != 0 {
		t.Fatalf("import preview posted %d activities", got)
	}
	result := ledgerCommit(t, c, uuid.NewString(), plan["planId"].(string))
	activity := result["activity"].(map[string]any)
	if activity["kind"] != "position_transfer" || activity["reason"] != "reconciliation" {
		t.Fatalf("import activity=%v", activity)
	}
	holdings := batchHoldings(t, c, fx.brokerage)
	if len(holdings) != 1 || holdings[0].(map[string]any)["quantity"] != "8" {
		t.Fatalf("import holdings=%v", holdings)
	}
	if got := batchCash(t, c, fx.brokerage); got != "0" {
		t.Fatalf("import charged cash=%s", got)
	}
	effects := result["effects"].([]any)
	if len(effects) != 1 || effects[0].(map[string]any)["costUnitPrice"] != "12.5" {
		t.Fatalf("import cost basis effects=%v", effects)
	}
	if code := ledgerErrorCode(t, c, "preview_change", request); code == "" {
		t.Fatal("duplicate active account/instrument import succeeded")
	}
	if got := ledgerActivityCount(t, fx.app); got != 1 {
		t.Fatalf("duplicate import posted %d activities", got)
	}
}

func TestBatchImportThenSellUsesImportedHolding(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	plan := batchPreview(t, c,
		map[string]any{"kind": "position_import", "accountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "10", "unitCost": "15", "currency": "USD", "effectiveAt": "2026-09-20T12:00:00Z"},
		map[string]any{"kind": "trade", "side": "sell", "settlementAccountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "4", "gross": "100", "grossCurrency": "USD", "fee": "1", "feeCurrency": "USD", "effectiveAt": "2026-09-21T12:00:00Z"},
	)
	if len(batchHoldings(t, c, fx.brokerage)) != 0 || ledgerActivityCount(t, fx.app) != 0 {
		t.Fatal("mixed import batch preview mutated ledger")
	}
	changes := batchChanges(t, batchCommit(t, c, uuid.NewString(), plan["planId"].(string)), 2)
	importActivity := changes[0].(map[string]any)["activity"].(map[string]any)
	sellActivity := changes[1].(map[string]any)["activity"].(map[string]any)
	if importActivity["kind"] != "position_transfer" || importActivity["reason"] != "reconciliation" || sellActivity["kind"] != "sell" {
		t.Fatalf("mixed batch activities=%v", changes)
	}
	holdings := batchHoldings(t, c, fx.brokerage)
	if len(holdings) != 1 || holdings[0].(map[string]any)["quantity"] != "6" {
		t.Fatalf("mixed batch holdings=%v", holdings)
	}
	if got := ledgerCash(t, c, fx.brokerage); got != "99" {
		t.Fatalf("import/sell cash=%s want=99", got)
	}
	if got := ledgerActivityCount(t, fx.app); got != 2 {
		t.Fatalf("mixed batch activities=%d", got)
	}
}

func TestBatchPendingReceiptRecoversWithoutReposting(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	plan := batchPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "40", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"})
	planID, opID := plan["planId"].(string), uuid.NewString()
	first := batchChanges(t, batchCommit(t, c, opID, planID), 1)
	firstID := first[0].(map[string]any)["activity"].(map[string]any)["id"]
	path, err := fx.service.operationPath(opID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := fx.service.GetOperation(opID)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Status, receipt.Result = "pending", nil
	if err := fx.service.writePrivate(path, receipt); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(fx.service.dir, "plans", planID+".json")
	raw, err := fx.service.readPrivate(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	saved["expiresAt"] = time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
	if err := fx.service.writePrivate(planPath, saved); err != nil {
		t.Fatal(err)
	}
	recovered := batchChanges(t, batchCommit(t, c, opID, planID), 1)
	if recovered[0].(map[string]any)["activity"].(map[string]any)["id"] != firstID {
		t.Fatalf("recovery returned a different activity: %v", recovered)
	}
	if got := ledgerActivityCount(t, fx.app); got != 1 {
		t.Fatalf("recovery reposted activities=%d", got)
	}
	if got := ledgerCash(t, c, fx.checking); got != "40" {
		t.Fatalf("recovery changed cash=%s", got)
	}
}

func TestBatchToolsRequireLedgerWrite(t *testing.T) {
	fx := newLedgerFixture(t)
	for _, mode := range []string{ReadOnly, DirectoryWrite} {
		if _, err := fx.service.Enable(mode); err != nil {
			t.Fatal(err)
		}
		c := connect(t, fx.service)
		listed, err := c.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range listed.Tools {
			if strings.HasSuffix(tool.Name, "_batch") {
				t.Fatalf("%s exposed %s", mode, tool.Name)
			}
		}
	}
}
