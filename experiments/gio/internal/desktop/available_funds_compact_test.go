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
	view := ui.View{Theme: ui.NewTheme(), Language: i18n.English, Colors: &lightPalette, DecimalSeparator: ".", GroupingSeparator: ","}
	breakdown := fundsBreakdown(view, fundsHorizon{bucket: &data.today, label: i18n.FundsHorizonToday}, data.native[data.today.HorizonOn])
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
	// The fixture lacks EUR-to-USD FX. The shared comparison marks that
	// cash category unavailable while retaining known native spendable euros.
	const cash = "Accessible cash before reservations · Unknown"
	const currency = "Native amounts · EUR"
	wide := draw(780)
	header := assertPageLabel(t, wide, "Currency")
	wideCurrency := assertPageLabel(t, wide, currency)
	wideCash := assertPageLabel(t, wide, cash)
	wideAfter := assertPageLabel(t, wide, unreserved)
	if header.Min.Y >= wideCurrency.Min.Y || wideCurrency.Max.X > wideCash.Min.X || wideCash.Max.X > wideAfter.Min.X ||
		wideCurrency.Intersect(image.Rect(0, wideCash.Min.Y, 780, wideCash.Max.Y)).Empty() {
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
	assertPageLabel(t, &h.router, "Reservations")
	assertPageLabel(t, &h.router, "No money set aside yet. Add a reservation when you need to keep money for a specific purpose.")
	assertPageLabel(t, &h.router, "No available sources. Add an account or asset before creating a reservation.")
	if findPageLabel(&h.router, "Add reservation") != nil {
		t.Fatal("an empty household offered an ineligible reservation")
	}
	h.click("Close", false)
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
	assertPageLabel(t, &h.router, "Actions for Harbor Savings · USD")
	if findPageLabel(&h.router, "Requested reservation · 40.125 USD") != nil {
		t.Fatal("source details were expanded before disclosure")
	}
	h.click(details, false)
	settle()
	assertPageLabel(t, &h.router, "Actions for Harbor Savings · USD")
	assertPageLabel(t, &h.router, "Requested reservation · 40.125 USD")
	if buttons := funds.links[domain.AccountValueSourceRef(ids.bank).Key()]; buttons == nil || !buttons.showDetails {
		t.Fatal("the source disclosure failed to open the selected row")
	}
}

func TestAvailableFundsSourceColumnsFollowContainerWidth(t *testing.T) {
	app, _, _, ids := openFundsFixture(t, true)
	funds, h, settle := reservationHarness(t, app, app)
	version := app.MobileDataVersion()
	setWidth := func(width int) {
		h.size = image.Pt(width, 1020)
		settle()
	}
	setWidth(1260)
	h.click("Within 7 days", false)
	settle()
	horizon := funds.selectedFundsBucket().HorizonOn
	assertPageLabel(t, &h.router, "Within 7 days · "+horizon)
	h.click("Funding sources", false)
	settle()
	h.focused(&funds.controls.filters[fundsAll])
	header := assertPageLabel(t, &h.router, "Assets · by "+horizon)
	reservations := semanticBounds(t, &h.router, "Manage reservations", false)
	if reservations.Min.Y >= header.Max.Y || reservations.Max.Y <= header.Min.Y {
		t.Fatalf("manage reservations is no longer next to the active source heading: %v / %v", reservations, header)
	}
	assertPageLabel(t, &h.router, "Availability needs more information")

	var expected string
	ref := domain.AccountValueSourceRef(ids.bank)
	for _, source := range funds.current.report.Sources {
		if source.Result.Ref.Key() == ref.Key() {
			result := fundsResultForHorizon(source.Result, funds.selectedFundsBucket().HorizonOn)
			if result == nil || result.SelectedRoute == nil || result.SelectedRoute.ReceiptOn == nil {
				t.Fatal("fixture cash must have a known receipt date")
			}
			expected = "Expected availability · " + *result.SelectedRoute.ReceiptOn
			break
		}
	}
	if expected == "" {
		t.Fatal("fixture cash source was not read")
	}
	const name = "Harbor Savings"
	const current = "Current value · US$125.43"
	const after = "Available after reservations · US$85.31"
	const action = "Actions for Harbor Savings · USD"
	wideHeaders := []string{"Asset", "Current value", "Expected availability", "Spendable", "Actions"}
	fields := []string{name, current, expected, after, action}
	lastX := 0
	for i, label := range fields {
		header := assertPageLabel(t, &h.router, wideHeaders[i])
		field := assertPageLabel(t, &h.router, label)
		if lastX >= field.Min.X || header.Min.Y >= field.Min.Y ||
			header.Max.X <= field.Min.X || field.Max.X <= header.Min.X {
			t.Fatalf("%q does not align with its wide heading: %v, %v", label, field, header)
		}
		lastX = field.Max.X - 1
	}
	assertPageLabel(t, &h.router, "Reserved from available funds · US$40.12")
	chooseFundsSourceAction(t, h, "Harbor Savings · USD", "Manage reservations")
	settle()
	if funds.reservation == nil || funds.reservation.source.Key() != ref.Key() {
		t.Fatal("the compact action did not retain its source scope")
	}
	h.click("Close", false)
	settle()
	h.focused(&funds.links[ref.Key()].menu.Trigger)

	setWidth(620)
	if findPageLabel(&h.router, "Asset") != nil || findPageLabel(&h.router, "Actions") != nil {
		t.Fatal("the narrow source list still showed its wide headings")
	}
	narrowName := assertPageLabel(t, &h.router, name)
	narrowCurrent := assertPageLabel(t, &h.router, current)
	narrowAction := assertPageLabel(t, &h.router, action)
	assertPageLabel(t, &h.router, expected)
	assertPageLabel(t, &h.router, after)
	if narrowName.Min.Y >= narrowCurrent.Min.Y || narrowAction.Min.Y >= narrowCurrent.Min.Y {
		t.Fatalf("narrow source did not place labelled values below identity and actions: %v %v %v", narrowName, narrowCurrent, narrowAction)
	}
	if app.MobileDataVersion() != version {
		t.Fatal("viewing the source layout and reservation list wrote to the ledger")
	}
}
