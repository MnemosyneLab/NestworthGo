package desktop

import (
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestAccountProductDrawerReceiptReturnsToItsSource(t *testing.T) {
	app, _, _, fixture := openManagedProductFixture(t)
	ctx := t.Context()
	clock := func() time.Time { return accountTestClock }
	funds := newAvailableFundsPage(ctx, app, func() {})
	p := newProductDetailPage(ctx, app, func() {})
	fault := &standaloneLifecycleFault{Service: app}
	p.receipts, p.clock = fault, clock
	p.termsLedger = fault
	funds.productPage = p
	accounts := newAccountsPage(ctx, app, clock, func() {})
	timeline := newLedgerHistory(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{funds: funds, accounts: accounts, history: timeline, version: app.MobileDataVersion})
	h.size = image.Pt(900, 600)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	drain := func() {
		p.workers.Wait()
		funds.workers.Wait()
		accounts.workers.Wait()
		timeline.model.Wait()
		timeline.reads.workers.Wait()
	}
	t.Cleanup(drain)
	settle := func() {
		for range 4 {
			h.frame()
			drain()
		}
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	scrollClick := func(label string) { t.Helper(); scrollProductControl(t, h, label); click(label) }
	settle()
	click("Accounts")
	click("Harbor Deposit Bank")
	const product = "Open product · Due Deposit"
	id := fixture.ids["Due Deposit"]
	scrollClick(product)
	if target := h.shell.trail.current().target; target.route != accountsRoute || target.account != fixture.account || h.shell.accountProduct == nil {
		t.Fatalf("product drawer lost its account context: %+v", target)
	}
	assertPageLabel(t, &h.router, "Principal · US$600.00")
	h.focused(&h.shell.accountProduct.panel.close)
	start := p.list.Position
	h.press(key.NamePageDown)
	if p.list.Position.First == start.First && p.list.Position.Offset == start.Offset {
		t.Fatal("Page Down from Close did not scroll product details")
	}
	h.press(key.NamePageUp)
	if p.list.Position.First != start.First || p.list.Position.Offset != start.Offset {
		t.Fatal("Page Up from Close did not return to the product summary")
	}

	click("Edit terms and rules")
	if p.terms == nil {
		t.Fatal("drawer did not open the product terms editor")
	}
	const renamed = "Revised Due Deposit"
	p.terms.fields.name.Editor.SetText(renamed)
	click("Save policy")
	if p.terms != nil || h.shell.accountProduct.name != renamed {
		t.Fatal("product drawer returned with the old contract name")
	}
	assertPageLabel(t, &h.router, renamed)

	click("Confirm actual receipt")
	draft := p.receipt
	if draft == nil || draft.principal.Text() != "600" {
		t.Fatal("drawer did not retain the shared exact receipt editor")
	}
	draft.interest.Editor.SetText("6.25")
	click("Review receipt")
	assertPageLabel(t, &h.router, "Interest received · 6.25 USD")
	fault.loseReceipt = true
	click("Post actual receipt")
	if !p.unresolved() || h.shell.accountProduct == nil {
		t.Fatal("lost response did not preserve the drawer and its retry")
	}
	h.press(key.NameEscape)
	settle()
	if !p.unresolved() || h.shell.accountProduct == nil {
		t.Fatal("Escape discarded a pending settlement outcome")
	}
	click("Retry same submission")
	if len(fault.receipts) != 2 || !reflect.DeepEqual(fault.receipts[0], fault.receipts[1]) || len(managedOperations(t, app, id)) != 2 {
		t.Fatal("drawer changed the settlement retry or duplicated it")
	}
	if p.current == nil || len(p.current.settlement) != 2 {
		t.Fatal("drawer did not refresh both settlement receipts")
	}
	for _, receipt := range []struct {
		label    string
		activity domain.ActivityID
		returnBy string
	}{
		{"Open principal receipt", p.current.settlement[0].ID, "Back to product details"},
		{"Open interest receipt", p.current.settlement[1].ID, "Open product · " + renamed},
	} {
		scrollProductControl(t, h, receipt.label)
		productPosition, accountPosition := p.list.Position, accounts.list.Position
		click(receipt.label)
		if target := h.shell.trail.current().target; target.activity != receipt.activity || target.route != historyRoute {
			t.Fatalf("drawer opened the wrong ledger receipt: %+v", target)
		}
		click(receipt.returnBy)
		if target := h.shell.trail.current().target; target.account != fixture.account || h.shell.accountProduct == nil {
			t.Fatalf("receipt return did not reopen the same account drawer: %+v", target)
		}
		if p.list.Position.First != productPosition.First || p.list.Position.Offset != productPosition.Offset ||
			accounts.list.Position.First != accountPosition.First || accounts.list.Position.Offset != accountPosition.Offset {
			t.Fatal("receipt return moved the product or account scroll position")
		}
		h.focused(p.activities[receipt.activity])
	}
	click("Close")
	h.focused(accounts.productButtons[id])
	if h.shell.accountProduct != nil || h.shell.activeRoute() != accountsRoute || len(managedOperations(t, app, id)) != 2 {
		t.Fatal("closing the drawer changed account scope or posted an operation")
	}
}
