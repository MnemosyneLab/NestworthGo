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

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	wailsanalysis "github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
)

type pausableStage6Calendar struct {
	*application.Service
	resume <-chan struct{}
}

func (r *pausableStage6Calendar) ReturnCalendar(ctx context.Context, q domain.AnalysisQuery, cursor, granularity string) (application.ReturnCalendarResult, error) {
	if r.resume != nil {
		select {
		case <-r.resume:
		case <-ctx.Done():
			return application.ReturnCalendarResult{}, ctx.Err()
		}
	}
	return r.Service.ReturnCalendar(ctx, q, cursor, granularity)
}

func seedStage6ReturnCalendar(t *testing.T, start time.Time) ([]byte, domain.AccountID, domain.InstrumentID) {
	t.Helper()
	ctx := t.Context()
	now := start
	db, _, svc := openStage6Database(t, filepath.Join(t.TempDir(), "source.db"), func() time.Time { return now })
	if err := svc.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic Stage 6 return calendar", BaseCurrency: "USD", MemberNames: []string{"Sample owner"},
	}); err != nil {
		t.Fatal(err)
	}
	bs, err := svc.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	createBroker := func(name string) domain.AccountID {
		t.Helper()
		record, err := svc.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
			DefaultCurrency: "EUR", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bs.Members[0].ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		return record.Account.ID
	}
	harbor, other := createBroker("Harbor Brokerage"), createBroker("Second Brokerage")
	fund, err := svc.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic euro fund", Symbol: "EURO", Type: "stock", MarketCode: "DE", QuoteCurrency: "EUR", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	costs := make(map[domain.HoldingID]string)
	for _, position := range []struct {
		account        domain.AccountID
		quantity, cost string
	}{{harbor, "10", "100"}, {other, "20", "200"}} {
		holding, err := svc.CreateHolding(ctx, application.HoldingInput{
			AccountID: position.account.String(), InstrumentID: fund.ID.String(), Quantity: position.quantity,
		})
		if err != nil {
			t.Fatal(err)
		}
		costs[holding.ID] = position.cost
	}
	if _, err := svc.AppendAccountCashValue(ctx, harbor, "40", "EUR", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetFXPreference(ctx, "EUR", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AppendManualFXQuote(ctx, "EUR", "USD", "1.25", start.Format(time.DateOnly)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AppendManualInstrumentQuote(ctx, fund.ID, "10", start.Format(time.DateOnly), false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CaptureSyntheticOriginWithCosts(ctx, "Asia/Singapore", costs); err != nil {
		t.Fatal(err)
	}
	now = start.AddDate(0, 0, 1)
	if _, err := svc.AppendManualInstrumentQuote(ctx, fund.ID, "12.5", now.Format(time.DateOnly), false); err != nil {
		t.Fatal(err)
	}
	return stage6SeedBytes(t, db), harbor, fund.ID
}

func TestStage6PairedReturnCalendarFilteredKeyboardHandoff(t *testing.T) {
	const from, to, selected = "2026-10-01", "2026-10-31", "2026-10-02"
	ctx := t.Context()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	seed, account, fund := seedStage6ReturnCalendar(t, time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC))
	gioPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "gio"), seed)
	wailsPath := writeStage6Fixture(t, filepath.Join(t.TempDir(), "wails"), seed)
	gioProvider, wailsProvider := &unavailableOverviewProvider{}, &unavailableOverviewProvider{}
	_, _, gio := openStage6Database(t, gioPath, clock, application.NewMarketDataRegistry(gioProvider))
	_, _, wails := openStage6Database(t, wailsPath, clock, application.NewMarketDataRegistry(wailsProvider))
	api := wailsanalysis.NewService(wails)
	accountID, currency := account.String(), "EUR"
	request := wailsanalysis.AnalysisQueryRequest{ScopeKind: "instrument", ScopeID: fund.String(),
		From: from, To: to, Valuation: "native", Basis: "investment", IncludeCash: false, AccountID: &accountID, Currency: &currency}
	wailsDay, err := api.ReturnDay(ctx, request, selected)
	if err != nil || wailsDay.ReturnAmount == nil || wailsDay.ReturnAmount.Amount != "25" || wailsDay.ReturnAmount.Currency != currency {
		t.Fatalf("Wails filtered return day = %+v (%v)", wailsDay, err)
	}
	assetsRequest := request
	assetsRequest.From, assetsRequest.To = selected, selected
	wailsAsset, err := api.AssetChange(ctx, assetsRequest)
	if err != nil || wailsAsset.Summary.Change == nil || wailsAsset.Summary.Change.Amount != "25" || wailsAsset.Summary.Change.Currency != currency {
		t.Fatalf("Wails filtered Asset Changes = %+v (%v)", wailsAsset, err)
	}
	wailsPrice, err := api.AssetDriverDetail(ctx, assetsRequest, string(domain.BucketPriceChange))
	if err != nil || len(wailsPrice.ByAccount) != 1 || wailsPrice.ByAccount[0].Key != accountID ||
		wailsPrice.ByAccount[0].Amount == nil || wailsPrice.ByAccount[0].Amount.Amount != "25" {
		t.Fatalf("Wails scoped price driver = %+v (%v)", wailsPrice, err)
	}

	reader := &pausableStage6Calendar{Service: gio}
	page := &returnAnalysisPage{ctx: ctx, reader: reader, clock: clock, invalidate: func() {}}
	assets := &assetChangesPage{ctx: ctx, reader: gio, clock: clock, invalidate: func() {}}
	t.Cleanup(page.workers.Wait)
	t.Cleanup(assets.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{returns: page, assets: assets})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1140, 860)
	h.frame()
	settle := func() {
		for range 3 {
			page.workers.Wait()
			assets.workers.Wait()
			h.frame()
		}
	}
	choose := func(field, name string) {
		t.Helper()
		h.click(field, true)
		h.click(name, false)
		settle()
	}
	h.click("Return analysis", false)
	settle()
	choose("Scope", "Instrument")
	choose("Instrument", domain.InstrumentSelectionLabel("Synthetic euro fund", "EURO", "", "DE", "EUR", "", "", "", ""))
	h.click("More filters", false)
	choose("Valuation", "Native currency")
	h.click("Include cash", false)
	settle()
	choose("Account filter", "Harbor Brokerage")
	h.click("Currency", true)
	h.router.Queue(key.EditEvent{Text: "EUR"})
	h.frame()
	h.click("EUR", false)
	settle()
	h.click("More filters", false)
	h.click("Previous month", false)
	settle()
	selectedQuery, err := request.ToDomain()
	if err != nil {
		t.Fatal(err)
	}
	if page.current == nil || !reflect.DeepEqual(page.current.query, selectedQuery) {
		t.Fatalf("Gio did not retain the Wails October request: %+v", page.current)
	}
	assertPageLabel(t, &h.router, "Summary valuation · Native (EUR)")
	assertPageLabel(t, &h.router, "Calendar valuation · Native (EUR)")
	const dateLabel = "2026-10-02 · +25% · +€25.00"
	h.click(dateLabel, true)
	settle()
	if page.day == nil || !page.day.hasValue || page.day.date != selected ||
		page.day.value.ReturnAmount == nil || page.day.value.ReturnAmount.CanonicalAmount() != wailsDay.ReturnAmount.Amount ||
		page.day.value.Currency.String() != currency {
		t.Fatalf("Gio filtered return day differs: %+v", page.day)
	}
	trigger := page.day.button
	h.size = image.Pt(665, 480)
	settle()
	h.press(key.NameEscape)
	settle()
	h.focused(trigger)
	h.shell.stage.Position.Offset = 410
	h.frame()
	originalOffset := h.shell.stage.Position.Offset
	if originalOffset <= 0 {
		t.Fatal("the compact calendar fixture did not scroll")
	}
	h.press(key.NameReturn)
	settle()
	h.router.Source().Execute(key.FocusCmd{Tag: &page.day.action})
	h.frame()
	h.press(key.NameReturn)
	settle()
	wantAssets, err := assetsRequest.ToDomain()
	if err != nil {
		t.Fatal(err)
	}
	if h.shell.activeRoute() != assetChangesRoute || assets.state.tab != assetDriversTab || assets.current == nil ||
		!reflect.DeepEqual(assets.current.query, wantAssets) || assets.current.drivers == nil ||
		assets.current.drivers.Summary.Change == nil || assets.current.drivers.Summary.Change.CanonicalAmount() != wailsAsset.Summary.Change.Amount {
		t.Fatalf("Gio carried the wrong filters or result to Asset Changes: %+v", assets.current)
	}
	// Let the returning Calendar lay out at least one loading frame before
	// its grid returns. That shorter frame must not consume the saved scroll.
	resume := make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	reader.resume = resume
	h.click("Back", false)
	h.frame()
	if page.current != nil || page.request == nil {
		t.Fatal("the returning calendar did not enter the loading state")
	}
	release()
	settle()
	if h.shell.stage.Position.Offset != originalOffset {
		t.Fatalf("compact Back restored scroll %d, want %d", h.shell.stage.Position.Offset, originalOffset)
	}
	h.focused(trigger)
	if page.current == nil || !reflect.DeepEqual(page.current.query, selectedQuery) || page.month.Format("2006-01") != "2026-10" {
		t.Fatal("Back did not restore the October selection")
	}
	h.press(key.NameReturn)
	settle()
	if page.day == nil || page.day.date != selected || gioProvider.requests != 0 || wailsProvider.requests != 0 {
		t.Fatal("Back focus reopened the wrong date or a provider was called")
	}
}
