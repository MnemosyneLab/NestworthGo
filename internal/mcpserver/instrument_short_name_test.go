package mcpserver

import (
	"context"
	"testing"
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
	if err != nil || list[0].ShortName != "家庭 🌿" {
		t.Fatalf("omitted alias %v %v", list, err)
	}
	mutate(t, c, "update_instrument", map[string]any{"id": id, "changes": map[string]any{"shortName": ""}})
	list, err = app.ListInstruments(context.Background(), false)
	if err != nil || list[0].ShortName != "" || list[0].ID.String() != id || list[0].Symbol == nil || *list[0].Symbol != "000001" {
		t.Fatalf("clear %v %v", list, err)
	}
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
		rows := call(t, c, "list_instruments", map[string]any{"query": tc.query}, false)["data"].([]any)
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
