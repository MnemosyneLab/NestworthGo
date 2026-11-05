package mcpserver

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func TestReviewV2FundedTradeHTTPPreviewAndRefresh(t *testing.T) {
	testReviewV2FundedTradeHTTPPreviewAndRefresh(t, false)
}

func TestReviewNoChangeMCPWailsParity(t *testing.T) {
	testReviewV2FundedTradeHTTPPreviewAndRefresh(t, true)
}

type noChangeParityRepository struct {
	*sqlite.Repository
	confirmed int
}

func (r *noChangeParityRepository) ConfirmCertifiedReviewNoChange(ctx context.Context, id domain.HouseholdID, now time.Time, clock func() time.Time, provider domain.ReviewProviderSelection) (domain.ReviewNoChangeResult, bool, error) {
	result, ok, err := r.Repository.ConfirmCertifiedReviewNoChange(ctx, id, now, clock, provider)
	if ok {
		r.confirmed++
	}
	return result, ok, err
}

func testReviewV2FundedTradeHTTPPreviewAndRefresh(t *testing.T, candidate bool) {
	now := time.Date(2026, 9, 1, 0, 5, 0, 0, time.UTC)
	var f ledgerFixture
	var repository *noChangeParityRepository
	if candidate {
		db, err := sqlite.Open(filepath.Join(t.TempDir(), "synthetic-c3-parity.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		repository = &noChangeParityRepository{Repository: sqlite.NewRepository(db).WithDefaultReviewOptimizations()}
		provider := &scaleForbiddenProvider{}
		app := application.NewService(repository, application.NewMarketDataRegistry(provider))
		if err := app.SetFXProvider(provider.Key()); err != nil {
			t.Fatal(err)
		}
		f = newLedgerFixtureWithApp(t, app, func() time.Time { return now })
	} else {
		f = newLedgerFixtureWithClock(t, func() time.Time { return now })
	}
	now = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	enableInvestmentReviewManagement(t, f.service)
	client := ledgerSession(t, f)
	bootstrap, err := f.app.Bootstrap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := domain.ParseAccountID(f.brokerage)
	if err != nil {
		t.Fatal(err)
	}
	instrumentID, err := domain.ParseInstrumentID(f.instrument)
	if err != nil {
		t.Fatal(err)
	}
	cash, _ := domain.ParseMoney("2000", "USD")
	if _, err := f.app.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: accountID, Amount: cash, Reason: domain.ReasonContribution, EffectiveAt: time.Date(2026, 9, 2, 0, 10, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	qty, _ := domain.ParseQuantity("10")
	gross, _ := domain.ParseMoney("1000", "USD")
	fee, _ := domain.ParseMoney("5", "USD")
	if _, err := f.app.RecordChange(t.Context(), domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: accountID, InstrumentID: instrumentID, Quantity: qty, Gross: gross, Fee: &fee, EffectiveAt: time.Date(2026, 9, 2, 0, 20, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: []application.AgentMarketDataItem{
		{InstrumentID: instrumentID.String(), Currency: "USD", Value: "100", Kind: "close", Date: "2026-09-02", Timezone: "UTC", SourceTitle: "Synthetic close"},
		{InstrumentID: instrumentID.String(), Currency: "USD", Value: "100", Kind: "close", Date: "2026-09-03", Timezone: "UTC", SourceTitle: "Synthetic close"},
		{InstrumentID: instrumentID.String(), Currency: "USD", Value: "100", Kind: "close", Date: "2026-09-04", Timezone: "UTC", SourceTitle: "Synthetic close"},
	}}); err != nil {
		t.Fatal(err)
	}
	spec := testReviewV2Spec()
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
		{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "900", Currency: "USD", FXMode: "native"}},
		{Leaf: &domain.ReviewRuleLeaf{Metric: "cost", Operator: ">", Value: "1002", Currency: "USD", FXMode: "native"}},
	}}
	var object map[string]any
	for _, policy := range []domain.ReviewTriggerPolicy{{Kind: "single"}, {Kind: "consecutive", Count: 2}, {Kind: "rolling", WindowDays: 3, MinHits: 2}} {
		spec.TriggerPolicy = policy
		encoded, _ := jsoncodec.Marshal(spec)
		if err := jsoncodec.Unmarshal(encoded, &object); err != nil {
			t.Fatal(err)
		}
		request := ReviewV2PreviewInput{Spec: object, InstrumentID: instrumentID.String(), LatestClosedOn: "2026-09-04"}
		wails, err := liquidity.NewService(f.app).PreviewReviewRuleV2(t.Context(), liquidity.PreviewReviewRuleV2Request{Spec: spec, InstrumentID: request.InstrumentID, LatestClosedOn: request.LatestClosedOn})
		if err != nil || wails.Window.Truth != domain.ReviewTrue || len(wails.Days) == 0 {
			t.Fatalf("%s known window: %+v %v", policy.Kind, wails, err)
		}
		latest := wails.Days[len(wails.Days)-1].Result
		if latest.Truth != domain.ReviewTrue || len(latest.Leaves) != 2 || latest.Leaves[0].Actual != "1000.0000" || latest.Leaves[1].Actual != "1005.0000" {
			t.Fatalf("%s metrics: %+v", policy.Kind, latest)
		}
		mcpPreview := sectionEvidence(t, client, "preview_investment_review_rule", request)
		if !reflect.DeepEqual(mcpPreview, jsonObject(t, wails)) {
			t.Fatalf("%s MCP and Wails preview differ: MCP=%+v Wails=%+v", policy.Kind, mcpPreview, wails)
		}
	}
	call(t, client, "create_investment_review_rule", map[string]any{"operationId": uuid.NewString(), "input": ReviewV2RuleInput{RuleID: uuid.NewString(), MutationID: uuid.NewString(), Name: "Synthetic funded trade", Enabled: true, Severity: "notice", Spec: object}}, false)
	refreshed := call(t, client, "refresh_investment_reviews", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{}}, false)["data"].(map[string]any)["result"]
	wailsRefresh, err := liquidity.NewService(f.app).ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || refreshed.(map[string]any)["sourceFingerprint"] != wailsRefresh.SourceFingerprint || refreshed.(map[string]any)["ruleInstrumentCount"] != float64(wailsRefresh.RuleInstrumentCount) || refreshed.(map[string]any)["latestClosedOn"] != wailsRefresh.LatestClosedOn || wailsRefresh.DayCount != 0 {
		t.Fatalf("MCP and Wails refresh differ: MCP=%+v Wails=%+v err=%v", refreshed, wailsRefresh, err)
	}
	if candidate && repository.confirmed != 1 {
		t.Fatal("Wails did not use shared no-change confirmation", repository.confirmed)
	}
	if candidate {
		call(t, client, "refresh_investment_reviews", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{}}, false)
		if repository.confirmed != 2 {
			t.Fatal("MCP did not use shared no-change confirmation", repository.confirmed)
		}
	}
	// Both adapters read the same expired publication after a time-only boundary;
	// neither implicitly refreshes it or claims that its certificate is current.
	now = now.AddDate(0, 0, 1)
	page, err := liquidity.NewService(f.app).ListReviewAlertsV2(t.Context(), liquidity.ListReviewAlertsV2Request{Filter: domain.ReviewAlertFilter{Status: "all"}, PageSize: 10})
	if err != nil || page.ProjectionStatus != "stale" {
		t.Fatalf("expired Wails publication: %+v %v", page, err)
	}
	listed := call(t, client, "list_investment_review_alerts", ReviewV2AlertPageInput{Filter: domain.ReviewAlertFilter{Status: "all"}, PageSize: 10}, false)["data"]
	if !reflect.DeepEqual(withoutPage(listed), reviewPageBusiness(t, page)) {
		t.Fatal("MCP/Wails temporal alert status differs")
	}
	coverage, err := liquidity.NewService(f.app).ListReviewCoverageV2(t.Context(), liquidity.ListReviewCoverageV2Request{Filter: domain.ReviewCoverageFilter{Status: "all"}, PageSize: 10})
	if err != nil || coverage.ProjectionStatus != "stale" {
		t.Fatalf("expired Wails coverage: %+v %v", coverage, err)
	}
	covered := call(t, client, "get_investment_review_coverage", ReviewV2CoverageInput{Filter: domain.ReviewCoverageFilter{Status: "all"}, PageSize: 10}, false)["data"]
	if !reflect.DeepEqual(withoutPage(covered), reviewPageBusiness(t, coverage)) {
		t.Fatal("MCP/Wails temporal coverage status differs")
	}
}

func testReviewV2Spec() domain.ReviewRuleSpec {
	return domain.ReviewRuleSpec{
		SpecVersion: 1, MetricPolicyVersion: "daily_metrics_v1", Evaluator: "daily_predicate_v1",
		Target:        domain.ReviewRuleTarget{Kind: "all_current"},
		Predicate:     domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "1", Currency: "USD", FXMode: "native"}}}},
		Schedule:      domain.ReviewRuleSchedule{Period: "calendar_day", Timezone: "UTC", CutoffVersion: "civil_day_end_v1"},
		TriggerPolicy: domain.ReviewTriggerPolicy{Kind: "single"}, DataPolicy: domain.DefaultReviewRuleDataPolicy(),
	}
}

func TestReviewV2MCPModesAndStrictRuleInput(t *testing.T) {
	fx := newLedgerFixture(t)
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		if _, err := fx.service.Enable(mode); err != nil {
			t.Fatal(err)
		}
		client := connect(t, fx.service)
		listed, err := listAllTools(t.Context(), client)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]*mcp.Tool{}
		for _, tool := range listed.Tools {
			found[tool.Name] = tool
		}
		for _, name := range []string{"list_investment_review_rules", "preview_investment_review_rule", "list_investment_review_alerts", "get_investment_review_alert", "list_investment_review_daily_evidence", "list_investment_review_episode_history", "list_investment_review_episode_events", "get_investment_review_coverage", "list_investment_review_suppressions"} {
			if found[name] == nil || !found[name].Annotations.ReadOnlyHint {
				t.Fatalf("%s missing pure read tool %s", mode, name)
			}
		}
		for _, name := range investmentReviewManagementTools {
			if found[name] != nil {
				t.Fatalf("%s default discovery listed management tool %s", mode, name)
			}
		}
		info := call(t, client, "get_context", Empty{}, false)["data"].(map[string]any)
		if info["investmentReviewManagement"] != false {
			t.Fatalf("%s investmentReviewManagement = %v", mode, info["investmentReviewManagement"])
		}
		caps, _ := jsoncodec.Marshal(info["capabilities"])
		if strings.Contains(string(caps), "investment_review_v2_write") {
			t.Fatalf("%s advertised review management: %s", mode, caps)
		}
		for _, name := range []string{
			"get_investment_reviews",
			"save_investment_review_policy",
			"acknowledge_investment_review",
			"set_investment_review_enrollment",
			"set_investment_review_snooze",
			"set_investment_review_enrollment_batch",
			"set_investment_review_snooze_batch",
			"acknowledge_investment_reviews_batch",
		} {
			if found[name] != nil {
				t.Fatalf("retired legacy review tool %s is still discoverable in %s mode", name, mode)
			}
		}
		call(t, client, "list_investment_review_rules", ReviewV2PageInput{PageSize: 10}, false)
		call(t, client, "list_investment_review_alerts", ReviewV2AlertPageInput{Filter: domain.ReviewAlertFilter{Status: "all"}, PageSize: 10}, false)
		call(t, client, "get_investment_review_coverage", ReviewV2CoverageInput{Filter: domain.ReviewCoverageFilter{Status: "all"}, PageSize: 10}, false)
		if mode != LedgerWrite {
			continue
		}
		hidden, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "create_investment_review_rule", Arguments: map[string]any{"operationId": uuid.NewString(), "input": map[string]any{}}})
		if err == nil || !strings.Contains(err.Error(), "unknown tool") {
			t.Fatalf("default ledger_write accepted management call: %v %v", err, hidden)
		}
	}
}

// Previous ledger_write contract hashes for the management tools. Enabling the
// switch must restore these schemas, not a new permission shape.
var investmentReviewManagementContract = map[string]string{
	"acknowledge_investment_review_alert": "3e1c7c6d35482e7d31aee015bf5739288706963383f1ce030e697ef4b64ca331",
	"archive_investment_review_rule":      "b5be38adcce79f4099e084021719672fac9bb0942ea5015c8dd327494b2d0a56",
	"clear_investment_review_suppression": "f838de7930c41504f16ac99b83d7f2bc3effb624eefa7637f34ca3cb3570fd4a",
	"create_investment_review_rule":       "a8e8d32632624b5b6b4c72489df2d512815171a8ebc18f9dd5a02c2d1b03fa02",
	"refresh_investment_reviews":          "4d84af6afddbf95acbd09d665cb3dbc22166d53350e6b8c6f068e62bccced0a0",
	"set_investment_review_suppression":   "de65f7fb79eb669b5e77b77eab8cdeb7f2615add1072a3687ecd24348f68b9d9",
	"update_investment_review_rule":       "347f3bf2843fee3a177bf11f5caefec855e2ad43b9987ac40cc7fc0745b1dbac",
}

func enableInvestmentReviewManagement(t *testing.T, s *Service) {
	t.Helper()
	if _, err := s.SetInvestmentReviewManagement(true); err != nil {
		t.Fatal(err)
	}
}

func TestInvestmentReviewManagementSwitchRestoresLedgerWrites(t *testing.T) {
	fx := newLedgerFixture(t)
	if _, err := fx.service.Enable(LedgerWrite); err != nil {
		t.Fatal(err)
	}
	before, err := fx.service.Connection()
	if err != nil {
		t.Fatal(err)
	}
	defaultListed, err := listAllTools(t.Context(), connect(t, fx.service))
	if err != nil {
		t.Fatal(err)
	}
	defaultNames := map[string]bool{}
	for _, tool := range defaultListed.Tools {
		defaultNames[tool.Name] = true
	}
	for _, name := range investmentReviewManagementTools {
		if defaultNames[name] {
			t.Fatalf("default discovery listed %s", name)
		}
	}
	enableInvestmentReviewManagement(t, fx.service)
	after, err := fx.service.Connection()
	if err != nil {
		t.Fatal(err)
	}
	if after.Token == "" || after.Token == before.Token {
		t.Fatal("enabling review management did not replace the access token")
	}
	client := connect(t, fx.service)
	listed, err := listAllTools(t.Context(), client)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]*mcp.Tool{}
	for _, tool := range listed.Tools {
		found[tool.Name] = tool
	}
	if len(found) != len(defaultNames)+len(investmentReviewManagementTools) {
		t.Fatalf("enabled discovery = %d tools, default %d", len(found), len(defaultNames))
	}
	hashes := discoveryContractHashes(t, listed.Tools)
	for _, name := range investmentReviewManagementTools {
		if found[name] == nil {
			t.Fatalf("enabled discovery missing %s", name)
		}
		if hashes[name] != investmentReviewManagementContract[name] {
			t.Fatalf("%s contract hash = %s", name, hashes[name])
		}
	}
	info := call(t, client, "get_context", Empty{}, false)["data"].(map[string]any)
	if info["mode"] != LedgerWrite || info["investmentReviewManagement"] != true {
		t.Fatal(info)
	}
	caps, _ := jsoncodec.Marshal(info["capabilities"])
	if !strings.Contains(string(caps), "investment_review_v2_write") {
		t.Fatal(string(caps))
	}
	spec := testReviewV2Spec()
	raw, _ := jsoncodec.Marshal(spec)
	var object map[string]any
	if err := jsoncodec.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	input := ReviewV2RuleInput{RuleID: uuid.NewString(), MutationID: uuid.NewString(), Name: "Synthetic MCP review", Enabled: true, Severity: "notice", Spec: object}
	created := call(t, client, "create_investment_review_rule", map[string]any{"operationId": uuid.NewString(), "input": input}, false)
	if created["data"] == nil {
		t.Fatal("missing create receipt")
	}
	rules := call(t, client, "list_investment_review_rules", ReviewV2PageInput{PageSize: 10}, false)
	serialized, _ := jsoncodec.Marshal(rules)
	if !strings.Contains(string(serialized), "Synthetic MCP review") {
		t.Fatal("created rule not visible", string(serialized))
	}
	object["sustainDays"] = 5
	bad := input
	bad.RuleID, bad.MutationID, bad.Spec = uuid.NewString(), uuid.NewString(), object
	call(t, client, "create_investment_review_rule", map[string]any{"operationId": uuid.NewString(), "input": bad}, true)

	// The same flag does not grant management tools without ledger_write.
	if _, err := fx.service.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	directory := connect(t, fx.service)
	directoryListed, err := listAllTools(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range directoryListed.Tools {
		for _, name := range investmentReviewManagementTools {
			if tool.Name == name {
				t.Fatalf("directory_write listed %s with the switch on", name)
			}
		}
	}
	directoryInfo := call(t, directory, "get_context", Empty{}, false)["data"].(map[string]any)
	if directoryInfo["investmentReviewManagement"] != true {
		t.Fatal(directoryInfo)
	}
	directoryCaps, _ := jsoncodec.Marshal(directoryInfo["capabilities"])
	if strings.Contains(string(directoryCaps), "investment_review_v2_write") {
		t.Fatal(string(directoryCaps))
	}

	// Returning to ledger_write keeps the saved switch and the prior tools.
	if _, err := fx.service.Enable(LedgerWrite); err != nil {
		t.Fatal(err)
	}
	restored := connect(t, fx.service)
	restoredListed, err := listAllTools(t.Context(), restored)
	if err != nil {
		t.Fatal(err)
	}
	restoredNames := map[string]bool{}
	for _, tool := range restoredListed.Tools {
		restoredNames[tool.Name] = true
	}
	for _, name := range investmentReviewManagementTools {
		if !restoredNames[name] {
			t.Fatalf("ledger_write lost %s after the switch stayed on", name)
		}
	}
	if _, err := fx.service.Disable(); err != nil {
		t.Fatal(err)
	}
	resumed := New(fx.app, fx.service.dir, nil)
	t.Cleanup(resumed.Close)
	if err := resumed.Resume(); err != nil {
		t.Fatal(err)
	}
	if resumed.Status().InvestmentReviewManagement != true || resumed.Status().Running {
		t.Fatal(resumed.Status())
	}
	if _, err := resumed.Enable(LedgerWrite); err != nil {
		t.Fatal(err)
	}
	resumedClient := connect(t, resumed)
	resumedListed, err := listAllTools(t.Context(), resumedClient)
	if err != nil {
		t.Fatal(err)
	}
	resumedNames := map[string]bool{}
	for _, tool := range resumedListed.Tools {
		resumedNames[tool.Name] = true
	}
	for _, name := range investmentReviewManagementTools {
		if !resumedNames[name] {
			t.Fatalf("resumed ledger_write hid %s", name)
		}
	}
	same, err := resumed.SetInvestmentReviewManagement(true)
	if err != nil || !same.InvestmentReviewManagement {
		t.Fatal(same, err)
	}
	unchanged, err := resumed.Connection()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resumed.SetInvestmentReviewManagement(false); err != nil {
		t.Fatal(err)
	}
	hiddenToken, err := resumed.Connection()
	if err != nil || hiddenToken.Token == unchanged.Token {
		t.Fatal("disabling review management did not replace the token", err)
	}
	hiddenClient := connect(t, resumed)
	hiddenListed, err := listAllTools(t.Context(), hiddenClient)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range hiddenListed.Tools {
		if _, managed := investmentReviewManagementContract[tool.Name]; managed {
			t.Fatalf("disabled discovery still listed %s", tool.Name)
		}
	}
	rejected, err := hiddenClient.CallTool(t.Context(), &mcp.CallToolParams{Name: "refresh_investment_reviews", Arguments: map[string]any{"operationId": uuid.NewString(), "input": map[string]any{}}})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("hidden management call: %v %v", err, rejected)
	}
}

// Paging metadata is transport-owned; keep every authoritative business field
// in the Wails parity oracle, including publication status/revision and rows.
func reviewPageBusiness(t *testing.T, v any) map[string]any {
	out := jsonObject(t, v)
	delete(out, "nextCursor")
	return out
}
func withoutPage(v any) map[string]any {
	out := v.(map[string]any)
	copy := map[string]any{}
	for k, v := range out {
		if k != "page" {
			copy[k] = v
		}
	}
	return copy
}
