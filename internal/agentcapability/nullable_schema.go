package agentcapability

import (
	"slices"

	"github.com/google/jsonschema-go/jsonschema"
)

// normalizeNullableInputSchema rewrites jsonschema-go's nullable type unions.
// Inspector warns on type arrays such as ["null","array"]. Optional pointers
// and slices decode JSON null the same as omission, so those fields publish a
// single type and callers omit them. Required pointers keep null, because
// omission fails required and null is the explicit unknown or empty value.
// Arrays never keep null: a required list must be an array, and an optional
// list's null is the same nil slice as omission. An optional pointer named
// shortName still publishes null: preview_product_terms treats omitted and
// null as keep, and an empty string as clear. Patch tools do not use that
// pointer. They use PatchString, where null is a distinct clear. Do not add
// another optional shortName pointer for a patch tool.
func normalizeNullableInputSchema(schema *jsonschema.Schema) {
	walkNullableSchema(schema, false, "")
}

func walkNullableSchema(schema *jsonschema.Schema, optional bool, name string) {
	if schema == nil {
		return
	}
	if branch := rewriteNullableUnion(schema, optional, name); branch != nil {
		walkNullableChildren(branch)
		return
	}
	walkNullableChildren(schema)
}

func walkNullableChildren(schema *jsonschema.Schema) {
	required := make(map[string]bool, len(schema.Required))
	for _, field := range schema.Required {
		required[field] = true
	}
	for field, property := range schema.Properties {
		walkNullableSchema(property, !required[field], field)
	}
	for _, property := range schema.PatternProperties {
		walkNullableSchema(property, false, "")
	}
	for _, property := range schema.DependentSchemas {
		walkNullableSchema(property, false, "")
	}
	for _, property := range schema.DependencySchemas {
		walkNullableSchema(property, false, "")
	}
	for _, property := range schema.Defs {
		walkNullableSchema(property, false, "")
	}
	for _, property := range schema.Definitions {
		walkNullableSchema(property, false, "")
	}
	walkNullableSchema(schema.Items, false, "")
	walkNullableSchema(schema.AdditionalProperties, false, "")
	walkNullableSchema(schema.AdditionalItems, false, "")
	walkNullableSchema(schema.Contains, false, "")
	walkNullableSchema(schema.PropertyNames, false, "")
	walkNullableSchema(schema.UnevaluatedItems, false, "")
	walkNullableSchema(schema.UnevaluatedProperties, false, "")
	walkNullableSchema(schema.ContentSchema, false, "")
	walkNullableSchema(schema.Not, false, "")
	walkNullableSchema(schema.If, false, "")
	walkNullableSchema(schema.Then, false, "")
	walkNullableSchema(schema.Else, false, "")
	for _, child := range schema.PrefixItems {
		walkNullableSchema(child, false, "")
	}
	for _, child := range schema.ItemsArray {
		walkNullableSchema(child, false, "")
	}
	for _, child := range schema.AllOf {
		walkNullableSchema(child, false, "")
	}
	for _, child := range schema.AnyOf {
		walkNullableSchema(child, false, "")
	}
	for _, child := range schema.OneOf {
		walkNullableSchema(child, false, "")
	}
}

// rewriteNullableUnion returns the non-null branch when null remains expressible
// through anyOf. A nil return means the node itself should be walked.
func rewriteNullableUnion(schema *jsonschema.Schema, optional bool, name string) *jsonschema.Schema {
	if len(schema.Types) == 0 || !slices.Contains(schema.Types, "null") {
		return nil
	}
	others := make([]string, 0, len(schema.Types)-1)
	for _, kind := range schema.Types {
		if kind != "null" {
			others = append(others, kind)
		}
	}
	if len(others) != 1 {
		return splitNullableUnion(schema, others)
	}
	concrete := others[0]
	keepNull := name == "shortName" || (len(schema.Enum) > 0 && enumContainsNull(schema.Enum))
	if concrete == "array" || (optional && !keepNull) || (len(schema.Enum) > 0 && !enumContainsNull(schema.Enum)) {
		schema.Type = concrete
		schema.Types = nil
		return nil
	}
	return splitNullableUnion(schema, others)
}

func splitNullableUnion(schema *jsonschema.Schema, concrete []string) *jsonschema.Schema {
	branch := schema.CloneSchemas()
	branch.Title = ""
	branch.Description = ""
	branch.Deprecated = false
	branch.ReadOnly = false
	branch.WriteOnly = false
	branch.Examples = nil
	branch.Default = nil
	if len(concrete) == 1 {
		branch.Type = concrete[0]
		branch.Types = nil
	} else {
		branch.Type = ""
		branch.Types = append([]string(nil), concrete...)
	}
	if enumContainsNull(branch.Enum) {
		kept := make([]any, 0, len(branch.Enum))
		for _, value := range branch.Enum {
			if !isJSONNull(value) {
				kept = append(kept, value)
			}
		}
		branch.Enum = kept
	}
	replacement := jsonschema.Schema{
		Title:       schema.Title,
		Description: schema.Description,
		Deprecated:  schema.Deprecated,
		ReadOnly:    schema.ReadOnly,
		WriteOnly:   schema.WriteOnly,
		Examples:    schema.Examples,
		Default:     schema.Default,
		AnyOf:       []*jsonschema.Schema{branch, {Type: "null"}},
	}
	*schema = replacement
	return branch
}

func enumContainsNull(values []any) bool {
	for _, value := range values {
		if isJSONNull(value) {
			return true
		}
	}
	return false
}

func isJSONNull(value any) bool {
	return value == nil
}
