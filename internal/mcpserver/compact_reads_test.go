package mcpserver

import (
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestCompactInstrumentIdentityFrozenDetailAndPermissions(t *testing.T) {
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		t.Run(mode, func(t *testing.T) {
			s, app, _ := fixture(t)
			note := strings.Repeat("Synthetic private note 数据 ", 30)
			want := map[string]wire.InstrumentDTO{}
			for _, currency := range []string{"USD", "EUR"} {
				i, err := app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Same display name", Type: "stock", Symbol: "SAME", MarketCode: "XNYS", QuoteCurrency: currency, QuoteSource: "manual", Note: &note})
				if err != nil {
					t.Fatal(err)
				}
				want[i.ID.String()] = wire.FromInstrument(i)
			}
			if _, err := s.Enable(mode); err != nil {
				t.Fatal(err)
			}
			c := connect(t, s)
			capture := scaleCapture(t, c, "list_instruments", map[string]any{"limit": 1})
			// A source write between first and later pages must not thaw details.
			for id := range want {
				if _, err := app.UpdateInstrument(t.Context(), domain.InstrumentID(id), application.InstrumentInput{Name: "Changed after capture", Note: productString("changed")}); err != nil {
					t.Fatal(err)
				}
			}
			rows := scaleSection(t, c, capture, "instruments")
			if len(rows) != len(want) {
				t.Fatal("lost duplicate-name identities", rows)
			}
			for _, raw := range rows {
				row := raw.(map[string]any)
				id := row["id"].(string)
				original := want[id]
				if row["name"] != original.Name || row["symbol"] != *original.Symbol || row["marketCode"] != *original.MarketCode || row["quoteCurrency"] != original.QuoteCurrency || row["managed"] != false || len(row["metadataVersion"].(string)) != 64 {
					t.Fatal("lost identity or write guards", row)
				}
				for _, key := range []string{"note", "iconKey", "sortOrder", "householdId", "createdAt", "updatedAt"} {
					if _, ok := row[key]; ok {
						t.Fatal("list disclosed detail field", key)
					}
				}
				detail := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: capture["captureId"].(string), Section: "instruments", ID: id})
				full := detail["summary"].(map[string]any)
				if detail["contentHash"] != capture["contentHash"] || full["note"] != *original.Note || full["metadataVersion"] != row["metadataVersion"] {
					t.Fatal("detail lost capture or CAS", detail)
				}
				for key, value := range row {
					productJSONEqual(t, full[key], value)
				}
				delete(full, "metadataVersion")
				delete(full, "managed")
				productJSONEqual(t, full, original)
			}
			call(t, c, "get_read_item", ReadItemInput{CaptureID: capture["captureId"].(string), Section: "members", ID: rows[0].(map[string]any)["id"].(string)}, true)
			empty := scaleCapture(t, c, "list_instruments", map[string]any{"query": "no such instrument"})
			if len(scaleSection(t, c, empty, "instruments")) != 0 {
				t.Fatal("empty search not complete")
			}
		})
	}
}

func TestCompactDirectoryRetainsFullFrozenMetadata(t *testing.T) {
	s, _, _ := fixture(t)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	for _, section := range []string{"members", "institutions", "groups", "holdings", "accounts"} {
		full := map[string]any{"id": "synthetic", "name": "Synthetic", "note": "private", "description": "private group description", "iconKey": "private", "sortOrder": 9, "createdAt": "synthetic", "updatedAt": "synthetic", "archivedAt": "synthetic", "institutionType": "bank", "countryCode": "US", "website": "https://synthetic.invalid", "accountId": "synthetic-account", "instrumentId": "synthetic-instrument", "quantity": "0.1234567890123456789", "futureGuiField": "must not leak"}
		if section == "accounts" {
			full = map[string]any{"account": full, "ownership": []any{map[string]any{"memberId": "owner", "shareBps": 10000}}, "latestValue": map[string]any{"amount": "123"}, "futureGuiField": "must not leak"}
		}
		capture, err := s.pagedDirectory(t.Context(), s.contextGeneration, section, Empty{}, 1, []any{full})
		if err != nil {
			t.Fatal(err)
		}
		var row map[string]any
		if err := decodeReadValue(capture.Sections[section].Items[0], &row); err != nil {
			t.Fatal(err)
		}
		entity := row
		if section == "accounts" {
			entity = row["account"].(map[string]any)
			if row["ownership"] == nil || row["latestValue"] != nil {
				t.Fatal("account projection lost ownership or repeats value", row)
			}
		}
		for _, key := range []string{"note", "description", "website", "iconKey", "sortOrder", "createdAt", "updatedAt", "futureGuiField"} {
			if entity[key] != nil || row["futureGuiField"] != nil {
				t.Fatal("directory leaked metadata", section, key)
			}
		}
		if entity["archivedAt"] == nil || (section == "holdings" && entity["quantity"] != full["quantity"]) {
			t.Fatal("lost archive status or exact quantity", row)
		}
		detail := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: capture.CaptureID, Section: section, ID: "synthetic"})
		productJSONEqual(t, detail["summary"], full)
	}
}
