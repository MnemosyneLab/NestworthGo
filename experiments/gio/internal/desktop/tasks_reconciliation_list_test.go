package desktop

import (
	"image"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

func TestReconciliationSessionListMatchesWailsAndRestoresOlderPage(t *testing.T) {
	ctx := t.Context()
	seed, storage, paths, ids := openExistingBuyFixture(t)
	bootstrap, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := seed.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic foreign reserve", AccountType: "bank_account", TrackingMode: "balance", BalanceSheetRole: "asset",
		DefaultCurrency: "EUR", InitialAmount: "80.25", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := accountTestClock
	clock := func() time.Time { return now }
	seed.SetClock(clock)
	api := reconciliation.NewService(seed)
	complete := func(account domain.AccountID, partial bool) domain.ReconciliationSession {
		t.Helper()
		now = now.Add(time.Hour)
		session := startTestHoldingReconciliation(t, api, account)
		if !partial {
			for _, item := range session.Accounts[0].Items {
				session = commitStatementAction(t, api, application.ReconciliationAction{Action: "check", SessionID: session.ID,
					ExpectedRevision: session.Revision, AccountID: account.String(), FinancialRevision: session.Accounts[0].FinancialRevision,
					ItemID: item.ID, Outcome: "confirmed", Source: domain.ReconciliationSource{AsOfDate: "2026-11-22", Reference: "Synthetic saved evidence"}})
			}
		}
		return commitStatementAction(t, api, application.ReconciliationAction{Action: "finish", SessionID: session.ID,
			ExpectedRevision: session.Revision, AllowPartial: partial, Note: "Synthetic close"})
	}
	var completed []domain.ReconciliationSession
	for i := range 27 {
		completed = append(completed, complete(ids.account, i == 26))
	}
	outside := complete(foreign.Account.ID, false)
	now = now.Add(time.Hour)
	active := startTestHoldingReconciliation(t, api, ids.account)
	active = commitStatementAction(t, api, application.ReconciliationAction{Action: "pause", SessionID: active.ID, ExpectedRevision: active.Revision})
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(paths.RootPath(), "data", "nestworth.db"))
	if err != nil {
		t.Fatal(err)
	}
	copied := writeStage6Fixture(t, filepath.Join(t.TempDir(), "reconciliation-history"), raw)
	_, _, reference := openStage6Database(t, copied, clock)
	wails := reconciliation.NewService(reference)
	want, err := wails.List(ctx)
	if err != nil || len(want) != 29 || want[0].ID != active.ID || want[1].ID != outside.ID || !want[2].Partial {
		t.Fatalf("fixture does not contain its Wails session order: %d rows / %v", len(want), err)
	}
	last, err := wails.LastSuccess(ctx, ids.account.String())
	if err != nil || last == nil || last.ID != completed[25].ID {
		t.Fatalf("fixture must keep a newer partial session outside the last-success receipt: %+v / %v", last, err)
	}

	app, _ := reopenOnboardingProfile(t, paths)
	app.SetClock(clock)
	accounts := newAccountsPage(ctx, app, clock, func() {})
	tasks := newTasksPage(ctx, app, func() {})
	c := &holdingReconciliation{service: reconciliation.NewService(app), ledger: app, clock: clock}
	tasks.checks, tasks.holdingRecon = app, c
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(900, 600)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() {
		accounts.workers.Wait()
		tasks.workers.Wait()
	}
	t.Cleanup(wait)
	settle := func() {
		for range 4 {
			h.frame()
			wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	scroll := func(delta float32) {
		h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse,
			Position: f32.Pt(float32(h.size.X*3/4), float32(h.size.Y*3/4)), Scroll: f32.Pt(0, delta)})
		h.frame()
	}
	reveal := func(label string) {
		t.Helper()
		for range 50 {
			if node := findPageLabel(&h.router, label); node != nil {
				bounds := node.Desc.Bounds
				if bounds.Min.Y >= headerHeight+8 && bounds.Min.Y <= h.size.Y-24 {
					return
				}
			}
			scroll(140)
		}
		t.Fatalf("could not scroll to reconciliation control %q", label)
	}
	// The counter can enter the virtual viewport before the pager button.
	// Wait for the actual hit target to settle before sending its pointer click.
	clickPager := func(label, counter string) {
		t.Helper()
		viewport := image.Rect(0, headerHeight+8, h.size.X, h.size.Y)
		var previous image.Rectangle
		for range 50 {
			if findPageLabel(&h.router, label) != nil {
				bounds := semanticBounds(t, &h.router, label, false)
				if bounds.In(viewport) {
					if bounds == previous {
						assertPageLabel(t, &h.router, counter)
						h.clickAt(bounds.Min.Add(bounds.Max).Div(2))
						settle()
						return
					}
					previous = bounds
					h.frame()
					continue
				}
			}
			previous = image.Rectangle{}
			scroll(140)
		}
		t.Fatalf("reconciliation pager %q did not settle inside the viewport: %+v", label, tasks.list.Position)
	}
	open := func(target navigationTarget) {
		h.shell.pendingNavigation = &navigationRequest{target: target}
		settle()
	}

	open(navigationTarget{route: accountsRoute, account: ids.account})
	click("View last successful record")
	if tasks.reconciliationAccount != ids.account || c.session == nil || c.session.ID != last.ID {
		t.Fatal("account receipt opened a different session or account")
	}
	h.shell.pendingNavigation = &navigationRequest{step: -1}
	settle()
	h.focused(&accounts.detail.receipt)
	click("Open account reconciliation")
	list := &tasks.reconciliationList
	if tasks.reconciliationAccount != ids.account || list.filter != reconciliationCompleted || list.page != 0 {
		t.Fatal("fresh account-scoped entry did not start on completed sessions")
	}
	var gotIDs, wantIDs []string
	for _, row := range c.directory.sessions {
		gotIDs = append(gotIDs, row.ID)
	}
	for _, row := range want {
		wantIDs = append(wantIDs, row.ID)
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatal("Gio changed the shared session order")
	}
	reveal("Saved sessions and history")
	view := ui.View{Language: i18n.English, DateFormat: h.shell.prefs.saved.DateFormat,
		TimeFormat: h.shell.prefs.saved.TimeFormat, TimeLocation: h.shell.prefs.displayLocation()}
	partial := "Completed · Partially completed · " + statementRecordedTime(view, completed[26].StartedAt)
	reveal(partial)
	if c.sessionButtons[outside.ID] != nil || c.sessionButtons[completed[0].ID] != nil || c.sessionButtons[completed[2].ID] == nil {
		t.Fatal("the first page did not contain exactly the 25 scoped completed sessions")
	}
	clickPager("Next", "Page 1 of 2")
	if list.page != 1 || list.filter != reconciliationCompleted || c.sessionButtons[completed[0].ID] == nil {
		t.Fatalf("Next did not open the older completed page: page %d, filter %d, older row present %t",
			list.page+1, list.filter, c.sessionButtons[completed[0].ID] != nil)
	}

	older := completed[0]
	scroll(-200)
	scrolledFrom := tasks.list.Position
	rowLabel := "Completed · " + statementRecordedTime(view, older.StartedAt)
	scroll(110)
	reveal(rowLabel)
	oldPosition := tasks.list.Position
	if oldPosition.First == scrolledFrom.First && oldPosition.Offset == scrolledFrom.Offset {
		t.Fatal("the older row was opened without scrolling on page two")
	}
	button := c.sessionButtons[older.ID]
	h.router.Source().Execute(key.FocusCmd{Tag: button})
	h.frame()
	h.focused(button)
	h.press(key.NameReturn)
	settle()
	if c.session == nil || c.session.ID != older.ID {
		t.Fatal("the page-two button opened the wrong saved session")
	}
	click("Back to session list")
	h.focused(button)
	if list.page != 1 || list.filter != reconciliationCompleted || tasks.list.Position.First != oldPosition.First ||
		tasks.list.Position.Offset != oldPosition.Offset {
		t.Fatalf("Back lost the selected page, status or list position: %+v vs %+v", tasks.list.Position, oldPosition)
	}
	clickPager("Previous", "Page 2 of 2")
	if list.page != 0 {
		t.Fatal("Previous did not restore the first completed page")
	}
	click("In progress")
	assertPageLabel(t, &h.router, "No sessions match this filter.")
	click("Paused")
	if list.page != 0 || list.filter != reconciliationPaused || c.sessionButtons[active.ID] == nil || c.sessionButtons[active.ID] == list.working[active.ID] {
		t.Fatal("paused history did not use its own row beside the working shortcut")
	}
	click("All sessions")
	if list.filter != reconciliationAll || list.page != 0 || c.sessionButtons[outside.ID] != nil {
		t.Fatal("All sessions lost its account scope")
	}
	h.shell.pendingNavigation = &navigationRequest{step: -1}
	settle()
	h.focused(&accounts.detail.reconcile)
}
