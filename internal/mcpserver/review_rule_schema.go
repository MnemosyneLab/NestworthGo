package mcpserver

import (
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Publish the finite wire grammar instead of an unconstrained map. Predicate
// depth is bounded by the domain contract, so refs share schemas without a
// recursive schema. Semantic scope, money precision, intervals, annualized type
// gates, total leaf count and source validation remain in DecodeReviewRuleSpec.
func addReviewRuleSchema(root *jsonschema.Schema, properties map[string]*jsonschema.Schema) {
	target := reviewSchemaFor[domain.ReviewRuleTarget]()
	setSchemaEnum(target.Properties["kind"], []string{"all_current", "instruments"})
	target.Properties["instrumentIds"].MaxItems = reviewInt(200)
	target.OneOf = []*jsonschema.Schema{
		{Properties: map[string]*jsonschema.Schema{"kind": {Enum: []any{"all_current"}}, "instrumentIds": {MaxItems: reviewInt(0)}}},
		{Properties: map[string]*jsonschema.Schema{"kind": {Enum: []any{"instruments"}}, "instrumentIds": {MinItems: reviewInt(1)}}, Required: []string{"instrumentIds"}},
	}
	schedule := reviewSchemaFor[domain.ReviewRuleSchedule]()
	setSchemaEnum(schedule.Properties["period"], []string{"calendar_day"})
	setSchemaEnum(schedule.Properties["cutoffVersion"], []string{domain.ReviewCutoffVersion})
	schedule.Properties["timezone"].Description = "Explicit fixed IANA timezone; never Local."
	data := reviewSchemaFor[domain.ReviewRuleDataPolicy]()
	raw, _ := jsoncodec.Marshal(domain.DefaultReviewRuleDataPolicy())
	var values map[string]any
	if err := jsoncodec.Unmarshal(raw, &values); err != nil {
		panic(err)
	}
	for name, value := range values {
		data.Properties[name].Enum = []any{value}
	}
	trigger := reviewSchemaFor[domain.ReviewTriggerPolicy]()
	setSchemaEnum(trigger.Properties["kind"], []string{"single", "consecutive", "rolling"})
	for _, field := range []string{"count", "windowDays", "minHits"} {
		trigger.Properties[field].Minimum = reviewNumber(0)
		trigger.Properties[field].Maximum = reviewNumber(365)
	}
	trigger.OneOf = []*jsonschema.Schema{
		{Properties: map[string]*jsonschema.Schema{"kind": {Enum: []any{"single"}}, "count": {Enum: []any{0}}, "windowDays": {Enum: []any{0}}, "minHits": {Enum: []any{0}}}},
		{Properties: map[string]*jsonschema.Schema{"kind": {Enum: []any{"consecutive"}}, "count": {Minimum: reviewNumber(1)}, "windowDays": {Enum: []any{0}}, "minHits": {Enum: []any{0}}}, Required: []string{"count"}},
		{Properties: map[string]*jsonschema.Schema{"kind": {Enum: []any{"rolling"}}, "count": {Enum: []any{0}}, "windowDays": {Minimum: reviewNumber(1)}, "minHits": {Minimum: reviewNumber(1)}}, Required: []string{"windowDays", "minHits"}},
	}
	leaf := reviewSchemaFor[domain.ReviewRuleLeaf]()
	setSchemaEnum(leaf.Properties["metric"], []string{"instrument", "type", "amount", "cost", "price_change", "annualized"})
	setSchemaEnum(leaf.Properties["operator"], []string{"in", "not_in", ">", ">=", "<", "<="})
	setSchemaEnum(leaf.Properties["fxMode"], []string{"", "native", "convert_on_day"})
	leaf.Properties["lookbackDays"].Enum = []any{0, 7, 30, 90, 180, 365}
	leaf.Properties["instrumentIds"].MaxItems = reviewInt(200)
	types := []string{}
	for _, value := range domain.AllInstrumentTypes() {
		types = append(types, string(value))
	}
	setSchemaEnum(leaf.Properties["types"].Items, types)
	numericOperator := &jsonschema.Schema{Enum: []any{">", ">=", "<", "<="}}
	leaf.OneOf = []*jsonschema.Schema{
		{Properties: map[string]*jsonschema.Schema{"metric": {Enum: []any{"amount", "cost"}}, "operator": numericOperator}, Required: []string{"operator", "value", "currency", "fxMode"}},
		{Properties: map[string]*jsonschema.Schema{"metric": {Enum: []any{"price_change", "annualized"}}, "operator": numericOperator, "lookbackDays": {Enum: []any{7, 30, 90, 180, 365}}}, Required: []string{"operator", "value", "lookbackDays"}},
		{Properties: map[string]*jsonschema.Schema{"metric": {Enum: []any{"instrument"}}, "operator": {Enum: []any{"in", "not_in"}}, "instrumentIds": {MinItems: reviewInt(1)}}, Required: []string{"operator", "instrumentIds"}},
		{Properties: map[string]*jsonschema.Schema{"metric": {Enum: []any{"type"}}, "operator": {Enum: []any{"in"}}, "types": {MinItems: reviewInt(1)}}, Required: []string{"operator", "types"}},
	}
	root.Defs = map[string]*jsonschema.Schema{
		"reviewLeaf":          leaf,
		"reviewLeafNode":      reviewObject(map[string]*jsonschema.Schema{"leaf": schemaRef("reviewLeaf")}, "leaf"),
		"reviewChildGroup":    reviewGroup(schemaRef("reviewLeafNode")),
		"reviewRootPredicate": reviewGroup(&jsonschema.Schema{OneOf: []*jsonschema.Schema{schemaRef("reviewLeafNode"), schemaRef("reviewChildGroup")}}),
	}
	properties["spec"] = reviewObject(map[string]*jsonschema.Schema{
		"specVersion":         {Type: "integer", Enum: []any{domain.ReviewSpecVersion}},
		"metricPolicyVersion": {Type: "string", Enum: []any{domain.ReviewMetricPolicyVersion}},
		"evaluator":           {Type: "string", Enum: []any{domain.ReviewEvaluator}},
		"target":              target, "predicate": schemaRef("reviewRootPredicate"), "schedule": schedule,
		"triggerPolicy": trigger, "dataPolicy": data,
	}, "specVersion", "metricPolicyVersion", "evaluator", "target", "predicate", "schedule", "triggerPolicy", "dataPolicy")
}

func reviewSchemaFor[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	return s
}
func reviewInt(n int) *int            { return &n }
func reviewNumber(n float64) *float64 { return &n }
func reviewObject(properties map[string]*jsonschema.Schema, required ...string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", Properties: properties, Required: required, AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}}
}
func reviewGroup(item *jsonschema.Schema) *jsonschema.Schema {
	array := &jsonschema.Schema{Type: "array", MinItems: reviewInt(1), MaxItems: reviewInt(20), Items: item}
	s := reviewObject(map[string]*jsonschema.Schema{"all": array, "any": array})
	s.OneOf = []*jsonschema.Schema{{Required: []string{"all"}}, {Required: []string{"any"}}}
	return s
}
