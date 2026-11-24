package desktop

import (
	"context"
	"errors"
	"image"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type ledgerRefreshProbe struct {
	*application.Service
	blockOverview, failActivity, failAccount atomic.Bool
	staleRead, releaseStale                  chan struct{}
	writes                                   atomic.Int32
	mu                                       sync.Mutex
	versions                                 map[string]string
}

func (s *ledgerRefreshProbe) noteRead(read string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versions[read] = s.MobileDataVersion()
}

func (s *ledgerRefreshProbe) Overview(ctx context.Context, filter domain.AccountFilter) (domain.OverviewResult, error) {
	value, err := s.Service.Overview(ctx, filter)
	s.noteRead("overview")
	if s.blockOverview.Swap(false) {
		close(s.staleRead)
		<-s.releaseStale
	}
	return value, err
}

func (s *ledgerRefreshProbe) Activity(ctx context.Context, id domain.ActivityID) (domain.Activity, error) {
	if s.failActivity.Swap(false) {
		return domain.Activity{}, errors.New("test: detail unavailable after acknowledged post")
	}
	value, err := s.Service.Activity(ctx, id)
	s.noteRead("activity")
	return value, err
}

func (s *ledgerRefreshProbe) ListActivityPage(ctx context.Context, query domain.ActivityQuery) (domain.ActivityPage, error) {
	value, err := s.Service.ListActivityPage(ctx, query)
	s.noteRead("timeline")
	return value, err
}

func (s *ledgerRefreshProbe) ListAccounts(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountRecord, error) {
	value, err := s.Service.ListAccounts(ctx, filter)
	if len(filter.AccountIDs) == 1 {
		s.noteRead("account")
	}
	return value, err
}

func (s *ledgerRefreshProbe) AccountValuations(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountValuation, error) {
	if len(filter.AccountIDs) == 1 && s.failAccount.Swap(false) {
		return nil, errors.New("test: valuation unavailable on first jump")
	}
	value, err := s.Service.AccountValuations(ctx, filter)
	if len(filter.AccountIDs) == 1 {
		s.noteRead("account valuation")
	}
	return value, err
}

func (s *ledgerRefreshProbe) RecordChangeGuarded(ctx context.Context, command any, mutation, hash, token string) (domain.ChangePreview, error) {
	s.writes.Add(1)
	out, err := s.Service.RecordChangeGuarded(ctx, command, mutation, hash, token)
	if out.Activity.ID != "" && err == nil {
		s.failActivity.Store(true)
	}
	return out, err
}

func TestLedgerPostRefreshesThreeViewsAndPreservesFilteredHistory(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, bank := seedBankBalanceFixture(t, true)
	adapter := historyapi.NewService(app)
	withdrawal, err := adapter.RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyRemoved, AccountID: bank.String(), Amount: "5", Currency: "USD",
		Reason: "other", EffectiveAt: accountTestClock.Add(-24 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	previousID := domain.ActivityID(withdrawal.Activity.ID)
	probe := &ledgerRefreshProbe{Service: app, staleRead: make(chan struct{}), releaseStale: make(chan struct{}), versions: make(map[string]string)}
	clock := func() time.Time { return accountTestClock }
	overview := newOverviewPage(ctx, probe, clock, func() {})
	accounts := newAccountsPage(ctx, probe, clock, func() {})
	ledger := newLedgerHistory(ctx, probe, clock, func() {})
	release := sync.OnceFunc(func() { close(probe.releaseStale) })
	t.Cleanup(func() { release(); overview.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{history: ledger, accounts: accounts, overview: overview, version: app.MobileDataVersion})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 3 {
			h.frame()
			ledger.cashMovements.workers.Wait()
			ledger.reads.workers.Wait()
			ledger.model.Wait()
			accounts.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	h.frame()
	overview.workers.Wait()
	h.frame()
	initialVersion := app.MobileDataVersion()
	if overview.current == nil || overview.current.value.NetWorth.String() != "65" {
		t.Fatal("the initial synthetic net worth was not displayed")
	}
	probe.blockOverview.Store(true)
	h.click("Reload", false)
	select {
	case <-probe.staleRead:
	case <-time.After(3 * time.Second):
		t.Fatal("the pre-write Overview snapshot did not start")
	}

	click("Ledger activity")
	h.click("Activity type", true)
	click("Money removed")
	wantFilter := history.Filters{Kind: domain.ActivityCashOut}
	timelineID := h.shell.trail.current().id
	label("Loaded records: 1")
	click("Record contribution")
	draft := ledger.cashMovements.draft
	setBankField(h, "Contribution amount", &draft.amount, "25")
	click("Preview contribution")
	label("Resulting balance · 120 USD")
	click("Confirm contribution")
	written := ledger.reads.posted.ID
	version := app.MobileDataVersion()
	if version == initialVersion || written == "" || ledger.reads.detail.id != written || ledger.model.Filters() != wantFilter {
		t.Fatal("posting did not open the exact Activity independently of its active filter")
	}
	label("Contribution posted.")
	label("Could not load this activity.")
	click("Retry")
	label("Resulting balance · 120 USD")

	probe.failAccount.Store(true)
	click("Open account · Harbor Checking")
	if accounts.target.account != bank || accounts.current != nil {
		t.Fatal("account jump showed a different or unverified account")
	}
	label("Could not load account data.")
	click("Retry")
	label("USD · $ 120.00")
	if row := accounts.current.rows[0]; row.record.LatestValue.Amount.CanonicalAmount() != "120" || row.value == nil {
		t.Fatal("the account facts and valuation were not refreshed")
	}

	click("Overview")
	// The replacement must land before the abandoned response is released.
	deadline := time.Now().Add(3 * time.Second)
	for overview.current == nil || overview.current.value.NetWorth.String() != "90" {
		if time.Now().After(deadline) {
			t.Fatal("updated Overview waited for the abandoned result")
		}
		h.frame()
		time.Sleep(time.Millisecond)
	}
	release()
	overview.workers.Wait()
	settle()
	if overview.current.value.NetWorth.String() != "90" || probe.writes.Load() != 1 {
		t.Fatal("the late Overview reply or a read Retry changed the acknowledged outcome")
	}
	probe.mu.Lock()
	for _, name := range []string{"timeline", "activity", "account", "account valuation", "overview"} {
		if got := probe.versions[name]; got != version {
			probe.mu.Unlock()
			t.Fatalf("%s observed %q, expected the post-write version %q", name, got, version)
		}
	}
	probe.mu.Unlock()

	click("Back")
	click("Back")
	if ledger.reads.detail == nil || ledger.reads.detail.id != written {
		t.Fatal("Back did not reopen the saved Activity")
	}
	h.focused(ledger.reads.detail.accountButton(bank))
	click("Back to ledger activity")
	if h.shell.trail.current().id != timelineID || ledger.model.Filters() != wantFilter || len(ledger.model.Rows()) != 1 ||
		ledger.model.Rows()[0].ID != previousID {
		t.Fatal("History return discarded the visit, filter, or matching row")
	}
	h.focused(&ledger.cashMovements.actions[history.CashContribution])

	// Open a different activity, cross to its account, then change the same
	// ledger through the application API without a Gio posting callback.
	click("View")
	click("Open account · Harbor Checking")
	if _, err := adapter.RecordChange(ctx, historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangeMoneyAdded, AccountID: bank.String(), Amount: "5", Currency: "USD",
		Reason: "contribution", EffectiveAt: accountTestClock.Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	settle()
	label("USD · $ 125.00")
	if h.shell.seenLedgerVersion != app.MobileDataVersion() || probe.writes.Load() != 1 {
		t.Fatal("the shell did not observe an outside-page service update")
	}
	// Model the same account-to-Activity navigation used by an account's
	// saved receipt. The nearest timeline now has two intervening visits.
	ledger.reads.open(written, nil)
	settle()
	click("Back to ledger activity")
	if h.shell.trail.current().id != timelineID || ledger.model.Filters() != wantFilter {
		t.Fatal("the detail's return action did not find the filtered visit across the detour")
	}
	h.focused(ledger.detailButton(previousID))
	click("Forward")
	if ledger.reads.detail == nil || ledger.reads.detail.id != previousID {
		t.Fatal("Forward lost the original activity after returning to the timeline")
	}

	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	if row := readBankRecord(t, reopened, bank); row.LatestValue.Amount.CanonicalAmount() != "125" {
		t.Fatal("account balance did not survive reopen")
	}
	posted, err := reopened.Activity(ctx, written)
	if err != nil || posted.ID != written || len(posted.Effects) != 1 || posted.Effects[0].Money.CanonicalAmount() != "25" {
		t.Fatalf("saved Activity did not survive reopen: %+v / %v", posted, err)
	}
	rows, err := reopened.ListActivities(ctx, 50)
	if err != nil || len(rows) != 3 {
		t.Fatalf("expected the baseline and two explicit writes: %+v / %v", rows, err)
	}
}
