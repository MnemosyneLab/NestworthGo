package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func transferDestinationAccount(t *testing.T, fx ledgerFixture) string {
	t.Helper()
	members, err := fx.app.ListMembers(context.Background(), false)
	if err != nil || len(members) != 1 {
		t.Fatalf("members: %v, %v", members, err)
	}
	account, err := fx.app.CreateAccount(context.Background(), application.AccountInput{
		Name: "Second broker", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true,
		IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	return account.Account.ID.String()
}

func importTransferSource(t *testing.T, fx ledgerFixture, c *mcp.ClientSession) string {
	t.Helper()
	plan := ledgerPreview(t, c, map[string]any{
		"kind": "position_import", "accountId": fx.brokerage, "instrumentId": fx.instrument,
		"quantity": "8", "unitCost": "12.5", "currency": "USD",
		"effectiveAt": "2026-09-20T12:00:00Z",
	})
	ledgerCommit(t, c, uuid.NewString(), plan["planId"].(string))
	holdings := batchHoldings(t, c, fx.brokerage)
	if len(holdings) != 1 {
		t.Fatalf("source holdings=%v", holdings)
	}
	return holdings[0].(map[string]any)["id"].(string)
}

func TestMCPPositionTransferCreatesDestinationAtomically(t *testing.T) {
	fx := newLedgerFixture(t)
	destination := transferDestinationAccount(t, fx)
	c := ledgerSession(t, fx)
	source := importTransferSource(t, fx, c)
	costPlan := reconciliationPreview(t, c, map[string]any{"holdingId": source, "totalCost": "240", "currency": "USD"})
	reconciliationCommit(t, c, uuid.NewString(), costPlan["planId"].(string))
	assertReconciliationCost(t, fx, source, "8", "30", "240")
	fx.app.SetClock(func() time.Time { return time.Date(2026, 9, 29, 13, 0, 0, 0, time.UTC) })
	before := ledgerActivityCount(t, fx.app)
	request := map[string]any{
		"kind": "position_transfer", "fromHoldingId": source,
		"toAccountId": destination, "quantity": "3", "effectiveAt": "2026-09-29T12:30:00Z",
	}
	plan := ledgerPreview(t, c, request)
	if len(batchHoldings(t, c, destination)) != 0 || ledgerActivityCount(t, fx.app) != before {
		t.Fatal("transfer preview mutated the ledger")
	}
	preview := plan["preview"].(map[string]any)
	if effects := preview["effects"].([]any); len(effects) != 2 {
		t.Fatalf("transfer preview effects=%v", effects)
	}
	op := uuid.NewString()
	result := ledgerCommit(t, c, op, plan["planId"].(string))
	if result["activity"].(map[string]any)["kind"] != "position_transfer" {
		t.Fatalf("transfer activity=%v", result)
	}
	effects := result["effects"].([]any)
	if len(effects) != 2 {
		t.Fatalf("transfer effects=%v", effects)
	}
	from, to := effects[0].(map[string]any), effects[1].(map[string]any)
	if from["holdingId"] != source || from["classification"] != "internal_transfer" ||
		to["classification"] != "internal_transfer" {
		t.Fatalf("transfer effects=%v", effects)
	}
	sourceHoldings, destinationHoldings := batchHoldings(t, c, fx.brokerage), batchHoldings(t, c, destination)
	if len(sourceHoldings) != 1 || sourceHoldings[0].(map[string]any)["quantity"] != "5" ||
		len(destinationHoldings) != 1 || destinationHoldings[0].(map[string]any)["quantity"] != "3" ||
		to["holdingId"] != destinationHoldings[0].(map[string]any)["id"] {
		t.Fatalf("transfer quantities source=%v destination=%v effects=%v", sourceHoldings, destinationHoldings, effects)
	}
	if batchCash(t, c, fx.brokerage) != "0" || batchCash(t, c, destination) != "0" {
		t.Fatal("transfer changed cash")
	}
	assertReconciliationCost(t, fx, source, "5", "30", "150")
	destinationID := destinationHoldings[0].(map[string]any)["id"].(string)
	assertReconciliationCost(t, fx, destinationID, "3", "30", "90")
	replayed := ledgerCommit(t, c, op, plan["planId"].(string))
	if ledgerActivityID(t, replayed) != ledgerActivityID(t, result) || ledgerActivityCount(t, fx.app) != before+1 {
		t.Fatalf("transfer retry reposted activity: first=%v replay=%v", result, replayed)
	}
	existing := ledgerPreview(t, c, map[string]any{
		"kind": "position_transfer", "fromHoldingId": source,
		"toHoldingId": destinationID,
		"quantity":    "1", "effectiveAt": "2026-09-29T12:45:00Z",
	})
	if existing["preview"] == nil {
		t.Fatalf("existing destination preview=%v", existing)
	}
}

func TestMCPBatchTransferCreatesHoldingForFollowingTrade(t *testing.T) {
	fx := newLedgerFixture(t)
	destination := transferDestinationAccount(t, fx)
	c := ledgerSession(t, fx)
	source := importTransferSource(t, fx, c)
	transfer := map[string]any{
		"kind": "position_transfer", "fromHoldingId": source,
		"toAccountId": destination, "quantity": "4", "effectiveAt": "2026-09-21T12:00:00Z",
	}
	sell := map[string]any{
		"kind": "trade", "side": "sell", "settlementAccountId": destination,
		"instrumentId": fx.instrument, "quantity": "1", "gross": "20", "grossCurrency": "USD",
		"effectiveAt": "2026-09-21T12:00:00Z",
	}
	invalidSell := map[string]any{
		"kind": "trade", "side": "sell", "settlementAccountId": destination,
		"instrumentId": fx.instrument, "quantity": "5", "gross": "100", "grossCurrency": "USD",
		"effectiveAt": "2026-09-21T12:00:00Z",
	}
	if code := ledgerErrorCode(t, c, "preview_batch", map[string]any{"commands": []any{transfer, invalidSell}}); code == "" {
		t.Fatal("batch accepted a sale larger than the transferred quantity")
	}
	if len(batchHoldings(t, c, destination)) != 0 {
		t.Fatal("failed batch preview created destination")
	}
	plan := batchPreview(t, c, transfer, sell)
	if len(batchHoldings(t, c, destination)) != 0 {
		t.Fatal("batch preview created destination")
	}
	changes := batchChanges(t, batchCommit(t, c, uuid.NewString(), plan["planId"].(string)), 2)
	if changes[0].(map[string]any)["activity"].(map[string]any)["kind"] != "position_transfer" ||
		changes[1].(map[string]any)["activity"].(map[string]any)["kind"] != "sell" {
		t.Fatalf("batch activities=%v", changes)
	}
	holdings := batchHoldings(t, c, destination)
	if len(holdings) != 1 || holdings[0].(map[string]any)["quantity"] != "3" {
		t.Fatalf("destination holdings=%v", holdings)
	}
	if got := batchCash(t, c, destination); got != "20" {
		t.Fatalf("sale proceeds=%s", got)
	}
}
