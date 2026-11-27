package desktop

import (
	"context"
	"errors"
	"image"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type fundsFixture struct {
	bank, foreign, broker domain.AccountID
	instrument            domain.InstrumentID
}

func seedAvailableFunds(ctx context.Context, app *application.Service, at time.Time, populated bool) (fundsFixture, error) {
	var ids fundsFixture
	app.SetClock(func() time.Time { return at })
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic funds", BaseCurrency: "USD", MemberNames: []string{"Ana Example"},
	}); err != nil || !populated {
		return ids, err
	}
	boot, err := app.Bootstrap(ctx)
	if err != nil {
		return ids, err
	}
	create := func(name, kind, tracking, amount string) (domain.AccountID, error) {
		row, err := app.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: kind, TrackingMode: tracking, BalanceSheetRole: "asset",
			DefaultCurrency: "USD", InitialAmount: amount, OwnerIDs: []domain.MemberID{boot.Members[0].ID},
			IncludeInNetWorth: true, IncludeInPortfolio: true,
		})
		return row.Account.ID, err
	}
	if ids.bank, err = create("Harbor Savings", "bank_account", "balance", "125.4321"); err != nil {
		return ids, err
	}
	if _, err = create("Empty Savings", "bank_account", "balance", "0"); err != nil {
		return ids, err
	}
	if ids.foreign, err = create("Travel Wallet", "cash_on_hand", "holdings", ""); err != nil {
		return ids, err
	}
	if _, err = app.AppendAccountCashValue(ctx, ids.foreign, "25.25", "EUR", ""); err != nil {
		return ids, err
	}
	if ids.broker, err = create("Hill Brokerage", "brokerage", "holdings", ""); err != nil {
		return ids, err
	}
	instrument, err := app.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic Index Fund", Symbol: "QA-FUND", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		return ids, err
	}
	ids.instrument = instrument.ID
	if _, err = app.CreateHolding(ctx, application.HoldingInput{AccountID: ids.broker.String(), InstrumentID: ids.instrument.String(), Quantity: "2"}); err != nil {
		return ids, err
	}
	api := liquidityapi.NewService(app)
	if _, err = api.SaveReservation(ctx, liquidityapi.SaveReservationRequest{
		Source: liquidityapi.SourceRefDTO{Kind: string(domain.SourceAccountValue), AccountID: ids.bank.String()},
		Label:  "Rent", Amount: "40.125",
	}); err != nil {
		return ids, err
	}
	archived, err := create("Old Savings", "bank_account", "balance", "10")
	if err != nil {
		return ids, err
	}
	if _, err = api.SaveReservation(ctx, liquidityapi.SaveReservationRequest{
		Source: liquidityapi.SourceRefDTO{Kind: string(domain.SourceAccountValue), AccountID: archived.String()},
		Label:  "Old reserve", Amount: "5",
	}); err != nil {
		return ids, err
	}
	return ids, app.ArchiveAccount(ctx, archived, true)
}

func openFundsFixture(t *testing.T, populated bool) (*application.Service, *pilot.Session, pilot.Paths, fundsFixture) {
	t.Helper()
	paths := prepareOnboardingProfile(t)
	app, storage := reopenOnboardingProfile(t, paths)
	ids, err := seedAvailableFunds(t.Context(), app, accountTestClock, populated)
	if err != nil {
		t.Fatal(err)
	}
	return app, storage, paths, ids
}

type fundsReadFault struct {
	*application.Service
	attempts       atomic.Int32
	started, allow chan struct{}
}

func (f *fundsReadFault) LiquidityRead(ctx context.Context, query application.LiquidityOverviewQuery) (application.LiquidityReadResult, error) {
	if f.attempts.Add(1) == 1 {
		close(f.started)
		<-f.allow
		return application.LiquidityReadResult{}, errors.New("synthetic liquidity snapshot read failure")
	}
	return f.Service.LiquidityRead(ctx, query)
}

func TestAvailableFundsReadRetryAndSourceNavigation(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openFundsFixture(t, true)
	ref, _, _, _ := openFundsFixture(t, true)
	adapter := liquidityapi.NewService(ref)
	want, err := adapter.Overview(ctx, liquidityapi.OverviewRequest{})
	if err != nil {
		t.Fatal(err)
	}
	fault := &fundsReadFault{Service: app, started: make(chan struct{}), allow: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.allow) })
	funds := newAvailableFundsPage(ctx, fault, func() {})
	accounts := newAccountsPage(ctx, app, func() time.Time { return accountTestClock }, func() {})
	portfolio := newPortfolioPage(ctx, app, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{funds: funds, accounts: accounts, portfolio: portfolio, version: app.MobileDataVersion})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	t.Cleanup(func() { release(); funds.workers.Wait(); accounts.workers.Wait(); portfolio.workers.Wait() })
	settle := func() {
		for range 3 {
			h.frame()
			funds.workers.Wait()
			accounts.workers.Wait()
			portfolio.workers.Wait()
		}
		h.frame()
	}
	click := func(text string) { t.Helper(); h.click(text, false); settle() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	scroll := func(by float32) {
		h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(440, 455), Scroll: f32.Pt(0, by)})
		h.frame()
	}
	// reveal scrolls until the label sits inside the window, so the test does
	// not depend on the exact height of the cards above it.
	reveal := func(text string, margin int) {
		t.Helper()
		for range 80 {
			if node := findPageLabel(&h.router, text); node != nil {
				if y := min((node.Desc.Bounds.Min.Y+node.Desc.Bounds.Max.Y)/2, node.Desc.Bounds.Min.Y+16); y > 0 && y < h.size.Y-margin {
					return
				}
			}
			scroll(60)
		}
		t.Fatalf("could not scroll %q into view", text)
	}

	version := app.MobileDataVersion()
	h.frame()
	h.click("Available funds", false)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("Available Funds did not start loading")
	}
	label("Loading available funds…")
	release()
	settle()
	label("Could not load available funds.")
	click("Retry")
	label("Today · 2026-11-21 (UTC)")
	label("Base currency total · USD")
	label("Partial coverage")
	reveal("Native amounts · EUR", 100)
	label("Native amounts · EUR")
	reveal("Available after reservations · €25.25", 80)
	label("Available after reservations · €25.25")
	reveal("Known available after reservations · US$85.31", 80)
	label("Known available after reservations · US$85.31")
	reveal("Missing exchange rate to the base currency.", 80)
	label("Missing exchange rate to the base currency.")
	if fault.attempts.Load() != 2 {
		t.Fatalf("read retry made %d attempts", fault.attempts.Load())
	}
	assertFundsParity(t, *funds.current, want)
	if got := funds.current.today.FullUnreserved; got != nil {
		t.Fatalf("partial conversion exposed a complete total: %+v", got)
	}

	reveal("Open account · Harbor Savings", 80)
	label("Harbor Savings · USD")
	label("Reserved from available funds · US$40.12")
	label("Available after reservations · US$85.31")
	bankLink := "Open account · Harbor Savings"
	label(bankLink)
	before := funds.list.Position
	if before.First == 0 {
		t.Fatal("account link was tested without leaving the summary position")
	}
	click(bankLink)
	if target := h.shell.trail.current().target; target.account != ids.bank || target.route != accountsRoute {
		t.Fatalf("wrong account destination: %+v", target)
	}
	label("Harbor Savings")
	click("Back")
	if h.shell.activeRoute() != fundsRoute || funds.list.Position.First != before.First {
		t.Fatalf("Back lost the Available Funds position: %+v -> %+v", before, funds.list.Position)
	}
	h.focused(&funds.links[domain.AccountValueSourceRef(ids.bank).Key()].account)
	click("Forward")
	if h.shell.trail.current().target.account != ids.bank {
		t.Fatal("Forward lost the account target")
	}
	click("Back")

	reveal("Open instrument · QA-FUND", 80)
	label("Missing access rules.")
	label("Current value · Unavailable")
	instrument := "Open instrument · QA-FUND"
	label(instrument)
	click(instrument)
	if target := h.shell.trail.current().target; target.instrument != ids.instrument || target.route != portfolioRoute {
		t.Fatalf("wrong instrument destination: %+v", target)
	}
	label("Current price unavailable")
	click("Back")
	label(instrument)
	reveal("The source is missing, archived, or no longer positive.", 4)
	label("Unresolved reservation · Old reserve")
	label("The source is missing, archived, or no longer positive.")
	if app.MobileDataVersion() != version {
		t.Fatal("a view, retry or source navigation wrote to the ledger")
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reopened.SetClock(func() time.Time { return accountTestClock })
	report, err := readAvailableFunds(ctx, reopened)
	if err != nil {
		t.Fatal(err)
	}
	assertFundsParity(t, report, want)
}

func TestAvailableFundsWithoutSourcesShowsKnownZero(t *testing.T) {
	app, _, _, _ := openFundsFixture(t, false)
	reference, _, _, _ := openFundsFixture(t, false)
	ctx := t.Context()
	funds := newAvailableFundsPage(ctx, app, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{funds: funds})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Available funds", false)
	funds.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "No funding sources.")
	assertPageLabel(t, &h.router, "Available after reservations · US$0.00")
	want, err := liquidityapi.NewService(reference).Overview(ctx, liquidityapi.OverviewRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertFundsParity(t, *funds.current, want)
	if funds.current.today.Status != domain.StatusComplete || !funds.current.today.FullUnreserved.IsZero() {
		t.Fatalf("empty household was not a known zero: %+v", funds.current.today)
	}
}

func assertFundsParity(t *testing.T, got availableFundsRead, want liquidityapi.LiquidityOverviewDTO) {
	t.Helper()
	read := got.report.Overview
	if read.LocalDate != want.LocalDate || read.Timezone != want.Timezone || read.BaseCurrency.String() != want.BaseCurrency || wire.FormatTime(read.AsOf) != want.AsOf {
		t.Fatalf("different availability clock or currency: Gio %+v, Wails %+v", read, want)
	}
	money := func(actual *domain.Money, expected *wire.MoneyView) {
		t.Helper()
		if (actual == nil) != (expected == nil) || (actual != nil && (actual.CanonicalAmount() != expected.Amount || actual.Currency().String() != expected.Currency)) {
			t.Fatalf("money = %+v, Wails %+v", actual, expected)
		}
	}
	i := slices.IndexFunc(want.Buckets, func(b liquidityapi.LiquidityBucketDTO) bool { return b.HorizonOn == want.LocalDate })
	if i < 0 {
		t.Fatal("Wails lacked today's bucket")
	}
	day := want.Buckets[i]
	if day.Status != string(got.today.Status) || !slices.Equal(day.Warnings, got.today.Warnings) || len(day.NativeCurrencyGroups) != len(got.today.NativeCurrencyGroups) {
		t.Fatalf("different coverage: Gio %+v, Wails %+v", got.today, day)
	}
	money(got.today.FullAvailable, day.FullAvailable)
	money(got.today.KnownAvailableSubtotal, day.KnownAvailableSubtotal)
	money(got.today.AppliedReserveSubtotal, day.AppliedReserveSubtotal)
	money(got.today.FullUnreserved, day.FullUnreserved)
	money(got.today.KnownUnreservedSubtotal, day.KnownUnreservedSubtotal)
	for i, group := range got.today.NativeCurrencyGroups {
		other := day.NativeCurrencyGroups[i]
		if other.Currency != group.Currency.String() || other.Status != string(group.Status) {
			t.Fatalf("different native group: Gio %+v, Wails %+v", group, other)
		}
		money(group.FullAvailable, other.FullAvailable)
		money(group.KnownAvailableSubtotal, other.KnownAvailableSubtotal)
		money(group.AppliedReserveSubtotal, other.AppliedReserveSubtotal)
		money(group.FullUnreserved, other.FullUnreserved)
		money(group.KnownUnreservedSubtotal, other.KnownUnreservedSubtotal)
	}
	if len(got.report.Sources) != len(want.Sources) || len(read.UnresolvedReservations) != len(want.UnresolvedReservations) {
		t.Fatal("Gio and Wails returned different source or unresolved-reservation counts")
	}
	for _, entry := range got.report.Sources {
		source := entry.Result
		at := slices.IndexFunc(want.Sources, func(v liquidityapi.LiquiditySourceDTO) bool {
			return v.DisplayName == source.DisplayName && v.NativeCurrency == source.NativeCurrency.String()
		})
		if at < 0 {
			t.Fatalf("Wails lacks source %s", source.DisplayName)
		}
		other := want.Sources[at]
		if source.Excluded != other.Excluded || !slices.Equal(source.Reasons, other.Reasons) || string(source.Ref.Kind) != other.SourceRef.Kind {
			t.Fatalf("different availability decisions for %s", source.DisplayName)
		}
		money(source.CurrentNativeValue, other.CurrentNativeValue)
		money(&source.ReservationRequested, &other.ReservationRequested)
		a := slices.IndexFunc(source.BucketResults, func(v domain.LiquidityBucketResult) bool { return v.HorizonOn == read.LocalDate })
		b := slices.IndexFunc(other.BucketResults, func(v liquidityapi.BucketResultDTO) bool { return v.HorizonOn == want.LocalDate })
		if a < 0 || b < 0 {
			t.Fatalf("source %s lacks today's result", source.DisplayName)
		}
		left, right := source.BucketResults[a], other.BucketResults[b]
		if string(left.Status) != right.Status || !slices.Equal(left.Reasons, right.Reasons) {
			t.Fatalf("different source completeness for %s", source.DisplayName)
		}
		money(left.NetNative, right.NetNative)
		money(left.AppliedReserveNative, right.AppliedReserveNative)
		money(left.UnreservedNative, right.UnreservedNative)
	}
	for i, row := range read.UnresolvedReservations {
		expected := want.UnresolvedReservations[i]
		if row.Reservation.Label != expected.Reservation.Label || row.Reason != expected.Reason {
			t.Fatalf("different unresolved reservation: Gio %+v, Wails %+v", row, expected)
		}
		money(&row.Reservation.Amount, &expected.Reservation.Amount)
	}
}
