package desktop

import (
	"image"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestLedgerRecordSheetPreservesFilterScrollAndFocus(t *testing.T) {
	app, _ := openBankFixture(t, createFixtureRoot(t))
	seedMixedTimeline(t, app)
	cash, timeline, h, finish := cashMovementHarness(t, app)
	h.size = image.Pt(1280, 800)
	filter := history.Filters{Kind: domain.ActivityCashIn}
	timeline.model.SetFilters(filter)
	finish()
	timeline.list.ScrollBy(7)
	h.frame()
	before := timeline.list.Position
	if before.First == 0 || len(timeline.model.Rows()) < 8 {
		t.Fatalf("fixture did not scroll the filtered ledger: %+v", before)
	}
	click := func(name string) {
		// The toolbar never needs revealing through the body scroll area.
		bounds := semanticBounds(t, &h.router, name, false)
		h.clickAt(bounds.Min.Add(bounds.Max).Div(2))
		finish()
	}
	if findPageLabel(&h.router, "Record contribution") != nil {
		t.Fatal("ledger exposes posting forms before the record trigger opens")
	}
	assertReturned := func() {
		t.Helper()
		if timeline.record.open || cash.draft != nil || !timeline.model.Filters().Equal(filter) || timeline.list.Position.First != before.First || timeline.list.Position.Offset != before.Offset {
			t.Fatalf("record closure changed the ledger visit or scroll: %+v / %+v", before, timeline.list.Position)
		}
		h.focused(&timeline.record.trigger)
	}
	click("Record change")
	h.focused(&timeline.record.sheet.close)
	if findPageLabel(&h.router, "Loaded records: 50") == nil {
		t.Fatal("opening the form removed the ledger context")
	}
	click("Cancel")
	assertReturned()
	click("Record change")
	h.press(key.NameEscape)
	finish()
	assertReturned()
	click("Record change")
	click("Close")
	assertReturned()
}

func TestLedgerDebtPaymentUsesWailsPreviewAndPosting(t *testing.T) {
	app, _, _, _ := seedBankBalanceFixture(t, true)
	ref, _, _, refBank := seedBankBalanceFixture(t, true)
	refCard := syntheticCardID(t, ref)
	cash, timeline, h, finish := cashMovementHarness(t, app)
	h.size = image.Pt(1280, 800)
	click := func(name string) { h.click(name, false); finish() }
	click("Record change")
	click("Debt payment")
	d := cash.draft
	if d == nil || d.movement != history.CashDebtPayment || d.account == "" {
		t.Fatal("ledger did not offer the supported debt editor and its eligible cash account")
	}
	h.click("Debt account", true)
	finish()
	click("Synthetic Card · USD")
	setBankField(h, "Debt principal · USD", &d.amount, "10.25")
	setBankField(h, "Interest or fee (optional) · USD", &d.fee, "1.0001")
	click("Preview change")
	if d.review == nil {
		t.Fatalf("standalone debt payment did not reach the shared preview: %v", d.problem)
	}
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeDebtPayment, DebtAccountID: refCard.String(), CashAccountID: refBank.String(),
		Principal: "10.25", PrincipalCurrency: "USD", InterestOrFee: "1.0001", InterestOrFeeCurrency: "USD",
		EffectiveLocalDate: d.day, EffectiveLocalTime: d.time,
	}
	adapter := historyapi.NewService(ref)
	compareLiabilityReview(t, d.review.Preview, adapter, request)
	click("Confirm posting")
	id := timeline.reads.detail.id
	if id == domain.ActivityID("") || timeline.record.open {
		t.Fatal("confirmed debt payment did not close the sheet and open its exact activity")
	}
	assertLiabilityPosting(t, app, id, adapter, request)
}
