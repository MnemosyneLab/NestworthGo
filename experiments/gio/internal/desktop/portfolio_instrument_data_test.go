package desktop

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/settings"
	quoteapi "github.com/waltwang/nestworth-go/internal/wailsapi/quote"
)

func TestPortfolioInstrumentDataKeepsQuoteAndAccountScope(t *testing.T) {
	ctx := t.Context()
	service, _, _, ids := openPortfolioFixture(t, "partial")
	instrument := ids.instruments["QA-FULL"]
	clock := func() time.Time { return accountTestClock }
	portfolio := newPortfolioPage(ctx, service, func() {})
	market := &marketDataPage{ctx: ctx, reader: service, invalidate: func() {}, quoteHistoryReader: service, quoteHistoryClock: clock,
		instrumentSources: service, manual: marketManualQuotes{service: service, clock: clock}}
	t.Cleanup(portfolio.workers.Wait)
	t.Cleanup(market.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: portfolio, marketData: market, version: service.MobileDataVersion})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1200, 960)

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
	click("Data")
	label("Current price · EUR 30.1234")
	if findPageLabel(&h.router, "Cost basis · €50.30") != nil {
		t.Fatal("Data rendered the holdings figures")
	}
	version := service.MobileDataVersion()
	click("View quote history")
	history := market.quoteHistory
	if history == nil || history.visit.target.instrument != instrument.ID || history.result == nil || history.result.current == nil ||
		history.result.current.Value != "30.1234" || history.result.current.Currency != "EUR" {
		t.Fatalf("quote history did not show the exact instrument's selected quote: %+v", history)
	}
	adapter := quoteapi.NewService(service)
	period := history.visit.window.from + ":" + history.visit.window.to
	want, err := adapter.InstrumentQuoteSeries(ctx, instrument.ID.String(), period, "all")
	if err != nil || len(want.Observations) != 1 || len(history.result.series.Observations) != 1 ||
		history.result.series.Observations[0].Value != want.Observations[0].Value {
		t.Fatalf("Gio and Wails quote observations differ: %+v / %+v / %v", history.result.series, want, err)
	}
	h.press(key.NameEscape)
	settle()
	h.focused(&portfolio.detail.history)

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
	label("Current price · EUR 31.12345678")
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
	label("Current price · EUR 31.12345678")
	h.focused(&portfolio.detail.manual)
	click("Holdings")
	label("Cost basis · €50.30")
	if len(portfolio.current.positions) != 1 || len(portfolio.current.positions[0].Holdings) != 1 ||
		portfolio.current.positions[0].Holdings[0].AccountID != ids.account {
		t.Fatal("the scoped holdings changed after saving a quote")
	}
}
