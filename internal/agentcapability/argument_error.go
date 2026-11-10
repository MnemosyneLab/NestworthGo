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
	issue := func(rule string, expected ...string) *ArgumentError {
		return &ArgumentError{Field: path, Rule: rule, Expected: expected}
	}
	if len(s.Enum) > 0 && !schemaAccepts(&jsonschema.Schema{Enum: s.Enum}, value) {
		expected := []string{}
		for _, v := range s.Enum {
			if text, ok := v.(string); ok {
				expected = append(expected, text)
			}
		}
		return issue("enum", expected...)
	}
	if s.Type != "" && !schemaAccepts(&jsonschema.Schema{Type: s.Type}, value) {
		return issue("type", s.Type)
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
		// Choose only a schema-owned discriminator; do not guess a union branch.
		for _, branch := range s.OneOf {
			for _, key := range []string{"kind", "action"} {
				if p := branch.Properties[key]; p != nil && len(p.Enum) > 0 && schemaAccepts(p, object[key]) {
					if e := explainArgument(root, branch, value, path, depth+1); e != nil {
						return e
					}
				}
			}
		}
		if s.PropertyNames != nil {
			for name := range object {
				if !schemaAccepts(s.PropertyNames, name) {
					return issue("unexpected_field")
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
	if s.Not != nil && schemaAccepts(s.Not, value) {
		return issue("forbidden_combination")
	}
	if len(s.OneOf) > 0 {
		matches := 0
		for _, b := range s.OneOf {
			if schemaAccepts(b, value) {
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
