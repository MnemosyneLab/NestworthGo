package desktop

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
)

type retryReturnReads struct {
	*application.Service
	failCalendar, failDay bool
}

func (r *retryReturnReads) ReturnCalendar(ctx context.Context, query domain.AnalysisQuery, cursor, granularity string) (application.ReturnCalendarResult, error) {
	if r.failCalendar {
		r.failCalendar = false
		return application.ReturnCalendarResult{}, errors.New("synthetic calendar error")
	}
	return r.Service.ReturnCalendar(ctx, query, cursor, granularity)
}

func (r *retryReturnReads) ReturnDay(ctx context.Context, query domain.AnalysisQuery, date domain.LocalDate) (application.ReturnDayResult, error) {
	if r.failDay {
		r.failDay = false
		return application.ReturnDayResult{}, errors.New("synthetic day error")
	}
	return r.Service.ReturnDay(ctx, query, date)
}

func TestReturnAnalysisCalendarAndDayMatchWails(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "seed.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seed := application.NewService(sqlite.NewRepository(db))
	seed.SetClock(clock)
	if err := seed.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic Return Calendar", BaseCurrency: "USD", MemberNames: []string{"Sample owner"},
	}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	broker, err := seed.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic broker", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := seed.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic fund", Type: "stock", MarketCode: "US", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := seed.CreateHolding(ctx, application.HoldingInput{
		AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-02-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.StartHistoryWithCosts(ctx, "Asia/Singapore", map[domain.HoldingID]string{holding.ID: "100"}); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	if _, err := seed.AppendManualInstrumentQuote(ctx, instrument.ID, "12.5", "2026-02-02", false); err != nil {
		t.Fatal(err)
	}
	now = now.Add(32 * time.Hour) // February 4 in Singapore.
	if err := errors.Join(db.CheckpointWAL(ctx), db.Close()); err != nil {
		t.Fatal(err)
	}
	seedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	openCopy := func() (*application.Service, *unavailableOverviewProvider) {
		t.Helper()
		copyPath := filepath.Join(t.TempDir(), "copy.db")
		if err := os.WriteFile(copyPath, seedBytes, 0o600); err != nil {
			t.Fatal(err)
		}
		copyDB, err := sqlite.Open(copyPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = copyDB.Close() })
		provider := &unavailableOverviewProvider{}
		s := application.NewService(sqlite.NewRepository(copyDB), application.NewMarketDataRegistry(provider))
		s.SetClock(clock)
		return s, provider
	}
	wails, wailsProvider := openCopy()
	adapter := analysis.NewService(wails)
	query := analysis.AnalysisQueryRequest{ScopeKind: "household", From: "2026-02-01", To: "2026-02-03", Valuation: "base", Basis: "investment", IncludeCash: true}
	calendar, err := adapter.ReturnCalendar(ctx, query, "2026-02", "day")
	if err != nil {
		t.Fatal(err)
	}
	wailsDay, err := adapter.ReturnDay(ctx, query, "2026-02-02")
	if err != nil {
		t.Fatal(err)
	}
	if calendar.Summary.ReturnAmount == nil || calendar.Summary.ReturnAmount.Amount != "25" ||
		calendar.Summary.ReturnRate == nil || *calendar.Summary.ReturnRate != "0.25" {
		t.Fatalf("fixture did not produce a USD 25 return at 25 percent: %+v", calendar.Summary)
	}

	gio, gioProvider := openCopy()
	reader := &retryReturnReads{Service: gio, failCalendar: true, failDay: true}
	page := &returnAnalysisPage{ctx: ctx, reader: reader, clock: clock, invalidate: func() {}}
	t.Cleanup(page.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{returns: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Return analysis", false)
	for range 3 {
		page.workers.Wait()
		h.frame()
	}
	assertPageLabel(t, &h.router, "Could not load investment returns.")
	h.click("Retry", false)
	page.workers.Wait()
	h.frame()
	if page.current == nil || page.err != nil {
		t.Fatalf("calendar retry did not load: %v", page.err)
	}
	current := page.current
	got := current.summary.Summary
	if current.summaryFrom != query.From || current.summaryTo != query.To ||
		got.ReturnAmount == nil || got.ReturnAmount.CanonicalAmount() != calendar.Summary.ReturnAmount.Amount ||
		got.ReturnRate == nil || got.ReturnRate.String() != *calendar.Summary.ReturnRate ||
		got.Coverage.RatedDays != calendar.Summary.RatedDays || got.Coverage.TotalDays != calendar.Summary.TotalDays {
		t.Fatalf("Gio summary differs from Wails: %+v and %+v", got, calendar.Summary)
	}
	for _, label := range []string{"Summary · 2026-02-01 to 2026-02-03 · USD", "Return amount", "Return rate", "+25%", "$ 25.00", "Day still open"} {
		assertPageLabel(t, &h.router, label)
	}
	h.click("2026-02-02 · +25% · $ 25.00", true)
	page.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "Could not load investment returns.")
	h.focused(&page.day.close)
	h.click("Retry", false)
	page.workers.Wait()
	h.frame()
	day := page.day.value
	if day.ReturnAmount == nil || day.ReturnAmount.CanonicalAmount() != wailsDay.ReturnAmount.Amount ||
		day.ReturnRate == nil || day.ReturnRate.String() != *wailsDay.ReturnRate || len(day.Composition) != len(wailsDay.Composition) ||
		gioProvider.requests != 0 || wailsProvider.requests != 0 {
		t.Fatalf("Gio and Wails day or provider reads differ: %+v and %+v", day, wailsDay)
	}
	assertPageLabel(t, &h.router, "Return amount · $ 25.00")
	assertPageLabel(t, &h.router, "Price change · $ 25.00")
	assertPageLabel(t, &h.router, "Synthetic fund · $ 25.00")
	trigger := page.day.button
	h.press(key.NameEscape)
	h.frame()
	h.focused(trigger)
	h.press(key.NameReturn)
	if page.day == nil || page.day.date != "2026-02-02" {
		t.Fatal("keyboard focus did not reopen the same return day")
	}
	h.clickAt(image.Pt(340, 420))
	h.frame()
	if page.day != nil {
		t.Fatal("clicking outside the detail sheet did not dismiss it")
	}
	h.focused(trigger)
}

func TestReturnCalendarGridUsesWholeWeeks(t *testing.T) {
	month := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		sunday bool
		first  string
		count  int
	}{
		{true, "2026-02-01", 28},
		{false, "2026-01-26", 35},
	} {
		first, count := returnGrid(month, tc.sunday)
		if first.Format(time.DateOnly) != tc.first || count != tc.count {
			t.Errorf("grid with Sunday start %v: got %v with %d days", tc.sunday, first, count)
		}
	}
}
