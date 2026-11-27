package desktop

import (
	"context"
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/font"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
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
	h.shell.preferences.language = i18n.English
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.theme.Face = font.Typeface("DejaVu Sans")
	h.locale = system.Locale{Language: "en-US"}
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
	assertCatalogRows := func(language i18n.Language, width, height int) {
		t.Helper()
		if h.drawn != image.Pt(width, height) {
			t.Fatalf("shell test viewport = %v, want %d×%d", h.drawn, width, height)
		}
		aapl := assertPageLabel(t, &h.router, "QA-AAPL")
		es3 := assertPageLabel(t, &h.router, "QA-ES3")
		qqq := assertPageLabel(t, &h.router, "QA-QQQ")
		aaplActions := semanticBounds(t, &h.router, fmt.Sprintf(language.Text(i18n.PortfolioCatalogOpen), "QA-AAPL"), false)
		es3Actions := semanticBounds(t, &h.router, fmt.Sprintf(language.Text(i18n.PortfolioCatalogOpen), "QA-ES3"), false)
		aaplEdit := semanticBounds(t, &h.router, fmt.Sprintf(language.Text(i18n.PortfolioCatalogEditNamed), "QA-AAPL"), false)
		es3Edit := semanticBounds(t, &h.router, fmt.Sprintf(language.Text(i18n.PortfolioCatalogEditNamed), "QA-ES3"), false)
		if aapl.Min.Y < 0 || aapl.Min.Y >= height || es3.Min.Y < 0 || es3.Min.Y >= height || qqq.Min.Y >= height ||
			aaplActions.Min.Y < 0 || aaplActions.Max.Y > height || es3Actions.Min.Y < 0 || es3Actions.Max.Y > height ||
			aaplEdit.Min.Y < 0 || aaplEdit.Max.Y > height || es3Edit.Min.Y < 0 || es3Edit.Max.Y > height {
			t.Fatalf("the 900px catalog viewport should show AAPL and ES3 and the start of QQQ: AAPL %v, ES3 %v, QQQ %v", aapl, es3, qqq)
		}
		if qqq.Min.Y-es3.Min.Y > 120 {
			t.Fatalf("the measured ETF row height should keep QQQ near ES3 in the viewport: ES3 %v, QQQ %v", es3, qqq)
		}
		identity := aapl
		quote := assertPageLabel(t, &h.router, language.Text(i18n.PortfolioInstrumentLatestQuote)+" · US$203.00")
		details := semanticBounds(t, &h.router, fmt.Sprintf(language.Text(i18n.PortfolioCatalogOpen), "QA-AAPL"), false)
		edit := semanticBounds(t, &h.router, fmt.Sprintf(language.Text(i18n.PortfolioCatalogEditNamed), "QA-AAPL"), false)
		if quote.Empty() || details.Empty() || edit.Empty() || absTitlebarPixel(details.Min.Y-identity.Min.Y) > 12 || absTitlebarPixel(details.Min.Y-edit.Min.Y) > 1 {
			t.Fatalf("the measured %d×%d catalog identity and action controls should share a compact top line: identity %v, quote %v, details %v, edit %v",
				width, height, identity, quote, details, edit)
		}
		if width == 900 {
			if quote.Empty() || quote.Min.X <= identity.Min.X || quote.Max.X != edit.Max.X {
				t.Fatalf("the compact catalog row should right-align its quote below the identity/actions: identity %v, quote %v, details %v, edit %v", identity, quote, details, edit)
			}
		} else if quote.Min.X <= identity.Max.X || details.Min.X <= quote.Max.X || edit.Min.X <= details.Max.X {
			t.Fatalf("the wide catalog row should keep quote and actions trailing across the identity row: identity %v, quote %v, details %v, edit %v", identity, quote, details, edit)
		}
	}
	for _, sample := range []struct {
		language i18n.Language
		setting  settings.Language
		system   string
		face     string
	}{
		{i18n.English, settings.LanguageEnglish, "en-US", "DejaVu Sans"},
		{i18n.SimplifiedChinese, settings.LanguageZhCN, "zh-CN", "Noto Sans CJK SC, DejaVu Sans"},
	} {
		h.shell.preferences.language = sample.language
		h.shell.prefs.saved.Language = sample.setting
		h.shell.theme.Face = font.Typeface(sample.face)
		h.locale = system.Locale{Language: sample.system}
		h.size = image.Pt(900, 600)
		settle()
		assertCatalogRows(sample.language, 900, 600)
		h.size = image.Pt(1280, 800)
		settle()
		assertCatalogRows(sample.language, 1280, 800)
	}
}

func TestMarketRowActionLayoutKeepsNarrowAndLongLabelFallback(t *testing.T) {
	render := func(width int, name string, language i18n.Language, setting settings.Language, face, locale string) (*previewHarness, image.Point) {
		t.Helper()
		h := newShellHarness(t)
		h.size = image.Pt(width, 600)
		h.shell.preferences.language = language
		h.shell.prefs.saved.Language = setting
		h.shell.theme.Face = font.Typeface(face)
		h.locale = system.Locale{Language: locale}
		var rowSize image.Point
		var details, edit widget.Clickable
		h.shell.routes[overviewRoute].body = func(gtx layout.Context, view ui.View) layout.Dimensions {
			lang := view.Language
			actions := []layout.Widget{
				view.ButtonOf(&details, ui.ButtonSpec{Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: lang.Text(i18n.PortfolioCatalogDetails), Name: fmt.Sprintf(lang.Text(i18n.PortfolioCatalogOpen), name)}),
				view.ButtonOf(&edit, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: lang.Text(i18n.InstitutionsEdit), Name: fmt.Sprintf(lang.Text(i18n.PortfolioCatalogEditNamed), name)}),
			}
			spec := marketRowSpec{
				icon: lucide.TrendingUp, name: name, caption: "Synthetic issuer full name", badges: marketBadges(view, "USD", domain.QuoteSourceManual),
				value: lang.Text(i18n.PortfolioInstrumentLatestQuote) + " · US$203.00", evidence: "Manual · Nov 27, 2026, 8:00 AM",
				catalogActions: true, actionLabels: []string{lang.Text(i18n.PortfolioCatalogDetails), lang.Text(i18n.InstitutionsEdit)},
			}
			row := marketQuoteRow(view, true, true, spec, actions)
			dims := row(gtx)
			rowSize = dims.Size
			return dims
		}
		h.frame()
		return h, rowSize
	}
	assertCompact := func(h *previewHarness, height int, language i18n.Language, name string) {
		t.Helper()
		identity := assertPageLabel(t, &h.router, name)
		quote := assertPageLabel(t, &h.router, language.Text(i18n.PortfolioInstrumentLatestQuote)+" · US$203.00")
		details := semanticBounds(t, &h.router, fmt.Sprintf(language.Text(i18n.PortfolioCatalogOpen), name), false)
		edit := semanticBounds(t, &h.router, fmt.Sprintf(language.Text(i18n.PortfolioCatalogEditNamed), name), false)
		if height > 110 || absTitlebarPixel(details.Min.Y-identity.Min.Y) > 12 || absTitlebarPixel(details.Min.Y-edit.Min.Y) > 1 ||
			quote.Empty() || quote.Max.X != edit.Max.X {
			t.Fatalf("compact row geometry should keep actions with identity and quote/timestamp right-aligned below: height %d, identity %v, quote %v, details %v, edit %v", height, identity, quote, details, edit)
		}
	}
	english, compactHeight := render(900, "QA-AAPL", i18n.English, settings.LanguageEnglish, "DejaVu Sans", "en-US")
	assertCompact(english, compactHeight.Y, i18n.English, "QA-AAPL")
	chinese, chineseHeight := render(900, "QA-AAPL", i18n.SimplifiedChinese, settings.LanguageZhCN, "Noto Sans CJK SC, DejaVu Sans", "zh-CN")
	assertCompact(chinese, chineseHeight.Y, i18n.SimplifiedChinese, "QA-AAPL")
	narrow, narrowHeight := render(400, "QA-AAPL", i18n.English, settings.LanguageEnglish, "DejaVu Sans", "en-US")
	narrowQuote := assertPageLabel(t, &narrow.router, "Latest quote · US$203.00")
	narrowDetails := semanticBounds(t, &narrow.router, fmt.Sprintf(i18n.English.Text(i18n.PortfolioCatalogOpen), "QA-AAPL"), false)
	if narrowHeight.Y <= compactHeight.Y || narrowDetails.Min.Y <= narrowQuote.Min.Y {
		t.Fatalf("a narrow viewport should stack row actions below quote details: compact %v, narrow %v, quote %v, actions %v", compactHeight, narrowHeight, narrowQuote, narrowDetails)
	}
	longName := strings.Repeat("Long synthetic issuer identity ", 8)
	long, longHeight := render(900, longName, i18n.English, settings.LanguageEnglish, "DejaVu Sans", "en-US")
	longQuote := assertPageLabel(t, &long.router, "Latest quote · US$203.00")
	longDetails := semanticBounds(t, &long.router, fmt.Sprintf(i18n.English.Text(i18n.PortfolioCatalogOpen), longName), false)
	if longHeight.Y <= compactHeight.Y || longDetails.Min.Y <= longQuote.Min.Y {
		t.Fatalf("a long identity should retain the stacked fallback: compact %v, long %v, quote %v, actions %v", compactHeight, longHeight, longQuote, longDetails)
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
	click(fmt.Sprintf(h.shell.preferences.language.Text(i18n.PortfolioCatalogOpen), instrument.DisplayLabel()))
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
