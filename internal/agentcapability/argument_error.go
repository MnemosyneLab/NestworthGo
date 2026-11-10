package agentcapability

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// ArgumentError contains only schema-owned names and constraints, never submitted
// values or validator error strings. It is safe across both transport adapters.
type ArgumentError struct {
	Field    string   `json:"field,omitempty"`
	Rule     string   `json:"rule"`
	Expected []string `json:"expected,omitempty"`
}

func (e ArgumentError) Error() string { return "invalid capability arguments" }

// Message describes only schema-owned paths and constraints, never input values.
func (e ArgumentError) Message() string {
	if e.Rule == "typed_decode" {
		return "Arguments must fit the declared field types and numeric ranges; integer fields require integer notation."
	}
	if e.Field != "" && e.Rule == "enum" && len(e.Expected) > 0 {
		return fmt.Sprintf("Field %s must be one of: %s.", e.Field, strings.Join(e.Expected, ", "))
	}
	if e.Field != "" && e.Rule == "type" && len(e.Expected) > 0 {
		return fmt.Sprintf("Field %s must have JSON type %s.", e.Field, strings.Join(e.Expected, " or "))
	}
	if e.Field != "" {
		return fmt.Sprintf("Field %s violates the tool schema (%s).", e.Field, e.Rule)
	}
	return "Arguments violate the declared tool contract; correct the indicated field or combination."
}
func (e ArgumentError) Unwrap() error { return ErrInvalidArguments }

func argumentError(schema any, value any) error {
	raw, _ := jsoncodec.Marshal(schema)
	var s jsonschema.Schema
	if jsoncodec.Unmarshal(raw, &s) != nil {
		return ErrInvalidArguments
	}
	if e := explainArgument(&s, &s, value, "", 0); e != nil {
		return *e
	}
	return ArgumentError{Rule: "schema_mismatch"}
}
func schemaAccepts(s *jsonschema.Schema, value any) bool {
	r, err := s.Resolve(nil)
	return err == nil && r.Validate(&value) == nil
}
func explainArgument(root, s *jsonschema.Schema, value any, path string, depth int) *ArgumentError {
	if s == nil || depth > 24 {
		return nil
	}
	if strings.HasPrefix(s.Ref, "#/$defs/") {
		return explainArgument(root, root.Defs[strings.TrimPrefix(s.Ref, "#/$defs/")], value, path, depth+1)
	}
	if schemaAcceptsRoot(root, s, value) {
		return nil
	}
	issue := func(rule string, expected ...string) *ArgumentError {
		return &ArgumentError{Field: path, Rule: rule, Expected: expected}
	}
	if len(s.Enum) > 0 && !schemaAcceptsRoot(root, &jsonschema.Schema{Enum: s.Enum}, value) {
		expected := []string{}
		for _, v := range s.Enum {
			if text, ok := v.(string); ok {
				expected = append(expected, text)
			}
		}
		return issue("enum", expected...)
	}
	if len(s.Types) > 0 && !schemaAcceptsRoot(root, &jsonschema.Schema{Types: s.Types}, value) {
		return issue("type", s.Types...)
	}
	if s.Type != "" && !schemaAcceptsRoot(root, &jsonschema.Schema{Type: s.Type}, value) {
		return issue("type", s.Type)
	}
	// Evaluate constraints independently so diagnostics contain only the
	// schema keyword and declared bounds, never validator text or input values.
	constraints := []struct {
		rule   string
		schema *jsonschema.Schema
		bound  any
	}{
		{"minimum", &jsonschema.Schema{Minimum: s.Minimum}, s.Minimum},
		{"maximum", &jsonschema.Schema{Maximum: s.Maximum}, s.Maximum},
		{"exclusiveMinimum", &jsonschema.Schema{ExclusiveMinimum: s.ExclusiveMinimum}, s.ExclusiveMinimum},
		{"exclusiveMaximum", &jsonschema.Schema{ExclusiveMaximum: s.ExclusiveMaximum}, s.ExclusiveMaximum},
		{"minLength", &jsonschema.Schema{MinLength: s.MinLength}, s.MinLength},
		{"maxLength", &jsonschema.Schema{MaxLength: s.MaxLength}, s.MaxLength},
		{"minItems", &jsonschema.Schema{MinItems: s.MinItems}, s.MinItems},
		{"maxItems", &jsonschema.Schema{MaxItems: s.MaxItems}, s.MaxItems},
		{"pattern", &jsonschema.Schema{Pattern: s.Pattern}, s.Pattern},
		{"multipleOf", &jsonschema.Schema{MultipleOf: s.MultipleOf}, s.MultipleOf},
		{"uniqueItems", &jsonschema.Schema{UniqueItems: s.UniqueItems}, s.UniqueItems},
	}
	for _, constraint := range constraints {
		if !schemaAcceptsRoot(root, constraint.schema, value) {
			raw, _ := jsoncodec.Marshal(constraint.bound)
			return issue(constraint.rule, string(raw))
		}
	}
	for _, branch := range s.AllOf {
		if e := explainArgument(root, branch, value, path, depth+1); e != nil {
			return e
		}
	}
	if s.If != nil {
		branch := s.Else
		if schemaAcceptsRoot(root, s.If, value) {
			branch = s.Then
		}
		if e := explainArgument(root, branch, value, path, depth+1); e != nil {
			return e
		}
	}
	if len(s.AnyOf) > 0 {
		expectedTypes := []string{}
		allTyped, matchedType := true, false
		for _, branch := range s.AnyOf {
			candidate := branch
			if strings.HasPrefix(branch.Ref, "#/$defs/") {
				candidate = root.Defs[strings.TrimPrefix(branch.Ref, "#/$defs/")]
			}
			if candidate == nil || (candidate.Type == "" && len(candidate.Types) == 0) {
				allTyped = false
			} else {
				if candidate.Type != "" {
					expectedTypes = append(expectedTypes, candidate.Type)
				} else {
					expectedTypes = append(expectedTypes, candidate.Types...)
				}
			}
			if candidate != nil && (candidate.Type != "" || len(candidate.Types) > 0) && schemaAcceptsRoot(root, &jsonschema.Schema{Type: candidate.Type, Types: candidate.Types}, value) {
				matchedType = true
				if e := explainArgument(root, branch, value, path, depth+1); e != nil {
					return e
				}
			}
		}
		if allTyped && !matchedType {
			return issue("type", expectedTypes...)
		}
		return issue("any_of")
	}
	if object, ok := value.(map[string]any); ok {
		for _, name := range s.Required {
			if _, exists := object[name]; !exists {
				return &ArgumentError{Field: joinArgumentPath(path, name), Rule: "required"}
			}
		}
		keys := make([]string, 0, len(s.Properties))
		for name := range s.Properties {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			if v, exists := object[name]; exists {
				if e := explainArgument(root, s.Properties[name], v, joinArgumentPath(path, name), depth+1); e != nil {
					return e
				}
			}
		}
		deps := make([]string, 0, len(s.DependentRequired))
		for name := range s.DependentRequired {
			deps = append(deps, name)
		}
		sort.Strings(deps)
		for _, name := range deps {
			if _, exists := object[name]; exists {
				for _, need := range s.DependentRequired[name] {
					if _, present := object[need]; !present {
						return &ArgumentError{Field: joinArgumentPath(path, need), Rule: "required_with", Expected: []string{name}}
					}
				}
			}
		}
		// Only diagnose a discriminator when every union branch requires it
		// and declares string enum values. Partial unions retain one_of.
		for _, key := range []string{"kind", "action"} {
			expected := []string{}
			complete, matched := len(s.OneOf) > 0, false
			for _, branch := range s.OneOf {
				p := branch.Properties[key]
				required := false
				for _, name := range branch.Required {
					required = required || name == key
				}
				if !required || p == nil || len(p.Enum) == 0 {
					complete = false
					break
				}
				for _, v := range p.Enum {
					text, ok := v.(string)
					if !ok {
						complete = false
						break
					}
					expected = append(expected, text)
				}
				matched = matched || schemaAcceptsRoot(root, p, object[key])
			}
			if complete && !matched {
				field := joinArgumentPath(path, key)
				if _, exists := object[key]; !exists {
					return &ArgumentError{Field: field, Rule: "required"}
				}
				sort.Strings(expected)
				return &ArgumentError{Field: field, Rule: "enum", Expected: expected}
			}
		}
		// Choose only a schema-owned discriminator; do not guess a union branch.
		for _, branch := range s.OneOf {
			for _, key := range []string{"kind", "action"} {
				if p := branch.Properties[key]; p != nil && len(p.Enum) > 0 && schemaAcceptsRoot(root, p, object[key]) {
					if e := explainArgument(root, branch, value, path, depth+1); e != nil {
						return e
					}
				}
			}
		}
		if s.PropertyNames != nil {
			for name := range object {
				if !schemaAcceptsRoot(root, s.PropertyNames, name) {
					return issue("unexpected_field")
				}
			}
		}
		if s.PropertyNames != nil && len(s.PropertyNames.Enum) > 0 && s.AdditionalProperties != nil {
			for _, known := range s.PropertyNames.Enum {
				if name, ok := known.(string); ok && s.Properties[name] == nil {
					if v, present := object[name]; present {
						if e := explainArgument(root, s.AdditionalProperties, v, joinArgumentPath(path, name), depth+1); e != nil {
							return e
						}
					}
				}
			}
		}
		if s.AdditionalProperties != nil && s.AdditionalProperties.Not != nil {
			for name := range object {
				if s.Properties[name] == nil {
					return issue("unexpected_field")
				}
			}
		}
	}
	if array, ok := value.([]any); ok {
		for i, v := range array {
			if e := explainArgument(root, s.Items, v, fmt.Sprintf("%s[%d]", path, i), depth+1); e != nil {
				return e
			}
		}
	}
	if s.Not != nil && schemaAcceptsRoot(root, s.Not, value) {
		return issue("forbidden_combination")
	}
	if len(s.OneOf) > 0 {
		matches := 0
		for _, b := range s.OneOf {
			if schemaAcceptsRoot(root, b, value) {
				matches++
			}
		}
		if matches != 1 {
			return issue("one_of")
		}
	}
	return nil
}
func joinArgumentPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

func schemaAcceptsRoot(root, s *jsonschema.Schema, value any) bool {
	if s == nil {
		return true
	}
	// Most nodes are self-contained. Avoid resolving the whole definition
	// catalog for every scalar type or bound checked during error reporting.
	if resolved, err := s.Resolve(nil); err == nil {
		return resolved.Validate(&value) == nil
	}
	// A subtree may also occur inside root.Defs. Resolve requires a tree,
	// so detach its nodes before attaching definitions for local references.
	detached := s.CloneSchemas()
	if detached.Defs == nil {
		detached.Defs = root.Defs
	}
	return schemaAccepts(detached, value)
}
