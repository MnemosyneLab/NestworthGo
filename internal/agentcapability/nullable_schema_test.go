package agentcapability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestNullableInputSchemaDropsEquivalentNullAndKeepsDistinctNull(t *testing.T) {
	t.Parallel()
	type filter struct {
		ID string `json:"id,omitempty"`
	}
	type input struct {
		AccountIDs []string `json:"accountIds,omitempty"`
		Note       *string  `json:"note,omitempty"`
		ShortName  *string  `json:"shortName,omitempty"`
		Fee        *string  `json:"fee"`
		Days       *int     `json:"days"`
		Commands   []string `json:"commands"`
		Filter     *filter  `json:"filter,omitempty"`
	}
	registry := New("nullable")
	Add(registry, &Definition{Name: "read", Effect: Read}, func(context.Context, input) (input, error) {
		return input{}, nil
	})
	definition, err := registry.Definition("read")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jsoncodec.Marshal(definition.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if jsonNullUnion(raw) {
		t.Fatalf("input schema still has a null type union: %s", raw)
	}
	var schema map[string]any
	if err := jsoncodec.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	props := schema["properties"].(map[string]any)
	if props["accountIds"].(map[string]any)["type"] != "array" {
		t.Fatalf("optional slice: %#v", props["accountIds"])
	}
	if props["note"].(map[string]any)["type"] != "string" {
		t.Fatalf("optional pointer: %#v", props["note"])
	}
	if props["filter"].(map[string]any)["type"] != "object" {
		t.Fatalf("optional object: %#v", props["filter"])
	}
	if props["commands"].(map[string]any)["type"] != "array" {
		t.Fatalf("required slice: %#v", props["commands"])
	}
	for _, field := range []string{"fee", "days", "shortName"} {
		if !anyOfNull(props[field]) {
			t.Fatalf("%s must keep null through anyOf: %#v", field, props[field])
		}
	}

	accept := []string{
		`{"fee":null,"days":null,"commands":[]}`,
		`{"fee":"1.00","days":2,"commands":["a"],"shortName":null}`,
		`{"fee":"1.00","days":2,"commands":["a"],"accountIds":["x"],"note":"kept"}`,
	}
	reject := []string{
		`{"fee":null,"days":null,"commands":null}`,
		`{"fee":null,"days":null,"commands":[],"accountIds":null}`,
		`{"fee":null,"days":null,"commands":[],"note":null}`,
		`{"fee":null,"days":null,"commands":[],"filter":null}`,
		`{"days":null,"commands":[]}`,
		`{"fee":null,"commands":[]}`,
	}
	for _, payload := range accept {
		if _, err := registry.Call(t.Context(), "read", json.RawMessage(payload)); err != nil {
			t.Errorf("accepted payload rejected: %s: %v", payload, err)
		}
	}
	for _, payload := range reject {
		if _, err := registry.Call(t.Context(), "read", json.RawMessage(payload)); !errors.Is(err, ErrInvalidArguments) {
			t.Errorf("payload %s error=%v, want invalid arguments", payload, err)
		}
	}
}

func anyOfNull(node any) bool {
	schema, ok := node.(map[string]any)
	if !ok {
		return false
	}
	branches, ok := schema["anyOf"].([]any)
	if !ok {
		return false
	}
	var sawString, sawNull, sawInteger bool
	for _, branch := range branches {
		kind, _ := branch.(map[string]any)["type"].(string)
		switch kind {
		case "null":
			sawNull = true
		case "string":
			sawString = true
		case "integer":
			sawInteger = true
		}
	}
	return sawNull && (sawString || sawInteger)
}

func jsonNullUnion(raw []byte) bool {
	var value any
	if err := jsoncodec.Unmarshal(raw, &value); err != nil {
		return true
	}
	var walk func(any) bool
	walk = func(node any) bool {
		switch node := node.(type) {
		case map[string]any:
			if kinds, ok := node["type"].([]any); ok {
				for _, kind := range kinds {
					if kind == "null" {
						return true
					}
				}
			}
			for _, child := range node {
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range node {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(value)
}
