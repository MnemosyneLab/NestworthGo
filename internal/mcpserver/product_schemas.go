package mcpserver

import "github.com/google/jsonschema-go/jsonschema"

func productTermsSchema(s *jsonschema.Schema) {
	setSchemaEnum(s.Properties["kind"], []string{"term_deposit", "locked_product"})
	setSchemaEnum(s.Properties["interestMode"], []string{"none", "manual_maturity_amount", "simple_act_365", "simple_act_360"})
	s.AllOf = append(s.AllOf, &jsonschema.Schema{If: propertyEquals("kind", "term_deposit"), Then: nonNullProperties("maturityOn")})
}
func nullableEnum(s *jsonschema.Schema, values ...string) {
	setSchemaEnum(s, values)
	s.Enum = append(s.Enum, nil)
}
func propertyEquals(name string, value any) *jsonschema.Schema {
	return &jsonschema.Schema{Required: []string{name}, Properties: map[string]*jsonschema.Schema{name: {Enum: []any{value}}}}
}
func nonNullProperties(names ...string) *jsonschema.Schema {
	s := &jsonschema.Schema{Required: names, Properties: map[string]*jsonschema.Schema{}}
	for _, name := range names {
		s.Properties[name] = &jsonschema.Schema{Not: &jsonschema.Schema{Type: "null"}}
	}
	return s
}
func productPolicySchema(s *jsonschema.Schema, managed bool) {
	access := []string{"on_request", "on_date", "unknown", "excluded"}
	early := []string{"not_allowed", "allowed", "unknown"}
	// Managed deposit construction supplies policy defaults for empty selectors.
	if managed {
		access = append(access, "")
		early = append(early, "")
	}
	setSchemaEnum(s.Properties["accessKind"], access)
	setSchemaEnum(s.Properties["earlyKind"], early)
	for _, name := range []string{"dayBasis", "earlyDayBasis"} {
		nullableEnum(s.Properties[name], "calendar", "weekdays", "")
	}
	nullableEnum(s.Properties["earlyAmountMode"], "current_value", "fixed_gross", "")
	low, high := float64(0), float64(365)
	for _, name := range []string{"settlementDays", "earlySettlementDays"} {
		s.Properties[name].Minimum = &low
		s.Properties[name].Maximum = &high
	}
	if !managed {
		s.AllOf = append(s.AllOf, &jsonschema.Schema{If: propertyEquals("accessKind", "on_date"), Then: nonNullProperties("unlockOn")})
	}
	s.AllOf = append(s.AllOf,
		&jsonschema.Schema{If: nonNullProperties("settlementDays"), Then: requiredEnum("dayBasis", "calendar", "weekdays")},
		&jsonschema.Schema{If: propertyEquals("earlyKind", "allowed"), Then: &jsonschema.Schema{AllOf: []*jsonschema.Schema{nonNullProperties("earlySettlementDays", "earlyFee"), requiredEnum("earlyDayBasis", "calendar", "weekdays"), requiredEnum("earlyAmountMode", "current_value", "fixed_gross")}}},
		&jsonschema.Schema{If: &jsonschema.Schema{AllOf: []*jsonschema.Schema{propertyEquals("earlyKind", "allowed"), propertyEquals("earlyAmountMode", "fixed_gross")}}, Then: nonNullProperties("earlyGrossAmount")},
	)
}
func productTimeSchema(s *jsonschema.Schema) {
	// effectiveAt remains a required string for existing product DTOs; empty
	// means frozen now, or selects the supplied local date/time pair.
	// Empty local fields are the DTO default, not a second time source.
	for _, pair := range [][2]string{{"effectiveLocalDate", "effectiveLocalTime"}, {"effectiveLocalTime", "effectiveLocalDate"}} {
		s.AllOf = append(s.AllOf, &jsonschema.Schema{
			If:   &jsonschema.Schema{Required: []string{pair[0]}, Properties: map[string]*jsonschema.Schema{pair[0]: {Not: &jsonschema.Schema{Enum: []any{""}}}}},
			Then: &jsonschema.Schema{Required: []string{pair[1]}, Properties: map[string]*jsonschema.Schema{pair[1]: {MinLength: intPointer(1)}, "effectiveAt": {Enum: []any{""}}}},
		})
	}
}
func intPointer(v int) *int { return &v }

func requiredEnum(name string, values ...string) *jsonschema.Schema {
	p := &jsonschema.Schema{}
	setSchemaEnum(p, values)
	return &jsonschema.Schema{Required: []string{name}, Properties: map[string]*jsonschema.Schema{name: p}}
}
