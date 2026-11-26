package desktop

import (
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

func linkedContractClock() time.Time { return time.Date(2026, 11, 23, 17, 0, 0, 0, time.UTC) }

func TestLinkedLockedProductTermsMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, refStorage, refPaths, refIDs := openExistingBuyFixture(t)
	id := seedTestLockedProductWithUnlock(t, app, ids.account, nil, "2026-11-22")
	refID := seedTestLockedProductWithUnlock(t, ref, refIDs.account, nil, "2026-11-22")
	app.SetClock(linkedContractClock)
	ref.SetClock(linkedContractClock)
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	session := startTestHoldingReconciliation(t, api, ids.account)
	refSession := startTestHoldingReconciliation(t, refAPI, refIDs.account)
	before, err := app.Product(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	rowsBefore, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	holdingsBefore, err := app.ListHoldings(ctx, ids.account, true)
	if err != nil {
		t.Fatal(err)
	}
	fault := &linkedProductCommitFault{holdingReconcileAPI: api}
	c := &holdingReconciliation{service: fault, ledger: app, clock: linkedContractClock}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks, tasks.holdingRecon = app, c
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(665, 1000)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	settle := func() {
		for range 5 {
			h.frame()
			tasks.workers.Wait()
		}
	}
	click := func(s string) {
		t.Helper()
		bounds := semanticBounds(t, &h.router, s, false)
		if y := bounds.Min.Add(bounds.Max).Y / 2; y < 70 || y >= h.size.Y-20 {
			h.shell.stage.Position.Offset = max(0, h.shell.stage.Position.Offset+y-235)
			settle()
		}
		h.click(s, false)
		settle()
	}
	label := func(s string) { t.Helper(); assertPageLabel(t, &h.router, s) }
	settle()
	click("Tasks & checks")
	click("Reconciliation")
	click("Open holdings reconciliation · Harbor Brokerage")
	const action, name, note = "Edit terms and rules", "Synthetic revised fixed contract", "Renewal terms reviewed separately"
	itemID := "product:" + id.String()
	entry := &c.checkpointActions(ids.account.String(), itemID).terms
	noWrite := func() {
		t.Helper()
		product, err := app.Product(ctx, id)
		links, linkErr := api.Links(ctx, session.ID)
		if err != nil || linkErr != nil || len(links) != 0 || !reflect.DeepEqual(product.Contract, before.Contract) || !reflect.DeepEqual(product.Policy, before.Policy) {
			t.Fatalf("cancel or stale preview changed the contract, policy or links: %+v %v %v", links, err, linkErr)
		}
	}
	edit := func() *holdingReconDraft {
		t.Helper()
		click(action)
		d := c.draft
		f := d.product.terms
		h.focused(&f.name.Editor)
		if d.product.id != id || d.product.link().ItemID != itemID || f.current.Revision != before.Contract.Revision || f.terms.StartOn != "2026-11-20" {
			t.Fatal("wrong product baseline")
		}
		setBankField(h, "Name", &f.name, name)
		f.note.Editor.SetText(note)
		f.days.Editor.SetText("2")
		f.fee.Editor.SetText("0.0123")
		basis := string(domain.DayBasisWeekdays)
		f.policy.DayBasis = &basis
		settle()
		return d
	}
	d := edit()
	d.product.terms.days.Editor.SetText("2.5")
	settle()
	click("Review")
	label("Enter a whole number of days.")
	d.product.terms.days.Editor.SetText("366")
	settle()
	click("Review")
	label("Could not review these terms. Check the contract and liquidity fields and try again.")
	d.product.terms.days.Editor.SetText("2")
	settle()
	click("Review")
	if d.review == nil || d.review.terms == nil || d.review.terms.After.Product.Policy.NormalExitFee.Amount != "0.0123" {
		t.Fatal("missing exact policy in review")
	}
	click("Back to details")
	h.focused(&d.product.terms.name.Editor)
	if d.review != nil || d.product.terms.name.Text() != name || d.product.terms.fee.Text() != "0.0123" {
		t.Fatal("Back lost the reviewed draft")
	}
	click("Review")
	click("Cancel")
	h.focused(entry)
	noWrite()

	d = edit()
	click("Review")
	stale := d.review.planID
	review, err := api.Preview(ctx, application.ReconciliationAction{Action: "check", SessionID: session.ID, ExpectedRevision: session.Revision,
		AccountID: ids.account.String(), ItemID: itemID, FinancialRevision: session.Accounts[0].FinancialRevision, Outcome: "skipped", Note: "Another reviewer deferred this checkpoint."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = api.Commit(ctx, review.PlanID); err != nil {
		t.Fatal(err)
	}
	h.press(key.NameReturn)
	settle()
	noWrite()
	if d.review != nil || d.product.terms.name.Text() != name || d.product.terms.fee.Text() != "0.0123" {
		t.Fatal("stale review lost its draft")
	}
	h.focused(&d.product.terms.name.Editor)
	click("Review")
	refDetail, err := ref.Product(ctx, refID)
	if err != nil {
		t.Fatal(err)
	}
	refTerms, refPolicy := productTermInputs(liquidity.FromProductDetail(refDetail).Product)
	refTerms.Name, refTerms.Note = name, productOptional(note)
	days, basis := 2, string(domain.DayBasisWeekdays)
	refPolicy.SettlementDays, refPolicy.DayBasis, refPolicy.NormalExitFee = &days, &basis, productOptional("0.0123")
	refLink := domain.ReconciliationLink{SessionID: refSession.ID, AccountID: refIDs.account.String(), ItemID: "product:" + refID.String(),
		ExpectedRevision: refSession.Revision, FinancialRevision: refSession.Accounts[0].FinancialRevision}
	want, err := refAPI.PreviewProductTerms(ctx, liquidity.UpdateProductTermsRequest{ProductID: refID.String(), ExpectedRevision: refDetail.Contract.Revision,
		ReconciliationLink: &refLink, Terms: refTerms, Policy: refPolicy})
	if err != nil || want.After == nil || d.review == nil || d.review.planID == stale {
		t.Fatalf("refresh or Wails preview failed: %+v %v", want, err)
	}
	got := d.review.terms.After.Product
	gotTerms, gotPolicy := productTermInputs(got)
	if got.Currency != "USD" || got.Principal.Amount != "125.25" || !reflect.DeepEqual(gotTerms, refTerms) || !reflect.DeepEqual(gotPolicy, refPolicy) {
		t.Fatalf("Gio preview differs from reference: %+v", got)
	}
	if err := refAPI.CommitProduct(ctx, want.PlanID); err != nil {
		t.Fatal(err)
	}
	plan := d.review.planID
	fault.loseNext = true
	h.press(key.NameReturn)
	settle()
	if !c.unresolved() {
		t.Fatal("lost acknowledgement discarded the plan")
	}
	h.press(key.NameReturn)
	settle()
	h.focused(entry)
	label("Contract terms recorded and linked.")
	if !reflect.DeepEqual(fault.plans, []string{stale, plan, plan}) || len(c.links) != 1 {
		t.Fatalf("retry or link count: %v %v", fault.plans, c.links)
	}
	saved := c.links[0]
	if saved.Family != "product_terms" || saved.OperationID != plan || saved.ItemID != itemID || saved.AccountID != ids.account.String() {
		t.Fatal("wrong linked terms receipt")
	}
	var evidence struct{ Committed domain.ProductTermsReceipt }
	if err := jsoncodec.Unmarshal(saved.Evidence, &evidence); err != nil || evidence.Committed.Contract.Name != name || evidence.Committed.Policy.NormalExitFee.CanonicalAmount() != "0.0123" {
		t.Fatalf("missing reviewed receipt in journal: %v", err)
	}

	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := refStorage.Close(); err != nil {
		t.Fatal(err)
	}
	app, _ = reopenOnboardingProfile(t, paths)
	ref, _ = reopenOnboardingProfile(t, refPaths)
	for _, copy := range []struct {
		svc     *application.Service
		product domain.ProductContractID
		account domain.AccountID
	}{{app, id, ids.account}, {ref, refID, refIDs.account}} {
		product, err := copy.svc.Product(ctx, copy.product)
		if err != nil {
			t.Fatal(err)
		}
		terms, policy := productTermInputs(liquidity.FromProductDetail(product).Product)
		if product.Contract.Revision != before.Contract.Revision+1 || product.Policy.Revision != before.Policy.Revision+1 || !reflect.DeepEqual(terms, refTerms) || !reflect.DeepEqual(policy, refPolicy) {
			t.Fatalf("reopened terms differ: %+v", product)
		}
		assertNativeTradeCash(t, copy.svc, copy.account, map[domain.CurrencyCode]string{"USD": "156"})
	}
	after, err := app.ListActivities(ctx, 100)
	holdings, holdErr := app.ListHoldings(ctx, ids.account, true)
	links, linkErr := reconciliation.NewService(app).Links(ctx, session.ID)
	if err != nil || holdErr != nil || linkErr != nil || !reflect.DeepEqual(rowsBefore, after) || !reflect.DeepEqual(holdingsBefore, holdings) || !reflect.DeepEqual(links, []domain.ReconciliationLinkedChange{saved}) {
		t.Fatalf("reopen changed financial rows or links: %v %v %v", err, holdErr, linkErr)
	}
}
