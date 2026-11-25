package desktop

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
)

type wealthHistoryTestReader struct {
	origin *domain.HistoryOrigin
	load   func(context.Context, wealthDateRange) (application.HistoricalOverviewSeriesResult, error)
}

func (r wealthHistoryTestReader) HistoryOrigin(context.Context) (*domain.HistoryOrigin, error) {
	return r.origin, nil
}

func (r wealthHistoryTestReader) HistoricalOverviewSeries(ctx context.Context, from, to string) (application.HistoricalOverviewSeriesResult, error) {
	return r.load(ctx, wealthDateRange{from, to})
}

func (r wealthHistoryTestReader) HistoricalOverview(context.Context, string, string) (application.HistoricalOverviewResult, error) {
	return application.HistoricalOverviewResult{}, nil
}

func openWealthHistoryTest(t *testing.T, reader wealthHistoryReader, clock func() time.Time) (*previewHarness, *wealthHistoryPage) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	page := newWealthHistoryPage(ctx, reader, clock, func() {})
	t.Cleanup(func() { cancel(); page.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{wealthHistory: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Wealth history", false)
	for until := time.Now().Add(3 * time.Second); page.originRead != nil; {
		if time.Now().After(until) {
			t.Fatal("Wealth History did not finish loading the origin")
		}
		time.Sleep(time.Millisecond)
		h.frame()
	}
	return h, page
}

func TestWealthHistoryRangeUsesClosedCivilDates(t *testing.T) {
	location, err := time.LoadLocation("Pacific/Apia")
	if err != nil {
		t.Fatal(err)
	}
	origin := wealthHistoryOrigin{first: "2011-12-01", location: location}
	now := time.Date(2011, 12, 31, 11, 0, 0, 0, time.UTC) // January 1 in Apia.
	for _, tc := range []struct {
		from, to string
		valid    bool
	}{
		{"2011-12-01", "2011-12-31", true},
		{"2011-12-31", "2011-12-31", true},
		{"2011-12-29", "2011-12-31", true},
		{"2011-12-30", "2011-12-31", false},
		{"2011-12-01", "2012-01-01", false},
		{"2011-12-31", "2011-12-29", false},
		{"2011-11-30", "2011-12-01", false},
		{"2011-12-1", "2011-12-01", false},
		{"", "2011-12-01", false},
	} {
		if valid := origin.permits(wealthDateRange{tc.from, tc.to}, now); valid != tc.valid {
			t.Errorf("%s to %s accepted = %v, want %v", tc.from, tc.to, valid, tc.valid)
		}
	}
	now = now.AddDate(0, 0, 1)
	if origin.permits(wealthDateRange{"2011-12-01", "2012-01-01"}, now) {
		t.Fatal("32 closed date labels were accepted")
	}
}

func TestWealthHistoryWithoutAClosedDay(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, text string
		origin     *domain.HistoryOrigin
	}{
		{"before History", "Start History to view daily wealth values.", nil},
		{"starting day", "No closed days are available since History started.", &domain.HistoryOrigin{StartedAt: now, Timezone: "UTC"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := openWealthHistoryTest(t, wealthHistoryTestReader{origin: tc.origin}, func() time.Time { return now })
			assertPageLabel(t, &h.router, tc.text)
		})
	}
}

func TestWealthHistoryRangeInteractionRetryAndKeyboardTable(t *testing.T) {
	origin := &domain.HistoryOrigin{Timezone: "UTC", StartedAt: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)}
	now := time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC)
	type call struct {
		ctx context.Context
		wealthDateRange
		reply chan historyResult[application.HistoricalOverviewSeriesResult]
	}
	calls := make(chan call, 4)
	h, page := openWealthHistoryTest(t, wealthHistoryTestReader{origin: origin, load: func(ctx context.Context, r wealthDateRange) (application.HistoricalOverviewSeriesResult, error) {
		reply := make(chan historyResult[application.HistoricalOverviewSeriesResult], 1)
		calls <- call{ctx, r, reply}
		select {
		case response := <-reply:
			return response.value, response.err
		case <-ctx.Done():
			return application.HistoricalOverviewSeriesResult{}, ctx.Err()
		}
	}}, func() time.Time { return now })
	h.size = image.Pt(900, 480)
	h.frame()
	assertPageLabel(t, &h.router, "Loading daily wealth values…")
	var active call
	expectCall := func(from, to string) context.Context {
		t.Helper()
		select {
		case request := <-calls:
			if request.wealthDateRange != (wealthDateRange{from, to}) {
				t.Fatalf("read range %+v, want %s to %s", request.wealthDateRange, from, to)
			}
			active = request
			return request.ctx
		case <-time.After(3 * time.Second):
			t.Fatal("no Wealth History read")
			return nil
		}
	}
	finish := func(series application.HistoricalOverviewSeriesResult, err error) {
		active.reply <- historyResult[application.HistoricalOverviewSeriesResult]{value: series, err: err}
		page.workers.Wait()
		h.frame()
	}
	expectCall("2026-11-30", "2026-11-30")
	finish(application.HistoricalOverviewSeriesResult{}, errors.New("first read failed"))
	assertPageLabel(t, &h.router, "Could not load daily wealth values.")
	h.click("Retry", false)
	previous := expectCall("2026-11-30", "2026-11-30")

	// Edit the From picker with the keyboard while the original request is in flight.
	h.click("From date", true)
	for i := 0; i < 3; i++ {
		h.press(key.NameUpArrow)
	}
	h.press(key.NameReturn)
	if page.draft.from != "2026-11-09" {
		t.Fatalf("calendar selection = %+v", page.draft)
	}
	h.focused(&page.from.Trigger)
	h.router.MoveFocus(key.FocusForward)
	h.frame()
	h.focused(&page.to.Trigger)
	h.router.MoveFocus(key.FocusForward)
	h.frame()
	h.focused(&page.load)
	h.press(key.NameReturn)
	expectCall("2026-11-09", "2026-11-30")
	if !errors.Is(previous.Err(), context.Canceled) {
		t.Fatal("loading a new range did not cancel the previous request")
	}
	series := application.HistoricalOverviewSeriesResult{From: "2026-11-09", To: "2026-11-30",
		Overview: application.HistoricalOverviewResult{Timezone: "UTC", Left: application.HistoricalOverviewState{Currency: "JPY"}}}
	for day := 9; day <= 30; day++ {
		point := application.HistoricalSeriesPoint{Date: time.Date(2026, 11, day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly), Available: true}
		if day == 29 {
			point.Available, point.GapReason = false, "nonexistent_civil_date"
		}
		series.Points = append(series.Points, point)
	}
	finish(series, nil)
	assertPageLabel(t, &h.router, "2026-11-09 to 2026-11-30 · Base currency JPY · UTC")
	assertPageLabel(t, &h.router, "Net worth · Unavailable")
	h.router.MoveFocus(key.FocusForward)
	h.frame()
	h.focused(&page.accountsTab)
	h.router.MoveFocus(key.FocusForward)
	h.frame()
	h.focused(&page.trendTab)
	h.router.MoveFocus(key.FocusForward)
	h.frame()
	h.focused(&page.list)
	h.press(key.NameEnd)
	assertPageLabel(t, &h.router, "2026-11-29 · This date did not occur in the History timezone.")
	h.press(key.NameHome)
	assertPageLabel(t, &h.router, "2026-11-09")
	h.click("Reload", false)
	expectCall("2026-11-09", "2026-11-30")
	finish(application.HistoricalOverviewSeriesResult{}, &domain.Error{Code: domain.ErrHistoricalSeriesBudget})
	assertPageLabel(t, &h.router, "This range exceeded the local replay budget. Try a shorter range.")
	assertPageLabel(t, &h.router, "Showing the previous capture.")
	h.click("Retry", false)
	pending := expectCall("2026-11-09", "2026-11-30")
	h.click("Back", false)
	if h.shell.activeRoute() != overviewRoute || !errors.Is(pending.Err(), context.Canceled) {
		t.Fatal("leaving Wealth History did not cancel its read")
	}
	page.workers.Wait()
	h.click("Forward", false)
	assertPageLabel(t, &h.router, "2026-11-09 to 2026-11-30 · Base currency JPY · UTC")
}

func TestWealthHistoryMatchesWailsOnReadOnlySyntheticCopies(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "seed.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seed := application.NewService(sqlite.NewRepository(db))
	seed.SetClock(clock)
	if err := seed.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic Wealth History", BaseCurrency: "AUD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []application.AccountInput{
		{Name: "Foreign cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100.05"},
		{Name: "Local cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "AUD", InitialAmount: "50"},
		{Name: "Local debt", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance", DefaultCurrency: "AUD", InitialAmount: "25.10"},
	} {
		input.IncludeInNetWorth, input.OwnerIDs = true, []domain.MemberID{bootstrap.Members[0].ID}
		if _, err := seed.CreateAccount(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := seed.SetFXPreference(ctx, "USD", "AUD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	if _, err := seed.AppendManualFXQuote(ctx, "USD", "AUD", "1.2345", now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Hour) // August 3 in Singapore, still August 2 in UTC.
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
		copyDB, err := sqlite.OpenReadOnlyForVerifyContext(ctx, copyPath)
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
	reference, err := analytics.NewService(wails).HistoricalOverviewSeries(ctx, "2026-08-01", "2026-08-02")
	if err != nil {
		t.Fatal(err)
	}
	gio, gioProvider := openCopy()
	h, page := openWealthHistoryTest(t, gio, clock)
	page.workers.Wait()
	h.frame()
	if page.err != nil || page.current == nil || page.current.series.To != "2026-08-02" {
		t.Fatalf("first capture did not use Singapore's latest closed day: %v", page.err)
	}
	h.click("From date", true)
	h.press(key.NameLeftArrow)
	h.press(key.NameReturn)
	h.click("Load range", false)
	page.workers.Wait()
	h.frame()
	if page.err != nil || !reflect.DeepEqual(reference, page.current.series) {
		t.Fatalf("Gio and Wails series disagree: %v", page.err)
	}
	first, second := reference.Points[0], reference.Points[1]
	if first.NetWorth != nil || len(first.Missing) != 1 || second.NetWorth == nil || *second.NetWorth != "148.411725" ||
		gioProvider.requests != 0 || wailsProvider.requests != 0 {
		t.Fatalf("unexpected exact result or provider request: first %+v, second %+v, Gio %d, Wails %d",
			first, second, gioProvider.requests, wailsProvider.requests)
	}
	for _, label := range []string{
		"2026-08-01 to 2026-08-02 · Base currency AUD · Asia/Singapore",
		"Net worth · Known net contribution · A$ 24.90",
		"2026-08-01 · Foreign cash · USD · Missing exchange rate",
		"Net worth · A$ 148.41",
		"Liabilities · -A$ 25.10",
	} {
		assertPageLabel(t, &h.router, label)
	}

	// Compare endpoint captures on the same independent read-only copies used
	// for the series. The first capture has no usable USD/AUD rate.
	wailsAnalytics := analytics.NewService(wails)
	compare, err := wailsAnalytics.HistoricalOverview(ctx, "2026-08-01", "2026-08-02")
	if err != nil {
		t.Fatal(err)
	}
	h.click("Accounts in Wealth history", false)
	page.workers.Wait()
	h.frame()
	h.click("Compare dates", false)
	h.click("Date A", true)
	h.press(key.NameLeftArrow)
	h.press(key.NameReturn)
	h.click("Capture accounts", false)
	page.workers.Wait()
	h.frame()
	if page.accounts.current == nil || !reflect.DeepEqual(compare, *page.accounts.current) {
		t.Fatalf("Gio and Wails endpoint captures disagree: %v", page.accounts.err)
	}
	assertPageLabel(t, &h.router, "A · 2026-08-01 · AUD")
	assertPageLabel(t, &h.router, "B · 2026-08-02 · AUD")
	h.click("Changed accounts only", false)
	assertPageLabel(t, &h.router, "Foreign cash")
	h.click("Foreign cash", false)
	h.frame()
	assertPageLabel(t, &h.router, "Missing exchange rate · AUD / USD · Manual quote")
	assertPageLabel(t, &h.router, "1 USD = 1.2345 AUD · Manual quote · 2026-08-02 12:00")
	h.click("Swap dates", false)
	h.click("Capture accounts", false)
	page.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "A · 2026-08-02 · AUD")
	assertPageLabel(t, &h.router, "B · 2026-08-01 · AUD")
	h.click("Swap dates", false)
	h.click("Compare with current", false)
	h.click("Capture accounts", false)
	page.workers.Wait()
	h.frame()
	current, err := wailsAnalytics.HistoricalOverview(ctx, "2026-08-01", "current")
	if err != nil || page.accounts.current == nil || !reflect.DeepEqual(current, *page.accounts.current) {
		t.Fatalf("Gio and Wails current comparisons disagree: %v, %v", err, page.accounts.err)
	}
	assertPageLabel(t, &h.router, "B · Current · AUD")
	h.click("Trend", false)
	assertPageLabel(t, &h.router, "2026-08-01 to 2026-08-02 · Base currency AUD · Asia/Singapore")
	h.click("Accounts in Wealth history", false)
	assertPageLabel(t, &h.router, "B · Current · AUD")
	if !page.accounts.changesOnly || gioProvider.requests != 0 || wailsProvider.requests != 0 {
		t.Fatalf("comparison did not retain its filter or requested quotes: Gio %d, Wails %d", gioProvider.requests, wailsProvider.requests)
	}
}
