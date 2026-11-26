package desktop

import (
	"image"
	"slices"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestPortfolioCatalogShowsUnheldInstrumentsAndArchivesWithConfirmation(t *testing.T) {
	ctx := t.Context()
	service, storage, paths, _ := openPortfolioFixture(t, "partial")
	instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic unheld bond", Symbol: "QA-BOND", Type: "bond", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
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
	click("Instruments")
	if port.current.catalog == nil || port.current.groups != nil || !port.catalog.active {
		t.Fatal("the instrument tab did not load the catalog")
	}
	setBankField(h, "Search instruments", &port.catalog.search.Field, "qa-bond")
	settle()
	assertPageLabel(t, &h.router, instrument.DisplayLabel())
	click("Details")
	if port.id != instrument.ID || port.current.instrument == nil || len(port.current.positions) != 0 {
		t.Fatal("unheld instrument did not open restored instrument details")
	}
	click("Back")
	if !port.catalog.active || port.catalog.search.Field.Text() != "qa-bond" {
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
