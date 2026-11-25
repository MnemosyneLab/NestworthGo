package desktop

import (
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

func TestReconciliationProductAndRemovedEvidence(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, products := openManagedProductFixture(t)
	clock := func() time.Time { return accountTestClock }
	api := reconciliation.NewService(app)
	security, err := app.CreateInstrument(ctx, application.InstrumentInput{Name: "Synthetic former holding", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := app.CreateHolding(ctx, application.HoldingInput{AccountID: products.account.String(), InstrumentID: security.ID.String(), Quantity: "0"})
	if err != nil {
		t.Fatal(err)
	}
	started := startTestHoldingReconciliation(t, api, products.account)
	account, original := testHoldingCheckpoint(t, started, holding.ID)
	const asOf = "2026-11-22"
	recorded, err := api.Preview(ctx, application.ReconciliationAction{Action: "check", SessionID: started.ID, AccountID: account.AccountID,
		ItemID: original.ID, ExpectedRevision: started.Revision, FinancialRevision: account.FinancialRevision, Outcome: "confirmed",
		Source: domain.ReconciliationSource{AsOfDate: asOf, Reference: "Synthetic former statement"}, Note: "Synthetic earlier evidence"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = api.Commit(ctx, recorded.PlanID); err != nil {
		t.Fatal(err)
	}
	if err := app.ArchiveHolding(ctx, holding.ID, true); err != nil {
		t.Fatal(err)
	}
	activities, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: api, ledger: app, clock: clock}
	tasks.holdingRecon = c
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(640, 2450)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 5 {
			h.frame()
			tasks.workers.Wait()
		}
	}
	click := func(s string) { t.Helper(); h.click(s, false); settle() }
	label := func(s string) { t.Helper(); assertPageLabel(t, &h.router, s) }
	settle()
	click("Tasks & checks")
	click("Reconciliation")
	click("Open holdings reconciliation · Harbor Deposit Bank")
	label("Removed from current holdings. Prior values and checks are retained.")
	label("Quantity 0")
	label("Principal · 1000 USD")
	label("Maturity date · 2026-12-20")
	label("Interest · Manual maturity amount")
	label("Maturity interest · 12.3456 USD")
	_, former := testHoldingCheckpoint(t, *c.session, holding.ID)
	if former.Kind != "removed" || !former.RecheckRequired || len(former.Attempts) != 1 ||
		former.Attempts[0].Kind != "holding" || former.Attempts[0].Note != "Synthetic earlier evidence" {
		t.Fatalf("wrong former checkpoint or overwritten attempt: %+v", former)
	}
	click("Show check attempts (1)")
	label("Synthetic former statement")
	label("Synthetic earlier evidence")

	version := app.MobileDataVersion()
	linkCount := len(c.links)
	productID := "product:" + products.ids["Winter Deposit"].String()
	productAction := &c.checkpointActions(account.AccountID, productID).check
	click("Check · Winter Deposit")
	label("Contract open")
	label("Maturity interest · 12.3456 USD")
	d := c.draft
	if d.itemID != productID || d.accountID != account.AccountID {
		t.Fatal("product check changed the selected item")
	}
	setBankField(h, "Notes", &d.note, "Synthetic terms matched")
	click("Review")
	label("Principal · 1000 USD")
	h.focused(&d.confirm)
	click("Cancel")
	h.focused(productAction)
	if app.MobileDataVersion() != version || len(c.links) != linkCount {
		t.Fatal("canceled product check changed the ledger")
	}

	click("Check · Winter Deposit")
	d = c.draft
	setBankField(h, "Notes", &d.note, "Synthetic terms matched")
	click("Review")
	h.press(key.NameReturn)
	settle()
	h.focused(productAction)

	removedAction := &c.checkpointActions(account.AccountID, former.ID).check
	click("Check · Synthetic former holding")
	d = c.draft
	label("Removed from current holdings. Prior values and checks are retained.")
	setBankField(h, "Notes", &d.note, "Synthetic removal reviewed")
	click("Review")
	h.press(key.NameReturn)
	settle()
	h.focused(removedAction)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	session, err := reconciliation.NewService(reopened).Get(ctx, started.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, preserved := testHoldingCheckpoint(t, session, holding.ID)
	if preserved.Kind != "removed" || len(preserved.Attempts) != 2 || preserved.Note != "Synthetic removal reviewed" ||
		preserved.Attempts[0].Note != "Synthetic earlier evidence" || preserved.Attempts[1].Note != preserved.Note {
		t.Fatalf("removed checkpoint did not retain both attempts across restart: %+v", preserved)
	}
	_, posted, found := checkpointInSession(&session, account.AccountID, productID, false)
	if !found || posted.Kind != "product" || posted.Outcome != "confirmed" || posted.Source.AsOfDate != asOf ||
		posted.Note != "Synthetic terms matched" || posted.Value != "1000" {
		t.Fatalf("product check did not persist: %+v", posted)
	}
	after, err := reopened.ListActivities(ctx, 100)
	if err != nil || !reflect.DeepEqual(activities, after) {
		t.Fatalf("evidence work changed financial activities: before %d, after %d / %v", len(activities), len(after), err)
	}
}
