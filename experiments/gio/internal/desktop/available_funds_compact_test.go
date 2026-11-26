package desktop

import (
	"image"
	"testing"

	"gioui.org/io/input"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAvailableFundsBreakdownWrapsAndKeepsAccessibleAmounts(t *testing.T) {
	app, _, _, _ := openFundsFixture(t, true)
	data, err := readAvailableFunds(t.Context(), app)
	if err != nil {
		t.Fatal(err)
	}
	view := ui.View{Theme: ui.NewTheme(), Language: i18n.English, Colors: &lightPalette}
	breakdown := fundsBreakdown(view, data.today.NativeCurrencyGroups, data.native[data.today.HorizonOn])
	draw := func(width int) *input.Router {
		t.Helper()
		gtx := chartTableContext(width)
		gtx.Constraints.Min.Y = 0
		if dims := breakdown(gtx); dims.Size.X != width || dims.Size.Y >= gtx.Constraints.Max.Y {
			t.Fatalf("breakdown at width %d did not fit: %+v", width, dims)
		}
		router := new(input.Router)
		router.Frame(gtx.Ops)
		return router
	}

	const unreserved = "Available after reservations · €25.25"
	const cash = "Accessible cash before reservations · €25.25"
	const currency = "Native amounts · EUR"
	wide := draw(900)
	header := assertPageLabel(t, wide, "Currency")
	wideCurrency := assertPageLabel(t, wide, currency)
	wideCash := assertPageLabel(t, wide, cash)
	wideAfter := assertPageLabel(t, wide, unreserved)
	if header.Max.Y > wideCurrency.Min.Y || wideCurrency.Max.X > wideCash.Min.X || wideCash.Max.X > wideAfter.Min.X ||
		wideCurrency.Intersect(image.Rect(0, wideCash.Min.Y, 900, wideCash.Max.Y)).Empty() {
		t.Fatalf("wide comparison did not align columns: %v %v %v %v", header, wideCurrency, wideCash, wideAfter)
	}
	narrow := draw(350)
	if findPageLabel(narrow, "Currency") != nil {
		t.Fatal("narrow comparison displayed the wide currency header")
	}
	narrowCurrency := assertPageLabel(t, narrow, currency)
	narrowCash := assertPageLabel(t, narrow, cash)
	narrowAfter := assertPageLabel(t, narrow, unreserved)
	if narrowCurrency.Max.Y > narrowCash.Min.Y || narrowCash.Min.Y >= narrowAfter.Min.Y {
		t.Fatalf("narrow comparison did not wrap under the currency: %v %v %v", narrowCurrency, narrowCash, narrowAfter)
	}
}

func TestAvailableFundsGlobalReservationEmptyAndReturnFocus(t *testing.T) {
	app, _, _, _ := openFundsFixture(t, false)
	funds, h, settle := reservationHarness(t, app, app)
	h.click("Manage reservations", false)
	settle()
	if funds.reservation == nil || funds.reservation.entry != &funds.manageReservations {
		t.Fatal("the initial global action did not open the reservation manager")
	}
	assertPageLabel(t, &h.router, "All reservations")
	assertPageLabel(t, &h.router, "No reservations yet.")
	assertPageLabel(t, &h.router, "No funding sources are eligible for reservations.")
	if findPageLabel(&h.router, "Add reservation") != nil {
		t.Fatal("an empty household offered an ineligible reservation")
	}
	h.click("Back to available funds", false)
	settle()
	if h.shell.activeRoute() != fundsRoute || funds.reservation != nil {
		t.Fatal("the global return did not close the reservation manager")
	}
	h.focused(&funds.manageReservations)
}

func TestAvailableFundsSourceJumpAndDetailsKeepActions(t *testing.T) {
	app, _, _, ids := openFundsFixture(t, true)
	funds, h, settle := reservationHarness(t, app, app)
	h.click("Funding sources", false)
	settle()
	h.focused(&funds.controls.filters[fundsAll])
	const details = "Details · Harbor Savings · USD"
	scrollReservationSource(t, h, details)
	assertPageLabel(t, &h.router, "Manage reservations · Harbor Savings")
	if findPageLabel(&h.router, "Requested reservation · 40.125 USD") != nil {
		t.Fatal("source details were expanded before disclosure")
	}
	h.click(details, false)
	settle()
	assertPageLabel(t, &h.router, "Manage reservations · Harbor Savings")
	assertPageLabel(t, &h.router, "Requested reservation · 40.125 USD")
	if buttons := funds.links[domain.AccountValueSourceRef(ids.bank).Key()]; buttons == nil || !buttons.showDetails {
		t.Fatal("the source disclosure failed to open the selected row")
	}
}
