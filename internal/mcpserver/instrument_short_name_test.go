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
