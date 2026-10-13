package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
)

func TestCashTransferLiabilityRejectedThroughMCP(t *testing.T) {
	fx := newLedgerFixture(t)
	ctx := context.Background()
	members, err := fx.app.ListMembers(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := account.NewService(fx.app).CreateAccount(ctx, account.CreateAccountRequest{Name: "Other debt", AccountType: "loan", BalanceSheetRole: "liability", TrackingMode: "balance", DefaultCurrency: "USD", OwnerIDs: []string{members[0].ID.String()}, InitialAmount: "500", IncludeInNetWorth: true})
	if err != nil {
		t.Fatal(err)
	}
	c := ledgerSession(t, fx)
	funding := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "1000", "currency": "USD", "reason": "contribution", "effectiveAt": "2026-09-20T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), funding["planId"].(string))
	valid := map[string]any{"kind": "cash_transfer", "fromAccountId": fx.checking, "toAccountId": fx.savings, "sent": "100", "sentCurrency": "USD", "received": "100", "receivedCurrency": "USD", "effectiveAt": "2026-09-21T12:00:00Z"}
	plan := ledgerPreview(t, c, valid)
	original := ledgerActivityID(t, ledgerCommit(t, c, uuid.NewString(), plan["planId"].(string)))
	before := ledgerActivityCount(t, fx.app)
	plans, err := os.ReadDir(filepath.Join(fx.service.dir, "plans"))
	if err != nil {
		t.Fatal(err)
	}
	for _, direction := range []struct{ name, from, to string }{
		{"asset_to_liability", fx.checking, fx.debt},
		{"liability_to_asset", fx.debt, fx.savings},
		{"liability_to_liability", fx.debt, other.Account.ID},
	} {
		t.Run(direction.name, func(t *testing.T) {
			invalid := map[string]any{"kind": "cash_transfer", "fromAccountId": direction.from, "toAccountId": direction.to, "sent": "100", "sentCurrency": "USD", "received": "100", "receivedCurrency": "USD", "effectiveAt": "2026-09-22T12:00:00Z"}
			replacement := map[string]any{"kind": "cash_transfer", "fromAccountId": direction.from, "toAccountId": direction.to, "sent": "100", "sentCurrency": "USD", "received": "100", "receivedCurrency": "USD"}
			for _, request := range []struct {
				name      string
				arguments any
			}{
				{"preview_change", invalid},
				{"preview_batch", map[string]any{"commands": []any{valid, invalid}}},
				{"preview_correction", map[string]any{"action": "fix", "activityId": original, "replacement": replacement}},
			} {
				t.Run(request.name, func(t *testing.T) {
					if code := ledgerErrorCode(t, c, request.name, request.arguments); code != "invalid_change" {
						t.Fatalf("error code = %s", code)
					}
					if after := ledgerActivityCount(t, fx.app); after != before {
						t.Fatalf("rejected preview wrote activities: %d -> %d", before, after)
					}
					if ledgerCash(t, c, fx.checking) != "900" || ledgerCash(t, c, fx.savings) != "100" || ledgerCash(t, c, fx.debt) != "0" || ledgerCash(t, c, other.Account.ID) != "500" {
						t.Fatal("rejected preview changed account balances")
					}
					currentPlans, err := os.ReadDir(filepath.Join(fx.service.dir, "plans"))
					if err != nil || len(currentPlans) != len(plans) {
						t.Fatalf("rejection created commit plan: %d -> %d, %v", len(plans), len(currentPlans), err)
					}
				})
			}
		})
	}
}
