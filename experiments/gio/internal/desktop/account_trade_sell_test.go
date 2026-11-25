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
)

type saleFixture struct {
	firstBuyFixture
	holding domain.HoldingID
}

func seedSales(ctx context.Context, service *application.Service, now time.Time) (saleFixture, error) {
	ids, err := seedFirstBuy(ctx, service, now)
	if err != nil {
		return saleFixture{}, err
	}
	fixture := saleFixture{firstBuyFixture: ids}
	api := historyapi.NewService(service)
	for _, buy := range []struct {
		quantity, gross, fee string
		daysAgo              int
	}{
		{"2", "42", "2", 2},
		{"3", "90", "1", 1},
	} {
		r, err := api.RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeTrade, Side: "buy", SettlementAccountID: ids.account.String(), InstrumentID: ids.instrument.String(),
			Quantity: buy.quantity, Gross: buy.gross, GrossCurrency: "USD", Fee: buy.fee, FeeCurrency: "USD",
			EffectiveAt: now.AddDate(0, 0, -buy.daysAgo).Add(time.Minute).Format(time.RFC3339),
		})
		if err != nil {
			return saleFixture{}, err
		}
		fixture.holding = domain.HoldingID(r.Activity.TradeDetail.HoldingID)
	}
	return fixture, nil
}

func openSaleFixture(t *testing.T) (*application.Service, *pilot.Session, pilot.Paths, saleFixture) {
	t.Helper()
	paths := prepareOnboardingProfile(t)
	app, session := reopenOnboardingProfile(t, paths)
	ids, err := seedSales(t.Context(), app, accountTestClock)
	if err != nil {
		t.Fatal(err)
	}
	return app, session, paths, ids
}

func TestSellPartialAndLiquidationMatchWailsAndReplay(t *testing.T) {
	ctx := t.Context()
	service, storage, paths, ids := openSaleFixture(t)
	reference, _, _, ref := openSaleFixture(t)
	refAPI := historyapi.NewService(reference)
	fault := &cashMovementFault{Service: service, loseAttempt: 1, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	now := accountTestClock
	clock := func() time.Time { return now }
	service.SetClock(clock)
	reference.SetClock(clock)
	accounts := newAccountsPage(ctx, service, clock, func() {})
	accounts.investment = &accountInvestment{service: fault}
	ledger := newLedgerHistory(ctx, fault, clock, func() {})
	investments := newPortfolioPage(ctx, service, func() {})
	overview := newOverviewPage(ctx, service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, history: ledger, portfolio: investments, overview: overview, version: service.MobileDataVersion})
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
	settle()
	label("$ 265.00")
	click("Accounts")
	click("Harbor Brokerage")
	click("Record sell")
	d := accounts.investment.draft
	if d.kind != domain.ActivitySell || len(d.sources.Instruments) != 1 || d.sources.Holdings[d.instrument].ID != ids.holding {
		t.Fatalf("unexpected sell target: %+v", d.sources)
	}
	setFields := func(d *accountInvestmentDraft, quantity, gross, fee string) {
		t.Helper()
		setBankField(h, "Sold quantity", &d.quantity, quantity)
		setBankField(h, "Gross proceeds before fees · USD", &d.gross, gross)
		setBankField(h, "Fee (optional) · USD", &d.fee, fee)
	}
	setFields(d, "5.00000001", "70.125", "1.125")
	click("Review sell")
	label("This holding did not have enough quantity at the effective time.")
	if len(fault.attempts) != 0 {
		t.Fatal("rejecting an oversell submitted a write")
	}
	setBankField(h, "Sold quantity", &d.quantity, "2")
	click("Review sell")
	label("Gross proceeds · 70.125 USD")
	label("Trading fee · 1.125 USD")
	label("Remaining quantity · 3")
	label("Remaining cost basis · 81 USD")
	label("Realized gain or loss · 15 USD")
	label("Settlement cash change · +69 USD")
	label("Resulting balance · 134 USD")
	partial := *d.review
	bounds := semanticBounds(t, &h.router, "Confirm sell", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("sell confirmation did not start")
	}
	h.clickAt(point)
	release()
	settle()
	label("Posting could not be confirmed. Retry the same submission to check its result.")
	h.click("Overview", false)
	if h.shell.activeRoute() != accountsRoute || len(fault.attempts) != 1 || !accounts.investmentUnresolved() {
		t.Fatal("the uncertain sale allowed navigation or an extra submission")
	}
	click("Retry same submission")
	if len(fault.attempts) != 2 || !reflect.DeepEqual(fault.attempts[0], fault.attempts[1]) ||
		ledger.reads.posted.ID != fault.firstActivity || fault.firstActivity == "" {
		t.Fatalf("a retry did not return the original sale: %+v", fault.attempts)
	}
	label("Sell recorded.")
	label("Could not load this activity.")
	click("Retry")
	label("Unit price · 35.0625 USD")
	if len(fault.attempts) != 2 {
		t.Fatal("a read retry issued another sale")
	}
	click("Open instrument · QA-BUY")
	label("Quantity 3")
	label("Cost basis · USD $ 81.00")
	label("Native market value · USD $ 120.00")

	recordReference := func(review history.ReviewedInvestment, quantity, gross, fee, at string) domain.ActivityID {
		t.Helper()
		request := historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeTrade, Side: "sell", SettlementAccountID: ref.account.String(), InstrumentID: ref.instrument.String(),
			HoldingID: ref.holding.String(), Quantity: quantity, Gross: gross, GrossCurrency: "USD", Fee: fee, FeeCurrency: "USD",
			EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: at, MutationID: domain.NewMutationID().String(),
		}
		r, err := refAPI.PreviewChange(ctx, request)
		if err != nil || len(r.Review) != len(review.Preview.Review) {
			t.Fatalf("bad Wails preview: %+v / %v", r, err)
		}
		for i, want := range review.Preview.Review {
			got := r.Review[i]
			if got.Before != want.Before || got.Delta != want.Delta || got.After != want.After || got.Target != string(want.Target) ||
				got.Currency != string(want.Currency) {
				t.Fatalf("Wails preview mismatch: %+v / %+v", got, want)
			}
		}
		r, err = refAPI.RecordChange(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		return domain.ActivityID(r.Activity.ID)
	}

	assertState := func(app *application.Service, fixture saleFixture, saleID domain.ActivityID, quantity, gross, fee, remaining, cost, gain, value, cash, net string, sales int) {
		t.Helper()
		rows, err := app.ListActivities(ctx, 20)
		if err != nil || len(rows) != 2+sales {
			t.Fatalf("unexpected History: %+v / %v", rows, err)
		}
		sale, err := app.Activity(ctx, saleID)
		trade := sale.TradeDetail
		if err != nil || sale.Kind != domain.ActivitySell || sale.EffectiveLocalDate != "2026-11-22" ||
			trade == nil || trade.HoldingID != fixture.holding || trade.Quantity.Canonical() != quantity ||
			trade.Gross.CanonicalAmount() != gross || trade.Fee == nil || trade.Fee.CanonicalAmount() != fee || len(sale.Effects) != 3 {
			t.Fatalf("bad recorded sale: %+v / %v", sale, err)
		}
		if q, principal, charge := sale.Effects[0], sale.Effects[1], sale.Effects[2]; q.Direction != domain.EffectRemoved ||
			q.Classification != domain.ClassificationTradePrincipal || principal.Direction != domain.EffectAdded ||
			principal.Classification != domain.ClassificationTradePrincipal || charge.Direction != domain.EffectRemoved ||
			charge.Classification != domain.ClassificationFee {
			t.Fatalf("incorrect effect directions or classifications: %+v", sale.Effects)
		}
		positions, err := analytics.NewService(app).AccountHoldingDetails(ctx, fixture.account.String())
		if err != nil || len(positions) != 1 || positions[0].Amounts.Quantity != remaining || positions[0].Amounts.TotalCost == nil ||
			positions[0].Amounts.TotalCost.Amount != cost || positions[0].Amounts.CurrentValue == nil || positions[0].Amounts.CurrentValue.Amount != value {
			t.Fatalf("unexpected remaining Wails Go holding projection: %+v / %v", positions, err)
		}
		holding, err := app.HoldingGain(ctx, fixture.holding)
		if err != nil || holding.RealizedGain == nil || holding.RealizedGain.Amount != gain {
			t.Fatalf("unexpected shared realized gain: %+v / %v", holding, err)
		}
		ob, err := app.AccountValuation(ctx, fixture.account)
		if err != nil || !slices.ContainsFunc(ob.Components, func(c domain.ValuationComponent) bool {
			return c.HoldingID == nil && c.NativeCurrency == "USD" && c.NativeAmount == cash
		}) {
			t.Fatalf("unexpected shared cash: %+v / %v", ob.Components, err)
		}
		ov, err := portfolioapi.NewService(app).Overview(ctx, account.AccountFilterRequest{})
		if err != nil || !ov.Complete || ov.NetWorth != net {
			t.Fatalf("unexpected Wails Go net worth: %+v / %v", ov, err)
		}
	}

	refPartial := recordReference(partial, "2", "70.125", "1.125", "01:00")
	assertState(service, ids, fault.firstActivity, "2", "70.125", "1.125", "3", "81", "15", "120", "134", "254", 1)
	assertState(reference, ref, refPartial, "2", "70.125", "1.125", "3", "81", "15", "120", "134", "254", 1)
	// A second sale in the same minute cannot yet produce a sequential basis.
	input := history.TradeInput{Side: domain.TradeSell, Account: ids.account, Instrument: ids.instrument, Holding: ids.holding, Currency: "USD",
		Quantity: "3", Gross: "60", Fee: "2", Date: "2026-11-22", Time: "01:00"}
	_, err := history.ReviewTrade(ctx, service, input)
	var issue *domain.Error
	if !errors.As(err, &issue) || issue.Field != "saleSequence" {
		t.Fatalf("missing out-of-sequence sell rejection: %v", err)
	}

	now = accountTestClock.Add(2 * time.Minute)
	click("Open account · Harbor Brokerage")
	click("Record sell")
	d = accounts.investment.draft
	setFields(d, "3", "60", "2")
	click("Review sell")
	label("Remaining quantity · 0")
	label("Remaining cost basis · 0 USD")
	label("Realized gain or loss · -23 USD")
	label("Settlement cash change · +58 USD")
	label("Resulting balance · 192 USD")
	closing := *d.review
	click("Confirm sell")
	label("Sell recorded.")
	soldAll := ledger.reads.posted.ID
	if soldAll == fault.firstActivity || len(fault.attempts) != 3 {
		t.Fatal("the liquidation did not get its own receipt")
	}
	click("Open account · Harbor Brokerage")
	if accounts.canSell() || accounts.canFirstBuy() {
		t.Fatal("a closed position still offers sell or first-buy")
	}
	click("Investment assets")
	label("No investment holdings.")
	click("Ledger activity")
	label("Loaded records: 4")
	refClosing := recordReference(closing, "3", "60", "2", "01:02")
	assertState(reference, ref, refClosing, "3", "60", "2", "0", "0", "-8", "0", "192", "192", 2)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reopened.SetClock(clock)
	assertState(reopened, ids, soldAll, "3", "60", "2", "0", "0", "-8", "0", "192", "192", 2)
	for _, i := range []int{1, 2} {
		frozen := fault.attempts[i]
		posted, err := reopened.RecordChangeGuarded(ctx, frozen.command, frozen.mutation, frozen.hash, frozen.token)
		want := map[int]domain.ActivityID{1: fault.firstActivity, 2: soldAll}[i]
		if err != nil || posted.Activity.ID != want {
			t.Fatalf("reopened receipt = %+v / %v, want %s", posted, err, want)
		}
	}
	assertState(reopened, ids, soldAll, "3", "60", "2", "0", "0", "-8", "0", "192", "192", 2)
}

func TestSellBasisPreviewRejectsInsertionBeforeReversedPositionActivity(t *testing.T) {
	ctx := t.Context()
	service, _, _, ids := openSaleFixture(t)
	api := historyapi.NewService(service)
	// An ordinary later acquisition and its reversal are absent from the
	// current economic cost view. They still exist in the dated audit ledger.
	later, err := api.RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeTrade, Side: "buy", SettlementAccountID: ids.account.String(), InstrumentID: ids.instrument.String(),
		HoldingID: ids.holding.String(), Quantity: "1", Gross: "25", GrossCurrency: "USD",
		EffectiveAt: accountTestClock.Add(-5 * time.Minute).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.UndoChange(ctx, later.Activity.ID); err != nil {
		t.Fatal(err)
	}
	gain, err := service.HoldingGain(ctx, ids.holding)
	if err != nil || gain.Quantity != "5" || gain.TotalCost == nil || gain.TotalCost.Amount != "135" {
		t.Fatalf("test did not restore the original economic facts: %+v / %v", gain, err)
	}

	input := history.TradeInput{Side: domain.TradeSell, Account: ids.account, Instrument: ids.instrument, Holding: ids.holding, Currency: "USD",
		Quantity: "1", Gross: "30", Date: "2026-11-22", Time: "00:30"}
	_, err = history.ReviewTrade(ctx, service, input)
	var issue *domain.Error
	if !errors.As(err, &issue) || issue.Code != domain.ErrInvalidChangeTime || issue.Field != "saleSequence" {
		t.Fatalf("would have previewed later-corrected cost for an earlier transaction: %v", err)
	}
}
