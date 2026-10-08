package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// Follow each section's own descriptor using the portable examples, never
// another section's reset cursor or a different package's accumulated rows.
func skillContextDetails(t *testing.T, c *mcp.ClientSession, initial FinancialContextResponse) application.FinancialContextContent {
	t.Helper()
	content := initial.Content
	for _, section := range []string{"positions", "gaps", "evidence"} {
		info := map[string]FinancialContextPageInfo{"positions": initial.PositionsPage, "gaps": initial.GapsPage, "evidence": initial.EvidencePage}[section]
		count := info.Returned
		for info.HasMore {
			previous := info.NextCursor
			page := decodeContext(t, call(t, c, "get_financial_context_page", skillExample(t, "analysis", "financial-context-"+section+"-page", map[string]string{"contextId": initial.ContextID, "cursor": previous}), false))
			if page.ContextID != initial.ContextID || page.ContentHash != initial.ContentHash || page.CapturedAt != initial.CapturedAt {
				t.Fatal("mixed financial captures")
			}
			info = map[string]FinancialContextPageInfo{"positions": page.PositionsPage, "gaps": page.GapsPage, "evidence": page.EvidencePage}[section]
			if info.Returned <= 0 || info.NextCursor == previous {
				t.Fatal("continuation did not advance", section, info)
			}
			count += info.Returned
			content.Positions = append(content.Positions, page.Content.Positions...)
			content.Gaps = append(content.Gaps, page.Content.Gaps...)
			content.Evidence = append(content.Evidence, page.Content.Evidence...)
		}
		if count != info.Total {
			t.Fatal("incomplete detail", section, count, info.Total)
		}
	}
	return content
}

func TestSkillFinancialContextDirectMinimalAndNamedExamples(t *testing.T) {
	t.Parallel()
	s, app, changes := fixture(t)
	bootstrap, err := app.Bootstrap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	secrets := []string{bootstrap.Household.Name, bootstrap.Household.ID.String(), bootstrap.Members[0].Name, bootstrap.Members[0].ID.String()}
	institution, err := app.CreateInstitution(t.Context(), "Private institution", domain.InstitutionBank)
	if err != nil {
		t.Fatal(err)
	}
	group, err := app.CreateGroup(t.Context(), "Private group")
	if err != nil {
		t.Fatal(err)
	}
	secrets = append(secrets, institution.Name, institution.ID.String(), group.Name, group.ID.String())
	for i := 0; i < 8; i++ {
		currency := "USD"
		if i%2 == 1 {
			currency = "EUR" // No FX: gaps are populated as well as positions/evidence.
		}
		a, err := app.CreateAccount(t.Context(), application.AccountInput{Name: "Private account", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: currency, InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		secrets = append(secrets, a.Account.ID.String())
	}
	secrets = append(secrets, "Private account")
	if _, err = s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	instructions := c.InitializeResult().Instructions
	if !strings.Contains(instructions, "call get_financial_context directly without get_context") || !strings.Contains(instructions, "ordinary management") {
		t.Fatal("initialize instructions must distinguish minimal summary from management")
	}
	listed, err := c.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, tool := range listed.Tools {
		seen[tool.Name] = true
	}
	if !seen["get_financial_context"] || !seen["get_financial_context_page"] {
		t.Fatal("skill route is not discoverable")
	}
	// No get_context/catalog/directory call precedes this shipped minimal example.
	initial := decodeContext(t, call(t, c, "get_financial_context", skillExample(t, "analysis", "financial-context", nil), false))
	if initial.Content.Disclosure != "minimal" || initial.Content.Summary.NetWorth.Value != nil {
		t.Fatal("minimal example lost nullable complete totals", initial.Content.Summary)
	}
	if !initial.PositionsPage.HasMore || !initial.GapsPage.HasMore || !initial.EvidencePage.HasMore {
		t.Fatal("fixture must exercise all three shipped page examples")
	}
	content := skillContextDetails(t, c, initial)
	raw, _ := json.Marshal(content)
	for _, secret := range secrets {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("minimal example disclosed directory identity %s", secret)
		}
	}
	// A minimal alias is not an account UUID. No directory fallback is performed.
	args := skillExample(t, "analysis", "financial-context", nil)
	args["scope"] = map[string]any{"kind": "accounts", "accountIds": []string{initial.Content.Scope.AccountRefs[0]}}
	call(t, c, "get_financial_context", args, true)
	// The test now explicitly chooses named disclosure, following its separate example.
	named := decodeContext(t, call(t, c, "get_financial_context", skillExample(t, "analysis", "financial-context-named", nil), false))
	namedContent := skillContextDetails(t, c, named)
	namedRaw, _ := json.Marshal(namedContent)
	if !strings.Contains(string(namedRaw), "Private account") || namedContent.Disclosure != "named" {
		t.Fatal("named example did not reveal the requested account identities")
	}
	// Ordinary management still receives the existing Bootstrap names and IDs.
	contextRaw, _ := json.Marshal(call(t, c, "get_context", Empty{}, false))
	for _, secret := range secrets[:8] {
		if !strings.Contains(string(contextRaw), secret) {
			t.Fatalf("ordinary get_context unexpectedly changed: %s", secret)
		}
	}
	if changes.Load() != 0 {
		t.Fatal("read-only skill examples emitted a write event")
	}
}

func TestSkillFinancialContextDeferredAndExpiredPageExamples(t *testing.T) {
	t.Parallel()
	s, app, _ := fixture(t)
	bootstrap, err := app.Bootstrap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := app.CreateAccount(t.Context(), application.AccountInput{Name: "Synthetic bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.contexts.now = func() time.Time { return now }
	c := connect(t, s)
	// Synthetic frozen output forces a zero-row section; the actual shipped
	// evidence-page arguments must successfully consume its offset-zero cursor.
	value := strings.Repeat("9", 7000)
	entry := cachedFinancialContext{result: application.FinancialContextResult{CapturedAt: now, ContentHash: "synthetic", Content: application.FinancialContextContent{
		Disclosure: "minimal", Summary: application.FinancialContextSummary{KnownAssets: strings.Repeat("9", 21000)},
		Positions: []application.FinancialContextPosition{{Ref: "account-1", NativeAmount: &value}}, Gaps: []application.FinancialContextGap{},
		Evidence: []application.FinancialContextEvidence{{Ref: "evidence-1", Kind: "price", Value: &value}},
	}}, expires: now.Add(contextTTL), size: 1}
	s.contexts.mu.Lock()
	deferred, err := s.contexts.response("deferred", entry, s.contextGeneration, "", 0, 1)
	if err == nil {
		s.contexts.entries["deferred"] = entry
		s.contexts.bytes += entry.size
	}
	s.contexts.mu.Unlock()
	if err != nil || deferred.EvidencePage.Returned != 0 || deferred.EvidencePage.NextCursor == "" {
		t.Fatal("fixture must defer evidence", deferred.EvidencePage, err)
	}
	page := decodeContext(t, call(t, c, "get_financial_context_page", skillExample(t, "analysis", "financial-context-evidence-page", map[string]string{"contextId": deferred.ContextID, "cursor": deferred.EvidencePage.NextCursor}), false))
	if page.EvidencePage.Returned != 1 || page.EvidencePage.HasMore {
		t.Fatal("zero-row section was mistaken for completion")
	}
	initial := decodeContext(t, call(t, c, "get_financial_context", skillExample(t, "analysis", "financial-context", nil), false))
	now = now.Add(contextTTL + time.Second)
	oldArgs := skillExample(t, "analysis", "financial-context-positions-page", map[string]string{"contextId": initial.ContextID, "cursor": initial.PositionsPage.NextCursor})
	checkError := func(arguments map[string]any, code string) {
		t.Helper()
		res, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_financial_context_page", Arguments: arguments})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(res.Content)
		if !res.IsError || !strings.Contains(string(raw), code) {
			t.Fatal("unexpected page failure", code, string(raw))
		}
	}
	checkError(oldArgs, "context_expired")
	fresh := decodeContext(t, call(t, c, "get_financial_context", skillExample(t, "analysis", "financial-context", nil), false))
	if fresh.ContextID == initial.ContextID || fresh.ContentHash != initial.ContentHash {
		t.Fatal("recapture must have a new ID even for unchanged semantic content")
	}
	mixed := skillExample(t, "analysis", "financial-context-positions-page", map[string]string{"contextId": fresh.ContextID, "cursor": initial.PositionsPage.NextCursor})
	checkError(mixed, "validation")
	_ = skillContextDetails(t, c, fresh) // Restart all sections; never append to the old page set.
}

func TestSkillFinancialContextRowFXAndTimeMeanings(t *testing.T) {
	t.Parallel()
	app := wailstest.NewService(t)
	now := time.Now().UTC().Truncate(time.Second)
	app.SetClock(func() time.Time { return now })
	if err := app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Synthetic CNY", BaseCurrency: "CNY", MemberNames: []string{"Synthetic owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := app.Bootstrap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = app.CreateAccount(t.Context(), application.AccountInput{Name: "Synthetic HKD balance", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "HKD", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}}); err != nil {
		t.Fatal(err)
	}
	s := New(app, t.TempDir(), nil)
	t.Cleanup(s.Close)
	if _, err = s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	read := func() application.FinancialContextContent {
		t.Helper()
		initial := decodeContext(t, call(t, c, "get_financial_context", skillExample(t, "analysis", "financial-context", nil), false))
		return skillContextDetails(t, c, initial)
	}
	missing := read()
	statuses := map[string]string{}
	for _, p := range missing.Positions {
		statuses[p.Kind] = p.Status
		if p.Complete || p.BaseAmount != nil || !strings.Contains(strings.Join(p.Missing, ","), "fx_rate") {
			t.Fatalf("missing FX must retain incomplete valuation: %+v", p)
		}
	}
	if statuses["account"] != "unknown" || statuses["balance"] != "active" || missing.Summary.NetWorth.Value != nil {
		t.Fatal("status is row-kind-dependent, not a uniform account lifecycle", statuses, missing.Summary)
	}
	// Fixture setup uses the App API; the MCP connection remains read_only.
	if _, err = app.AppendManualFXQuote(t.Context(), "HKD", "CNY", "0.92", now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err = app.SetFXPreference(t.Context(), "HKD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	valued := read()
	if valued.Basis.BaseCurrency != "CNY" || valued.Summary.NetWorth.Value == nil || *valued.Summary.NetWorth.Value != "92" || valued.DataAsOf.UnknownTimeCount != 0 {
		t.Fatal("synthetic HKD/CNY reporting or unknown-time count changed", valued.Basis, valued.Summary, valued.DataAsOf)
	}
	seenFX := false
	for _, e := range valued.Evidence {
		if e.Kind != "fx" {
			continue
		}
		seenFX = true
		if e.BaseCurrency != "HKD" || e.QuoteCurrency != "CNY" || e.Value == nil || *e.Value != "0.92" || e.Status != "available" || e.ObservationKind != "manual" || e.EffectiveAt == nil || *e.EffectiveAt != now.Format(time.RFC3339) || e.TimestampBasis != "unknown" {
			t.Fatalf("FX direction/time provenance does not match the documented semantics: %+v", e)
		}
	}
	if !seenFX {
		t.Fatal("FX evidence missing")
	}
}

func TestSkillFinancialComparisonExamples(t *testing.T) {
	t.Parallel()
	s, app, changes := fixture(t)
	comparisonFixture(t, s, app)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	initial := decodeComparison(t, call(t, client, "compare_financial_context", skillExample(t, "analysis", "financial-comparison", nil), false))
	if initial.Content.ChangeBasis != "right_minus_left; not_return_or_attribution" {
		t.Fatal(initial.Content.ChangeBasis)
	}
	for _, section := range []string{"positions", "gaps", "evidence"} {
		info := initial.PositionsPage
		if section == "gaps" {
			info = initial.GapsPage
		}
		if section == "evidence" {
			info = initial.EvidencePage
		}
		if !info.HasMore {
			t.Fatal("fixture must exercise every comparison page example", section)
		}
		count, total := info.Returned, info.Total
		for info.HasMore {
			page := decodeComparison(t, call(t, client, "get_financial_comparison_page", skillExample(t, "analysis", "financial-comparison-"+section+"-page", map[string]string{"comparisonId": initial.ComparisonID, "cursor": info.NextCursor}), false))
			if page.ContentHash != initial.ContentHash {
				t.Fatal("mixed package")
			}
			info = page.PositionsPage
			if section == "gaps" {
				info = page.GapsPage
			}
			if section == "evidence" {
				info = page.EvidencePage
			}
			if info.Returned == 0 {
				t.Fatal("page did not advance")
			}
			count += info.Returned
		}
		if count != total {
			t.Fatal(count, total)
		}
	}
	if changes.Load() != 0 {
		t.Fatal("write event")
	}
}
