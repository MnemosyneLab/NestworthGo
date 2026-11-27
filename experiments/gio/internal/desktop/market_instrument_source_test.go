package desktop

import (
	"context"
	"errors"
	"image"
	"path/filepath"
	"sync/atomic"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	instrumentapi "github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	marketapi "github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
)

type instrumentBindingTestProvider struct {
	unavailableOverviewProvider
	key       string
	hits      []application.InstrumentSearchHit
	failFirst bool
	calls     atomic.Int32
}

func (p *instrumentBindingTestProvider) Key() string { return p.key }
func (*instrumentBindingTestProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{InstrumentSearch: true}
}
func (p *instrumentBindingTestProvider) SearchInstruments(_ context.Context, _, _ string, _ int) ([]application.InstrumentSearchHit, error) {
	if p.calls.Add(1) == 1 && p.failFirst {
		return nil, &domain.Error{Code: domain.ErrProviderUnavailable}
	}
	return p.hits, nil
}

type instrumentBindingTestEditor struct {
	marketInstrumentSourceService
	calls atomic.Int32
}

func (w *instrumentBindingTestEditor) PatchInstrument(ctx context.Context, id domain.InstrumentID, patch application.InstrumentPatch) (domain.Instrument, error) {
	if w.calls.Add(1) == 1 {
		return domain.Instrument{}, errors.New("synthetic write failure")
	}
	return w.marketInstrumentSourceService.PatchInstrument(ctx, id, patch)
}

func TestMarketInstrumentSourceLookupSaveRetryAndFindingReturn(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "binding.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	native := application.InstrumentSearchHit{
		ProviderKey: application.YahooFinanceProviderKey, ProviderSymbol: "DEMO", Symbol: "DEMO", Name: "Synthetic lookup result",
		Type: "etf", QuoteCurrency: "USD", MarketCode: "NASDAQ", CountryCode: "US",
	}
	mismatch := native
	mismatch.ProviderSymbol, mismatch.QuoteCurrency = "DEMO-EUR", "EUR"
	provider := &instrumentBindingTestProvider{key: application.YahooFinanceProviderKey, hits: []application.InstrumentSearchHit{mismatch, native}, failFirst: true}
	app := application.NewService(&testfixture.BaselineRepository{Repository: sqlite.NewRepository(db)}, application.NewMarketDataRegistryWithDefault(provider.Key(), provider))
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic binding", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	note := "Unrelated catalog note"
	instrument, err := app.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic binding fund", ShortName: new("Display alias"), Type: "etf", QuoteCurrency: "USD",
		QuoteSource: "manual", Symbol: "LOCAL", Note: &note, SortOrder: 9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AppendManualInstrumentQuote(ctx, instrument.ID, "12.34567891", "2026-01-01T12:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	writer := &instrumentBindingTestEditor{marketInstrumentSourceService: app}
	market := &marketDataPage{ctx: ctx, reader: app, invalidate: func() {}, instrumentSources: writer}
	finding := application.HealthIssue{ID: "instrument-binding", Kind: application.HealthKindMissingBinding, Action: application.HealthActionInstrumentEditor,
		InstrumentID: instrument.ID.String(), Label: instrument.DisplayLabel()}
	healthReader := &countedHealthReport{data: application.MarketDataHealthReport{Issues: []application.HealthIssue{finding}}}
	health := &dataHealthPage{ctx: ctx, reader: healthReader, invalidate: func() {}}
	t.Cleanup(market.workers.Wait)
	t.Cleanup(health.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{marketData: market, dataHealth: health})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.trail = newNavigationTrail(marketDataRoute)
	h.size = image.Pt(700, 500)
	market.enter(marketDataVisit{search: "binding fund"})
	wait := func() {
		t.Helper()
		market.workers.Wait()
		h.frame()
	}
	wait()
	h.click("Edit quote source", false)
	wait()
	first := market.instrumentSource
	h.click("Provider quote", false)
	h.frame()
	h.click("Save quote source", false)
	if first.saveProblem == 0 || writer.calls.Load() != 0 {
		t.Fatal("blank binding entered the writer")
	}
	h.press(key.NameEscape)
	h.frame()
	h.focused(first.from)
	h.press(key.NameReturn)
	wait()
	h.click("Provider quote", false)
	h.frame()
	draft := market.instrumentSource
	h.click("Provider lookup query", true)
	h.router.Queue(key.EditEvent{Text: "Synthetic"})
	h.frame()
	if draft.query.Text() != "Synthetic" {
		t.Fatal("the sheet consumed the lookup editor's text input")
	}
	h.frame()
	if provider.calls.Load() != 0 {
		t.Fatal("typing a lookup query contacted a provider")
	}
	h.click("Look up instrument", false)
	wait()
	assertPageLabel(t, &h.router, "Could not look up instruments. Try again.")
	h.click("Look up instrument", false)
	wait()
	if len(draft.hits) != 2 || draft.canChoose(draft.hits[0]) || !draft.canChoose(draft.hits[1]) {
		t.Fatalf("wrong compatibility or results: %+v", draft.hits)
	}
	h.click(instrumentBindingHitLabel(native), false)
	h.frame()
	h.focused(&draft.symbol.Editor)
	if draft.symbol.Text() != "DEMO" || draft.market.Text() != "NASDAQ" || draft.country.Text() != "US" {
		t.Fatal("lookup did not fill the selected binding")
	}
	h.click("Save quote source", false)
	wait()
	if market.instrumentSource != draft || draft.symbol.Text() != "DEMO" || writer.calls.Load() != 1 {
		t.Fatal("save failure did not retain the draft")
	}
	h.click("Save quote source", false)
	wait()
	wait()
	h.focused(draft.from)
	if market.instrumentSource != nil || market.search.Field.Text() != "binding fund" {
		t.Fatal("save lost the market selection")
	}
	api := instrumentapi.NewService(app)
	rows, err := api.ListInstruments(ctx, true)
	if err != nil || len(rows) != 1 {
		t.Fatalf("Wails readback = %+v, %v", rows, err)
	}
	row := rows[0]
	if row.ID != instrument.ID.String() || row.Name != instrument.Name || row.ShortName != "Display alias" || marketBindingText(row.Symbol) != "LOCAL" ||
		row.QuoteSource != "provider" || marketBindingText(row.ProviderKey) != "yahoo_finance" || marketBindingText(row.ProviderSymbol) != "DEMO" ||
		row.QuoteCurrency != "USD" || marketBindingText(row.MarketCode) != "NASDAQ" || marketBindingText(row.CountryCode) != "US" || row.SortOrder != 9 || marketBindingText(row.Note) != note {
		t.Fatalf("wrong binding or unrelated changes in Wails readback: %+v", row)
	}
	result, err := marketapi.SearchInstruments(ctx, app, "Synthetic", "etf")
	if err != nil || len(result) != 2 || result[1].ProviderSymbol != native.ProviderSymbol {
		t.Fatalf("Wails lookup = %+v, %v", result, err)
	}
	quotes, err := app.InstrumentQuoteHistory(ctx, instrument.ID)
	if err != nil || len(quotes) != 1 || quotes[0].UnitPrice.Canonical() != "12.34567891" ||
		provider.unavailableOverviewProvider.requests != 0 {
		t.Fatalf("binding edit changed quote history or ran a refresh: %+v, %v", quotes, err)
	}

	focus := healthFindingFocus{id: finding.ID, instrument: finding.InstrumentID, label: finding.Label}
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: dataHealthRoute, health: focus}}
	h.frame()
	health.workers.Wait()
	h.frame()
	h.click("Edit quote source", false)
	wait()
	draft = market.instrumentSource
	h.press(key.NameEscape)
	h.frame()
	h.focused(draft.from)
	if health.focus != focus || healthReader.scans.Load() != 1 || writer.calls.Load() != 2 {
		t.Fatal("dismissal changed, rechecked or wrote the finding")
	}
	h.press(key.NameReturn)
	wait()
	h.click("Agent quote", false)
	h.frame()
	h.click("Save quote source", false)
	wait()
	health.workers.Wait()
	h.frame()
	rows, err = api.ListInstruments(ctx, true)
	if err != nil || rows[0].QuoteSource != "agent" || marketBindingText(rows[0].ProviderSymbol) != "DEMO" ||
		healthReader.scans.Load() != 2 || health.focus != focus {
		t.Fatalf("finding save did not retain scope or the existing binding: %+v, %v", rows, err)
	}
}
