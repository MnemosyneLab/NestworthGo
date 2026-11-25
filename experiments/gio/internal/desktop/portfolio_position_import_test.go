package desktop

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestInvestmentAssetsImportsIntoSelectedAccountAndRestoresContext(t *testing.T) {
	ctx := t.Context()
	service, session, paths, ids := openExistingBuyFixture(t)
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, accountType, mode string) domain.AccountID {
		t.Helper()
		input := application.AccountInput{
			Name: name, AccountType: accountType, TrackingMode: mode, BalanceSheetRole: "asset",
			DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInPortfolio: true,
		}
		if mode == "balance" {
			input.InitialAmount = "0"
		}
		r, err := service.CreateAccount(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		return r.Account.ID
	}
	second := create("Second desk", "brokerage", "holdings")
	create("Synthetic wallet", "cash_on_hand", "balance")
	clock := func() time.Time { return accountTestClock }
	service.SetClock(clock)

	portfolio := newPortfolioPage(ctx, service, func() {})
	f := &accountInvestment{service: service}
	portfolio.investment, portfolio.clock = f, clock
	ledger := newLedgerHistory(ctx, service, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{portfolio: portfolio, history: ledger, version: service.MobileDataVersion})
	h.size = image.Pt(640, 810)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		for range 4 {
			h.frame()
			portfolio.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	click := func(text string) { t.Helper(); h.click(text, false); finish() }
	finish()
	click("Investment assets")
	portfolio.search.Field.Editor.SetText("QA-BUY")
	finish()
	click("Record existing holding")
	d := f.draft
	h.focused(&d.selectAccount.Trigger)
	if len(d.accounts) != 2 || d.account.ID != "" {
		t.Fatal("expected explicit selection between two eligible accounts")
	}
	pick := func(label string) { t.Helper(); h.click("Holdings account", true); finish(); click(label) }
	pick("Harbor Brokerage · USD")
	if d.instrument != ids.foreign || len(d.sources.Instruments) != 1 {
		t.Fatal("the picker included a security already held by the selected account")
	}
	setBankField(h, "Existing quantity", &d.quantity, "4")
	setBankField(h, "Original cost per unit · EUR", &d.unitCost, "7")
	pick("Second desk · USD")
	if d.account.ID != second || d.instrument != "" || d.quantity.Text() != "" || d.unitCost.Text() != "" {
		t.Fatal("switching the account reused the previous instrument draft")
	}
	version := service.MobileDataVersion()
	click("Cancel")
	h.focused(&f.importOpen)
	if portfolio.search.Field.Text() != "QA-BUY" || service.MobileDataVersion() != version {
		t.Fatal("cancel changed search or the ledger")
	}
	h.press(key.NameReturn)
	finish()
	d = f.draft
	pick("Second desk · USD")
	chooseTradeInstrument(t, h, d, ids.foreign, finish)
	setBankField(h, "Existing quantity", &d.quantity, "1.125")
	setBankField(h, "Original cost per unit · EUR", &d.unitCost, "2.12345678")
	click("Review existing position")
	h.focused(&d.confirm)
	h.press(key.NameReturn)
	finish()
	assertPageLabel(t, &h.router, "Unit cost · 2.12345678 EUR")
	id := ledger.reads.detail.id
	click("Back to Investment Assets")
	h.focused(&f.importOpen)
	if portfolio.search.Field.Text() != "QA-BUY" {
		t.Fatal("Ledger Back lost the Investment Assets search")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	rows, err := reopened.HoldingsByAccounts(ctx, []domain.AccountID{ids.account, second})
	if err != nil || len(rows[ids.account]) != 1 || len(rows[second]) != 1 ||
		rows[second][0].InstrumentID != ids.foreign || rows[second][0].Quantity.Canonical() != "1.125" {
		t.Fatalf("import did not persist in the selected account: %+v / %v", rows, err)
	}
	record, err := reopened.Activity(ctx, id)
	if err != nil || len(record.Effects) != 1 || record.Effects[0].HoldingID == nil || *record.Effects[0].HoldingID != rows[second][0].ID {
		t.Fatalf("receipt did not retain its selected Holding: %+v / %v", record, err)
	}
}
