package desktop

import (
	"context"
	"errors"
	"image"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	quoteapi "github.com/waltwang/nestworth-go/internal/wailsapi/quote"
)

func TestMarketDataManualQuotesRefreshValuationAndPreserveUnrelatedGaps(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "manual.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewRepository(db)
	provider := &unavailableOverviewProvider{}
	svc := application.NewService(repo, application.NewMarketDataRegistry(provider))
	now := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	svc.SetClock(clock)
	if err := svc.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic manual quotes", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	bs, err := svc.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := []domain.MemberID{bs.Members[0].ID}
	broker, err := svc.CreateAccount(ctx, application.AccountInput{Name: "Synthetic broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: owner})
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"JPY", "SGD"} {
		if _, err := svc.CreateAccount(ctx, application.AccountInput{Name: "Synthetic " + code, AccountType: "cash_on_hand", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: code, InitialAmount: "1000", IncludeInNetWorth: true, OwnerIDs: owner}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SetFXPreference(ctx, code, "USD", "manual"); err != nil {
			t.Fatal(err)
		}
	}
	instrument, err := svc.CreateInstrument(ctx, application.InstrumentInput{Name: "Synthetic manual fund", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := svc.CreateHolding(ctx, application.HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "2", UnitCost: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartHistoryWithCosts(ctx, "Asia/Singapore", map[domain.HoldingID]string{holding.ID: "1"}); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 4)
	if _, err := svc.RebuildHistoricalSnapshots(ctx, "2026-11-21", "2026-11-24"); err != nil {
		t.Fatal(err)
	}
	snapshots := func() []domain.DailyValuationSnapshot {
		t.Helper()
		rows, err := repo.ListDailyValuationSnapshots(ctx, bs.Household.ID, time.Time{}, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	baseline := snapshots()
	counts := func(prices, fx int) {
		t.Helper()
		var a, b int
		if err := db.SQL.QueryRow(`SELECT (SELECT count(*) FROM instrument_quotes), (SELECT count(*) FROM fx_quotes)`).Scan(&a, &b); err != nil || a != prices || b != fx {
			t.Fatalf("quote counts %d/%d, want %d/%d: %v", a, b, prices, fx, err)
		}
	}
	p := &marketDataPage{ctx: ctx, reader: svc, invalidate: func() {}, manual: marketManualQuotes{service: svc, clock: clock}}
	health := &dataHealthPage{ctx: ctx, reader: svc, invalidate: func() {}}
	t.Cleanup(health.workers.Wait)
	t.Cleanup(p.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{marketData: p, dataHealth: health})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1160, 950)
	marketFrame := func() {
		t.Helper()
		p.workers.Wait()
		h.frame()
	}
	h.frame()
	h.click("Data maintenance", false)
	marketFrame()
	if current := p.current[marketInstruments]; current == nil || len(current.instruments) != 1 || current.instruments[0].quote != nil {
		t.Fatalf("synthetic empty quote route=%v loaded=%+v error=%v", h.shell.activeRoute(), current, p.err)
	}
	assertPageLabel(t, &h.router, "No current price")
	h.click("Set price", false)
	marketFrame()
	h.size = image.Pt(700, 480)
	h.frame()
	d := p.manual.draft
	if d == nil || !d.available() || d.date != "" || d.refs.instrument.ID != instrument.ID || d.refs.location.String() != "Asia/Singapore" {
		t.Fatalf("wrong price target or date default: %+v", d)
	}
	d.amount.Editor.SetText("9.99")
	h.press(key.NameEscape)
	h.frame()
	h.focused(p.manual.prices[instrument.ID.String()])
	counts(0, 0)
	h.press(key.NameReturn)
	marketFrame()
	d = p.manual.draft
	h.click("Unit price · USD", true)
	h.focused(&d.amount.Editor)
	h.router.Queue(key.EditEvent{Text: "12.345678901"})
	h.frame()
	h.frame()
	h.click("Save price", false)
	h.frame()
	assertPageLabel(t, &h.router, "Enter a non-negative decimal price with up to eight fractional digits.")
	if d.amount.Text() != "12.345678901" || d.saving != nil {
		t.Fatal("validation discarded the entered price or started a write")
	}
	counts(0, 0)
	d.amount.Editor.SetText("12.34567891")
	d.date = "2026-11-24"
	h.frame()
	h.click("Save price", false)
	marketFrame()
	marketFrame()
	h.focused(p.manual.prices[instrument.ID.String()])
	assertPageLabel(t, &h.router, "Manual quote saved")
	counts(1, 0)
	h.click("FX rates", false)
	marketFrame()
	h.click("Add manual FX quote", false)
	marketFrame()
	d = p.manual.draft
	if d.base != "AUD" || d.quote != "CNY" || d.date != "" {
		t.Fatalf("default FX form used %s/%s on %q", d.base, d.quote, d.date)
	}
	h.press(key.NameEscape)
	h.frame()
	h.focused(&p.manual.addFX)
	counts(1, 0)
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: dataHealthRoute,
		health: healthFindingFocus{currencyA: "JPY", currencyB: "USD", from: "2026-11-22", to: "2026-11-24"}}}
	h.frame()
	health.workers.Wait()
	h.frame()
	h.click("Add manual quote", false)
	marketFrame()
	d = p.manual.draft
	if d == nil || d.base != "JPY" || d.quote != "USD" || d.date != "2026-11-22" {
		t.Fatalf("focused rate source = %+v", d)
	}
	d.amount.Editor.SetText("0")
	h.frame()
	h.click("Save manual FX quote", false)
	h.frame()
	assertPageLabel(t, &h.router, "Enter a positive decimal rate with up to twelve fractional digits.")
	if d.amount.Text() != "0" || d.date != "2026-11-22" {
		t.Fatal("FX validation lost the focused draft")
	}
	counts(1, 0)
	d.amount.Editor.SetText("0.006789012345")
	h.frame()
	h.click("Save manual FX quote", false)
	marketFrame()
	health.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "Manual quote saved")
	counts(1, 1)
	if !reflect.DeepEqual(baseline, snapshots()) {
		t.Fatal("a manual quote changed or rebuilt historical snapshot rows")
	}
	report, err := svc.ScanMarketDataHealth(ctx)
	if err != nil || report.Healthy || !slices.ContainsFunc(report.Issues, func(i application.HealthIssue) bool {
		return i.Kind == application.HealthKindMissingManualFX && (i.CurrencyA == "SGD" || i.CurrencyB == "SGD")
	}) {
		t.Fatalf("unrelated SGD prerequisite was cleared: %+v, %v", report, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restarted := application.NewService(sqlite.NewRepository(reopened), application.NewMarketDataRegistry(provider))
	restarted.SetClock(clock)
	wails := quoteapi.NewService(restarted)
	price, err := wails.CurrentInstrumentQuote(ctx, instrument.ID.String())
	if err != nil || price == nil || price.UnitPrice != "12.34567891" || price.SourceKind != "manual" || price.QuotedAt != "2026-11-23T16:00:00.000Z" {
		t.Fatalf("reopened price = %+v, %v", price, err)
	}
	fx, err := wails.CurrentFXQuote(ctx, "JPY", "USD")
	if err != nil || fx == nil || fx.Rate != "0.006789012345" || fx.BaseCurrency != "JPY" || fx.QuotedAt != "2026-11-21T16:00:00.000Z" {
		t.Fatalf("reopened rate = %+v, %v", fx, err)
	}
	value, err := restarted.Overview(ctx, domain.AccountFilter{})
	if err != nil || value.Assets.String() != "31.480370165" || value.Complete || provider.requests != 0 {
		t.Fatalf("reopened valuation did not use both quotes: %+v, %v, provider calls=%d", value, err, provider.requests)
	}
}

type scriptedManualFX struct {
	*application.Service
	calls chan [4]string
	reply chan historyResult[domain.FXQuote]
}

func (s scriptedManualFX) AppendManualFXQuoteWithReceipt(ctx context.Context, base, quote, rate, date string) (domain.FXQuote, error) {
	s.calls <- [4]string{base, quote, rate, date}
	select {
	case result := <-s.reply:
		return result.value, result.err
	case <-ctx.Done():
		return domain.FXQuote{}, ctx.Err()
	}
}

func TestMarketDataManualQuoteRetryBlocksOverlapAndHonorsCommittedReceipt(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "manual-retry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := application.NewService(sqlite.NewRepository(db))
	clock := func() time.Time { return time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC) }
	svc.SetClock(clock)
	if err := svc.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic manual retry", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	writer := scriptedManualFX{svc, make(chan [4]string, 4), make(chan historyResult[domain.FXQuote], 1)}
	p := &marketDataPage{ctx: ctx, reader: svc, invalidate: func() {}, manual: marketManualQuotes{service: writer, clock: clock}}
	t.Cleanup(func() { cancel(); p.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{marketData: p})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Data maintenance", false)
	p.workers.Wait()
	h.frame()
	// A focused link can outlive an instrument. It must never retarget another one.
	p.openManualQuote(marketManualTarget{instrumentID: domain.NewInstrumentID()}, &p.reload)
	p.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "This instrument is unavailable for manual entry.")
	h.press(key.NameEscape)
	h.frame()
	h.click("FX rates", false)
	p.workers.Wait()
	h.frame()
	h.click("Add manual FX quote", false)
	p.workers.Wait()
	h.frame()
	d := p.manual.draft
	d.amount.Editor.SetText("1.234567890123")
	h.frame()
	firstCall := func() {
		t.Helper()
		select {
		case got := <-writer.calls:
			if got != [4]string{"AUD", "CNY", "1.234567890123", ""} {
				t.Fatalf("write called with %v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("quote write did not start")
		}
	}
	h.click("Save manual FX quote", false)
	firstCall()
	p.saveManualQuote()
	h.press(key.NameEscape)
	if p.manual.draft != d || d.saving == nil || len(writer.calls) != 0 {
		t.Fatal("a second save or Escape interrupted the pending write")
	}
	writer.reply <- historyResult[domain.FXQuote]{err: errors.New("synthetic failed write before commit")}
	p.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "Could not save the manual quote.")
	if d.amount.Text() != "1.234567890123" || d.date != "" {
		t.Fatal("retry lost the original quote draft")
	}
	h.focused(&d.sheet.action)
	h.press(key.NameReturn)
	firstCall()
	accepted, err := svc.AppendManualFXQuoteWithReceipt(ctx, "AUD", "CNY", "1.234567890123", "")
	if err != nil || !accepted.QuotedAt.Equal(clock()) {
		t.Fatalf("blank-date quote did not use the current time: %+v, %v", accepted, err)
	}
	writer.reply <- historyResult[domain.FXQuote]{value: accepted, err: errors.New("synthetic follow-up refresh failure")}
	p.workers.Wait()
	h.frame()
	p.workers.Wait()
	h.frame()
	if p.manual.draft != nil || p.manual.latest.fx.ID != accepted.ID || !p.manual.latest.partial {
		t.Fatal("a committed FX observation was offered for another write")
	}
	assertPageLabel(t, &h.router, "The rate was saved. Related metal prices could not be refreshed.")
	var count int
	if err := db.SQL.QueryRow(`SELECT count(*) FROM fx_quotes`).Scan(&count); err != nil || count != 1 || len(writer.calls) != 0 {
		t.Fatalf("FX writes = %d, %v", count, err)
	}
	if preferences, err := svc.ListFXPreferences(ctx); err != nil || len(preferences) != 0 {
		t.Fatalf("saving an observation changed the configured source: %+v, %v", preferences, err)
	}
}
