package mcpserver

import (
	"sort"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// String fields share one schema. The kind oneOf stays a list of required
// names: copying each field schema into every branch would repeat it. Fields
// the handler rejects for every kind are not advertised.
func slimLedgerCommandSchema(s *jsonschema.Schema) {
	if s == nil || s.Properties == nil {
		return
	}
	for _, name := range []string{"added", "mutationId", "newValue", "newValueCurrency", "reconciliationLink"} {
		delete(s.Properties, name)
	}
	sharePlainStringProperties(s)
}

// sharePlainStringProperties replaces repeated {"type":"string"} properties
// with one string schema and an allowlist of names. Nullable, object and ref
// properties stay explicit, and required names are unchanged.
func sharePlainStringProperties(s *jsonschema.Schema) {
	if s == nil || len(s.Properties) == 0 {
		return
	}
	names := make([]string, 0, len(s.Properties))
	kept := make(map[string]*jsonschema.Schema)
	plain := 0
	for name, prop := range s.Properties {
		names = append(names, name)
		if plainStringSchema(prop) {
			plain++
			continue
		}
		kept[name] = prop
	}
	// Measured: an allowlist costs more than four {"type":"string"} copies,
	// because every property name is repeated. Larger groups still shrink.
	if plain < 4 {
		return
	}
	sort.Strings(names)
	if len(kept) == 0 {
		s.Properties = nil
	} else {
		s.Properties = kept
	}
	s.PropertyOrder = nil
	enums := make([]any, len(names))
	for i, name := range names {
		enums[i] = name
	}
	s.PropertyNames = &jsonschema.Schema{Enum: enums}
	s.AdditionalProperties = &jsonschema.Schema{Type: "string"}
}

func plainStringSchema(s *jsonschema.Schema) bool {
	raw, err := jsoncodec.Marshal(s)
	return err == nil && string(raw) == `{"type":"string"}`
}

// productOperationInputSchema publishes terms, policy and settle once. The
// kind oneOf names the required payload and does not copy those objects.
// The kind sentence stays in the tool description.
func productOperationInputSchema() *jsonschema.Schema {
	s, err := jsonschema.For[ProductOperationInput](nil)
	if err != nil {
		panic(err)
	}
	terms := nested(s, "open", "terms")
	policy := nested(s, "open", "policy")
	settle := nested(s, "renew", "settle")
	sameObject(terms, nested(s, "recordExisting", "terms"))
	sameObject(terms, nested(s, "renew", "terms"))
	sameObject(policy, nested(s, "recordExisting", "policy"))
	sameObject(policy, nested(s, "renew", "policy"))
	sameObject(settle, nested(s, "settle"))
	s.Defs = map[string]*jsonschema.Schema{
		"productPolicy": policy,
		"productSettle": settle,
		"productTerms":  terms,
	}
	for _, parent := range []string{"open", "recordExisting", "renew"} {
		props := s.Properties[parent].Properties
		props["terms"] = schemaRef("productTerms")
		props["policy"] = schemaRef("productPolicy")
	}
	s.Properties["renew"].Properties["settle"] = schemaRef("productSettle")
	s.Properties["settle"] = &jsonschema.Schema{AnyOf: []*jsonschema.Schema{{Type: "null"}, schemaRef("productSettle")}}
	s.OneOf = productKindOneOf()
	return s
}

func schemaRef(name string) *jsonschema.Schema {
	return &jsonschema.Schema{Ref: "#/$defs/" + name}
}

func nested(root *jsonschema.Schema, path ...string) *jsonschema.Schema {
	s := root
	for _, name := range path {
		if s == nil || s.Properties == nil || s.Properties[name] == nil {
			panic("product operation schema missing " + name)
		}
		s = s.Properties[name]
	}
	return s
}

func sameObject(a, b *jsonschema.Schema) {
	if canonicalObject(a) != canonicalObject(b) {
		panic("product operation schema shared object diverged")
	}
}

func canonicalObject(s *jsonschema.Schema) string {
	copy := *s
	copy.Type = "object"
	copy.Types = nil
	raw, err := jsoncodec.Marshal(&copy)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func productKindOneOf() []*jsonschema.Schema {
	kinds := []struct{ kind, field string }{
		{"open", "open"},
		{"record_existing", "recordExisting"},
		{"receive_interest", "receiveInterest"},
		{"settle", "settle"},
		{"renew", "renew"},
		{"undo", "undo"},
	}
	out := make([]*jsonschema.Schema, 0, len(kinds))
	for _, kind := range kinds {
		props := map[string]*jsonschema.Schema{"kind": {Enum: []any{kind.kind}}}
		for _, other := range kinds {
			if other.field != kind.field {
				// An empty Not publishes as false, so the other payload key is
				// rejected without copying its fields.
				props[other.field] = &jsonschema.Schema{Not: &jsonschema.Schema{}}
			}
		}
		out = append(out, &jsonschema.Schema{Properties: props, Required: []string{"kind", kind.field}})
	}
	return out
}
