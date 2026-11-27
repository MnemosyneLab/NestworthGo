package desktop

import (
	"fmt"
	"image"
	"testing"

	"gioui.org/io/key"
	"gioui.org/io/semantic"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAvailableFundsOrdinaryPolicyPanelCashAndDateCancellation(t *testing.T) {
	ctx := t.Context()
	app, _, _, ids := openFundsFixture(t, true)
	funds, h, settle := reservationHarness(t, app, app)
	t.Cleanup(funds.workers.Wait)
	h.size = image.Pt(1200, 960)
	settle()
	click := func(name string) {
		t.Helper()
		h.click(name, false)
		settle()
	}
	click("Within 30 days")
	month := fundsPresetDay(funds.localDate, 30)
	visit := funds.state
	if visit.horizonOn != month {
		t.Fatalf("expected the 30-day Funds visit, got %+v", visit)
	}
	ref := domain.AccountValueSourceRef(ids.bank)
	button := &funds.sourceButtons(ref.Key()).policy
	const trigger = "Edit source rules · Harbor Savings · USD"
	scrollReservationSource(t, h, trigger)
	click(trigger)
	draft := funds.policy
	if draft == nil || draft.entry != button || draft.source.Key() != ref.Key() || h.shell.activeRoute() != fundsRoute {
		t.Fatalf("cash rule did not open in the Funds visit: %+v", draft)
	}
	title := assertPageLabel(t, &h.router, "Availability rules")
	closeButton := semanticBounds(t, &h.router, "Close", false)
	if title.Min.X < h.size.X/2 || closeButton.Min.X <= title.Min.X || closeButton.Max.X > h.size.X {
		t.Fatalf("cash rules should open at the right edge: title %v, close %v", title, closeButton)
	}
	assertPageLabel(t, &h.router, "Harbor Savings · USD")
	if draft.fields.advanced || findPageLabel(&h.router, "Settlement days") != nil {
		t.Fatal("cash rule opened with advanced inputs exposed")
	}
	h.focused(&draft.fields.presets[policyNow])
	h.press(key.NameDownArrow)
	h.focused(&draft.fields.presets[policySoon])
	h.press(key.NameDownArrow)
	settle()
	h.focused(&draft.fields.presets[policyDate])
	dateChoice := findPageLabel(&h.router, "Available on a date")
	nowChoice := findPageLabel(&h.router, "Available now")
	if dateChoice == nil || dateChoice.Desc.Class != semantic.RadioButton || !dateChoice.Desc.Selected ||
		nowChoice == nil || nowChoice.Desc.Class != semantic.RadioButton || nowChoice.Desc.Selected {
		t.Fatalf("quick choices did not expose the selected availability radio")
	}
	h.click("Available date", true)
	h.frame()
	h.press(key.NameEscape)
	settle()
	if funds.policy != draft {
		t.Fatal("Escape from the date popup discarded the cash policy draft")
	}
	h.focused(&draft.fields.quickUnlock.Trigger)
	h.press(key.NameEscape)
	settle()
	if funds.policy != nil || funds.state != visit || h.shell.activeRoute() != fundsRoute {
		t.Fatalf("closing the draft changed the Funds visit: %+v", funds.state)
	}
	h.focused(button)
	report, err := app.LiquidityRead(ctx, application.LiquidityOverviewQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Sources {
		if row.Result.Ref.Key() == ref.Key() && row.Result.PolicyOrigin == domain.PolicyOriginAssumed {
			return
		}
	}
	t.Fatal("cancelling the draft changed the bank's assumed access rule")
}

func TestAvailableFundsOrdinaryPolicyPanelKeepsScrolledMissingRulesVisit(t *testing.T) {
	ctx := t.Context()
	app, _, _, ids := openFundsFixture(t, true)
	for index := 1; index <= 14; index++ {
		name := fmt.Sprintf("QA-POL-%02d", index)
		instrument, err := app.CreateInstrument(ctx, application.InstrumentInput{
			Name: name, Symbol: name, Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := app.CreateHolding(ctx, application.HoldingInput{
			AccountID: ids.broker.String(), InstrumentID: instrument.ID.String(), Quantity: "2", UnitCost: "10",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := app.AppendManualInstrumentQuote(ctx, instrument.ID, "20", "", false); err != nil {
			t.Fatal(err)
		}
	}
	funds, h, settle := reservationHarness(t, app, app)
	t.Cleanup(funds.workers.Wait)
	h.size = image.Pt(1200, 660)
	settle()
	click := func(name string) {
		t.Helper()
		h.click(name, false)
		settle()
	}
	click("Within 30 days")
	month := fundsPresetDay(funds.localDate, 30)
	missing := fundsSourceCount(funds.current.report.Sources, month, fundsNeedsRules)
	click(fundsMissingRuleText(i18n.English, missing))
	visit := funds.state
	header := funds.list.Position.First
	if visit.filter != fundsNeedsRules || visit.horizonOn != month {
		t.Fatalf("could not select the 30-day Missing rules view: %+v", visit)
	}
	const source = "QA-POL-08"
	const entry = "Complete availability rules · " + source + " · USD"
	for attempts := 0; attempts <= len(funds.current.report.Sources); attempts++ {
		if node := findPageLabel(&h.router, entry); node != nil {
			bounds := semanticBounds(t, &h.router, entry, false)
			if bounds.Min.Y > 140 && bounds.Max.Y < h.size.Y-20 {
				break
			}
		}
		funds.list.ScrollBy(1)
		h.frame()
	}
	position := funds.list.Position
	if position.First <= header {
		t.Fatalf("expected the list to be scrolled past its filter chips: %+v", position)
	}
	click(entry)
	draft := funds.policy
	if draft == nil || draft.source.Kind != domain.SourceHolding || draft.revision != 0 || draft.fields.advanced {
		t.Fatalf("wrong initial holding rule draft: %+v", draft)
	}
	ref, original := draft.source, draft.entry
	assertPageLabel(t, &h.router, source+" · USD")
	click("Close")
	if funds.policy != nil || funds.state != visit {
		t.Fatalf("closed holding rule did not restore the Missing rules visit: %+v", funds.state)
	}
	h.focused(original)
	click(entry)
	click("Available now")
	click("Save policy")
	if funds.policy != nil || funds.state != visit || h.shell.activeRoute() != fundsRoute {
		t.Fatalf("saving the rule left the 30-day Missing rules view: %+v", funds.state)
	}
	h.focused(&funds.reload)
	if got := funds.list.Position; got.First != position.First {
		t.Fatalf("save moved the background list to row %d from %d", got.First, position.First)
	}
	if count := fundsSourceCount(funds.current.report.Sources, month, fundsNeedsRules); count != missing-1 {
		t.Fatalf("saved rule did not remove one source from Missing rules: %d -> %d", missing, count)
	}
	row := funds.policySource(ref)
	if row == nil || row.Result.PolicyOrigin != domain.PolicyOriginExplicit || row.Result.Policy == nil || row.Result.Policy.Revision != 1 {
		t.Fatalf("result lacks the saved holding rule: %+v", row)
	}
	projected := fundsResultForHorizon(row.Result, month)
	if projected == nil || projected.NeedsRules || projected.NetNative == nil || projected.NetNative.CanonicalAmount() != "40" {
		t.Fatalf("wrong 30-day projection after saving: %+v", projected)
	}
}
