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

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	reconciliation "github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

type statementTestAdapter struct {
	*reconciliation.Service
	previews       []application.ReconciliationAction
	commits        []string
	started, allow chan struct{}
	failRead       atomic.Bool
}

func (s *statementTestAdapter) Preview(ctx context.Context, in application.ReconciliationAction) (reconciliation.SessionPreview, error) {
	s.previews = append(s.previews, in)
	return s.Service.Preview(ctx, in)
}

func (s *statementTestAdapter) Commit(ctx context.Context, plan string) (domain.ReconciliationSession, error) {
	s.commits = append(s.commits, plan)
	attempt := len(s.commits)
	if attempt == 3 {
		close(s.started)
		<-s.allow
	}
	record, err := s.Service.Commit(ctx, plan)
	if err == nil && attempt == 3 {
		return domain.ReconciliationSession{}, errors.New("test: lost reconciliation reply")
	}
	if err == nil && attempt == 4 {
		s.failRead.Store(true)
	}
	return record, err
}

func (s *statementTestAdapter) Get(ctx context.Context, id string) (domain.ReconciliationSession, error) {
	if s.failRead.Swap(false) {
		return domain.ReconciliationSession{}, errors.New("test: failed to read the acknowledged reconciliation")
	}
	return s.Service.Get(ctx, id)
}

func TestCreditCardStatementMatchesWailsAndKeepsEvidence(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, _ := seedBankBalanceFixture(t, true)
	ref, _, _, _ := seedBankBalanceFixture(t, true)
	card, refCard := syntheticCardID(t, app), syntheticCardID(t, ref)
	reference := reconciliation.NewService(ref)
	fault := &statementTestAdapter{Service: reconciliation.NewService(app), started: make(chan struct{}), allow: make(chan struct{})}
	release := sync.OnceFunc(func() { close(fault.allow) })
	t.Cleanup(release)
	p, h, settle := statementHarness(t, app, fault)
	click := func(label string) { t.Helper(); h.click(label, false); settle() }
	assertLabel := func(label string) { assertPageLabel(t, &h.router, label) }

	click("Statement reconciliation")
	d := p.statement.screen
	assertLabel("No reconciliation records for this card.")
	click("Start statement check")
	assertLabel("Recorded amount owed · 30 USD (balance -30 USD)")
	if got, err := fault.List(ctx); err != nil || len(got) != 0 {
		t.Fatalf("start preview created a visible session: %+v / %v", got, err)
	}
	click("Confirm review")
	sessionID := d.selected
	refStart := commitStatementAction(t, reference, application.ReconciliationAction{Action: "start", AccountIDs: []string{refCard.String()}})
	assertLabel("Record completeness · Pending")
	assertLabel("Credit-card balance · Pending")

	click("Check balance and completeness")
	assertLabel("Recorded amount owed · 30 USD (balance -30 USD)")
	previewButton := assertPageLabel(t, &h.router, "Review statement check")
	h.clickAt(previewButton.Min.Add(previewButton.Max).Div(2))
	settle()
	if d.review != nil || len(fault.previews) != 1 {
		t.Fatal("the account could be reviewed without affirming completeness")
	}
	h.click("Statement as-of date", true)
	settle()
	h.press(key.NameRightArrow)
	h.press(key.NameReturn)
	if d.form.day != "2026-11-22" {
		t.Fatalf("a future date escaped the ledger timezone bound: %s", d.form.day)
	}
	h.click("Statement as-of date", true)
	settle()
	h.press(key.NameUpArrow)
	h.press(key.NameReturn)
	h.click("Statement time (optional)", true)
	settle()
	h.press(key.NameDownArrow)
	h.press(key.NameReturn)
	h.press(key.NameEscape)
	if d.form.day != "2026-11-15" || d.form.time != "01:00" {
		t.Fatalf("source pickers chose %s %s", d.form.day, d.form.time)
	}
	setBankField(h, "Statement reference (optional)", &d.form.reference, "Synthetic November PDF")
	setBankField(h, "Check note (optional)", &d.form.note, "Reviewed the sample statement")
	completeness := "I checked the balance and checked for missing or unsettled entries."
	bounds := assertPageLabel(t, &h.router, completeness)
	h.clickAt(image.Pt(bounds.Min.X-16, bounds.Min.Y+10))
	settle()
	if !d.form.completeness.Value {
		t.Fatalf("could not affirm completeness through the control: %v", bounds)
	}
	click("Review statement check")
	assertLabel("Statement reference · Synthetic November PDF")
	click("Edit evidence")
	if d.form.reference.Text() != "Synthetic November PDF" || d.form.day != "2026-11-15" {
		t.Fatal("returning to edit discarded evidence")
	}
	click("Review statement check")

	// A real, independent financial write between review and confirmation
	// changes the working value. It is not part of the reconciliation.
	for _, side := range []struct {
		ledger *application.Service
		id     domain.AccountID
	}{{app, card}, {ref, refCard}} {
		if _, err := historyapi.NewService(side.ledger).RecordChange(ctx, wailsBalanceAdjustment(side.id, "35", accountTestClock)); err != nil {
			t.Fatal(err)
		}
	}
	click("Confirm review")
	assertLabel("The account or session changed. Review the refreshed checks before confirming again.")
	assertLabel("Recorded amount owed · 35 USD (balance -35 USD)")
	if d.form.completeness.Value || d.form.reference.Text() != "Synthetic November PDF" {
		t.Fatal("a stale review kept the old attestation or discarded the source")
	}
	bounds = assertPageLabel(t, &h.router, completeness)
	h.clickAt(image.Pt(bounds.Min.X-16, bounds.Min.Y+10))
	settle()
	if !d.form.completeness.Value {
		t.Fatalf("could not affirm completeness through the control: %v", bounds)
	}
	click("Review statement check")
	assertLabel("Recorded amount owed · 35 USD (balance -35 USD)")
	bound := semanticBounds(t, &h.router, "Confirm review", false)
	point := bound.Min.Add(bound.Max).Div(2)
	h.clickAt(point)
	select {
	case <-fault.started:
	case <-time.After(3 * time.Second):
		t.Fatal("session submission did not start")
	}
	h.clickAt(point)
	release()
	settle()
	assertLabel("Saving could not be confirmed. Retry the same submission.")
	click("Retry same submission")
	assertLabel("Reconciliation saved.")
	assertLabel("Could not load statement reconciliation.")
	click("Retry")
	assertLabel("Credit-card balance · Checked")
	if len(fault.commits) != 4 || fault.commits[1] == fault.commits[2] || fault.commits[2] != fault.commits[3] {
		t.Fatalf("wrong stale/retry identities or read retry wrote: %v", fault.commits)
	}

	refWorking, err := reference.Get(ctx, refStart.ID)
	if err != nil {
		t.Fatal(err)
	}
	check := fault.previews[len(fault.previews)-1]
	check.SessionID, check.AccountID = refWorking.ID, refCard.String()
	check.ExpectedRevision, check.FinancialRevision = refWorking.Revision, refWorking.Accounts[0].FinancialRevision
	previous := commitStatementAction(t, reference, check)
	click("Finish reconciliation")
	assertLabel("Completed")
	click("Confirm review")
	assertLabel("Completed")
	assertLabel("Statement reference · Synthetic November PDF")
	for _, button := range []string{"Check balance and completeness", "Finish reconciliation", "Edit evidence"} {
		if findPageLabel(&h.router, button) != nil {
			t.Fatalf("completed session offered %q", button)
		}
	}
	refCompleted := commitStatementAction(t, reference, application.ReconciliationAction{
		Action: "finish", SessionID: previous.ID, ExpectedRevision: previous.Revision,
	})
	if _, err := reference.Preview(ctx, check); err == nil {
		t.Fatal("Wails accepted an edit to a completed record")
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	reopenedService := reconciliation.NewService(reopened)
	record, err := reopenedService.LastSuccess(ctx, card.String())
	if err != nil || record == nil || record.ID != sessionID {
		t.Fatalf("completed reconciliation missing after reopen: %+v / %v", record, err)
	}
	twice, err := reopenedService.Commit(ctx, fault.commits[2])
	if err != nil || twice.ID != sessionID || twice.Revision != previous.Revision {
		t.Fatalf("reopening did not recover the original check receipt: %+v / %v", twice, err)
	}
	if record.Status != "completed" || record.Revision != refCompleted.Revision || record.Counts.Confirmed != 2 ||
		record.StartedAt != refCompleted.StartedAt || record.CompletedAt != refCompleted.CompletedAt {
		t.Fatalf("completed session differs from Wails: %+v / %+v", record, refCompleted)
	}
	for i, item := range record.Accounts[0].Items {
		want := refCompleted.Accounts[0].Items[i]
		if item.Kind != want.Kind || item.Value != want.Value || item.Source != want.Source || item.CheckedAt != want.CheckedAt ||
			len(item.Attempts) != 1 || len(want.Attempts) != 1 || !reflect.DeepEqual(item.Attempts, want.Attempts) ||
			item.Source.AsOfDate != "2026-11-15" || item.Source.AsOfTime != "01:00" || item.Source.Timezone != "Asia/Singapore" || item.ReferenceComparable {
			t.Fatalf("stored checkpoint differs from Wails: %+v / %+v", item, want)
		}
	}
	assertBalanceTargetNetWorth(t, reopened, ref, "65", "35")
	assertStatementLedgerUnchanged(t, reopened, ref)

	if _, err := historyapi.NewService(reopened).RecordChange(ctx,
		wailsBalanceAdjustment(card, "40", accountTestClock.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	history, err := reopenedService.LastSuccess(ctx, card.String())
	if err != nil || history == nil || history.ID != sessionID || !history.Accounts[0].RecheckRequired ||
		!reflect.DeepEqual(history.Accounts[0].Items, record.Accounts[0].Items) {
		t.Fatalf("later bookkeeping did not preserve the completed evidence: %+v / %v", history, err)
	}
}

func TestCreditCardStatementShowsExistingSharedSessionWithoutEditingIt(t *testing.T) {
	app, _, _, bank := seedBankBalanceFixture(t, false)
	card := syntheticCardID(t, app)
	adapter := reconciliation.NewService(app)
	commitStatementAction(t, adapter, application.ReconciliationAction{Action: "start", AccountIDs: []string{bank.String()}})
	combined := commitStatementAction(t, adapter, application.ReconciliationAction{
		Action: "start", AccountIDs: []string{bank.String(), card.String()},
	})

	p, h, settle := statementHarness(t, app, adapter)
	h.click("Statement reconciliation", false)
	settle()
	d := p.statement.screen
	if len(d.data.records) != 1 || d.data.records[0].ID != combined.ID {
		t.Fatalf("expected only existing sessions containing the card: %+v", d.data.records)
	}
	d.records[combined.ID].Click()
	settle()
	assertPageLabel(t, &h.router, "View only here. Use the full reconciliation workflow to resolve outstanding checks.")
	assertPageLabel(t, &h.router, "Recorded amount owed · 30 USD (balance -30 USD)")
	if findPageLabel(&h.router, "Check balance and completeness") != nil {
		t.Fatal("the card-only entry could edit an existing multi-account session")
	}
}

func statementHarness(t *testing.T, service *application.Service, reconciliation statementSessions) (*accountsPage, *previewHarness, func()) {
	t.Helper()
	p := newAccountsPage(t.Context(), service, func() time.Time { return accountTestClock }, func() {})
	p.statement = &accountStatement{service: reconciliation, origin: service}
	h := newShellHarnessWithPages(t, ledgerPages{accounts: p})
	h.size = image.Pt(900, 660)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() { h.frame(); p.workers.Wait(); h.frame(); p.workers.Wait(); h.frame() }
	h.frame()
	h.click("Accounts", false)
	settle()
	h.click("Synthetic Card", false)
	settle()
	return p, h, settle
}

func commitStatementAction(t *testing.T, api *reconciliation.Service, action application.ReconciliationAction) domain.ReconciliationSession {
	t.Helper()
	ctx := t.Context()
	plan, err := api.Preview(ctx, action)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := api.Commit(ctx, plan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func assertStatementLedgerUnchanged(t *testing.T, gio, ref *application.Service) {
	t.Helper()
	ctx := t.Context()
	setBalanceTargetClock(72*time.Hour, gio, ref)
	request := analysis.AnalysisQueryRequest{ScopeKind: string(domain.ScopeHousehold), From: "2026-11-21", To: "2026-11-22",
		Valuation: string(domain.ValuationBase), Basis: string(domain.ReturnBasisInvestment), IncludeCash: true}
	for _, ledger := range []*application.Service{gio, ref} {
		activities, err := ledger.ListActivities(ctx, 50)
		if err != nil || len(activities) != 1 || activities[0].Kind != domain.ActivityValueUpdate || activities[0].Reason != domain.ReasonReconciliation {
			t.Fatalf("statement check changed ledger history: %+v / %v", activities, err)
		}
		result, err := analysis.NewService(ledger).AssetChange(ctx, request)
		if err != nil || result.Status != string(domain.CompletenessOK) {
			t.Fatalf("incomplete Asset Changes: %s / %v", result.Status, err)
		}
		var adjustment string
		for _, row := range result.Waterfall {
			if row.Bucket == string(domain.BucketExternalFlow) && row.Amount != nil && row.Amount.Amount != "0" {
				t.Fatalf("statement check changed external flow: %+v", row)
			}
			if row.Bucket == string(domain.BucketAdjustment) && row.Amount != nil {
				adjustment = row.Amount.Amount
			}
		}
		if adjustment != "-5" {
			t.Fatalf("expected only the USD -5 control adjustment, got %s", adjustment)
		}
	}
}
