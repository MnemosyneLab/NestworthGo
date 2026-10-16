package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func syntheticReviewSpec() ReviewRuleSpec {
	return ReviewRuleSpec{
		SpecVersion: ReviewSpecVersion, MetricPolicyVersion: ReviewMetricPolicyVersion, Evaluator: ReviewEvaluator,
		Target: ReviewRuleTarget{Kind: "all_current"},
		Predicate: ReviewPredicateNode{All: []ReviewPredicateNode{
			{Leaf: &ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []InstrumentType{InstrumentBankInvestmentProduct}}},
			{Leaf: &ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "1000.00", Currency: "CNY", FXMode: "convert_on_day"}},
			{Leaf: &ReviewRuleLeaf{Metric: "annualized", Operator: "<", Value: "1.20", LookbackDays: 30}},
		}},
		Schedule:      ReviewRuleSchedule{Period: "calendar_day", Timezone: "Asia/Shanghai", CutoffVersion: ReviewCutoffVersion},
		TriggerPolicy: ReviewTriggerPolicy{Kind: "rolling", WindowDays: 10, MinHits: 5},
		DataPolicy:    DefaultReviewRuleDataPolicy(),
	}
}

func TestReviewSpecStrictDecodeAndCanonicalHash(t *testing.T) {
	spec := syntheticReviewSpec()
	hash, err := spec.SemanticHash()
	if err != nil {
		t.Fatal(err)
	}
	reordered := syntheticReviewSpec()
	reordered.Predicate.All[1], reordered.Predicate.All[2] = reordered.Predicate.All[2], reordered.Predicate.All[1]
	reordered.Predicate.All[1].Leaf.Value = "1.2"
	reordered.Predicate.All[2].Leaf.Value = "1000"
	other, err := reordered.SemanticHash()
	if err != nil || hash != other {
		t.Fatalf("semantic hash changed: %s, %s, %v", hash, other, err)
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeReviewRuleSpec(raw)
	if err != nil || decoded.TriggerPolicy.Kind != "rolling" {
		t.Fatalf("decode: %+v %v", decoded, err)
	}
	for _, invalid := range [][]byte{
		[]byte(strings.Replace(string(raw), `"metric":"annualized"`, `"metric":"annualized","duration":{"scope":"performance"}`, 1)),
		[]byte(string(raw) + ` {}`),
		[]byte(strings.Replace(string(raw), `"specVersion":1`, `"specVersion":2`, 1)),
	} {
		if _, err := DecodeReviewRuleSpec(invalid); err == nil {
			t.Fatalf("accepted invalid JSON: %s", invalid)
		}
	}
}

func TestReviewSpecRejectsAmbiguousOrUnsupportedSemantics(t *testing.T) {
	checks := []struct {
		name string
		edit func(*ReviewRuleSpec)
	}{
		{"native threshold with excessive minor units", func(s *ReviewRuleSpec) { s.Predicate.All[1].Leaf.Value = "1000.001" }},
		{"scientific percentage", func(s *ReviewRuleSpec) { s.Predicate.All[2].Leaf.Value = "1.2e0" }},
		{"unscoped annualized", func(s *ReviewRuleSpec) { s.Predicate.All = s.Predicate.All[1:] }},
		{"stock cannot scope annualized", func(s *ReviewRuleSpec) { s.Predicate.All[0].Leaf.Types = []InstrumentType{InstrumentStock} }},
		{"unknown type", func(s *ReviewRuleSpec) { s.Predicate.All[0].Leaf.Types = []InstrumentType{"mystery"} }},
		{"old duration", func(s *ReviewRuleSpec) { s.TriggerPolicy = ReviewTriggerPolicy{Kind: "consecutive", Count: 0} }},
		{"wrong timezone", func(s *ReviewRuleSpec) { s.Schedule.Timezone = "Mars/Base" }},
		{"unknown data policy", func(s *ReviewRuleSpec) { s.DataPolicy.QuoteCarry = "forward_fill" }},
		{"machine-local timezone", func(s *ReviewRuleSpec) { s.Schedule.Timezone = "Local" }},
		{"impossible amount interval", func(s *ReviewRuleSpec) {
			s.Predicate.All = append(s.Predicate.All, ReviewPredicateNode{Leaf: &ReviewRuleLeaf{Metric: "amount", Operator: "<=", Value: "1000", Currency: "CNY", FXMode: "convert_on_day"}})
		}},
		{"too many nested groups", func(s *ReviewRuleSpec) {
			s.Predicate.All[0] = ReviewPredicateNode{Any: []ReviewPredicateNode{
				{All: []ReviewPredicateNode{
					{Leaf: &ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []InstrumentType{InstrumentBankInvestmentProduct}}},
				}},
			}}
		}},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			s := syntheticReviewSpec()
			tc.edit(&s)
			if _, err := s.Normalize(); err == nil {
				t.Fatal("accepted invalid spec")
			}
		})
	}
}

func TestReviewSpecOneDayPoliciesNormalizeTogether(t *testing.T) {
	s := syntheticReviewSpec()
	s.TriggerPolicy = ReviewTriggerPolicy{Kind: "single"}
	hash, err := s.SemanticHash()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []ReviewTriggerPolicy{{Kind: "consecutive", Count: 1}, {Kind: "rolling", WindowDays: 1, MinHits: 1}} {
		s.TriggerPolicy = p
		got, err := s.SemanticHash()
		if err != nil || got != hash {
			t.Fatalf("%+v hash: %s %v", p, got, err)
		}
	}
}

func TestReviewPredicateTwentyLeafLimitIncludesNestedGroups(t *testing.T) {
	makeGroup := func(count int) ReviewPredicateNode {
		children := make([]ReviewPredicateNode, count)
		for index := range children {
			children[index] = ReviewPredicateNode{Leaf: &ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: fmt.Sprintf("%d", index+1), Currency: "CNY", FXMode: "native"}}
		}
		return ReviewPredicateNode{All: children}
	}
	spec := syntheticReviewSpec()
	// Keep root fanout below 20 so the test exercises the global leaf cap
	// across the allowed nested groups rather than a single group's fanout cap.
	spec.Predicate = ReviewPredicateNode{All: []ReviewPredicateNode{makeGroup(10), makeGroup(10)}}
	if _, err := spec.Normalize(); err != nil {
		t.Fatalf("exactly 20 nested leaves should be accepted: %v", err)
	}
	spec.Predicate.All = append(spec.Predicate.All, ReviewPredicateNode{Leaf: &ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "21", Currency: "CNY", FXMode: "native"}})
	if _, err := spec.Normalize(); err == nil {
		t.Fatal("accepted 21 leaves spread across nested groups")
	}
}
