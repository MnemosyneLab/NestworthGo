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
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type lostHoldingReviewReceipt struct {
	holdingReconcileAPI
	attempts []string
	loseNext bool
}

func (f *lostHoldingReviewReceipt) CommitAdjustment(ctx context.Context, plan string) ([]wire.ChangePreviewDTO, error) {
	f.attempts = append(f.attempts, plan)
	out, err := f.holdingReconcileAPI.CommitAdjustment(ctx, plan)
	if err == nil && f.loseNext {
		f.loseNext = false
		return nil, errors.New("synthetic lost acknowledgment")
	}
	return out, err
}

func TestTasksHoldingReconciliationCostCorrection(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	reference, _, _, refIDs := openExistingBuyFixture(t)
	clock := func() time.Time { return accountTestClock }
	app.SetClock(clock)
	reference.SetClock(clock)
	api, ref := reconciliation.NewService(app), reconciliation.NewService(reference)
	changing := &lostHoldingReviewReceipt{holdingReconcileAPI: api}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: changing}
	tasks.holdingRecon = c
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(640, 540)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 4 {
			h.frame()
			tasks.workers.Wait()
		}
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	confirm := func() { h.focused(&c.draft.confirm); h.press(key.NameReturn); settle() }
	sessions := func(app *application.Service, count int) {
		t.Helper()
		out, err := app.ListReconciliationSessions(ctx)
		if err != nil || len(out) != count {
			t.Fatalf("sessions: %+v, %v", out, err)
		}
	}
	changeSession := func(api *reconciliation.Service, action application.ReconciliationAction) domain.ReconciliationSession {
		t.Helper()
		plan, err := api.Preview(ctx, action)
		if err != nil {
			t.Fatal(err)
		}
		result, err := api.Commit(ctx, plan.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	holdingItem := func(s *domain.ReconciliationSession, id domain.HoldingID) (domain.ReconciliationAccount, domain.ReconciliationItem) {
		t.Helper()
		for _, account := range s.Accounts {
			for _, item := range account.Items {
				if item.HoldingID == id.String() {
					return account, item
				}
			}
		}
		t.Fatalf("missing holding %s from %+v", id, s)
		return domain.ReconciliationAccount{}, domain.ReconciliationItem{}
	}
	assertEvidence := func(app *application.Service, id string, count int) []domain.ReconciliationLinkedChange {
		t.Helper()
		rows, err := app.ReconciliationLinkedChanges(ctx, id)
		if err != nil || len(rows) != count {
			t.Fatalf("linked reconciliation changes: %+v, %v", rows, err)
		}
		return rows
	}

	settle()
	click("Tasks & checks")
	click("Reconciliation")
	sessions(app, 0)
	click("Start holdings reconciliation")
	h.focused(&c.draft.accountPick.Trigger)
	h.press(key.NameReturn)
	settle()
	click("Harbor Brokerage")
	click("Review")
	click("Cancel")
	h.focused(&c.start)
	sessions(app, 0)

	click("Start holdings reconciliation")
	h.press(key.NameReturn)
	settle()
	click("Harbor Brokerage")
	click("Review")
	if findPageLabel(&h.router, "Harbor Brokerage") == nil {
		t.Fatal("start review omitted the selected account")
	}
	confirm()
	sessions(app, 1)
	id := c.id
	a, item := holdingItem(c.session, ids.holding)
	if item.Value != "2" || item.Cost != "22" || item.Currency != "USD" || a.AccountID != ids.account.String() {
		t.Fatalf("unexpected starting evidence: %+v", item)
	}
	button := c.itemButtons[a.AccountID+"/"+item.ID]
	click("Correct total cost")
	h.focused(&c.draft.total.Editor)
	setBankField(h, "New total cost · USD", &c.draft.total, "bad")
	click("Review")
	if c.draft.total.Text() != "bad" || findPageLabel(&h.router, "Enter a valid total cost in the holding’s currency.") == nil {
		t.Fatal("validation lost the input or its specific error")
	}
	setBankField(h, "New total cost · USD", &c.draft.total, "31.2468")
	click("Review")
	click("Cancel")
	h.focused(button)
	assertEvidence(app, id, 0)

	click("Correct total cost")
	setBankField(h, "New total cost · USD", &c.draft.total, "31.2468")
	click("Review")
	stalePlan := c.draft.review.planID
	paused := changeSession(api, application.ReconciliationAction{
		Action: "pause", SessionID: id, ExpectedRevision: c.session.Revision,
	})
	resumed := changeSession(api, application.ReconciliationAction{
		Action: "resume", SessionID: id, ExpectedRevision: paused.Revision,
	})
	confirm()
	if c.draft.review != nil || c.draft.total.Text() != "31.2468" || c.session == nil || c.session.Revision != resumed.Revision {
		t.Fatal("stale plan did not return to the same draft with refreshed evidence")
	}
	assertEvidence(app, id, 0)

	click("Review")
	adjustment := c.draft.review
	if adjustment.planID == stalePlan {
		t.Fatal("stale review was reused")
	}
	refSession := changeSession(ref, application.ReconciliationAction{Action: "start", AccountIDs: []string{refIDs.account.String()}})
	refAccount, refItem := holdingItem(&refSession, refIDs.holding)
	want, err := ref.PreviewAdjustment(ctx, reconciliation.AdjustmentRequest{
		Link: domain.ReconciliationLink{SessionID: refSession.ID, AccountID: refAccount.AccountID, ItemID: refItem.ID,
			ExpectedRevision: refSession.Revision, FinancialRevision: refAccount.FinancialRevision},
		HoldingID: refIDs.holding.String(), Currency: "USD", Quantity: "2", TotalCost: "31.2468",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := adjustment.changes[0].Effects[0]
	wantEffect := want.Changes[0].Effects[0]
	if got.HoldingID == nil || *got.HoldingID != ids.holding.String() || got.Quantity != nil || got.Target != "holding_cost" ||
		got.CostUnitPrice == nil || *got.CostUnitPrice != *wantEffect.CostUnitPrice || *got.CostUnitPrice != "15.6234" {
		t.Fatalf("Gio/Wails correction: %+v / %+v", got, wantEffect)
	}

	changing.loseNext = true
	confirm()
	if !c.unresolved() || c.draft.review.planID != adjustment.planID {
		t.Fatal("uncertain commit discarded the reviewed plan")
	}
	first := assertEvidence(app, id, 1)[0]
	if first.ItemID != item.ID || first.AccountID != a.AccountID {
		t.Fatalf("linked the wrong reconciliation item: %+v", first)
	}
	confirm()
	if c.unresolved() || !c.saved || len(changing.attempts) != 3 ||
		changing.attempts[1] != adjustment.planID || changing.attempts[2] != adjustment.planID {
		t.Fatalf("retry did not reuse the active plan: %+v", changing.attempts)
	}
	h.focused(button)
	assertEvidence(app, id, 1)
	_, corrected := holdingItem(c.session, ids.holding)
	if corrected.Value != "2" || corrected.Cost != "15.6234" {
		t.Fatalf("refreshed holding evidence: %+v", corrected)
	}
	click("Back to reconciliation")
	h.focused(&c.start)
	click("Open holdings reconciliation · Harbor Brokerage")
	click("Back to reconciliation")
	h.focused(c.sessionButtons[id])

	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	saved, err := reconciliation.NewService(reopened).Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, after := holdingItem(&saved, ids.holding)
	if after.Value != "2" || after.Cost != "15.6234" || assertEvidence(reopened, id, 1)[0].OperationID != first.OperationID {
		t.Fatalf("reopened evidence: %+v", after)
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
}
