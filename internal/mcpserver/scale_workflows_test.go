package mcpserver

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func scaleCapture(t *testing.T, c *mcp.ClientSession, name string, args any) map[string]any {
	t.Helper()
	return call(t, c, name, args, false)["data"].(map[string]any)
}
func scaleSection(t *testing.T, c *mcp.ClientSession, capture map[string]any, name string) []any {
	t.Helper()
	section := capture["sections"].(map[string]any)[name].(map[string]any)
	rows := append([]any{}, section["items"].([]any)...)
	page := section["page"].(map[string]any)
	for page["hasMore"] == true {
		next := scaleCapture(t, c, "get_read_page", skillExample(t, "paging-and-batches", "scale-page", map[string]string{"captureId": capture["captureId"].(string), "section": name, "cursor": page["nextCursor"].(string)}))
		if next["contentHash"] != capture["contentHash"] {
			t.Fatal("mixed capture")
		}
		items := next["items"].([]any)
		if len(items) == 0 {
			t.Fatal("cursor did not advance")
		}
		rows = append(rows, items...)
		page = next["page"].(map[string]any)
	}
	if len(rows) != int(page["total"].(float64)) {
		t.Fatalf("incomplete section %s: %d/%v", name, len(rows), page["total"])
	}
	return rows
}
func TestScaleHTTPProductsLiquidityAndReservations(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	var id string
	for range 12 {
		p := productPolicy("2026-12-01")
		p.Note = productString(strings.Repeat("private😀\\\"", 100))
		r := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "record_existing", RecordExisting: &application.RecordExistingProductCommand{AccountID: fx.brokerage, Currency: "USD", Principal: "100", TotalCostBasis: "100", CurrentValue: "100", CashExcludesProduct: true, Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Synthetic large product", Note: productString(strings.Repeat("private😀", 100)), StartOn: "2026-09-01", MaturityOn: productString("2026-12-01"), InterestMode: "none"}, Policy: p}})
		id = r.ProductIDs[0]
	}
	detail, err := fx.app.Product(t.Context(), domain.ProductContractID(id))
	if err != nil {
		t.Fatal(err)
	}
	for range 91 {
		if _, err := fx.app.SaveLiquidityReservation(t.Context(), application.SaveReservationInput{Source: domain.HoldingSourceRef(detail.Contract.AccountID, detail.Contract.HoldingID), Label: strings.Repeat("synthetic reserve ", 5), Amount: "1"}); err != nil {
			t.Fatal(err)
		}
	}
	c := ledgerSession(t, fx)
	cap := scaleCapture(t, c, "list_products", skillExample(t, "paging-and-batches", "scale-products", nil))
	products := scaleSection(t, c, cap, "products")
	if len(products) != 12 {
		t.Fatal(len(products))
	}
	for _, raw := range products {
		p := raw.(map[string]any)
		if _, ok := p["note"]; ok {
			t.Fatal("compact row includes note")
		}
		if _, ok := p["policy"]; ok {
			t.Fatal("compact row includes policy")
		}
	}
	item := scaleCapture(t, c, "get_read_item", skillExample(t, "paging-and-batches", "scale-product-item", map[string]string{"captureId": cap["captureId"].(string), "productId": id}))
	reserves := scaleSection(t, c, item, "reservations/"+id)
	if len(reserves) != 91 {
		t.Fatal(len(reserves))
	}
	if item["contentHash"] != cap["contentHash"] {
		t.Fatal("detail recaptured")
	}
	one := scaleCapture(t, c, "get_product", map[string]any{"responseMode": "paged", "id": id, "limit": 1})
	if one["query"].(map[string]any)["id"] != id {
		t.Fatal("standalone product query identity missing")
	}
	if len(scaleSection(t, c, one, "reservations/"+id)) != 91 {
		t.Fatal("standalone reservations")
	}
	overview, err := liquidity.NewService(fx.app).Overview(t.Context(), liquidity.OverviewRequest{})
	if err != nil {
		t.Fatal(err)
	}
	liq := scaleCapture(t, c, "get_liquidity_overview", map[string]any{"responseMode": "paged", "limit": 1})
	if liq["query"].(map[string]any)["includeEarlyWithdrawal"] != false {
		t.Fatal("liquidity request basis missing from frozen query")
	}
	sources := scaleSection(t, c, liq, "sources")
	if len(sources) != len(overview.Sources) {
		t.Fatalf("source union %d/%d", len(sources), len(overview.Sources))
	}
	summary := liq["summary"].(map[string]any)
	productJSONEqual(t, summary["buckets"], overview.Buckets)
	filtered := scaleCapture(t, c, "get_liquidity_overview", map[string]any{"responseMode": "paged", "filter": map[string]any{"instrumentTypes": []string{"bank_investment_product"}, "policyOrigins": []string{"contract"}, "hasRule": true}, "sort": map[string]any{"field": "currentValueBase", "direction": "desc"}, "limit": 1})
	for _, r := range scaleSection(t, c, filtered, "sources") {
		v := r.(map[string]any)
		if v["hasRule"] != true || v["policyEditable"] != false || v["policyOrigin"] != "contract" {
			t.Fatal(v)
		}
	}
	empty := scaleCapture(t, c, "get_liquidity_overview", skillExample(t, "paging-and-batches", "scale-empty-match", nil))
	if len(scaleSection(t, c, empty, "sources")) != 0 {
		t.Fatal("empty intersection")
	}
	productJSONEqual(t, empty["summary"].(map[string]any)["buckets"], overview.Buckets)
	call(t, c, "get_liquidity_overview", map[string]any{"filter": map[string]any{}}, true)
	call(t, c, "get_liquidity_overview", map[string]any{"responseMode": "paged", "sort": map[string]any{"field": "availableBase"}}, true)
	for _, name := range []string{"get_read_page", "get_read_item", "get_market_data_batch", "update_instruments_batch"} {
		payload := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": map[string]any{strings.Repeat("private", 12000): true}}}
		status, wire := productHTTP(t, fx.service, payload, "")
		if status != 200 || len(wire) > contextWireLimit || strings.Contains(string(wire), "privateprivate") {
			t.Fatalf("unsafe wire %s %d/%d", name, status, len(wire))
		}
		if status, _ := productHTTP(t, fx.service, []any{payload}, ""); status != 400 {
			t.Fatal("JSON-RPC batch accepted")
		}
	}
}
func TestScaleHTTPMetadataAtomicCASAndReceipts(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	second, err := fx.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Second", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	c := ledgerSession(t, fx)
	capture := scaleCapture(t, c, "list_instruments", map[string]any{"responseMode": "paged", "limit": 1})
	rows := scaleSection(t, c, capture, "instruments")
	versions := map[string]string{}
	for _, r := range rows {
		v := r.(map[string]any)
		versions[v["id"].(string)] = v["metadataVersion"].(string)
	}
	items := []any{map[string]any{"id": fx.instrument, "expectedMetadataVersion": versions[fx.instrument], "changes": map[string]any{"shortName": "Alias", "note": nil}}, map[string]any{"id": second.ID.String(), "expectedMetadataVersion": strings.Repeat("0", 64), "changes": map[string]any{"name": "Changed second"}}}
	call(t, c, "update_instruments_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": items}}, true)
	now := scaleCapture(t, c, "list_instruments", map[string]any{"responseMode": "paged"})
	for _, r := range scaleSection(t, c, now, "instruments") {
		v := r.(map[string]any)
		if versions[v["id"].(string)] != v["metadataVersion"] {
			t.Fatal("partial batch")
		}
	}
	items[1].(map[string]any)["expectedMetadataVersion"] = versions[second.ID.String()]
	args := map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": items}}
	first := call(t, c, "update_instruments_batch", args, false)["data"]
	again := call(t, c, "update_instruments_batch", args, false)["data"]
	if !reflect.DeepEqual(first, again) {
		t.Fatal("receipt retry changed")
	}
	call(t, c, "update_instruments_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": items}}, true)
	frozen := scaleSection(t, c, capture, "instruments")
	if !reflect.DeepEqual(rows, frozen) {
		t.Fatal("metadata write changed old capture")
	}
	if _, err := fx.service.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	tools := featureToolList(t, connect(t, fx.service))
	if tools["update_instruments_batch"] != nil {
		t.Fatal("read-only write discovery")
	}
}
func TestScaleHTTPMarketCurrentHistoryAndAgentRetraction(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	item := map[string]any{"instrumentId": fx.instrument, "currency": "USD", "value": "1.0197", "kind": "nav", "date": "2026-09-28", "sourceTitle": "Synthetic issuer"}
	receipt := mutate(t, c, "import_market_data", map[string]any{"items": []any{item, map[string]any{"baseCurrency": "USD", "quoteCurrency": "SGD", "value": "1.28", "kind": "daily_reference", "date": "2026-09-28", "sourceTitle": "Synthetic FX"}}})
	id := receipt["quoteIds"].([]any)[0]
	targets := []any{map[string]any{"instrumentId": fx.instrument}, map[string]any{"baseCurrency": "SGD", "quoteCurrency": "USD"}, map[string]any{"instrumentId": uuid.NewString()}}
	current := scaleCapture(t, c, "get_market_data_batch", map[string]any{"targets": targets, "limit": 1})
	if len(current["sections"].(map[string]any)) != 1 {
		t.Fatal("current included history")
	}
	got := scaleSection(t, c, current, "targets")
	if len(got) != 3 {
		t.Fatal("missing target")
	}
	for _, view := range []string{"coverage", "history"} {
		cap := scaleCapture(t, c, "get_market_data_batch", map[string]any{"targets": targets, "view": view, "limit": 1})
		for _, raw := range scaleSection(t, c, cap, "targets") {
			v := raw.(map[string]any)
			coverage := v["coverage"].(map[string]any)
			if coverage["gapAssessment"] != "not_evaluated" {
				t.Fatal("invented coverage")
			}
			if view == "history" {
				for _, k := range []string{"pointsSection", "observationsSection"} {
					targetCapture := map[string]any{"captureId": cap["captureId"], "contentHash": cap["contentHash"], "sections": v["sections"]}
					rows := scaleSection(t, c, targetCapture, v[k].(string))
					if len(rows) != int(coverage[map[string]string{"pointsSection": "selectedPointsCount", "observationsSection": "observationsCount"}[k]].(float64)) {
						t.Fatal("history count")
					}
				}
			}
		}
	}
	mutate(t, c, "import_market_data", map[string]any{"items": []any{map[string]any{"operation": "retract", "targetQuoteId": id, "sourceTitle": "Synthetic withdrawal"}}})
	audit := scaleCapture(t, c, "list_agent_market_data", skillExample(t, "paging-and-batches", "scale-agent-retractions", map[string]string{"instrumentId": fx.instrument}))
	records := scaleSection(t, c, audit, "records")
	if len(records) != 1 || records[0].(map[string]any)["instrumentId"] != fx.instrument {
		t.Fatal("retraction lost target identity")
	}
	many := []any{targets[0]}
	for range 99 {
		many = append(many, map[string]any{"instrumentId": uuid.NewString()})
	}
	large := scaleCapture(t, c, "get_market_data_batch", map[string]any{"targets": many, "view": "history", "limit": 100})
	if len(scaleSection(t, c, large, "targets")) != 100 {
		t.Fatal("100 target completeness")
	}
	call(t, c, "get_market_data_batch", map[string]any{"targets": []any{targets[0], targets[0]}}, true)
	call(t, c, "list_agent_market_data", map[string]any{"responseMode": "paged", "offset": 1}, true)
}
