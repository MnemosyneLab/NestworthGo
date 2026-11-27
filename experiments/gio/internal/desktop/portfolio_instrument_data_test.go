package desktop

import (
	"context"
	"errors"
	"image"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	quoteapi "github.com/waltwang/nestworth-go/internal/wailsapi/quote"
)

type instrumentHistoryReadFailure struct {
	*application.Service
	failFX atomic.Bool
}

func (r *instrumentHistoryReadFailure) FXQuoteSeries(ctx context.Context, a, b domain.CurrencyCode, period domain.TrendRange, filter domain.QuoteSourceFilter) (domain.QuoteSeries, error) {
	if r.failFX.Swap(false) {
		return domain.QuoteSeries{}, errors.New("synthetic unavailable FX history")
	}
	return r.Service.FXQuoteSeries(ctx, a, b, period, filter)
}

func TestPortfolioInstrumentDataKeepsQuoteAndAccountScope(t *testing.T) {
	ctx := t.Context()
	service, _, _, ids := openPortfolioFixture(t, "partial")
	instrument := ids.instruments["QA-FULL"]
	clock := func() time.Time { return accountTestClock }
	portfolio := newPortfolioPage(ctx, service, func() {})
	reader := &instrumentHistoryReadFailure{Service: service}
	reader.failFX.Store(true)
	market := &marketDataPage{ctx: ctx, reader: service, invalidate: func() {}, quoteHistoryReader: reader, quoteHistoryClock: clock,
		instrumentSources: service, manual: marketManualQuotes{service: service, clock: clock}}
	t.Cleanup(portfolio.workers.Wait)
	t.Cleanup(market.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: portfolio, marketData: market, version: service.MobileDataVersion})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1280, 1080)

	settle := func() {
		for range 3 {
			h.frame()
			portfolio.workers.Wait()
			market.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	label := func(label string) { t.Helper(); assertPageLabel(t, &h.router, label) }

	settle()
	portfolio.openHoldingInstrument(instrument.ID, ids.account, instrument.DisplayLabel(), nil)
	settle()
	label("Cost basis · €50.30")
	if portfolio.detail.histories[instrumentPriceHistory] != nil {
		t.Fatal("Holdings requested saved quote history")
	}
	click("Data records")
	label("Latest quote")
	label("€30.1234")
	if findPageLabel(&h.router, "Cost basis · €50.30") != nil {
		t.Fatal("Data rendered the holdings figures")
	}
	version := service.MobileDataVersion()
	history := portfolio.detail.histories[instrumentPriceHistory]
	if market.quoteHistory != nil || history == nil || history.visit.target.instrument != instrument.ID || history.result == nil || history.result.current == nil ||
		history.result.current.Value != "30.1234" || history.result.current.Currency != "EUR" {
		t.Fatalf("inline history did not use the exact instrument's selected quote: %+v", history)
	}
	click("Manual quote")
	click("Last week")
	adapter := quoteapi.NewService(service)
	period := history.visit.window.from + ":" + history.visit.window.to
	want, err := adapter.InstrumentQuoteSeries(ctx, instrument.ID.String(), period, "manual")
	if err != nil || len(want.Observations) != 1 || len(history.result.series.Observations) != 1 ||
		history.result.series.Observations[0].Value != want.Observations[0].Value {
		t.Fatalf("Gio and Wails quote observations differ: %+v / %+v / %v", history.result.series, want, err)
	}
	label("Quote observations over time")
	click("View data table")
	if !history.table.open {
		t.Fatal("the exact price table did not open within Data")
	}
	priceWindow := history.visit.window

	click("FX history")
	label("Could not read saved market data.")
	click("Retry")
	fx := portfolio.detail.histories[instrumentFXHistory]
	if fx == nil || fx.result == nil || fx.err != nil || len(fx.result.series.Observations) != 1 {
		t.Fatalf("the inline FX history did not recover: %+v", fx)
	}
	label("Historical direction · 1 EUR in USD")
	if fx.result.series.Observations[0].Value != "1.2" {
		t.Fatalf("the initial FX history did not match the instrument to base rate: %+v", fx.result.series)
	}
	click("Swap direction")
	label("Historical direction · 1 USD in EUR")
	fxPeriod := fx.visit.window.from + ":" + fx.visit.window.to
	wantFX, err := adapter.FXQuoteSeries(ctx, "USD", "EUR", fxPeriod, "all")
	if err != nil || len(wantFX.Observations) != 1 || len(fx.result.series.Observations) != 1 ||
		fx.result.series.Observations[0].Value != wantFX.Observations[0].Value {
		t.Fatalf("Gio and Wails reverse FX observations differ: %+v / %+v / %v", fx.result.series, wantFX, err)
	}
	click("Price history")
	if history.visit.filter != 1 || history.visit.window != priceWindow || !history.table.open {
		t.Fatal("switching history left behind the price range, source or table state")
	}
	portfolio.list.Position = layout.Position{}
	settle()

	click("Edit quote source")
	if d := market.instrumentSource; d == nil || d.id != instrument.ID {
		t.Fatal("the quote source editor opened on another instrument")
	}
	h.press(key.NameEscape)
	settle()
	h.focused(&portfolio.detail.source)
	if service.MobileDataVersion() != version {
		t.Fatal("browsing history or canceling quote maintenance wrote to the ledger")
	}

	click("Set price")
	draft := market.manual.draft
	if draft == nil || draft.target.instrumentID != instrument.ID || draft.refs == nil {
		t.Fatal("manual price did not resolve the instrument from Data")
	}
	setBankField(h, "Unit price · EUR", &draft.amount, "31.12345678")
	click("Save price")
	label("€31.12345678")
	quote, err := adapter.CurrentInstrumentQuote(ctx, instrument.ID.String())
	if err != nil || quote == nil || quote.UnitPrice != "31.12345678" {
		t.Fatalf("could not read the exact saved price through Wails: %+v / %v", quote, err)
	}
	h.focused(&portfolio.detail.manual)
	click("Investment assets")
	click("Back")
	if portfolio.detail.visit.section != instrumentDetailData || portfolio.id != instrument.ID || portfolio.accountScope != ids.account {
		t.Fatal("Back did not restore the instrument's Data tab and account scope")
	}
	label("€31.12345678")
	h.focused(&portfolio.detail.manual)
	refreshed := portfolio.detail.histories[instrumentPriceHistory]
	if refreshed == nil || refreshed.result == nil || refreshed.visit.window != priceWindow || refreshed.visit.filter != 1 || !refreshed.table.open ||
		!slices.ContainsFunc(refreshed.result.series.Observations, func(point domain.QuoteSeriesPoint) bool { return point.Value == "31.12345678" }) {
		t.Fatal("returning to Data did not retain the price settings and refresh the saved observations")
	}
	click("My holdings")
	label("Cost basis · €50.30")
	if len(portfolio.current.positions) != 1 || len(portfolio.current.positions[0].Holdings) != 1 ||
		portfolio.current.positions[0].Holdings[0].AccountID != ids.account {
		t.Fatal("the scoped holdings changed after saving a quote")
	}
}
