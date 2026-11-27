package desktop

import (
	"context"
	"errors"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
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
	testfixture.SetClock(app, clock)
	testfixture.SetClock(reference, clock)
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
	start := &linkedProductHarness{previewHarness: h, c: c, tasks: tasks}
	click("Start reconciliation")
	h.focused(&c.draft.selection.ownerPick.Trigger)
	start.activate(t, "Review")
	start.activate(t, "Cancel")
	h.focused(&c.draft.preview)
	sessions(app, 0)
	start.activate(t, "Cancel")
	h.focused(&c.start)
	sessions(app, 0)

	click("Start reconciliation")
	start.activate(t, "Review")
	if findPageLabel(&h.router, "Harbor Brokerage") == nil {
		t.Fatal("start review omitted the selected account")
	}
	h.focused(&c.draft.startSheet.close)
	start.activate(t, "Start reconciliation")
	sessions(app, 1)
	id := c.id
	a, item := holdingItem(c.session, ids.holding)
	if item.Value != "2" || item.Cost != "22" || item.Currency != "USD" || a.AccountID != ids.account.String() {
		t.Fatalf("unexpected starting evidence: %+v", item)
	}
	button := &c.itemButtons[a.AccountID+"/"+item.ID].cost
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
	if c.unresolved() || c.saved != i18n.HoldingReconSaved || len(changing.attempts) != 3 ||
		changing.attempts[1] != adjustment.planID || changing.attempts[2] != adjustment.planID {
		t.Fatalf("retry did not reuse the active plan: %+v", changing.attempts)
	}
	h.focused(button)
	assertEvidence(app, id, 1)
	_, corrected := holdingItem(c.session, ids.holding)
	if corrected.Value != "2" || corrected.Cost != "15.6234" {
		t.Fatalf("refreshed holding evidence: %+v", corrected)
	}
	click("Back to session list")
	h.focused(&c.start)
	click("Open holdings reconciliation · Harbor Brokerage")
	click("Back to session list")
	h.focused(tasks.reconciliationList.working[id])

	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
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

func TestTasksHoldingReconciliationQuantityTargetsMatchWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	clock := func() time.Time { return accountTestClock }
	testfixture.SetClock(app, clock)
	testfixture.SetClock(ref, clock)
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	start := func(api *reconciliation.Service, account domain.AccountID) domain.ReconciliationSession {
		t.Helper()
		plan, err := api.Preview(ctx, application.ReconciliationAction{Action: "start", AccountIDs: []string{account.String()}})
		if err != nil {
			t.Fatal(err)
		}
		result, err := api.Commit(ctx, plan.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	getItem := func(session domain.ReconciliationSession, holding domain.HoldingID) (domain.ReconciliationAccount, domain.ReconciliationItem) {
		t.Helper()
		for _, a := range session.Accounts {
			for _, item := range a.Items {
				if item.HoldingID == holding.String() {
					return a, item
				}
			}
		}
		t.Fatal("missing holding checkpoint")
		return domain.ReconciliationAccount{}, domain.ReconciliationItem{}
	}
	session, other := start(api, ids.account), start(refAPI, refIDs.account)
	fault := &lostHoldingReviewReceipt{holdingReconcileAPI: api}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: fault}
	tasks.holdingRecon = c
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(640, 680)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		for range 4 {
			h.frame()
			tasks.workers.Wait()
		}
	}
	click := func(label string) { t.Helper(); h.click(label, false); finish() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	linked := func(app *application.Service, id string, count int) []domain.ReconciliationLinkedChange {
		t.Helper()
		rows, err := app.ReconciliationLinkedChanges(ctx, id)
		if err != nil || len(rows) != count {
			t.Fatalf("expected %d links, got %+v / %v", count, rows, err)
		}
		return rows
	}

	finish()
	click("Tasks & checks")
	click("Reconciliation")
	click("Open holdings reconciliation · Harbor Brokerage")
	a, item := getItem(*c.session, ids.holding)
	button := &c.itemButtons[a.AccountID+"/"+item.ID].quantity
	click("Adjust holding quantity")
	d := c.draft
	h.focused(&d.target.quantity.Editor)
	if d.target.quantity.Text() != "2" {
		t.Fatal("target did not start at the checkpoint’s exact quantity")
	}
	setBankField(h, "Target quantity", &d.target.quantity, "2.125")
	click("Review")
	label("For added units, enter either their unit cost or the whole holding’s total cost.")
	setBankField(h, "Unit cost for added units (optional) · USD", &d.target.unitCost, "18.12345678")
	click("Review")
	label("Resulting quantity · 2.125")
	click("Cancel")
	h.focused(button)
	linked(app, session.ID, 0)

	for _, step := range []struct {
		start, end, delta, cost, total, direction string
		count                                     int
	}{{"2", "2.125", "0.125", "18.12345678", "", string(domain.EffectAdded), 1},
		{"2.125", "1.875", "0.25", "", "", string(domain.EffectRemoved), 2},
		{"1.875", "2", "0.125", "", "40", string(domain.EffectAdded), 4}} {
		click("Adjust holding quantity")
		d = c.draft
		if d.target.quantity.Text() != step.start {
			t.Fatal("next adjustment did not read back the saved quantity")
		}
		setBankField(h, "Target quantity", &d.target.quantity, step.end)
		if step.cost != "" {
			setBankField(h, "Unit cost for added units (optional) · USD", &d.target.unitCost, step.cost)
		}
		if step.total != "" {
			setBankField(h, "Whole holding total cost (optional) · USD", &d.total, step.total)
		}
		click("Review")
		label("Resulting quantity · " + step.end)
		if step.total != "" {
			label("New cost per unit · 20 USD")
			if len(d.review.changes) != 2 {
				t.Fatal("whole-holding cost and quantity were not reviewed together")
			}
		}
		r, tItem := getItem(other, refIDs.holding)
		want, err := refAPI.PreviewAdjustment(ctx, reconciliation.AdjustmentRequest{
			Link: domain.ReconciliationLink{SessionID: other.ID, AccountID: r.AccountID, ItemID: tItem.ID,
				ExpectedRevision: other.Revision, FinancialRevision: r.FinancialRevision},
			HoldingID: tItem.HoldingID, Currency: "USD", Quantity: step.end, UnitCost: step.cost, TotalCost: step.total,
		})
		if err != nil {
			t.Fatal(err)
		}
		got := d.review.changes[0].Effects[0]
		w := want.Changes[0].Effects[0]
		if *got.HoldingID != ids.holding.String() || got.Direction != step.direction ||
			got.Direction != w.Direction || *got.Quantity != *w.Quantity || *got.Quantity != step.delta ||
			!reflect.DeepEqual(got.CostUnitPrice, w.CostUnitPrice) {
			t.Fatalf("quantity effect differs from Wails: %+v / %+v", got, w)
		}
		if _, err := refAPI.CommitAdjustment(ctx, want.PlanID); err != nil {
			t.Fatal(err)
		}
		other, err = refAPI.Get(ctx, other.ID)
		if err != nil {
			t.Fatal(err)
		}
		if step.count == 1 {
			fault.loseNext = true
		}
		h.focused(&d.confirm)
		h.press(key.NameReturn)
		finish()
		if step.count == 1 {
			label("The save could not be confirmed. Retry the same review to check or finish posting.")
			h.press(key.NameReturn)
			finish()
			if len(fault.attempts) != 2 || fault.attempts[0] != fault.attempts[1] {
				t.Fatal("quantity retry did not reuse the same shared plan")
			}
		}
		label("Quantity adjustment recorded.")
		h.focused(button)
		_, savedItem := getItem(*c.session, ids.holding)
		_, refItem := getItem(other, refIDs.holding)
		if savedItem.Value != step.end || savedItem.Cost != refItem.Cost {
			t.Fatalf("unexpected quantity or per-unit cost: %+v / %+v", savedItem, refItem)
		}
		linked(ref, other.ID, step.count)
		for _, link := range linked(app, session.ID, step.count) {
			if link.ItemID != item.ID || link.AccountID != a.AccountID {
				t.Fatal("adjustment was linked to the wrong checkpoint")
			}
		}
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	persisted, err := reconciliation.NewService(reopened).Get(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, savedItem := getItem(persisted, ids.holding)
	if savedItem.Value != "2" || savedItem.Cost != "20" {
		t.Fatalf("combined adjustment was not persisted on the selected holding: %+v", savedItem)
	}
	linked(reopened, session.ID, 4)
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
}
