package desktop

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestLiabilityStartingPointRestoresAccountAndContinuesPosting(t *testing.T) {
	ctx := t.Context()
	app, session, paths, bank := seedBankBalanceFixture(t, false)
	ref, _, _, _ := seedBankBalanceFixture(t, false)
	card, refCard := syntheticCardID(t, app), syntheticCardID(t, ref)
	fault := &startingPointFault{Service: app, loseReply: true}
	now := accountTestClock
	p, h, finish := liabilityPostingHarness(t, fault, "Synthetic Card", image.Pt(620, 720), func() time.Time { return now })
	zone, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	h.shell.prefs.savedLocation = zone
	h.shell.prefs.saved.Timezone = zone.String()
	click := func(text string) { h.click(text, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	adapter := historyapi.NewService(ref)

	fault.failedRead.Store(true)
	click("Record debt activity")
	label("Could not load account data.")
	if p.liabilityPosting.draft.starting != nil {
		t.Fatal("a failed origin read opened Starting Point")
	}
	click("Retry")
	label("Synthetic Card · The Starting Point applies to the whole household.")
	d := p.liabilityPosting.draft
	h.focused(&d.starting.calendar.Trigger)
	click("Review Starting Point")
	label("Harbor Checking · 100 USD")
	label("Synthetic Card · 30 USD")
	h.focused(&d.starting.confirm)
	click("Cancel")
	h.focused(&p.liabilityPosting.open)
	if origin, err := app.HistoryOrigin(ctx); err != nil || origin != nil || fault.writes.Load() != 0 {
		t.Fatalf("cancel wrote a Starting Point: %+v %v", origin, err)
	}
	assertBankObservations(t, session, card, 1, 0)
	click("Record debt activity")
	click("Review Starting Point")
	h.press(key.NameReturn)
	finish()
	label("Could not verify the Starting Point. Retry checks the saved state.")
	h.shell.pendingNavigation = &navigationRequest{step: -1}
	h.frame()
	finish()
	if p.target.account != card || !p.liabilityPostingUnresolved() {
		t.Fatal("an unresolved confirmation allowed navigation away from the account")
	}
	click("Retry")
	label("Starting Point saved.")
	d = p.liabilityPosting.draft
	h.focused(&d.amount.Editor)
	if d.account.ID != card || d.currency != "USD" || d.kind != historyapi.ChangeMoneyAdded || d.date != "2026-11-22" || fault.writes.Load() != 1 {
		t.Fatal("verification did not resume the original action and defaults")
	}
	assertBankObservations(t, session, card, 1, 0)
	origin, err := app.HistoryOrigin(ctx)
	want, wailsErr := adapter.StartHistoryOnDate(ctx, zone.String(), "2026-11-22", nil)
	if err != nil || wailsErr != nil || origin == nil || want.Timezone != origin.Timezone || want.StartedAt != wire.FormatTime(origin.StartedAt) {
		t.Fatalf("Starting Point differs from Wails: %+v / %+v, %v / %v", origin, want, err, wailsErr)
	}
	now = now.Add(time.Minute)
	setBalanceTargetClock(time.Minute, app, ref)
	click("Use current time")
	setBankField(h, "Amount", &d.amount, "1.2345")
	click("Preview change")
	label("Synthetic Card · -30 USD → -31.2345 USD")
	request := historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyAdded, AccountID: refCard.String(),
		Amount: "1.2345", Currency: "USD", Reason: "card_purchase", EffectiveAt: now.UTC().Format(time.RFC3339Nano)}
	compareLiabilityReview(t, d.review.Preview, adapter, request)
	click("Confirm posting")
	id := h.shell.historyPage.reads.detail.id
	assertLiabilityPosting(t, app, id, adapter, request)
	click("Back to account")
	h.focused(&p.liabilityPosting.open)
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, after := reopenOnboardingProfile(t, paths)
	assertBankObservations(t, after, card, 1, 1)
	savedOrigin, err := reopened.HistoryOrigin(ctx)
	if err != nil || savedOrigin == nil || savedOrigin.ID != origin.ID ||
		readBankRecord(t, reopened, bank).LatestValue.Amount.CanonicalAmount() != "100" ||
		readBankRecord(t, reopened, card).LatestValue.Amount.CanonicalAmount() != "31.2345" {
		t.Fatal("the scoped Starting Point, unchanged bank and independent posting did not survive reopen")
	}
	active, err := reopened.Activity(ctx, id)
	if err != nil || active.ID != id || active.Reason != domain.ReasonCardPurchase {
		t.Fatalf("posting did not survive reopen: %+v, %v", active, err)
	}
}
