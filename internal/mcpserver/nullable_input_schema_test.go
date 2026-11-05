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
// pointers and slices omit the field. Required pointers, product-terms
// shortName, and patch fields that null clears keep null through anyOf.
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
	if !schemaAllowsNull(note) || !schemaAllowsType(note, "string") || note["type"] != nil {
		t.Fatalf("update_account note = %#v, want string or null", note)
	}
	if _, ok := property(t, registry, "update_account", "input", "changes")["properties"].(map[string]any)["noteSet"]; ok {
		t.Fatal("update_account noteSet is still published")
	}
	shortName := property(t, registry, "preview_product_terms", "terms", "shortName")
	if !schemaAllowsNull(shortName) || !schemaAllowsType(shortName, "string") {
		t.Fatalf("terms.shortName = %#v, want string or null", shortName)
	}
	policyNote := property(t, registry, "preview_product_terms", "policy", "note")
	if !schemaAllowsNull(policyNote) || !schemaAllowsType(policyNote, "string") {
		t.Fatalf("policy.note = %#v, want required string or null", policyNote)
	}
	batchShortName := property(t, registry, "update_instruments_batch", "input", "items", "[]", "changes", "shortName")
	if !schemaAllowsNull(batchShortName) || !schemaAllowsType(batchShortName, "string") {
		t.Fatalf("batch shortName = %#v, want string or null", batchShortName)
	}
	batchNote := property(t, registry, "update_instruments_batch", "input", "items", "[]", "changes", "note")
	if !schemaAllowsNull(batchNote) || !schemaAllowsType(batchNote, "string") {
		t.Fatalf("batch note = %#v, want string or null", batchNote)
	}
	batchName := property(t, registry, "update_instruments_batch", "input", "items", "[]", "changes", "name")
	if schemaAllowsNull(batchName) {
		t.Fatalf("batch name = %#v, want a string that rejects null", batchName)
	}

	_, err := registry.Call(t.Context(), "update_account", json.RawMessage(`{"operationId":"00000000-0000-4000-8000-000000000001","input":{"id":"00000000-0000-4000-8000-000000000002","changes":{"note":null,"noteSet":true}}}`))
	if !errors.Is(err, agentcapability.ErrInvalidArguments) {
		t.Fatalf("noteSet error=%v, want invalid arguments", err)
	}
	_, err = registry.Call(t.Context(), "update_account", json.RawMessage(`{"operationId":"00000000-0000-4000-8000-000000000003","input":{"id":"00000000-0000-4000-8000-000000000002","changes":{"note":null}}}`))
	if errors.Is(err, agentcapability.ErrInvalidArguments) {
		t.Fatal("null note was rejected by the schema; it is the clear signal")
	}
	_, err = registry.Call(t.Context(), "update_instrument", json.RawMessage(`{"operationId":"00000000-0000-4000-8000-000000000004","input":{"id":"00000000-0000-4000-8000-000000000002","changes":{"replace":true}}}`))
	if !errors.Is(err, agentcapability.ErrInvalidArguments) {
		t.Fatalf("replace error=%v, want invalid arguments", err)
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
