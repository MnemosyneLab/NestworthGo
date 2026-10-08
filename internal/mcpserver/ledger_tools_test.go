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
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

type ledgerFixture struct {
	service    *Service
	app        *application.Service
	checking   string
	savings    string
	brokerage  string
	debt       string
	instrument string
}

func newLedgerFixture(t *testing.T) ledgerFixture {
	return newLedgerFixtureWithClock(t, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
}

func newLedgerFixtureWithClock(t *testing.T, clock func() time.Time) ledgerFixture {
	t.Helper()
	app := wailstest.NewService(t)
	return newLedgerFixtureWithApp(t, app, clock)
}

func newLedgerFixtureWithApp(t *testing.T, app *application.Service, clock func() time.Time) ledgerFixture {
	t.Helper()
	ctx := context.Background()
	app.SetClock(clock)
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "MCP ledger", BaseCurrency: "USD", MemberNames: []string{"Alice"},
		Timezone: "UTC", HistoryStartDate: "2026-09-01",
	}); err != nil {
		t.Fatal(err)
	}
	members, err := app.ListMembers(ctx, false)
	if err != nil || len(members) != 1 {
		t.Fatalf("members: %v, %v", members, err)
	}
	createAccount := func(name, kind, role, mode string, portfolio bool, initial string) string {
		t.Helper()
		created, err := account.NewService(app).CreateAccount(ctx, account.CreateAccountRequest{
			Name: name, AccountType: kind, BalanceSheetRole: role, TrackingMode: mode,
			DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: portfolio,
			OwnerIDs: []string{members[0].ID.String()}, InitialAmount: initial,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return created.Account.ID
	}
	checking := createAccount("Checking", "bank_account", "asset", "balance", false, "0")
	savings := createAccount("Savings", "bank_account", "asset", "balance", false, "0")
	brokerage := createAccount("Brokerage", "brokerage", "asset", "holdings", true, "")
	debt := createAccount("Credit card", "credit_card", "liability", "balance", false, "0")
	asset, err := instrument.NewService(app).CreateInstrument(ctx, instrument.InstrumentRequest{
		Name: "Example stock", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := New(app, t.TempDir(), nil)
	t.Cleanup(s.Close)
	return ledgerFixture{service: s, app: app, checking: checking, savings: savings, brokerage: brokerage, debt: debt, instrument: asset.ID}
}

func ledgerSession(t *testing.T, fx ledgerFixture) *mcp.ClientSession {
	t.Helper()
	if _, err := fx.service.Enable(LedgerWrite); err != nil {
		t.Fatal(err)
	}
	return connect(t, fx.service)
}

func ledgerPreview(t *testing.T, c *mcp.ClientSession, request map[string]any) map[string]any {
	t.Helper()
	data := call(t, c, "preview_change", request, false)["data"].(map[string]any)
	if _, err := uuid.Parse(data["planId"].(string)); err != nil {
		t.Fatalf("invalid plan ID: %v", data)
	}
	if _, err := time.Parse(time.RFC3339Nano, data["expiresAt"].(string)); err != nil {
		t.Fatalf("invalid plan expiry: %v", data)
	}
	return data
}

func ledgerCommit(t *testing.T, c *mcp.ClientSession, operationID, planID string) map[string]any {
	t.Helper()
	data := call(t, c, "commit_change", map[string]any{
		"operationId": operationID, "input": map[string]any{"planId": planID},
	}, false)["data"].(map[string]any)
	if data["status"] != "succeeded" {
		t.Fatalf("commit receipt: %v", data)
	}
	return data["result"].(map[string]any)
}

func ledgerActivityID(t *testing.T, result map[string]any) string {
	t.Helper()
	activity := result["activity"].(map[string]any)
	id, ok := activity["id"].(string)
	if !ok || id == "" {
		t.Fatalf("missing activity ID: %v", result)
	}
	return id
}

func ledgerActivityCount(t *testing.T, app *application.Service) int {
	t.Helper()
	values, err := app.ListActivities(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	return len(values)
}

func ledgerErrorCode(t *testing.T, c *mcp.ClientSession, name string, arguments any) string {
	t.Helper()
	result, err := c.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || len(result.Content) == 0 {
		t.Fatalf("%s accepted invalid input: %+v", name, result)
	}
	encoded, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatal(err)
	}
	var blocks []map[string]any
	if err = json.Unmarshal(encoded, &blocks); err != nil {
		t.Fatal(err)
	}
	var failure map[string]any
	if err = json.Unmarshal([]byte(blocks[0]["text"].(string)), &failure); err != nil {
		t.Fatalf("%s non-JSON tool failure: %s", name, encoded)
	}
	return failure["code"].(string)
}

func ledgerAmount(t *testing.T, c *mcp.ClientSession, accountID, currency string) string {
	t.Helper()
	snapshot := call(t, c, "get_account_snapshot", IDInput{ID: accountID}, false)["data"].(map[string]any)
	for _, raw := range snapshot["components"].([]any) {
		component := raw.(map[string]any)
		if component["holdingId"] == nil && component["nativeCurrency"] == currency {
			return component["nativeAmount"].(string)
		}
	}
	t.Fatalf("%s account component missing: %v", currency, snapshot)
	return ""
}

func ledgerCash(t *testing.T, c *mcp.ClientSession, accountID string) string {
	t.Helper()
	return ledgerAmount(t, c, accountID, "USD")
}

func TestLedgerToolsRequireOptIn(t *testing.T) {
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
		if found["preview_change"] != (mode == LedgerWrite) || found["commit_change"] != (mode == LedgerWrite) {
			t.Fatalf("mode=%s ledger tools=%v", mode, found)
		}
		if found["create_account"] != (mode != ReadOnly) {
			t.Fatalf("mode=%s directory permission=%v", mode, found["create_account"])
		}
		if mode != LedgerWrite {
			for _, name := range []string{"preview_change", "commit_change"} {
				_, err := c.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
				if err == nil {
					t.Fatalf("%s accepted %s", mode, name)
				}
			}
		}
	}
	fx.service.Close()
	resumed := New(fx.app, fx.service.dir, nil)
	t.Cleanup(resumed.Close)
	if err := resumed.Resume(); err != nil {
		t.Fatal(err)
	}
	if got := resumed.Status(); !got.Running || got.Mode != LedgerWrite {
		t.Fatalf("ledger opt-in did not persist: %+v", got)
	}
}

func TestLedgerCashIncomeExpenseAndIdempotentCommit(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	add := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "100", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"})
	if got := ledgerActivityCount(t, fx.app); got != 0 {
		t.Fatalf("preview posted %d activities", got)
	}
	if got := ledgerCash(t, c, fx.checking); got != "0" {
		t.Fatalf("preview changed cash to %s", got)
	}
	planID := add["planId"].(string)
	opID := uuid.NewString()
	first := ledgerCommit(t, c, opID, planID)
	firstID := ledgerActivityID(t, first)
	if got := ledgerCash(t, c, fx.checking); got != "100" {
		t.Fatalf("income cash=%s", got)
	}
	if got := ledgerActivityID(t, ledgerCommit(t, c, opID, planID)); got != firstID {
		t.Fatalf("same operation returned activity %s, want %s", got, firstID)
	}
	if got := ledgerActivityID(t, ledgerCommit(t, c, uuid.NewString(), planID)); got != firstID {
		t.Fatalf("same plan with new operation returned activity %s, want %s", got, firstID)
	}
	remove := ledgerPreview(t, c, map[string]any{"kind": "money_removed", "accountId": fx.checking, "amount": "25", "currency": "USD", "reason": "expense", "effectiveAt": "2026-09-21T12:00:00Z"})
	if got := ledgerActivityID(t, ledgerCommit(t, c, uuid.NewString(), remove["planId"].(string))); got == firstID {
		t.Fatal("expense reused income activity")
	}
	if got := ledgerCash(t, c, fx.checking); got != "75" {
		t.Fatalf("income minus expense cash=%s", got)
	}
	if got := ledgerActivityCount(t, fx.app); got != 2 {
		t.Fatalf("retries duplicated activities: %d", got)
	}
	if code := ledgerErrorCode(t, c, "commit_change", map[string]any{"operationId": opID, "input": map[string]any{"planId": remove["planId"]}}); code != "conflict" {
		t.Fatalf("changed operation arguments code=%s", code)
	}
	if got := ledgerActivityCount(t, fx.app); got != 2 {
		t.Fatalf("changed operation arguments posted activity: %d", got)
	}
}

func TestLedgerTradeFirstBuyAndSell(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	deposit := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "1000", "currency": "USD", "reason": "contribution", "effectiveAt": "2026-09-29T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), deposit["planId"].(string))
	buy := ledgerPreview(t, c, map[string]any{"kind": "trade", "side": "buy", "settlementAccountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "10", "gross": "200", "grossCurrency": "USD", "fee": "5", "feeCurrency": "USD", "effectiveAt": "2026-09-29T12:00:00Z"})
	trade := ledgerCommit(t, c, uuid.NewString(), buy["planId"].(string))
	detail := trade["activity"].(map[string]any)["tradeDetail"].(map[string]any)
	holdingID := detail["holdingId"].(string)
	if holdingID == "" || detail["side"] != "buy" || ledgerCash(t, c, fx.brokerage) != "795" {
		t.Fatalf("first buy=%v cash=%s", detail, ledgerCash(t, c, fx.brokerage))
	}
	holdings := call(t, c, "list_holdings", map[string]any{"accountId": fx.brokerage}, false)["data"].([]any)
	if len(holdings) != 1 || holdings[0].(map[string]any)["id"] != holdingID || holdings[0].(map[string]any)["quantity"] != "10" {
		t.Fatalf("first buy holdings=%v", holdings)
	}
	sell := ledgerPreview(t, c, map[string]any{"kind": "trade", "side": "sell", "settlementAccountId": fx.brokerage, "holdingId": holdingID, "instrumentId": fx.instrument, "quantity": "4", "gross": "120", "grossCurrency": "USD", "fee": "2", "feeCurrency": "USD", "effectiveAt": "2026-09-29T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), sell["planId"].(string))
	holdings = call(t, c, "list_holdings", map[string]any{"accountId": fx.brokerage}, false)["data"].([]any)
	if len(holdings) != 1 || holdings[0].(map[string]any)["quantity"] != "6" || ledgerCash(t, c, fx.brokerage) != "913" {
		t.Fatalf("sell holdings=%v cash=%s", holdings, ledgerCash(t, c, fx.brokerage))
	}
	dividend := ledgerPreview(t, c, map[string]any{"kind": "cash_dividend", "holdingId": holdingID, "amount": "15", "currency": "USD", "effectiveAt": "2026-09-29T12:00:00Z"})
	dividendResult := ledgerCommit(t, c, uuid.NewString(), dividend["planId"].(string))
	if dividendResult["activity"].(map[string]any)["dividendDetail"] == nil || ledgerCash(t, c, fx.brokerage) != "928" {
		t.Fatalf("dividend did not credit brokerage cash: result=%v cash=%s", dividendResult, ledgerCash(t, c, fx.brokerage))
	}
}

func TestLedgerTransferFXAndDebtBalances(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	post := func(request map[string]any) map[string]any {
		t.Helper()
		plan := ledgerPreview(t, c, request)
		return ledgerCommit(t, c, uuid.NewString(), plan["planId"].(string))
	}
	at := "2026-09-29T12:00:00Z"
	post(map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "500", "currency": "USD", "reason": "contribution", "effectiveAt": at})
	post(map[string]any{"kind": "cash_transfer", "fromAccountId": fx.checking, "toAccountId": fx.savings, "sent": "120", "sentCurrency": "USD", "received": "120", "receivedCurrency": "USD", "fee": "2", "feeCurrency": "USD", "effectiveAt": at})
	if got, want := ledgerCash(t, c, fx.checking), "378"; got != want {
		t.Fatalf("transfer source cash=%s want=%s", got, want)
	}
	if got, want := ledgerCash(t, c, fx.savings), "120"; got != want {
		t.Fatalf("transfer destination cash=%s want=%s", got, want)
	}
	post(map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "1000", "currency": "USD", "reason": "contribution", "effectiveAt": at})
	post(map[string]any{"kind": "fx_conversion", "accountId": fx.brokerage, "sold": "100", "soldCurrency": "USD", "bought": "90", "boughtCurrency": "EUR", "fee": "1", "feeCurrency": "USD", "effectiveAt": at})
	if usd, eur := ledgerCash(t, c, fx.brokerage), ledgerAmount(t, c, fx.brokerage, "EUR"); usd != "899" || eur != "90" {
		t.Fatalf("FX balances USD=%s EUR=%s", usd, eur)
	}
	post(map[string]any{"kind": "debt_draw", "debtAccountId": fx.debt, "cashAccountId": fx.checking, "principal": "300", "principalCurrency": "USD", "effectiveAt": at})
	if cash, debt := ledgerCash(t, c, fx.checking), ledgerCash(t, c, fx.debt); cash != "678" || debt != "300" {
		t.Fatalf("debt draw cash=%s debt=%s", cash, debt)
	}
	post(map[string]any{"kind": "debt_payment", "debtAccountId": fx.debt, "cashAccountId": fx.checking, "principal": "100", "principalCurrency": "USD", "interestOrFee": "10", "interestOrFeeCurrency": "USD", "effectiveAt": at})
	if cash, debt := ledgerCash(t, c, fx.checking), ledgerCash(t, c, fx.debt); cash != "568" || debt != "200" {
		t.Fatalf("debt payment cash=%s debt=%s", cash, debt)
	}
	call(t, c, "preview_change", map[string]any{"kind": "money_removed", "accountId": fx.checking, "amount": "9999", "currency": "USD", "reason": "expense", "effectiveAt": at}, true)
	call(t, c, "preview_change", map[string]any{"kind": "fx_conversion", "accountId": fx.brokerage, "sold": "1", "soldCurrency": "USD", "bought": "1", "boughtCurrency": "EUR", "fee": "1", "effectiveAt": at}, true)
	if got := ledgerActivityCount(t, fx.app); got != 6 {
		t.Fatalf("invalid previews posted activities: %d", got)
	}
}

func TestLedgerPlanRejectsStaleAndExpiredCommit(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	request := map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "50", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"}
	stale := ledgerPreview(t, c, request)
	_, err := history.NewService(fx.app).RecordChange(context.Background(), history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: fx.checking, Amount: "10", Currency: "USD", Reason: "income", EffectiveAt: "2026-09-19T12:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if code := ledgerErrorCode(t, c, "commit_change", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": stale["planId"]}}); code != "stale_preview" {
		t.Fatalf("stale plan code=%s", code)
	}
	if got := ledgerCash(t, c, fx.checking); got != "10" {
		t.Fatalf("stale plan changed cash to %s", got)
	}
	expired := ledgerPreview(t, c, request)
	planID := expired["planId"].(string)
	path := filepath.Join(fx.service.dir, "plans", planID+".json")
	raw, err := fx.service.readPrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan changePlan
	if err = json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	plan.ExpiresAt = time.Now().Add(-time.Second)
	if err = fx.service.writePrivate(path, plan); err != nil {
		t.Fatal(err)
	}
	if code := ledgerErrorCode(t, c, "commit_change", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"planId": planID}}); code != "stale_preview" {
		t.Fatalf("expired plan code=%s", code)
	}
	if got := ledgerActivityCount(t, fx.app); got != 1 {
		t.Fatalf("expired plan posted %d activities", got)
	}
}

func TestLedgerUnknownReceiptRecoversAfterExpiryAndRestart(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	plan := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "40", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"})
	planID := plan["planId"].(string)
	opID := uuid.NewString()
	activityID := ledgerActivityID(t, ledgerCommit(t, c, opID, planID))
	path, err := fx.service.operationPath(opID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := fx.service.GetOperation(opID)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Status, receipt.Result = "pending", nil
	if err = fx.service.writePrivate(path, receipt); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(fx.service.dir, "plans", planID+".json")
	raw, err := fx.service.readPrivate(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved changePlan
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	saved.ExpiresAt = time.Now().Add(-time.Second)
	if err = fx.service.writePrivate(planPath, saved); err != nil {
		t.Fatal(err)
	}
	fx.service.Close()
	resumed := New(fx.app, fx.service.dir, nil)
	t.Cleanup(resumed.Close)
	if err = resumed.Resume(); err != nil {
		t.Fatal(err)
	}
	c2 := connect(t, resumed)
	if got := ledgerActivityID(t, ledgerCommit(t, c2, opID, planID)); got != activityID {
		t.Fatalf("recovered activity=%s want=%s", got, activityID)
	}
	if got := ledgerActivityID(t, ledgerCommit(t, c2, uuid.NewString(), planID)); got != activityID {
		t.Fatalf("expired committed plan replay=%s want=%s", got, activityID)
	}
	if got := ledgerActivityCount(t, fx.app); got != 1 || ledgerCash(t, c2, fx.checking) != "40" {
		t.Fatalf("recovery duplicated write: activities=%d cash=%s", got, ledgerCash(t, c2, fx.checking))
	}
}

func TestLedgerHistoricalReplayAndUnionValidation(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	for _, request := range []map[string]any{
		{"kind": "money_added", "accountId": fx.checking, "amount": "100", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z"},
		{"kind": "money_removed", "accountId": fx.checking, "amount": "20", "currency": "USD", "reason": "expense", "effectiveAt": "2026-09-22T12:00:00Z"},
		{"kind": "money_added", "accountId": fx.checking, "amount": "50", "currency": "USD", "reason": "income", "effectiveLocalDate": "2026-09-21", "effectiveLocalTime": "12:00"},
	} {
		plan := ledgerPreview(t, c, request)
		ledgerCommit(t, c, uuid.NewString(), plan["planId"].(string))
	}
	if got := ledgerCash(t, c, fx.checking); got != "130" {
		t.Fatalf("historical replay cash=%s want=130", got)
	}
	for _, invalid := range []map[string]any{
		{"kind": "money_added", "accountId": fx.checking, "amount": "1", "currency": "USD", "reason": "income", "gross": "2"},
		{"kind": "money_added", "accountId": fx.checking, "amount": "1", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T12:00:00Z", "effectiveLocalDate": "2026-09-20", "effectiveLocalTime": "12:00"},
		{"kind": "position_adjustment", "holdingId": uuid.NewString(), "quantity": "1"},
	} {
		call(t, c, "preview_change", invalid, true)
	}
	if got := ledgerActivityCount(t, fx.app); got != 3 {
		t.Fatalf("invalid preview posted %d activities", got)
	}
}

func TestLedgerHistoricalFirstBuyReplaysLaterCashAndHolding(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	funding := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "1000", "currency": "USD", "reason": "contribution", "effectiveAt": "2026-09-20T12:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), funding["planId"].(string))
	buy := ledgerPreview(t, c, map[string]any{"kind": "trade", "side": "buy", "settlementAccountId": fx.brokerage, "instrumentId": fx.instrument, "quantity": "10", "gross": "200", "grossCurrency": "USD", "fee": "5", "feeCurrency": "USD", "effectiveAt": "2026-09-21T12:00:00Z"})
	result := ledgerCommit(t, c, uuid.NewString(), buy["planId"].(string))
	holdingID := result["activity"].(map[string]any)["tradeDetail"].(map[string]any)["holdingId"]
	holdings := call(t, c, "list_holdings", map[string]any{"accountId": fx.brokerage}, false)["data"].([]any)
	if len(holdings) != 1 || holdings[0].(map[string]any)["id"] != holdingID || holdings[0].(map[string]any)["quantity"] != "10" || ledgerCash(t, c, fx.brokerage) != "795" {
		t.Fatalf("historical first buy holdings=%v cash=%s", holdings, ledgerCash(t, c, fx.brokerage))
	}
	backdated := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.brokerage, "amount": "50", "currency": "USD", "reason": "income", "effectiveAt": "2026-09-20T18:00:00Z"})
	ledgerCommit(t, c, uuid.NewString(), backdated["planId"].(string))
	if cash := ledgerCash(t, c, fx.brokerage); cash != "845" {
		t.Fatalf("replayed cash=%s want=845", cash)
	}
}

func TestLedgerOmittedTimeIsFrozenAtPreview(t *testing.T) {
	fx := newLedgerFixtureWithClock(t, time.Now)
	c := ledgerSession(t, fx)
	before := time.Now().Add(-time.Second)
	plan := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": fx.checking, "amount": "10", "currency": "USD", "reason": "income"})
	command := plan["command"].(map[string]any)
	frozen, err := time.Parse(time.RFC3339Nano, command["effectiveAt"].(string))
	if err != nil || frozen.Before(before) || frozen.After(time.Now().Add(time.Second)) {
		t.Fatalf("frozen preview time=%s err=%v", command["effectiveAt"], err)
	}
	result := ledgerCommit(t, c, uuid.NewString(), plan["planId"].(string))
	activity := result["activity"].(map[string]any)
	committedAt, err := time.Parse(time.RFC3339Nano, activity["effectiveAt"].(string))
	if err != nil || frozen.Sub(committedAt) < 0 || frozen.Sub(committedAt) >= time.Millisecond || ledgerCash(t, c, fx.checking) != "10" {
		t.Fatalf("commit changed frozen time or amount: activity=%v cash=%s", activity, ledgerCash(t, c, fx.checking))
	}
}
