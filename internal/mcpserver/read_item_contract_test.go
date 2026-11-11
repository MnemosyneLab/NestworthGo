package mcpserver

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestReadItemLazyChildrenContract(t *testing.T) {
	for _, kind := range []string{"market_import_comparison", "returnDays", "assetDrivers"} {
		t.Run(kind, func(t *testing.T) {
			cache := newBoundedReadCache()
			cache.activate(1)
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			cache.now = func() time.Time { return now }
			capture := readCapture{Kind: "period_analysis"}
			in := ReadItemInput{Section: kind, ID: "2024-01-01"}
			child, other := "contributors", "issues"
			rows := make([]map[string]any, 101)
			for i := range rows {
				rows[i] = map[string]any{"key": fmt.Sprint(i), "label": "Synthetic", "evidenceId": fmt.Sprint(i), "delta": "0.00000001"}
			}
			encode := func(v any) json.RawMessage {
				b, err := jsoncodec.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			if kind == "market_import_comparison" {
				capture.Kind = kind
				in.Section, in.ID = "results", "csv/001"
				child, other = "relations", "evidence"
				capture.Sections = map[string][]json.RawMessage{
					"results": {encode(map[string]any{"clientItemId": in.ID, "matchId": "match"})},
					"matches": {encode(map[string]any{"id": "match", "relations": rows})},
				}
			} else {
				var day wire.ReturnDayDTO
				var driver wire.AssetDriverDetailDTO
				if err := jsoncodec.Unmarshal(encode(map[string]any{"date": in.ID, "contributors": rows}), &day); err != nil {
					t.Fatal(err)
				}
				if err := jsoncodec.Unmarshal(encode(map[string]any{"driverKey": "income", "byAccount": rows}), &driver); err != nil {
					t.Fatal(err)
				}
				var err error
				capture.drilldown, err = newPeriodDrilldown(in.ID, in.ID, []wire.ReturnDayDTO{day}, []wire.AssetDriverDetailDTO{driver})
				if err != nil {
					t.Fatal(err)
				}
				if kind == "assetDrivers" {
					in.ID, child, other = "income", "byAccount", "byInstrument"
				}
			}
			env, err := cache.store(1, capture, 25)
			if err != nil {
				t.Fatal(err)
			}
			in.CaptureID = env.CaptureID
			original := cache.entries[env.CaptureID]
			service := &Service{reads: cache}
			section := kind + "/" + in.ID + "/" + child
			if kind == "market_import_comparison" {
				section = "comparison_item/0/" + child
			}
			for _, limit := range []int{0, 1, 100} {
				in.Limit, in.Sections = limit, []string{child, child}
				value, err := service.readItem(1, in, "")
				if err != nil {
					t.Fatal(err)
				}
				detail := value.(ReadEnvelope)
				want := limit
				if want == 0 {
					want = 25
				}
				part := detail.Sections[section]
				if len(detail.Sections) != 1 || len(part.Items) != want || !part.Page.HasMore || part.Page.Total != 101 || !slices.Contains(detail.AvailableSections, other) {
					t.Fatalf("selection/limit: %+v", detail)
				}
				if detail.Delivery == nil || detail.Delivery.SelectionComplete || !slices.Contains(detail.Delivery.PendingSections, section) {
					t.Fatalf("delivery: %+v", detail.Delivery)
				}
				got := append([]json.RawMessage(nil), part.Items...)
				for part.Page.HasMore {
					page, err := cache.page(1, ReadPageInput{CaptureID: env.CaptureID, Section: section, Cursor: *part.Page.NextCursor, Limit: 100})
					if err != nil {
						t.Fatal(err)
					}
					got = append(got, page.Items...)
					part.Page = *page.Page
				}
				expected, _, err := readSectionRows(original, section)
				if err != nil || string(encode(got)) != string(encode(expected)) {
					t.Fatal("continuation lost, reordered or altered frozen rows", err)
				}
				if detail.CapturedAt != env.CapturedAt || cache.entries[env.CaptureID].expires != original.expires || cache.bytes != original.size {
					t.Fatal("item changed capture identity, TTL or retained bytes")
				}
			}
			for _, bad := range []ReadItemInput{
				{Limit: -1}, {Limit: 101}, {Sections: []string{"unknown"}}, {Sections: []string{section}},
			} {
				bad.CaptureID, bad.Section, bad.ID = in.CaptureID, in.Section, in.ID
				if _, err := service.readItem(1, bad, ""); err == nil || !strings.Contains(err.Error(), "validation") {
					t.Fatalf("invalid input accepted: %+v %v", bad, err)
				}
			}
			in.Limit, in.Sections = 100, []string{other}
			value, err := service.readItem(1, in, "")
			if err != nil || !value.(ReadEnvelope).Delivery.SelectionComplete {
				t.Fatal("empty selected section not complete", err)
			}
			now = now.Add(readTTL)
			if _, err := service.readItem(1, in, ""); err == nil || !strings.Contains(err.Error(), "read_expired") {
				t.Fatal("item renewed TTL", err)
			}
		})
	}
}

func TestReadItemLazyChildrenWireBudget(t *testing.T) {
	for _, labelBytes := range []int{4096, contextWireLimit} {
		t.Run(fmt.Sprint(labelBytes), func(t *testing.T) {
			cache := newBoundedReadCache()
			cache.activate(1)
			capture := readCapture{Kind: "period_analysis", drilldown: syntheticPeriodDetails(t, 1, 30, labelBytes)}
			env, err := cache.store(1, capture, 25)
			if err != nil {
				t.Fatal(err)
			}
			service := &Service{reads: cache}
			value, err := service.readItem(1, ReadItemInput{CaptureID: env.CaptureID, Section: "returnDays", ID: "2024-01-01", Sections: []string{"contributors"}, Limit: 100}, "")
			if err != nil {
				t.Fatal(err)
			}
			detail := value.(ReadEnvelope)
			section := "returnDays/2024-01-01/contributors"
			part := detail.Sections[section]
			if ok, err := readFits(detail); err != nil || !ok || !part.Page.HasMore || len(part.Items) >= 30 {
				t.Fatal("wire budget not enforced", err)
			}
			total := len(part.Items)
			for part.Page.HasMore {
				page, err := cache.page(1, ReadPageInput{CaptureID: env.CaptureID, Section: section, Cursor: *part.Page.NextCursor, Limit: 100})
				if labelBytes == contextWireLimit {
					if total != 0 || err == nil || !strings.Contains(err.Error(), "too_large") {
						t.Fatal("oversized row was skipped or truncated", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if ok, err := readFits(page); err != nil || !ok {
					t.Fatal("continuation exceeds wire budget", err)
				}
				total += len(page.Items)
				part.Page = *page.Page
			}
			if total != 30 {
				t.Fatalf("budget dropped rows: %d", total)
			}
		})
	}
}

func TestProductItemDuplicateChildren(t *testing.T) {
	cache := newBoundedReadCache()
	cache.activate(1)
	rows, err := readRows([]map[string]string{{"id": "first"}, {"id": "second"}})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := cache.store(1, readCapture{Kind: "products", Details: map[string]any{"product": map[string]string{"id": "product"}}, Sections: map[string][]json.RawMessage{"reservations/product": rows}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{reads: cache}
	input := ReadItemInput{CaptureID: initial.CaptureID, Section: "products", ID: "product", Sections: []string{"reservations", "reservations"}, Limit: 1}
	value, err := service.readItem(1, input, "")
	if err != nil {
		t.Fatal(err)
	}
	detail := value.(ReadEnvelope)
	section := detail.Sections["reservations/product"]
	if len(detail.Sections) != 1 || len(section.Items) != 1 || section.Page.Total != 2 || !section.Page.HasMore {
		t.Fatalf("duplicate selection changed page: %+v", detail)
	}
	page, err := cache.page(1, ReadPageInput{CaptureID: initial.CaptureID, Section: "reservations/product", Cursor: *section.Page.NextCursor, Limit: 100})
	if err != nil || len(page.Items) != 1 || string(page.Items[0]) != string(rows[1]) {
		t.Fatal("continuation lost reservation", err)
	}
	input.Sections = []string{"reservations", "unknown"}
	if _, err := service.readItem(1, input, ""); err == nil {
		t.Fatal("unknown child accepted")
	}
}

func TestReadPagePublicSchemaOnlyCursorAndLimit(t *testing.T) {
	schema := scaleReadSchema[ReadPageInput]("get_read_page")
	if len(schema.Properties) != 2 || schema.Properties["cursor"] == nil || schema.Properties["limit"] == nil {
		t.Fatalf("unexpected public properties: %v", schema.Properties)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]any{"cursor": "signed-cursor", "limit": 100}
	if err := resolved.Validate(&valid); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"captureId", "section"} {
		invalid := map[string]any{"cursor": "signed-cursor", field: "scope"}
		if err := resolved.Validate(&invalid); err == nil {
			t.Fatalf("public schema accepted %s", field)
		}
	}
}
