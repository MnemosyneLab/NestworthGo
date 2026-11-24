package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type fixInvocation struct {
	id                  domain.ActivityID
	command             any
	mutation, hash, key string
}

type undoInvocation struct {
	id                  domain.ActivityID
	at                  time.Time
	mutation, hash, key string
}

type amendmentFault struct {
	*application.Service
	fix              []fixInvocation
	undo             []undoInvocation
	started, release chan struct{}
	detailFailure    atomic.Bool
}

func (f *amendmentFault) RecordFixChangeGuarded(ctx context.Context, id domain.ActivityID, command any, mutation, hash, token string) (domain.ChangePreview, error) {
	f.fix = append(f.fix, fixInvocation{id, command, mutation, hash, token})
	if len(f.fix) == 2 {
		close(f.started)
		<-f.release
	}
	posted, err := f.Service.RecordFixChangeGuarded(ctx, id, command, mutation, hash, token)
	if err == nil {
		f.detailFailure.Store(true)
		return posted, errors.New("synthetic snapshot refresh failure after known commit")
	}
	return posted, err
}

func (f *amendmentFault) RecordUndoChangeGuardedAt(ctx context.Context, id domain.ActivityID, at time.Time, mutation, hash, token string) (domain.ChangePreview, error) {
	f.undo = append(f.undo, undoInvocation{id, at, mutation, hash, token})
	posted, err := f.Service.RecordUndoChangeGuardedAt(ctx, id, at, mutation, hash, token)
	if err == nil && len(f.undo) == 1 {
		return domain.ChangePreview{}, errors.New("synthetic lost reversal response")
	}
	return posted, err
}

func (f *amendmentFault) Activity(ctx context.Context, id domain.ActivityID) (domain.Activity, error) {
	if f.detailFailure.Swap(false) {
		return domain.Activity{}, errors.New("synthetic read failure after acknowledgement")
	}
	return f.Service.Activity(ctx, id)
}

func TestHistoryCashCorrectionAndReversalMatchWails(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seed, seedSession := openBankFixture(t, root)
	bank, deposit, withdrawal, transfer := seedAmendmentTimeline(t, seed)
	if err := seedSession.Close(); err != nil {
		t.Fatal(err)
	}
	reference, _ := openBankFixture(t, copyBankProfile(t, root))
	service, session := openBankFixture(t, root)
	fault := &amendmentFault{Service: service, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	page := newLedgerHistory(ctx, fault, func() time.Time { return accountTestClock }, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: page})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() {
		page.reads.workers.Wait()
		page.model.Wait()
	}
	t.Cleanup(func() { release(); wait() })
	finish := func() { h.frame(); wait(); h.frame(); wait(); h.frame() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	click := func(text string) { h.click(text, false); finish() }
	amount := func(want string) {
		t.Helper()
		got := readBankRecord(t, service, bank).LatestValue.Amount.CanonicalAmount()
		if got != want {
			t.Fatalf("balance = %s, want %s", got, want)
		}
	}
	activity := func(s *application.Service, id domain.ActivityID) domain.Activity {
		t.Helper()
		record, err := s.Activity(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	isCode := func(err error, code domain.ErrorCode) {
		t.Helper()
		var issue *domain.Error
		if !errors.As(err, &issue) || issue.Code != code {
			t.Fatalf("got %v, want %s", err, code)
		}
	}
	noActions := func() {
		t.Helper()
		for _, text := range []string{"Correct amount", "Reverse activity"} {
			if findPageLabel(&h.router, text) != nil {
				t.Fatalf("unsupported activity offered %s", text)
			}
		}
	}
	filter := func(title string) {
		t.Helper()
		h.click("Activity type", true)
		click(title)
	}

	h.frame()
	click("Ledger activity")
	filter("Money added")
	click("View")
	if page.reads.detail.id != deposit {
		t.Fatal("did not open the new deposit")
	}
	label("Amendable deposit")
	click("Correct amount")
	click("Preview correction")
	label("Enter a different amount.")
	form := page.reads.detail.amendment
	setBankField(h, "New amount · USD", &form.amount, "30")
	click("Preview correction")
	label("Replacement amount · 30 USD")
	click("Edit amount")
	setBankField(h, "New amount · USD", &form.amount, "40")
	click("Preview correction")
	label("Replacement amount · 40 USD")
	label("Balance at original time · 1774.13 USD")
	if _, err := service.CreateGroup(ctx, "Intervening synthetic change"); err != nil {
		t.Fatal(err)
	}
	click("Confirm correction")
	label("The ledger changed. Preview this action again.")
	amount("1746.13")
	if len(openingActivity(t, service, bank)) != 9 {
		t.Fatal("a stale correction wrote an activity")
	}
	click("Preview correction")
	label("Balance at original time · 1774.13 USD")
	pre := wire.FromReviewedChangePreview(form.review.Preview)
	adapter := historyapi.NewService(reference)
	request := historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyAdded, AccountID: bank.String(), Amount: "40", Currency: "USD",
		Reason: "contribution", Note: activity(service, deposit).Note}
	wantPreview, err := adapter.PreviewFixChange(ctx, deposit.String(), request)
	if err != nil || !reflect.DeepEqual(pre.Review, wantPreview.Review) || !reflect.DeepEqual(pre.Resulting, wantPreview.Resulting) {
		t.Fatalf("Gio preview differs from the Wails original-time preview: %+v / %+v, %v", pre, wantPreview, err)
	}
	bounds := semanticBounds(t, &h.router, "Confirm correction", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("correction confirmation did not start")
	}
	h.clickAt(point)
	h.click("Back", false)
	if page.reads.detail == nil || page.reads.detail.id != deposit || !page.amendmentUnresolved() {
		t.Fatal("navigation escaped the unresolved confirmation")
	}
	release()
	finish()
	label("Correction saved. Historical snapshots could not refresh.")
	label("Could not load this activity.")
	fixedID := page.reads.detail.id
	click("Retry")
	label("Contribution · 40 USD")
	label("Replacement record")
	label("View correction reversal")
	if len(fault.fix) != 2 {
		t.Fatal("a known correction was submitted again by a detail read")
	}
	amount("1761.13")
	click("View original")
	if page.reads.detail.id != deposit {
		t.Fatal("replacement link did not open the original")
	}
	label("Contribution · 25 USD")
	label("This record has been corrected.")
	noActions()
	click("View reversal")
	correctionReversal := page.reads.detail.id
	label("External inflow reversed")
	label("View replacement")
	noActions()
	click("View replacement")
	if page.reads.detail.id != fixedID {
		t.Fatal("correction reversal did not link to the saved replacement")
	}

	request.MutationID = fault.fix[1].mutation
	wailsFixed, err := adapter.FixChange(ctx, deposit.String(), request)
	if err != nil {
		t.Fatal(err)
	}
	wailsLinks, err := history.LoadDetail(ctx, reference, domain.ActivityID(wailsFixed.Activity.ID))
	if err != nil || wailsLinks.Links.Original != deposit {
		t.Fatalf("Wails correction did not link to its original: %+v, %v", wailsLinks.Links, err)
	}
	normalize := func(dto wire.ActivityDTO) wire.ActivityDTO {
		dto.ID, dto.CorrectionGroupID = "", nil
		for i := range dto.Effects {
			dto.Effects[i].ID, dto.Effects[i].ActivityID = "", ""
		}
		return dto
	}
	compare := func(gioID, wailsID domain.ActivityID) {
		t.Helper()
		got, want := activity(service, gioID), activity(reference, wailsID)
		if !reflect.DeepEqual(normalize(wire.FromActivity(got)), normalize(wire.FromActivity(want))) {
			t.Fatalf("Gio fact differs from Wails: %+v / %+v", got, want)
		}
	}
	compare(fixedID, domain.ActivityID(wailsFixed.Activity.ID))
	compare(correctionReversal, wailsLinks.Links.CorrectionReversal)

	click("Ledger activity")
	filter("Money removed")
	click("View")
	if page.reads.detail.id != withdrawal {
		t.Fatal("did not open the new withdrawal")
	}
	click("Reverse activity")
	label("Review reversal")
	label("Reversal amount · 9 USD")
	label("Balance after reversal · 1770.13 USD")
	click("Confirm reversal")
	label("Posting could not be confirmed. Retry the same submission to check its result.")
	click("Retry same submission")
	label("Reversal saved.")
	label("External outflow reversed")
	undoneID := page.reads.detail.id
	if len(fault.undo) != 2 || !reflect.DeepEqual(fault.undo[0], fault.undo[1]) {
		t.Fatal("reversal did not replay the exact reviewed intent")
	}
	click("View original")
	if page.reads.detail.id != withdrawal {
		t.Fatal("reversal did not link to the withdrawal")
	}
	label("This record has been reversed.")
	noActions()
	if findPageLabel(&h.router, "View replacement") != nil {
		t.Fatal("a standalone reversal incorrectly offered a replacement")
	}
	wailsUndo, err := adapter.UndoChange(ctx, withdrawal.String())
	if err != nil {
		t.Fatal(err)
	}
	compare(undoneID, domain.ActivityID(wailsUndo.Activity.ID))
	amount("1770.13")
	if got := readBankRecord(t, reference, bank).LatestValue.Amount.CanonicalAmount(); got != "1770.13" {
		t.Fatalf("Wails balance = %s, want 1770.13", got)
	}
	_, err = history.ReviewAmendment(ctx, service, history.CorrectCash, deposit, "75", accountTestClock)
	isCode(err, domain.ErrCannotFixChange)
	_, err = history.ReviewAmendment(ctx, service, history.ReverseCash, withdrawal, "", accountTestClock)
	isCode(err, domain.ErrAlreadyUndone)
	_, err = history.ReviewAmendment(ctx, service, history.CorrectCash, transfer, "5", accountTestClock)
	isCode(err, domain.ErrInvalidChange)
	_, err = history.ReviewAmendment(ctx, service, history.ReverseCash, domain.NewActivityID(), "", accountTestClock)
	isCode(err, domain.ErrNotFound)
	click("Ledger activity")
	filter("Cash transfer")
	click("View")
	noActions()
	if len(openingActivity(t, service, bank)) != 12 {
		t.Fatal("the correction and reversal did not append exactly three records")
	}

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	if saved, err := reopened.RecordFixChangeGuarded(ctx, deposit, fault.fix[1].command, fault.fix[1].mutation, fault.fix[1].hash, fault.fix[1].key); err != nil || saved.Activity.ID != fixedID {
		t.Fatalf("reopened correction receipt returned %s, %v", saved.Activity.ID, err)
	}
	u := fault.undo[0]
	if saved, err := reopened.RecordUndoChangeGuardedAt(ctx, withdrawal, u.at, u.mutation, u.hash, u.key); err != nil || saved.Activity.ID != undoneID {
		t.Fatalf("reopened reversal receipt returned %s, %v", saved.Activity.ID, err)
	}
	for _, app := range []*application.Service{reopened, reference} {
		app.SetClock(func() time.Time { return accountTestClock.Add(48 * time.Hour) })
		assets, err := app.AssetChange(ctx, domain.AnalysisQuery{
			Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-11-20", To: "2026-11-22",
			Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true,
		})
		if err != nil || assets.Status != domain.CompletenessOK {
			t.Fatalf("external-flow evidence unavailable: %+v, %v", assets.AnalysisAvailability, err)
		}
		var external string
		for _, row := range assets.Waterfall {
			if row.Bucket == domain.BucketExternalFlow && row.Amount != nil {
				external = row.Amount.CanonicalAmount()
			}
		}
		if external != "40" {
			t.Fatalf("external flow = %q, want USD 40 after replacing the original and offsetting the withdrawal", external)
		}
	}
}

func seedAmendmentTimeline(t *testing.T, service *application.Service) (bank domain.AccountID, deposit, withdrawal, transfer domain.ActivityID) {
	t.Helper()
	ctx := t.Context()
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(rows) != 1 || rows[0].Account.Name != "Harbor Checking" {
		t.Fatalf("unexpected synthetic sources: %+v, %v", rows, err)
	}
	bank = rows[0].Account.ID
	service.SetClock(func() time.Time { return accountTestClock.Add(-96 * time.Hour) })
	reserve, err := service.CreateAccount(ctx, application.AccountInput{
		Name: "Amendment reserve", AccountType: "cash_on_hand", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "USD", InitialAmount: "0", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInNetWorth: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	service.SetClock(func() time.Time { return accountTestClock })
	adapter := historyapi.NewService(service)
	record := func(request historyapi.ChangeCommandRequest) domain.ActivityID {
		t.Helper()
		posted, err := adapter.RecordChange(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		return domain.ActivityID(posted.Activity.ID)
	}
	depositNote, withdrawalNote := "Amendable deposit", "Withdrawal to reverse"
	deposit = record(historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyAdded, AccountID: bank.String(), Amount: "25", Currency: "USD",
		Reason: "contribution", EffectiveLocalDate: "2026-11-20", EffectiveLocalTime: "01:00", Note: &depositNote})
	withdrawal = record(historyapi.ChangeCommandRequest{Kind: historyapi.ChangeMoneyRemoved, AccountID: bank.String(), Amount: "9", Currency: "USD",
		Reason: "other", EffectiveLocalDate: "2026-11-21", EffectiveLocalTime: "01:00", Note: &withdrawalNote})
	transfer = record(historyapi.ChangeCommandRequest{Kind: historyapi.ChangeCashTransfer, FromAccountID: bank.String(), ToAccountID: reserve.Account.ID.String(),
		Sent: "4", SentCurrency: "USD", Received: "4", ReceivedCurrency: "USD", EffectiveLocalDate: "2026-11-21", EffectiveLocalTime: "02:00"})
	return
}
