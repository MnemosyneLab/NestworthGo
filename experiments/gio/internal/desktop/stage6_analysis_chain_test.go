package desktop

import (
	"context"
	"image"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	wailsanalysis "github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	wailshistory "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type pausableStage6AssetChanges struct {
	*application.Service
	resume <-chan struct{}
}

func (r *pausableStage6AssetChanges) AssetChange(ctx context.Context, query domain.AnalysisQuery) (application.AssetChangeResult, error) {
	if r.resume != nil {
		select {
		case <-r.resume:
		case <-ctx.Done():
			return application.AssetChangeResult{}, ctx.Err()
		}
	}
	return r.Service.AssetChange(ctx, query)
}

func TestStage6PairedAccountFilteredAnalysisRoundTrip(t *testing.T) {
	const day = "2026-10-02"
	ctx := t.Context()
	clock := func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }
	seed, account, fund, receipt := seedStage6Contribution(t, time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC))
	gioPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "gio"), seed)
	wailsPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "wails"), seed)
	gioProvider, wailsProvider := &unavailableOverviewProvider{}, &unavailableOverviewProvider{}
	_, _, gio := openStage6Database(t, gioPath, clock, application.NewMarketDataRegistry(gioProvider))
	_, _, wails := openStage6Database(t, wailsPath, clock, application.NewMarketDataRegistry(wailsProvider))
	adapter := wailsanalysis.NewService(wails)
	accountID, currency := account.String(), "EUR"
	request := wailsanalysis.AnalysisQueryRequest{ScopeKind: "instrument", ScopeID: fund.String(), From: day, To: day,
		Valuation: "native", Basis: "investment", IncludeCash: false, AccountID: &accountID, Currency: &currency}
	wantQuery, err := request.ToDomain()
	if err != nil {
		t.Fatal(err)
	}
	wantDriver, err := adapter.AssetDriverDetail(ctx, request, string(domain.BucketPriceChange))
	if err != nil || len(wantDriver.ByAccount) != 1 || wantDriver.ByAccount[0].Key != accountID ||
		wantDriver.ByAccount[0].Amount == nil || wantDriver.ByAccount[0].Amount.Amount != "25" {
		t.Fatalf("unexpected scoped Wails price breakdown: %+v (%v)", wantDriver, err)
	}
	wantItem, err := adapter.ContributionItem(ctx, request, string(application.ContributionTotalReturn),
		string(application.ContributionGroupInstrument), fund.String())
	if err != nil || wantItem.Amount == nil || wantItem.Amount.Amount != "37.3456" || wantItem.Amount.Currency != "EUR" {
		t.Fatalf("Wails total return should include both the price change and the dividend: %+v (%v)", wantItem, err)
	}
	hint := wantItem.HistoryHint
	if hint.From != day || hint.To != day || hint.AccountID != accountID || hint.InstrumentID != fund.String() || len(hint.Kinds) != 0 {
		t.Fatalf("the scoped Wails Ledger hint changed: %+v", hint)
	}
	wantLedger, err := wailshistory.NewService(wails).ListActivityPage(ctx, wailshistory.ActivityQueryRequest{
		FromLocalDate: hint.From, ToLocalDate: hint.To, AccountID: &hint.AccountID, InstrumentID: &hint.InstrumentID})
	if err != nil || len(wantLedger.Activities) != 1 || wantLedger.Activities[0].ID != receipt.ID {
		t.Fatalf("Wails Ledger did not isolate the same-day dividend: %+v (%v)", wantLedger, err)
	}

	assetReader := &pausableStage6AssetChanges{Service: gio}
	returns := &returnAnalysisPage{ctx: ctx, reader: gio, clock: clock, invalidate: func() {}}
	assets := &assetChangesPage{ctx: ctx, reader: assetReader, clock: clock, invalidate: func() {}}
	ledger := newLedgerHistory(ctx, gio, clock, func() {})
	t.Cleanup(returns.workers.Wait)
	t.Cleanup(assets.workers.Wait)
	t.Cleanup(ledger.model.Wait)
	t.Cleanup(ledger.reads.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{returns: returns, assets: assets, history: ledger})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1140, 960)
	settle := func() {
		for range 3 {
			h.frame()
			returns.workers.Wait()
			assets.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	choose := func(field, name string) { t.Helper(); h.click(field, true); click(name) }
	settle()
	click("Return analysis")
	choose("Scope", "Instrument")
	choose("Instrument", domain.InstrumentSelectionLabel("Synthetic euro fund", "EURO", "", "DE", "EUR", "", "", "", ""))
	click("More filters")
	choose("Valuation", "Native currency")
	click("Include cash")
	choose("Account filter", "Harbor Brokerage")
	h.click("Currency", true)
	h.router.Queue(key.EditEvent{Text: "EUR"})
	h.frame()
	click("EUR")
	click("More filters")
	click("Previous month")
	calendarQuery := returns.current.query
	h.size = image.Pt(665, 480)
	settle()
	const dayControl = day + " · +37.35% · €37.35"
	h.click(dayControl, true)
	settle()
	if returns.day == nil || !returns.day.hasValue || returns.day.value.ReturnAmount == nil || returns.day.value.ReturnAmount.CanonicalAmount() != "37.3456" {
		t.Fatalf("wrong Calendar day return: %+v", returns.day)
	}
	dayTrigger := returns.day.button
	click("View asset changes for this day")
	if h.shell.activeRoute() != assetChangesRoute || assets.state.tab != assetDriversTab || assets.current == nil ||
		!reflect.DeepEqual(assets.current.query, wantQuery) {
		t.Fatalf("Calendar did not carry exact analysis filters and dates: %+v", assets.current)
	}
	const priceRow = "Price Change · +€25.00"
	click(priceRow)
	if assets.driver == nil || assets.driver.value == nil || len(assets.driver.value.ByAccount) != 1 ||
		assets.driver.value.ByAccount[0].Amount == nil || assets.driver.value.ByAccount[0].Amount.CanonicalAmount() != wantDriver.ByAccount[0].Amount.Amount {
		t.Fatalf("the grouped driver breakdown differs from Wails: %+v", assets.driver)
	}
	priceTrigger := assets.driver.trigger
	savedAssetOffset := h.shell.stage.Position.Offset
	if savedAssetOffset <= 0 {
		t.Fatal("the fixture did not scroll to the compact grouped driver row")
	}
	click("Open return analysis")
	if h.shell.activeRoute() != returnAnalysisRoute || returns.tab != returnContributionTab || returns.current == nil ||
		!reflect.DeepEqual(returns.current.query, wantQuery) || returns.output.contributionMetric() != application.ContributionTotalReturn {
		t.Fatalf("the price driver did not carry its effective Contribution request: %+v", returns.current)
	}
	click("EURO · €37.35 · +37.35%")
	if returns.item == nil || returns.item.value == nil || returns.item.value.Amount == nil || returns.item.value.Amount.CanonicalAmount() != wantItem.Amount.Amount {
		t.Fatalf("the Contribution sheet differs from Wails: %+v", returns.item)
	}
	contributionTrigger := returns.item.trigger
	click("View in Ledger activity")
	rows := ledger.model.Rows()
	if h.shell.activeRoute() != historyRoute || !ledger.model.Filters().Equal(history.Filters{From: day, To: day, Account: account, Instrument: fund}) ||
		len(rows) != 1 || rows[0].ID.String() != receipt.ID {
		t.Fatalf("Gio Ledger missed the exact Wails row: %+v (%+v)", rows, ledger.model.Filters())
	}
	click("Back")
	h.focused(contributionTrigger)

	resume := make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	assetReader.resume = resume
	h.click("Back", false)
	h.frame()
	if h.shell.activeRoute() != assetChangesRoute || assets.request == nil || assets.current != nil {
		t.Fatal("the delayed Asset Changes read did not enter its shorter loading frame")
	}
	release()
	settle()
	if h.shell.stage.Position.Offset != savedAssetOffset {
		t.Fatalf("Back restored Asset Changes scroll %d, want %d", h.shell.stage.Position.Offset, savedAssetOffset)
	}
	h.focused(priceTrigger)
	bounds := assertPageLabel(t, &h.router, priceRow)
	if bounds.Min.Y < headerHeight || bounds.Max.Y > h.size.Y {
		t.Fatalf("Back focused the price driver outside the window: %v", bounds)
	}
	h.press(key.NameReturn)
	settle()
	if assets.driver == nil || assets.driver.row.Bucket != domain.BucketPriceChange {
		t.Fatal("Return after Back did not reopen the same grouped price driver")
	}
	h.press(key.NameEscape)
	settle()
	click("Back")
	h.focused(dayTrigger)
	if returns.tab != returnCalendarTab || returns.current == nil || !reflect.DeepEqual(returns.current.query, calendarQuery) {
		t.Fatal("the full chain discarded the Calendar month or filter state")
	}
	if gioProvider.requests != 0 || wailsProvider.requests != 0 {
		t.Fatal("scoped analysis requested live quotes")
	}
}
