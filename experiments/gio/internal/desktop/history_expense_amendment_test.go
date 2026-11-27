package desktop

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

const bankExpenseHousehold = "Synthetic bank expense amendments"
const bankExpenseAccount = "Synthetic Expense Bank"
const bankExpenseCorrectNote = "Synthetic expense to correct"
const bankExpenseReverseNote = "Synthetic expense to reverse"

type bankExpenseFixture struct {
	bank                           domain.AccountID
	correct, reverse, linked, cash domain.ActivityID
}

func seedBankExpenseAmendments(t *testing.T, start time.Time) ([]byte, bankExpenseFixture) {
	t.Helper()
	ctx := t.Context()
	now := start
	db, _, app := openStage6Database(t, filepath.Join(t.TempDir(), "expense.db"), func() time.Time { return now })
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: bankExpenseHousehold, BaseCurrency: "USD", MemberNames: []string{"Sample owner"},
	}); err != nil {
		t.Fatal(err)
	}
	state, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, kind string) domain.AccountID {
		t.Helper()
		record, err := app.CreateAccount(ctx, application.AccountInput{
			Name: name, AccountType: kind, BalanceSheetRole: "asset", TrackingMode: "balance",
			DefaultCurrency: "USD", InitialAmount: "100", OwnerIDs: []domain.MemberID{state.Members[0].ID}, IncludeInNetWorth: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return record.Account.ID
	}
	bank := create(bankExpenseAccount, "bank_account")
	wallet := create("Synthetic Expense Wallet", "digital_wallet")
	if _, err := app.CaptureSyntheticOrigin(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	api := historyapi.NewService(app)
	record := func(account domain.AccountID, amount, note string, link *domain.ReconciliationLink) domain.ActivityID {
		t.Helper()
		now = now.Add(time.Hour + 11*time.Second)
		result, err := api.RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyRemoved, AccountID: account.String(), Amount: amount, Currency: "USD",
			Reason: string(domain.ReasonExpense), Note: &note, EffectiveAt: wire.FormatTime(now),
			MutationID: domain.NewMutationID().String(), ReconciliationLink: link,
		})
		if err != nil {
			t.Fatal(err)
		}
		return domain.ActivityID(result.Activity.ID)
	}
	correct := record(bank, "12.3456", bankExpenseCorrectNote, nil)
	reverse := record(bank, "4.5678", bankExpenseReverseNote, nil)
	cash := record(wallet, "2.1234", "Synthetic wallet expense", nil)
	session := commitStatementAction(t, reconciliation.NewService(app), application.ReconciliationAction{
		Action: "start", AccountIDs: []string{bank.String()},
	})
	linked := record(bank, "1.2345", "Synthetic journal-linked expense", &domain.ReconciliationLink{
		SessionID: session.ID, AccountID: bank.String(), ItemID: "balance",
		ExpectedRevision: session.Revision, FinancialRevision: session.Accounts[0].FinancialRevision,
	})
	return stage6SeedBytes(t, db), bankExpenseFixture{bank, correct, reverse, linked, cash}
}

func TestExportBankExpenseAmendmentFixtures(t *testing.T) {
	output := os.Getenv("NESTWORTH_STAGE6_QA_DIR")
	if output == "" {
		t.Skip("set NESTWORTH_STAGE6_QA_DIR to an existing disposable directory to export bank expense fixtures")
	}
	root, err := os.MkdirTemp(output, "nestworth-expense-amendment-")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -4).Add(9 * time.Hour)
	raw, ids := seedBankExpenseAmendments(t, start)
	for _, name := range []string{"source", "gio", "wails"} {
		writeStage6Fixture(t, filepath.Join(root, name), raw)
	}
	t.Logf("fixtures=%s household=%q bank=%q correct=%s reverse=%s linked=%s", root, bankExpenseHousehold, bankExpenseAccount, ids.correct, ids.reverse, ids.linked)
}

func TestHistoryStandaloneBankExpenseCorrectionAndUndoMatchWails(t *testing.T) {
	ctx := t.Context()
	clock := func() time.Time { return accountTestClock }
	raw, ids := seedBankExpenseAmendments(t, clock().AddDate(0, 0, -4))
	copyProfile := func(label string) *application.Service {
		t.Helper()
		path := writeStage6Fixture(t, filepath.Join(t.TempDir(), label), raw)
		_, _, app := openStage6Database(t, path, clock)
		return app
	}
	app, reference := copyProfile("gio"), copyProfile("wails")
	adapter := historyapi.NewService(reference)
	page, h, settle := repeatHarness(t, app, historyapi.NewService(app), clock())
	t.Cleanup(page.reads.workers.Wait)
	h.size = image.Pt(1040, 1120)
	settle()
	activate := func(id domain.ActivityID) {
		t.Helper()
		h.router.Source().Execute(key.FocusCmd{Tag: page.historyActionButton(id)})
		h.frame()
		h.press(key.NameReturn)
		settle()
		if menu := page.reads.rowActions; menu == nil || menu.id != id || menu.record == nil {
			t.Fatalf("did not open actions for %s", id)
		}
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	balance := func(svc *application.Service, want string, count int) {
		t.Helper()
		value := readBankRecord(t, svc, ids.bank).LatestValue.Amount
		if value.CanonicalAmount() != want || value.Currency().String() != "USD" || len(openingActivity(t, svc, ids.bank)) != count {
			t.Fatalf("bank balance/count = %s %s/%d, want %s USD/%d", value.CanonicalAmount(), value.Currency(), len(openingActivity(t, svc, ids.bank)), want, count)
		}
	}
	activity := func(svc *application.Service, id domain.ActivityID) domain.Activity {
		t.Helper()
		value, err := svc.Activity(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	compare := func(gioID, wailsID domain.ActivityID) {
		t.Helper()
		normalize := func(a domain.Activity) wire.ActivityDTO {
			a.ID, a.CorrectionGroupID = "", nil
			for i := range a.Effects {
				a.Effects[i].ID, a.Effects[i].ActivityID = "", ""
			}
			return wire.FromActivity(a)
		}
		got, want := normalize(activity(app, gioID)), normalize(activity(reference, wailsID))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Gio record differs from Wails: %+v / %+v", got, want)
		}
	}
	for _, id := range []domain.ActivityID{ids.linked, ids.cash} {
		detail, err := history.LoadDetail(ctx, app, id)
		if err != nil || detail.Amendable || detail.Reversible {
			t.Fatalf("an out-of-scope expense was eligible: %+v, %v", detail, err)
		}
		for _, action := range []history.AmendmentAction{history.CorrectActivity, history.ReverseActivity} {
			_, err := history.ReviewAmendment(ctx, app, action, id, history.AmendmentInput{Amount: "2"}, clock())
			var issue *domain.Error
			if !errors.As(err, &issue) || issue.Code != domain.ErrInvalidChange {
				t.Fatalf("out-of-scope %s review returned %v", action, err)
			}
		}
	}
	activate(ids.correct)
	click("Correct amount")
	form := page.reads.detail.amendment
	if page.reads.detail.id != ids.correct || form.amount.Text() != "12.3456" {
		t.Fatal("expense correction did not open with the original exact amount")
	}
	setBankField(h, "New amount · USD", &form.amount, "10.2345")
	click("Preview correction")
	label("Replacement amount · 10.2345 USD")
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyRemoved, AccountID: ids.bank.String(), Currency: "USD", Amount: "10.2345",
		Reason: string(domain.ReasonExpense), Note: activity(app, ids.correct).Note, MutationID: domain.NewMutationID().String(),
	}
	wantPreview, err := adapter.PreviewFixChange(ctx, ids.correct.String(), request)
	gotPreview := wire.FromReviewedChangePreview(form.review.Preview)
	if err != nil || !reflect.DeepEqual(gotPreview.Review, wantPreview.Review) || !reflect.DeepEqual(gotPreview.Resulting, wantPreview.Resulting) {
		t.Fatalf("Gio expense review differs from Wails: %+v / %+v, %v", gotPreview, wantPreview, err)
	}
	balance(app, "81.8521", 3)
	click("Confirm correction")
	label("Correction saved.")
	label(bankExpenseCorrectNote)
	gioFixed := page.reads.detail.id
	wailsFixed, err := adapter.FixChange(ctx, ids.correct.String(), request)
	if err != nil {
		t.Fatal(err)
	}
	compare(gioFixed, domain.ActivityID(wailsFixed.Activity.ID))
	corrected, original := activity(app, gioFixed), activity(app, ids.correct)
	if corrected.Reason != domain.ReasonExpense || corrected.Note == nil || *corrected.Note != bankExpenseCorrectNote ||
		!corrected.EffectiveAt.Equal(original.EffectiveAt) || corrected.EffectiveLocalDate != original.EffectiveLocalDate ||
		corrected.Effects[0].Classification != domain.ClassificationExternalOutflow {
		t.Fatalf("correction lost the expense's original business context: %+v", corrected)
	}
	gioReversal := page.reads.detail.record.Links.CorrectionReversal
	refLinks, err := history.LoadDetail(ctx, reference, domain.ActivityID(wailsFixed.Activity.ID))
	if err != nil || refLinks.Links.Original != ids.correct || page.reads.detail.record.Links.Original != ids.correct {
		t.Fatalf("correction did not link back to its original: %+v, %v", refLinks.Links, err)
	}
	compare(gioReversal, refLinks.Links.CorrectionReversal)
	inverse := activity(app, gioReversal)
	if inverse.ReversesActivityID == nil || *inverse.ReversesActivityID != ids.correct ||
		inverse.CorrectionGroupID == nil || corrected.CorrectionGroupID == nil || *inverse.CorrectionGroupID != *corrected.CorrectionGroupID ||
		!inverse.EffectiveAt.Equal(original.EffectiveAt) || inverse.Effects[0].Money.CanonicalAmount() != "12.3456" ||
		inverse.Effects[0].Direction != domain.EffectAdded {
		t.Fatalf("correction did not append the exact historical offset: %+v", inverse)
	}
	balance(app, "83.9632", 5)
	balance(reference, "83.9632", 5)

	click("Back to ledger activity")
	activate(ids.reverse)
	click("Reverse activity")
	label("Reversal amount · 4.5678 USD")
	click("Cancel")
	if page.reads.detail != nil {
		t.Fatal("cancel did not return to the expense list")
	}
	h.focused(page.historyActionButton(ids.reverse))
	balance(app, "83.9632", 5)
	activate(ids.reverse)
	click("Reverse activity")
	click("Confirm reversal")
	label("Reversal saved.")
	if record := page.reads.detail.record; record.Links.Original != ids.reverse || record.Links.Replacement != "" {
		t.Fatalf("standalone reversal readback lost the original: %+v", record.Links)
	}
	wailsUndo, err := adapter.UndoChange(ctx, ids.reverse.String())
	if err != nil {
		t.Fatal(err)
	}
	compare(page.reads.detail.id, domain.ActivityID(wailsUndo.Activity.ID))
	compare(ids.correct, ids.correct)
	compare(ids.reverse, ids.reverse)
	compare(ids.linked, ids.linked)
	balance(app, "88.531", 6)
	balance(reference, "88.531", 6)
}
