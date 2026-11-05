package mcpserver

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Discovered input schemas must not use a nullable type union. Optional
// pointers and slices omit the field. Required pointers and the instrument
// batch note keep null through anyOf.
func TestDiscoveredInputSchemasHaveNoNullTypeUnion(t *testing.T) {
	s, _, _ := fixture(t)
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		for _, definition := range s.toolRegistry(mode).Definitions() {
			raw, err := jsoncodec.Marshal(definition.InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			if inputSchemaHasNullUnion(raw) {
				t.Errorf("%s %s input schema still has a null type union", mode, definition.Name)
			}
		}
	}
}

func TestNullableInputSchemaDecisions(t *testing.T) {
	fx := newLedgerFixture(t)
	registry := fx.service.toolRegistry(LedgerWrite)
	accountIDs := property(t, registry, "get_account_valuations", "accountIds")
	if accountIDs["type"] != "array" {
		t.Fatalf("accountIds = %#v", accountIDs)
	}
	sessionIDs := property(t, registry, "preview_reconciliation_session", "accountIds")
	if sessionIDs["type"] != "array" {
		t.Fatalf("reconciliation accountIds = %#v", sessionIDs)
	}
	note := property(t, registry, "update_account", "input", "changes", "note")
	if note["type"] != "string" || note["anyOf"] != nil {
		t.Fatalf("update_account note = %#v, want a plain string", note)
	}
	if _, ok := property(t, registry, "update_account", "input", "changes")["properties"].(map[string]any)["noteSet"]; !ok {
		t.Fatal("update_account noteSet disappeared")
	}
	shortName := property(t, registry, "preview_product_terms", "terms", "shortName")
	if !schemaAllowsNull(shortName) || !schemaAllowsType(shortName, "string") {
		t.Fatalf("terms.shortName = %#v, want string or null", shortName)
	}
	policyNote := property(t, registry, "preview_product_terms", "policy", "note")
	if !schemaAllowsNull(policyNote) || !schemaAllowsType(policyNote, "string") {
		t.Fatalf("policy.note = %#v, want required string or null", policyNote)
	}
	batchNote := property(t, registry, "update_instruments_batch", "input", "items", "[]", "changes", "note")
	if !schemaAllowsNull(batchNote) || !schemaAllowsType(batchNote, "string") {
		t.Fatalf("batch note = %#v, want string or null", batchNote)
	}

	_, err := registry.Call(t.Context(), "update_account", json.RawMessage(`{"operationId":"00000000-0000-4000-8000-000000000001","input":{"id":"00000000-0000-4000-8000-000000000002","changes":{"note":null,"noteSet":true}}}`))
	if !errors.Is(err, agentcapability.ErrInvalidArguments) {
		t.Fatalf("null note with noteSet error=%v, want invalid arguments", err)
	}
	_, err = registry.Call(t.Context(), "update_account", json.RawMessage(`{"operationId":"00000000-0000-4000-8000-000000000001","input":{"id":"00000000-0000-4000-8000-000000000002","changes":{"noteSet":true}}}`))
	if errors.Is(err, agentcapability.ErrInvalidArguments) {
		t.Fatal("omitted note with noteSet was rejected; Set is the clear signal")
	}
}

func property(t *testing.T, registry *agentcapability.Registry, tool string, path ...string) map[string]any {
	t.Helper()
	definition, err := registry.Definition(tool)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jsoncodec.Marshal(definition.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var node any
	if err := jsoncodec.Unmarshal(raw, &node); err != nil {
		t.Fatal(err)
	}
	for _, key := range path {
		object, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("%s %s: %#v is not an object", tool, strings.Join(path, "."), node)
		}
		if key == "[]" {
			node = object["items"]
			continue
		}
		props, _ := object["properties"].(map[string]any)
		node = props[key]
		if node == nil {
			t.Fatalf("%s missing %s", tool, strings.Join(path, "."))
		}
	}
	object, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("%s %s: %#v", tool, strings.Join(path, "."), node)
	}
	return object
}

func schemaAllowsNull(schema map[string]any) bool {
	return schemaAllowsType(schema, "null")
}

func schemaAllowsType(schema map[string]any, kind string) bool {
	if schema["type"] == kind {
		return true
	}
	branches, _ := schema["anyOf"].([]any)
	for _, branch := range branches {
		object, _ := branch.(map[string]any)
		if object["type"] == kind {
			return true
		}
	}
	return false
}

func inputSchemaHasNullUnion(raw []byte) bool {
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
