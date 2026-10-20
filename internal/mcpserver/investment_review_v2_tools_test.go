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
		mcpPreview := call(t, client, "preview_investment_review_rule", request, false)["data"]
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
	if !reflect.DeepEqual(listed, jsonObject(t, page)) {
		t.Fatal("MCP/Wails temporal alert status differs")
	}
	coverage, err := liquidity.NewService(f.app).ListReviewCoverageV2(t.Context(), liquidity.ListReviewCoverageV2Request{Filter: domain.ReviewCoverageFilter{Status: "all"}, PageSize: 10})
	if err != nil || coverage.ProjectionStatus != "stale" {
		t.Fatalf("expired Wails coverage: %+v %v", coverage, err)
	}
	covered := call(t, client, "get_investment_review_coverage", ReviewV2CoverageInput{Filter: domain.ReviewCoverageFilter{Status: "all"}, PageSize: 10}, false)["data"]
	if !reflect.DeepEqual(covered, jsonObject(t, coverage)) {
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
		listed, err := client.ListTools(t.Context(), nil)
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
		for _, name := range []string{"create_investment_review_rule", "update_investment_review_rule", "archive_investment_review_rule", "acknowledge_investment_review_alert", "set_investment_review_suppression", "clear_investment_review_suppression", "refresh_investment_reviews"} {
			if (found[name] != nil) != (mode == LedgerWrite) {
				t.Fatalf("%s write tool visibility: %s", mode, name)
			}
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
	}
}
