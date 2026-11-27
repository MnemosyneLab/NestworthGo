package desktop

import (
	"context"
	"errors"
	"image"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

type reconciliationSelectionFixture struct {
	reconciliationDebtAccounts
	reserve, archived domain.AccountID
}

func seedReconciliationSelection(t *testing.T, app *application.Service, holdings domain.AccountID) reconciliationSelectionFixture {
	t.Helper()
	ctx := t.Context()
	ids := seedReconciliationDebtAccounts(t, app, holdings)
	owner, err := app.CreateMember(ctx, "Synthetic Second Owner")
	if err != nil {
		t.Fatal(err)
	}
	bank, err := app.CreateInstitution(ctx, "Synthetic North Bank", domain.InstitutionBank)
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, amount string) domain.AccountID {
		t.Helper()
		row, err := app.CreateAccount(ctx, application.AccountInput{Name: name, AccountType: "bank_account", BalanceSheetRole: "asset",
			TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: amount, InstitutionID: bank.ID.String(), OwnerIDs: []domain.MemberID{owner.ID}})
		if err != nil {
			t.Fatal(err)
		}
		return row.Account.ID
	}
	reserve, archived := create("Synthetic Second Reserve", "42.125"), create("Synthetic Archived Account", "8")
	if err := app.ArchiveAccount(ctx, archived, true); err != nil {
		t.Fatal(err)
	}
	return reconciliationSelectionFixture{ids, reserve, archived}
}

type lostReconciliationStart struct {
	holdingReconcileAPI
	attempts []string
	loseNext bool
}

func (f *lostReconciliationStart) Commit(ctx context.Context, plan string) (domain.ReconciliationSession, error) {
	f.attempts = append(f.attempts, plan)
	out, err := f.holdingReconcileAPI.Commit(ctx, plan)
	if err == nil && f.loseNext {
		f.loseNext = false
		return domain.ReconciliationSession{}, errors.New("synthetic lost reconciliation start receipt")
	}
	return out, err
}

func TestCreateMixedReconciliationMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, seeded := openExistingBuyFixture(t)
	ref, _, _, refSeeded := openExistingBuyFixture(t)
	ids := seedReconciliationSelection(t, app, seeded.account)
	refIDs := seedReconciliationSelection(t, ref, refSeeded.account)
	now := linkedContractClock()
	clock := func() time.Time { return now }
	app.SetClock(clock)
	ref.SetClock(clock)
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	prior := startTestHoldingReconciliation(t, api, ids.unrelated)
	fault := &lostReconciliationStart{holdingReconcileAPI: api}
	c := &holdingReconciliation{service: fault, ledger: app, clock: clock}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks, tasks.holdingRecon = app, c
	shell := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	shell.size = image.Pt(665, 700)
	shell.shell.prefs.saved.Language = settings.LanguageEnglish
	h := &linkedProductHarness{previewHarness: shell, c: c, tasks: tasks}
	h.settle()
	h.activate(t, "Tasks & checks")
	h.activate(t, "Reconciliation")
	h.activate(t, "Start reconciliation")

	sessionCount := func(n int) {
		t.Helper()
		rows, err := api.List(ctx)
		if err != nil || len(rows) != n {
			t.Fatalf("sessions = %v / %v, want %d", rows, err, n)
		}
	}
	s := c.draft.selection
	h.focused(&s.ownerPick.Trigger)
	if got := s.chosen(c.directory.accounts); len(got) != 5 || slices.Contains(got, ids.archived.String()) {
		t.Fatalf("default must select all active accounts: %v", got)
	}
	overlap := i18n.English.Text(i18n.ReconciliationOverlap)
	assertPageLabel(t, &h.router, overlap)
	h.activate(t, "Deselect all accounts")
	if c.canReview(c.draft) || len(s.chosen(c.directory.accounts)) != 0 {
		t.Fatal("empty selection could fall back to all accounts")
	}
	h.activate(t, "Cancel")
	h.focused(&c.start)
	sessionCount(1)
	h.press(key.NameReturn)
	h.settle()
	s = c.draft.selection
	if len(s.chosen(c.directory.accounts)) != 5 {
		t.Fatal("fresh start did not reset default selection")
	}
	choose := func(field, label string) {
		t.Helper()
		h.click(field, true)
		h.activate(t, label)
	}
	choose("Owner", "Synthetic Second Owner")
	if rows := s.visible(c.directory.accounts); len(rows) != 1 || rows[0].Account.ID != ids.reserve {
		t.Fatalf("owner filter displayed the wrong rows: %+v", rows)
	}
	choose("Institution", "Synthetic North Bank")
	choose("Account type", "Property")
	assertPageLabel(t, &h.router, "No accounts match these filters.")
	h.activate(t, "Clear account filters")
	choose("Account type", "Bank account")
	h.activate(t, "Deselect filtered accounts")
	if got := s.chosen(c.directory.accounts); len(got) != 3 || slices.Contains(got, ids.unrelated.String()) || findPageLabel(&h.router, overlap) != nil {
		t.Fatalf("filtered deselection or overlap warning used the wrong accounts: %v", got)
	}
	h.activate(t, "Select filtered accounts")
	if len(s.chosen(c.directory.accounts)) != 5 {
		t.Fatal("filtered selection dropped hidden accounts")
	}
	h.activate(t, "Deselect filtered accounts")
	h.activate(t, "Clear account filters")
	box := s.checks[ids.loan]
	h.router.Source().Execute(key.FocusCmd{Tag: box})
	h.press(key.NameSpace)
	h.settle()
	if s.includes(ids.loan) {
		t.Fatal("Space did not deselect the focused account")
	}
	h.press(key.NameSpace)
	h.settle()
	if !s.includes(ids.loan) {
		t.Fatal("Space did not restore the focused account")
	}
	choose("Account type", "Property")
	selected := s.chosen(c.directory.accounts)
	beforeCancelAccounts, err := app.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	beforeCancelLedger, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	h.activate(t, "Review")
	d := c.draft
	if d.review == nil || len(d.review.session.Accounts) != 3 || !h.shell.startReconciliationReviewOpen() {
		t.Fatal("the start sheet did not open with the frozen mixed scope")
	}
	assertPageLabel(t, &h.router, "Preview")
	assertPageLabel(t, &h.router, "Confirm only what you have reviewed. Saving a correction does not confirm a check.")
	assertPageLabel(t, &h.router, "Start reconciliation")
	h.focused(&d.startSheet.close)
	h.router.Source().Execute(key.FocusCmd{Tag: &d.startSheet.cancel})
	h.press(key.NameReturn)
	h.settle()
	h.focused(&d.preview)
	if d.review != nil || s.kind != "property" || !slices.Equal(s.chosen(c.directory.accounts), selected) {
		t.Fatal("Cancel lost the selected accounts or their filter")
	}
	h.activate(t, "Review")
	h.press(key.NameEscape)
	h.settle()
	h.focused(&d.preview)
	if d.review != nil || h.shell.startReconciliationReviewOpen() || !slices.Equal(s.chosen(c.directory.accounts), selected) || len(fault.attempts) != 0 {
		t.Fatal("Escape abandoned the selection or submitted a reconciliation")
	}
	sessionCount(1)
	afterCancelAccounts, err := app.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil || !reflect.DeepEqual(afterCancelAccounts, beforeCancelAccounts) {
		t.Fatal("canceling preview changed accounts")
	}
	afterCancelLedger, err := app.ListActivities(ctx, 100)
	if err != nil || !reflect.DeepEqual(afterCancelLedger, beforeCancelLedger) {
		t.Fatal("canceling preview changed the journal")
	}
	h.activate(t, "Review")
	stale := d.review.planID
	for _, change := range []struct {
		app *application.Service
		id  domain.AccountID
	}{{app, ids.property}, {ref, refIDs.property}} {
		seed, err := change.app.Bootstrap(ctx)
		if err != nil {
			t.Fatal(err)
		}
		value, err := domain.ParseMoney("415.4321", "USD")
		if err != nil {
			t.Fatal(err)
		}
		_, err = change.app.RecordChange(ctx, domain.ValueUpdateInput{HouseholdID: seed.Household.ID, AccountID: change.id,
			NewValue: value, Reason: domain.ReasonReconciliation, EffectiveAt: now.Add(-time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
	}
	h.activate(t, "Start reconciliation")
	if d.review != nil || d.uncertain || d.problem != i18n.HoldingReconChanged || len(s.chosen(c.directory.accounts)) != 3 {
		t.Fatal("stale start did not retain a reviewable selection")
	}
	sessionCount(1)
	now = now.Add(8 * time.Minute)
	h.activate(t, "Review")
	if d.review == nil || d.review.planID == stale {
		t.Fatal("stale plan was reused")
	}
	planID := d.review.planID
	plan, err := refAPI.Preview(ctx, application.ReconciliationAction{Action: "start",
		AccountIDs: []string{refIDs.holdings.String(), refIDs.loan.String(), refIDs.property.String()}})
	if err != nil {
		t.Fatal(err)
	}
	want, err := refAPI.Commit(ctx, plan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	exactTime := now.UTC().Format(time.RFC3339Nano)
	if d.review.session.StartedAt != exactTime || want.StartedAt != exactTime {
		t.Fatalf("start timestamp differed from the shared preview clock: %s / %s", d.review.session.StartedAt, want.StartedAt)
	}
	assertMixedReconciliation(t, d.review.session, ids)
	h.activate(t, "Show checkpoints · Synthetic Manual Property")
	assertPageLabel(t, &h.router, "415.4321 USD")
	beforeAccounts, err := app.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	beforeLedger, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(17 * time.Minute)
	fault.loseNext = true
	h.activate(t, "Start reconciliation")
	if !d.uncertain || d.review.planID != planID {
		t.Fatal("lost acknowledgement released or replaced the frozen plan")
	}
	h.focused(&d.confirm)
	h.press(key.NameReturn)
	h.settle()
	sessionCount(2)
	if !slices.Equal(fault.attempts, []string{stale, planID, planID}) || c.id != planID {
		t.Fatal("retry did not open exactly one saved session")
	}
	assertMixedReconciliation(t, c.session, ids)
	h.focused(&c.back)
	h.press(key.NameReturn)
	h.settle()
	h.focused(&c.start)
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenOnboardingProfile(t, paths)
	reopened.SetClock(clock)
	persisted, err := reconciliation.NewService(reopened).Get(ctx, planID)
	if err != nil || persisted.StartedAt != exactTime {
		t.Fatalf("reopened session: %+v / %v", persisted, err)
	}
	assertMixedReconciliation(t, &persisted, ids)
	assertMixedReconciliation(t, &want, refIDs)
	old, err := reconciliation.NewService(reopened).Get(ctx, prior.ID)
	if err != nil || old.Revision != prior.Revision || len(old.Accounts) != 1 || old.Accounts[0].AccountID != ids.unrelated.String() || old.Counts.Pending != 2 {
		t.Fatalf("unrelated session changed: %+v / %v", old, err)
	}
	afterAccounts, err := reopened.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil || !reflect.DeepEqual(afterAccounts, beforeAccounts) {
		t.Fatal("starting reconciliation changed accounts")
	}
	afterLedger, err := reopened.ListActivities(ctx, 100)
	if err != nil || !reflect.DeepEqual(afterLedger, beforeLedger) {
		t.Fatal("starting reconciliation changed the journal")
	}
}

func assertMixedReconciliation(t *testing.T, session *domain.ReconciliationSession, ids reconciliationSelectionFixture) {
	t.Helper()
	if session == nil || len(session.Accounts) != 3 || session.Counts.Pending != 7 {
		t.Fatalf("expected exactly three scoped accounts and seven pending checks: %+v", session)
	}
	want := map[domain.AccountID]map[string]string{
		ids.holdings: {"scope": "", "cash": "156", "holding": "2"},
		ids.loan:     {"scope": "", "balance": "80"},
		ids.property: {"scope": "", "balance": "415.4321"},
	}
	for _, account := range session.Accounts {
		values := want[domain.AccountID(account.AccountID)]
		if values == nil || len(account.Items) != len(values) {
			t.Fatalf("unexpected account scope: %+v", account)
		}
		for _, item := range account.Items {
			value, known := values[item.Kind]
			if !known || item.AccountID != account.AccountID || item.Outcome != "pending" || item.Kind != "scope" && item.Value != value {
				t.Fatalf("unexpected captured checkpoint: %+v", item)
			}
		}
	}
}
