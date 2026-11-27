package desktop

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestReconciliationAccountImportAndCashEntriesMatchWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	clock := func() time.Time { return accountTestClock }
	app.SetClock(clock)
	ref.SetClock(clock)
	fault := &linkedDatedFault{Service: app}
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	session, refSession := startTestHoldingReconciliation(t, api, ids.account), startTestHoldingReconciliation(t, refAPI, refIDs.account)
	wails := historyapi.NewService(ref)
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: api, ledger: fault, clock: clock}
	tasks.holdingRecon = c
	ledger := newLedgerHistory(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, history: ledger, version: app.MobileDataVersion})
	h.size = image.Pt(640, 1520)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 5 {
			h.frame()
			tasks.workers.Wait()
			if c.cashDated != nil {
				c.cashDated.workers.Wait()
			}
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	click := func(s string) { t.Helper(); h.click(s, false); settle() }
	label := func(s string) { t.Helper(); assertPageLabel(t, &h.router, s) }
	settle()
	click("Tasks & checks")
	click("Reconciliation")
	click("Open holdings reconciliation · Harbor Brokerage")
	const day = "2026-11-22"
	const importNote = "Synthetic imported position"
	recordReference := func(itemID, at, note string, req historyapi.ChangeCommandRequest, got domain.ChangePreview) {
		t.Helper()
		req.Note, req.EffectiveLocalDate, req.EffectiveLocalTime = &note, day, at
		req.MutationID = domain.NewMutationID().String()
		req.ReconciliationLink = &domain.ReconciliationLink{SessionID: refSession.ID, AccountID: refIDs.account.String(), ItemID: itemID,
			ExpectedRevision: refSession.Revision, FinancialRevision: refSession.Accounts[0].FinancialRevision}
		want, err := wails.PreviewChange(ctx, req)
		if err != nil || want.Activity.EffectiveAt != wire.FormatTime(got.Activity.EffectiveAt) || len(want.Effects) != len(got.Effects) ||
			len(want.Resulting) != len(got.Resulting) {
			t.Fatalf("checkpoint preview differs: %+v / %+v / %v", want, got, err)
		}
		for i, e := range got.Resulting {
			if w := want.Resulting[i]; w.Amount != e.Amount || w.Quantity != e.Quantity || w.Currency != e.Currency.String() {
				t.Fatalf("result at posting differs: %+v / %+v", w, e)
			}
		}
		if _, err := wails.RecordChange(ctx, req); err != nil {
			t.Fatal(err)
		}
		if refSession, err = refAPI.Get(ctx, refSession.ID); err != nil {
			t.Fatal(err)
		}
	}
	scope := c.checkpointActions(ids.account.String(), "scope")
	importDraft := func() *accountInvestmentDraft {
		t.Helper()
		click("Record existing position")
		d := c.dated.draft
		if d.pickAccount || d.account.ID != ids.account || d.instrument != ids.foreign ||
			d.link().SessionID != session.ID || d.link().ItemID != "scope" {
			t.Fatal("import did not keep the exact account completeness checkpoint")
		}
		h.focused(&d.quantity.Editor)
		d.day, d.time = day, "00:45"
		setBankField(h, "Existing quantity", &d.quantity, "2.12345678")
		setBankField(h, "Original cost per unit · EUR", &d.unitCost, "11.12345678")
		setBankField(h, "Notes", &d.note, importNote)
		click("Review existing position")
		label("This position does not move cash.")
		h.focused(&d.confirm)
		return d
	}
	before := app.MobileDataVersion()
	importDraft()
	click("Cancel")
	h.focused(&scope.importPosition)
	if app.MobileDataVersion() != before || len(fault.attempts) != 0 || len(c.links) != 0 {
		t.Fatal("canceling the scoped import wrote an activity or changed reconciliation")
	}
	d := importDraft()
	recordReference("scope", "00:45", importNote, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionImport, AccountID: refIDs.account.String(), InstrumentID: refIDs.foreign.String(),
		Quantity: "2.12345678", UnitCost: "11.12345678", Currency: "EUR",
	}, d.review.Preview)
	h.press(key.NameReturn)
	settle()
	h.focused(&scope.importPosition)
	assertNativeTradeCash(t, app, ids.account, map[domain.CurrencyCode]string{"USD": "156"})

	cash := c.checkpointActions(ids.account.String(), "cash:USD")
	cashDraft := func(mode history.CashMovement, amount, at, note, reason string) *cashMovementDraft {
		t.Helper()
		click("Record dated cash change · USD")
		f := c.cashDated
		if f.draft.account != ids.account || f.draft.currency != "USD" || f.linked.link().ItemID != "cash:USD" {
			t.Fatal("cash action did not keep the exact account and USD sleeve")
		}
		h.focused(&f.draft.amount.Editor)
		f.draft.day, f.draft.time = day, at
		setBankField(h, "Notes", &f.draft.note, note)
		if mode == history.CashWithdrawal {
			click("Money removed")
			if f.draft.day != day || f.draft.time != at || f.draft.note.Text() != note {
				t.Fatal("switching cash action dropped the date, time or note")
			}
		}
		h.click("Reason", true)
		settle()
		click(reason)
		setBankField(h, "Amount · USD", &f.draft.amount, amount)
		click("Preview change")
		h.focused(&f.draft.post)
		return f.draft
	}
	const addNote, removeNote = "Synthetic linked cash income", "Synthetic linked cash fee"
	before = app.MobileDataVersion()
	cashDraft(history.CashContribution, "1.2345", "00:50", addNote, "Income")
	click("Cancel")
	h.focused(&cash.cash)
	if app.MobileDataVersion() != before || len(fault.attempts) != 1 || len(c.links) != 1 {
		t.Fatal("canceling the cash preview wrote to the ledger")
	}
	add := cashDraft(history.CashContribution, "1.2345", "00:50", addNote, "Income")
	recordReference("cash:USD", "00:50", addNote, historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyAdded,
		AccountID: refIDs.account.String(), Currency: "USD", Amount: "1.2345", Reason: "income"}, add.review.Preview)
	fault.loseNext = true
	h.press(key.NameReturn)
	settle()
	if !c.unresolved() {
		t.Fatal("ambiguous cash posting retired the exact plan")
	}
	h.press(key.NameReturn)
	settle()
	h.focused(&cash.cash)
	if len(fault.attempts) != 3 || fault.attempts[1] != fault.attempts[2] {
		t.Fatalf("cash retry changed the reviewed identity: %+v", fault.attempts)
	}

	remove := cashDraft(history.CashWithdrawal, "0.2345", "00:55", removeNote, "Fee")
	for _, action := range []string{"pause", "resume"} {
		next, err := api.Preview(ctx, application.ReconciliationAction{Action: action, SessionID: session.ID, ExpectedRevision: c.session.Revision})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := api.Commit(ctx, next.PlanID); err != nil {
			t.Fatal(err)
		}
		tasks.readHoldingReconciliation()
		settle()
	}
	h.press(key.NameReturn)
	settle()
	label("The ledger changed. Preview this cash movement again.")
	if remove.review != nil || remove.day != day || remove.time != "00:55" || remove.note.Text() != removeNote ||
		remove.amount.Text() != "0.2345" || len(c.links) != 2 {
		t.Fatal("stale cash review wrote a link or lost the draft")
	}
	click("Preview change")
	h.focused(&remove.post)
	recordReference("cash:USD", "00:55", removeNote, historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyRemoved,
		AccountID: refIDs.account.String(), Currency: "USD", Amount: "0.2345", Reason: "fee"}, remove.review.Preview)
	h.press(key.NameReturn)
	settle()
	h.focused(&cash.cash)
	activityByNote := make(map[string]domain.ActivityID)
	for _, link := range c.links {
		var e holdingLinkedEvidence
		if err := jsoncodec.Unmarshal(link.Evidence, &e); err != nil || e.Before == nil || e.Committed.Note == nil ||
			link.AccountID != ids.account.String() || link.ItemID != e.Before.ID || link.SessionID != session.ID {
			t.Fatalf("persisted link differs: %+v / %v", link, err)
		}
		note := *e.Committed.Note
		if (note == importNote && link.ItemID != "scope") || (note != importNote && link.ItemID != "cash:USD") {
			t.Fatalf("wrong working point for %q: %s", note, link.ItemID)
		}
		activityByNote[note] = domain.ActivityID(link.OperationID)
	}
	if len(activityByNote) != 3 {
		t.Fatalf("expected each note and link once, got %+v", activityByNote)
	}
	receipt := "View linked activity · " + day + " · " + addNote
	position := semanticBounds(t, &h.router, receipt, false).Min.Y - 240
	h.size.Y = 480
	h.shell.stage.Position.Offset = position
	settle()
	click(receipt)
	if ledger.reads.detail.id != activityByNote[addNote] {
		t.Fatal("cash link opened the wrong receipt")
	}
	click("Back to reconciliation")
	h.focused(c.receiptButtons[activityByNote[addNote]])
	if h.shell.stage.Position.Offset == 0 {
		t.Fatal("Back scrolled away from the cash receipt at compact size")
	}

	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "157"})
	links, err := reconciliation.NewService(reopened).Links(ctx, session.ID)
	if err != nil || len(links) != 3 {
		t.Fatalf("the three linked receipts did not survive reopen: %+v / %v", links, err)
	}
	for _, check := range []struct {
		note, amount string
		reason       domain.ActivityReason
	}{{addNote, "1.2345", domain.ReasonIncome}, {removeNote, "0.2345", domain.ReasonFee}} {
		a, err := reopened.Activity(ctx, activityByNote[check.note])
		if err != nil || a.EffectiveLocalDate != day || a.Note == nil || *a.Note != check.note || a.Reason != check.reason ||
			len(a.Effects) != 1 || a.Effects[0].Money == nil || a.Effects[0].Money.CanonicalAmount() != check.amount {
			t.Fatalf("wrong reopened cash activity: %+v / %v", a, err)
		}
	}
	a, err := reopened.Activity(ctx, activityByNote[importNote])
	if err != nil || a.EffectiveLocalDate != day || len(a.Effects) != 1 || a.Effects[0].HoldingID == nil ||
		a.Effects[0].Quantity.Canonical() != "2.12345678" || a.Effects[0].CostUnitPrice.Canonical() != "11.12345678" {
		t.Fatalf("wrong reopened import: %+v / %v", a, err)
	}
	positionHolding, err := reopened.HoldingsByAccounts(ctx, []domain.AccountID{ids.account})
	if err != nil || len(positionHolding[ids.account]) != 2 {
		t.Fatalf("wrong reopened holdings: %+v / %v", positionHolding, err)
	}
	for _, holding := range positionHolding[ids.account] {
		if holding.InstrumentID == ids.foreign && holding.ID == *a.Effects[0].HoldingID {
			return
		}
	}
	t.Fatal("the import did not link to the new holding in the reconciled account")
}
