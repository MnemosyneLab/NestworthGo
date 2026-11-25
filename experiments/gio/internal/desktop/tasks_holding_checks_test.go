package desktop

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

type lostSessionCheckReceipt struct {
	holdingReconcileAPI
	loseNext bool
	plans    []string
}

func (f *lostSessionCheckReceipt) Commit(ctx context.Context, plan string) (domain.ReconciliationSession, error) {
	f.plans = append(f.plans, plan)
	session, err := f.holdingReconcileAPI.Commit(ctx, plan)
	if err == nil && f.loseNext {
		f.loseNext = false
		return domain.ReconciliationSession{}, errors.New("synthetic lost session confirmation")
	}
	return session, err
}

func TestHoldingCheckAndClosureMatchWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	clock := func() time.Time { return accountTestClock }
	app.SetClock(clock)
	ref.SetClock(clock)
	api, reference := reconciliation.NewService(app), reconciliation.NewService(ref)
	session := startTestHoldingReconciliation(t, api, ids.account)
	wails := startTestHoldingReconciliation(t, reference, refIDs.account)
	account, item := testHoldingCheckpoint(t, session, ids.holding)
	otherAccount, otherItem := testHoldingCheckpoint(t, wails, refIDs.holding)
	fault := &lostSessionCheckReceipt{holdingReconcileAPI: api}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: fault, ledger: app, clock: clock}
	tasks.holdingRecon = c
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(640, 960)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 5 {
			h.frame()
			tasks.workers.Wait()
		}
	}
	click := func(s string) { t.Helper(); h.click(s, false); settle() }
	label := func(s string) { t.Helper(); assertPageLabel(t, &h.router, s) }
	rec := func(state string, checked int) domain.ReconciliationSession {
		t.Helper()
		got, err := api.Get(ctx, session.ID)
		if err != nil || got.Status != state || got.Counts.Confirmed != checked {
			t.Fatalf("unexpected session state: %s, %+v, %v", got.Status, got.Counts, err)
		}
		return got
	}
	refCommit := func(in application.ReconciliationAction) domain.ReconciliationSession {
		t.Helper()
		in.SessionID, in.ExpectedRevision = wails.ID, wails.Revision
		plan, err := reference.Preview(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		result, err := reference.Commit(ctx, plan.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	confirm := func() { h.focused(&c.draft.confirm); h.press(key.NameReturn); settle() }

	settle()
	click("Tasks & checks")
	click("Reconciliation")
	click("Open holdings reconciliation · Harbor Brokerage")
	click("Check · Synthetic Index Fund")
	d := c.draft
	h.focused(&d.check.picker.Trigger)
	if got, _ := c.checkDate(d.check); got != "2026-11-22" {
		t.Fatalf("expected the Singapore civil day, got %s", got)
	}
	setBankField(h, "Evidence reference (optional)", &d.check.reference, "Synthetic custody statement")
	setBankField(h, "Source value (optional)", &d.check.value, "2.00000001")
	d.check.comparable.Value = true
	click("Review")
	label("Check the numeric source value. If it differs from the ledger, explain the difference, correct the ledger, or leave the item unresolved.")
	rec("in_progress", 0)
	d.check.day, d.check.daySet = "2026-11-21", true
	click("Review")
	label("A historical source value cannot be compared with the current ledger checkpoint.")
	d.check.day, d.check.daySet = "", false
	d.check.time = "00:55"
	explanation := "The source reports a separate rounding difference."
	setBankField(h, "Notes", &d.note, explanation)
	click("Review")
	label("Source value · 2.00000001")
	click("Cancel")
	h.focused(&c.itemButtons[account.AccountID+"/"+item.ID].check)
	rec("in_progress", 0)

	click("Check · Synthetic Index Fund")
	d = c.draft
	setBankField(h, "Evidence reference (optional)", &d.check.reference, "Synthetic custody statement")
	setBankField(h, "Source value (optional)", &d.check.value, "2.00000001")
	setBankField(h, "Notes", &d.note, explanation)
	d.check.comparable.Value, d.check.time = true, "00:55"
	click("Review")
	confirm()
	label("Check recorded.")
	h.focused(&c.itemButtons[account.AccountID+"/"+item.ID].check)
	evidence := domain.ReconciliationSource{Reference: "Synthetic custody statement", AsOfDate: "2026-11-22", AsOfTime: "00:55"}
	wails = refCommit(application.ReconciliationAction{Action: "check", AccountID: otherAccount.AccountID, ItemID: otherItem.ID,
		FinancialRevision: otherAccount.FinancialRevision, Outcome: "confirmed", Source: evidence, ReferenceComparable: true,
		ReferenceValue: "2.00000001", Note: explanation})
	_, checked := testHoldingCheckpoint(t, rec("in_progress", 1), ids.holding)
	_, expected := testHoldingCheckpoint(t, wails, refIDs.holding)
	if checked.Source != expected.Source || checked.Source.Timezone != "Asia/Singapore" ||
		checked.ReferenceValue != expected.ReferenceValue || checked.Note != expected.Note || len(checked.Attempts) != 1 {
		t.Fatalf("persisted evidence differs from Wails: %+v / %+v", checked, expected)
	}

	click("Finish reconciliation")
	click("Review")
	label("To finish with outstanding checks, choose partial completion and enter a closing note.")
	c.draft.partial.Value = true
	click("Review")
	label("To finish with outstanding checks, choose partial completion and enter a closing note.")
	setBankField(h, "Closing note", &c.draft.note, "Two checks awaiting independent evidence.")
	click("Review")
	label("Partially completed")
	click("Cancel")
	h.focused(&c.finish)
	rec("in_progress", 1)

	for _, target := range []struct{ label, id string }{{"Record completeness", "scope"}, {"USD cash", "cash:USD"}} {
		click("Check · " + target.label)
		click("Review")
		confirm()
		otherAccount, _ = testHoldingCheckpoint(t, wails, refIDs.holding)
		wails = refCommit(application.ReconciliationAction{Action: "check", AccountID: otherAccount.AccountID, ItemID: target.id,
			FinancialRevision: otherAccount.FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{AsOfDate: "2026-11-22"}})
	}
	rec("in_progress", 3)
	click("Finish reconciliation")
	closing := "Synthetic statement checked in full."
	setBankField(h, "Closing note", &c.draft.note, closing)
	click("Review")
	label("Completed")
	fault.loseNext = true
	confirm()
	if !c.unresolved() {
		t.Fatal("ambiguous closure lost its reviewed plan")
	}
	confirm()
	if n := len(fault.plans); n != 5 || fault.plans[n-1] != fault.plans[n-2] {
		t.Fatalf("closure retry changed its plan identity: %+v", fault.plans)
	}
	closed := rec("completed", 3)
	wails = refCommit(application.ReconciliationAction{Action: "finish", Note: closing})
	if closed.Partial || closed.Counts != wails.Counts || closed.ClosingNote != wails.ClosingNote || closed.CompletedAt != wails.CompletedAt {
		t.Fatalf("closure differs from Wails: %+v / %+v", closed, wails)
	}
	h.focused(&c.back)
	if findPageLabel(&h.router, "Finish reconciliation") != nil || findPageLabel(&h.router, "Check · Synthetic Index Fund") != nil {
		t.Fatal("completed session still offers mutable checkpoint actions")
	}
	click("Back to reconciliation")
	click("Open holdings reconciliation · Harbor Brokerage")
	label("Completed")
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	persisted, err := reconciliation.NewService(reopened).Get(ctx, session.ID)
	if err != nil || persisted.Status != "completed" || persisted.ClosingNote != closing || persisted.Revision != closed.Revision {
		t.Fatalf("closure did not survive reopen: %+v / %v", persisted, err)
	}
	if links, err := reconciliation.NewService(reopened).Links(ctx, session.ID); err != nil || len(links) != 0 {
		t.Fatalf("checking and closure changed financial activity links: %+v / %v", links, err)
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
}
