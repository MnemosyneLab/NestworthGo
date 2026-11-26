package desktop

import (
	"fmt"
	"image"
	"reflect"
	"slices"
	"testing"

	"gioui.org/io/event"
	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestPortfolioCatalogSheetPreservesFilteredListAndWritesOnlyOnSave(t *testing.T) {
	ctx := t.Context()
	service, _, _, _ := openPortfolioFixture(t, "partial")
	var target domain.Instrument
	for index := range 12 {
		item, err := service.CreateInstrument(ctx, application.InstrumentInput{
			Name: fmt.Sprintf("Synthetic catalog %02d", index), Symbol: fmt.Sprintf("QA-SHEET-%02d", index),
			Type: "bond", QuoteCurrency: "EUR", QuoteSource: "manual",
		})
		if err != nil {
			t.Fatal(err)
		}
		if index == 5 {
			target = item
		}
	}
	list := func() []domain.Instrument {
		t.Helper()
		items, err := service.ListInstruments(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		return items
	}
	initial := list()
	page := newPortfolioPage(ctx, service, func() {})
	t.Cleanup(page.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: page})
	h.size = image.Pt(1280, 920)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 3 {
			h.frame()
			page.workers.Wait()
		}
		h.frame()
	}
	click := func(name string) {
		t.Helper()
		h.click(name, false)
		settle()
	}
	const query = "QA-SHEET"
	settle()
	click("Investment assets")
	click("All instruments")
	setBankField(h, "Search instruments", &page.catalog.search.Field, query)
	settle()
	page.list.ScrollBy(3.25)
	settle()
	before := page.list.Position
	if before.First == 0 || before.Offset == 0 {
		t.Fatalf("fixture did not scroll the filtered catalog: %+v", before)
	}
	edit := "Edit " + target.DisplayLabel()
	assertPageLabel(t, &h.router, edit)

	assertListState := func() {
		t.Helper()
		if page.tabs.active != portfolioInstruments || page.catalog.search.Field.Text() != query ||
			page.list.Position.First != before.First || page.list.Position.Offset != before.Offset {
			t.Fatalf("sheet changed catalog tab, filter or scroll: %q, %+v vs %+v",
				page.catalog.search.Field.Text(), page.list.Position, before)
		}
	}
	assertSheet := func(title string) *portfolioCatalogDraft {
		t.Helper()
		draft := page.catalog.draft
		if draft == nil || !draft.sheet.modal {
			t.Fatal("catalog action did not open a modal sheet")
		}
		assertPageLabel(t, &h.router, title)
		if findPageLabel(&h.router, target.DisplayLabel()) == nil {
			t.Fatal("catalog row was removed behind the sheet")
		}
		bounds := semanticBounds(t, &h.router, "Close", false)
		if bounds.Min.X < h.size.X-h.metric().Dp(draft.sheet.width) || bounds.Max.X > h.size.X {
			t.Fatalf("sheet close button escaped the right-hand panel: %v", bounds)
		}
		assertListState()
		return draft
	}
	assertReturned := func(trigger event.Tag) {
		t.Helper()
		if page.catalog.draft != nil {
			t.Fatal("catalog sheet remained open")
		}
		assertListState()
		h.focused(trigger)
	}
	assertUnchanged := func() {
		t.Helper()
		if !reflect.DeepEqual(list(), initial) {
			t.Fatal("invalid or dismissed catalog form changed persisted instruments")
		}
	}

	click("Add instrument")
	add := assertSheet("Add instrument")
	h.focused(&add.fields[portfolioInstrumentName].Editor)
	h.press(key.NameReturn)
	settle()
	if page.catalog.draft != add || add.issue != i18n.PortfolioCatalogNameRequired {
		t.Fatal("blank Add lost its draft or omitted name validation")
	}
	assertPageLabel(t, &h.router, "Must not be empty")
	h.focused(&add.fields[portfolioInstrumentName].Editor)
	assertUnchanged()
	// A rejected service call can invalidate preview tokens without persisting
	// anything. Closing the form also must avoid a new write attempt.
	version := service.MobileDataVersion()
	assertDismissed := func(trigger event.Tag) {
		t.Helper()
		assertReturned(trigger)
		assertUnchanged()
		if service.MobileDataVersion() != version {
			t.Fatal("dismissal attempted a ledger write")
		}
	}
	click("Close")
	assertDismissed(&page.catalog.add)

	// A press behind the panel dismisses the sheet without activating navigation.
	click("Add instrument")
	assertSheet("Add instrument")
	click("Ledger activity")
	assertDismissed(&page.catalog.add)

	button := &page.catalog.links[target.ID].manage
	click(edit)
	draft := assertSheet("Edit instrument")
	h.focused(&draft.archive)
	if draft.instrument.ID != target.ID || draft.fields[portfolioInstrumentName].Text() != target.Name {
		t.Fatal("Edit did not display the selected instrument")
	}
	setBankField(h, "Name", &draft.fields[portfolioInstrumentName], "Synthetic catalog 05 discarded")
	h.press(key.NameEscape)
	settle()
	assertDismissed(button)

	click(edit)
	draft = assertSheet("Edit instrument")
	if draft.fields[portfolioInstrumentName].Text() != target.Name {
		t.Fatal("a dismissed draft leaked into the next Edit")
	}
	const edited = "Synthetic catalog 05 revised"
	setBankField(h, "Name", &draft.fields[portfolioInstrumentName], edited)
	click("Save")
	assertReturned(button)
	assertPageLabel(t, &h.router, edited)
	items := list()
	index := slices.IndexFunc(items, func(item domain.Instrument) bool { return item.ID == target.ID })
	if index < 0 || len(items) != len(initial) || service.MobileDataVersion() == version {
		t.Fatal("Save failed to update the existing catalog entry")
	}
	if item := items[index]; item.Name != edited || item.DisplayLabel() != target.DisplayLabel() ||
		item.Type != target.Type || item.QuoteCurrency != target.QuoteCurrency || item.QuoteSource != target.QuoteSource {
		t.Fatalf("Edit failed to preserve the selected instrument's identity/configuration: %+v", item)
	}
}
