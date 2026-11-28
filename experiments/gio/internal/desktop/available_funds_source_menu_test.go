package desktop

import (
	"fmt"
	"image"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
)

func chooseFundsSourceAction(t *testing.T, h *previewHarness, source, action string) {
	t.Helper()
	trigger := "Actions for " + source
	scrollReservationSource(t, h, trigger)
	h.click(trigger, false)
	h.frame()
	chooseFundsMenuOption(t, h, action)
}

func chooseFundsMenuOption(t *testing.T, h *previewHarness, action string) {
	t.Helper()
	panel := h.shell.overlay.Panel
	if h.shell.overlay.Trigger == nil || panel.Empty() {
		t.Fatalf("the source action menu did not open for %q", action)
	}
	var bounds image.Rectangle
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Class == semantic.Button && node.Desc.Bounds.Overlaps(panel) &&
				(node.Desc.Label == action || node.Desc.Description == action ||
					len(node.Children) == 1 && node.Children[0].Desc.Label == action) {
				bounds = node.Desc.Bounds.Intersect(panel)
			}
			visit(node.Children)
		}
	}
	visit(h.router.AppendSemantics(nil))
	if bounds.Empty() {
		t.Fatalf("action %q is missing from the source menu", action)
	}
	h.clickAt(bounds.Min.Add(bounds.Max).Div(2))
}

func TestAvailableFundsMissingRulesShortcutScrollsAndRestoresSourceActions(t *testing.T) {
	app, _, _, ids := openFundsFixture(t, true)
	funds, h, settle := reservationHarness(t, app, app)
	h.size = image.Pt(900, 600)
	settle()
	if funds.list.Position.First != 0 {
		t.Fatal("the viewport did not start at its comparison")
	}
	bucket := funds.selectedFundsBucket()
	count := fundsSourceCount(funds.current.report.Sources, bucket.HorizonOn, fundsNeedsRules)
	if count != 1 {
		t.Fatalf("expected one missing rule, got %d", count)
	}
	h.click(fmt.Sprintf("%d item needs availability rules", count), false)
	settle()
	if funds.list.Position.First == 0 || funds.state.filter != fundsNeedsRules || funds.selectedFundsBucket().HorizonOn != bucket.HorizonOn {
		t.Fatalf("missing-rules shortcut did not jump with its selected horizon: %+v / %+v", funds.list.Position, funds.state)
	}
	h.focused(&funds.controls.filters[fundsNeedsRules])
	assertPageLabel(t, &h.router, "QA-FUND")
	assertPageLabel(t, &h.router, "Hill Brokerage")
	assertPageLabel(t, &h.router, "Needs information")
	const menu = "Actions for QA-FUND · USD"
	if findPageLabel(&h.router, "Manage reservations · QA-FUND · USD") != nil {
		t.Fatal("the scoped reservation action is duplicated outside its menu")
	}
	scrollReservationSource(t, h, menu)
	h.click(menu, false)
	h.frame()
	trigger := semanticBounds(t, &h.router, menu, false)
	if anchor := h.shell.overlay.Anchor; !anchor.Overlaps(trigger) {
		t.Fatalf("source menu detached from its scrolled trigger: %v / %v", anchor, trigger)
	}
	h.press(key.NameEscape)
	settle()
	var sourceKey string
	for _, source := range funds.current.report.Sources {
		if source.InstrumentID != nil && *source.InstrumentID == ids.instrument.String() {
			sourceKey = source.Result.SourceKey
			break
		}
	}
	if sourceKey == "" {
		t.Fatal("missing seeded holding")
	}
	h.focused(&funds.sourceButtons(sourceKey).menu.Trigger)
	h.click("About source amounts", false)
	h.frame()
	assertPageLabel(t, &h.router, "Source amounts are shown in each source’s own currency.")
	h.press(key.NameEscape)
	settle()
	h.focused(&funds.controls.sourceHelp)
	h.click("Show items", false)
	settle()
	if funds.state.filter != fundsNeedsInfo || funds.selectedFundsBucket().HorizonOn != bucket.HorizonOn {
		t.Fatal("the needs-information banner lost the intended source scope")
	}
	h.focused(&funds.controls.filters[fundsNeedsInfo])
	assertPageLabel(t, &h.router, fmt.Sprintf("Needs information %d", fundsSourceCount(funds.current.report.Sources, bucket.HorizonOn, fundsNeedsInfo)))
}
