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

	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type cashTargetTestService struct {
	*application.Service
	attempts       []*application.CashBalancePlan
	started, allow chan struct{}
	failedRead     atomic.Bool
}

func (s *cashTargetTestService) ConfirmCashBalance(ctx context.Context, plan *application.CashBalancePlan) (domain.AccountCashValue, error) {
	s.attempts = append(s.attempts, plan)
	attempt := len(s.attempts)
	if attempt == 2 {
		close(s.started)
		<-s.allow
	}
	value, err := s.Service.ConfirmCashBalance(ctx, plan)
	if err == nil && attempt == 2 {
		return domain.AccountCashValue{}, errors.New("synthetic loss of the committed cash reply")
	}
	if err == nil && attempt == 3 {
		s.failedRead.Store(true)
	}
	return value, err
}

func (s *cashTargetTestService) ListAccounts(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountRecord, error) {
	if len(filter.AccountIDs) == 1 && s.failedRead.Swap(false) {
		return nil, errors.New("synthetic detail read failure after receipt")
	}
	return s.Service.ListAccounts(ctx, filter)
}

func TestAccountCashReconciliationMatchesWailsAndReplaysSafely(t *testing.T) {
	ctx := t.Context()
	service, session, paths, id := seedCashTargetFixture(t, true)
	ref, _, _, refID := seedCashTargetFixture(t, true)
	fault := &cashTargetTestService{Service: service, started: make(chan struct{}), allow: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.allow) })
	t.Cleanup(release)
	p, h, finish := cashBalanceHarness(t, fault)
	click := func(label string) { h.click(label, false); finish() }
	label := func(text string) { assertPageLabel(t, &h.router, text) }
	assertNetWorth := func(app *application.Service, want string) {
		t.Helper()
		value, err := app.Overview(ctx, domain.AccountFilter{})
		wails, refErr := portfolio.NewService(ref).Overview(ctx, account.AccountFilterRequest{})
		if err != nil || refErr != nil || !value.Complete || !wails.Complete || value.NetWorth.String() != want || wails.NetWorth != want {
			t.Fatalf("net worth Gio %s, Wails %s, want %s: %v / %v", value.NetWorth, wails.NetWorth, want, err, refErr)
		}
	}
	assertNetWorth(service, "124.6")
	scrollToAccountDetailLabel(h, "Set cash balance")
	click("Set cash balance")
	d := p.cashBalance.draft
	setBankField(h, "Target balance · USD", &d.amount, "-1")
	click("Review cash balance")
	label("Enter a nonnegative balance with up to four decimal places.")
	setBankField(h, "Target balance · USD", &d.amount, "100")
	click("Review cash balance")
	label("The target matches the current cash balance.")
	if len(fault.attempts) != 0 {
		t.Fatal("invalid or unchanged balances reached confirmation")
	}
	setBankField(h, "Target balance · USD", &d.amount, "150")
	note := "Counted statement"
	setBankField(h, "Reason (optional)", &d.reason, note)
	click("Review cash balance")
	label("Adjustment · +50 USD")
	for _, side := range []struct {
		app *application.Service
		id  domain.AccountID
	}{{service, id}, {ref, refID}} {
		preview, err := side.app.PreviewCashBalance(ctx, side.id, "110", "USD", nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := side.app.ConfirmCashBalance(ctx, preview); err != nil {
			t.Fatal(err)
		}
	}
	advanceCashTargetClock(service, ref, time.Minute)
	click("Confirm cash balance")
	label("The ledger changed. Preview this action again.")
	label("Balance before · 110 USD")
	click("Review cash balance")
	label("Adjustment · +40 USD")
	label("Effective: 2026-11-22 01:03:03.456 (Asia/Singapore)")
	plan := d.review.plan
	bounds := semanticBounds(t, &h.router, "Confirm cash balance", false)
	point := bounds.Min.Add(bounds.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("cash confirmation did not start")
	}
	h.clickAt(point)
	h.click("Back", false)
	if !p.cashBalanceUnresolved() || h.shell.activeRoute() != accountsRoute || p.target.account != id {
		t.Fatal("navigation escaped a cash confirmation")
	}
	release()
	finish()
	label("Saving could not be confirmed. Retry the same submission.")
	if findPageLabel(&h.router, "Confirm cash balance") != nil || len(fault.attempts) != 2 {
		t.Fatal("double click issued an extra confirmation")
	}
	click("Retry same submission")
	label("Cash balance saved.")
	label("Update failed. Showing the previous result.")
	up := p.cashBalance.saved
	click("Retry")
	if len(fault.attempts) != 3 || fault.attempts[1] != plan || fault.attempts[2] != plan {
		t.Fatal("confirmation retries lost the reviewed plan, or read Retry wrote")
	}
	assertWailsCashTarget(t, ref, refID, plan, up, &note)
	assertNetWorth(service, "174.6")
	click("View reconciliation")
	label("Cash reconciliation")
	label("Remeasurement")
	if up.ActivityID == nil || h.shell.historyPage.reads.detail.id != *up.ActivityID {
		t.Fatal("the account receipt did not open its reconciliation")
	}
	click("Back")
	advanceCashTargetClock(service, ref, 2*time.Minute)
	scrollToAccountDetailLabel(h, "Set cash balance")
	click("Set cash balance")
	d = p.cashBalance.draft
	setBankField(h, "Target balance · USD", &d.amount, "80")
	click("Review cash balance")
	label("Balance before · 150 USD")
	label("Adjustment · -70 USD")
	downPlan := d.review.plan
	click("Confirm cash balance")
	down := p.cashBalance.saved
	assertWailsCashTarget(t, ref, refID, downPlan, down, nil)
	assertNetWorth(service, "104.6")
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	advanceCashTargetClock(reopened, ref, 2*time.Minute)
	again, err := reopened.ConfirmCashBalance(ctx, plan)
	if err != nil || again.ID != up.ID || again.ActivityID == nil || *again.ActivityID != *up.ActivityID {
		t.Fatalf("reopened replay returned %+v, %v", again, err)
	}
	assertNetWorth(reopened, "104.6")
	for _, app := range []*application.Service{reopened, ref} {
		rows, err := app.ListActivities(ctx, 50)
		if err != nil || len(rows) != 3 {
			t.Fatalf("expected only three reconciliations: %+v, %v", rows, err)
		}
		for i, activity := range rows {
			want := []string{"70", "40", "10"}[i]
			direction := domain.EffectAdded
			if i == 0 {
				direction = domain.EffectRemoved
			}
			if activity.Reason != domain.ReasonReconciliation || len(activity.Effects) != 1 {
				t.Fatalf("unexpected activity provenance: %+v", activity)
			}
			e := activity.Effects[0]
			if e.Classification != domain.ClassificationRemeasurement || e.Target != domain.EffectTargetAccountCash ||
				e.Direction != direction || e.Money == nil || e.Money.Currency() != "USD" || e.Money.CanonicalAmount() != want {
				t.Fatalf("incorrect cash effect: %+v", e)
			}
		}
		app.SetClock(func() time.Time { return accountTestClock.Add(72 * time.Hour) })
		result, err := app.AssetChange(ctx, domain.AnalysisQuery{
			Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-11-21", To: "2026-11-22",
			Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true,
		})
		if err != nil || result.Status != domain.CompletenessOK {
			t.Fatalf("asset changes incomplete: %+v, %v", result.AnalysisAvailability, err)
		}
		var adjustment string
		for _, bucket := range result.Waterfall {
			if bucket.Bucket == domain.BucketExternalFlow && bucket.Amount != nil && !bucket.Amount.IsZero() {
				t.Fatalf("cash reconciliation counted as external flow: %+v", bucket)
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

func cashBalanceHarness(t *testing.T, ledger interface {
	accountsReader
	cashBalanceLedger
	historyService
}) (*accountsPage, *previewHarness, func()) {
	t.Helper()
	clock := func() time.Time { return accountTestClock }
	p := newAccountsPage(t.Context(), ledger, clock, func() {})
	p.cashBalance = &accountCashBalance{service: ledger}
	historyPage := newLedgerHistory(t.Context(), ledger, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p, history: historyPage})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() { p.workers.Wait(); historyPage.model.Wait(); historyPage.reads.workers.Wait() }
	finish := func() { h.frame(); wait(); h.frame(); wait(); h.frame() }
	h.frame()
	h.click("Accounts", false)
	finish()
	h.click("Cash Studio", false)
	finish()
	return p, h, finish
}

func advanceCashTargetClock(gio, wails *application.Service, offset time.Duration) {
	clock := func() time.Time { return accountTestClock.Add(2*time.Minute + 3456*time.Millisecond + offset) }
	gio.SetClock(clock)
	wails.SetClock(clock)
}

func seedCashTargetFixture(t *testing.T, started bool) (*application.Service, *pilot.Session, pilot.Paths, domain.AccountID) {
	t.Helper()
	ctx := t.Context()
	paths := prepareOnboardingProfile(t)
	service, session := reopenBaselineProfile(t, paths)
	service.SetClock(func() time.Time { return accountTestClock.Add(-72 * time.Hour) })
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic cash balance", BaseCurrency: "USD", MemberNames: []string{"Ana Example"}}); err != nil {
		t.Fatal(err)
	}
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.CreateAccount(ctx, application.AccountInput{Name: "Cash Studio", AccountType: "cash_on_hand", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{boot.Members[0].ID}, IncludeInNetWorth: true, IncludeInPortfolio: true})
	if err != nil {
		t.Fatal(err)
	}
	id := record.Account.ID
	if _, err := service.SetFXPreference(ctx, "EUR", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "EUR", "USD", "1.2", "2026-11-18T16:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if started {
		for code, amount := range map[string]string{"USD": "100", "EUR": "20.5"} {
			if _, err := service.AppendAccountCashValue(ctx, id, amount, code, ""); err != nil {
				t.Fatal(err)
			}
		}
		service.SetClock(func() time.Time { return accountTestClock.Add(-48 * time.Hour) })
		if _, err := service.CaptureSyntheticOrigin(ctx, "Asia/Singapore"); err != nil {
			t.Fatal(err)
		}
		advanceCashTargetClock(service, service, 0)
	} else {
		service.SetClock(func() time.Time { return time.Date(2026, 11, 22, 0, 2, 3, 456000000, time.UTC) })
	}
	return service, session, paths, id
}

func assertWailsCashTarget(t *testing.T, app *application.Service, id domain.AccountID, plan *application.CashBalancePlan, saved domain.AccountCashValue, note *string) {
	t.Helper()
	ctx := t.Context()
	adapter := holding.NewService(app)
	preview, err := adapter.PreviewCashBalance(ctx, id.String(), plan.Target, plan.Currency, note)
	if err != nil || preview.Current != plan.Current || preview.Target != plan.Target || preview.Delta != plan.Delta ||
		preview.EffectiveAt != wire.FormatTime(plan.EffectiveAt) || preview.Timezone != plan.Timezone || !reflect.DeepEqual(preview.Reason, plan.Reason) {
		t.Fatalf("cash review differs from Wails: %+v / %+v, %v", plan, preview, err)
	}
	want, err := adapter.ConfirmCashBalance(ctx, preview.PlanID)
	got := wire.FromAccountCashValue(saved)
	if err != nil || (got.ActivityID == nil) != (want.ActivityID == nil) || (got.ActivityEffectID == nil) != (want.ActivityEffectID == nil) {
		t.Fatalf("cash result has different provenance: %+v / %+v, %v", got, want, err)
	}
	got.ID, got.AccountID, got.ActivityID, got.ActivityEffectID = "", "", nil, nil
	want.ID, want.AccountID, want.ActivityID, want.ActivityEffectID = "", "", nil, nil
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cash result differs from Wails: %+v / %+v", got, want)
	}
}
