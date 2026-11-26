package desktop

import (
	"image"
	"path/filepath"
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

const stage6CardStatementHousehold = "Synthetic Stage 6 card statement"
const stage6CardStatementCard = "Synthetic Statement Card"

type cardStatementFixture struct {
	card    domain.AccountID
	session string
	receipt domain.ActivityID
}

func seedStage6CardStatement(t *testing.T, start time.Time) ([]byte, cardStatementFixture) {
	t.Helper()
	ctx := t.Context()
	now := start
	db, _, app := openStage6Database(t, filepath.Join(t.TempDir(), "card-statement.db"), func() time.Time { return now })
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: stage6CardStatementHousehold, BaseCurrency: "USD", MemberNames: []string{"Sample owner"},
	}); err != nil {
		t.Fatal(err)
	}
	state, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, kind, role, amount string) domain.AccountID {
		t.Helper()
		record, err := app.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: kind, BalanceSheetRole: role, TrackingMode: "balance",
			DefaultCurrency: "USD", InitialAmount: amount, OwnerIDs: []domain.MemberID{state.Members[0].ID}, IncludeInNetWorth: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return record.Account.ID
	}
	card := create(stage6CardStatementCard, "credit_card", "liability", "30")
	create("Synthetic Statement Bank", "bank_account", "asset", "100")
	if _, err := app.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	api := reconciliation.NewService(app)
	session := commitStatementAction(t, api, application.ReconciliationAction{Action: "start", AccountIDs: []string{card.String()}})
	now = now.Add(time.Hour)
	note := "Synthetic statement purchase"
	receipt, err := historyapi.NewService(app).RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyAdded, AccountID: card.String(), Currency: "USD", Amount: "12.3456",
		Reason: "card_purchase", Note: &note, EffectiveAt: now.Format(time.RFC3339Nano),
		MutationID: domain.NewMutationID().String(),
		ReconciliationLink: &domain.ReconciliationLink{
			SessionID: session.ID, AccountID: card.String(), ItemID: "balance",
			ExpectedRevision: session.Revision, FinancialRevision: session.Accounts[0].FinancialRevision,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return stage6SeedBytes(t, db), cardStatementFixture{card, session.ID, domain.ActivityID(receipt.Activity.ID)}
}

func TestCreditCardStatementHandoffReusesLedgerAndJournal(t *testing.T) {
	ctx := t.Context()
	clock := func() time.Time { return accountTestClock }
	raw, ids := seedStage6CardStatement(t, clock().AddDate(0, 0, -4))
	copyProfile := func() *application.Service {
		t.Helper()
		path := writeStage6Fixture(t, filepath.Join(t.TempDir(), "card"), raw)
		_, _, app := openStage6Database(t, path, clock)
		return app
	}
	app, reference := copyProfile(), copyProfile()
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(reference)
	wails := historyapi.NewService(reference)
	accounts := newAccountsPage(ctx, app, clock, func() {})
	accounts.statement = &accountStatement{service: api, origin: app}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	session := &holdingReconciliation{service: api, ledger: app, clock: clock}
	tasks.holdingRecon = session
	ledger := newLedgerHistory(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: accounts, tasks: tasks, history: ledger, version: app.MobileDataVersion})
	h.size = image.Pt(1040, 1120)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() {
		accounts.workers.Wait()
		tasks.workers.Wait()
		if session.cashDated != nil {
			session.cashDated.workers.Wait()
		}
		ledger.model.Wait()
		ledger.reads.workers.Wait()
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
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	openJournalReceipt := func(link domain.ReconciliationLinkedChange) {
		t.Helper()
		button := session.journalReceiptButton(journalReceiptKey(link))
		// Checkpoint rows also have a receipt link. Use the exact journal row.
		h.router.Source().Execute(key.FocusCmd{Tag: button})
		h.frame()
		h.press(key.NameReturn)
		settle()
		if ledger.reads.detail == nil || ledger.reads.detail.id.String() != link.OperationID {
			t.Fatalf("journal did not open exact activity %s", link.OperationID)
		}
		click("Back to reconciliation")
		h.focused(session.journalReceiptButton(journalReceiptKey(link)))
	}
	h.frame()
	click("Accounts")
	click(stage6CardStatementCard)
	click("Statement reconciliation")
	button := accounts.statement.screen.records[ids.session]
	if button == nil {
		t.Fatal("card statement list lost the seeded session")
	}
	h.router.Source().Execute(key.FocusCmd{Tag: button})
	h.frame()
	h.press(key.NameReturn)
	settle()
	label("Recorded amount owed · 42.3456 USD (balance -42.3456 USD)")
	click("Review checkpoints and journal")
	refSession, err := refAPI.Get(ctx, ids.session)
	if err != nil || h.shell.activeRoute() != tasksRoute || tasks.tab != tasksReconciliation ||
		session.id != ids.session || session.session == nil || !reflect.DeepEqual(*session.session, refSession) ||
		len(refSession.Accounts) != 1 || refSession.Accounts[0].AccountID != ids.card.String() {
		t.Fatalf("specialized entry did not open the exact shared card session: %+v / %v", session.session, err)
	}
	click("Linked changes")
	links, err := refAPI.Links(ctx, ids.session)
	if err != nil || len(links) != 1 || links[0].OperationID != ids.receipt.String() || !reflect.DeepEqual(links, session.links) {
		t.Fatalf("Gio and Wails lost the existing receipt: %+v / %+v / %v", links, session.links, err)
	}
	openJournalReceipt(links[0])

	const amount, note = "2.3456", "Synthetic statement desktop comparison"
	preview := func() *cashMovementDraft {
		t.Helper()
		click("Record a change")
		f := session.cashDated
		if f == nil || f.draft == nil || f.draft.account != ids.card || f.draft.reason != domain.ReasonCardPurchase ||
			f.linked.link().SessionID != ids.session || f.linked.link().ItemID != "balance" {
			t.Fatal("the shared record form did not lock the card balance checkpoint")
		}
		setBankField(h, "Amount · USD", &f.draft.amount, amount)
		setBankField(h, "Notes", &f.draft.note, note)
		click("Preview change")
		if f.draft.review == nil {
			t.Fatal("missing guarded card preview")
		}
		return f.draft
	}
	version := app.MobileDataVersion()
	preview()
	click("Cancel")
	h.focused(&session.checkpointActions(ids.card.String(), "balance").cash)
	if app.MobileDataVersion() != version || len(session.links) != 1 {
		t.Fatal("cancel wrote an activity or changed reconciliation")
	}

	draft := preview()
	noteValue := note
	req := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyAdded, AccountID: ids.card.String(), Currency: "USD", Amount: amount,
		Reason: "card_purchase", Note: &noteValue, EffectiveAt: clock().Format(time.RFC3339Nano),
		MutationID: domain.NewMutationID().String(),
		ReconciliationLink: &domain.ReconciliationLink{
			SessionID: ids.session, AccountID: ids.card.String(), ItemID: "balance",
			ExpectedRevision: refSession.Revision, FinancialRevision: refSession.Accounts[0].FinancialRevision,
		},
	}
	want, err := wails.PreviewChange(ctx, req)
	got := draft.review.Preview
	if err != nil || len(want.Resulting) != 1 || len(got.Resulting) != 1 ||
		want.Resulting[0].Amount != "44.6912" || got.Resulting[0].Amount != want.Resulting[0].Amount ||
		got.Resulting[0].Currency.String() != want.Resulting[0].Currency || !got.Activity.EffectiveAt.Equal(clock()) {
		t.Fatalf("linked card preview differs from Wails: %+v / %+v / %v", got, want, err)
	}
	h.focused(&draft.post)
	h.press(key.NameReturn)
	settle()
	referenceReceipt, err := wails.RecordChange(ctx, req)
	if err != nil || len(session.links) != 2 {
		t.Fatalf("expected one additional linked purchase: %+v / %v", session.links, err)
	}
	var newLink domain.ReconciliationLinkedChange
	for _, link := range session.links {
		if link.OperationID != ids.receipt.String() {
			newLink = link
		}
	}
	posted, err := app.Activity(ctx, domain.ActivityID(newLink.OperationID))
	recorded, refErr := reference.Activity(ctx, domain.ActivityID(referenceReceipt.Activity.ID))
	if err != nil || refErr != nil || posted.Reason != recorded.Reason || posted.Note == nil || *posted.Note != note ||
		newLink.AccountID != ids.card.String() || newLink.ItemID != "balance" || newLink.SessionID != ids.session ||
		len(posted.Effects) != 1 || len(recorded.Effects) != 1 ||
		posted.Effects[0].Money == nil || recorded.Effects[0].Money == nil ||
		posted.Effects[0].Money.Currency() != recorded.Effects[0].Money.Currency() ||
		posted.Effects[0].Money.CanonicalAmount() != recorded.Effects[0].Money.CanonicalAmount() || posted.Effects[0].Money.CanonicalAmount() != amount {
		t.Fatalf("linked card posting differs from Wails: %+v / %+v / %v / %v", posted, recorded, err, refErr)
	}
	assertRepeatActivityCount(t, app, 2)
	assertRepeatActivityCount(t, reference, 2)
	openJournalReceipt(newLink)
	click("Back")
	if h.shell.activeRoute() != accountsRoute || accounts.statement.screen == nil || accounts.statement.screen.selected != ids.session {
		t.Fatal("Back did not restore the selected statement")
	}
	h.focused(&accounts.statement.reconciliation)
	label("Recorded amount owed · 44.6912 USD (balance -44.6912 USD)")
}
