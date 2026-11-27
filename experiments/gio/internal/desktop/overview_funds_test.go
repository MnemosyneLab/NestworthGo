package desktop

import (
	"image"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func TestOverviewAvailableFundsJourney(t *testing.T) {
	ctx := t.Context()
	app, _, _, ids := openFundsFixture(t, true)
	reference, _, _, refIDs := openFundsFixture(t, true)
	clock := func() time.Time { return accountTestClock }
	today := accountTestClock.Format(time.DateOnly)
	month := accountTestClock.AddDate(0, 0, 30).Format(time.DateOnly)
	unlock := accountTestClock.AddDate(0, 0, 14).Format(time.DateOnly)
	zero, basis, fee := 0, "calendar", "0"
	for _, ledger := range []*application.Service{app, reference} {
		boot, err := ledger.Bootstrap(ctx)
		if err != nil {
			t.Fatal(err)
		}
		deferred, err := ledger.CreateAccount(ctx, application.AccountInput{
			Name: "Notice Reserve", AccountType: "bank_account", TrackingMode: "balance", BalanceSheetRole: "asset",
			DefaultCurrency: "USD", InitialAmount: "40", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInNetWorth: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.SaveLiquidityPolicy(ctx, application.SavePolicyInput{
			Source: domain.AccountValueSourceRef(deferred.Account.ID),
			Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &unlock, SettlementDays: &zero, DayBasis: &basis, NormalExitFee: &fee, EarlyKind: "not_allowed"},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.SetFXPreference(ctx, "EUR", "USD", "manual"); err != nil {
			t.Fatal(err)
		}
	}
	adapter := liquidityapi.NewService(reference)
	fault := &fundsReadFault{Service: app, started: make(chan struct{}), allow: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.allow) })
	overview := newOverviewPage(ctx, app, clock, func() {})
	funds := newAvailableFundsPage(ctx, fault, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{overview: overview, funds: funds, version: app.MobileDataVersion})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	t.Cleanup(func() { release(); overview.workers.Wait(); funds.workers.Wait() })
	settle := func() {
		for range 4 {
			h.frame()
			overview.workers.Wait()
			funds.workers.Wait()
		}
		h.frame()
	}
	assertAmount := func(day, exact string, status domain.CompletenessStatus) {
		t.Helper()
		want, err := adapter.Overview(ctx, liquidityapi.OverviewRequest{})
		if err != nil {
			t.Fatal(err)
		}
		got := fundsBucketForHorizon(overview.funds.current.Overview.Buckets, day)
		if got == nil || got.Status != status {
			t.Fatalf("%s coverage: %+v, want %s", day, got, status)
		}
		amount := got.FullUnreserved
		if status == domain.StatusPartial {
			if amount != nil {
				t.Fatalf("%s presented a partial result as a full total: %v", day, amount)
			}
			amount = got.KnownUnreservedSubtotal
		}
		for _, bucket := range want.Buckets {
			if bucket.HorizonOn != day {
				continue
			}
			refAmount := bucket.FullUnreserved
			if status == domain.StatusPartial {
				refAmount = bucket.KnownUnreservedSubtotal
			}
			if amount == nil || refAmount == nil || amount.CanonicalAmount() != exact || refAmount.Amount != exact ||
				amount.Currency() != "USD" || refAmount.Currency != "USD" || bucket.Status != string(status) {
				t.Fatalf("%s: Gio %v Wails %+v, want %s USD (%s)", day, amount, refAmount, exact, status)
			}
			return
		}
		t.Fatalf("Wails omitted %s", day)
	}

	h.frame()
	overview.workers.Wait()
	h.frame()
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("Overview did not start the independent liquidity query")
	}
	assertPageLabel(t, &h.router, "Loading available funds…")
	release()
	settle()
	assertPageLabel(t, &h.router, "Could not load available funds.")
	assertPageLabel(t, &h.router, "Incomplete valuation")
	h.click("Retry", false)
	settle()
	assertAmount(today, "85.3071", domain.StatusPartial)
	assertAmount(month, "125.3071", domain.StatusPartial)
	assertPageLabel(t, &h.router, "US$85.31")
	assertPageLabel(t, &h.router, "US$125.31")
	h.shell.prefs.saved.DecimalSeparator = settings.DecimalComma
	h.shell.prefs.saved.GroupingSeparator = settings.GroupingDot
	h.frame()
	assertPageLabel(t, &h.router, "US$85,31")
	assertPageLabel(t, &h.router, "US$125,31")
	h.shell.prefs.saved.DecimalSeparator = settings.DecimalDot
	h.shell.prefs.saved.GroupingSeparator = settings.GroupingComma
	h.frame()

	h.click("Open available funds", false)
	settle()
	if h.shell.activeRoute() != fundsRoute || funds.state.filter != fundsAll {
		t.Fatalf("default link did not open the unfiltered Available Funds page: %+v", funds.state)
	}
	assertPageLabel(t, &h.router, "US$85.31")
	assertFundsParity(t, *funds.current, mustReadFundsAdapter(t, adapter))
	h.click("Back", false)
	settle()
	h.focused(&overview.funds.detail)

	h.size = image.Pt(700, 460)
	h.frame()
	h.revealed("Within 30 days · 1 item needs availability rules", false)
	for range 50 {
		if h.router.Source().Focused(&overview.funds.rules[1]) {
			break
		}
		h.router.MoveFocus(key.FocusForward)
		h.frame()
	}
	h.focused(&overview.funds.rules[1])
	h.press(key.NameReturn)
	settle()
	if h.shell.activeRoute() != fundsRoute || funds.state.horizonOn != month || funds.state.filter != fundsNeedsRules || funds.state.includeEarly {
		t.Fatalf("missing-rules link did not preserve the normal 30-day scope: %+v", funds.state)
	}
	assertPageLabel(t, &h.router, "QA-FUND · USD")
	if findPageLabel(&h.router, "Harbor Savings · USD") != nil {
		t.Fatal("scoped source list included an account with known rules")
	}
	h.click("Back", false)
	settle()
	h.focused(&overview.funds.rules[1])
	h.click("Forward", false)
	settle()
	if funds.state.horizonOn != month || funds.state.filter != fundsNeedsRules {
		t.Fatalf("Forward lost the horizon or filter: %+v", funds.state)
	}
	h.click("Back", false)
	settle()

	for _, entry := range []struct {
		app *application.Service
		ids fundsFixture
	}{{app, ids}, {reference, refIDs}} {
		if _, err := entry.app.AppendManualFXQuote(ctx, "EUR", "USD", "1.2", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := entry.app.AppendManualInstrumentQuote(ctx, entry.ids.instrument, "12.3456", "", false); err != nil {
			t.Fatal(err)
		}
	}
	settle()
	assertAmount(today, "115.6071", domain.StatusPartial)
	assertAmount(month, "155.6071", domain.StatusPartial)

	for _, entry := range []struct {
		app *application.Service
		ids fundsFixture
	}{{app, ids}, {reference, refIDs}} {
		holdings, err := entry.app.HoldingsByAccounts(ctx, []domain.AccountID{entry.ids.broker})
		if err != nil || len(holdings[entry.ids.broker]) != 1 {
			t.Fatalf("holding fixture: %v, %v", holdings, err)
		}
		if _, err := entry.app.SaveLiquidityPolicy(ctx, application.SavePolicyInput{
			Source: domain.HoldingSourceRef(entry.ids.broker, holdings[entry.ids.broker][0].ID),
			Policy: application.ProductPolicyInput{AccessKind: "on_request", SettlementDays: &zero, DayBasis: &basis, NormalExitFee: &fee, EarlyKind: "not_allowed"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	settle()
	assertAmount(today, "140.2983", domain.StatusComplete)
	assertAmount(month, "180.2983", domain.StatusComplete)
	assertPageLabel(t, &h.router, "No availability rules missing for this date")
}

func mustReadFundsAdapter(t *testing.T, adapter *liquidityapi.Service) liquidityapi.LiquidityOverviewDTO {
	t.Helper()
	result, err := adapter.Overview(t.Context(), liquidityapi.OverviewRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
