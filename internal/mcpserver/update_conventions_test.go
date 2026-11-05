package mcpserver

import (
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// update_account keeps every omitted field. JSON null clears a clearable field.
// An empty string is rejected, so it neither clears nor keeps. Identity fields,
// inclusion flags and ownership cannot be cleared. A rejected call does not
// apply any other field sent with it.
func TestUpdateAccountPatchOmitsKeepAndNullClears(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"note": "kept"}})
	if stored := storedLedgerAccount(t, fx, fx.checking); stored.Note == nil || *stored.Note != "kept" {
		t.Fatalf("note was not set: %+v", stored)
	}
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"name": "Renamed"}})
	renamed := storedLedgerAccount(t, fx, fx.checking)
	if renamed.Name != "Renamed" || renamed.Note == nil || *renamed.Note != "kept" {
		t.Fatalf("omitted note was not preserved: %+v", renamed)
	}
	if got := rejectedUpdate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"note": ""}}); !strings.Contains(got, "empty string is invalid") {
		t.Fatalf("empty note error=%s", got)
	}
	if got := rejectedUpdate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"note": "", "noteSet": true}}); !strings.Contains(got, "invalid write arguments") {
		t.Fatalf("removed noteSet error=%s", got)
	}
	if stored := storedLedgerAccount(t, fx, fx.checking); stored.Note == nil || *stored.Note != "kept" {
		t.Fatalf("rejected note edit changed the account: %+v", stored)
	}
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"note": nil}})
	cleared := storedLedgerAccount(t, fx, fx.checking)
	if cleared.Name != "Renamed" || cleared.Note != nil || !cleared.IncludeInNetWorth {
		t.Fatalf("null note did not clear only the note: %+v", cleared)
	}

	institution := mutate(t, c, "create_institution", map[string]any{"name": "Bank One", "institutionType": "bank"})
	group := mutate(t, c, "create_group", map[string]any{"name": "Cash"})
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{
		"institutionId": institution["id"], "groupId": group["id"], "iconKey": "fund",
		"openedOn": "2026-09-15", "closedOn": "2026-09-20", "includeInNetWorth": false,
	}})
	linked := storedLedgerAccount(t, fx, fx.checking)
	if linked.InstitutionID == nil || linked.InstitutionID.String() != institution["id"] || linked.GroupID == nil || linked.GroupID.String() != group["id"] || linked.IconKey == nil || *linked.IconKey != "fund" || linked.OpenedOn == nil || *linked.OpenedOn != "2026-09-15" || linked.ClosedOn == nil || *linked.ClosedOn != "2026-09-20" || linked.IncludeInNetWorth {
		t.Fatalf("linked account = %+v", linked)
	}
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"institutionId": nil}})
	unlinked := storedLedgerAccount(t, fx, fx.checking)
	if unlinked.InstitutionID != nil || unlinked.GroupID == nil || unlinked.IconKey == nil || unlinked.OpenedOn == nil || unlinked.IncludeInNetWorth {
		t.Fatalf("clearing institution changed other fields: %+v", unlinked)
	}
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"groupId": nil, "iconKey": nil, "openedOn": nil, "closedOn": nil}})
	stripped := storedLedgerAccount(t, fx, fx.checking)
	if stripped.GroupID != nil || stripped.IconKey != nil || stripped.OpenedOn != nil || stripped.ClosedOn != nil || stripped.IncludeInNetWorth || stripped.Name != "Renamed" {
		t.Fatalf("null clear = %+v", stripped)
	}
	if got := rejectedUpdate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"institutionId": ""}}); !strings.Contains(got, "empty string is invalid") {
		t.Fatalf("empty institution error=%s", got)
	}
	if got := rejectedUpdate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"name": nil, "note": "nope"}}); !strings.Contains(got, "invalid write arguments") {
		t.Fatalf("null name error=%s", got)
	}
	if got := rejectedUpdate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"ownership": []any{}, "note": "nope"}}); !strings.Contains(got, "at least one owner") {
		t.Fatalf("empty ownership error=%s", got)
	}
	if got := rejectedUpdate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"ownership": nil, "note": "nope"}}); !strings.Contains(got, "invalid write arguments") {
		t.Fatalf("null ownership error=%s", got)
	}
	if got := rejectedUpdate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"includeInNetWorth": nil}}); !strings.Contains(got, "invalid write arguments") {
		t.Fatalf("null inclusion error=%s", got)
	}
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"note": "again"}})
	if got := rejectedUpdate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"defaultCurrency": "EUR", "note": nil}}); !strings.Contains(got, "currency is immutable") {
		t.Fatalf("currency error=%s", got)
	}
	if got := rejectedUpdate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"trackingMode": "holdings", "note": nil}}); !strings.Contains(got, "tracking mode is immutable") {
		t.Fatalf("tracking mode error=%s", got)
	}
	members, err := fx.app.ListMembers(t.Context(), false)
	if err != nil || len(members) != 1 {
		t.Fatal(err)
	}
	bob := mutate(t, c, "create_member", map[string]any{"name": "Bob"})
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"ownership": []any{map[string]any{"memberId": bob["id"], "shareBps": 10000}}}})
	mutate(t, c, "update_account", map[string]any{"id": fx.checking, "changes": map[string]any{"name": "Owned"}})
	owned := storedLedgerAccountRecord(t, fx, fx.checking)
	if owned.Account.Name != "Owned" || owned.Account.Note == nil || *owned.Account.Note != "again" || len(owned.Ownership.Shares()) != 1 || owned.Ownership.Shares()[0].MemberID.String() != bob["id"] {
		t.Fatalf("omitted ownership was not preserved: %+v", owned)
	}
	if stored := storedLedgerAccount(t, fx, fx.checking); stored.InstitutionID != nil || stored.IncludeInNetWorth {
		t.Fatalf("rejected edits persisted: %+v", stored)
	}

	definition, err := fx.service.toolRegistry(LedgerWrite).Definition("update_account")
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"Omitted fields are kept", "null clears", "empty string is rejected", "cannot be empty"} {
		if !strings.Contains(definition.Description, phrase) {
			t.Errorf("update_account description lacks %q: %s", phrase, definition.Description)
		}
	}
	for _, phrase := range []string{"Set=true", "Empty string clears", "replace="} {
		if strings.Contains(definition.Description, phrase) {
			t.Errorf("update_account description still says %q", phrase)
		}
	}
}

func storedLedgerAccount(t *testing.T, fx ledgerFixture, id string) domain.Account {
	t.Helper()
	return storedLedgerAccountRecord(t, fx, id).Account
}

func storedLedgerAccountRecord(t *testing.T, fx ledgerFixture, id string) domain.AccountRecord {
	t.Helper()
	records, err := fx.app.ListAccounts(t.Context(), domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Account.ID.String() == id {
			return record
		}
	}
	t.Fatalf("account %s not found", id)
	return domain.AccountRecord{}
}

// Write tools follow one of a few named update conventions. The table below is
// the audited inventory (see docs/development/mcp-update-conventions.md); a new
// update_*/set_* tool or versioned replacement preview must be classified here
// and match its convention's schema shape, so an omitted field cannot silently
// mean "keep" in one tool and "clear" in another unnoticed.
var updateConventions = map[string]string{
	"update_member":                     "patch",
	"update_institution":                "patch",
	"update_group":                      "patch",
	"update_account":                    "patch",
	"update_instrument":                 "patch",
	"update_instruments_batch":          "patch",
	"update_investment_review_rule":     "revisioned_replacement",
	"preview_save_liquidity_policy":     "versioned_full_replacement",
	"preview_product_terms":             "versioned_full_replacement",
	"set_fx_source":                     "selection",
	"set_investment_review_suppression": "selection",
}

var removedPatchFlags = []string{"institutionIdSet", "groupIdSet", "noteSet", "iconKeySet", "openedOnSet", "closedOnSet", "replace"}

var accountClearableFields = []string{"institutionId", "groupId", "note", "iconKey", "openedOn", "closedOn"}

var accountKeptFields = []string{"name", "accountType", "balanceSheetRole", "trackingMode", "defaultCurrency", "includeInNetWorth", "includeInPortfolio", "includeInLiquidAssets", "initialAmount", "ownership", "ownerIds", "ownershipPercentages"}

var instrumentClearableFields = []string{"shortName", "symbol", "marketCode", "countryCode", "isin", "note", "iconKey", "providerKey", "providerSymbol"}

var instrumentKeptFields = []string{"name", "type", "metalTemplate", "quantityUnit", "quoteCurrency", "quoteSource", "sortOrder"}

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

func schemaTypes(field any) map[string]bool {
	node, _ := field.(map[string]any)
	out := map[string]bool{}
	switch typed := node["type"].(type) {
	case string:
		out[typed] = true
	case []any:
		for _, item := range typed {
			if name, ok := item.(string); ok {
				out[name] = true
			}
		}
	}
	return out
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
	// Management writes are off by default. The convention still applies when
	// Settings enables them.
	enableInvestmentReviewManagement(t, fx.service)
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
		if convention == "patch" && !strings.Contains(d.Description, "Omitted fields are kept") && !strings.Contains(d.Description, "Omission keeps") {
			t.Errorf("%s description does not state the patch keep rule: %s", name, d.Description)
		}
		switch convention {
		case "patch":
			if schemaHasProperty(root, removedPatchFlags...) {
				t.Errorf("%s still publishes a Set or replace flag", name)
			}
			switch name {
			case "update_member", "update_institution", "update_group":
				if !req["id"] || req["name"] || req["iconKey"] || req["archived"] {
					t.Errorf("%s: only id is required; name, iconKey and archived are optional patch fields", name)
				}
				props, _ := input["properties"].(map[string]any)
				for _, field := range []string{"name", "iconKey", "archived"} {
					if _, ok := props[field]; !ok {
						t.Errorf("%s: missing patch field %s", name, field)
					}
					if schemaAllowsNull(props[field].(map[string]any)) {
						t.Errorf("%s: %s cannot be cleared, so null must be rejected", name, field)
					}
				}
				// Omission keeps the stored archive state. true archives and
				// false restores. Null is not a third state.
				if !schemaTypes(props["archived"])["boolean"] || schemaTypes(props["archived"])["array"] {
					t.Errorf("%s: archived must be an optional boolean, got %v", name, props["archived"])
				}
			case "update_account", "update_instrument":
				changes := schemaChild(t, input, "changes")
				if len(requiredSet(changes)) != 0 {
					t.Errorf("%s: patch fields must all be optional", name)
				}
				clearable, kept := accountClearableFields, accountKeptFields
				if name == "update_instrument" {
					clearable, kept = instrumentClearableFields, instrumentKeptFields
				}
				assertPatchFields(t, name, changes, clearable, kept)
			case "update_instruments_batch":
				row := schemaChild(t, input, "items", "[]")
				if !requiredSet(row)["expectedMetadataVersion"] || !requiredSet(row)["changes"] {
					t.Errorf("%s: every row needs expectedMetadataVersion and changes", name)
				}
				changes := schemaChild(t, row, "changes")
				if changes["minProperties"] != float64(1) {
					t.Errorf("%s: an empty changes object must be rejected", name)
				}
				assertPatchFields(t, name, changes, []string{"shortName", "note"}, []string{"name"})
			default:
				t.Errorf("%s has no patch schema checks", name)
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

func assertPatchFields(t *testing.T, name string, node map[string]any, clearable, kept []string) {
	t.Helper()
	props, _ := node["properties"].(map[string]any)
	for _, field := range clearable {
		property, _ := props[field].(map[string]any)
		if !schemaAllowsNull(property) || !schemaAllowsType(property, "string") {
			t.Errorf("%s.%s must accept string or null, got %#v", name, field, props[field])
		}
	}
	for _, field := range kept {
		property, _ := props[field].(map[string]any)
		if schemaAllowsNull(property) {
			t.Errorf("%s.%s must reject null, got %#v", name, field, props[field])
		}
	}
}

func schemaHasProperty(node any, names ...string) bool {
	want := map[string]bool{}
	for _, name := range names {
		want[name] = true
	}
	var walk func(any) bool
	walk = func(value any) bool {
		switch typed := value.(type) {
		case map[string]any:
			props, _ := typed["properties"].(map[string]any)
			for key := range props {
				if want[key] {
					return true
				}
			}
			for _, child := range typed {
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(node)
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
