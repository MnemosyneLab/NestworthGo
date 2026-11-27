package desktop

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestLedgerCashDefaultAndEditedTimePersistAndOrder(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	app, session := openBankFixture(t, root)
	now := accountTestClock.Add(34*time.Second + 851*time.Millisecond)
	clock := func() time.Time { return now }
	testfixture.SetClock(app, clock)
	cash, page, h, finish := cashMovementHarness(t, app)
	cash.clock = clock
	page.reads.clock = clock
	h.size = image.Pt(1280, 800)
	var account domain.AccountID
	var ids [2]domain.ActivityID
	for i := range ids {
		h.click("Record change", false)
		finish()
		d := cash.draft
		pickAccountReference(h, "Receiving account", "Harbor", "Harbor Checking · USD")
		account = d.account
		if d.day != "2026-11-22" || d.time != "01:00" {
			t.Fatalf("default civil time in Asia/Singapore = %s %s", d.day, d.time)
		}
		if i == 1 {
			// Selecting even the same civil date is an explicit minute-precision edit.
			h.click("Effective date", true)
			h.press(key.NameReturn)
			if !d.timeEdited {
				t.Fatal("explicit date selection was not recorded")
			}
		}
		before := len(openingActivity(t, app, account))
		setBankField(h, "Contribution amount", &d.amount, "125.4321")
		h.click("Preview contribution", false)
		finish()
		if d.review == nil || len(openingActivity(t, app, account)) != before {
			t.Fatal("preview did not complete without writing")
		}
		want := now
		if i == 1 {
			want = now.Truncate(time.Minute)
		}
		if got := d.review.Preview.Activity; !got.EffectiveAt.Equal(want) || got.EffectiveLocalDate != "2026-11-22" {
			t.Fatalf("review effective instant/date = %s/%s, want %s/2026-11-22", got.EffectiveAt, got.EffectiveLocalDate, want)
		}
		review := *d.review
		h.click("Confirm contribution", false)
		finish()
		ids[i] = page.reads.detail.id
		got, err := app.Activity(ctx, ids[i])
		if err != nil || !got.EffectiveAt.Equal(want) || got.EffectiveLocalDate != "2026-11-22" || len(openingActivity(t, app, account)) != before+1 {
			t.Fatalf("posted instant/date/count = %+v, %v", got, err)
		}
		retry, err := review.Commit(ctx, app)
		if err != nil || retry.Activity.ID != ids[i] || len(openingActivity(t, app, account)) != before+1 {
			t.Fatalf("retry did not return the original posting: %+v, %v", retry, err)
		}
		h.click("Back to ledger activity", false)
		finish()
	}
	rows, err := app.ListActivityPage(ctx, domain.ActivityQuery{AccountID: &account, Limit: 50})
	if err != nil || len(rows.Activities) < 2 || rows.Activities[0].ID != ids[0] || rows.Activities[1].ID != ids[1] {
		t.Fatalf("same-minute activities were not ordered by their exact instants: %+v, %v", rows, err)
	}
	if got := readBankRecord(t, app, account).LatestValue.Amount.CanonicalAmount(); got != "1984.9942" {
		t.Fatalf("exact balance after both contributions = %s", got)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	for i, id := range ids {
		got, err := reopened.Activity(ctx, id)
		want := now
		if i == 1 {
			want = now.Truncate(time.Minute)
		}
		if err != nil || !got.EffectiveAt.Equal(want) || got.EffectiveLocalDate != "2026-11-22" {
			t.Fatalf("reopened activity lost instant/date: %+v, %v", got, err)
		}
	}
}

func TestCashDraftCapturesInstantBeforeSourcesLoad(t *testing.T) {
	app, _ := openBankFixture(t, createFixtureRoot(t))
	now := accountTestClock.Add(34*time.Second + 851234567*time.Nanosecond)
	captured := now.Truncate(time.Millisecond)
	cash := newHistoryCashMovements(t.Context(), app, func() time.Time { return now }, func() {})
	cash.start(history.CashContribution)
	now = now.Add(2 * time.Minute)
	cash.workers.Wait()
	result := <-cash.draft.loading
	if result.err != nil {
		t.Fatal(result.err)
	}
	cash.draft.receiveSources(result.value, now, nil)
	if got := cash.draft.effectiveAt(); !got.Equal(captured) || cash.draft.day != "2026-11-22" || cash.draft.time != "01:00" {
		t.Fatalf("source loading changed the draft instant or local display: %s, %s %s", got, cash.draft.day, cash.draft.time)
	}
}

func TestLedgerRecordSheetPreservesFilterScrollAndFocus(t *testing.T) {
	app, _ := openBankFixture(t, createFixtureRoot(t))
	seedMixedTimeline(t, app)
	cash, timeline, h, finish := cashMovementHarness(t, app)
	h.size = image.Pt(1280, 800)
	filter := history.Filters{Kind: domain.ActivityCashIn}
	timeline.model.SetFilters(filter)
	finish()
	timeline.list.ScrollBy(7)
	h.frame()
	before := timeline.list.Position
	if before.First == 0 || len(timeline.model.Rows()) < 8 {
		t.Fatalf("fixture did not scroll the filtered ledger: %+v", before)
	}
	click := func(name string) {
		// The toolbar never needs revealing through the body scroll area.
		bounds := semanticBounds(t, &h.router, name, false)
		h.clickAt(bounds.Min.Add(bounds.Max).Div(2))
		finish()
	}
	if findPageLabel(&h.router, "Record contribution") != nil {
		t.Fatal("ledger exposes posting forms before the record trigger opens")
	}
	assertReturned := func() {
		t.Helper()
		if timeline.record.open || cash.draft != nil || !timeline.model.Filters().Equal(filter) || timeline.list.Position.First != before.First || timeline.list.Position.Offset != before.Offset {
			t.Fatalf("record closure changed the ledger visit or scroll: %+v / %+v", before, timeline.list.Position)
		}
		h.focused(&timeline.record.trigger)
	}
	click("Record change")
	h.focused(&timeline.record.sheet.close)
	if findPageLabel(&h.router, "Loaded records: 50") == nil {
		t.Fatal("opening the form removed the ledger context")
	}
	click("Cancel")
	assertReturned()
	click("Record change")
	h.press(key.NameEscape)
	finish()
	assertReturned()
	click("Record change")
	click("Close")
	assertReturned()
}

func TestLedgerDebtPaymentUsesWailsPreviewAndPosting(t *testing.T) {
	app, _, _, _ := seedBankBalanceFixture(t, true)
	ref, _, _, refBank := seedBankBalanceFixture(t, true)
	refCard := syntheticCardID(t, ref)
	cash, timeline, h, finish := cashMovementHarness(t, app)
	h.size = image.Pt(1280, 800)
	click := func(name string) { h.click(name, false); finish() }
	click("Record change")
	click("Debt payment")
	d := cash.draft
	if d == nil || d.movement != history.CashDebtPayment || d.account == "" {
		t.Fatal("ledger did not offer the supported debt editor and its eligible cash account")
	}
	h.click("Debt account", true)
	finish()
	click("Synthetic Card · USD")
	setBankField(h, "Debt principal · USD", &d.amount, "10.25")
	setBankField(h, "Interest or fee (optional) · USD", &d.fee, "1.0001")
	click("Preview change")
	if d.review == nil {
		t.Fatalf("standalone debt payment did not reach the shared preview: %v", d.problem)
	}
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeDebtPayment, DebtAccountID: refCard.String(), CashAccountID: refBank.String(),
		Principal: "10.25", PrincipalCurrency: "USD", InterestOrFee: "1.0001", InterestOrFeeCurrency: "USD",
		EffectiveLocalDate: d.day, EffectiveLocalTime: d.time,
	}
	adapter := historyapi.NewService(ref)
	compareLiabilityReview(t, d.review.Preview, adapter, request)
	click("Confirm posting")
	id := timeline.reads.detail.id
	if id == domain.ActivityID("") || timeline.record.open {
		t.Fatal("confirmed debt payment did not close the sheet and open its exact activity")
	}
	assertLiabilityPosting(t, app, id, adapter, request)
}
