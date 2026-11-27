package desktop

import (
	"context"
	"errors"
	"fmt"
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

type reconciliationJournalReadFault struct {
	holdingReconcileAPI
	failRecent bool
}

func (s *reconciliationJournalReadFault) Changes(ctx context.Context, id, account string) ([]domain.ReconciliationFinancialChange, error) {
	if s.failRecent {
		return nil, errors.New("synthetic recent journal read failure")
	}
	return s.holdingReconcileAPI.Changes(ctx, id, account)
}

func TestReconciliationJournalPagesAndReceiptBackMatchWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	clock := func() time.Time { return accountTestClock }
	app.SetClock(clock)
	api := reconciliation.NewService(app)
	wailsHistory := historyapi.NewService(app)
	act := func(in application.ReconciliationAction) domain.ReconciliationSession {
		t.Helper()
		plan, err := api.Preview(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		v, err := api.Commit(ctx, plan.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	first := startTestHoldingReconciliation(t, api, ids.account)
	asOf := domain.ReconciliationSource{AsOfDate: "2026-11-22", Reference: "Synthetic baseline"}
	for _, item := range first.Accounts[0].Items {
		first = act(application.ReconciliationAction{Action: "check", SessionID: first.ID, AccountID: ids.account.String(),
			ExpectedRevision: first.Revision, FinancialRevision: first.Accounts[0].FinancialRevision, ItemID: item.ID,
			Outcome: "confirmed", Source: asOf})
	}
	act(application.ReconciliationAction{Action: "finish", SessionID: first.ID, ExpectedRevision: first.Revision})
	session := startTestHoldingReconciliation(t, api, ids.account)
	noteByID := make(map[string]string)
	for i := range 26 {
		note := fmt.Sprintf("Synthetic journal entry %02d", i+1)
		in := historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyAdded, AccountID: ids.account.String(), Currency: "USD", Amount: "0.0001", Reason: "interest",
			Note: &note, EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: fmt.Sprintf("00:%02d", 30+i),
			MutationID: domain.NewMutationID().String(),
			ReconciliationLink: &domain.ReconciliationLink{SessionID: session.ID, AccountID: ids.account.String(), ItemID: "cash:USD",
				ExpectedRevision: session.Revision, FinancialRevision: session.Accounts[0].FinancialRevision},
		}
		posted, err := wailsHistory.RecordChange(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		noteByID[posted.Activity.ID] = note
		session, err = api.Get(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	session = act(application.ReconciliationAction{Action: "finish", SessionID: session.ID, ExpectedRevision: session.Revision,
		AllowPartial: true, Note: "Synthetic check left outstanding"})
	if _, err := wailsHistory.RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyAdded, AccountID: ids.account.String(), Currency: "USD", Amount: "0.1234", Reason: "income",
		EffectiveLocalDate: "2026-11-22", EffectiveLocalTime: "00:57", MutationID: domain.NewMutationID().String(),
	}); err != nil {
		t.Fatal(err)
	}
	wantLinks, err := api.Links(ctx, session.ID)
	if err != nil || len(wantLinks) != 26 {
		t.Fatalf("wanted 26 linked receipts: %d / %v", len(wantLinks), err)
	}
	wantRecent, err := api.Changes(ctx, session.ID, "")
	if err != nil || len(wantRecent) <= 25 || !wantRecent[len(wantRecent)-1].AfterCompletion || wantRecent[0].AfterCompletion {
		t.Fatalf("expected both intervals in the Wails journal: %d / %v", len(wantRecent), err)
	}
	version := app.MobileDataVersion()
	fault := &reconciliationJournalReadFault{holdingReconcileAPI: api, failRecent: true}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: fault, ledger: app, clock: clock}
	tasks.holdingRecon = c
	ledger := newLedgerHistory(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, history: ledger, version: app.MobileDataVersion})
	h.size = image.Pt(640, 480)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 5 {
			h.frame()
			tasks.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	click := func(s string) {
		t.Helper()
		bounds := semanticBounds(t, &h.router, s, false)
		if center := bounds.Min.Add(bounds.Max).Y / 2; center < 70 || center >= h.size.Y-20 {
			h.shell.stage.Position.Offset = max(0, h.shell.stage.Position.Offset+center-225)
			settle()
		}
		h.click(s, false)
		settle()
	}
	label := func(s string) { t.Helper(); assertPageLabel(t, &h.router, s) }
	settle()
	h.click("Tasks & checks", false)
	settle()
	click("Reconciliation")
	h.router.Source().Execute(key.FocusCmd{Tag: c.sessionButtons[session.ID]})
	h.press(key.NameReturn)
	settle()
	if !reflect.DeepEqual(wantLinks, c.links) || !c.changesFailed {
		t.Fatal("a recent journal failure suppressed the working session or linked evidence")
	}

	click("Recent changes")
	label("Could not load the changes.")
	fault.failRecent = false
	click("Retry")
	h.focused(&c.recentJournal.toggle)
	if !reflect.DeepEqual(wantRecent, c.changes) || !c.recentJournal.open {
		t.Fatal("Retry did not restore the same recent journal with Wails ordering")
	}
	label("1–25 of " + fmt.Sprint(len(wantRecent)))
	click("Next page")
	label(fmt.Sprintf("26–%d of %d", min(50, len(wantRecent)), len(wantRecent)))
	h.focused(&c.recentJournal.next)
	h.press(key.NameReturn)
	settle()
	label(fmt.Sprintf("51–%d of %d", len(wantRecent), len(wantRecent)))
	label("After completion")
	h.focused(&c.recentJournal.previous)
	click("Recent changes")

	click("Linked changes")
	label("1–25 of 26")
	label("Linked change 25 · Harbor Brokerage · USD cash")
	if findPageLabel(&h.router, "Linked change 26 · Harbor Brokerage · USD cash") != nil {
		t.Fatal("linked row 26 was painted on page one")
	}
	click("Next page")
	label("26–26 of 26")
	label("Linked change 26 · Harbor Brokerage · USD cash")
	h.focused(&c.linkedJournal.previous)
	last := wantLinks[25]
	id := domain.ActivityID(last.OperationID)
	button := c.journalReceiptButton(journalReceiptKey(last))
	// The last receipt also has a direct shortcut. Activate the journal's
	// stable button so Back is verified against the correct occurrence.
	h.router.Source().Execute(key.FocusCmd{Tag: button})
	h.press(key.NameReturn)
	settle()
	if ledger.reads.detail.id != id {
		t.Fatalf("journal page opened %v rather than %v", ledger.reads.detail.id, id)
	}
	label(noteByID[last.OperationID])
	click("Back to reconciliation")
	h.focused(c.journalReceiptButton(journalReceiptKey(last)))
	if c.linkedJournal.page != 1 || !c.linkedJournal.open || c.recentJournal.page != 2 ||
		c.recentJournal.open || h.shell.stage.Position.Offset == 0 {
		t.Fatal("Back lost a journal page, disclosure, or compact scroll position")
	}
	label("26–26 of 26")
	if app.MobileDataVersion() != version {
		t.Fatal("paging, retry and receipt inspection wrote to the ledger")
	}

	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	again := reconciliation.NewService(reopened)
	links, err := again.Links(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(links, wantLinks) {
		t.Fatalf("linked order or identity changed across reopen: %v", err)
	}
	changes, err := again.Changes(ctx, session.ID, "")
	if err != nil || !reflect.DeepEqual(changes, wantRecent) {
		t.Fatalf("recent order or identity changed across reopen: %v", err)
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156.126"})
}
