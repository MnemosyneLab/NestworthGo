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
)

func TestLedgerCustodyTransferMatchesWailsAndRestoresScope(t *testing.T) {
	ctx := t.Context()
	app, session, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	addDestination := func(service *application.Service) domain.AccountID {
		t.Helper()
		boot, err := service.Bootstrap(ctx)
		if err != nil {
			t.Fatal(err)
		}
		row, err := service.CreateAccount(ctx, application.AccountInput{Name: "Secondary custody", AccountType: "brokerage",
			TrackingMode: "holdings", BalanceSheetRole: "asset", DefaultCurrency: "EUR", IncludeInPortfolio: true,
			OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		return row.Account.ID
	}
	target, refTarget := addDestination(app), addDestination(ref)
	adapter := historyapi.NewService(ref)
	now := accountTestClock
	clock := func() time.Time { return now }
	app.SetClock(clock)
	ref.SetClock(clock)
	fault := &cashMovementFault{Service: app, loseAttempt: 2, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.release) })
	ledger := newLedgerHistory(ctx, fault, clock, func() {})
	f := &accountInvestment{service: fault}
	ledger.investment = f
	accounts := newAccountsPage(ctx, app, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: ledger, accounts: accounts, version: app.MobileDataVersion})
	h.size = image.Pt(640, 800)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	t.Cleanup(func() { release(); ledger.reads.workers.Wait() })
	finish := func() {
		for range 5 {
			h.frame()
			accounts.workers.Wait()
			ledger.model.Wait()
			ledger.reads.workers.Wait()
		}
	}
	click := func(text string) { t.Helper(); h.click(text, false); finish() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	finish()
	click("Ledger activity")
	filter := history.Filters{Account: ids.account, Instrument: ids.instrument}
	ledger.model.SetFilters(filter)
	finish()

	open := func() *accountInvestmentDraft {
		t.Helper()
		click("Transfer position")
		d := f.draft
		h.focused(&d.custody.fromPicker.Trigger)
		if len(d.custody.sources.Accounts) != 2 {
			t.Fatal("wrong set of custody accounts")
		}
		h.click("Source holding", true)
		finish()
		for _, row := range d.custody.sources.Holdings {
			if row.Holding.ID == ids.holding {
				click(row.Instrument.SelectionLabel() + " · Harbor Brokerage · " + row.Holding.Quantity.Canonical())
				break
			}
		}
		h.click("Destination account", true)
		finish()
		if findPageLabel(&h.router, "Harbor Brokerage") != nil {
			t.Fatal("same account was offered as the destination")
		}
		click("Secondary custody")
		if d.custody.from != ids.holding || d.custody.to != target {
			t.Fatal("selectors lost exact source or destination")
		}
		return d
	}
	d := open()
	version := app.MobileDataVersion()
	setBankField(h, "Quantity to transfer", &d.quantity, "2.00000001")
	click("Review position transfer")
	label("This holding did not have enough quantity at the effective time.")
	click("Cancel")
	h.focused(&f.custodyOpen)
	if app.MobileDataVersion() != version || !ledger.model.Filters().Equal(filter) {
		t.Fatal("failed preview or cancel modified the ledger or its filter")
	}

	d = open()
	setBankField(h, "Quantity to transfer", &d.quantity, "0.625")
	setBankField(h, "Notes", &d.note, "  Synthetic transfer  ")
	click("Review position transfer")
	label("Source quantity after · 1.375")
	label("Destination quantity after · 0.625")
	label("This transfer does not move cash.")
	h.focused(&d.confirm)
	for _, item := range []struct {
		service *application.Service
		account domain.AccountID
	}{{app, ids.account}, {ref, refIDs.account}} {
		if _, err := historyapi.NewService(item.service).RecordChange(ctx, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyAdded, AccountID: item.account.String(), Amount: "1", Currency: "USD",
			Reason: "contribution", EffectiveAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatal(err)
		}
	}
	click("Confirm position transfer")
	label("The ledger changed. Review the transfer again before confirming.")
	if d.quantity.Text() != "0.625" || d.custody.from != ids.holding || d.custody.to != target {
		t.Fatal("stale review discarded the compatible transfer draft")
	}
	click("Review position transfer")
	note := "Synthetic transfer"
	request := historyapi.ChangeCommandRequest{Kind: historyapi.ChangePositionTransfer, FromHoldingID: refIDs.holding.String(),
		ToAccountID: refTarget.String(), Quantity: "0.625", EffectiveAt: now.UTC().Format(time.RFC3339Nano), Note: &note}
	wantPreview, err := adapter.PreviewChange(ctx, request)
	if err != nil || len(wantPreview.Resulting) != 2 ||
		wantPreview.Resulting[0].Quantity != d.review.Preview.Resulting[0].Quantity ||
		wantPreview.Resulting[1].Quantity != d.review.Preview.Resulting[1].Quantity {
		t.Fatalf("review differed from Wails: %+v / %v", wantPreview, err)
	}
	plan := *d.review
	h.press(key.NameReturn)
	select {
	case <-fault.started:
	case <-time.After(5 * time.Second):
		t.Fatal("custody confirmation did not start")
	}
	h.click("Back", false)
	if h.shell.activeRoute() != historyRoute || !f.unresolved() {
		t.Fatal("navigation dismissed pending confirmation")
	}
	release()
	finish()
	click("Retry same submission")
	if ledger.reads.detail.err == nil {
		t.Fatal("expected the simulated read failure")
	}
	click("Retry")
	id := ledger.reads.detail.id
	if id != fault.firstActivity || len(fault.attempts) != 3 || fault.attempts[1].mutation != fault.attempts[2].mutation {
		t.Fatal("retry did not retain the committed mutation")
	}
	label("Open account · Harbor Brokerage")
	label("Open account · Secondary custody")
	got, err := app.Activity(ctx, id)
	want, refErr := adapter.RecordChange(ctx, request)
	if err != nil || refErr != nil || got.Kind != domain.ActivityPositionTransfer || len(got.Effects) != 2 || len(want.Effects) != 2 ||
		!reflect.DeepEqual(got.Note, want.Activity.Note) || got.Effects[0].Quantity.Canonical() != *want.Effects[0].Quantity ||
		got.Effects[1].Quantity.Canonical() != *want.Effects[1].Quantity {
		t.Fatalf("recorded custody transfer differs from Wails: %+v / %+v / %v / %v", got, want, err, refErr)
	}
	destinationHolding := *got.Effects[1].HoldingID
	click("Back to ledger activity")
	h.focused(&f.custodyOpen)
	if !ledger.model.Filters().Equal(filter) {
		t.Fatal("receipt lost the account and instrument filters")
	}

	now = now.Add(time.Minute)
	d = open()
	setBankField(h, "Quantity to transfer", &d.quantity, "0.125")
	click("Review position transfer")
	label("Source quantity after · 1.25")
	label("Destination quantity after · 0.75")
	click("Confirm position transfer")
	secondID := ledger.reads.detail.id
	request.Quantity, request.EffectiveAt, request.Note = "0.125", now.UTC().Format(time.RFC3339Nano), nil
	assertLiabilityPosting(t, app, secondID, adapter, request)
	second, err := app.Activity(ctx, secondID)
	if err != nil || *second.Effects[1].HoldingID != destinationHolding {
		t.Fatalf("subsequent transfer duplicated the destination Holding: %+v / %v", second, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reopened.SetClock(clock)
	retry, err := plan.Commit(ctx, reopened)
	if err != nil || retry.Activity.ID != id {
		t.Fatalf("reopened receipt did not deduplicate: %+v / %v", retry, err)
	}
	costs := func(service *application.Service, instrument domain.InstrumentID, source, destination domain.AccountID) {
		t.Helper()
		rows, err := service.InstrumentHoldings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		actual := map[domain.AccountID][2]string{}
		for _, row := range rows {
			if row.InstrumentID == instrument {
				for _, item := range row.Holdings {
					if item.Amounts.TotalCost != nil {
						actual[item.AccountID] = [2]string{item.Amounts.Quantity, item.Amounts.TotalCost.Amount}
					}
				}
			}
		}
		if !reflect.DeepEqual(actual, map[domain.AccountID][2]string{source: {"1.25", "27.5"}, destination: {"0.75", "16.5"}}) {
			t.Fatalf("unexpected shared cost projection: %+v", actual)
		}
	}
	costs(reopened, ids.instrument, ids.account, target)
	costs(ref, refIDs.instrument, refIDs.account, refTarget)
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "157"})
	assertNativeTradeCash(t, reopened, target, map[domain.CurrencyCode]string{})
}

type unavailableHoldingReferences struct {
	*application.Service
	fail atomic.Bool
}

func (s *unavailableHoldingReferences) HoldingsByAccounts(ctx context.Context, ids []domain.AccountID) (map[domain.AccountID][]domain.Holding, error) {
	if s.fail.Swap(false) {
		return nil, errors.New("synthetic holding reference failure")
	}
	return s.Service.HoldingsByAccounts(ctx, ids)
}

func TestLedgerCollapsedCustodyNamesAndExactQuantity(t *testing.T) {
	ctx := t.Context()
	service, _, _, ids := openExistingBuyFixture(t)
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := service.CreateAccount(ctx, application.AccountInput{
		Name: "Secondary custody", AccountType: "brokerage", TrackingMode: "holdings",
		BalanceSheetRole: "asset", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	const quantity = "0.62543217"
	adapter := historyapi.NewService(service)
	moved, err := adapter.RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionTransfer, FromHoldingID: ids.holding.String(),
		ToAccountID: destination.Account.ID.String(), Quantity: quantity,
		EffectiveAt: accountTestClock.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveAccount(ctx, destination.Account.ID, true); err != nil {
		t.Fatal(err)
	}
	ref, err := adapter.Activity(ctx, moved.Activity.ID)
	if err != nil || len(ref.Effects) != 2 || ref.Effects[0].Quantity == nil || *ref.Effects[0].Quantity != quantity {
		t.Fatalf("Wails did not return the recorded transfer quantity: %+v, %v", ref.Effects, err)
	}

	reader := &unavailableHoldingReferences{Service: service}
	reader.fail.Store(true)
	page := newLedgerHistory(ctx, reader, func() time.Time { return accountTestClock }, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: page})
	h.size = image.Pt(1040, 820)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	t.Cleanup(page.reads.workers.Wait)
	wait := func() {
		for range 4 {
			h.frame()
			page.reads.workers.Wait()
			page.model.Wait()
		}
		h.frame()
	}
	h.frame()
	h.click("Ledger activity", false)
	wait()
	assertPageLabel(t, &h.router, "Investment transfer · 0.62543217 · QA-BUY")
	assertPageLabel(t, &h.router, "Account filter")

	h.click("Reload", false)
	wait()
	assertPageLabel(t, &h.router, "Investment transfer · 0.62543217 · QA-BUY · Harbor Brokerage → Secondary custody")
	if page.reads.rowActions != nil || page.reads.detail != nil {
		t.Fatal("custody names required opening the row")
	}
}
