package desktop

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestWealthCaptureShortcutsMatchWailsAndKeepLedgerContext(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC)
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
		HouseholdName: "Synthetic Wealth Shortcuts", BaseCurrency: "USD", MemberNames: []string{"Sample owner"},
	}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cash, err := seed.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic checking", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "USD", InitialAmount: "100.0001", IncludeInNetWorth: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	now = time.Date(2026, 11, 5, 17, 30, 0, 0, time.UTC) // November 6 in Singapore.
	commands := historyapi.NewService(seed)
	for _, topUp := range []struct{ day, time, amount string }{
		{"2026-10-31", "21:30", "3.3333"},
		{"2026-11-05", "16:30", "5.125"},
		{"2026-11-06", "00:30", "1.25"},
	} {
		if _, err := commands.RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyAdded, AccountID: cash.Account.ID.String(),
			Amount: topUp.amount, Currency: "USD", Reason: "contribution",
			EffectiveLocalDate: topUp.day, EffectiveLocalTime: topUp.time,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := errors.Join(db.CheckpointWAL(ctx), db.Close()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	openCopy := func() *application.Service {
		t.Helper()
		path := filepath.Join(t.TempDir(), "pair.db")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		db, err := sqlite.OpenReadOnlyForVerifyContext(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		app := application.NewService(sqlite.NewRepository(db))
		app.SetClock(clock)
		return app
	}
	gio := openCopy()
	wails := analytics.NewService(openCopy())
	h, page := openWealthHistoryTest(t, gio, clock, gio)
	h.size = image.Pt(1100, 1020)
	ledger := h.shell.historyPage
	settle := func() {
		for range 4 {
			h.frame()
			page.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
			ledger.cashMovements.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	assertCapture := func(from, to string) {
		t.Helper()
		expected, err := wails.HistoricalOverview(ctx, from, to)
		if err != nil || page.accounts.current == nil || !reflect.DeepEqual(expected, *page.accounts.current) {
			t.Fatalf("Gio capture did not match Wails for %s, %s: Gio %v, Wails %v", from, to, page.accounts.err, err)
		}
	}
	settle()
	assertCapture("2026-10-31", "")
	date := assertPageLabel(t, &h.router, "Available dates: 2026-10-20 – 2026-11-05 · Asia/Singapore")
	total := assertPageLabel(t, &h.router, "US$103.33")
	coverage := assertPageLabel(t, &h.router, "2026-10-31 · Coverage verified")
	if date.Min.Y >= total.Min.Y || total.Min.Y >= coverage.Min.Y {
		t.Fatalf("expected capture dates, headline totals and coverage in that order: %v, %v, %v", date, total, coverage)
	}
	click("Previous day")
	assertCapture("2026-10-30", "")
	click("Next day")
	assertCapture("2026-10-31", "")
	click("Starting point")
	assertCapture("2026-10-20", "")
	click("Last closed day")
	assertCapture("2026-11-05", "")
	click("Last month end")
	assertCapture("2026-10-31", "")
	click("Compare dates")
	click("Last month end → last closed day")
	assertCapture("2026-10-31", "2026-11-05")
	click("Baseline → captured current")
	assertCapture("2026-10-31", "current")
	captured := page.accounts.current
	if captured.Right == nil || captured.Left.NetWorth == nil || captured.Right.NetWorth == nil ||
		*captured.Left.NetWorth != "103.3334" || *captured.Right.NetWorth != "109.7084" {
		t.Fatalf("fixture did not separate month-end and current exact totals: %+v / %+v", captured.Left, captured.Right)
	}
	assertPageLabel(t, &h.router, "B · Current")
	assertPageLabel(t, &h.router, "US$109.71")
	assertPageLabel(t, &h.router, "Change B − A · +US$6.38")
	assertPageLabel(t, &h.router, "A · 2026-10-31")
	assertPageLabel(t, &h.router, "US$103.33")

	click("Account details")
	click("Synthetic checking")
	click("Valuation details")
	click("View in Ledger activity")
	expectedScope := history.Filters{Account: cash.Account.ID, From: "2026-10-31", To: "2026-11-05"}
	if got := ledger.model.Filters(); !got.Equal(expectedScope) {
		t.Fatalf("captured account and dates did not scope Ledger: got %+v, want %+v", got, expectedScope)
	}
	if len(ledger.model.Rows()) != 2 {
		t.Fatalf("the closed-date range included today's records: %+v", ledger.model.Rows())
	}
	click("Record change")
	if ledger.cashMovements.draft == nil {
		t.Fatal("Ledger did not open its ordinary draft")
	}
	click("Cancel")
	if ledger.cashMovements.draft != nil || !ledger.model.Filters().Equal(expectedScope) {
		t.Fatal("cancel changed the captured Ledger scope")
	}
	assertRepeatActivityCount(t, gio, 3)
	click("Back")
	if h.shell.activeRoute() != wealthHistoryRoute || page.lens != wealthAccountsLens ||
		page.accounts.current != captured || page.detail == nil || page.detail.row.AccountID != cash.Account.ID.String() {
		t.Fatal("Back did not restore the exact read-only account capture")
	}
	h.focused(&page.detail.action)
	h.press(key.NameEscape)
	settle()
	h.focused(page.accounts.details[cash.Account.ID.String()])
}

func TestWealthCaptureShortcutsAtSingaporeMonthBoundary(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 12, 1, 16, 30, 0, 0, time.UTC) }
	for _, tc := range []struct {
		name, defaultDay string
		start            time.Time
	}{
		{"previous month in history", "2026-11-30", time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)},
		{"history starts this month", "2026-12-01", time.Date(2026, 12, 1, 4, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := wealthHistoryTestReader{origin: &domain.HistoryOrigin{StartedAt: tc.start, Timezone: "Asia/Singapore"}}
			h, page := openWealthHistoryTest(t, reader, clock)
			settle := func() {
				page.workers.Wait()
				h.frame()
			}
			assertCapture := func(day string) {
				t.Helper()
				if page.accounts.current == nil || page.accounts.captured.from != day {
					t.Fatalf("capture date = %q, want %q", page.accounts.captured.from, day)
				}
			}
			settle()
			assertCapture(tc.defaultDay)
			if tc.defaultDay == "2026-12-01" {
				if h.inPage("Last month end", false) {
					t.Fatal("month end before History Origin is available as a clickable shortcut")
				}
				return
			}
			for _, shortcut := range []struct{ label, day string }{
				{"Last closed day", "2026-12-01"},
				{"Starting point", "2026-11-20"},
				{"Last month end", "2026-11-30"},
			} {
				h.click(shortcut.label, false)
				settle()
				assertCapture(shortcut.day)
			}
		})
	}
}

type interruptedWealthCapture struct {
	wealthHistoryTestReader
	calls   atomic.Int32
	started chan struct{}
	stop    chan struct{}
	release chan struct{}
}

func (r *interruptedWealthCapture) HistoricalOverview(ctx context.Context, from, _ string) (application.HistoricalOverviewResult, error) {
	if r.calls.Add(1) == 2 {
		close(r.started)
		<-ctx.Done()
		close(r.stop)
		<-r.release
		// A late response must not replace the resumed capture.
		return application.HistoricalOverviewResult{}, errors.New("obsolete capture")
	}
	return application.HistoricalOverviewResult{Left: application.HistoricalOverviewState{Date: from, Currency: "USD"}}, nil
}

func TestWealthCaptureResumesShortcutAfterLeaving(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 11, 5, 17, 30, 0, 0, time.UTC) }
	reader := &interruptedWealthCapture{
		wealthHistoryTestReader: wealthHistoryTestReader{origin: &domain.HistoryOrigin{
			StartedAt: time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC),
			Timezone:  "Asia/Singapore",
		}},
		started: make(chan struct{}), stop: make(chan struct{}), release: make(chan struct{}),
	}
	h, p := openWealthHistoryTest(t, reader, clock)
	release := sync.OnceFunc(func() { close(reader.release) })
	t.Cleanup(release)
	p.workers.Wait()
	h.frame()
	initial := p.accounts.current
	if initial == nil || initial.Left.Date != "2026-10-31" {
		t.Fatal("expected retained month-end capture")
	}

	h.click("Last closed day", false)
	select {
	case <-reader.started:
	case <-time.After(3 * time.Second):
		t.Fatal("shortcut did not start a new read")
	}
	h.click("Overview", false)
	select {
	case <-reader.stop:
	case <-time.After(3 * time.Second):
		t.Fatal("leaving did not cancel the old read")
	}
	if p.accounts.current != initial {
		t.Fatal("leaving discarded the prior capture")
	}
	h.click("Back", false)
	h.frame()
	release()
	p.workers.Wait()
	h.frame()
	if p.accounts.current == nil || p.accounts.current.Left.Date != "2026-11-05" ||
		p.accounts.err != nil || reader.calls.Load() != 3 {
		t.Fatalf("Back did not resume the selected read: %+v, %v, calls %d", p.accounts.captured, p.accounts.err, reader.calls.Load())
	}
}
