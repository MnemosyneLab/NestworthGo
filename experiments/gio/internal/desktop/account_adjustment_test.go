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

func TestAccountAndHoldingAdjustmentPreserveScopeAndFocus(t *testing.T) {
	ctx := t.Context()
	app, session, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	now := accountTestClock
	clock := func() time.Time { return now }
	app.SetClock(clock)
	ref.SetClock(clock)
	boot, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	other, err := app.CreateAccount(ctx, application.AccountInput{
		Name: "Other investment", AccountType: "brokerage", TrackingMode: "holdings", BalanceSheetRole: "asset",
		DefaultCurrency: "EUR", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInPortfolio: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = historyapi.NewService(app).RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionImport, AccountID: other.Account.ID.String(), InstrumentID: ids.foreign.String(),
		Currency: "EUR", Quantity: "7", UnitCost: "0", EffectiveAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	accounts := newAccountsPage(ctx, app, clock, func() {})
	f := &accountInvestment{service: app}
	accounts.investment = f
	ledger := newLedgerHistory(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, history: ledger, version: app.MobileDataVersion})
	h.size = image.Pt(640, 920)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		for range 4 {
			h.frame()
			accounts.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	click := func(label string) { t.Helper(); h.click(label, false); finish() }
	finish()
	click("Accounts")
	click("Harbor Brokerage")
	scrollToAccountDetailLabel(h, "Adjust position")
	click("Adjust position")
	d := f.draft
	h.focused(&d.adjustment.holdingPick.Trigger)
	if len(d.adjustment.sources.Holdings) != 1 || d.adjustment.sources.Holdings[0].Holding.ID != ids.holding {
		t.Fatal("account selector crossed account scope")
	}
	item := d.adjustment.sources.Holdings[0]
	h.click("Holding", true)
	finish()
	click(item.Instrument.SelectionLabel() + " · Harbor Brokerage · 2")
	before := app.MobileDataVersion()
	click("Cancel")
	h.focused(&f.adjustmentOpen)
	if app.MobileDataVersion() != before {
		t.Fatal("cancel changed the ledger")
	}

	rowLabel := "Adjust · " + item.Instrument.DisplayLabel()
	scrollToAccountDetailLabel(h, rowLabel)
	click(rowLabel)
	d = f.draft
	h.focused(&d.quantity.Editor)
	if d.adjustment.holding != ids.holding || findPageLabel(&h.router, "Holding") != nil {
		t.Fatal("holding action did not lock the original Holding")
	}
	setBankField(h, "Quantity to adjust", &d.quantity, "0.125")
	setBankField(h, "Unit cost for added units · USD", &d.unitCost, "18.12345678")
	click("Review position adjustment")
	h.focused(&d.confirm)
	h.press(key.NameReturn)
	finish()
	addID := ledger.reads.detail.id
	adapter := historyapi.NewService(ref)
	req := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionAdjustment, HoldingID: refIDs.holding.String(), Added: true,
		Quantity: "0.125", UnitCost: "18.12345678", EffectiveAt: now.Format(time.RFC3339Nano),
	}
	assertLiabilityPosting(t, app, addID, adapter, req)
	click("Back to account")
	button := f.adjustmentHoldings[ids.holding]
	h.focused(button)

	now = now.Add(time.Minute)
	h.press(key.NameReturn)
	finish()
	d = f.draft
	click("Remove units")
	setBankField(h, "Quantity to adjust", &d.quantity, "0.25")
	click("Review position adjustment")
	click("Confirm position adjustment")
	removeID := ledger.reads.detail.id
	req.Added, req.Quantity, req.UnitCost = false, "0.25", ""
	req.EffectiveAt = now.Format(time.RFC3339Nano)
	assertLiabilityPosting(t, app, removeID, adapter, req)
	click("Back to account")
	h.focused(button)

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	row, err := reopened.Activity(ctx, addID)
	if err != nil || len(row.Effects) != 1 || *row.Effects[0].HoldingID != ids.holding ||
		row.Effects[0].CostUnitPrice == nil || row.Effects[0].CostUnitPrice.Canonical() != "18.12345678" {
		t.Fatalf("unexpected recorded addition: %+v / %v", row, err)
	}
	positions, err := reopened.HoldingsByAccounts(ctx, []domain.AccountID{ids.account, other.Account.ID})
	if err != nil || len(positions[ids.account]) != 1 || positions[ids.account][0].ID != ids.holding ||
		positions[ids.account][0].Quantity.Canonical() != "1.875" || len(positions[other.Account.ID]) != 1 ||
		positions[other.Account.ID][0].Quantity.Canonical() != "7" {
		t.Fatalf("unexpected scoped positions: %+v / %v", positions, err)
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
}
