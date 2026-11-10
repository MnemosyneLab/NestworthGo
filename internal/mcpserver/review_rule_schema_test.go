package mcpserver

import (
	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/agentguidance"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"os"
	"strings"
	"testing"
)

func TestReviewRuleSchemaMatchesFiniteDomainGrammar(t *testing.T) {
	s, _, _ := fixture(t)
	registry := s.toolRegistry(ReadWrite)
	for _, name := range []string{"preview_investment_review_rule", "create_investment_review_rule", "update_investment_review_rule"} {
		t.Run(name, func(t *testing.T) {
			makeSpec := func() map[string]any {
				raw, _ := jsoncodec.Marshal(testReviewV2Spec())
				var spec map[string]any
				jsoncodec.Unmarshal(raw, &spec)
				return spec
			}
			arguments := func(spec map[string]any) []byte {
				var value any = map[string]any{"spec": spec, "instrumentId": uuid.NewString()}
				if name != "preview_investment_review_rule" {
					value = map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"ruleId": uuid.NewString(), "mutationId": uuid.NewString(), "expectedRevision": 0, "name": "Synthetic schema", "enabled": true, "severity": "p1", "spec": spec}}
				}
				raw, _ := jsoncodec.Marshal(value)
				return raw
			}
			for _, leaf := range []domain.ReviewRuleLeaf{
				{Metric: "amount", Operator: ">", Value: "1", Currency: "USD", FXMode: "native"},
				{Metric: "cost", Operator: "<=", Value: "12.34", Currency: "USD", FXMode: "convert_on_day"},
				{Metric: "instrument", Operator: "not_in", InstrumentIDs: []domain.InstrumentID{domain.InstrumentID(uuid.NewString())}},
				{Metric: "type", Operator: "in", Types: []domain.InstrumentType{domain.InstrumentETF}},
				{Metric: "price_change", Operator: "<", Value: "-10", LookbackDays: 30},
				{Metric: "annualized", Operator: "<=", Value: "1.5", LookbackDays: 365},
			} {
				spec := testReviewV2Spec()
				spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []domain.InstrumentType{domain.InstrumentETF}}}, {Any: []domain.ReviewPredicateNode{{Leaf: &leaf}}}}}
				// An annualized branch inherits the root AND type gate.
				if _, err := spec.Normalize(); err != nil {
					t.Fatal(err)
				}
				raw, _ := jsoncodec.Marshal(spec)
				var object map[string]any
				jsoncodec.Unmarshal(raw, &object)
				if err := registry.ValidateArguments(name, arguments(object)); err != nil {
					t.Fatalf("valid metric %s rejected: %v", leaf.Metric, err)
				}
			}
			for _, mutate := range []func(map[string]any){
				func(s map[string]any) { delete(s, "specVersion") },
				func(s map[string]any) { s["specVersion"] = 2 },
				func(s map[string]any) { s["evaluator"] = "invented" },
				func(s map[string]any) { s["legacyDuration"] = 5 },
				func(s map[string]any) { s["dataPolicy"].(map[string]any)["missing"] = "zero" },
				func(s map[string]any) { s["schedule"].(map[string]any)["cutoffVersion"] = "invented" },
				func(s map[string]any) { s["target"] = map[string]any{"kind": "instruments"} },
				func(s map[string]any) { s["predicate"] = map[string]any{"leaf": map[string]any{"metric": "amount"}} },
				func(s map[string]any) { s["predicate"] = map[string]any{"all": []any{}} },
				func(s map[string]any) {
					s["predicate"] = map[string]any{"all": []any{map[string]any{"all": []any{map[string]any{"all": []any{map[string]any{"leaf": map[string]any{"metric": "amount"}}}}}}}}
				},
				func(s map[string]any) { s["triggerPolicy"] = map[string]any{"kind": "rolling", "windowDays": 30} },
				func(s map[string]any) { s["triggerPolicy"] = map[string]any{"kind": "single", "count": 2} },
			} {
				spec := makeSpec()
				mutate(spec)
				if err := registry.ValidateArguments(name, arguments(spec)); err == nil {
					t.Fatalf("invalid spec passed published schema: %v", spec)
				}
			}
		})
	}
}

func TestSkillReviewRuleRecipeRunsWithoutSourceOnlySpec(t *testing.T) {
	fx := newLedgerFixture(t)
	client := ledgerSession(t, fx)
	args := skillExample(t, "analysis", "review-rule-preview", map[string]string{"instrumentId": fx.instrument})
	call(t, client, "preview_investment_review_rule", args, false)
	input := map[string]any{"ruleId": uuid.NewString(), "mutationId": uuid.NewString(), "expectedRevision": 0, "name": "Synthetic shared recipe", "enabled": true, "severity": "p1", "spec": args["spec"]}
	call(t, client, "create_investment_review_rule", map[string]any{"operationId": uuid.NewString(), "input": input}, false)
	rules := call(t, client, "list_investment_review_rules", map[string]any{}, false)
	raw, _ := jsoncodec.Marshal(rules)
	if !strings.Contains(string(raw), "Synthetic shared recipe") {
		t.Fatal("recipe did not create a readable rule")
	}
}

func TestSharedRecipeEntriesMatchOptionalRegistryTools(t *testing.T) {
	s, _, _ := fixture(t)
	registry := s.toolRegistry(ReadWrite)
	raw, err := os.ReadFile("../agentguidance/recipes.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		Tool string `json:"tool"`
	}
	if err := jsoncodec.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		d, err := registry.Definition(entry.Tool)
		if err != nil || d.ToolGroup() == agentcapability.CoreTools {
			t.Fatalf("missing/nonoptional recipe entry %s: %v", entry.Tool, err)
		}
		if agentguidance.Description(d.Name, d.Description) == d.Description {
			t.Fatalf("recipe unreachable: %s", d.Name)
		}
	}
}
