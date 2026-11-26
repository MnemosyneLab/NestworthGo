package desktop

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestAccountForeignDividendCreditsOneExactCurrencyAndMatchesWails(t *testing.T) {
	ctx := t.Context()
	clock := func() time.Time { return accountTestClock }
	seed := func() (*application.Service, firstBuyFixture, domain.HoldingID) {
		t.Helper()
		app, _, _, ids := openFirstBuyFixture(t)
		api := historyapi.NewService(app)
		if _, err := api.RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyAdded, AccountID: ids.account.String(), Amount: "7.0101",
			Currency: "EUR", Reason: "contribution", EffectiveAt: clock().Add(-2 * time.Hour).Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatal(err)
		}
		buy, err := api.RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeTrade, SettlementAccountID: ids.account.String(), InstrumentID: ids.foreign.String(),
			Side: "buy", Quantity: "1.25", Gross: "5.0001", GrossCurrency: "EUR",
			EffectiveAt: clock().Add(-time.Hour).Format(time.RFC3339Nano),
		})
		if err != nil || buy.Activity.TradeDetail == nil {
			t.Fatalf("could not seed EUR holding: %+v / %v", buy, err)
		}
		return app, ids, domain.HoldingID(buy.Activity.TradeDetail.HoldingID)
	}
	app, ids, holding := seed()
	reference, refIDs, refHolding := seed()
	adapter := historyapi.NewService(reference)
	cashBefore := map[domain.CurrencyCode]string{"USD": "200", "EUR": "2.01"}
	cashAfter := map[domain.CurrencyCode]string{"USD": "200", "EUR": "14.3556"}

	accounts := newAccountsPage(ctx, app, clock, func() {})
	accounts.investment = &accountInvestment{service: app}
	ledger := newLedgerHistory(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, history: ledger})
	h.size = image.Pt(1100, 1200)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() {
		accounts.workers.Wait()
		ledger.model.Wait()
		ledger.reads.workers.Wait()
	}
	t.Cleanup(wait)
	settle := func() {
		for range 4 {
			h.frame()
			wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }

	h.frame()
	click("Accounts")
	click("Harbor Brokerage")
	click("Record cash dividend")
	draft := accounts.investment.draft
	if draft == nil || draft.account.ID != ids.account || draft.instrument != ids.foreign || draft.dividendCurrency != "EUR" {
		t.Fatalf("account dividend did not offer the sole EUR holding and its native currency: %+v", draft)
	}
	click("Cancel")
	h.focused(&accounts.investment.dividend)

	click("Actions for QA-EUR")
	button := accounts.investment.dividendHoldings[holding]
	if button == nil {
		t.Fatal("EUR holding has no scoped dividend action")
	}
	h.router.Source().Execute(key.FocusCmd{Tag: button})
	h.press(key.NameReturn)
	settle()
	draft = accounts.investment.draft
	if draft == nil || draft.sources == nil || draft.lockedHolding.ID != holding || draft.account.ID != ids.account ||
		draft.instrument != ids.foreign || len(draft.sources.Instruments) != 1 || draft.dividendCurrency != "USD" {
		t.Fatalf("scoped dividend lost the holding or account default payment currency: %+v", draft)
	}
	setBankField(h, "Cash received · USD", &draft.gross, "12.3456")
	pickAccountReference(h, "Cash currency", "EUR", "EUR")
	settle()
	if draft.dividendCurrency != "EUR" || draft.gross.Text() != "12.3456" {
		t.Fatal("changing the payment currency changed the entered amount")
	}
	click("Review cash dividend")
	label("Balance before · 2.01 EUR")
	label("Net cash credited · 12.3456 EUR")
	label("Resulting balance · 14.3556 EUR")
	if draft.review == nil {
		t.Fatal("no guarded dividend review")
	}
	assertNativeTradeCash(t, app, ids.account, cashBefore)
	assertRepeatActivityCount(t, app, 2)

	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeCashDividend, HoldingID: refHolding.String(), Currency: "EUR", Amount: "12.3456",
		EffectiveLocalDate: draft.day, EffectiveLocalTime: draft.time, MutationID: domain.NewMutationID().String(),
	}
	want, err := adapter.PreviewChange(ctx, request)
	got := draft.review.Preview.Review
	if err != nil || len(want.Review) != 1 || len(got) != 1 {
		t.Fatalf("Gio and Wails did not preview one cash endpoint: %+v / %+v / %v", got, want, err)
	}
	if a, b := got[0], want.Review[0]; a.Currency.String() != b.Currency || a.Before != b.Before || a.Delta != b.Delta ||
		a.After != b.After || string(a.Target) != b.Target || !draft.review.Preview.Activity.EffectiveAt.Equal(accountTestClock) {
		t.Fatalf("cash credit differs from Wails: %+v / %+v", a, b)
	}
	click("Confirm cash dividend")
	label("Cash dividend recorded.")
	label("Harbor Brokerage · Added 12.3456 EUR")
	if ledger.reads.detail == nil || ledger.reads.detail.record == nil {
		t.Fatal("confirmation did not open the activity receipt")
	}
	refPosted, err := adapter.RecordChange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		app        *application.Service
		ids        firstBuyFixture
		holding    domain.HoldingID
		activityID domain.ActivityID
	}{
		{app, ids, holding, ledger.reads.detail.id},
		{reference, refIDs, refHolding, domain.ActivityID(refPosted.Activity.ID)},
	} {
		assertRepeatActivityCount(t, item.app, 3)
		assertNativeTradeCash(t, item.app, item.ids.account, cashAfter)
		record, err := item.app.Activity(ctx, item.activityID)
		if err != nil || record.Kind != domain.ActivityCashDividend || len(record.Effects) != 1 || len(record.Resulting) != 1 {
			t.Fatalf("dividend was not recorded as one activity and endpoint: %+v / %v", record, err)
		}
		detail, effect, resulting := record.DividendDetail, record.Effects[0], record.Resulting[0]
		if detail == nil || detail.HoldingID != item.holding || detail.InstrumentID != item.ids.foreign ||
			detail.Amount.Currency() != "EUR" || detail.Amount.CanonicalAmount() != "12.3456" ||
			effect.AccountID == nil || *effect.AccountID != item.ids.account || effect.Classification != domain.ClassificationIncome ||
			effect.Direction != domain.EffectAdded || resulting.Currency != "EUR" || resulting.Amount != "14.3556" {
			t.Fatalf("unexpected native dividend receipt: %+v", record)
		}
		gain, err := item.app.HoldingGain(ctx, item.holding)
		if err != nil || gain.Quantity != "1.25" || gain.TotalCost == nil || gain.TotalCost.Currency != "EUR" || gain.TotalCost.Amount != "5.0001" {
			t.Fatalf("dividend changed the exact holding quantity or cost: %+v / %v", gain, err)
		}
	}
	click("Back to account")
	h.focused(button)
}
