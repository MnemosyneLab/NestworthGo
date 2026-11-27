package desktop

import (
	"errors"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"reflect"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestCashMovementWithdrawalAndTransferMatchWails(t *testing.T) {
	ctx := t.Context()
	root := createFixtureRoot(t)
	seed, seedSession := openBankFixture(t, root)
	source, destination, _, archived := seedMovementAccounts(t, seed, false)
	if err := seedSession.Close(); err != nil {
		t.Fatal(err)
	}
	reference, _ := openBankFixture(t, copyBankProfile(t, root))
	service, session := openBankFixture(t, root)
	fault := &cashMovementFault{Service: service, loseAttempt: 3, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	c, timeline, h, finish := cashMovementHarness(t, fault)
	t.Cleanup(func() { release(); c.workers.Wait() })
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	historyAPI := historyapi.NewService(reference)
	wailsAccounts := account.NewService(reference)
	wailsPortfolio := portfolio.NewService(reference)
	const day = "2026-11-21"
	netWorth := func(want string) {
		t.Helper()
		got, err := service.Overview(ctx, domain.AccountFilter{})
		ref, refErr := wailsPortfolio.Overview(ctx, account.AccountFilterRequest{})
		if err != nil || refErr != nil || !got.Complete || !ref.Complete || got.NetWorth.String() != want || ref.NetWorth != want {
			t.Fatalf("Gio net worth %s, Wails %s, want %s, errors %v / %v", got.NetWorth, ref.NetWorth, want, err, refErr)
		}
	}
	netWorth("1934.13")
	balance := func(id domain.AccountID, want string) {
		t.Helper()
		got := readBankRecord(t, service, id)
		rows, err := wailsAccounts.ListAccounts(ctx, account.AccountFilterRequest{})
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range rows {
			if ref.Account.ID == id.String() && ref.LatestValue != nil && got.LatestValue != nil &&
				ref.LatestValue.Amount == wire.FromMoney(got.LatestValue.Amount) && ref.LatestValue.EffectiveAt == wire.FormatTime(got.LatestValue.EffectiveAt) &&
				got.LatestValue.Amount.CanonicalAmount() == want {
				return
			}
		}
		t.Fatalf("expected matching balance %s for %s, got %+v", want, id, got.LatestValue)
	}
	openDraft := func(action, field string) *cashMovementDraft {
		t.Helper()
		h.click("Record change", false)
		h.click(action, false)
		finish()
		pickAccountReference(h, field, "Harbor", "Harbor Checking · USD")
		h.click("Effective date", true)
		h.press(key.NameLeftArrow)
		h.press(key.NameReturn)
		h.click("Effective time", true)
		h.press(key.NameDownArrow)
		h.press(key.NameReturn)
		h.press(key.NameEscape)
		d := c.draft
		if d.day != day || d.time != "02:00" {
			t.Fatalf("unexpected test picker selection %s %s", d.day, d.time)
		}
		return d
	}
	// Give each transaction a distinct valid timestamp so the test checks
	// the before state without relying on a tie-break between posted records.
	tick := func(d *cashMovementDraft, steps int) {
		t.Helper()
		for range steps {
			h.click("Effective time", true)
			h.press(key.NameDownArrow)
			h.press(key.NameReturn)
			h.press(key.NameEscape)
		}
		if d.time != []string{"02:00", "03:00", "04:00"}[steps] {
			t.Fatalf("unexpected effective time %s", d.time)
		}
	}
	checkWails := func(got wire.ChangePreviewDTO, id domain.ActivityID, request historyapi.ChangeCommandRequest) domain.Activity {
		t.Helper()
		refPreview, err := historyAPI.PreviewChange(ctx, request)
		if err != nil || !reflect.DeepEqual(got.Review, refPreview.Review) || !reflect.DeepEqual(got.Resulting, refPreview.Resulting) {
			t.Fatalf("Review drift between Gio and Wails: %+v vs %+v, %v", got, refPreview, err)
		}
		ref, err := historyAPI.RecordChange(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		refActivity, err := historyAPI.Activity(ctx, ref.Activity.ID)
		if err != nil {
			t.Fatal(err)
		}
		activity, err := service.Activity(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		normalize := func(dto wire.ActivityDTO) wire.ActivityDTO {
			dto.ID = ""
			for i := range dto.Effects {
				dto.Effects[i].ID, dto.Effects[i].ActivityID = "", ""
			}
			return dto
		}
		if !reflect.DeepEqual(normalize(wire.FromActivity(activity)), normalize(refActivity)) {
			t.Fatalf("History drift between Gio and Wails: %+v vs %+v", activity, refActivity)
		}
		_, wantHash, err := request.MutationEnvelope()
		if err != nil || fault.attempts[len(fault.attempts)-1].hash != wantHash {
			t.Fatalf("Gio and reference envelope differ: %v", err)
		}
		return activity
	}
	effect := func(got domain.ActivityEffect, role domain.EffectRole, direction domain.EffectDirection, class domain.ActivityClassification, id domain.AccountID, amount string) {
		t.Helper()
		if got.Role != role || got.Direction != direction || got.Classification != class || got.Target != domain.EffectTargetAccountValue ||
			got.AccountID == nil || *got.AccountID != id || got.Money == nil || got.Money.Currency() != "USD" || got.Money.CanonicalAmount() != amount {
			t.Fatalf("unexpected effect %+v, want %s/%s/%s/%s/%s", got, role, direction, class, id, amount)
		}
	}

	d := openDraft("Record withdrawal", "Withdrawal account")
	setBankField(h, "Withdrawal amount", &d.amount, "2000")
	h.click("Preview withdrawal", false)
	finish()
	label("The account balance at the effective time cannot cover this amount.")
	setBankField(h, "Withdrawal amount", &d.amount, "25.125")
	h.click("Preview withdrawal", false)
	finish()
	label("Resulting balance · 1709.005 USD")
	withdrawalPreview := wire.FromReviewedChangePreview(d.review.Preview)
	h.click("Confirm withdrawal", false)
	finish()
	label("Withdrawal posted.")
	label("Harbor Checking")
	label("Withdrawal · 25.125 USD")
	withdrawalID := timeline.reads.detail.id
	withdraw := checkWails(withdrawalPreview, withdrawalID, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyRemoved, AccountID: source.String(), Amount: "25.125", Currency: "USD", Reason: "other",
		EffectiveLocalDate: day, EffectiveLocalTime: "02:00", MutationID: fault.attempts[0].mutation,
	})
	if withdraw.Kind != domain.ActivityCashOut || withdraw.Reason != domain.ReasonOther || len(withdraw.Effects) != 1 ||
		withdraw.EffectiveAt.Format(time.RFC3339) != "2026-11-20T18:00:00Z" {
		t.Fatalf("unexpected withdrawal %+v", withdraw)
	}
	effect(withdraw.Effects[0], domain.EffectRoleAmount, domain.EffectRemoved, domain.ClassificationExternalOutflow, source, "25.125")
	balance(source, "1709.005")
	netWorth("1909.005")
	h.click("Back to ledger activity", false)
	finish()

	d = openDraft("Record transfer", "From account")
	tick(d, 1)
	h.click("To account", true)
	label("Cash Reserve · USD")
	label("Euro Cash · EUR")
	if findPageLabel(&h.router, "Archived Cash · USD") != nil {
		t.Fatal("the archived destination was exposed")
	}
	h.press(key.NameEscape)
	for _, invalid := range []domain.AccountID{source, archived, domain.NewAccountID()} {
		_, err := history.ReviewCashMovement(ctx, service, history.CashMovementInput{
			Movement: history.CashTransfer, Account: source, Destination: invalid, Amount: "40", Date: day, Time: d.time,
		})
		var issue *domain.Error
		if !errors.As(err, &issue) || issue.Field != "toAccountId" {
			t.Fatalf("invalid destination %s reached a review: %v", invalid, err)
		}
	}
	setBankField(h, "Transfer amount", &d.amount, "40")
	h.click("Preview transfer", false)
	finish()
	label("Choose a different active cash account.")
	pickAccountReference(h, "To account", "Reserve", "Cash Reserve · USD")
	h.click("From account", true)
	h.router.Queue(key.EditEvent{Text: "Reserve"})
	h.frame()
	h.router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press, Modifiers: key.ModAlt})
	h.frame()
	h.press(key.NameReturn)
	if d.destination != "" {
		t.Fatal("changing the source retained the same account as the destination")
	}
	pickAccountReference(h, "From account", "Harbor", "Harbor Checking · USD")
	pickAccountReference(h, "To account", "Reserve", "Cash Reserve · USD")
	h.click("Preview transfer", false)
	finish()
	label("Transfer principal · 40 USD")
	freePreview := wire.FromReviewedChangePreview(d.review.Preview)
	h.click("Confirm transfer", false)
	finish()
	freeID := timeline.reads.detail.id
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeCashTransfer, FromAccountID: source.String(), ToAccountID: destination.String(),
		Sent: "40", Received: "40", SentCurrency: "USD", ReceivedCurrency: "USD", FeeCurrency: "USD",
		EffectiveLocalDate: day, EffectiveLocalTime: "03:00", MutationID: fault.attempts[1].mutation,
	}
	free := checkWails(freePreview, freeID, request)
	if len(free.Effects) != 2 {
		t.Fatalf("fee-free transfer produced %d effects", len(free.Effects))
	}
	effect(free.Effects[0], domain.EffectRoleTransferFrom, domain.EffectRemoved, domain.ClassificationInternalTransfer, source, "40")
	effect(free.Effects[1], domain.EffectRoleTransferTo, domain.EffectAdded, domain.ClassificationInternalTransfer, destination, "40")
	balance(source, "1669.005")
	balance(destination, "240")
	netWorth("1909.005")
	h.click("Back to ledger activity", false)
	finish()

	d = openDraft("Record transfer", "From account")
	tick(d, 2)
	pickAccountReference(h, "To account", "Reserve", "Cash Reserve · USD")
	setBankField(h, "Transfer amount", &d.amount, "30.25")
	setBankField(h, "Fee in source currency (optional)", &d.fee, "0")
	h.click("Preview transfer", false)
	finish()
	label("Leave the fee blank or enter a positive amount with up to four decimal places.")
	setBankField(h, "Fee in source currency (optional)", &d.fee, "1650")
	h.click("Preview transfer", false)
	finish()
	label("The account balance at the effective time cannot cover this amount.")
	setBankField(h, "Fee in source currency (optional)", &d.fee, "2")
	h.click("Preview transfer", false)
	finish()
	label("Fee · 2 USD")
	h.click("Back to details", false)
	finish()
	setBankField(h, "Fee in source currency (optional)", &d.fee, "1.125")
	h.click("Preview transfer", false)
	finish()
	label("Fee · 1.125 USD")
	label("From account · Harbor Checking")
	label("To account · Cash Reserve")
	label("Resulting balance · 1637.63 USD")
	label("Resulting balance · 270.25 USD")
	feePreview := wire.FromReviewedChangePreview(d.review.Preview)
	bounds := semanticBounds(t, &h.router, "Confirm transfer", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("transfer confirmation did not start")
	}
	h.clickAt(point)
	release()
	finish()
	label("Posting could not be confirmed. Retry the same submission to check its result.")
	h.click("Retry same submission", false)
	finish()
	label("Transfer posted.")
	label("Could not load this activity.")
	id := fault.firstActivity
	if len(fault.attempts) != 4 || timeline.reads.detail.id != id || !reflect.DeepEqual(fault.attempts[2], fault.attempts[3]) {
		t.Fatal("double-click or replay submitted another intent")
	}
	h.click("Retry", false)
	finish()
	label("Sent from Harbor Checking · 30.25 USD")
	label("Received by Cash Reserve · 30.25 USD")
	label("Fee paid by Harbor Checking · 1.125 USD")
	label("Cash Reserve balance after · 270.25 USD")
	if len(fault.attempts) != 4 {
		t.Fatal("detail retry resubmitted the transfer")
	}
	request.Sent, request.Received, request.Fee, request.EffectiveLocalTime, request.MutationID = "30.25", "30.25", "1.125", "04:00", fault.attempts[2].mutation
	paid := checkWails(feePreview, id, request)
	if len(paid.Effects) != 3 || paid.EffectiveLocalDate != day {
		t.Fatalf("unexpected fee transfer %+v", paid)
	}
	effect(paid.Effects[0], domain.EffectRoleTransferFrom, domain.EffectRemoved, domain.ClassificationInternalTransfer, source, "30.25")
	effect(paid.Effects[1], domain.EffectRoleTransferTo, domain.EffectAdded, domain.ClassificationInternalTransfer, destination, "30.25")
	effect(paid.Effects[2], domain.EffectRoleFee, domain.EffectRemoved, domain.ClassificationFee, source, "1.125")
	balance(source, "1637.63")
	balance(destination, "270.25")
	netWorth("1907.88")
	h.click("Back to ledger activity", false)
	finish()
	if timeline.model.Rows()[0].ID != id {
		t.Fatal("the transfer was not visible in refreshed History")
	}
	h.click("Overview", false)
	finish()
	label("US$1,907.88")

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := openBankFixture(t, root)
	for attempt, expectedID := range []domain.ActivityID{withdrawalID, freeID, id} {
		frozen := fault.attempts[attempt]
		replay, err := reopened.RecordChangeWithMutation(ctx, frozen.command, frozen.mutation, frozen.hash)
		if err != nil || replay.Activity.ID != expectedID {
			t.Fatalf("replay %d lost its activity after reopening: %+v, %v", attempt, replay.Activity, err)
		}
	}
	changed := fault.attempts[2]
	request.Fee = "2"
	changedCommand, err := request.ToCommand(paid.HouseholdID, "Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	_, hash, err := request.MutationEnvelope()
	if err != nil {
		t.Fatal(err)
	}
	var issue *domain.Error
	if _, err := reopened.RecordChangeWithMutation(ctx, changedCommand, changed.mutation, hash); !errors.As(err, &issue) || issue.Code != domain.ErrConflict || issue.Field != "mutationId" {
		t.Fatalf("same ID with a changed fee returned %v", err)
	}
	if rows := openingActivity(t, reopened, source); len(rows) != 9 || rows[0].ID != id || rows[1].ID != freeID || rows[2].ID != withdrawalID {
		t.Fatalf("restarting or replay duplicated or dropped an activity: %d", len(rows))
	}
}

func seedMovementAccounts(t *testing.T, service *application.Service, includeEuro bool) (source, destination, euro, archived domain.AccountID) {
	t.Helper()
	ctx := t.Context()
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	records, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range records {
		if row.Account.Name == "Harbor Checking" {
			source = row.Account.ID
		}
	}
	if source == "" {
		t.Fatal("synthetic Harbor Checking is missing")
	}
	testfixture.SetClock(service, func() time.Time { return accountTestClock.Add(-72 * time.Hour) })
	defer testfixture.SetClock(service, func() time.Time { return accountTestClock })
	create := func(name, currency, starting string, include bool) domain.AccountID {
		t.Helper()
		row, err := service.CreateAccount(ctx, application.AccountInput{Name: name,
			AccountType: "cash_on_hand", BalanceSheetRole: "asset", TrackingMode: "balance",
			DefaultCurrency: currency, InitialAmount: starting, IncludeInNetWorth: include, OwnerIDs: []domain.MemberID{boot.Members[0].ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		return row.Account.ID
	}
	destination = create("Cash Reserve", "USD", "200", true)
	euro = create("Euro Cash", "EUR", "42", includeEuro)
	archived = create("Archived Cash", "USD", "0", false)
	if err := service.ArchiveAccount(ctx, archived, true); err != nil {
		t.Fatal(err)
	}
	return
}
