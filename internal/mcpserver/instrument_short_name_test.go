package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestMCPInstrumentOptionalShortNamePatchAndClear(t *testing.T) {
	s, app, _ := fixture(t)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	i := mutate(t, c, "create_instrument", map[string]any{"name": "Fund A", "symbol": "000001", "type": "mutual_fund", "quoteCurrency": "USD"})
	id := i["id"].(string)
	mutate(t, c, "update_instrument", map[string]any{"id": id, "changes": map[string]any{"shortName": " 家庭 🌿 "}})
	list, err := app.ListInstruments(context.Background(), false)
	if err != nil || len(list) != 1 || list[0].ShortName != "家庭 🌿" {
		t.Fatalf("alias %v %v", list, err)
	}
	mutate(t, c, "update_instrument", map[string]any{"id": id, "changes": map[string]any{"note": "metadata"}})
	list, err = app.ListInstruments(context.Background(), false)
	if err != nil || list[0].ShortName != "家庭 🌿" || list[0].Note == nil || *list[0].Note != "metadata" {
		t.Fatalf("omitted alias %v %v", list, err)
	}
	mutate(t, c, "update_instrument", map[string]any{"id": id, "changes": map[string]any{"shortName": nil}})
	list, err = app.ListInstruments(context.Background(), false)
	if err != nil || list[0].ShortName != "" || list[0].ID.String() != id || list[0].Symbol == nil || *list[0].Symbol != "000001" || list[0].Note == nil {
		t.Fatalf("null clear %v %v", list, err)
	}
	if got := rejectedUpdate(t, c, "update_instrument", map[string]any{"id": id, "changes": map[string]any{"shortName": ""}}); !strings.Contains(got, "must be nonempty") {
		t.Fatalf("empty shortName error=%s", got)
	}
	if got := rejectedUpdate(t, c, "update_instrument", map[string]any{"id": id, "changes": map[string]any{"name": nil}}); !strings.Contains(got, "invalid write arguments") {
		t.Fatalf("null name error=%s", got)
	}
	if got := rejectedUpdate(t, c, "update_instrument", map[string]any{"id": id, "changes": map[string]any{"replace": true, "name": "Nope"}}); !strings.Contains(got, "invalid write arguments") {
		t.Fatalf("replace error=%s", got)
	}
	mutate(t, c, "update_instrument", map[string]any{"id": id, "changes": map[string]any{"sortOrder": 4, "iconKey": "fund"}})
	mutate(t, c, "update_instrument", map[string]any{"id": id, "changes": map[string]any{"sortOrder": 0, "iconKey": nil, "symbol": nil}})
	list, err = app.ListInstruments(context.Background(), false)
	if err != nil || list[0].SortOrder != 0 || list[0].Symbol != nil || list[0].ShortName != "" || list[0].Name != "Fund A" || list[0].IconKey == nil || *list[0].IconKey != domain.DefaultInstrumentIcon(domain.InstrumentMutualFund) {
		t.Fatalf("sort, icon and symbol patch %v %v", list, err)
	}
}

func TestInstrumentBatchNullClearsAndEmptyStringRejects(t *testing.T) {
	s, app, _ := fixture(t)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	created := mutate(t, c, "create_instrument", map[string]any{"name": "Fund A", "symbol": "000001", "shortName": "Alias", "type": "mutual_fund", "quoteCurrency": "USD"})
	id := created["id"].(string)
	version := instrumentMetadataVersion(t, c, id)
	mutate(t, c, "update_instruments_batch", map[string]any{"items": []any{map[string]any{"id": id, "expectedMetadataVersion": version, "changes": map[string]any{"note": "hello"}}}})
	version = instrumentMetadataVersion(t, c, id)
	mutate(t, c, "update_instruments_batch", map[string]any{"items": []any{map[string]any{"id": id, "expectedMetadataVersion": version, "changes": map[string]any{"shortName": nil}}}})
	list, err := app.ListInstruments(context.Background(), false)
	if err != nil || list[0].ShortName != "" || list[0].Note == nil || *list[0].Note != "hello" || list[0].Name != "Fund A" {
		t.Fatalf("batch null shortName = %+v %v", list, err)
	}
	version = instrumentMetadataVersion(t, c, id)
	for _, changes := range []map[string]any{{"shortName": ""}, {"note": " "}, {"name": nil}} {
		call(t, c, "update_instruments_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{map[string]any{"id": id, "expectedMetadataVersion": version, "changes": changes}}}}, true)
	}
	list, err = app.ListInstruments(context.Background(), false)
	if err != nil || list[0].ShortName != "" || list[0].Note == nil || *list[0].Note != "hello" || list[0].Name != "Fund A" {
		t.Fatalf("rejected batch changed metadata: %+v %v", list, err)
	}
	version = instrumentMetadataVersion(t, c, id)
	mutate(t, c, "update_instruments_batch", map[string]any{"items": []any{map[string]any{"id": id, "expectedMetadataVersion": version, "changes": map[string]any{"note": nil}}}})
	list, err = app.ListInstruments(context.Background(), false)
	if err != nil || list[0].Note != nil || list[0].ShortName != "" || list[0].Symbol == nil || *list[0].Symbol != "000001" {
		t.Fatalf("batch null note = %+v %v", list, err)
	}
}

func instrumentMetadataVersion(t *testing.T, c *mcp.ClientSession, id string) string {
	t.Helper()
	rows := scaleSection(t, c, scaleCapture(t, c, "list_instruments", map[string]any{"filter": map[string]any{"ids": []string{id}}}), "instruments")
	if len(rows) != 1 {
		t.Fatalf("instrument rows = %v", rows)
	}
	version, _ := rows[0].(map[string]any)["metadataVersion"].(string)
	if version == "" {
		t.Fatalf("missing metadataVersion: %v", rows[0])
	}
	return version
}

func TestMCPInstrumentSearchAliasNameCodeAndDuplicates(t *testing.T) {
	s, _, _ := fixture(t)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	first := mutate(t, c, "create_instrument", map[string]any{"name": "Share class A", "symbol": "000001", "shortName": " Family Fund ", "type": "mutual_fund", "quoteCurrency": "USD"})
	second := mutate(t, c, "create_instrument", map[string]any{"name": "Share class B", "symbol": "000002", "shortName": "Family Fund", "type": "mutual_fund", "quoteCurrency": "USD"})
	for _, tc := range []struct {
		query string
		ids   []string
	}{{" family FUND ", []string{first["id"].(string), second["id"].(string)}}, {"\uFEFF\u0085Family Fund\uFEFF", []string{first["id"].(string), second["id"].(string)}}, {" SHARE CLASS a ", []string{first["id"].(string)}}, {" 000002 ", []string{second["id"].(string)}}, {"missing", nil}} {
		rows := scaleSection(t, c, scaleCapture(t, c, "list_instruments", map[string]any{"query": tc.query}), "instruments")
		if len(rows) != len(tc.ids) {
			t.Fatalf("query %q: got %v want %v", tc.query, rows, tc.ids)
		}
		got := map[string]bool{}
		for _, row := range rows {
			r := row.(map[string]any)
			got[r["id"].(string)] = true
			if r["shortName"] != "Family Fund" || r["name"] == nil || r["symbol"] == nil {
				t.Fatalf("lost identity %+v", r)
			}
		}
		for _, id := range tc.ids {
			if !got[id] {
				t.Fatalf("query %q lost %s", tc.query, id)
			}
		}
	}
}

func TestMCPInstrumentBindingNullClearRejectsAtomically(t *testing.T) {
	s, app, _ := fixture(t)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	for _, template := range []string{"gold", "silver"} {
		i := mutate(t, c, "create_instrument", map[string]any{"name": template, "type": "precious_metal", "metalTemplate": template, "quantityUnit": "g", "quoteCurrency": "USD", "quoteSource": "manual"})
		for _, field := range []string{"marketCode", "providerKey", "providerSymbol"} {
			got := rejectedUpdate(t, c, "update_instrument", map[string]any{"id": i["id"], "changes": map[string]any{field: nil, "name": "must not persist"}})
			if !strings.Contains(got, "cannot be cleared") {
				t.Fatalf("%s/%s: %s", template, field, got)
			}
		}
	}
	crypto := mutate(t, c, "create_instrument", map[string]any{"name": "Synthetic Bitcoin", "type": "crypto", "quoteCurrency": "USD", "quoteSource": "provider", "providerKey": "coingecko", "providerSymbol": "bitcoin"})
	got := rejectedUpdate(t, c, "update_instrument", map[string]any{"id": crypto["id"], "changes": map[string]any{"marketCode": nil, "name": "must not persist"}})
	if !strings.Contains(got, "cannot be cleared") {
		t.Fatal(got)
	}
	rows, err := app.ListInstruments(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Name == "must not persist" {
			t.Fatal("rejected patch partially persisted")
		}
	}
}
