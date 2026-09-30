package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
)

// Exercise the shipped argument examples themselves, so a contract change
// cannot leave a valid-looking but unusable user recipe unnoticed.
func skillExample(t *testing.T, reference, name string, values map[string]string) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", "skills", "nestworth", "references", reference+".md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(content), "<!-- example: "+name+" -->")
	if !ok {
		t.Fatalf("missing skill example %s in %s", name, path)
	}
	_, rest, ok = strings.Cut(rest, "```json\n")
	if !ok {
		t.Fatalf("missing JSON for %s", name)
	}
	raw, _, ok := strings.Cut(rest, "\n```")
	if !ok {
		t.Fatalf("unterminated JSON for %s", name)
	}
	for key, value := range values {
		encoded, _ := json.Marshal(value)
		raw = strings.ReplaceAll(raw, `"${`+key+`}"`, string(encoded))
	}
	if strings.Contains(raw, "${") {
		t.Fatalf("unresolved placeholder in %s: %s", name, raw)
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
		t.Fatal(err)
	}
	return arguments
}

func skillCommit(t *testing.T, c *mcp.ClientSession, name, planID string) map[string]any {
	t.Helper()
	arguments := skillExample(t, "positions-and-trades", "commit", map[string]string{"operationId": uuid.NewString(), "planId": planID})
	receipt := call(t, c, name, arguments, false)["data"].(map[string]any)
	if receipt["status"] != "succeeded" {
		t.Fatalf("skill commit failed: %+v", receipt)
	}
	return receipt["result"].(map[string]any)
}

func TestSkillAccountTradeImportAndReconciliationExamples(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	call(t, c, "get_context", map[string]any{}, false)
	call(t, c, "get_catalog", map[string]any{}, false)
	members := call(t, c, "list_members", map[string]any{}, false)["data"].([]any)
	created := call(t, c, "create_account", skillExample(t, "accounts-and-instruments", "create-brokerage", map[string]string{
		"operationId": uuid.NewString(), "memberId": members[0].(map[string]any)["id"].(string),
	}), false)["data"].(map[string]any)["result"].(map[string]any)
	accountID := created["account"].(map[string]any)["id"].(string)
	values := map[string]string{"accountId": accountID, "instrumentId": fx.instrument, "effectiveAt": "2026-09-29T12:00:00Z"}
	post := func(reference, name string) map[string]any {
		t.Helper()
		preview := ledgerPreview(t, c, skillExample(t, reference, name, values))
		return skillCommit(t, c, "commit_change", preview["planId"].(string))
	}
	post("cash-and-debt", "income")
	buy := post("positions-and-trades", "buy")
	values["holdingId"] = buy["activity"].(map[string]any)["tradeDetail"].(map[string]any)["holdingId"].(string)
	post("positions-and-trades", "sell")
	if cash, quantity := ledgerCash(t, c, accountID), batchHoldings(t, c, accountID)[0].(map[string]any)["quantity"]; cash != "913" || quantity != "6" {
		t.Fatalf("documented buy/sell misstates cash/quantity: %s %v", cash, quantity)
	}
	post("cash-and-debt", "fx-conversion")
	if usd, eur := ledgerCash(t, c, accountID), ledgerAmount(t, c, accountID, "EUR"); usd != "812" || eur != "90" {
		t.Fatalf("documented FX should include separate fee: USD=%s EUR=%s", usd, eur)
	}
	preview := call(t, c, "preview_reconciliation", skillExample(t, "reconciliation-and-corrections", "cost-target", values), false)["data"].(map[string]any)
	skillCommit(t, c, "commit_reconciliation", preview["planId"].(string))
	assertReconciliationCost(t, fx, values["holdingId"], "6", "50", "300")
	if ledgerCash(t, c, accountID) != "812" {
		t.Fatal("cost-target example moved cash")
	}
	preview = call(t, c, "preview_reconciliation", skillExample(t, "reconciliation-and-corrections", "balance-target", values), false)["data"].(map[string]any)
	skillCommit(t, c, "commit_reconciliation", preview["planId"].(string))
	if ledgerCash(t, c, accountID) != "1250" {
		t.Fatal("balance-target example did not set target")
	}
	values["accountId"] = fx.brokerage
	post("positions-and-trades", "import-position")
	if cash, quantity := batchCash(t, c, fx.brokerage), batchHoldings(t, c, fx.brokerage)[0].(map[string]any)["quantity"]; cash != "0" || quantity != "10" {
		t.Fatalf("position-import example must not deduct cash: %s %v", cash, quantity)
	}
}

func TestSkillCashDebtAndBatchExamples(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	values := map[string]string{"accountId": fx.checking, "effectiveAt": "2026-09-29T12:00:00Z", "debtAccountId": fx.debt}
	income := skillExample(t, "cash-and-debt", "income", values)
	expense := skillExample(t, "cash-and-debt", "expense", values)
	plan := batchPreview(t, c, income, expense)
	opID := uuid.NewString()
	arguments := skillExample(t, "positions-and-trades", "commit", map[string]string{"operationId": opID, "planId": plan["planId"].(string)})
	call(t, c, "commit_batch", arguments, false)
	call(t, c, "commit_batch", arguments, false)
	if ledgerCash(t, c, fx.checking) != "950" || ledgerActivityCount(t, fx.app) != 2 {
		t.Fatal("documented batch/retry duplicated or misclassified income/expense")
	}
	draw := ledgerPreview(t, c, map[string]any{"kind": "debt_draw", "debtAccountId": fx.debt, "cashAccountId": fx.checking, "principal": "300", "principalCurrency": "USD", "effectiveAt": values["effectiveAt"]})
	skillCommit(t, c, "commit_change", draw["planId"].(string))
	repayment := ledgerPreview(t, c, skillExample(t, "cash-and-debt", "debt-payment", values))
	skillCommit(t, c, "commit_change", repayment["planId"].(string))
	if cash, debt := ledgerCash(t, c, fx.checking), ledgerCash(t, c, fx.debt); cash != "1140" || debt != "200" {
		t.Fatalf("documented repayment should separate principal and interest: %s %s", cash, debt)
	}
}

func TestSkillFundQuoteAnalysisAndHealthExamples(t *testing.T) {
	fx := newLedgerFixture(t)
	// Any accidental provider dependency must fail locally rather than use a network.
	fx.app.SetMarketDataRegistry(application.NewMarketDataRegistry())
	t.Cleanup(fx.app.CancelMarketDataSyncAndWait)
	c := ledgerSession(t, fx)
	values := map[string]string{"operationId": uuid.NewString(), "fundCode": "EXAMPLE-A"}
	fund := call(t, c, "create_instrument", skillExample(t, "accounts-and-instruments", "create-fund", values), false)["data"].(map[string]any)["result"].(map[string]any)
	values["instrumentId"], values["operationId"] = fund["id"].(string), uuid.NewString()
	imported := call(t, c, "import_market_data", skillExample(t, "market-data", "fund-nav", values), false)["data"].(map[string]any)["result"].(map[string]any)
	data := call(t, c, "get_market_data", map[string]any{"instrumentId": values["instrumentId"], "range": "2026-09-28:2026-09-28"}, false)["data"].(map[string]any)
	if data["current"].(map[string]any)["unitPrice"] != "1.0197" || len(data["history"].(map[string]any)["observations"].([]any)) != 1 {
		t.Fatal("documented fund NAV date/value not available through MCP")
	}
	values["quoteId"], values["operationId"] = imported["quoteIds"].([]any)[0].(string), uuid.NewString()
	call(t, c, "import_market_data", skillExample(t, "market-data", "withdraw-quote", values), false)
	data = call(t, c, "get_market_data", map[string]any{"instrumentId": values["instrumentId"]}, false)["data"].(map[string]any)
	if data["current"] != nil {
		t.Fatal("documented withdrawal retained a selected NAV")
	}
	values["operationId"] = uuid.NewString()
	call(t, c, "set_fx_source", skillExample(t, "market-data", "fx-source", values), false)
	values["operationId"] = uuid.NewString()
	call(t, c, "import_market_data", skillExample(t, "market-data", "fx-reference", values), false)
	query := skillExample(t, "analysis", "analyze-period", nil)
	report := call(t, c, "analyze_period", query, false)["data"].(map[string]any)
	for _, section := range []string{"income", "expenses", "assetChange", "investmentReturns"} {
		if _, ok := report[section]; !ok {
			t.Fatalf("documented analysis query missing %s: %v", section, report)
		}
	}
	returns := report["investmentReturns"].(map[string]any)
	if returns["available"] != false || returns["rate"] != nil {
		t.Fatalf("incomplete analysis must preserve unavailable/null rate: %v", returns)
	}
	call(t, c, "scan_data_health", map[string]any{}, false)
	call(t, c, "preview_data_repair", map[string]any{}, false)
	// Exercise durable submission and exact job tracking without real providers.
	result := mutate(t, c, "start_data_repair", map[string]any{})
	job := result["job"].(map[string]any)
	call(t, c, "get_data_repair_job", map[string]any{"jobId": job["jobId"]}, false)
	if _, err := c.ListTools(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}
