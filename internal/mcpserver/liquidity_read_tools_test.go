package mcpserver

import (
	"reflect"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestScaleLiquidityInstrumentTypesPolicyAndSortMatrix(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixture(t)
	ids := map[string]string{}
	refs := map[string]domain.LiquiditySourceRef{}
	for _, kind := range domain.AllInstrumentTypes() {
		i, err := fx.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Same label", Type: string(kind), QuoteCurrency: "USD", QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		h, err := fx.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: fx.brokerage, InstrumentID: i.ID.String(), Quantity: "10", UnitCost: "1"})
		if err != nil {
			t.Fatal(err)
		}
		ids[string(kind)] = i.ID.String()
		refs[string(kind)] = domain.HoldingSourceRef(domain.AccountID(fx.brokerage), h.ID)
		if _, err := fx.app.AppendManualInstrumentQuote(t.Context(), i.ID, "1.01", "2026-09-28", false); err != nil {
			t.Fatal(err)
		}
	}
	zero, calendar := 0, "calendar"
	for _, kind := range []string{"mutual_fund", "bond"} {
		access := "on_request"
		if kind == "bond" {
			access = "excluded"
		}
		if _, err := fx.app.SaveLiquidityPolicy(t.Context(), application.SavePolicyInput{Source: refs[kind], Policy: application.ProductPolicyInput{AccessKind: access, SettlementDays: &zero, DayBasis: &calendar, EarlyKind: "not_allowed"}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fx.app.AppendAccountValue(t.Context(), domain.AccountID(fx.checking), "100", "2026-09-28"); err != nil {
		t.Fatal(err)
	}
	c := ledgerSession(t, fx)
	for kind := range ids {
		cap := scaleCapture(t, c, "get_liquidity_overview", map[string]any{"responseMode": "paged", "filter": map[string]any{"instrumentTypes": []string{kind}}})
		rows := scaleSection(t, c, cap, "sources")
		if len(rows) != 1 || rows[0].(map[string]any)["instrumentId"] != ids[kind] {
			t.Fatalf("type filter %s", kind)
		}
	}
	for _, field := range []string{"sourceKey", "displayName", "instrumentType", "accessKind", "policyOrigin", "normalReceiptOn", "currentValueBase", "availableBase"} {
		for _, direction := range []string{"asc", "desc"} {
			order := map[string]any{"field": field, "direction": direction}
			if field == "availableBase" {
				order["horizonOn"] = "2026-09-29"
			}
			a := scaleCapture(t, c, "get_liquidity_overview", map[string]any{"responseMode": "paged", "sort": order, "limit": 1})
			b := scaleCapture(t, c, "get_liquidity_overview", map[string]any{"responseMode": "paged", "sort": order, "limit": 100})
			if !reflect.DeepEqual(scaleSection(t, c, a, "sources"), scaleSection(t, c, b, "sources")) {
				t.Fatalf("page sort matrix %s %s", field, direction)
			}
		}
	}
	for _, tc := range []struct {
		filter   map[string]any
		expected int
	}{{map[string]any{"instrumentTypes": []string{"bond"}, "hasRule": true, "excluded": true, "policyOrigins": []string{"explicit"}}, 1}, {map[string]any{"instrumentTypes": []string{"stock", "etf"}, "hasRule": false}, 2}, {map[string]any{"instrumentTypes": []string{"stock"}, "policyOrigins": []string{"explicit"}}, 0}, {map[string]any{"instrumentIds": []string{ids["mutual_fund"]}, "hasRule": false}, 0}, {map[string]any{"instrumentTypes": []string{"mutual_fund"}, "query": " SAME "}, 1}, {map[string]any{"sourceKinds": []string{"account_value"}, "instrumentTypes": []string{"stock"}}, 0}} {
		cap := scaleCapture(t, c, "get_liquidity_overview", map[string]any{"responseMode": "paged", "filter": tc.filter})
		rows := scaleSection(t, c, cap, "sources")
		if len(rows) != tc.expected {
			t.Fatalf("filter %v count %d", tc.filter, len(rows))
		}
	}
}
