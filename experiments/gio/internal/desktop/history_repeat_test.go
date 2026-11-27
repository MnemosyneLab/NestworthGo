package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type repeatTestAdapter struct {
	*historyapi.Service
	ledger         *cashMovementFault
	confirmations  []string
	started, allow chan struct{}
	lost           domain.ActivityID
}

type repeatDroppedPlan struct {
	*historyapi.Service
	newSession *historyapi.Service
}

func (s repeatDroppedPlan) ConfirmRepeatChange(ctx context.Context, id string) (wire.ChangePreviewDTO, error) {
	if _, err := s.Service.ConfirmRepeatChange(ctx, id); err != nil {
		return wire.ChangePreviewDTO{}, err
	}
	return s.newSession.ConfirmRepeatChange(ctx, id)
}

func (s *repeatTestAdapter) ConfirmRepeatChange(ctx context.Context, id string) (wire.ChangePreviewDTO, error) {
	s.confirmations = append(s.confirmations, id)
	attempt := len(s.confirmations)
	if attempt == 1 {
		close(s.started)
		<-s.allow
	}
	out, err := s.Service.ConfirmRepeatChange(ctx, id)
	if err == nil && attempt == 1 {
		s.lost = domain.ActivityID(out.Activity.ID)
		return wire.ChangePreviewDTO{}, errors.New("test: response discarded after saving repeat")
	}
	if err == nil && attempt == 2 {
		s.ledger.failDetail.Store(true)
	}
	return out, err
}

func TestHistoryRepeatPrefillMatchesWailsAndRetriesSamePlan(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, bank := seedBankBalanceFixture(t, true)
	ref, _, _, refBank := seedBankBalanceFixture(t, true)
	sources, refSources := seedRepeatActivities(t, app, bank), seedRepeatActivities(t, ref, refBank)
	ledger := &cashMovementFault{Service: app}
	fault := &repeatTestAdapter{Service: historyapi.NewService(app), ledger: ledger, started: make(chan struct{}), allow: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.allow) })
	t.Cleanup(release)
	now := accountTestClock.Add(1723 * time.Millisecond)
	setBalanceTargetClock(now.Sub(accountTestClock), app, ref)
	page, h, settle := repeatHarness(t, ledger, fault, now)
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }

	page.detailButton(domain.ActivityID(sources[0].ID)).Click()
	settle()
	click("Repeat as new draft")
	d := page.reads.detail.repeatDraft
	request := d.request()
	if request.Kind != historyapi.ChangeMoneyAdded || request.AccountID != bank.String() || request.Amount != "25" || request.Currency != "USD" ||
		request.Reason != "income" || request.Note == nil || *request.Note != "Synthetic consulting income" || request.MutationID != "" ||
		request.EffectiveAt != wire.FormatTime(now) || request.EffectiveLocalDate != "" || d.day != "2026-11-22" || d.minute != "01:00" {
		t.Fatalf("repeat copied or lost defaults: %+v, %s %s", request, d.day, d.minute)
	}
	click("Review new activity")
	label("Balance change · +25 USD")
	click("Back to draft")
	click("Cancel")
	if page.reads.detail.id.String() != sources[0].ID || page.reads.detail.repeatDraft != nil {
		t.Fatal("cancel did not return to the original activity")
	}
	assertRepeatActivityCount(t, app, 2)

	click("Repeat as new draft")
	d = page.reads.detail.repeatDraft
	setBankField(h, "Amount · USD", &d.amount, "17.25")
	setBankField(h, "Note (optional)", &d.note, "Synthetic repeat income")
	click("Review new activity")
	label("Effective: 2026-11-22 01:00:01.723 (Asia/Singapore)")
	label("Balance before · 120.5 USD")
	label("Balance change · +17.25 USD")
	label("Resulting balance · 137.75 USD")
	label("Synthetic repeat income")
	reference := historyapi.NewService(ref)
	refPlan := compareRepeatReview(t, reference, refSources[0].ID, refBank, d)
	bounds := semanticBounds(t, &h.router, "Record new activity", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("repeat confirmation did not start")
	}
	h.clickAt(point)
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: overviewRoute}}
	h.frame()
	if !page.repeatUnresolved() || h.shell.activeRoute() != historyRoute {
		t.Fatal("navigation escaped unresolved repeat")
	}
	release()
	settle()
	label("Saving could not be confirmed. Retry the same submission.")
	assertRepeatActivityCount(t, app, 3)
	click("Retry same submission")
	label("New activity recorded.")
	label("Could not load this activity.")
	click("Retry")
	label("Synthetic repeat income")
	if len(fault.confirmations) != 2 || fault.confirmations[0] != fault.confirmations[1] || page.reads.detail.id != fault.lost {
		t.Fatalf("duplicate or read retry wrote another plan: %v, %s", fault.confirmations, page.reads.detail.id)
	}
	wantIncome, err := reference.ConfirmRepeatChange(ctx, refPlan)
	if err != nil {
		t.Fatal(err)
	}
	assertNewRepeatMatches(t, app, sources[0], fault.lost, wantIncome.Activity)

	click("Back to ledger activity")
	page.detailButton(domain.ActivityID(sources[1].ID)).Click()
	settle()
	click("Repeat as new draft")
	d = page.reads.detail.repeatDraft
	if request := d.request(); request.Amount != "4.5" || request.Reason != "expense" || request.Kind != historyapi.ChangeMoneyRemoved {
		t.Fatalf("expense draft defaults: %+v", request)
	}
	h.click("Effective date", true)
	h.press(key.NameLeftArrow)
	h.press(key.NameReturn)
	settle()
	if request := d.request(); request.EffectiveAt != "" || request.EffectiveLocalDate != "2026-11-21" || request.EffectiveLocalTime != "01:00" {
		t.Fatalf("editing the date did not opt into manual ledger time: %+v", request)
	}
	click("Review new activity")
	label("Balance change · -4.5 USD")
	expenseRefPlan := compareRepeatReview(t, reference, refSources[1].ID, refBank, d)
	click("Record new activity")
	label("New activity recorded.")
	expenseID := page.reads.detail.id
	wantExpense, err := reference.ConfirmRepeatChange(ctx, expenseRefPlan)
	if err != nil {
		t.Fatal(err)
	}
	assertNewRepeatMatches(t, app, sources[1], expenseID, wantExpense.Activity)
	for _, side := range []struct {
		app *application.Service
		id  string
	}{{app, sources[1].ID}, {ref, refSources[1].ID}} {
		row, err := side.app.Activity(ctx, domain.ActivityID(side.id))
		if err != nil || len(row.Resulting) != 1 || row.Resulting[0].Amount != "116" {
			t.Fatalf("the earlier repeat should reproject the later USD 116 balance: %+v / %v", row.Resulting, err)
		}
	}
	assertBalanceTargetNetWorth(t, app, ref, "103.25", "30")
	assertRepeatActivityCount(t, app, 4)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	assertRepeatActivityCount(t, reopened, 4)
	assertNewRepeatMatches(t, reopened, sources[0], fault.lost, wantIncome.Activity)
	assertNewRepeatMatches(t, reopened, sources[1], expenseID, wantExpense.Activity)
	_, err = historyapi.NewService(reopened).ConfirmRepeatChange(ctx, fault.confirmations[0])
	if !repeatPlanUnavailable(err) {
		t.Fatalf("expected the original desktop plan to be unavailable after reopen: %v", err)
	}
	assertRepeatActivityCount(t, reopened, 4)
}

func TestHistoryRepeatRejectsSourceUndoneAfterPreview(t *testing.T) {
	ctx := t.Context()
	app, _, _, bank := seedBankBalanceFixture(t, true)
	sources := seedRepeatActivities(t, app, bank)
	adapter := historyapi.NewService(app)
	page, h, settle := repeatHarness(t, app, adapter, accountTestClock)
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	page.detailButton(domain.ActivityID(sources[0].ID)).Click()
	settle()
	click("Repeat as new draft")
	click("Review new activity")
	if _, err := adapter.UndoChange(ctx, sources[0].ID); err != nil {
		t.Fatal(err)
	}
	click("Record new activity")
	assertPageLabel(t, &h.router, "The ledger changed. Preview this cash movement again.")
	click("Review new activity")
	assertPageLabel(t, &h.router, "This source can no longer be repeated. Close the draft and choose another activity.")
	if findPageLabel(&h.router, "Review new activity") != nil || !page.reads.detail.repeatDraft.refused {
		t.Fatal("a refused source offered a write-ready form")
	}
	click("Cancel")
	if findPageLabel(&h.router, "Repeat as new draft") != nil {
		t.Fatal("the reversed source retained a Repeat action after refreshing")
	}
	assertRepeatActivityCount(t, app, 3)
}

func TestHistoryRepeatMissingPlanOpensReadOnlyHistory(t *testing.T) {
	app, _, _, bank := seedBankBalanceFixture(t, true)
	sources := seedRepeatActivities(t, app, bank)
	adapter := repeatDroppedPlan{Service: historyapi.NewService(app), newSession: historyapi.NewService(app)}
	page, h, settle := repeatHarness(t, app, adapter, accountTestClock)
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	page.detailButton(domain.ActivityID(sources[0].ID)).Click()
	settle()
	click("Repeat as new draft")
	click("Review new activity")
	click("Record new activity")
	assertPageLabel(t, &h.router, "The original confirmation is unavailable. Check Ledger activity for its result before starting a new draft.")
	for _, label := range []string{"Record new activity", "Back to draft", "Retry same submission"} {
		if findPageLabel(&h.router, label) != nil {
			t.Fatalf("the unavailable plan offered %q", label)
		}
	}
	click("Back to ledger activity")
	assertPageLabel(t, &h.router, "Loaded records: 3")
	assertRepeatActivityCount(t, app, 3)
}

func repeatHarness(t *testing.T, ledger historyService, adapter historyRepeatAPI, now time.Time) (*historyPage, *previewHarness, func()) {
	t.Helper()
	page := newLedgerHistory(t.Context(), ledger, func() time.Time { return now }, func() {})
	page.reads.repeatAPI = adapter
	h := newShellHarnessWithPages(t, ledgerPages{history: page})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.prefs.saved.DateFormat = settings.DateFormatISO
	settle := func() {
		h.frame()
		page.reads.workers.Wait()
		page.model.Wait()
		h.frame()
		page.reads.workers.Wait()
		page.model.Wait()
		h.frame()
	}
	h.frame()
	h.click("Ledger activity", false)
	settle()
	return page, h, settle
}

func seedRepeatActivities(t *testing.T, app *application.Service, bank domain.AccountID) [2]wire.ActivityDTO {
	t.Helper()
	adapter := historyapi.NewService(app)
	var result [2]wire.ActivityDTO
	for i, in := range []struct {
		kind                 historyapi.ChangeCommandKind
		amount, reason, note string
	}{
		{historyapi.ChangeMoneyAdded, "25", "income", "Synthetic consulting income"},
		{historyapi.ChangeMoneyRemoved, "4.5", "expense", "Synthetic bus fare"},
	} {
		posted, err := adapter.RecordChange(t.Context(), historyapi.ChangeCommandRequest{
			Kind: in.kind, AccountID: bank.String(), Currency: "USD", Amount: in.amount, Reason: in.reason, Note: &in.note,
			EffectiveAt: wire.FormatTime(accountTestClock.Add(time.Duration(i-2) * 12 * time.Hour)), MutationID: domain.NewMutationID().String(),
		})
		if err != nil {
			t.Fatal(err)
		}
		saved, err := app.Activity(t.Context(), domain.ActivityID(posted.Activity.ID))
		if err != nil {
			t.Fatal(err)
		}
		result[i] = wire.FromActivity(saved)
	}
	return result
}

func compareRepeatReview(t *testing.T, api *historyapi.Service, source string, account domain.AccountID, d *historyRepeatDraft) string {
	t.Helper()
	in := d.request()
	in.AccountID = account.String()
	want, err := api.PreviewRepeatChange(t.Context(), source, in)
	got := d.review
	if err != nil || got == nil || got.EffectiveAt != want.EffectiveAt || !reflect.DeepEqual(got.Review, want.Review) {
		t.Fatalf("repeat preview differs from Wails: %+v / %+v, %v", got, want, err)
	}
	return want.PlanID
}

func assertRepeatActivityCount(t *testing.T, app *application.Service, count int) {
	t.Helper()
	rows, err := app.ListActivities(t.Context(), 50)
	if err != nil || len(rows) != count {
		t.Fatalf("expected %d ledger activities, got %d: %v", count, len(rows), err)
	}
}

func assertNewRepeatMatches(t *testing.T, app *application.Service, original wire.ActivityDTO, saved domain.ActivityID, want wire.ActivityDTO) {
	t.Helper()
	old, err := app.Activity(t.Context(), domain.ActivityID(original.ID))
	actualOriginal := wire.FromActivity(old)
	// Inserting an earlier event can reproject balances. The original
	// activity and effects remain the immutable audit facts.
	actualOriginal.Resulting, original.Resulting = nil, nil
	if err != nil || !reflect.DeepEqual(actualOriginal, original) {
		t.Fatalf("repeat modified the original activity: %+v / %+v, %v", old, original, err)
	}
	row, err := app.Activity(t.Context(), saved)
	if err != nil {
		t.Fatal(err)
	}
	got := wire.FromActivity(row)
	if got.ID == original.ID || len(got.Effects) != 1 || len(want.Effects) != 1 || got.Effects[0].ID == original.Effects[0].ID ||
		got.ReversesActivityID != nil || got.CorrectionGroupID != nil || got.EffectiveAt == original.EffectiveAt {
		t.Fatalf("repeat carried the original identity or date: %+v", got)
	}
	a, b := got.Effects[0], want.Effects[0]
	if got.Kind != want.Kind || got.Reason != want.Reason || got.EffectiveAt != want.EffectiveAt || got.EffectiveLocalDate != want.EffectiveLocalDate ||
		!reflect.DeepEqual(got.Note, want.Note) || a.Direction != b.Direction || a.Classification != b.Classification || a.Target != b.Target ||
		!reflect.DeepEqual(a.AccountID, original.Effects[0].AccountID) || !reflect.DeepEqual(a.Money, b.Money) {
		t.Fatalf("repeat differs from Wails: %+v / %+v", got, want)
	}
}
