package desktop

import (
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"reflect"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

func TestConfirmHoldingAccountMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
	testfixture.SetClock(app, linkedContractClock)
	testfixture.SetClock(ref, linkedContractClock)
	api, reference := reconciliation.NewService(app), reconciliation.NewService(ref)
	session := startTestHoldingReconciliation(t, api, ids.account)
	wails := startTestHoldingReconciliation(t, reference, refIDs.account)
	commit := func(service *reconciliation.Service, current domain.ReconciliationSession, action application.ReconciliationAction) domain.ReconciliationSession {
		t.Helper()
		action.SessionID, action.ExpectedRevision = current.ID, current.Revision
		plan, err := service.Preview(ctx, action)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := service.Commit(ctx, plan.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	preserve := func(service *reconciliation.Service, current domain.ReconciliationSession, id domain.HoldingID) domain.ReconciliationSession {
		t.Helper()
		a, row := testHoldingCheckpoint(t, current, id)
		return commit(service, current, application.ReconciliationAction{Action: "check", AccountID: a.AccountID, FinancialRevision: a.FinancialRevision,
			ItemID: row.ID, Outcome: "confirmed", Source: domain.ReconciliationSource{AsOfDate: "2026-11-22", Reference: "Earlier synthetic holding statement"},
			Note: "Previously checked shares and acquisition cost."})
	}
	session, wails = preserve(api, session, ids.holding), preserve(reference, wails, refIDs.holding)
	_, previous := testHoldingCheckpoint(t, session, ids.holding)
	before, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	fault := &lostSessionCheckReceipt{holdingReconcileAPI: api}
	h := newLinkedProductHarness(t, app, fault)
	accountButton := &h.c.checkpointActions(ids.account.String(), "scope").checkAccount
	const title = "Confirm account"
	const evidence = "Synthetic brokerage statement"
	const note = "Checked the complete account against the same statement."
	const date, minute = "2026-11-23", "18:05"
	open := func() *holdingReconDraft {
		t.Helper()
		h.activate(t, title)
		d := h.c.draft
		if d.accountID != ids.account.String() || d.itemID != "scope" || h.c.canReview(d) {
			t.Fatal("whole account draft did not require explicit completeness acknowledgement")
		}
		if findPageLabel(&h.router, "Synthetic Index Fund") != nil || findPageLabel(&h.router, "Record completeness") == nil || findPageLabel(&h.router, "USD cash") == nil {
			t.Fatal("review scope did not isolate the remaining items")
		}
		h.focused(&d.check.scopeReviewed)
		h.press(key.NameSpace)
		h.settle()
		if !d.check.scopeReviewed.Value {
			t.Fatal("space did not acknowledge completeness")
		}
		d.check.reference.Editor.SetText(evidence)
		d.check.day, d.check.daySet, d.check.time = date, true, minute
		d.note.Editor.SetText(note)
		h.settle()
		h.activate(t, "Review")
		if d.review == nil || d.review.session.Counts.Confirmed != 3 || d.review.session.Status != "in_progress" {
			t.Fatal("batch review does not show the three scoped checks")
		}
		return d
	}
	d := open()
	h.activate(t, "Back to details")
	h.focused(&d.check.scopeReviewed)
	if !d.check.scopeReviewed.Value || d.check.reference.Text() != evidence || d.note.Text() != note {
		t.Fatal("Back discarded the source or acknowledgement")
	}
	h.activate(t, "Review")
	h.activate(t, "Cancel")
	h.focused(accountButton)
	unchanged, err := api.Get(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(unchanged, session) {
		t.Fatalf("confirmed evidence changed on Cancel: %v", err)
	}
	d = open()
	stale := d.review.planID
	session = commit(api, session, application.ReconciliationAction{Action: "pause"})
	session = commit(api, session, application.ReconciliationAction{Action: "resume"})
	h.press(key.NameReturn)
	h.settle()
	if d.review != nil || d.check.reference.Text() != evidence || !d.check.scopeReviewed.Value || h.c.unresolved() {
		t.Fatal("stale account confirmation did not preserve the draft")
	}
	h.activate(t, "Review")
	plan := d.review.planID
	fault.loseNext = true
	h.press(key.NameReturn)
	h.settle()
	if !h.c.unresolved() {
		t.Fatal("unknown confirmation outcome did not protect the reviewed plan")
	}
	h.press(key.NameReturn)
	h.settle()
	if !reflect.DeepEqual(fault.plans, []string{stale, plan, plan}) {
		t.Fatalf("unexpected posted plan IDs: %v", fault.plans)
	}
	assertPageLabel(t, &h.router, "Account check recorded.")
	h.focused(&h.c.back)
	wails = commit(reference, wails, application.ReconciliationAction{Action: "check_account", AccountID: refIDs.account.String(),
		FinancialRevision: wails.Accounts[0].FinancialRevision, Outcome: "confirmed", ScopeReviewed: true,
		Source: domain.ReconciliationSource{Reference: evidence, AsOfDate: date, AsOfTime: minute}, Note: note})
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := refStorage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	testfixture.SetClock(reopened, linkedContractClock)
	referenceApp, _ := reopenBaselineProfile(t, refPaths)
	testfixture.SetClock(referenceApp, linkedContractClock)
	got, err := reconciliation.NewService(reopened).Get(ctx, session.ID)
	want, refErr := reconciliation.NewService(referenceApp).Get(ctx, wails.ID)
	if err != nil || refErr != nil || got.Status != "in_progress" || got.Counts.Confirmed != 3 || got.Counts != want.Counts {
		t.Fatalf("wrong reopened review: %+v / %+v, %v / %v", got.Counts, want.Counts, err, refErr)
	}
	_, preserved := testHoldingCheckpoint(t, got, ids.holding)
	if !reflect.DeepEqual(preserved, previous) {
		t.Fatalf("existing evidence changed: %+v / %+v", preserved, previous)
	}
	for _, id := range []string{"scope", "cash:USD"} {
		_, actual, ok := checkpointInSession(&got, ids.account.String(), id, false)
		_, expected, exists := checkpointInSession(&want, refIDs.account.String(), id, false)
		if !ok || !exists || len(actual.Attempts) != 1 || actual.Source != expected.Source || actual.Source.Timezone != "Asia/Singapore" || actual.Note != expected.Note || actual.ReferenceValue != "" {
			t.Fatalf("scope or evidence did not survive reopen: %+v / %+v", actual, expected)
		}
	}
	rows, err := reopened.ListActivities(ctx, 100)
	if err != nil || !reflect.DeepEqual(rows, before) {
		t.Fatal("account confirmation changed the financial journal")
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
}

func TestConfirmHoldingAccountRejectsExistingException(t *testing.T) {
	app, _, _, ids := openExistingBuyFixture(t)
	testfixture.SetClock(app, linkedContractClock)
	api := reconciliation.NewService(app)
	session := startTestHoldingReconciliation(t, api, ids.account)
	a, item := testHoldingCheckpoint(t, session, ids.holding)
	plan, err := api.Preview(t.Context(), application.ReconciliationAction{Action: "check", SessionID: session.ID, ExpectedRevision: session.Revision,
		AccountID: a.AccountID, FinancialRevision: a.FinancialRevision, ItemID: item.ID, Outcome: "skipped", Note: "Custody evidence unavailable."})
	if err != nil {
		t.Fatal(err)
	}
	before, err := api.Commit(t.Context(), plan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	h := newLinkedProductHarness(t, app, api)
	h.activate(t, "Confirm account")
	d := h.c.draft
	h.press(key.NameSpace)
	h.settle()
	h.activate(t, "Review")
	assertPageLabel(t, &h.router, "Review unresolved or previously recorded evidence individually before confirming this account.")
	after, err := api.Get(t.Context(), session.ID)
	if err != nil || d.review != nil || !reflect.DeepEqual(after, before) {
		t.Fatal("a whole-account check bypassed an individual exception")
	}
	h.activate(t, "Cancel")
	h.focused(&h.c.checkpointActions(ids.account.String(), "scope").checkAccount)
}
