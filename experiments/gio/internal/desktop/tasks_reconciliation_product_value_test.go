package desktop

import (
	"context"
	"errors"
	"github.com/waltwang/nestworth-go/internal/testfixture"
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
)

type linkedProductCommitFault struct {
	holdingReconcileAPI
	loseNext bool
	plans    []string
}

func (s *linkedProductCommitFault) CommitProduct(ctx context.Context, id string) error {
	s.plans = append(s.plans, id)
	err := s.holdingReconcileAPI.CommitProduct(ctx, id)
	if err == nil && s.loseNext {
		s.loseNext = false
		return errors.New("synthetic lost product operation acknowledgement")
	}
	return err
}

func seedTestLockedProduct(t *testing.T, service *application.Service, account domain.AccountID, maturityInterest *string) domain.ProductContractID {
	t.Helper()
	return seedTestLockedProductWithUnlock(t, service, account, maturityInterest, "2026-12-22")
}

func seedTestLockedProductWithUnlock(t *testing.T, service *application.Service, account domain.AccountID, maturityInterest *string, unlock string) domain.ProductContractID {
	t.Helper()
	testfixture.SetClock(service, func() time.Time { return accountTestClock.Add(-time.Minute) })
	days, calendar, fee := 0, "calendar", "0"
	request := application.ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &application.RecordExistingProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "125.25", TotalCostBasis: "120", CurrentValue: "123.4567", CashExcludesProduct: true,
		Terms: application.ProductTermsInput{Kind: "locked_product", Name: "Synthetic locked product", StartOn: "2026-11-20", InterestMode: "none"},
		Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &unlock, EarlyKind: "not_allowed",
			SettlementDays: &days, DayBasis: &calendar, NormalExitFee: &fee},
	}}
	if maturityInterest != nil {
		maturity := "2026-12-22"
		terms := &request.RecordExisting.Terms
		terms.MaturityOn, terms.InterestMode, terms.MaturityInterest = &maturity, "manual_maturity_amount", maturityInterest
	}
	preview, err := service.PreviewProductOperation(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.RecordProductOperation(t.Context(), request, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if err != nil || len(receipt.ProductIDs) != 1 {
		t.Fatalf("synthetic product: %+v / %v", receipt, err)
	}
	testfixture.SetClock(service, func() time.Time { return accountTestClock })
	return receipt.ProductIDs[0]
}

func TestReconciliationLockedProductValuationMatchesWails(t *testing.T) {
	ctx := t.Context()
	app, storage, paths, ids := openExistingBuyFixture(t)
	ref, _, _, refIDs := openExistingBuyFixture(t)
	clock := func() time.Time { return accountTestClock }
	id := seedTestLockedProduct(t, app, ids.account, nil)
	refID := seedTestLockedProduct(t, ref, refIDs.account, nil)
	api, refAPI := reconciliation.NewService(app), reconciliation.NewService(ref)
	session, refSession := startTestHoldingReconciliation(t, api, ids.account), startTestHoldingReconciliation(t, refAPI, refIDs.account)
	initial, err := app.Product(ctx, id)
	if err != nil || initial.CurrentValue == nil || initial.CurrentValue.CanonicalAmount() != "123.4567" {
		t.Fatalf("missing initial product valuation: %+v / %v", initial, err)
	}
	before, err := app.ListActivities(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}

	fault := &linkedProductCommitFault{holdingReconcileAPI: api}
	tasks := newTasksPage(ctx, app, func() {})
	tasks.checks = app
	c := &holdingReconciliation{service: fault, ledger: app, clock: clock}
	tasks.holdingRecon = c
	h := newShellHarnessWithPages(t, ledgerPages{tasks: tasks, version: app.MobileDataVersion})
	h.size = image.Pt(640, 1200)
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
		if center := bounds.Min.Add(bounds.Max).Y / 2; center < 70 || center >= h.size.Y-20 {
			h.shell.stage.Position.Offset = max(0, h.shell.stage.Position.Offset+center-250)
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
	const action = "Update contract valuation"
	itemID := "product:" + id.String()
	entry := &c.checkpointActions(ids.account.String(), itemID).valuation
	edit := func(raw string) *holdingReconDraft {
		t.Helper()
		click(action)
		d := c.draft
		h.focused(&d.total.Editor)
		if d.product.id != id || d.product.link().ItemID != itemID {
			t.Fatal("the valuation opened a different product checkpoint")
		}
		label("Current contract value · 123.4567 USD")
		setBankField(h, action+" · USD", &d.total, raw)
		click("Review")
		return d
	}
	d := edit("131.12345")
	label("Could not review this contract valuation. Check the amount and try again.")
	if d.total.Text() != "131.12345" {
		t.Fatal("invalid precision cleared the valuation draft")
	}
	setBankField(h, action+" · USD", &d.total, "131.2345")
	click("Review")
	label("Contract value before · 123.4567 USD")
	label("Contract value after · 131.2345 USD")
	click("Cancel")
	h.focused(entry)
	saved, err := api.Get(ctx, session.ID)
	if err != nil || saved.Revision != session.Revision || len(c.links) != 0 || len(fault.plans) != 0 {
		t.Fatal("preview or Cancel committed a reconciliation change")
	}

	d = edit("131.2345")
	refLink := domain.ReconciliationLink{SessionID: refSession.ID, AccountID: refIDs.account.String(), ItemID: "product:" + refID.String(),
		ExpectedRevision: refSession.Revision, FinancialRevision: refSession.Accounts[0].FinancialRevision}
	want, err := refAPI.PreviewProductValuation(ctx, refLink, refID.String(), "131.2345", "")
	if err != nil || d.review == nil || want.Valuation == nil ||
		!reflect.DeepEqual(d.review.valuation.CurrentValueBefore, want.Valuation.CurrentValueBefore) ||
		!reflect.DeepEqual(d.review.valuation.CurrentValueAfter, want.Valuation.CurrentValueAfter) ||
		!reflect.DeepEqual(d.review.valuation.NetWorthDelta, want.Valuation.NetWorthDelta) ||
		d.review.valuation.Command.ObservedAt != want.Valuation.Command.ObservedAt {
		t.Fatalf("Gio valuation review differs from Wails: %+v / %v", want, err)
	}
	if err := refAPI.CommitProduct(ctx, want.PlanID); err != nil {
		t.Fatal(err)
	}

	fault.loseNext = true
	h.focused(&d.confirm)
	h.press(key.NameReturn)
	settle()
	if !c.unresolved() {
		t.Fatal("ambiguous acknowledgement lost the exact product plan")
	}
	h.focused(&d.confirm)
	h.press(key.NameReturn)
	settle()
	h.focused(entry)
	if len(fault.plans) != 2 || fault.plans[0] != fault.plans[1] || len(c.links) != 1 ||
		c.links[0].ItemID != itemID || c.links[0].OperationID != fault.plans[0] || c.links[0].Family != "product_operation" {
		t.Fatalf("valuation retry changed the plan or link identity: %+v / %+v", fault.plans, c.links)
	}
	link := c.links[0]
	var evidence holdingLinkedEvidence
	if err := jsoncodec.Unmarshal(link.Evidence, &evidence); err != nil || evidence.Before == nil ||
		evidence.Before.ProductID != id.String() || evidence.productValuationReceipt() == nil ||
		evidence.productValuationReceipt().Amount.CanonicalAmount() != "131.2345" {
		t.Fatalf("wrong linked observation evidence: %+v / %v", evidence, err)
	}
	click("Linked changes")
	label("Observed contract value · 131.2345 USD")
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := reopenBaselineProfile(t, paths)
	got, err := reopened.Product(ctx, id)
	if err != nil || !reflect.DeepEqual(got.Contract, initial.Contract) || got.CurrentValue == nil || got.CurrentValue.CanonicalAmount() != "131.2345" {
		t.Fatalf("reopened wrong value or changed the product contract: %+v / %v", got, err)
	}
	rows, err := reopened.ListActivities(ctx, 100)
	if err != nil || !reflect.DeepEqual(rows, before) {
		t.Fatalf("valuation changed ledger activities: %v", err)
	}
	links, err := reconciliation.NewService(reopened).Links(ctx, session.ID)
	if err != nil || len(links) != 1 || !reflect.DeepEqual(links[0], link) {
		t.Fatalf("reopen lost the linked product observation: %+v / %v", links, err)
	}
	assertNativeTradeCash(t, reopened, ids.account, map[domain.CurrencyCode]string{"USD": "156"})
}
