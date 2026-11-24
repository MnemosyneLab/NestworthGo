package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
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
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
)

type firstBuyFixture struct {
	account             domain.AccountID
	instrument, foreign domain.InstrumentID
}

func seedFirstBuy(ctx context.Context, service *application.Service, now time.Time) (firstBuyFixture, error) {
	var fixture firstBuyFixture
	service.SetClock(func() time.Time { return now.Add(-48 * time.Hour) })
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic first buy", BaseCurrency: "USD", MemberNames: []string{"Ana Example"},
	}); err != nil {
		return fixture, err
	}
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		return fixture, err
	}
	broker, err := service.CreateAccount(ctx, application.AccountInput{
		Name: "Harbor Brokerage", AccountType: "brokerage", TrackingMode: "holdings", BalanceSheetRole: "asset",
		DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInNetWorth: true, IncludeInPortfolio: true,
	})
	if err != nil {
		return fixture, err
	}
	fixture.account = broker.Account.ID
	if _, err = service.AppendAccountCashValue(ctx, fixture.account, "200", "USD", ""); err != nil {
		return fixture, err
	}
	if _, err = service.StartHistory(ctx, "Asia/Singapore"); err != nil {
		return fixture, err
	}
	i, err := service.CreateInstrument(ctx, application.InstrumentInput{Name: "Synthetic Index Fund", Symbol: "QA-BUY", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		return fixture, err
	}
	fixture.instrument = i.ID
	i, err = service.CreateInstrument(ctx, application.InstrumentInput{Name: "Euro Sample Fund", Symbol: "QA-EUR", Type: "etf", QuoteCurrency: "EUR", QuoteSource: "manual"})
	if err != nil {
		return fixture, err
	}
	fixture.foreign = i.ID
	service.SetClock(func() time.Time { return now })
	if _, err := service.AppendManualInstrumentQuote(ctx, fixture.instrument, "40", now.Add(-time.Hour).Format(time.RFC3339), false); err != nil {
		return fixture, err
	}
	return fixture, nil
}

func openFirstBuyFixture(t *testing.T) (*application.Service, *pilot.Session, pilot.Paths, firstBuyFixture) {
	t.Helper()
	paths := prepareOnboardingProfile(t)
	service, session := reopenOnboardingProfile(t, paths)
	ids, err := seedFirstBuy(t.Context(), service, accountTestClock)
	if err != nil {
		t.Fatal(err)
	}
	return service, session, paths, ids
}

func TestFirstBuyGioPreviewReplayAndWailsParity(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openFirstBuyFixture(t)
	ref, _, _, refIDs := openFirstBuyFixture(t)
	adapter := historyapi.NewService(ref)
	fault := &cashMovementFault{Service: app, loseAttempt: 2, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	clock := func() time.Time { return accountTestClock }
	accounts := newAccountsPage(ctx, app, clock, func() {})
	accounts.trade = &accountTrade{service: fault}
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
	settle()
	label("$ 200.00")
	click("Accounts")
	click("Harbor Brokerage")
	click("Record first buy")
	d := accounts.trade.draft
	if len(d.sources.Instruments) != 1 || d.instrument != ids.instrument || d.day != "2026-11-22" || d.time != "01:00" {
		t.Fatalf("wrong eligible choices or History date and time: %+v", d)
	}
	setBankField(h, "Bought quantity", &d.quantity, "2.500000001")
	setBankField(h, "Gross before fees · USD", &d.gross, "199")
	setBankField(h, "Fee (optional) · USD", &d.fee, "1.2345")
	click("Review buy")
	label("Enter a positive quantity with up to eight decimal places.")
	setBankField(h, "Bought quantity", &d.quantity, "2.5")
	setBankField(h, "Gross before fees · USD", &d.gross, "75.12345")
	click("Review buy")
	label("Enter a positive gross amount with up to four decimal places.")
	setBankField(h, "Gross before fees · USD", &d.gross, "199")
	click("Review buy")
	label("The account balance at the effective time cannot cover this amount.")
	setBankField(h, "Gross before fees · USD", &d.gross, "75.1234")
	setBankField(h, "Fee (optional) · USD", &d.fee, "1.23450")
	click("Review buy")
	label("Leave the fee blank or enter a non-negative amount with up to four decimal places.")
	setBankField(h, "Fee (optional) · USD", &d.fee, "1.2345")
	click("Review buy")
	review := d.review.Preview
	label("Execution price per unit · 30.04936 USD")
	label("Cost basis per unit including fee · 30.54316 USD")
	label("Settlement cash change · -76.3579 USD")
	label("Resulting balance · 123.6421 USD")
	noBuy := func() {
		t.Helper()
		rows, err := app.ListActivities(ctx, 50)
		if err != nil || len(rows) != 0 {
			t.Fatalf("preview or rejection created history: %+v / %v", rows, err)
		}
		holdings, err := app.HoldingsByAccounts(ctx, []domain.AccountID{ids.account})
		if err != nil || len(holdings[ids.account]) != 0 {
			t.Fatalf("preview or rejection created a Holding: %+v / %v", holdings, err)
		}
	}
	noBuy()
	if _, err := app.AppendManualInstrumentQuote(ctx, ids.foreign, "7", accountTestClock.Add(-time.Minute).Format(time.RFC3339), false); err != nil {
		t.Fatal(err)
	}
	click("Confirm buy")
	label("The ledger changed. Review the buy again before confirming.")
	noBuy()
	click("Review buy")
	click("Back to details")
	if d.review != nil {
		t.Fatal("editing retained the old review")
	}
	setBankField(h, "Gross before fees · USD", &d.gross, "75")
	click("Review buy")
	label("Execution price per unit · 30 USD")
	click("Cancel")
	noBuy()

	click("Record first buy")
	d = accounts.trade.draft
	setBankField(h, "Bought quantity", &d.quantity, "2.5")
	setBankField(h, "Gross before fees · USD", &d.gross, "75.1234")
	setBankField(h, "Fee (optional) · USD", &d.fee, "1.2345")
	click("Review buy")
	bounds := semanticBounds(t, &h.router, "Confirm buy", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("purchase confirmation did not start")
	}
	h.clickAt(point)
	release()
	settle()
	label("Posting could not be confirmed. Retry the same submission to check its result.")
	if len(fault.attempts) != 2 || !accounts.tradeUnresolved() {
		t.Fatal("a duplicate click started another write or the uncertain plan was discarded")
	}
	h.click("Overview", false)
	if h.shell.activeRoute() != accountsRoute || d.review == nil {
		t.Fatal("navigation discarded an uncertain submission")
	}
	click("Retry same submission")
	if len(fault.attempts) != 3 || !reflect.DeepEqual(fault.attempts[1], fault.attempts[2]) ||
		fault.firstActivity == "" || ledger.reads.posted.ID != fault.firstActivity {
		t.Fatalf("retry did not replay the frozen buy: %+v", fault.attempts)
	}
	label("Buy recorded.")
	label("Could not load this activity.")
	click("Retry")
	label("Unit price · 30.04936 USD")
	if len(fault.attempts) != 3 {
		t.Fatal("post-commit read retry submitted a buy")
	}
	click("Open instrument · QA-BUY")
	label("Cost basis · USD $ 76.36")
	label("Native market value · USD $ 100.00")
	label("Current price · USD 40 · manual · 2026-11-21 16:00")
	click("Open account · Harbor Brokerage")
	if accounts.target.account != ids.account || accounts.canFirstBuy() {
		t.Fatal("the refreshed Holding did not close the first-buy entry")
	}
	click("Overview")
	label("$ 223.64")
	click("Ledger activity")
	label("Loaded records: 1")

	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeTrade, Side: "buy", SettlementAccountID: refIDs.account.String(), InstrumentID: refIDs.instrument.String(),
		Quantity: "2.5", Gross: "75.1234", GrossCurrency: "USD", Fee: "1.2345", FeeCurrency: "USD",
		EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: "01:00", MutationID: domain.NewMutationID().String(),
	}
	wp, err := adapter.PreviewChange(ctx, request)
	if err != nil || len(wp.Review) != len(review.Review) || refValue(wp.DerivedUnitPrice) != "30.04936" {
		t.Fatalf("Wails preview: %+v / %v", wp, err)
	}
	for i, endpoint := range review.Review {
		wire := wp.Review[i]
		if wire.Before != endpoint.Before || wire.After != endpoint.After || wire.Delta != endpoint.Delta || wire.Target != string(endpoint.Target) ||
			wire.Currency != string(endpoint.Currency) {
			t.Fatalf("preview endpoints differ: %+v / %+v", endpoint, wire)
		}
	}
	wr, err := adapter.RecordChange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := adapter.RecordChange(ctx, request)
	if err != nil || again.Activity.ID != wr.Activity.ID {
		t.Fatalf("Wails did not replay its keyed activity: %+v / %v", again, err)
	}
	request.Gross = "25"
	if _, err := adapter.RecordChange(ctx, request); err == nil {
		t.Fatal("Wails accepted a different payload for the same mutation")
	}
	assertFacts := func(service *application.Service, fixture firstBuyFixture, activityID domain.ActivityID) {
		t.Helper()
		activities, err := service.ListActivities(ctx, 50)
		if err != nil || len(activities) != 1 || activities[0].ID != activityID ||
			activities[0].Kind != domain.ActivityBuy || activities[0].EffectiveLocalDate != "2026-11-22" {
			t.Fatalf("expected exactly one buy on the original civil date: %+v / %v", activities, err)
		}
		buy := activities[0]
		detail := buy.TradeDetail
		if detail == nil || detail.InstrumentID != fixture.instrument || detail.Side != domain.TradeBuy || detail.Quantity.Canonical() != "2.5" ||
			detail.UnitPrice.Canonical() != "30.04936" || detail.Gross.CanonicalAmount() != "75.1234" ||
			detail.Fee == nil || detail.Fee.CanonicalAmount() != "1.2345" || len(buy.Effects) != 3 ||
			buy.Effects[0].Classification != domain.ClassificationTradePrincipal || buy.Effects[1].Classification != domain.ClassificationTradePrincipal ||
			buy.Effects[2].Classification != domain.ClassificationFee || buy.Effects[1].Money == nil ||
			buy.Effects[1].Money.CanonicalAmount() != "75.1234" || buy.Effects[2].Money == nil ||
			buy.Effects[2].Money.CanonicalAmount() != "1.2345" {
			t.Fatalf("unexpected trade fact or classifications: %+v / %+v", detail, buy.Effects)
		}
		cash, err := service.ListAccountCashValues(ctx, fixture.account)
		amounts := make(map[domain.ActivityEffectID]string)
		for _, row := range cash {
			if row.ActivityEffectID != nil {
				amounts[*row.ActivityEffectID] = row.Amount.CanonicalAmount()
			}
		}
		if err != nil || len(cash) != 3 || amounts[buy.Effects[1].ID] != "123.6421" || amounts[buy.Effects[2].ID] != "123.6421" {
			t.Fatalf("wrong cash projections: %v / %v", amounts, err)
		}
		positions, err := analytics.NewService(service).AccountHoldingDetails(ctx, fixture.account.String())
		if err != nil || len(positions) != 1 || positions[0].Amounts.Quantity != "2.5" || positions[0].Amounts.TotalCost == nil ||
			positions[0].Amounts.TotalCost.Amount != "76.3579" || positions[0].Amounts.CurrentValue == nil || positions[0].Amounts.CurrentValue.Amount != "100" {
			t.Fatalf("wrong cost or current market value: %+v / %v", positions, err)
		}
		overview, err := portfolio.NewService(service).Overview(ctx, account.AccountFilterRequest{})
		if err != nil || !overview.Complete || overview.NetWorth != "223.6421" {
			t.Fatalf("wrong net worth: %+v / %v", overview, err)
		}
	}
	assertFacts(app, ids, fault.firstActivity)
	assertFacts(ref, refIDs, domain.ActivityID(wr.Activity.ID))
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reopened.SetClock(clock)
	assertFacts(reopened, ids, fault.firstActivity)
	frozen := fault.attempts[2]
	r, err := reopened.RecordChangeGuarded(ctx, frozen.command, frozen.mutation, frozen.hash, frozen.token)
	if err != nil || r.Activity.ID != fault.firstActivity {
		t.Fatalf("post-restart replay did not return the original receipt: %+v / %v", r, err)
	}
	assertFacts(reopened, ids, fault.firstActivity)
	sources, err := history.LoadTradeSources(ctx, reopened, ids.account, domain.TradeBuy)
	if err != nil || len(sources.Instruments) != 1 || sources.Holdings[ids.instrument].Quantity.Canonical() != "2.5" {
		t.Fatalf("recorded first buy did not become an eligible existing position: %+v / %v", sources, err)
	}
}

func TestFirstBuyRejectsForeignSettlementAndSpentCash(t *testing.T) {
	ctx := t.Context()
	service, _, _, ids := openFirstBuyFixture(t)
	input := history.TradeInput{Side: domain.TradeBuy, Account: ids.account, Instrument: ids.foreign, Quantity: "1", Gross: "5",
		Date: "2026-11-22", Time: "01:00"}
	_, err := history.ReviewTrade(ctx, service, input)
	var issue *domain.Error
	if !errors.As(err, &issue) || issue.Field != "instrumentId" {
		t.Fatalf("wrong rejection for foreign-currency instrument: %v", err)
	}
	if _, err := service.AppendAccountCashValue(ctx, ids.account, "0", "USD", ""); err != nil {
		t.Fatal(err)
	}
	_, err = history.LoadTradeSources(ctx, service, ids.account, domain.TradeBuy)
	if !errors.As(err, &issue) || issue.Code != domain.ErrInsufficientBalance || issue.Field != "settlementCash" {
		t.Fatalf("historical cash made an unfunded account eligible: %v", err)
	}
	rows, err := service.ListActivities(ctx, 50)
	if err != nil || len(rows) != 1 || rows[0].Kind == domain.ActivityBuy {
		t.Fatalf("rejected buy created an activity: %+v / %v", rows, err)
	}
}
