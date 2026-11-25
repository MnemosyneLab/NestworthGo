package desktop

import (
	"context"
	"errors"
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

type linkedDatedAttempt struct {
	mutation, hash, token string
	link                  domain.ReconciliationLink
}

type linkedDatedFault struct {
	*application.Service
	loseNext bool
	attempts []linkedDatedAttempt
}

func (f *linkedDatedFault) RecordChangeGuarded(ctx context.Context, cmd any, mutation, hash, token string) (domain.ChangePreview, error) {
	link := domain.ReconciliationLinkFrom(ctx)
	if link != nil {
		f.attempts = append(f.attempts, linkedDatedAttempt{mutation, hash, token, *link})
	}
	result, err := f.Service.RecordChangeGuarded(ctx, cmd, mutation, hash, token)
	if err == nil && f.loseNext {
		f.loseNext = false
		return domain.ChangePreview{}, errors.New("synthetic lost dated posting acknowledgement")
	}
	return result, err
}

func startTestHoldingReconciliation(t *testing.T, api *reconciliation.Service, account domain.AccountID) domain.ReconciliationSession {
	t.Helper()
	preview, err := api.Preview(t.Context(), application.ReconciliationAction{Action: "start", AccountIDs: []string{account.String()}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := api.Commit(t.Context(), preview.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func testHoldingCheckpoint(t *testing.T, session domain.ReconciliationSession, id domain.HoldingID) (domain.ReconciliationAccount, domain.ReconciliationItem) {
	t.Helper()
	for _, account := range session.Accounts {
		for _, item := range account.Items {
			if item.HoldingID == id.String() {
				return account, item
			}
		}
	}
	t.Fatal("missing holding checkpoint")
	return domain.ReconciliationAccount{}, domain.ReconciliationItem{}
}

func TestHoldingReconciliationDatedHistoryHandoffMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	clock := func() time.Time { return accountTestClock }
	app.SetClock(clock)
	ref.SetClock(clock)
	api := reconciliation.NewService(app)
	session := startTestHoldingReconciliation(t, api, ids.account)
	wailsSession := startTestHoldingReconciliation(t, reconciliation.NewService(ref), refIDs.account)
	account, item := testHoldingCheckpoint(t, session, ids.holding)
	refAccount, refItem := testHoldingCheckpoint(t, wailsSession, refIDs.holding)
	fault := &linkedDatedFault{Service: app}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: api, ledger: fault, clock: clock}
	tasks.holdingRecon = c
	ledger := newLedgerHistory(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: ledger, tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(640, 840)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 5 {
			h.frame()
			tasks.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	click := func(s string) { t.Helper(); h.click(s, false); settle() }
	settle()
	click("Tasks & checks")
	click("Reconciliation")
	click("Open holdings reconciliation · Harbor Brokerage")
	button := &c.itemButtons[account.AccountID+"/"+item.ID].record
	click("Record dated position change")
	click("Adjust position")
	const day, clockText = "2026-11-21", "20:45"
	note := "Synthetic dated checkpoint change"
	fill := func() *accountInvestmentDraft {
		t.Helper()
		d := c.dated.draft
		h.focused(&d.quantity.Editor)
		if a := d.adjustment; a.holding != ids.holding || a.scope.account.ID != ids.account || len(a.sources.Holdings) != 1 {
			t.Fatal("dated posting is not locked to the originating holding")
		}
		d.day, d.time = day, clockText
		setBankField(h, "Quantity to adjust", &d.quantity, "0.125")
		setBankField(h, "Unit cost for added units · USD", &d.unitCost, "13.12345678")
		setBankField(h, "Notes", &d.note, note)
		return d
	}
	d := fill()
	click("Review position adjustment")
	h.focused(&d.confirm)
	click("Cancel")
	h.focused(button)
	if links, err := api.Links(ctx, session.ID); err != nil || len(links) != 0 || len(fault.attempts) != 0 {
		t.Fatalf("canceled preview wrote financial evidence: %+v / %v", links, err)
	}
	click("Record dated position change")
	click("Adjust position")
	d = fill()
	click("Review position adjustment")

	// A session change with no financial effects must also invalidate the link.
	sessionStep := func(action string, revision int64) domain.ReconciliationSession {
		t.Helper()
		plan, err := api.Preview(ctx, application.ReconciliationAction{Action: action, SessionID: session.ID, ExpectedRevision: revision})
		if err != nil {
			t.Fatal(err)
		}
		result, err := api.Commit(ctx, plan.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	paused := sessionStep("pause", session.Revision)
	resumed := sessionStep("resume", paused.Revision)
	h.press(key.NameReturn)
	settle()
	if d.review != nil || d.day != day || d.time != clockText || d.quantity.Text() != "0.125" ||
		c.session.Revision != resumed.Revision {
		t.Fatal("stale session preview lost its civil date, value, or working point")
	}
	click("Review position adjustment")
	want := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionAdjustment, HoldingID: refIDs.holding.String(), Added: true,
		Quantity: "0.125", UnitCost: "13.12345678", EffectiveLocalDate: day, EffectiveLocalTime: clockText, Note: &note,
		ReconciliationLink: &domain.ReconciliationLink{SessionID: wailsSession.ID, AccountID: refAccount.AccountID, ItemID: refItem.ID,
			ExpectedRevision: wailsSession.Revision, FinancialRevision: refAccount.FinancialRevision},
		MutationID: domain.NewMutationID().String(),
	}
	wails := historyapi.NewService(ref)
	wanted, err := wails.PreviewChange(ctx, want)
	wantedAt, timeErr := time.Parse(time.RFC3339Nano, wanted.Activity.EffectiveAt)
	if timeErr != nil || err != nil || len(wanted.Resulting) != 1 || wanted.Resulting[0].Quantity != "2.125" ||
		!d.review.Preview.Activity.EffectiveAt.Equal(wantedAt) {
		t.Fatalf("dated review differs from Wails: %+v / %v", wanted, err)
	}
	if _, err := wails.RecordChange(ctx, want); err != nil {
		t.Fatal(err)
	}
	fault.loseNext = true
	h.press(key.NameReturn)
	settle()
	if !c.unresolved() {
		t.Fatal("an ambiguous post did not retain its reviewed intent")
	}
	h.press(key.NameReturn)
	settle()
	h.focused(button)
	if len(fault.attempts) != 3 || !reflect.DeepEqual(fault.attempts[1], fault.attempts[2]) ||
		fault.attempts[2].link.ItemID != item.ID || fault.attempts[2].link.AccountID != account.AccountID {
		t.Fatalf("retry lost the reviewed dated identity: %+v", fault.attempts)
	}
	if len(c.links) != 1 || c.links[0].ItemID != item.ID {
		t.Fatalf("expected one linked ledger entry, got %+v", c.links)
	}
	id := domain.ActivityID(c.links[0].OperationID)
	click("View linked activity · 2026-11-21 · " + note)
	if ledger.reads.detail.id != id {
		t.Fatal("handoff read a different ledger activity")
	}
	click("Back to reconciliation")
	if c.id != session.ID {
		t.Fatal("Back discarded the originating reconciliation session")
	}
	h.focused(c.receiptButtons[id])
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	got, err := reopened.Activity(ctx, id)
	if err != nil || got.EffectiveLocalDate != day || !got.EffectiveAt.Equal(wantedAt) ||
		len(got.Effects) != 1 || *got.Effects[0].HoldingID != ids.holding || got.Effects[0].CostUnitPrice.Canonical() != "13.12345678" {
		t.Fatalf("reopened wrong dated activity: %+v / %v", got, err)
	}
	reopenedLinks, err := reconciliation.NewService(reopened).Links(ctx, session.ID)
	if err != nil || len(reopenedLinks) != 1 || reopenedLinks[0].OperationID != id.String() {
		t.Fatalf("reopen lost the exact activity link: %+v / %v", reopenedLinks, err)
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
}
