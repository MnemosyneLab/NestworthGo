package desktop

import (
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"image"
	"reflect"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestCardPostingReviewCancellationStaleAndDurableRetry(t *testing.T) {
	ctx := t.Context()
	app, session, paths, bank := seedBankBalanceFixture(t, true)
	ref, _, _, refBank := seedBankBalanceFixture(t, true)
	card, refCard := syntheticCardID(t, app), syntheticCardID(t, ref)
	fault := &balanceTargetTestService{Service: app, started: make(chan struct{}), allow: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.allow) })
	t.Cleanup(release)
	now := accountTestClock.Add(37*time.Second + 425*time.Millisecond)
	testfixture.SetClock(app, func() time.Time { return now })
	testfixture.SetClock(ref, func() time.Time { return now })
	p, h, finish := liabilityPostingHarness(t, fault, "Synthetic Card", image.Pt(1120, 820), func() time.Time { return now })
	click := func(text string) { h.click(text, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	open := &p.liabilityPosting.open

	// Existing unrelated activity and value must survive all card operations.
	income := historyapi.NewService(app)
	unrelated, err := income.RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyAdded, AccountID: bank.String(), Amount: "5", Currency: "USD",
		Reason: "income", EffectiveAt: now.Add(-time.Hour).UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	unrelatedID := domain.ActivityID(unrelated.Activity.ID)
	initialUnrelated, err := app.Activity(ctx, unrelatedID)
	if err != nil {
		t.Fatal(err)
	}

	click("Record debt activity")
	d := p.liabilityPosting.draft
	h.focused(&d.amount.Editor)
	setBankField(h, "Amount", &d.amount, "12.34567")
	click("Preview change")
	label("Enter a positive amount with up to four decimal places.")
	setBankField(h, "Amount", &d.amount, "12.3456")
	setBankField(h, "Notes", &d.note, "  Stationery sample  ")
	click("Preview change")
	label("Synthetic Card · -30 USD → -42.3456 USD")
	h.focused(&d.confirm)
	click("Cancel")
	h.focused(open)
	assertBankObservations(t, session, card, 1, 1)

	click("Record debt activity")
	d = p.liabilityPosting.draft
	setBankField(h, "Amount", &d.amount, "12.3456")
	setBankField(h, "Notes", &d.note, "  Stationery sample  ")
	click("Preview change")
	// An intervening write invalidates the guard without applying this draft.
	adjusted := wailsBalanceAdjustment(card, "35", now.Add(-time.Minute))
	if _, err := income.RecordChange(ctx, adjusted); err != nil {
		t.Fatal(err)
	}
	adjusted.AccountID, adjusted.MutationID = refCard.String(), domain.NewMutationID().String()
	adapter := historyapi.NewService(ref)
	if _, err := adapter.RecordChange(ctx, adjusted); err != nil {
		t.Fatal(err)
	}
	click("Confirm posting")
	label("The ledger changed. Preview the change again.")
	h.focused(&d.amount.Editor)
	if d.amount.Text() != "12.3456" || d.note.Text() != "  Stationery sample  " {
		t.Fatal("stale confirmation lost the editable draft")
	}
	assertBankObservations(t, session, card, 1, 2)

	click("Preview change")
	label("Synthetic Card · -35 USD → -47.3456 USD")
	note := "Stationery sample"
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyAdded, AccountID: refCard.String(), Amount: "12.3456", Currency: "USD",
		Reason: "card_purchase", Note: &note, EffectiveAt: now.UTC().Format(time.RFC3339Nano),
	}
	compareLiabilityReview(t, d.review.Preview, adapter, request)
	plan := *d.review
	point := semanticBounds(t, &h.router, "Confirm posting", false).Min.Add(image.Pt(8, 8))
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(5 * time.Second):
		t.Fatal("confirmation did not start")
	}
	h.clickAt(point)
	h.shell.pendingNavigation = &navigationRequest{step: -1}
	h.frame()
	if !p.liabilityPostingUnresolved() || h.shell.activeRoute() != accountsRoute {
		t.Fatal("pending post allowed navigation")
	}
	release()
	finish()
	label("Saving could not be confirmed. Retry the same submission.")
	h.focused(&d.retryConfirm)
	assertBankObservations(t, session, card, 1, 3)
	click("Retry same submission")
	label("Card purchase")
	label("Change recorded.")
	click("Retry")
	label("Synthetic Card · Debt increased by 12.3456 USD")
	label("Amount owed after · 47.3456 USD")
	id := h.shell.historyPage.reads.detail.id
	assertLiabilityPosting(t, app, id, adapter, request)
	if len(fault.records) != 3 || fault.records[1] != fault.records[2] || fault.records[0] == fault.records[1] {
		t.Fatalf("confirmation did not preserve retry identity: %v", fault.records)
	}
	click("Back to account")
	h.focused(open)
	if p.target.account != card {
		t.Fatal("Back lost card scope")
	}

	now = now.Add(time.Minute)
	click("Record debt activity")
	d = p.liabilityPosting.draft
	click("Card refund")
	setBankField(h, "Amount", &d.amount, "99")
	click("Preview change")
	label("The account balance at the effective time cannot cover this amount.")
	setBankField(h, "Amount", &d.amount, "3.12")
	// An edited wall clock uses the History Origin zone and minute precision.
	d.date, d.time, d.exactTime = "2026-11-22", "01:01", time.Time{}
	click("Preview change")
	label("Synthetic Card · -47.3456 USD → -44.2256 USD")
	request.Kind, request.Reason, request.Amount, request.Note = historyapi.ChangeMoneyRemoved, "card_refund", "3.12", nil
	request.EffectiveAt, request.EffectiveLocalDate, request.EffectiveLocalTime = "", d.date, d.time
	compareLiabilityReview(t, d.review.Preview, adapter, request)
	h.focused(&d.confirm)
	h.press(key.NameReturn)
	finish()
	label("Card refund")
	assertLiabilityPosting(t, app, h.shell.historyPage.reads.detail.id, adapter, request)
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, after := reopenBaselineProfile(t, paths)
	testfixture.SetClock(reopened, func() time.Time { return now })
	replay, err := plan.Commit(ctx, reopened)
	if err != nil || replay.Activity.ID != id {
		t.Fatalf("reopen did not replay original receipt: %+v, %v", replay.Activity, err)
	}
	assertBankObservations(t, after, card, 1, 4)
	if readBankRecord(t, reopened, card).LatestValue.Amount.CanonicalAmount() != "44.2256" ||
		readBankRecord(t, ref, refCard).LatestValue.Amount.CanonicalAmount() != "44.2256" ||
		readBankRecord(t, reopened, bank).LatestValue.Amount.CanonicalAmount() != "105" ||
		readBankRecord(t, ref, refBank).LatestValue.Amount.CanonicalAmount() != "100" {
		t.Fatal("reopened balances differ from committed scope")
	}
	original, err := reopened.Activity(ctx, unrelatedID)
	if err != nil || !reflect.DeepEqual(wire.FromActivity(original), wire.FromActivity(initialUnrelated)) {
		t.Fatalf("unrelated activity changed: %+v, %v", original, err)
	}
}

func TestLiabilityDebtPostingCompactMatchesWails(t *testing.T) {
	app, session, _, bank := seedBankBalanceFixture(t, true)
	ref, _, _, refBank := seedBankBalanceFixture(t, true)
	card, refCard := syntheticCardID(t, app), syntheticCardID(t, ref)
	now := accountTestClock
	p, h, finish := liabilityPostingHarness(t, app, "Synthetic Card", image.Pt(620, 720), func() time.Time { return now })
	click := func(text string) { h.click(text, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	adapter := historyapi.NewService(ref)

	click("Record debt activity")
	click("Debt draw")
	d := p.liabilityPosting.draft
	h.click("Cash account", true)
	finish()
	click("Harbor Checking · USD")
	setBankField(h, "Debt principal · USD", &d.amount, "20.125")
	click("Preview change")
	if d.review == nil {
		t.Fatalf("debt draw was not reviewed (cash %q, amount %q, issue %v)", d.counterparty, d.amount.Text(), d.problem)
	}
	label("Synthetic Card · -30 USD → -50.125 USD")
	label("Harbor Checking · 100 USD → 120.125 USD")
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeDebtDraw, DebtAccountID: refCard.String(), CashAccountID: refBank.String(),
		Principal: "20.125", PrincipalCurrency: "USD", EffectiveAt: now.UTC().Format(time.RFC3339Nano),
	}
	compareLiabilityReview(t, d.review.Preview, adapter, request)
	click("Confirm posting")
	assertLiabilityPosting(t, app, h.shell.historyPage.reads.detail.id, adapter, request)
	click("Back to account")
	h.focused(&p.liabilityPosting.open)

	now = now.Add(time.Minute)
	setBalanceTargetClock(time.Minute, app, ref)
	click("Record debt activity")
	click("Debt payment")
	d = p.liabilityPosting.draft
	h.click("Cash account", true)
	finish()
	click("Harbor Checking · USD")
	setBankField(h, "Debt principal · USD", &d.amount, "10.25")
	setBankField(h, "Interest or fee (optional) · USD", &d.fee, "-1")
	click("Preview change")
	label("Enter a nonnegative fee with up to four decimal places.")
	setBankField(h, "Interest or fee (optional) · USD", &d.fee, "1.0001")
	click("Preview change")
	label("Interest or fee · 1.0001 USD")
	label("Synthetic Card · -50.125 USD → -39.875 USD")
	label("Harbor Checking · 120.125 USD → 108.8749 USD")
	request.Kind, request.Principal, request.EffectiveAt = historyapi.ChangeDebtPayment, "10.25", now.UTC().Format(time.RFC3339Nano)
	request.InterestOrFee, request.InterestOrFeeCurrency = "1.0001", "USD"
	compareLiabilityReview(t, d.review.Preview, adapter, request)
	click("Confirm posting")
	label("Debt payment")
	assertLiabilityPosting(t, app, h.shell.historyPage.reads.detail.id, adapter, request)
	assertBankObservations(t, session, card, 1, 2)
	if readBankRecord(t, app, bank).LatestValue.Amount.CanonicalAmount() != "108.8749" ||
		readBankRecord(t, app, card).LatestValue.Amount.CanonicalAmount() != "39.875" {
		t.Fatal("debt balance and settlement do not reconcile")
	}
}

type liabilityPostingTestLedger interface {
	accountsReader
	historyService
	history.CashMovementLedger
}

func liabilityPostingHarness(t *testing.T, ledger liabilityPostingTestLedger, name string, size image.Point, clock func() time.Time) (*accountsPage, *previewHarness, func()) {
	t.Helper()
	p := newAccountsPage(t.Context(), ledger, clock, func() {})
	p.liabilityPosting = &accountLiabilityPosting{service: ledger}
	timeline := newLedgerHistory(t.Context(), ledger, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p, history: timeline})
	h.size = size
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() {
		for range 4 {
			h.frame()
			p.workers.Wait()
			timeline.model.Wait()
			timeline.reads.workers.Wait()
		}
	}
	h.frame()
	h.click("Accounts", false)
	finish()
	h.click(name, false)
	finish()
	return p, h, finish
}

func compareLiabilityReview(t *testing.T, preview domain.ChangePreview, adapter *historyapi.Service, request historyapi.ChangeCommandRequest) {
	t.Helper()
	want, err := adapter.PreviewChange(t.Context(), request)
	got := wire.FromReviewedChangePreview(preview)
	if err != nil || !reflect.DeepEqual(got.Review, want.Review) || got.Activity.EffectiveAt != want.Activity.EffectiveAt ||
		got.Activity.EffectiveLocalDate != want.Activity.EffectiveLocalDate {
		t.Fatalf("Gio review differs from Wails: %+v / %+v, %v", got, want, err)
	}
}

func assertLiabilityPosting(t *testing.T, app *application.Service, id domain.ActivityID, adapter *historyapi.Service, request historyapi.ChangeCommandRequest) {
	t.Helper()
	want, err := adapter.RecordChange(t.Context(), request)
	saved, readErr := app.Activity(t.Context(), id)
	if err != nil || readErr != nil {
		t.Fatalf("recorded activity: %v / %v", err, readErr)
	}
	got := wire.FromActivity(saved)
	if got.Kind != want.Activity.Kind || got.Reason != want.Activity.Reason || got.EffectiveAt != want.Activity.EffectiveAt ||
		got.EffectiveLocalDate != want.Activity.EffectiveLocalDate || !reflect.DeepEqual(got.Note, want.Activity.Note) || len(got.Effects) != len(want.Effects) {
		t.Fatalf("activity differs from Wails: %+v / %+v", got, want.Activity)
	}
	for i, effect := range got.Effects {
		ref := want.Effects[i]
		if effect.Role != ref.Role || effect.Direction != ref.Direction || effect.Target != ref.Target ||
			effect.Classification != ref.Classification || !reflect.DeepEqual(effect.Money, ref.Money) {
			t.Fatalf("effect differs from Wails: %+v / %+v", effect, ref)
		}
	}
}
