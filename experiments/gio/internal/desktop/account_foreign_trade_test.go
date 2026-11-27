package desktop

import (
	"image"
	"reflect"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestForeignTradesUseExplicitSettlementAndMatchWails(t *testing.T) {
	ctx := t.Context()
	app, session, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	adapter := historyapi.NewService(ref)
	now := accountTestClock
	clock := func() time.Time { return now }
	app.SetClock(clock)
	ref.SetClock(clock)
	fault := &cashMovementFault{Service: app, loseAttempt: 2, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	t.Cleanup(release)
	p := newAccountsPage(ctx, fault, clock, func() {})
	p.investment = &accountInvestment{service: fault}
	timeline := newLedgerHistory(ctx, fault, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p, history: timeline})
	h.size = image.Pt(640, 775)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		for range 4 {
			h.frame()
			p.workers.Wait()
			timeline.model.Wait()
			timeline.reads.workers.Wait()
		}
	}
	click := func(label string) { h.click(label, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	setBuy := func(d *accountInvestmentDraft) {
		setBankField(h, "Bought quantity", &d.quantity, "3")
		setBankField(h, "Gross before fees · EUR", &d.gross, "60.0001")
		setBankField(h, "Fee (optional) · EUR", &d.fee, "0.0002")
		setBankField(h, "Notes", &d.note, "  Foreign execution sample  ")
	}
	h.frame()
	click("Accounts")
	click("Harbor Brokerage")
	scrollToAccountDetailLabel(h, "Record buy")
	click("Record buy")
	d := p.investment.draft
	h.focused(&d.selectInstrument.Trigger)
	chooseTradeInstrument(t, h, d, ids.foreign, finish)
	label("This trade settles in EUR. Record any cash conversion separately in Ledger activity.")
	setBuy(d)
	click("Review buy")
	label("The account balance at the effective time cannot cover this amount.")
	click("Cancel")
	h.focused(&p.investment.buy)
	assertNativeTradeCash(t, app, ids.account, map[domain.CurrencyCode]string{"USD": "156"})

	fx, err := history.ReviewCashMovement(ctx, app, history.CashMovementInput{
		Movement: history.CashConversion, Account: ids.account, Currency: "USD", ToCurrency: "EUR",
		Amount: "120.25", Received: "100.125", Fee: "0.1234", Date: "2026-11-21", Time: "23:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := fx.Commit(ctx, app)
	if err != nil {
		t.Fatal(err)
	}
	fxRequest := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeFXConversion, AccountID: refIDs.account.String(), Sold: "120.25", SoldCurrency: "USD",
		Bought: "100.125", BoughtCurrency: "EUR", Fee: "0.1234", FeeCurrency: "USD",
		EffectiveLocalDate: "2026-11-21", EffectiveLocalTime: "23:00",
	}
	compareLiabilityReview(t, fx.Preview, adapter, fxRequest)
	assertLiabilityPosting(t, app, exchange.Activity.ID, adapter, fxRequest)
	scrollToAccountDetailLabel(h, "Record buy")
	click("Record buy")
	d = p.investment.draft
	chooseTradeInstrument(t, h, d, ids.foreign, finish)
	setBuy(d)
	click("Review buy")
	label("Execution price per unit · 20.00003333 EUR")
	label("Settlement cash change · -60.0003 EUR")
	label("Resulting balance · 40.1247 EUR")
	h.focused(&d.confirm)

	rec := historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyAdded, AccountID: ids.account.String(),
		Amount: "2", Currency: "EUR", Reason: "contribution", EffectiveAt: now.Add(-time.Minute).Format(time.RFC3339Nano)}
	if _, err := historyapi.NewService(app).RecordChange(ctx, rec); err != nil {
		t.Fatal(err)
	}
	rec.AccountID = refIDs.account.String()
	if _, err := adapter.RecordChange(ctx, rec); err != nil {
		t.Fatal(err)
	}
	click("Confirm buy")
	label("The ledger changed. Review the buy again before confirming.")
	if d.instrument != ids.foreign || d.currency != "EUR" || d.gross.Text() != "60.0001" || d.note.Text() != "  Foreign execution sample  " {
		t.Fatal("stale buy lost the explicit currency or edited input")
	}
	click("Review buy")
	label("Resulting balance · 42.1247 EUR")
	note := "Foreign execution sample"
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeTrade, Side: "buy", SettlementAccountID: refIDs.account.String(), InstrumentID: refIDs.foreign.String(),
		Gross: "60.0001", GrossCurrency: "EUR", Fee: "0.0002", FeeCurrency: "EUR", Quantity: "3", Note: &note,
		EffectiveAt: now.UTC().Format(time.RFC3339Nano),
	}
	wrong := request
	wrong.FeeCurrency = "USD"
	if _, err := adapter.PreviewChange(ctx, wrong); err == nil {
		t.Fatal("Wails accepted a fee in a different currency")
	}
	compareLiabilityReview(t, d.review.Preview, adapter, request)
	plan := *d.review
	box := semanticBounds(t, &h.router, "Confirm buy", false)
	h.press(key.NameReturn)
	select {
	case <-fault.started:
	case <-time.After(5 * time.Second):
		t.Fatal("confirmation did not begin")
	}
	h.clickAt(box.Min.Add(box.Max).Div(2))
	h.click("Back", false)
	if h.shell.activeRoute() != accountsRoute || !p.investment.unresolved() {
		t.Fatal("pending trade allowed dismissal")
	}
	release()
	finish()
	h.focused(&d.retryConfirm)
	click("Retry same submission")
	if timeline.reads.detail.err != nil {
		click("Retry")
	}
	buyID := timeline.reads.detail.id
	gotBuy, wantBuy := compareForeignTrade(t, app, buyID, adapter, request)
	if len(fault.attempts) != 3 || fault.attempts[1].mutation != fault.attempts[2].mutation || fault.attempts[1].hash != fault.attempts[2].hash {
		t.Fatal("ambiguous confirmation lost the original mutation")
	}
	click("Back to account")
	h.focused(&p.investment.buy)

	now = now.Add(time.Minute)
	scrollToAccountDetailLabel(h, "Record sell")
	click("Record sell")
	d = p.investment.draft
	chooseTradeInstrument(t, h, d, ids.foreign, finish)
	setBankField(h, "Sold quantity", &d.quantity, "1.25")
	setBankField(h, "Gross proceeds before fees · EUR", &d.gross, "30.1251")
	setBankField(h, "Fee (optional) · EUR", &d.fee, "0.0001")
	click("Review sell")
	label("Settlement cash change · +30.125 EUR")
	label("Resulting balance · 72.2497 EUR")
	request.Side, request.HoldingID, request.Quantity = "sell", wantBuy.HoldingID, "1.25"
	request.Gross, request.Fee, request.Note = "30.1251", "0.0001", nil
	request.EffectiveAt = now.UTC().Format(time.RFC3339Nano)
	compareLiabilityReview(t, d.review.Preview, adapter, request)
	click("Confirm sell")
	sellID := timeline.reads.detail.id
	gotSell, _ := compareForeignTrade(t, app, sellID, adapter, request)
	if gotSell.HoldingID != gotBuy.HoldingID {
		t.Fatal("foreign sale lost its original holding link")
	}
	click("Back to account")
	h.focused(&p.investment.sell)
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	reopened.SetClock(clock)
	replayed, err := plan.Commit(ctx, reopened)
	if err != nil || replayed.Activity.ID != buyID {
		t.Fatalf("receipt changed after reopen: %+v / %v", replayed.Activity, err)
	}
	balances := map[domain.CurrencyCode]string{"EUR": "72.2497", "USD": "35.6266"}
	assertNativeTradeCash(t, reopened, ids.account, balances)
	assertNativeTradeCash(t, ref, refIDs.account, balances)
	for _, service := range []*application.Service{reopened, ref} {
		activities, err := service.ListActivities(ctx, 50)
		if err != nil || len(activities) != 5 {
			t.Fatalf("unexpected activities after reopen: %d, %v", len(activities), err)
		}
	}
}

func compareForeignTrade(t *testing.T, app *application.Service, id domain.ActivityID, adapter *historyapi.Service, request historyapi.ChangeCommandRequest) (wire.TradeDetailDTO, wire.TradeDetailDTO) {
	t.Helper()
	want, err := adapter.RecordChange(t.Context(), request)
	gotActivity, readErr := app.Activity(t.Context(), id)
	if err != nil || readErr != nil {
		t.Fatalf("could not read both trade facts: %v / %v", readErr, err)
	}
	got := wire.FromActivity(gotActivity)
	a, b := *got.TradeDetail, *want.Activity.TradeDetail
	a.InstrumentID, a.HoldingID = b.InstrumentID, b.HoldingID
	if !reflect.DeepEqual(a, b) || got.EffectiveAt != want.Activity.EffectiveAt ||
		!reflect.DeepEqual(got.Note, want.Activity.Note) || len(got.Effects) != len(want.Effects) {
		t.Fatalf("trade differs from Wails: %+v / %+v", got, want.Activity)
	}
	for i, effect := range got.Effects {
		ref := want.Effects[i]
		if effect.Role != ref.Role || effect.Direction != ref.Direction || effect.Target != ref.Target ||
			effect.Classification != ref.Classification || !reflect.DeepEqual(effect.Money, ref.Money) {
			t.Fatalf("effect differs from Wails: %+v / %+v", effect, ref)
		}
	}
	return *got.TradeDetail, *want.Activity.TradeDetail
}

func assertNativeTradeCash(t *testing.T, service *application.Service, account domain.AccountID, want map[domain.CurrencyCode]string) {
	t.Helper()
	rows, err := service.ListAccountCashValues(t.Context(), account)
	if err != nil {
		t.Fatal(err)
	}
	got := map[domain.CurrencyCode]string{}
	for _, row := range rows {
		if code := row.Amount.Currency(); got[code] == "" {
			got[code] = row.Amount.CanonicalAmount()
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cash sleeves got %v, want %v", got, want)
	}
}
