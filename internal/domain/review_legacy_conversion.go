package domain

import (
	"slices"
	"time"

	"github.com/shopspring/decimal"
)

// BuildReviewLegacyConversion proposes two independent daily rules. The old
// rule is an OR across low-yield and loss evidence over a calendar span; this
// proposal explicitly changes both the vote frequency and trigger semantics.
func BuildReviewLegacyConversion(legacy ReviewLegacySpec, targetKind, timezone string, trigger ReviewTriggerPolicy) (ReviewRuleSpec, ReviewRuleSpec, error) {
	var empty ReviewRuleSpec
	if legacy.Evaluator != ReviewLegacyEvaluator || legacy.SpecVersion != 1 {
		return empty, empty, validation("legacyRule", "a current legacy specification is required")
	}
	trigger, err := trigger.Normalize()
	if err != nil {
		return empty, empty, err
	}
	old := legacy.Configuration
	ids := make([]InstrumentID, 0, len(old.Enrolled))
	for raw, enrolled := range old.Enrolled {
		if enrolled {
			id, err := ParseInstrumentID(raw)
			if err != nil {
				return empty, empty, err
			}
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	target := ReviewRuleTarget{Kind: targetKind, InstrumentIDs: ids}
	if targetKind == "all_current" {
		target.InstrumentIDs = nil // expansion must be visible and explicit
	}
	base := ReviewRuleSpec{
		SpecVersion: ReviewSpecVersion, MetricPolicyVersion: ReviewMetricPolicyVersion,
		Evaluator: ReviewEvaluator, Target: target,
		Schedule:      ReviewRuleSchedule{Period: "calendar_day", Timezone: timezone, CutoffVersion: ReviewCutoffVersion},
		TriggerPolicy: trigger, DataPolicy: DefaultReviewRuleDataPolicy(),
	}
	bank := ReviewPredicateNode{Leaf: &ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []InstrumentType{InstrumentBankInvestmentProduct}}}
	fund := ReviewPredicateNode{Leaf: &ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []InstrumentType{InstrumentMutualFund}}}
	low, _ := decimal.NewFromString(old.Policy.LowAnnualizedPercent)
	// Old annualization uses actual baseline-to-end elapsed days. The period
	// threshold is a bounded recommendation, not a claim of exact equivalence.
	fundThreshold := low.Mul(decimal.NewFromInt(int64(old.Policy.WindowDays))).Div(decimal.NewFromInt(365)).Round(8).String()
	lowYield := base
	lowYield.Predicate = ReviewPredicateNode{Any: []ReviewPredicateNode{
		{All: []ReviewPredicateNode{bank, {Leaf: &ReviewRuleLeaf{Metric: "annualized", Operator: "<", Value: old.Policy.LowAnnualizedPercent, LookbackDays: old.Policy.WindowDays}}}},
		{All: []ReviewPredicateNode{fund, {Leaf: &ReviewRuleLeaf{Metric: "price_change", Operator: "<", Value: fundThreshold, LookbackDays: old.Policy.WindowDays}}}},
	}}
	loss := base
	loss.Predicate = ReviewPredicateNode{All: []ReviewPredicateNode{
		{Leaf: &ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []InstrumentType{InstrumentBankInvestmentProduct, InstrumentMutualFund}}},
		{Leaf: &ReviewRuleLeaf{Metric: "price_change", Operator: "<=", Value: old.Policy.LossPercent, LookbackDays: old.Policy.WindowDays}},
	}}
	lowYield, err = lowYield.Normalize()
	if err != nil {
		return empty, empty, err
	}
	loss, err = loss.Normalize()
	return lowYield, loss, err
}

type ReviewLegacyConversionMutation struct {
	HouseholdID            HouseholdID
	LegacyRuleID           ReviewRuleID
	ExpectedRevision       int
	ExpectedConfigRevision int
	ExpectedStateRevision  int
	ExpectedSourceToken    string
	LowYieldRuleID         ReviewRuleID
	LossRuleID             ReviewRuleID
	MutationID             MutationID
	TargetKind             string
	Timezone               string
	Trigger                ReviewTriggerPolicy
	At                     time.Time
}
