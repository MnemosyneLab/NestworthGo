package desktop

import (
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestLedgerQuantityAdjustmentsPreserveExactCostAndMatchWails(t *testing.T) {
	ctx := t.Context()
	app, session, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	now := accountTestClock
	clock := func() time.Time { return now }
	app.SetClock(clock)
	ref.SetClock(clock)
	adapter := historyapi.NewService(ref)
	ledger := newLedgerHistory(ctx, app, clock, func() {})
	f := &accountInvestment{service: app}
	ledger.investment = f
	h := newShellHarnessWithPages(t, ledgerPages{history: ledger, version: app.MobileDataVersion})
	h.size = image.Pt(640, 810)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		for range 4 {
			h.frame()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	click := func(label string) { t.Helper(); h.click(label, false); finish() }
	label := func(label string) { t.Helper(); assertPageLabel(t, &h.router, label) }
	finish()
	click("Ledger activity")
	open := func() *accountInvestmentDraft {
		t.Helper()
		click("Adjust position")
		d := f.draft
		h.focused(&d.adjustment.holdingPick.Trigger)
		h.click("Holding", true)
		finish()
		for _, item := range d.adjustment.sources.Holdings {
			if item.Holding.ID == ids.holding {
				click(item.Instrument.SelectionLabel() + " · Harbor Brokerage · " + item.Holding.Quantity.Canonical())
				return d
			}
		}
		t.Fatal("original holding was unavailable")
		return nil
	}
	d := open()
	setBankField(h, "Quantity to adjust", &d.quantity, "0.625")
	costLabel := "Unit cost for added units · USD"
	version := app.MobileDataVersion()
	click("Review position adjustment")
	label("Enter the original cost per added unit.")
	if _, err := adapter.PreviewChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionAdjustment, HoldingID: refIDs.holding.String(), Added: true,
		Quantity: "0.625", EffectiveAt: now.Format(time.RFC3339Nano),
	}); err == nil {
		t.Fatal("the shared Wails preview unexpectedly accepted a blank cost")
	}
	setBankField(h, costLabel, &d.unitCost, "12.123456789")
	click("Review position adjustment")
	label("Enter a non-negative original cost per unit with up to eight decimal places.")
	if d.unitCost.Text() != "12.123456789" || app.MobileDataVersion() != version {
		t.Fatal("invalid cost lost the draft or changed the ledger")
	}
	setBankField(h, costLabel, &d.unitCost, "12.12345678")
	click("Review position adjustment")
	label("Original cost per unit · 12.12345678 USD")
	label("Quantity added · 0.625")
	label("Resulting quantity · 2.625")
	h.focused(&d.confirm)
	h.press(key.NameReturn)
	finish()
	label("Unit cost · 12.12345678 USD")
	label("Resulting quantity · 2.625")
	addID := ledger.reads.detail.id
	req := historyapi.ChangeCommandRequest{Kind: historyapi.ChangePositionAdjustment,
		HoldingID: refIDs.holding.String(), Added: true, Quantity: "0.625", UnitCost: "12.12345678",
		EffectiveAt: now.Format(time.RFC3339Nano)}
	assertLiabilityPosting(t, app, addID, adapter, req)
	click("Back to ledger activity")
	h.focused(&f.adjustmentOpen)
	label("Position adjustment · Quantity added · 0.625 · QA-BUY · Harbor Brokerage")

	now = now.Add(time.Minute)
	d = open()
	setBankField(h, costLabel, &d.unitCost, "5")
	click("Remove units")
	if d.unitCost.Text() != "" || findPageLabel(&h.router, costLabel) != nil {
		t.Fatal("removal retained an acquisition cost")
	}
	setBankField(h, "Quantity to adjust", &d.quantity, "3")
	click("Review position adjustment")
	label("This holding did not have enough quantity at the effective time.")
	setBankField(h, "Quantity to adjust", &d.quantity, "0.125")
	click("Review position adjustment")
	label("Quantity removed · 0.125")
	label("Resulting quantity · 2.5")
	click("Confirm position adjustment")
	removeID := ledger.reads.detail.id
	req.Added, req.UnitCost, req.Quantity = false, "", "0.125"
	req.EffectiveAt = now.Format(time.RFC3339Nano)
	assertLiabilityPosting(t, app, removeID, adapter, req)
	click("Back to ledger activity")
	h.focused(&f.adjustmentOpen)
	label("Position adjustment · Quantity removed · 0.125 · QA-BUY · Harbor Brokerage")
	d = open()
	click("Cancel")
	h.focused(&f.adjustmentOpen)

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	for _, item := range []struct {
		id           domain.ActivityID
		direction    domain.EffectDirection
		units, price string
	}{{addID, domain.EffectAdded, "0.625", "12.12345678"}, {removeID, domain.EffectRemoved, "0.125", ""}} {
		record, err := reopened.Activity(ctx, item.id)
		if err != nil || len(record.Effects) != 1 {
			t.Fatalf("unexpected recorded adjustment: %+v / %v", record, err)
		}
		effect := record.Effects[0]
		price := ""
		if effect.CostUnitPrice != nil {
			price = effect.CostUnitPrice.Canonical()
		}
		if effect.HoldingID == nil || *effect.HoldingID != ids.holding || effect.Direction != item.direction ||
			effect.Quantity.Canonical() != item.units || price != item.price || effect.Money != nil {
			t.Fatalf("unexpected adjustment effect: %+v", effect)
		}
	}
	read := func(service *application.Service, id domain.InstrumentID) domain.HoldingAmounts {
		t.Helper()
		rows, err := service.InstrumentHoldings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.InstrumentID == id {
				return row.Amounts
			}
		}
		t.Fatal("missing adjusted instrument")
		return domain.HoldingAmounts{}
	}
	got, want := read(reopened, ids.instrument), read(ref, refIDs.instrument)
	if got.Quantity != "2.5" || got.TotalCost == nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("result differs from the Wails projection: %+v / %+v", got, want)
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
}
