package desktop

import (
	"context"
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestPortfolioCatalogActionsTrailAt900x600ShellViewport(t *testing.T) {
	ctx := t.Context()
	service, session, _, _ := openPortfolioFixture(t, "empty")
	t.Cleanup(func() { _ = session.Close() })
	quotedAt := accountTestClock.Add(-time.Hour).Format(time.RFC3339)
	for _, instrument := range []struct{ name, shortName, symbol, kind, price string }{
		{"Synthetic AAPL issuer", "QA-AAPL", "QA-AAPL", "stock", "203"},
		{"Synthetic ES3 issuer", "QA-ES3", "QA-ES3", "etf", "101"},
		{"Synthetic QQQ issuer", "QA-QQQ", "QA-QQQ", "etf", "99"},
	} {
		shortName := instrument.shortName
		item, err := service.CreateInstrument(ctx, application.InstrumentInput{
			Name: instrument.name, ShortName: &shortName, Symbol: instrument.symbol,
			Type: instrument.kind, QuoteCurrency: "USD", QuoteSource: "manual",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.AppendManualInstrumentQuote(ctx, item.ID, instrument.price, quotedAt, false); err != nil {
			t.Fatal(err)
		}
	}
	portfolio := newPortfolioPage(ctx, service, func() {})
	t.Cleanup(portfolio.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: portfolio})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(900, 600)
	settle := func() {
		for range 4 {
			h.frame()
			portfolio.workers.Wait()
		}
		h.frame()
	}
	settle()
	h.click("Investment assets", false)
	settle()
	h.click("All instruments", false)
	settle()

	if h.drawn != image.Pt(900, 600) {
		t.Fatalf("shell test viewport = %v, want the actual 900×600 shell viewport", h.drawn)
	}
	assertCatalogRows := func(width, height int) {
		t.Helper()
		if h.drawn != image.Pt(width, height) {
			t.Fatalf("shell test viewport = %v, want %d×%d", h.drawn, width, height)
		}
		for _, label := range []string{"QA-AAPL", "QA-ES3", "QA-QQQ"} {
			bounds := assertPageLabel(t, &h.router, label)
			if bounds.Min.Y < 0 || bounds.Min.Y >= height {
				t.Fatalf("catalog identity %q is outside the %d×%d viewport: %v", label, width, height, bounds)
			}
		}
		identity := assertPageLabel(t, &h.router, "QA-AAPL")
		quote := assertPageLabel(t, &h.router, "Latest quote · US$203.00")
		details := semanticBounds(t, &h.router, fmt.Sprintf(i18n.English.Text(i18n.PortfolioCatalogOpen), "QA-AAPL"), false)
		edit := semanticBounds(t, &h.router, fmt.Sprintf(i18n.English.Text(i18n.PortfolioCatalogEditNamed), "QA-AAPL"), false)
		if quote.Empty() || quote.Min.X <= identity.Max.X || details.Min.X <= quote.Max.X || edit.Min.X <= details.Max.X ||
			quote.Min.Y > identity.Min.Y+40 || details.Min.Y > identity.Min.Y+40 || edit.Min.Y > identity.Min.Y+40 ||
			absTitlebarPixel(details.Min.Y-edit.Min.Y) > 8 {
			t.Fatalf("the shell's measured %d×%d catalog content must keep quote and actions trailing on the identity row: identity %v, quote %v, details %v, edit %v",
				width, height, identity, quote, details, edit)
		}
	}
	assertCatalogRows(900, 600)
	h.size = image.Pt(1280, 800)
	settle()
	assertCatalogRows(1280, 800)
}

func TestMarketRowActionLayoutKeepsNarrowAndLongLabelFallback(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(748, 600)}}
	view := ui.View{Theme: ui.NewTheme(), Language: i18n.English, Colors: &lightPalette}
	spec := marketRowSpec{name: "QA-AAPL", caption: "Synthetic AAPL issuer",
		badges: []marketBadge{{text: "USD"}, {text: "Manual"}},
		value:  "Latest quote · US$203", evidence: "Manual · Nov 27, 2026, 8:00 AM"}
	if !marketRowActionsAreTrailing(gtx, view, spec, 2) {
		t.Fatal("ordinary catalog labels and two actions should fit the measured 900px shell content")
	}
	gtx.Constraints.Max.X = 400
	if marketRowActionsAreTrailing(gtx, view, spec, 2) {
		t.Fatalf("actions should stack when the available row width is genuinely narrow: required %dpx", marketRowActionsMinimumWidth(gtx, view, spec, 2))
	}
	gtx.Constraints.Max.X = 748
	long := spec
	long.name = strings.Repeat("Long instrument label ", 4)
	if marketRowActionsAreTrailing(gtx, view, long, 2) {
		t.Fatalf("very long instrument identity should retain the stacked action fallback: required %dpx", marketRowActionsMinimumWidth(gtx, view, long, 2))
	}
}

func TestPortfolioCatalogSortMatchesWailsTypeCurrencyAndLabelOrder(t *testing.T) {
	stringValue := func(value string) *string { return &value }
	items := []domain.Instrument{
		{ID: domain.InstrumentID("stock-usd-beta"), Type: domain.InstrumentStock, QuoteCurrency: "USD", Name: "Beta fund", ShortName: "Beta"},
		{ID: domain.InstrumentID("fund-usd"), Type: domain.InstrumentETF, QuoteCurrency: "USD", Name: "Fund"},
		{ID: domain.InstrumentID("stock-eur-zulu"), Type: domain.InstrumentStock, QuoteCurrency: "EUR", Name: "Zulu", Symbol: stringValue("ZULU")},
		{ID: domain.InstrumentID("stock-usd-alpha"), Type: domain.InstrumentStock, QuoteCurrency: "USD", Name: "Alpha", Symbol: stringValue("ALPHA")},
	}

	got := sortPortfolioCatalogInstruments(items, i18n.Language(settings.LanguageEnglish))
	ids := make([]string, len(got))
	for index, item := range got {
		ids[index] = item.ID.String()
	}
	want := []string{"stock-eur-zulu", "stock-usd-alpha", "stock-usd-beta", "fund-usd"}
	if !slices.Equal(ids, want) {
		t.Fatalf("catalog order = %v, want Wails type/currency/display-label order %v", ids, want)
	}
}

func TestPortfolioCatalogSearchUsesWailsIdentityFields(t *testing.T) {
	stringValue := func(value string) *string { return &value }
	item := domain.Instrument{
		Name: "Issuer full name", ShortName: "Short label", Type: domain.InstrumentStock, QuoteCurrency: "USD",
		Symbol: stringValue("SYM"), ISIN: stringValue("US0123456789"), MarketCode: stringValue("NASDAQ"),
		ProviderSymbol: stringValue("SYM.NASDAQ"), ProviderKey: stringValue("yahoo"),
	}
	for _, query := range []string{"short label", "issuer full name", "sym", "us0123456789", "nasdaq", "sym.nasdaq", "usd"} {
		if !catalogMatches(item, normalizeDirectoryQuery(query)) {
			t.Errorf("catalog search %q did not match an identity field used by Wails", query)
		}
	}
	if !catalogMatches(item, normalizeDirectoryQuery("short label issuer")) {
		t.Fatal("catalog search should preserve Wails' short-name-then-full-name phrase order")
	}
	for _, query := range []string{"issuer full name short label", "issuer short label"} {
		if catalogMatches(item, normalizeDirectoryQuery(query)) {
			t.Errorf("catalog search %q should not match the reverse field order", query)
		}
	}
	missingShortName := item
	missingShortName.ShortName = ""
	if !catalogMatches(missingShortName, normalizeDirectoryQuery("issuer full name sym")) {
		t.Fatal("catalog search should skip the empty short-name field between full name and symbol")
	}
	if catalogMatches(item, normalizeDirectoryQuery("yahoo")) {
		t.Fatal("catalog search should not match provider key, which Wails does not search")
	}
}

func TestPortfolioCatalogRequestedEditorUsesCurrentInstrument(t *testing.T) {
	ctx := t.Context()
	service, _, _, _ := openPortfolioFixture(t, "partial")
	added, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic maintenance bond", Symbol: "QA-EDIT", Type: "bond", QuoteCurrency: "EUR", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	portfolio := newPortfolioPage(ctx, service, func() {})
	t.Cleanup(portfolio.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: portfolio})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1120, 950)
	frame := func() {
		for range 4 {
			h.frame()
			portfolio.workers.Wait()
		}
		h.frame()
	}
	var completions []i18n.MessageID
	portfolio.catalog.finishExternal = func(message i18n.MessageID) { completions = append(completions, message) }
	frame()
	h.click("Investment assets", false)
	frame()
	h.click("All instruments", false)
	frame()

	older := added
	older.Name = "Earlier stale name"
	portfolio.read()
	portfolio.requestCatalogEditor(older)
	frame()
	draft := portfolio.catalog.draft
	if draft == nil || !draft.external || draft.instrument.ID != added.ID || draft.fields[portfolioInstrumentName].Text() != added.Name {
		t.Fatal("opening through maintenance did not start from the current identity")
	}
	setBankField(h, "Name", &draft.fields[portfolioInstrumentName], "Updated maintenance bond")
	h.click("Save", false)
	frame()
	items, err := service.ListInstruments(ctx, true)
	if err != nil || !slices.ContainsFunc(items, func(item domain.Instrument) bool {
		return item.ID == added.ID && item.Name == "Updated maintenance bond" && item.QuoteCurrency == "EUR"
	}) || !slices.Equal(completions, []i18n.MessageID{i18n.PortfolioCatalogSaved}) {
		t.Fatalf("the shared editor did not save the same instrument and report completion: %v / %v", err, completions)
	}
	portfolio.requestCatalogEditor(domain.Instrument{})
	frame()
	draft = portfolio.catalog.draft
	if draft == nil || !draft.external || draft.instrument.ID != "" || draft.config.currency != portfolio.current.summary.Currency {
		t.Fatal("opening Add instrument through maintenance lost the shared household default")
	}
	draft.save.Click()
	frame()
	assertPageLabel(t, &h.router, "Must not be empty")
	h.focused(&draft.fields[portfolioInstrumentName].Editor)
	h.click("Close", false)
	frame()
	if portfolio.catalog.draft != nil || !slices.Equal(completions, []i18n.MessageID{i18n.PortfolioCatalogSaved, i18n.NoMessage}) {
		t.Fatal("cancel did not return to the requesting page without a save")
	}
}

type catalogLookupFixture struct {
	*application.Service
}

func (s catalogLookupFixture) SearchInstruments(_ context.Context, query, kind string) ([]application.InstrumentSearchHit, error) {
	if query != "synthetic" || kind != string(domain.InstrumentStock) {
		return nil, nil
	}
	return []application.InstrumentSearchHit{{
		ProviderKey: application.YahooFinanceProviderKey, ProviderSymbol: "QA-STOCK", Symbol: "QA-STOCK",
		Name: "Synthetic lookup stock", Type: kind, QuoteCurrency: "AUD", MarketCode: "ASX", CountryCode: "AU",
	}}, nil
}

func TestPortfolioCatalogCreateFromProviderPopulatesAndPreservesIdentity(t *testing.T) {
	ctx := t.Context()
	service, storage, paths, _ := openPortfolioFixture(t, "partial")
	port := newPortfolioPage(ctx, catalogLookupFixture{service}, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: port})
	h.size = image.Pt(1120, 1140)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 4 {
			h.frame()
			port.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		settle()
	}
	settle()
	click("Investment assets")
	click("All instruments")
	click("Add instrument")
	d := port.catalog.draft
	if d == nil || d.config.currency != port.current.summary.Currency || d.config.source != domain.QuoteSourceProvider {
		t.Fatal("add instrument did not use the household currency and stock provider default")
	}
	setBankField(h, "Name", &d.fields[portfolioInstrumentName], "Temporary")
	d.save.Click()
	settle()
	assertPageLabel(t, &h.router, "Choose a provider and enter its symbol.")
	if d.fields[portfolioInstrumentName].Text() != "Temporary" {
		t.Fatal("an invalid submission discarded the draft")
	}
	setBankField(h, "Provider lookup query", &d.config.lookup.query, "synthetic")
	settle()
	click("Look up instrument")
	hit := d.config.lookup.hits[0]
	click(instrumentBindingHitLabel(hit))
	if d.config.currency != "AUD" || d.fields[portfolioInstrumentMarket].Text() != "ASX" ||
		d.fields[portfolioInstrumentName].Text() != "Synthetic lookup stock" {
		t.Fatal("provider selection did not fill the identity and currency")
	}
	d.save.Click()
	settle()
	assertPageLabel(t, &h.router, "Instrument added")
	h.focused(&port.catalog.add)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	items, err := reopened.ListInstruments(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(items, func(item domain.Instrument) bool { return item.Name == "Synthetic lookup stock" })
	if index < 0 {
		t.Fatal("new instrument did not persist")
	}
	item := items[index]
	if item.Type != domain.InstrumentStock || item.QuoteCurrency != "AUD" || item.QuoteSource != domain.QuoteSourceProvider ||
		productString(item.ProviderKey) != application.YahooFinanceProviderKey || productString(item.ProviderSymbol) != "QA-STOCK" ||
		productString(item.MarketCode) != "ASX" || productString(item.CountryCode) != "AU" {
		t.Fatalf("saved metadata differs from the selected instrument: %+v", item)
	}
}

func TestPortfolioCatalogShowsUnheldInstrumentsAndArchivesWithConfirmation(t *testing.T) {
	ctx := t.Context()
	service, storage, paths, _ := openPortfolioFixture(t, "partial")
	instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic unheld bond", Symbol: "QA-BOND", Type: "bond", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	quotedAt := accountTestClock.Add(-time.Hour)
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "5.6789123", quotedAt.Format(time.RFC3339), false); err != nil {
		t.Fatal(err)
	}
	port := newPortfolioPage(ctx, service, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: port})
	h.size = image.Pt(820, 760)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 4 {
			h.frame()
			port.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) {
		t.Helper()
		h.click(label, false)
		settle()
	}
	current := func(svc *application.Service) domain.Instrument {
		t.Helper()
		instruments, err := svc.ListInstruments(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(instruments, func(item domain.Instrument) bool { return item.ID == instrument.ID })
		if i < 0 {
			t.Fatal("instrument missing")
		}
		return instruments[i]
	}
	settle()
	click("Investment assets")
	click("All instruments")
	if port.current.catalog == nil || port.current.groups != nil || port.tabs.active != portfolioInstruments {
		t.Fatal("the instrument tab did not load the catalog")
	}
	setBankField(h, "Search instruments", &port.catalog.search.Field, "qa-bond")
	settle()
	assertPageLabel(t, &h.router, instrument.DisplayLabel())
	assertPageLabel(t, &h.router, "Latest quote · US$5.6789123")
	if quote := port.current.catalog.quotes[instrument.ID]; quote == nil || !quote.QuotedAt.Equal(quotedAt) || quote.SourceKind != domain.QuoteSourceManual {
		t.Fatalf("catalog changed the saved quote provenance: %+v", quote)
	}
	click("Details")
	if port.id != instrument.ID || port.current.instrument == nil || len(port.current.positions) != 0 {
		t.Fatal("unheld instrument did not open restored instrument details")
	}
	click("Back")
	if port.tabs.active != portfolioInstruments || port.catalog.search.Field.Text() != "qa-bond" {
		t.Fatal("Back did not restore the instrument list and query")
	}
	h.focused(&port.catalog.links[instrument.ID].open)
	click("Edit")
	click("Archive")
	click("Cancel")
	if current(service).ArchivedAt != nil {
		t.Fatal("cancel wrote an archive")
	}
	click("Archive")
	click("Archive")
	if current(service).ArchivedAt == nil {
		t.Fatal("confirmed archive did not save")
	}
	assertPageLabel(t, &h.router, "Entry archived.")
	h.focused(&port.catalog.links[instrument.ID].manage)
	click("Edit")
	click("Restore")
	click("Restore")
	assertPageLabel(t, &h.router, "Entry restored.")
	click("Edit")
	draft := port.catalog.draft
	setBankField(h, "Name", &draft.fields[portfolioInstrumentName], "Renamed unheld bond")
	click("Save")
	assertPageLabel(t, &h.router, "Saved locally")
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	if instrument := current(reopened); instrument.ArchivedAt != nil || instrument.Name != "Renamed unheld bond" ||
		instrument.QuoteCurrency != "USD" || instrument.QuoteSource != domain.QuoteSourceManual {
		t.Fatalf("catalog change did not preserve the expected identity and denomination after reopen: %+v", instrument)
	}
}
