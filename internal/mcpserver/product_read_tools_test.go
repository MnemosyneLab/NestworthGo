package mcpserver

import (
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func TestPagedStandaloneProductRequiresCanonicalIdentity(t *testing.T) {
	fx := newLedgerFixture(t)
	ids := []string{}
	for range 2 {
		r := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "record_existing", RecordExisting: &application.RecordExistingProductCommand{AccountID: fx.brokerage, Currency: "USD", Principal: "100", TotalCostBasis: "100", CurrentValue: "100", CashExcludesProduct: true, Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Synthetic product", StartOn: "2026-09-01", MaturityOn: productString("2026-12-01"), InterestMode: "none"}, Policy: productPolicy("2026-12-01")}})
		ids = append(ids, r.ProductIDs[0])
	}
	detail, err := fx.app.Product(t.Context(), domain.ProductContractID(ids[0]))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := fx.app.SaveLiquidityReservation(t.Context(), application.SaveReservationInput{Source: domain.HoldingSourceRef(detail.Contract.AccountID, detail.Contract.HoldingID), Label: "Synthetic reserve", Amount: "1"}); err != nil {
			t.Fatal(err)
		}
	}
	c := ledgerSession(t, fx)
	for _, id := range []string{"", " ", "not-a-uuid"} {
		args := map[string]any{"id": id}
		before := len(fx.service.reads.entries)
		call(t, c, "get_product", args, true)
		if len(fx.service.reads.entries) != before {
			t.Fatal("invalid single-get built a list capture")
		}
	}
	for _, input := range []string{strings.ToUpper(ids[0]), "  " + strings.ToUpper(ids[0]) + "  "} {
		cap := scaleCapture(t, c, "get_product", map[string]any{"id": input, "limit": 1})
		products := scaleSection(t, c, cap, "products")
		if len(products) != 1 || products[0].(map[string]any)["id"] != ids[0] {
			t.Fatal("single-get became list")
		}
		if cap["query"].(map[string]any)["id"] != ids[0] {
			t.Fatal("query identity not canonical")
		}
		summary := cap["summary"].(map[string]any)
		if summary["detail"].(map[string]any)["product"].(map[string]any)["id"] != ids[0] {
			t.Fatal("detail missing")
		}
		if len(scaleSection(t, c, cap, "reservations/"+ids[0])) != 2 {
			t.Fatal("canonical reservation entrance missing")
		}
	}
	empty := domain.ProductContractID("")
	if _, err := fx.service.pagedProducts(t.Context(), fx.service.contextGeneration, ProductListInput{}, &empty); err == nil {
		t.Fatal("internal empty single-get became list")
	}
}
