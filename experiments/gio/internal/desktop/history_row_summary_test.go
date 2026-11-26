package desktop

import (
	"image"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestHistoryActivitySummariesAndLocalDateShortcuts(t *testing.T) {
	ctx := t.Context()
	service, _ := openBankFixture(t, createFixtureRoot(t))
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return accountTestClock }
	service.SetClock(func() time.Time { return accountTestClock.AddDate(0, 0, -4) })
	create := func(name, kind, mode, currency, amount string) domain.AccountID {
		t.Helper()
		record, err := service.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: kind, TrackingMode: mode, BalanceSheetRole: "asset", DefaultCurrency: currency,
			InitialAmount: amount, OwnerIDs: []domain.MemberID{boot.Members[0].ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		return record.Account.ID
	}
	cash := create("AUD Cash", "cash_on_hand", "balance", "AUD", "100")
	broker := create("Sample Brokerage", "brokerage", "holdings", "USD", "")
	instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic Apple Position", Symbol: "AAPL", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}

	service.SetClock(clock)
	adapter := historyapi.NewService(service)
	funding, err := adapter.RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyAdded, AccountID: broker.String(), Amount: "100", Currency: "USD", Reason: "contribution",
		EffectiveLocalDate: "2026-11-21", EffectiveLocalTime: "06:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	buy, err := adapter.RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeTrade, SettlementAccountID: broker.String(), InstrumentID: instrument.ID.String(),
		Side: "buy", Quantity: "2", Gross: "20", GrossCurrency: "USD",
		EffectiveLocalDate: funding.Activity.EffectiveLocalDate, EffectiveLocalTime: "07:00",
	})
	if err != nil || buy.Activity.TradeDetail == nil {
		t.Fatalf("could not seed purchase: %+v, %v", buy, err)
	}
	fee, err := adapter.RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyRemoved, AccountID: cash.String(), Amount: "15", Currency: "AUD", Reason: "fee",
		EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: "00:45",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, dividend := range []struct{ amount, at string }{{"12.50", "00:30"}, {"12.3456", "00:10"}} {
		if _, err := adapter.RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeCashDividend, HoldingID: buy.Activity.TradeDetail.HoldingID, Amount: dividend.amount, Currency: "USD",
			EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: dividend.at,
		}); err != nil {
			t.Fatal(err)
		}
	}

	page := newLedgerHistory(ctx, service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: page})
	h.size = image.Pt(1000, 830)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() {
		page.reads.workers.Wait()
		page.cashMovements.workers.Wait()
		page.model.Wait()
	}
	t.Cleanup(wait)
	settle := func() { h.frame(); wait(); h.frame(); wait(); h.frame() }
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }

	h.frame()
	click("Ledger activity")
	if all := findPageLabel(&h.router, "All history"); all == nil || !all.Desc.Selected {
		t.Fatal("the unfiltered Ledger should select All history")
	}
	label("Money removed · A$15.00 · AUD Cash · Fee")
	label("Cash dividend · US$12.50 · AAPL · Sample Brokerage")
	label("Cash dividend · US$12.35 (12.3456 USD) · AAPL · Sample Brokerage")

	pickAccountReference(h, "Account filter", "AUD Cash", "AUD Cash")
	settle()
	assertRange := func(from, to string) {
		t.Helper()
		want := history.Filters{Account: cash, From: from, To: to}
		if got := page.model.Filters(); !got.Equal(want) {
			t.Fatalf("Ledger filter = %+v, want %+v", got, want)
		}
		id := cash.String()
		reference, err := adapter.ListActivityPage(ctx, historyapi.ActivityQueryRequest{
			AccountID: &id, FromLocalDate: from, ToLocalDate: to, Limit: 50,
		})
		rows := page.model.Rows()
		if err != nil || len(rows) != len(reference.Activities) {
			t.Fatalf("Ledger and Wails report different counts: %d and %d, %v", len(rows), len(reference.Activities), err)
		}
		for i, row := range rows {
			if row.ID.String() != reference.Activities[i].ID {
				t.Fatalf("Ledger row %d = %s, want %s", i, row.ID, reference.Activities[i].ID)
			}
		}
	}
	click("Last week")
	assertRange("2026-11-16", "2026-11-22")
	click("Previous date range")
	assertRange("2026-11-09", "2026-11-15")
	click("Next date range")
	assertRange("2026-11-16", "2026-11-22")
	click("1D")
	assertRange("2026-11-22", "2026-11-22")
	label("Money removed · A$15.00 · AUD Cash · Fee")
	click("View")
	if page.reads.detail == nil || page.reads.detail.id.String() != fee.Activity.ID {
		t.Fatal("View did not open the displayed fee")
	}
	click("Back to ledger activity")
	assertRange("2026-11-22", "2026-11-22")
	click("All history")
	assertRange("2026-11-01", "2026-11-22")
}
