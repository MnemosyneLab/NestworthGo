package desktop

import (
	"context"
	"image"
	"slices"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

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
	setBankField(h, "Instrument name", &draft.fields[portfolioInstrumentName], "Updated maintenance bond")
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
	h.click("Cancel", false)
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
	setBankField(h, "Instrument name", &d.fields[portfolioInstrumentName], "Temporary")
	click("Save")
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
	click("Save")
	assertPageLabel(t, &h.router, "Instrument added.")
	h.focused(&port.catalog.add)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
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
	assertPageLabel(t, &h.router, "Current price · USD 5.6789123")
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
	click("Manage instrument")
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
	click("Manage instrument")
	click("Restore")
	click("Restore")
	assertPageLabel(t, &h.router, "Entry restored.")
	click("Manage instrument")
	draft := port.catalog.draft
	setBankField(h, "Instrument name", &draft.fields[portfolioInstrumentName], "Renamed unheld bond")
	click("Save")
	assertPageLabel(t, &h.router, "Instrument saved.")
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	if instrument := current(reopened); instrument.ArchivedAt != nil || instrument.Name != "Renamed unheld bond" ||
		instrument.QuoteCurrency != "USD" || instrument.QuoteSource != domain.QuoteSourceManual {
		t.Fatalf("catalog change did not preserve the expected identity and denomination after reopen: %+v", instrument)
	}
}
