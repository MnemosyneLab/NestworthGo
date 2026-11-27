package desktop

import (
	"fmt"
	"image"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestAccountsCustodyTransferKeepsItsAccountAndHoldingScope(t *testing.T) {
	ctx := t.Context()
	app, session, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	now := accountTestClock
	clock := func() time.Time { return now }
	seed := func(service *application.Service, other domain.InstrumentID) domain.AccountID {
		t.Helper()
		service.SetClock(clock)
		boot, err := service.Bootstrap(ctx)
		if err != nil {
			t.Fatal(err)
		}
		account, err := service.CreateAccount(ctx, application.AccountInput{
			Name: "Other broker", AccountType: "brokerage", TrackingMode: "holdings", BalanceSheetRole: "asset",
			DefaultCurrency: "EUR", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInPortfolio: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := historyapi.NewService(service).RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangePositionImport, AccountID: account.Account.ID.String(), InstrumentID: other.String(),
			Currency: "EUR", Quantity: "7", UnitCost: "0", EffectiveAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatal(err)
		}
		return account.Account.ID
	}
	other, refOther := seed(app, ids.foreign), seed(ref, refIDs.foreign)
	adapter := historyapi.NewService(ref)
	instruments, err := app.ListInstruments(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	instrument := instruments[slices.IndexFunc(instruments, func(i domain.Instrument) bool { return i.ID == ids.instrument })]

	accounts := newAccountsPage(ctx, app, clock, func() {})
	f := &accountInvestment{service: app}
	accounts.investment = f
	portfolio := newPortfolioPage(ctx, app, func() {})
	ledger := newLedgerHistory(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, history: ledger, portfolio: portfolio, version: app.MobileDataVersion})
	h.size = image.Pt(640, 990)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		for range 4 {
			h.frame()
			accounts.workers.Wait()
			portfolio.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	click := func(label string) { t.Helper(); h.click(label, false); finish() }
	pick := func(field, option string) { t.Helper(); h.click(field, true); finish(); click(option) }
	finish()
	click("Investment assets")
	setBankField(h, "Search holdings", &portfolio.search.Field, "QA-BUY")
	finish()
	click(instrument.DisplayLabel())
	click("Open account · Harbor Brokerage")
	click("Transfer position")
	d := f.draft
	h.focused(&d.custody.fromPicker.Trigger)
	if d.custody.scope.account.ID != ids.account {
		t.Fatal("account entry lost its scope")
	}
	for _, row := range d.custody.sources.Holdings {
		if d.custody.offersSource(row) && row.Account.ID != ids.account {
			t.Fatal("outgoing picker accepted another account")
		}
	}
	click("Transfer in")
	if d.custody.to != ids.account || d.custody.from != "" {
		t.Fatal("incoming entry did not freeze the account")
	}
	version := app.MobileDataVersion()
	click("Cancel")
	h.focused(&f.custodyOpen)
	if app.MobileDataVersion() != version {
		t.Fatal("cancel wrote to the ledger")
	}

	rowLabel := "Transfer · " + instrument.DisplayLabel()
	scrollToAccountDetailLabel(h, rowLabel)
	click(rowLabel)
	d = f.draft
	h.focused(&d.custody.toPicker.Trigger)
	if d.custody.from != ids.holding {
		t.Fatal("holding entry did not lock the source")
	}
	if findPageLabel(&h.router, "Source holding") != nil {
		t.Fatal("locked holding was editable")
	}
	pick("Destination account", "Other broker")
	setBankField(h, "Quantity to transfer", &d.quantity, "0.625")
	click("Review position transfer")
	h.focused(&d.confirm)
	h.press(key.NameReturn)
	finish()
	outID := ledger.reads.detail.id
	req := historyapi.ChangeCommandRequest{Kind: historyapi.ChangePositionTransfer, FromHoldingID: refIDs.holding.String(),
		ToAccountID: refOther.String(), Quantity: "0.625", EffectiveAt: now.Format(time.RFC3339Nano)}
	refOut, err := adapter.RecordChange(ctx, req)
	if err != nil || outID == "" || len(refOut.Effects) != 2 {
		t.Fatalf("outgoing transfer did not persist: %+v / %v", refOut, err)
	}
	out, err := app.Activity(ctx, outID)
	if err != nil || len(out.Effects) != 2 || *out.Effects[0].HoldingID != ids.holding {
		t.Fatalf("wrong source effect: %+v / %v", out, err)
	}
	click("Back to account")
	button := f.custodyHoldings[ids.holding]
	h.focused(button)

	now = now.Add(time.Minute)
	h.press(key.NameReturn)
	finish()
	d = f.draft
	click("Transfer in")
	otherHolding := *out.Effects[1].HoldingID
	from := "QA-BUY"
	for _, row := range d.custody.sources.Holdings {
		if d.custody.offersSource(row) {
			if row.Holding.ID != otherHolding {
				t.Fatal("incoming picker offered a different instrument or account")
			}
			from = fmt.Sprintf("%s · Other broker · 0.625", row.Instrument.SelectionLabel())
		}
	}
	pick("Source holding", from)
	setBankField(h, "Quantity to transfer", &d.quantity, "0.125")
	click("Review position transfer")
	assertPageLabel(t, &h.router, "To account · Harbor Brokerage")
	click("Confirm position transfer")
	inID := ledger.reads.detail.id
	refFrom := *refOut.Effects[1].HoldingID
	req.FromHoldingID, req.ToAccountID, req.ToHoldingID, req.Quantity = refFrom, "", refIDs.holding.String(), "0.125"
	req.EffectiveAt = now.Format(time.RFC3339Nano)
	assertLiabilityPosting(t, app, inID, adapter, req)
	click("Back to account")
	h.focused(button)
	click("Back")
	h.focused(portfolio.accounts[ids.account])
	click("Back")
	if portfolio.search.Field.Text() != "QA-BUY" {
		t.Fatal("the original portfolio search was lost")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	record, err := reopened.Activity(ctx, inID)
	if err != nil || len(record.Effects) != 2 || *record.Effects[0].HoldingID != otherHolding || *record.Effects[1].HoldingID != ids.holding {
		t.Fatalf("incoming transfer did not retain both exact holdings: %+v / %v", record, err)
	}
	for _, item := range []struct {
		app           *application.Service
		instrument    domain.InstrumentID
		source, other domain.AccountID
	}{{reopened, ids.instrument, ids.account, other}, {ref, refIDs.instrument, refIDs.account, refOther}} {
		groups, err := item.app.InstrumentHoldings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		rows := groups[slices.IndexFunc(groups, func(group domain.InstrumentHoldingsView) bool { return group.InstrumentID == item.instrument })].Holdings
		for _, want := range []struct {
			account     domain.AccountID
			units, cost string
		}{{item.source, "1.5", "33"}, {item.other, "0.5", "11"}} {
			row := rows[slices.IndexFunc(rows, func(row domain.InstrumentHoldingMember) bool { return row.AccountID == want.account })]
			if row.Amounts.Quantity != want.units || row.Amounts.TotalCost == nil || row.Amounts.TotalCost.Amount != want.cost {
				t.Fatalf("unexpected units or cost: %+v", row)
			}
		}
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
	assertNativeTradeCash(t, reopened, other, map[domain.CurrencyCode]string{})
}
