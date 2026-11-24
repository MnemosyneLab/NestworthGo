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

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type cashMovementAttempt struct {
	command  any
	mutation string
	hash     string
	token    string
}

type cashMovementFault struct {
	*application.Service
	attempts         []cashMovementAttempt
	firstActivity    domain.ActivityID
	loseAttempt      int
	started, release chan struct{}
	failDetail       atomic.Bool
}

func (f *cashMovementFault) RecordChangeGuarded(ctx context.Context, command any, mutationID, hash, token string) (domain.ChangePreview, error) {
	f.attempts = append(f.attempts, cashMovementAttempt{command, mutationID, hash, token})
	if len(f.attempts) == f.loseAttempt {
		close(f.started)
		<-f.release
	}
	record, err := f.Service.RecordChangeGuarded(ctx, command, mutationID, hash, token)
	if err == nil && f.loseAttempt > 0 {
		if len(f.attempts) == f.loseAttempt {
			f.firstActivity = record.Activity.ID
			return domain.ChangePreview{}, errors.New("synthetic lost reply after commit")
		}
		if len(f.attempts) == f.loseAttempt+1 {
			f.failDetail.Store(true)
		}
	}
	return record, err
}

func (f *cashMovementFault) Activity(ctx context.Context, id domain.ActivityID) (domain.Activity, error) {
	if f.failDetail.Swap(false) {
		return domain.Activity{}, errors.New("synthetic detail read failure after acknowledged commit")
	}
	return f.Service.Activity(ctx, id)
}

func TestContributionReviewRetryAndHistoryMatchWails(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seed, first := openBankFixture(t, root)
	core, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.CreateAccount(ctx, application.AccountInput{
		Name: "Other sample cash", AccountType: "cash_on_hand", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "USD", InitialAmount: "0", OwnerIDs: []domain.MemberID{core.Members[0].ID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reference, _ := openBankFixture(t, copyBankProfile(t, root))
	service, session := openBankFixture(t, root)
	fault := &cashMovementFault{Service: service, loseAttempt: 1, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	c, timeline, h, finish := cashMovementHarness(t, fault)
	t.Cleanup(func() { release(); c.workers.Wait() })
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	h.click("Record contribution", false)
	finish()
	d := c.draft
	pickAccountReference(h, "Receiving account", "Harbor", "Harbor Checking · USD")
	selected := d.account
	if selected == "" {
		t.Fatal("the workflow did not bind an eligible account")
	}
	label("History timezone: Asia/Singapore")
	h.click("Effective date", true)
	h.press(key.NameLeftArrow)
	h.press(key.NameReturn)
	if d.day != "2026-11-21" || d.time != "01:00" {
		t.Fatalf("History wall-clock pair got %s %s", d.day, d.time)
	}
	setBankField(h, "Contribution amount", &d.amount, "0")
	h.click("Preview contribution", false)
	finish()
	label("Enter a positive amount with up to four decimal places.")
	setBankField(h, "Contribution amount", &d.amount, "50")
	h.click("Preview contribution", false)
	finish()
	label("Contribution · 50 USD")
	h.click("Cancel", false)
	finish()
	if c.draft != nil || len(fault.attempts) != 0 || len(openingActivity(t, service, selected)) != 6 {
		t.Fatal("canceling the preview created an activity")
	}
	h.click("Record contribution", false)
	finish()
	d = c.draft
	pickAccountReference(h, "Receiving account", "Harbor", "Harbor Checking · USD")
	h.click("Effective date", true)
	h.press(key.NameLeftArrow)
	h.press(key.NameReturn)
	setBankField(h, "Contribution amount", &d.amount, "50")
	h.click("Preview contribution", false)
	finish()
	label("Contribution · 50 USD")
	h.click("Back to details", false)
	finish()
	setBankField(h, "Contribution amount", &d.amount, "25")
	if d.review != nil {
		t.Fatal("editing left the previous review active")
	}
	h.click("Preview contribution", false)
	finish()
	label("Review contribution")
	label("Effective: 2026-11-21 01:00 (Asia/Singapore)")
	label("Balance before · 1734.13 USD")
	label("Contribution · 25 USD")
	label("Resulting balance · 1759.13 USD")
	preview := wire.FromReviewedChangePreview(d.review.Preview)
	bounds := semanticBounds(t, &h.router, "Confirm contribution", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("confirmation did not start")
	}
	h.clickAt(point)
	h.shell.pendingNavigation = &navigationRequest{target: navigationTarget{route: overviewRoute}}
	h.frame()
	if h.shell.activeRoute() != historyRoute || !c.writing() {
		t.Fatal("another action escaped while posting")
	}
	release()
	finish()
	label("Posting could not be confirmed. Retry the same submission to check its result.")
	if len(fault.attempts) != 1 || findPageLabel(&h.router, "Back to details") != nil || len(openingActivity(t, service, selected)) != 7 {
		t.Fatal("lost response allowed an edited submission or did not retain one posted activity")
	}
	h.click("Retry same submission", false)
	finish()
	label("Contribution posted.")
	label("Could not load this activity.")
	if len(fault.attempts) != 2 || !reflect.DeepEqual(fault.attempts[0], fault.attempts[1]) || c.detail.id != fault.firstActivity || c.draft != nil {
		t.Fatal("the retry did not send the same immutable request and recover its original ID")
	}
	h.click("Retry", false)
	finish()
	label("Cash contribution")
	label("Contribution · 25 USD")
	label("Resulting balance · 1759.13 USD")
	if len(fault.attempts) != 2 || findPageLabel(&h.router, "Retry same submission") != nil {
		t.Fatal("a known posted activity offered another write")
	}
	h.click("Back to ledger activity", false)
	finish()
	label("Cash contribution")
	if len(timeline.model.Rows()) != 7 || timeline.model.Rows()[0].ID != fault.firstActivity {
		t.Fatal("History did not reload from the first page")
	}
	h.click("View", false)
	finish()
	label("Contribution · 25 USD")
	h.click("Overview", false)
	finish()
	label("$ 1,759.13")
	h.click("Accounts", false)
	finish()
	h.click("Harbor Checking", false)
	finish()
	label("USD · $ 1,759.13")

	adapter := historyapi.NewService(reference)
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyAdded, AccountID: selected.String(), Amount: "25", Currency: "USD", Reason: "contribution",
		EffectiveLocalDate: "2026-11-21", EffectiveLocalTime: "01:00", MutationID: fault.attempts[0].mutation,
	}
	wantPreview, err := adapter.PreviewChange(ctx, request)
	if err != nil || !reflect.DeepEqual(preview.Review, wantPreview.Review) || !reflect.DeepEqual(preview.Resulting, wantPreview.Resulting) {
		t.Fatalf("Gio review differs from Wails: %+v vs %+v, %v", preview, wantPreview, err)
	}
	want, err := adapter.RecordChange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	wailsDetail, err := adapter.Activity(ctx, want.Activity.ID)
	if err != nil {
		t.Fatal(err)
	}
	id := fault.firstActivity
	actual, err := service.Activity(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	normalize := func(activity wire.ActivityDTO) wire.ActivityDTO {
		activity.ID = ""
		for i := range activity.Effects {
			activity.Effects[i].ID, activity.Effects[i].ActivityID = "", ""
		}
		return activity
	}
	if !reflect.DeepEqual(normalize(wire.FromActivity(actual)), normalize(wailsDetail)) ||
		actual.Kind != domain.ActivityCashIn || actual.Reason != domain.ReasonContribution ||
		actual.EffectiveLocalDate != "2026-11-21" || actual.EffectiveAt.Format(time.RFC3339) != "2026-11-20T17:00:00Z" ||
		len(actual.Effects) != 1 || len(actual.Resulting) != 1 {
		t.Fatalf("unexpected stored activity: %+v versus Wails %+v", actual, wailsDetail)
	}
	effect := actual.Effects[0]
	if effect.Target != domain.EffectTargetAccountValue || effect.Direction != domain.EffectAdded || effect.Classification != domain.ClassificationExternalInflow ||
		effect.Money == nil || effect.Money.CanonicalAmount() != "25" || effect.Money.Currency() != "USD" ||
		effect.AccountID == nil || *effect.AccountID != selected || actual.Resulting[0].Amount != "1759.13" {
		t.Fatalf("unexpected stored effect or balance: %+v", effect)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	mutation := fault.attempts[0]
	replayed, err := reopened.RecordChangeWithMutation(ctx, mutation.command, mutation.mutation, mutation.hash)
	if err != nil || replayed.Activity.ID != id {
		t.Fatalf("durable replay lost original ID: %s, %v", replayed.Activity.ID, err)
	}
	request.Amount = "50"
	command, err := request.ToCommand(core.Household.ID, "Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	key, differentHash, err := request.MutationEnvelope()
	if err != nil {
		t.Fatal(err)
	}
	var problem *domain.Error
	_, err = reopened.RecordChangeWithMutation(ctx, command, key, differentHash)
	if !errors.As(err, &problem) || problem.Code != domain.ErrConflict || problem.Field != "mutationId" {
		t.Fatalf("changed amount with the same ID returned %v", err)
	}
	activities := openingActivity(t, reopened, selected)
	current := readBankRecord(t, reopened, selected)
	if len(activities) != 7 || activities[0].ID != id || current.LatestValue.Amount.CanonicalAmount() != "1759.13" {
		t.Fatal("restart or replay changed the stored business activity or account balance")
	}
}

func TestContributionRequiresNewPreviewAndRejectsUnavailableAccount(t *testing.T) {
	ctx := t.Context()
	service, _ := openBankFixture(t, createFixtureRoot(t))
	fault := &cashMovementFault{Service: service}
	c, _, h, finish := cashMovementHarness(t, fault)
	h.click("Record contribution", false)
	finish()
	d := c.draft
	id := d.account
	setBankField(h, "Contribution amount", &d.amount, "25")
	if _, err := history.ReviewCashMovement(ctx, fault, history.CashMovementInput{Account: id, Amount: "25", Date: "2026-10-31", Time: "23:59"}); err == nil {
		t.Fatal("a timestamp before the History Origin was reviewed")
	}
	h.click("Preview contribution", false)
	finish()
	assertPageLabel(t, &h.router, "Review contribution")
	if err := service.ArchiveAccount(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	h.click("Confirm contribution", false)
	finish()
	assertPageLabel(t, &h.router, "The ledger changed. Preview the contribution again.")
	assertPageLabel(t, &h.router, "There are no active cash-balance accounts.")
	if d.review != nil || len(fault.attempts) != 1 || len(openingActivity(t, service, id)) != 6 {
		t.Fatal("stale confirmation posted or kept an old authorization")
	}
	if _, err := history.ReviewCashMovement(ctx, fault, history.CashMovementInput{Account: id, Amount: "25", Date: "2026-11-21", Time: "01:00"}); err == nil {
		t.Fatal("the authoritative preview selected an archived account")
	}
	h.click("Cancel", false)
	finish()
	if err := service.ArchiveAccount(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	h.click("Record contribution", false)
	finish()
	d = c.draft
	setBankField(h, "Contribution amount", &d.amount, "25")
	h.click("Preview contribution", false)
	finish()
	assertPageLabel(t, &h.router, "Review contribution")
	h.click("Confirm contribution", false)
	finish()
	assertPageLabel(t, &h.router, "Contribution posted.")
	if len(fault.attempts) != 2 || fault.attempts[0].mutation == fault.attempts[1].mutation || len(openingActivity(t, service, id)) != 7 {
		t.Fatal("the replacement review reused an abandoned mutation or produced duplicate activities")
	}
}

func TestContributionRequiresHistoryOrigin(t *testing.T) {
	root := createFixtureRoot(t)
	seedAccountFixture(t, root)
	service, _ := openBankFixture(t, root)
	fault := &cashMovementFault{Service: service}
	_, _, h, finish := cashMovementHarness(t, fault)
	h.click("Record contribution", false)
	finish()
	assertPageLabel(t, &h.router, "Start History before recording a contribution.")
	if findPageLabel(&h.router, "Preview contribution") != nil || len(fault.attempts) != 0 {
		t.Fatal("a ledger without History offered a write")
	}
}

func cashMovementHarness(t *testing.T, service *cashMovementFault) (*historyCashMovements, *historyPage, *previewHarness, func()) {
	t.Helper()
	ctx := t.Context()
	clock := func() time.Time { return accountTestClock }
	cashMovements := newHistoryCashMovements(ctx, service, clock, func() {})
	overview := newOverviewPage(ctx, service, clock, func() {})
	accounts := newAccountsPage(ctx, service, clock, func() {})
	timeline := newHistoryPage(history.New(ctx, service, func() {}))
	timeline.cashMovements = cashMovements
	wait := func() {
		cashMovements.workers.Wait()
		timeline.model.Wait()
		overview.workers.Wait()
		accounts.workers.Wait()
	}
	t.Cleanup(wait)
	h := newShellHarnessWithPages(t, ledgerPages{overview: overview, accounts: accounts, history: timeline})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	finish := func() { h.frame(); wait(); h.frame(); wait(); h.frame() }
	finish() // Cache the before-write Overview to exercise invalidation.
	h.click("Ledger activity", false)
	finish()
	return cashMovements, timeline, h, finish
}
