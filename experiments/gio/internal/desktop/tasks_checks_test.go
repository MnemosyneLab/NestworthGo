package desktop

import (
	"context"
	"fmt"
	"image"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	liquidityapi "github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	reconciliation "github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

type taskChecksFixture struct {
	instrument domain.InstrumentID
	card       domain.AccountID
	paused     string
	active     string
}

func seedTaskChecks(ctx context.Context, service *application.Service, base managedProductFixture, at time.Time) (out taskChecksFixture, err error) {
	service.SetClock(func() time.Time { return at.Add(-20 * time.Hour) })
	boot, err := service.Bootstrap(ctx)
	if err != nil {
		return out, err
	}
	owner := []domain.MemberID{boot.Members[0].ID}
	broker, err := service.CreateAccount(ctx, application.AccountInput{
		Name: "Task Brokerage", AccountType: "brokerage", TrackingMode: "holdings", BalanceSheetRole: "asset",
		DefaultCurrency: "USD", OwnerIDs: owner, IncludeInNetWorth: true, IncludeInPortfolio: true,
	})
	if err != nil {
		return out, err
	}
	i, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Review Stock", Symbol: "QA-REVIEW", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		return out, err
	}
	out.instrument = i.ID
	if _, err = service.CreateHolding(ctx, application.HoldingInput{
		AccountID: broker.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "3", UnitCost: "15",
	}); err != nil {
		return out, err
	}
	if _, err = service.AppendManualInstrumentQuote(ctx, i.ID, "20", "", false); err != nil {
		return out, err
	}
	card, err := service.CreateAccount(ctx, application.AccountInput{
		Name: "Daily Credit Card", AccountType: "credit_card", TrackingMode: "balance", BalanceSheetRole: "liability",
		DefaultCurrency: "USD", InitialAmount: "30", OwnerIDs: owner, IncludeInNetWorth: true,
	})
	if err != nil {
		return out, err
	}
	out.card = card.Account.ID
	service.SetClock(func() time.Time { return at })
	api := liquidityapi.NewService(service)
	if _, err := api.CreateReviewRuleV2(ctx, application.SaveReviewRuleV2Input{
		RuleID: string(domain.NewReviewRuleID()), MutationID: domain.NewMutationID().String(),
		Name: "Original cost under 100 USD", Enabled: true, Severity: "warning", Spec: domain.ReviewRuleSpec{
			SpecVersion: domain.ReviewSpecVersion, MetricPolicyVersion: domain.ReviewMetricPolicyVersion, Evaluator: domain.ReviewEvaluator,
			Target: domain.ReviewRuleTarget{Kind: "instruments", InstrumentIDs: []domain.InstrumentID{i.ID}},
			Predicate: domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
				{Leaf: &domain.ReviewRuleLeaf{Metric: "cost", Operator: "<", Value: "100", Currency: "USD", FXMode: "native"}},
			}},
			Schedule:      domain.ReviewRuleSchedule{Period: "calendar_day", Timezone: "Asia/Singapore", CutoffVersion: domain.ReviewCutoffVersion},
			TriggerPolicy: domain.ReviewTriggerPolicy{Kind: "single"}, DataPolicy: domain.DefaultReviewRuleDataPolicy(),
		},
	}); err != nil {
		return out, err
	}
	stmt := reconciliation.NewService(service)
	record := func(action application.ReconciliationAction) (domain.ReconciliationSession, error) {
		preview, err := stmt.Preview(ctx, action)
		if err != nil {
			return domain.ReconciliationSession{}, err
		}
		return stmt.Commit(ctx, preview.PlanID)
	}
	paused, err := record(application.ReconciliationAction{Action: "start", AccountIDs: []string{out.card.String()}})
	if err != nil {
		return out, err
	}
	if _, err := record(application.ReconciliationAction{Action: "pause", SessionID: paused.ID, ExpectedRevision: paused.Revision}); err != nil {
		return out, err
	}
	out.paused = paused.ID
	active, err := record(application.ReconciliationAction{Action: "start", AccountIDs: []string{out.card.String()}})
	if err != nil {
		return out, err
	}
	out.active = active.ID
	for _, ids := range [][]string{{base.account.String()}, {base.account.String(), out.card.String()}} {
		if _, err := record(application.ReconciliationAction{Action: "start", AccountIDs: ids}); err != nil {
			return out, err
		}
	}
	if _, err := api.ReconcileInvestmentReviewsV2(ctx); err != nil {
		return out, err
	}
	return out, nil
}

func TestTaskChecksMatchPublishedWailsReviewAndExactCardSession(t *testing.T) {
	ctx := t.Context()
	app, session, paths, term := openManagedProductFixture(t)
	ref, refSession, refPaths, refTerm := openManagedProductFixture(t)
	fixture, err := seedTaskChecks(ctx, app, term, accountTestClock)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := seedTaskChecks(ctx, ref, refTerm, accountTestClock)
	if err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return accountTestClock }
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	funds := newAvailableFundsPage(ctx, app, func() {})
	funds.productPage = newProductDetailPage(ctx, app, func() {})
	portfolio := newPortfolioPage(ctx, app, func() {})
	accounts := newAccountsPage(ctx, app, clock, func() {})
	accounts.statement = &accountStatement{service: reconciliation.NewService(app), origin: app}
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, funds: funds, accounts: accounts, portfolio: portfolio})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language, h.shell.prefs.saved.Timezone = settings.LanguageEnglish, "Asia/Singapore"
	finish := func() {
		for range 5 {
			h.frame()
			tasks.workers.Wait()
			portfolio.workers.Wait()
			accounts.workers.Wait()
		}
		h.frame()
	}
	click := func(label string) { t.Helper(); h.click(label, false); finish() }
	label := func(text string) { t.Helper(); assertPageLabel(t, &h.router, text) }
	wailsReview := func(s *application.Service) domain.ReviewAlertPage {
		t.Helper()
		got, err := liquidityapi.NewService(s).ListReviewAlertsV2(ctx, liquidityapi.ListReviewAlertsV2Request{
			Filter: domain.ReviewAlertFilter{Status: "pending"}, PageSize: 50,
		})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	checkReview := func(got, want domain.ReviewAlertPage, state string) {
		t.Helper()
		if got.ProjectionStatus != state || want.ProjectionStatus != state || got.PublishedOn != want.PublishedOn ||
			got.Summary != want.Summary || len(got.Items) != 1 || len(want.Items) != 1 ||
			len(got.Items[0].Rules) != 1 || len(want.Items[0].Rules) != 1 ||
			got.Items[0].InstrumentID != fixture.instrument || want.Items[0].InstrumentID != reference.instrument {
			t.Fatalf("published review differs: %+v / %+v", got, want)
		}
		a, b := got.Items[0].Rules[0], want.Items[0].Rules[0]
		if a.Name != "Original cost under 100 USD" || a.Name != b.Name || a.TriggeredOn != "2026-11-21" ||
			a.LatestClosedOn != b.LatestClosedOn || a.Window != b.Window || a.Status != b.Status || !a.Actionable || !b.Actionable ||
			a.LatestResult == nil || b.LatestResult == nil || !reflect.DeepEqual(a.LatestResult, b.LatestResult) {
			t.Fatalf("review rule, reason or evidence differs: %+v / %+v", a, b)
		}
	}

	before, err := app.ListActivities(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	version := app.MobileDataVersion()
	h.frame()
	click("Tasks & checks")
	label("Upcoming actions")
	upcoming, err := liquidityapi.NewService(ref).UpcomingActions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	label(fmt.Sprintf("Upcoming needs attention · %d", upcoming.Counts.Attention))
	label("Investment review · 1")
	label("Open reconciliations · 4")
	click("Receipts due")
	label("Overdue. Receipt is unconfirmed.")
	click("Investment review")
	label("Original cost under 100 USD")
	label("Triggered on 2026-11-21")
	checkReview(*tasks.review, wailsReview(ref), "complete")
	click("Open instrument · " + tasks.review.Items[0].DisplayName)
	if h.shell.trail.current().target.instrument != fixture.instrument || portfolio.current.instrument == nil {
		t.Fatal("review did not open the authoritative instrument")
	}
	if len(portfolio.current.positions) != 1 || portfolio.current.positions[0].Amounts.Quantity != "3" {
		t.Fatal("linked instrument did not show the authoritative holding")
	}
	label("QA-REVIEW")
	label("Review Stock")
	click("Back")
	h.focused(tasks.instruments[fixture.instrument])
	if tasks.tab != tasksInvestment {
		t.Fatal("Back lost the investment tab")
	}

	late := func() time.Time { return accountTestClock.Add(24 * time.Hour) }
	app.SetClock(late)
	ref.SetClock(late)
	click("Reload")
	label("The published review is out of date.")
	checkReview(*tasks.review, wailsReview(ref), "stale")

	click("Reconciliation")
	wantSessions, err := reconciliation.NewService(ref).List(ctx)
	if err != nil || len(wantSessions) != 4 || len(tasks.statements) != 2 {
		t.Fatalf("unexpected session scope: %d / %d, %v", len(tasks.statements), len(wantSessions), err)
	}
	for _, got := range tasks.statements {
		expectedID := reference.active
		if got.session.ID == fixture.paused {
			expectedID = reference.paused
		}
		i := slices.IndexFunc(wantSessions, func(s domain.ReconciliationSession) bool { return s.ID == expectedID })
		if i < 0 || got.account.ID != fixture.card || got.session.Status != wantSessions[i].Status ||
			got.session.Counts != wantSessions[i].Counts || got.session.StartedAt != wantSessions[i].StartedAt ||
			!slices.Equal(got.session.AccountNames, []string{"Daily Credit Card"}) {
			t.Fatalf("session list differs: %+v / %+v", got, wantSessions)
		}
	}
	view := ui.View{Language: i18n.English, DateFormat: h.shell.prefs.saved.DateFormat,
		TimeFormat: h.shell.prefs.saved.TimeFormat, TimeLocation: h.shell.prefs.displayLocation()}
	pausedIndex := slices.IndexFunc(tasks.statements, func(row taskStatement) bool { return row.session.ID == fixture.paused })
	if pausedIndex < 0 {
		t.Fatal("paused statement is missing from tasks")
	}
	when := statementRecordedTime(view, tasks.statements[pausedIndex].session.StartedAt)
	label(fmt.Sprintf("Started at %s", when))
	click(fmt.Sprintf("Open statement · Daily Credit Card · Paused · %s", when))
	d := accounts.statement.screen
	if h.shell.trail.current().target.statement != fixture.paused || d == nil || d.data == nil ||
		d.data.detail.ID != fixture.paused || d.account.ID != fixture.card || d.data.detail.Status != "paused" {
		t.Fatal("task did not open the exact paused session on the card")
	}
	label("Recorded amount owed · 30 USD (balance -30 USD)")
	click("Back")
	h.focused(tasks.statementLinks[fixture.paused])
	click("Forward")
	if accounts.statement.screen == nil || accounts.statement.screen.selected != fixture.paused {
		t.Fatal("Forward lost the exact statement session")
	}
	click("Back")
	click("Upcoming actions")
	label("Upcoming actions")
	click("All actions")
	label("Receipt schedule incomplete")
	if after, err := app.ListActivities(ctx, 30); err != nil || !reflect.DeepEqual(before, after) || version != app.MobileDataVersion() {
		t.Fatalf("task browsing changed ledger facts or version: %v", err)
	}

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := refSession.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reopenedRef, _ := reopenOnboardingProfile(t, refPaths)
	reopened.SetClock(late)
	reopenedRef.SetClock(late)
	checkReview(wailsReview(reopened), wailsReview(reopenedRef), "stale")
	reopenedSessions, _, err := readTaskReconciliation(ctx, reopened, false)
	if err != nil || len(reopenedSessions) != 2 || !slices.ContainsFunc(reopenedSessions, func(row taskStatement) bool { return row.session.ID == fixture.paused }) {
		t.Fatalf("session list was not preserved at reopen: %+v / %v", reopenedSessions, err)
	}
}

type delayedTasksReview struct {
	*application.Service
	started, release chan struct{}
	first            sync.Once
}

func (s *delayedTasksReview) ListReviewAlertGroupsV2(ctx context.Context, filter domain.ReviewAlertFilter, cursor string, size int) (domain.ReviewAlertPage, error) {
	if filter.Status == "pending" {
		s.first.Do(func() {
			close(s.started)
			<-s.release
		})
	}
	return s.Service.ListReviewAlertGroupsV2(context.WithoutCancel(ctx), filter, cursor, size)
}

func TestTaskChecksSeparateEmptyStatesAndLateRead(t *testing.T) {
	ctx := t.Context()
	app, _ := reopenOnboardingProfile(t, prepareOnboardingProfile(t))
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Synthetic empty checks", BaseCurrency: "USD", MemberNames: []string{"Ana Example"},
	}); err != nil {
		t.Fatal(err)
	}
	service := &delayedTasksReview{Service: app, started: make(chan struct{}), release: make(chan struct{})}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = service
	health := &dataHealthPage{ctx: ctx, reader: fixedDataHealthRead{}, invalidate: func() {}}
	t.Cleanup(health.workers.Wait)
	release := sync.OnceFunc(func() { close(service.release) })
	t.Cleanup(func() { release(); tasks.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, dataHealth: health})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Tasks & checks", false)
	tasks.workers.Wait()
	h.frame()
	h.click("Open Data maintenance", false)
	health.workers.Wait()
	h.frame()
	if h.shell.activeRoute() != dataHealthRoute {
		t.Fatal("the data maintenance action did not open its issues page")
	}
	h.click("Back", false)
	tasks.workers.Wait()
	h.frame()
	h.frame()
	h.focused(&tasks.upcomingHealth)
	h.click("Investment review", false)
	select {
	case <-service.started:
	case <-time.After(3 * time.Second):
		t.Fatal("review read did not start")
	}
	assertPageLabel(t, &h.router, "Loading tasks…")
	h.click("Reconciliation", false)
	// Waiting for every worker here would hide the late-read race.
	deadline := time.Now().Add(3 * time.Second)
	for tasks.request != nil && time.Now().Before(deadline) {
		h.frame()
		time.Sleep(time.Millisecond)
	}
	assertPageLabel(t, &h.router, "No open credit card statement sessions.")
	release()
	tasks.workers.Wait()
	h.frame()
	if tasks.tab != tasksReconciliation || tasks.review != nil {
		t.Fatal("the late investment result replaced the current tab")
	}
	assertPageLabel(t, &h.router, "No open credit card statement sessions.")
	h.click("Investment review", false)
	tasks.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "No current investment review has been published.")
	if findPageLabel(&h.router, "No pending investment checks in the published review.") != nil {
		t.Fatal("an unpublished review appeared complete")
	}
}
