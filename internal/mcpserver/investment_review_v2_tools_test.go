package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/domain"
)

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
		for _, name := range []string{"list_investment_review_rules", "preview_investment_review_rule", "preview_legacy_investment_review_conversion", "list_investment_review_alerts", "get_investment_review_alert", "list_investment_review_daily_evidence", "get_investment_review_coverage", "list_investment_review_suppressions"} {
			if found[name] == nil || !found[name].Annotations.ReadOnlyHint {
				t.Fatalf("%s missing pure read tool %s", mode, name)
			}
		}
		for _, name := range []string{"create_investment_review_rule", "update_investment_review_rule", "archive_investment_review_rule", "convert_legacy_investment_review_rule", "acknowledge_investment_review_alert", "set_investment_review_suppression", "clear_investment_review_suppression", "refresh_investment_reviews"} {
			if (found[name] != nil) != (mode == LedgerWrite) {
				t.Fatalf("%s write tool visibility: %s", mode, name)
			}
		}
		call(t, client, "list_investment_review_rules", ReviewV2PageInput{PageSize: 10}, false)
		call(t, client, "list_investment_review_alerts", ReviewV2AlertPageInput{Filter: domain.ReviewAlertFilter{Status: "all"}, PageSize: 10}, false)
		call(t, client, "get_investment_review_coverage", ReviewV2CoverageInput{Filter: domain.ReviewCoverageFilter{Status: "all"}, PageSize: 10}, false)
		if mode != LedgerWrite {
			continue
		}
		spec := testReviewV2Spec()
		raw, _ := json.Marshal(spec)
		var object map[string]any
		if err := json.Unmarshal(raw, &object); err != nil {
			t.Fatal(err)
		}
		input := ReviewV2RuleInput{RuleID: uuid.NewString(), MutationID: uuid.NewString(), Name: "Synthetic MCP review", Enabled: true, Severity: "notice", Spec: object}
		created := call(t, client, "create_investment_review_rule", map[string]any{"operationId": uuid.NewString(), "input": input}, false)
		if created["data"] == nil {
			t.Fatal("missing create receipt")
		}
		rules := call(t, client, "list_investment_review_rules", ReviewV2PageInput{PageSize: 10}, false)
		serialized, _ := json.Marshal(rules)
		if !strings.Contains(string(serialized), "Synthetic MCP review") {
			t.Fatal("created rule not visible", string(serialized))
		}
		object["sustainDays"] = 5
		bad := input
		bad.RuleID, bad.MutationID, bad.Spec = uuid.NewString(), uuid.NewString(), object
		call(t, client, "create_investment_review_rule", map[string]any{"operationId": uuid.NewString(), "input": bad}, true)
	}
}
