package mcpserver

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func accountNote(t *testing.T, c *mcp.ClientSession, id string) string {
	t.Helper()
	list := scaleCapture(t, c, "list_accounts", map[string]any{})
	item := scaleCapture(t, c, "get_read_item", map[string]any{"captureId": list["captureId"], "section": "accounts", "id": id})
	account := item["summary"].(map[string]any)["account"].(map[string]any)
	note, _ := account["note"].(string)
	return note
}

// update_account keeps every omitted field. A nullable field is cleared only by
// its <field>Set flag; an empty value alone keeps the stored one. The tool
// description must say so, because it is the one place an agent reads it.
func TestUpdateAccountClearRequiresSetFlag(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"note": "kept", "noteSet": true}})
	if accountNote(t, c, fx.checking) != "kept" {
		t.Fatal("note was not set")
	}
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"name": "Renamed"}})
	if accountNote(t, c, fx.checking) != "kept" {
		t.Fatal("an omitted note was not preserved")
	}
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"note": ""}})
	if accountNote(t, c, fx.checking) != "kept" {
		t.Fatal("an empty note without noteSet must keep the stored note")
	}
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"note": "", "noteSet": true}})
	if accountNote(t, c, fx.checking) != "" {
		t.Fatal("noteSet=true with an empty note must clear it")
	}
	definition, err := fx.service.toolRegistry(LedgerWrite).Definition("update_account")
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"omitted fields are preserved", "Set=true", "note"} {
		if !strings.Contains(definition.Description, phrase) {
			t.Errorf("update_account description lacks %q: %s", phrase, definition.Description)
		}
	}
	if strings.Contains(definition.Description, "Empty string clears") {
		t.Error("update_account description still claims an empty string clears a field")
	}
}

// Write tools follow one of a few named update conventions. The table below is
// the audited inventory (see docs/development/mcp-update-conventions.md); a new
// update_*/set_* tool or versioned replacement preview must be classified here
// and match its convention's schema shape, so an omitted field cannot silently
// mean "keep" in one tool and "clear" in another unnoticed.
var updateConventions = map[string]string{
	"update_member":                     "patch_at_least_one",
	"update_institution":                "patch_at_least_one",
	"update_group":                      "patch_at_least_one",
	"update_account":                    "patch_set_flags",
	"update_instrument":                 "patch_replace_flag",
	"update_instruments_batch":          "patch_versioned_batch",
	"update_investment_review_rule":     "revisioned_replacement",
	"preview_save_liquidity_policy":     "versioned_full_replacement",
	"preview_product_terms":             "versioned_full_replacement",
	"set_fx_source":                     "selection",
	"set_investment_review_suppression": "selection",
}

// Audited exceptions inside a full replacement. terms.shortName is optional:
// omitted or null keeps the stored value and an empty string clears it, which the
// tool description states.
var optionalReplacementFields = map[string]bool{"preview_product_terms.terms.shortName": true}

func schemaObject(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := jsoncodec.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := jsoncodec.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func schemaChild(t *testing.T, node map[string]any, keys ...string) map[string]any {
	t.Helper()
	for _, key := range keys {
		props, _ := node["properties"].(map[string]any)
		next, ok := props[key].(map[string]any)
		if !ok {
			if key == "[]" {
				next, ok = node["items"].(map[string]any)
			}
			if !ok {
				t.Fatalf("schema has no %q under %v", key, node)
			}
		}
		node = next
	}
	return node
}

func requiredSet(node map[string]any) map[string]bool {
	out := map[string]bool{}
	list, _ := node["required"].([]any)
	for _, v := range list {
		out[v.(string)] = true
	}
	return out
}

func propertyNames(node map[string]any) []string {
	props, _ := node["properties"].(map[string]any)
	names := make([]string, 0, len(props))
	for k := range props {
		names = append(names, k)
	}
	return names
}

func TestWriteToolUpdateConventionsAreClassifiedAndMatchSchemas(t *testing.T) {
	fx := newLedgerFixture(t)
	registry := fx.service.toolRegistry(LedgerWrite)
	for _, d := range registry.Definitions() {
		covered := strings.HasPrefix(d.Name, "update_") || strings.HasPrefix(d.Name, "set_") || d.Name == "preview_save_liquidity_policy" || d.Name == "preview_product_terms"
		if _, ok := updateConventions[d.Name]; covered && !ok {
			t.Errorf("%s is an update-style tool without an audited convention; add it to updateConventions and the audit document", d.Name)
		}
	}
	for name, convention := range updateConventions {
		d, err := registry.Definition(name)
		if err != nil {
			t.Fatalf("classified tool %s is not registered", name)
		}
		root := schemaObject(t, d.InputSchema)
		input := root
		if _, wrapped := schemaChild2(root, "input"); wrapped {
			input = schemaChild(t, root, "input")
		}
		req := requiredSet(input)
		switch convention {
		case "patch_at_least_one":
			if !req["id"] || req["name"] || req["iconKey"] {
				t.Errorf("%s: only id is required; name and iconKey are optional patch fields", name)
			}
			for _, field := range []string{"name", "iconKey"} {
				if _, ok := input["properties"].(map[string]any)[field]; !ok {
					t.Errorf("%s: missing patch field %s", name, field)
				}
			}
		case "patch_set_flags":
			changes := schemaChild(t, input, "changes")
			if len(requiredSet(changes)) != 0 {
				t.Errorf("%s: patch fields must all be optional", name)
			}
			for _, field := range []string{"institutionId", "groupId", "note", "iconKey", "openedOn", "closedOn"} {
				if _, ok := changes["properties"].(map[string]any)[field+"Set"]; !ok {
					t.Errorf("%s: nullable field %s lacks its %sSet clear flag", name, field, field)
				}
			}
		case "patch_replace_flag":
			changes := schemaChild(t, input, "changes")
			if len(requiredSet(changes)) != 0 {
				t.Errorf("%s: patch fields must all be optional", name)
			}
			if schemaChild(t, changes, "replace")["type"] != "boolean" {
				t.Errorf("%s: replace must be a boolean", name)
			}
		case "patch_versioned_batch":
			row := schemaChild(t, input, "items", "[]")
			if !requiredSet(row)["expectedMetadataVersion"] || !requiredSet(row)["changes"] {
				t.Errorf("%s: every row needs expectedMetadataVersion and changes", name)
			}
			if schemaChild(t, row, "changes")["minProperties"] != float64(1) {
				t.Errorf("%s: an empty changes object must be rejected", name)
			}
		case "revisioned_replacement":
			for _, field := range []string{"ruleId", "expectedRevision", "name", "enabled", "severity", "spec"} {
				if !req[field] {
					t.Errorf("%s: %s must be required", name, field)
				}
			}
		case "versioned_full_replacement":
			if !req["expectedRevision"] {
				t.Errorf("%s: expectedRevision must be required", name)
			}
			for _, group := range []string{"policy", "terms"} {
				props, _ := input["properties"].(map[string]any)
				if _, ok := props[group]; !ok {
					continue
				}
				obj := schemaChild(t, input, group)
				if !req[group] {
					t.Errorf("%s: %s must be required", name, group)
				}
				for _, field := range propertyNames(obj) {
					if optionalReplacementFields[name+"."+group+"."+field] {
						continue
					}
					if !requiredSet(obj)[field] {
						t.Errorf("%s.%s.%s must be required: omission never keeps the stored value", name, group, field)
					}
				}
			}
		case "selection":
			if name == "preview_product_terms" {
				t.Fatal("misclassified")
			}
		default:
			t.Fatalf("unknown convention %q for %s", convention, name)
		}
	}
}

func schemaChild2(node map[string]any, key string) (map[string]any, bool) {
	props, _ := node["properties"].(map[string]any)
	child, ok := props[key].(map[string]any)
	return child, ok
}

func TestProductTermsDescriptionStatesOptionalShortName(t *testing.T) {
	fx := newLedgerFixture(t)
	d, err := fx.service.toolRegistry(LedgerWrite).Definition("preview_product_terms")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Description, "terms.shortName is the only optional field") {
		t.Fatal("preview_product_terms must state its one optional field")
	}
}
