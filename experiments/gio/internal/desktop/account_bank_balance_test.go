package desktop

import (
	"context"
	"errors"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"image"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type balanceTargetTestService struct {
	*application.Service
	appends        int
	records        []string
	lostAppend     bool
	keepReads      bool
	failedRead     atomic.Bool
	started, allow chan struct{}
}

func (s *balanceTargetTestService) AppendAccountValue(ctx context.Context, id domain.AccountID, amount, day string) (domain.AccountValue, error) {
	s.appends++
	v, err := testfixture.AppendAccountValue(s.Service, ctx, id, amount, day)
	if err == nil && s.lostAppend {
		return domain.AccountValue{}, errors.New("test: lost acknowledgement from unkeyed append")
	}
	return v, err
}

func (s *balanceTargetTestService) RecordChangeGuarded(ctx context.Context, cmd any, id, hash, token string) (domain.ChangePreview, error) {
	s.records = append(s.records, id)
	attempt := len(s.records)
	if attempt == 2 {
		close(s.started)
		<-s.allow
	}
	result, err := s.Service.RecordChangeGuarded(ctx, cmd, id, hash, token)
	if err == nil && attempt == 2 {
		return domain.ChangePreview{}, errors.New("test: lost keyed balance response")
	}
	if err == nil && attempt == 3 && !s.keepReads {
		s.failedRead.Store(true)
	}
	return result, err
}

func (s *balanceTargetTestService) ListAccounts(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountRecord, error) {
	if len(filter.AccountIDs) == 1 && s.failedRead.Swap(false) {
		return nil, errors.New("test: failed read after the keyed receipt")
	}
	return s.Service.ListAccounts(ctx, filter)
}

func TestBankBalanceHistoryMatchesWailsAndKeepsReceipt(t *testing.T) {
	ctx := t.Context()
	app, session, paths, bank := seedBankBalanceFixture(t, true)
	ref, _, _, refBank := seedBankBalanceFixture(t, true)
	fault := &balanceTargetTestService{Service: app, started: make(chan struct{}), allow: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.allow) })
	t.Cleanup(release)
	uiNow := accountTestClock
	p, h, finish := balanceTargetHarness(t, fault, "Harbor Checking", func() time.Time { return uiNow })
	click := func(text string) { h.click(text, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	assertBalanceTargetNetWorth(t, app, ref, "70", "30")

	click("Update bank balance")
	d := p.balanceTarget.draft
	setBankField(h, "Target balance · USD", &d.amount, "-30")
	click("Review bank balance")
	label("Enter a nonnegative balance with up to four decimal places.")
	setBankField(h, "Target balance · USD", &d.amount, "100")
	click("Review bank balance")
	label("The target matches the current bank balance.")
	setBankField(h, "Target balance · USD", &d.amount, "150")
	click("Review bank balance")
	label("Adjustment · +50 USD")
	for _, side := range []struct {
		app *application.Service
		id  domain.AccountID
	}{{app, bank}, {ref, refBank}} {
		request := wailsBalanceAdjustment(side.id, "110", accountTestClock)
		if _, err := historyapi.NewService(side.app).RecordChange(ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	setBalanceTargetClock(time.Minute, app, ref)
	uiNow = accountTestClock.Add(time.Minute)
	click("Confirm bank balance")
	label("The ledger changed. Preview this action again.")
	click("Review bank balance")
	label("Balance before · 110 USD")
	label("Adjustment · +40 USD")
	label("Effective: 2026-11-22 01:01:00.000 (Asia/Singapore)")
	plan := *d.review
	bounds := semanticBounds(t, &h.router, "Confirm bank balance", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("bank confirmation did not start")
	}
	h.clickAt(point)
	h.click("Back", false)
	if !p.balanceTargetUnresolved() || p.target.account != bank {
		t.Fatal("navigation escaped pending confirmation")
	}
	release()
	finish()
	label("Saving could not be confirmed. Retry the same submission.")
	if len(fault.records) != 2 || findPageLabel(&h.router, "Confirm bank balance") != nil {
		t.Fatal("double click issued an additional confirmation")
	}
	click("Retry same submission")
	label("Bank balance saved.")
	label("Update failed. Showing the previous result.")
	posted := p.balanceTarget.saved.activity
	click("Retry")
	if len(fault.records) != 3 || fault.records[1] != plan.mutation || fault.records[2] != plan.mutation || fault.appends != 0 {
		t.Fatal("confirmation lost the mutation identity or read Retry wrote")
	}
	assertWailsBalanceAdjustment(t, app, ref, refBank, plan, posted)
	assertBalanceTargetNetWorth(t, app, ref, "120", "30")
	click("View reconciliation")
	label("Value update")
	label("Remeasurement")
	if h.shell.historyPage.reads.detail.id != posted {
		t.Fatal("the account receipt did not open the saved activity")
	}
	click("Back")
	setBalanceTargetClock(2*time.Minute, app, ref)
	uiNow = accountTestClock.Add(2 * time.Minute)
	click("Update bank balance")
	d = p.balanceTarget.draft
	setBankField(h, "Target balance · USD", &d.amount, "80")
	click("Review bank balance")
	label("Adjustment · -70 USD")
	down := *d.review
	click("Confirm bank balance")
	assertWailsBalanceAdjustment(t, app, ref, refBank, down, p.balanceTarget.saved.activity)
	assertBalanceTargetNetWorth(t, app, ref, "50", "30")
	click("Back to accounts")
	click("Synthetic Card")
	if findPageLabel(&h.router, "Update bank balance") != nil {
		t.Fatal("the asset-balance action appeared on a credit card")
	}
	label("Update card balance")
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	setBalanceTargetClock(3*time.Minute, reopened, ref)
	again, err := plan.commit(ctx, reopened)
	if err != nil || again.activity != posted {
		t.Fatalf("receipt replay after reopen: %+v, %v", again, err)
	}
	assertBalanceTargetNetWorth(t, reopened, ref, "50", "30")
	for _, ledger := range []*application.Service{reopened, ref} {
		rows, err := ledger.ListActivities(ctx, 50)
		if err != nil || len(rows) != 3 {
			t.Fatalf("expected only three explicit adjustments: %+v, %v", rows, err)
		}
		for i, activity := range rows {
			if activity.Kind != domain.ActivityValueUpdate || activity.Reason != domain.ReasonReconciliation || len(activity.Effects) != 1 {
				t.Fatalf("unexpected history record: %+v", activity)
			}
			effect := activity.Effects[0]
			if effect.Classification != domain.ClassificationRemeasurement || effect.Target != domain.EffectTargetAccountValue ||
				effect.Direction != []domain.EffectDirection{domain.EffectRemoved, domain.EffectAdded, domain.EffectAdded}[i] ||
				effect.Money == nil || effect.Money.CanonicalAmount() != []string{"70", "40", "10"}[i] {
				t.Fatalf("unexpected remeasurement effect: %+v", effect)
			}
		}
		setBalanceTargetClock(72*time.Hour, ledger)
		result, err := ledger.AssetChange(ctx, domain.AnalysisQuery{
			Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-11-21", To: "2026-11-22",
			Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true,
		})
		if err != nil || result.Status != domain.CompletenessOK {
			t.Fatalf("asset changes incomplete: %+v, %v", result.AnalysisAvailability, err)
		}
		var adjustment string
		for _, bucket := range result.Waterfall {
			if bucket.Bucket == domain.BucketExternalFlow && bucket.Amount != nil && !bucket.Amount.IsZero() {
				t.Fatalf("bank reconciliation counted as external flow: %+v", bucket)
			}
			if bucket.Bucket == domain.BucketAdjustment && bucket.Amount != nil {
				adjustment = bucket.Amount.CanonicalAmount()
			}
		}
		if adjustment != "-20" {
			t.Fatalf("net adjustment = %s, want -20 USD", adjustment)
		}
	}
}

func balanceTargetHarness(t *testing.T, ledger *balanceTargetTestService, accountName string, clock func() time.Time) (*accountsPage, *previewHarness, func()) {
	t.Helper()
	p := newAccountsPage(t.Context(), ledger, clock, func() {})
	p.balanceTarget = &accountBalanceTarget{service: ledger}
	historyPage := newLedgerHistory(t.Context(), ledger, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p, history: historyPage})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() { p.workers.Wait(); historyPage.model.Wait(); historyPage.reads.workers.Wait() }
	finish := func() { h.frame(); wait(); h.frame(); wait(); h.frame() }
	h.frame()
	h.click("Accounts", false)
	finish()
	h.click(accountName, false)
	finish()
	return p, h, finish
}

func wailsBalanceAdjustment(id domain.AccountID, target string, at time.Time) historyapi.ChangeCommandRequest {
	return historyapi.ChangeCommandRequest{Kind: historyapi.ChangeValueUpdate, AccountID: id.String(), NewValue: target, NewValueCurrency: "USD",
		Reason: "reconciliation", EffectiveAt: at.UTC().Format(time.RFC3339Nano), MutationID: domain.NewMutationID().String()}
}

func assertWailsBalanceAdjustment(t *testing.T, gio, ref *application.Service, refID domain.AccountID, plan balanceTargetPlan, saved domain.ActivityID) {
	t.Helper()
	ctx := t.Context()
	adapter := historyapi.NewService(ref)
	request := wailsBalanceAdjustment(refID, plan.target, plan.at)
	preview, err := adapter.PreviewChange(ctx, request)
	if err != nil || len(preview.Review) != 1 || preview.Review[0].Before != plan.before || preview.Review[0].After != plan.after || preview.Review[0].Delta != plan.delta ||
		preview.Activity.EffectiveAt != wire.FormatTime(plan.at) {
		t.Fatalf("review differs from Wails: %+v, %v", preview, err)
	}
	want, err := adapter.RecordChange(ctx, request)
	actual, localErr := gio.Activity(ctx, saved)
	if err != nil || localErr != nil {
		t.Fatalf("reading committed adjustments: %v / %v", localErr, err)
	}
	got := wire.FromActivity(actual)
	if got.Kind != want.Activity.Kind || got.Reason != want.Activity.Reason || got.EffectiveAt != want.Activity.EffectiveAt || len(got.Effects) != 1 || len(want.Effects) != 1 ||
		got.Effects[0].Classification != want.Effects[0].Classification || got.Effects[0].Direction != want.Effects[0].Direction ||
		!reflect.DeepEqual(got.Effects[0].Money, want.Effects[0].Money) || readBankRecord(t, gio, plan.account).LatestValue.Amount.CanonicalAmount() != plan.target ||
		readBankRecord(t, ref, refID).LatestValue.Amount.CanonicalAmount() != plan.target {
		t.Fatalf("saved values differ from Wails: %+v / %+v", got, want.Activity)
	}
}

func assertBalanceTargetNetWorth(t *testing.T, gio, ref *application.Service, target, debt string) {
	t.Helper()
	ctx := t.Context()
	got, err := gio.Overview(ctx, domain.AccountFilter{})
	want, refErr := portfolio.NewService(ref).Overview(ctx, account.AccountFilterRequest{})
	if err != nil || refErr != nil || !got.Complete || !want.Complete || got.NetWorth.String() != target || want.NetWorth != target || got.Liabilities.String() != debt || want.Liabilities != debt {
		t.Fatalf("net worth Gio %s, Wails %s, want %s; debts %s / %s, want %s: %v / %v", got.NetWorth, want.NetWorth, target, got.Liabilities, want.Liabilities, debt, err, refErr)
	}
}

func assertBankObservations(t *testing.T, session *pilot.Session, bank domain.AccountID, observations, activities int) {
	t.Helper()
	var gotObs, gotAct int
	if err := session.DB.SQL.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM account_values WHERE account_id=? AND projection_kind='baseline'", bank).Scan(&gotObs); err != nil {
		t.Fatal(err)
	}
	if err := session.DB.SQL.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM activities").Scan(&gotAct); err != nil {
		t.Fatal(err)
	}
	if gotObs != observations || gotAct != activities {
		t.Fatalf("got %d baseline observations and %d activities, want %d and %d", gotObs, gotAct, observations, activities)
	}
}

func setBalanceTargetClock(offset time.Duration, ledgers ...*application.Service) {
	for _, ledger := range ledgers {
		testfixture.SetClock(ledger, func() time.Time { return accountTestClock.Add(offset) })
	}
}

func seedBankBalanceFixture(t *testing.T, started bool) (*application.Service, *pilot.Session, pilot.Paths, domain.AccountID) {
	t.Helper()
	ctx := t.Context()
	paths := prepareOnboardingProfile(t)
	app, session := reopenBaselineProfile(t, paths)
	setBalanceTargetClock(-72*time.Hour, app)
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic bank balance", BaseCurrency: "USD", MemberNames: []string{"Ana Example"}}); err != nil {
		t.Fatal(err)
	}
	boot, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, kind, role, amount string) domain.AccountID {
		t.Helper()
		row, err := testfixture.CreateAccount(app, ctx, application.AccountInput{Name: name, AccountType: kind, BalanceSheetRole: role,
			TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: amount, IncludeInNetWorth: true,
			OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		return row.Account.ID
	}
	bank := create("Harbor Checking", "bank_account", "asset", "100")
	create("Synthetic Card", "credit_card", "liability", "30")
	if started {
		setBalanceTargetClock(-48*time.Hour, app)
		if _, err := testfixture.CaptureSyntheticOrigin(app, ctx, "Asia/Singapore"); err != nil {
			t.Fatal(err)
		}
	}
	setBalanceTargetClock(0, app)
	return app, session, paths, bank
}

// Balance reviews now always carry a dated, keyed ledger command. A clock
// rollback must reject the retained future instant rather than silently append
// an unkeyed value or mark the review as saved.
func TestBalanceTargetRejectsClockRollback(t *testing.T) {
	for _, tc := range []struct {
		name, target string
		card         bool
	}{
		{"bank", "120", false},
		{"card", "40", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, session, _, id := seedBankBalanceFixture(t, true)
			if tc.card {
				id = syntheticCardID(t, app)
			}
			before := readBankRecord(t, app, id).LatestValue
			now := accountTestClock.Add(time.Minute)
			clock := func() time.Time { return now }
			testfixture.SetClock(app, clock)
			plan, err := reviewBalanceTarget(t.Context(), app, id, tc.target, clock)
			if err != nil {
				t.Fatal(err)
			}
			now = accountTestClock
			receipt, err := plan.commit(t.Context(), app)
			var issue *domain.Error
			if !errors.As(err, &issue) || issue.Code != domain.ErrInvalidChangeTime || receipt.known() {
				t.Fatalf("rollback accepted or mapped incorrectly: %+v %v", receipt, err)
			}
			if got := readBankRecord(t, app, id).LatestValue; got.ID != before.ID {
				t.Fatalf("rejected review wrote %+v", got)
			}
			assertBankObservations(t, session, id, 1, 0)
			now = accountTestClock.Add(2 * time.Minute)
			plan, err = reviewBalanceTarget(t.Context(), app, id, tc.target, clock)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err = plan.commit(t.Context(), app)
			if err != nil || !receipt.known() {
				t.Fatalf("corrected clock: %+v %v", receipt, err)
			}
			if got := readBankRecord(t, app, id).LatestValue.Amount.CanonicalAmount(); got != tc.target {
				t.Fatalf("target = %s, want %s", got, tc.target)
			}
			assertBankObservations(t, session, id, 1, 1)
		})
	}
}
