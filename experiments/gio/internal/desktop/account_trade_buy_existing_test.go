package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	portfolioapi "github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func seedExistingBuy(ctx context.Context, app *application.Service, now time.Time) (saleFixture, error) {
	ids, err := seedFirstBuy(ctx, app, now)
	if err != nil {
		return saleFixture{}, err
	}
	first, err := historyapi.NewService(app).RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeTrade, Side: "buy", SettlementAccountID: ids.account.String(), InstrumentID: ids.instrument.String(),
		Quantity: "2", Gross: "42", GrossCurrency: "USD", Fee: "2", FeeCurrency: "USD",
		EffectiveAt: now.Add(-24 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		return saleFixture{}, err
	}
	return saleFixture{firstBuyFixture: ids, holding: domain.HoldingID(first.Activity.TradeDetail.HoldingID)}, nil
}

func openExistingBuyFixture(t *testing.T) (*application.Service, *pilot.Session, pilot.Paths, saleFixture) {
	t.Helper()
	paths := prepareOnboardingProfile(t)
	app, storage := reopenOnboardingProfile(t, paths)
	ids, err := seedExistingBuy(t.Context(), app, accountTestClock)
	if err != nil {
		t.Fatal(err)
	}
	return app, storage, paths, ids
}

func TestExistingBuyAndReopenedHoldingMatchWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	fault := &cashMovementFault{Service: app, loseAttempt: 2, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	now := accountTestClock.Add(2345 * time.Millisecond)
	clock := func() time.Time { return now }
	app.SetClock(clock)
	ref.SetClock(clock)
	accounts := newAccountsPage(ctx, app, clock, func() {})
	accounts.investment = &accountInvestment{service: fault}
	ledger := newLedgerHistory(ctx, fault, clock, func() {})
	investments := newPortfolioPage(ctx, app, func() {})
	overview := newOverviewPage(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, history: ledger, portfolio: investments, overview: overview, version: app.MobileDataVersion})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	t.Cleanup(func() { release(); accounts.workers.Wait() })
	settle := func() {
		for range 3 {
			h.frame()
			accounts.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
			investments.workers.Wait()
			overview.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	fields := func(d *accountInvestmentDraft, quantity, gross, fee string) {
		t.Helper()
		setBankField(h, "Bought quantity", &d.quantity, quantity)
		setBankField(h, "Gross before fees · USD", &d.gross, gross)
		setBankField(h, "Fee (optional) · USD", &d.fee, fee)
	}
	request := func(f saleFixture, side, qty, gross, fee, at string) historyapi.ChangeCommandRequest {
		return historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeTrade, Side: side, SettlementAccountID: f.account.String(), InstrumentID: f.instrument.String(),
			HoldingID: f.holding.String(), Quantity: qty, Gross: gross, GrossCurrency: "USD", Fee: fee, FeeCurrency: "USD",
			EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: at, MutationID: domain.NewMutationID().String(),
		}
	}
	commitReference := func(plan history.ReviewedInvestment, r historyapi.ChangeCommandRequest) domain.ActivityID {
		t.Helper()
		api := historyapi.NewService(ref)
		w, err := api.PreviewChange(ctx, r)
		if err != nil || len(w.Review) != len(plan.Preview.Review) {
			t.Fatalf("unexpected Wails review: %+v / %v", w, err)
		}
		for i, e := range plan.Preview.Review {
			if w.Review[i].Target != string(e.Target) || w.Review[i].Before != e.Before ||
				w.Review[i].Delta != e.Delta || w.Review[i].After != e.After || w.Review[i].Currency != string(e.Currency) {
				t.Fatalf("Wails preview endpoint differs: %+v / %+v", w.Review[i], e)
			}
		}
		w, err = api.RecordChange(ctx, r)
		if err != nil || w.Activity.TradeDetail.HoldingID != r.HoldingID {
			t.Fatalf("unexpected Wails commit: %+v / %v", w, err)
		}
		if w.Activity.EffectiveAt != wire.FormatTime(plan.Preview.Activity.EffectiveAt) ||
			w.Activity.TradeDetail.Fee.Amount != r.Fee {
			t.Fatalf("Wails returned different time or fee: %+v", w.Activity)
		}
		return domain.ActivityID(w.Activity.ID)
	}
	assertPosition := func(reader *application.Service, f saleFixture, qty, cost, gain, cash, net string, count int) {
		t.Helper()
		list, err := reader.HoldingsByAccounts(ctx, []domain.AccountID{f.account})
		if err != nil || len(list[f.account]) != 1 || list[f.account][0].ID != f.holding {
			t.Fatalf("holding identity changed: %+v / %v", list, err)
		}
		wp, err := analytics.NewService(reader).AccountHoldingDetails(ctx, f.account.String())
		if err != nil || len(wp) != 1 || wp[0].Amounts.Quantity != qty || wp[0].Amounts.TotalCost == nil || wp[0].Amounts.TotalCost.Amount != cost {
			t.Fatalf("unexpected Wails Go holding: %+v / %v", wp, err)
		}
		wgain, err := reader.HoldingGain(ctx, f.holding)
		if err != nil || wgain.RealizedGain == nil || wgain.RealizedGain.Amount != gain {
			t.Fatalf("unexpected realized result: %+v / %v", wgain, err)
		}
		av, err := reader.AccountValuation(ctx, f.account)
		if err != nil || !slices.ContainsFunc(av.Components, func(c domain.ValuationComponent) bool {
			return c.HoldingID == nil && c.NativeCurrency == "USD" && c.NativeAmount == cash
		}) {
			t.Fatalf("unexpected cash sleeve: %+v / %v", av.Components, err)
		}
		ov, err := portfolioapi.NewService(reader).Overview(ctx, account.AccountFilterRequest{})
		if err != nil || !ov.Complete || ov.NetWorth != net {
			t.Fatalf("unexpected net worth: %+v / %v", ov, err)
		}
		items, err := reader.ListActivities(ctx, 20)
		if err != nil || len(items) != count {
			t.Fatalf("unexpected history: %+v / %v", items, err)
		}
		for _, a := range items {
			if a.TradeDetail == nil || a.TradeDetail.HoldingID != f.holding {
				t.Fatalf("trade lost the original holding link: %+v", a)
			}
		}
	}

	settle()
	label("$ 236.00")
	click("Accounts")
	click("Harbor Brokerage")
	click("Record buy")
	d := accounts.investment.draft
	chooseTradeInstrument(t, h, d, ids.instrument, settle)
	if len(d.sources.Instruments) != 2 || d.sources.Holdings[d.instrument].ID != ids.holding {
		t.Fatalf("picker offered an unexpected instrument: %+v", d.sources)
	}
	fields(d, "2", "155", "2")
	click("Review buy")
	label("The account balance at the effective time cannot cover this amount.")
	if len(fault.attempts) != 0 {
		t.Fatal("an over-budget preview wrote an activity")
	}
	fields(d, "2", "63", "1")
	click("Review buy")
	label("Quantity before buy · 2")
	label("Quantity after buy · 4")
	label("Cost basis before buy · 44 USD")
	label("Total cost basis after buy · 108 USD")

	// Change this same Holding between review and confirmation.
	for _, f := range []struct {
		app *application.Service
		ids saleFixture
	}{{app, ids}, {ref, refIDs}} {
		if _, err := historyapi.NewService(f.app).RecordChange(ctx, request(f.ids, "buy", "1", "26", "1", "01:00:01")); err != nil {
			t.Fatal(err)
		}
	}
	click("Confirm buy")
	label("The ledger changed. Review the buy again before confirming.")
	assertPosition(app, ids, "3", "71", "0", "129", "249", 2)
	click("Review buy")
	label("Quantity before buy · 3")
	label("Quantity after buy · 5")
	label("Cost basis before buy · 71 USD")
	label("Total cost basis after buy · 135 USD")
	label("Weighted average cost per unit · 27 USD")
	label("Cost per unit for this buy including fee · 32 USD")
	label("Resulting balance · 65 USD")
	addition := *d.review
	b := semanticBounds(t, &h.router, "Confirm buy", false)
	point := b.Min.Add(b.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("buy confirmation did not start")
	}
	h.clickAt(point)
	release()
	settle()
	label("Posting could not be confirmed. Retry the same submission to check its result.")
	click("Retry same submission")
	if len(fault.attempts) != 3 || !reflect.DeepEqual(fault.attempts[1], fault.attempts[2]) ||
		ledger.reads.posted.ID != fault.firstActivity || fault.firstActivity == "" {
		t.Fatalf("a retry did not return the original buy: %+v", fault.attempts)
	}
	label("Buy recorded.")
	label("Could not load this activity.")
	click("Retry")
	label("Unit price · 31.5 USD")
	label("Effective: 2026-11-22 01:00:02.345 (Asia/Singapore)")
	if len(fault.attempts) != 3 {
		t.Fatal("an acknowledged activity read submitted another buy")
	}
	click("Open instrument · QA-BUY")
	label("Quantity 5")
	label("Cost basis · USD $ 135.00")
	label("Native market value · USD $ 200.00")
	refAddition := request(refIDs, "buy", "2", "63", "1", "")
	refAddition.EffectiveLocalDate = ""
	refAddition.EffectiveAt = now.Format(time.RFC3339Nano)
	commitReference(addition, refAddition)
	assertPosition(app, ids, "5", "135", "0", "65", "265", 3)
	assertPosition(ref, refIDs, "5", "135", "0", "65", "265", 3)

	// A reviewed trade at an older or exactly equal instant must not
	// rewrite an existing Holding's cost sequence.
	for _, side := range []domain.TradeSide{domain.TradeBuy, domain.TradeSell} {
		for _, at := range []time.Time{now.Add(-time.Millisecond), now} {
			_, err := history.ReviewTrade(ctx, app, history.TradeInput{
				Side: side, Account: ids.account, Instrument: ids.instrument, Holding: ids.holding, Currency: "USD",
				Quantity: "1", Gross: "40", Fee: "1", EffectiveAt: at,
			})
			var issue *domain.Error
			if !errors.As(err, &issue) || issue.Code != domain.ErrInvalidChangeTime {
				t.Fatalf("%s at %s should preserve strict cost order: %v", side, at, err)
			}
		}
	}

	for _, sale := range []struct {
		second                           int
		qty, gross, fee, at              string
		remaining, cost, gain, cash, net string
		count                            int
	}{
		{3, "2", "70.125", "1.125", "01:00:03", "3", "81", "15", "134", "254", 4},
		{4, "3", "60", "2", "01:00:04", "0", "0", "-8", "192", "192", 5},
	} {
		now = accountTestClock.Add(time.Duration(sale.second) * time.Second)
		plan, err := history.ReviewTrade(ctx, app, history.TradeInput{
			Side: domain.TradeSell, Account: ids.account, Instrument: ids.instrument, Holding: ids.holding, Currency: "USD",
			Quantity: sale.qty, Gross: sale.gross, Fee: sale.fee, Date: "2026-11-22", Time: sale.at,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := plan.Commit(ctx, app)
		if err != nil || !result.Activity.EffectiveAt.Equal(now) ||
			result.Activity.TradeDetail.Fee.CanonicalAmount() != sale.fee {
			t.Fatalf("sale did not store its exact time and fee: %+v / %v", result.Activity, err)
		}
		commitReference(plan, request(refIDs, "sell", sale.qty, sale.gross, sale.fee, sale.at))
		assertPosition(app, ids, sale.remaining, sale.cost, sale.gain, sale.cash, sale.net, sale.count)
		assertPosition(ref, refIDs, sale.remaining, sale.cost, sale.gain, sale.cash, sale.net, sale.count)
	}
	now = accountTestClock.Add(6 * time.Minute)
	click("Open account · Harbor Brokerage")
	if accounts.canSell() || accounts.canFirstBuy() {
		t.Fatal("a liquidated holding became eligible for sell or a new first holding")
	}
	click("Record buy")
	chooseTradeInstrument(t, h, accounts.investment.draft, ids.instrument, settle)
	d = accounts.investment.draft
	if holding := d.sources.Holdings[d.instrument]; holding.ID != ids.holding || !holding.Quantity.IsZero() {
		t.Fatalf("closed holding was not selectable: %+v", holding)
	}
	fields(d, "4", "96", "4")
	click("Review buy")
	label("Quantity before buy · 0")
	label("Quantity after buy · 4")
	label("Cost basis before buy · 0 USD")
	label("Total cost basis after buy · 100 USD")
	label("Weighted average cost per unit · 25 USD")
	label("Resulting balance · 92 USD")
	reentry := *d.review
	click("Confirm buy")
	label("Buy recorded.")
	reentryID := ledger.reads.posted.ID
	commitReference(reentry, request(refIDs, "buy", "4", "96", "4", "01:06"))
	assertPosition(app, ids, "4", "100", "-8", "92", "252", 6)
	assertPosition(ref, refIDs, "4", "100", "-8", "92", "252", 6)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reopened.SetClock(clock)
	assertPosition(reopened, ids, "4", "100", "-8", "92", "252", 6)
	r, err := reentry.Commit(ctx, reopened)
	if err != nil || r.Activity.ID != reentryID {
		t.Fatalf("reopened buy receipt = %+v / %v, want %s", r, err, reentryID)
	}
	assertPosition(reopened, ids, "4", "100", "-8", "92", "252", 6)
}
